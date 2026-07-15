package provider

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestCreateProviderRoutesEasyPayProtocolVersion(t *testing.T) {
	merchantKey := mustGenerateEasyPayV2RSAKey(t)
	platformKey := mustGenerateEasyPayV2RSAKey(t)

	v1Config := map[string]string{
		"pid": "pid-v1", "pkey": "secret", "apiBase": "https://pay.example.com",
		"notifyUrl": "https://merchant.example.com/notify", "returnUrl": "https://merchant.example.com/return",
	}
	providerV1, err := CreateProvider(payment.TypeEasyPay, "v1", v1Config)
	if err != nil {
		t.Fatalf("CreateProvider legacy V1: %v", err)
	}
	if _, ok := providerV1.(*EasyPay); !ok {
		t.Fatalf("legacy EasyPay type = %T, want *EasyPay", providerV1)
	}
	identityV1, ok := providerV1.(payment.MerchantIdentityProvider)
	if !ok {
		t.Fatalf("legacy EasyPay type %T does not expose merchant identity", providerV1)
	}
	if got := identityV1.MerchantIdentityMetadata()["protocol_version"]; got != "1" {
		t.Fatalf("legacy protocol metadata = %q, want 1", got)
	}

	v1Config["protocolVersion"] = " 1 "
	providerV1, err = CreateProvider(payment.TypeEasyPay, "v1-explicit", v1Config)
	if err != nil {
		t.Fatalf("CreateProvider explicit V1: %v", err)
	}
	if _, ok := providerV1.(*EasyPay); !ok {
		t.Fatalf("explicit EasyPay type = %T, want *EasyPay", providerV1)
	}

	v2Config := easyPayV2TestConfig(t, "https://pay.example.com", merchantKey, &platformKey.PublicKey)
	providerV2, err := CreateProvider(payment.TypeEasyPay, "v2", v2Config)
	if err != nil {
		t.Fatalf("CreateProvider V2: %v", err)
	}
	if _, ok := providerV2.(*EasyPayV2); !ok {
		t.Fatalf("V2 EasyPay type = %T, want *EasyPayV2", providerV2)
	}
	identityV2, ok := providerV2.(payment.MerchantIdentityProvider)
	if !ok {
		t.Fatalf("V2 EasyPay type %T does not expose merchant identity", providerV2)
	}
	metadata := identityV2.MerchantIdentityMetadata()
	if metadata["pid"] != "pid-v2" || metadata["protocol_version"] != "2" {
		t.Fatalf("V2 metadata = %#v", metadata)
	}
}

func TestCreateProviderRejectsInvalidEasyPayProtocolVersionWithConfigError(t *testing.T) {
	_, err := CreateProvider(payment.TypeEasyPay, "bad", map[string]string{"protocolVersion": "3"})
	if err == nil {
		t.Fatal("CreateProvider returned nil error")
	}
	var configErr *ProviderConfigError
	if !errors.As(err, &configErr) {
		t.Fatalf("error type = %T, want *ProviderConfigError", err)
	}
	if configErr.ProviderKey != payment.TypeEasyPay || configErr.Field != "protocolVersion" {
		t.Fatalf("config error = %#v", configErr)
	}
}

func TestNewEasyPayV2ReturnsStructuredConfigErrors(t *testing.T) {
	merchantKey := mustGenerateEasyPayV2RSAKey(t)
	platformKey := mustGenerateEasyPayV2RSAKey(t)
	base := easyPayV2TestConfig(t, "https://pay.example.com", merchantKey, &platformKey.PublicKey)
	tests := []struct {
		name  string
		field string
		value string
	}{
		{name: "missing pid", field: "pid", value: ""},
		{name: "invalid api base", field: "apiBase", value: "file:///tmp/pay"},
		{name: "invalid private key", field: "merchantPrivateKey", value: "not-a-key"},
		{name: "invalid public key", field: "platformPublicKey", value: "not-a-key"},
		{name: "invalid custom methods", field: "customMethods", value: "{"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := cloneStringMap(base)
			config[test.field] = test.value
			_, err := NewEasyPayV2("v2", config)
			var configErr *ProviderConfigError
			if !errors.As(err, &configErr) || configErr.Field != test.field {
				t.Fatalf("error = %#v, want config field %q", err, test.field)
			}
		})
	}
}

func TestEasyPayV2SupportedTypesIncludeQQPayAndCustomMethods(t *testing.T) {
	merchantKey := mustGenerateEasyPayV2RSAKey(t)
	platformKey := mustGenerateEasyPayV2RSAKey(t)
	config := easyPayV2TestConfig(t, "https://pay.example.com", merchantKey, &platformKey.PublicKey)
	config["customMethods"] = `[{"type":"usdt_trc20","upstreamType":"usdt","displayName":"USDT"},{"type":"qqpay","upstreamType":"custom-qq","displayName":"ignored duplicate"}]`

	provider, err := NewEasyPayV2("v2", config)
	if err != nil {
		t.Fatalf("NewEasyPayV2: %v", err)
	}
	got := provider.SupportedTypes()
	want := []payment.PaymentType{payment.TypeAlipay, payment.TypeWxpay, payment.TypeQQPay, "usdt_trc20"}
	if len(got) != len(want) {
		t.Fatalf("SupportedTypes = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("SupportedTypes = %#v, want %#v", got, want)
		}
	}
	if upstream, ok := provider.resolvePaymentType(payment.TypeQQPay); !ok || upstream != payment.TypeQQPay {
		t.Fatalf("qqpay mapping = %q, %v; want built-in qqpay", upstream, ok)
	}
}

func TestEasyPayV1KeepsCustomQQPayMapping(t *testing.T) {
	config := map[string]string{
		"protocolVersion": "1", "pid": "pid-v1", "pkey": "secret", "apiBase": "https://pay.example.com",
		"notifyUrl": "https://merchant.example.com/notify", "returnUrl": "https://merchant.example.com/return",
		"customMethods": `[{"type":"qqpay","upstreamType":"qq_wallet","displayName":"QQ Wallet"}]`,
	}
	provider, err := NewEasyPay("v1", config)
	if err != nil {
		t.Fatalf("NewEasyPay: %v", err)
	}
	if got := provider.upstreamPaymentType(payment.TypeQQPay); got != "qq_wallet" {
		t.Fatalf("V1 qqpay upstream type = %q", got)
	}
	found := false
	for _, paymentType := range provider.SupportedTypes() {
		if paymentType == payment.TypeQQPay {
			found = true
		}
	}
	if !found {
		t.Fatalf("V1 SupportedTypes = %#v, want qqpay custom mapping", provider.SupportedTypes())
	}
}

func TestGetBasePaymentTypeKeepsQQPayIndependent(t *testing.T) {
	if got := payment.GetBasePaymentType(payment.TypeQQPay); got != payment.TypeQQPay {
		t.Fatalf("GetBasePaymentType(qqpay) = %q", got)
	}
}

func mustGenerateEasyPayV2RSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return key
}

func easyPayV2TestConfig(t *testing.T, apiBase string, merchantKey *rsa.PrivateKey, platformKey *rsa.PublicKey) map[string]string {
	t.Helper()
	privateDER, err := x509.MarshalPKCS8PrivateKey(merchantKey)
	if err != nil {
		t.Fatalf("marshal PKCS8 key: %v", err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(platformKey)
	if err != nil {
		t.Fatalf("marshal PKIX key: %v", err)
	}
	return map[string]string{
		"protocolVersion":    easyPayV2ProtocolVersion,
		"pid":                "pid-v2",
		"apiBase":            apiBase,
		"merchantPrivateKey": base64.StdEncoding.EncodeToString(privateDER),
		"platformPublicKey":  base64.StdEncoding.EncodeToString(publicDER),
		"notifyUrl":          "https://merchant.example.com/notify",
		"returnUrl":          "https://merchant.example.com/return",
		"paymentMode":        "qrcode",
	}
}

package provider

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"
)

func TestEasyPayV2SignContentMatchesSDKRules(t *testing.T) {
	params := map[string]string{
		"z":         "最后",
		"a":         "a&b=c +/%",
		"middle":    "  保留两侧空格  ",
		"empty":     " \t\r\n\v\x00",
		"sign":      "ignored",
		"sign_type": "RSA",
	}
	want := "a=a&b=c +/%&middle=  保留两侧空格  &z=最后"
	if got := easyPayV2SignContent(params); got != want {
		t.Fatalf("sign content = %q, want %q", got, want)
	}
}

func TestEasyPayV2RSASignAndVerify(t *testing.T) {
	privateKey := mustGenerateEasyPayV2RSAKey(t)
	params := map[string]string{
		"pid":          "1000",
		"timestamp":    "1721050000",
		"out_trade_no": "订单-123",
		"money":        "1.00",
	}
	signature1, err := easyPayV2RSASign(params, privateKey)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	signature2, err := easyPayV2RSASign(params, privateKey)
	if err != nil {
		t.Fatalf("sign again: %v", err)
	}
	if signature1 != signature2 {
		t.Fatalf("PKCS#1 v1.5 signature must be deterministic: %q != %q", signature1, signature2)
	}
	params["sign"] = signature1
	params["sign_type"] = easyPayV2SignType
	if err := easyPayV2RSAVerify(params, signature1, &privateKey.PublicKey); err != nil {
		t.Fatalf("verify: %v", err)
	}
	params["money"] = "2.00"
	if err := easyPayV2RSAVerify(params, signature1, &privateKey.PublicKey); err == nil {
		t.Fatal("tampered params unexpectedly verified")
	}
}

func TestEasyPayV2KeyParsingSupportsSDKBase64PEMAndPKCS1(t *testing.T) {
	privateKey := mustGenerateEasyPayV2RSAKey(t)
	pkcs8DER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal PKCS8: %v", err)
	}
	pkixDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal PKIX: %v", err)
	}

	privateInputs := []string{
		base64.StdEncoding.EncodeToString(pkcs8DER),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8DER})),
		base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PrivateKey(privateKey)),
		string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})),
	}
	for index, input := range privateInputs {
		parsed, err := parseEasyPayV2PrivateKey(input)
		if err != nil {
			t.Fatalf("private input %d: %v", index, err)
		}
		assertSameEasyPayV2PublicKey(t, &parsed.PublicKey, &privateKey.PublicKey)
	}

	publicInputs := []string{
		base64.StdEncoding.EncodeToString(pkixDER),
		string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pkixDER})),
		base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&privateKey.PublicKey)),
		string(pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&privateKey.PublicKey)})),
	}
	for index, input := range publicInputs {
		parsed, err := parseEasyPayV2PublicKey(input)
		if err != nil {
			t.Fatalf("public input %d: %v", index, err)
		}
		assertSameEasyPayV2PublicKey(t, parsed, &privateKey.PublicKey)
	}
}

func assertSameEasyPayV2PublicKey(t *testing.T, got, want *rsa.PublicKey) {
	t.Helper()
	if got.E != want.E || got.N.Cmp(want.N) != 0 {
		t.Fatalf("RSA public key mismatch")
	}
}

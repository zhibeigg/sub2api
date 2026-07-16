package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestUsesOfficialWxpayVisibleMethodDerivesFromEnabledProviderInstance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeWxpay).
		SetName("Official WeChat").
		SetConfig("{}").
		SetSupportedTypes("wxpay").
		SetEnabled(true).
		SetSortOrder(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("create official wxpay instance: %v", err)
	}

	svc := &PaymentService{
		configService: &PaymentConfigService{entClient: client},
	}

	if !svc.usesOfficialWxpayVisibleMethod(ctx) {
		t.Fatal("expected official wxpay visible method to be detected from enabled provider instance")
	}
}

func TestPrepareCreateOrderSelectionContextAllowsNativeFallbackWithoutOAuthCredential(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeWxpay).
		SetName("Official WeChat").
		SetConfig(`{"appId":"wx-pay-app","nativeEnabled":"true","h5Enabled":"false","jsapiEnabled":"true"}`).
		SetSupportedTypes("wxpay").
		SetEnabled(true).
		SetSortOrder(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("create official wxpay instance: %v", err)
	}

	svc := &PaymentService{configService: &PaymentConfigService{entClient: client}}
	prepared, err := svc.prepareCreateOrderSelectionContext(ctx, CreateOrderRequest{
		PaymentType:     payment.TypeWxpay,
		IsWeChatBrowser: true,
	})
	if err != nil {
		t.Fatalf("expected native fallback without oauth credential, got %v", err)
	}
	if prepared == nil {
		t.Fatal("expected original selection context")
	}

	_, err = svc.prepareCreateOrderSelectionContext(ctx, CreateOrderRequest{
		PaymentType: payment.TypeWxpay,
		OpenID:      "openid-requires-jsapi",
	})
	if err == nil {
		t.Fatal("expected OpenID request to require OAuth-compatible JSAPI configuration")
	}
}

func TestOfficialWxpayVisibleMethodSupportsJSAPIUsesInstanceCapabilities(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   bool
	}{
		{name: "explicitly disabled", config: `{"nativeEnabled":"true","h5Enabled":"false","jsapiEnabled":"false"}`, want: false},
		{name: "historical mp app id infers enabled", config: `{"mpAppId":"wx-mp-app"}`, want: true},
		{name: "explicit false overrides historical mp app id", config: `{"mpAppId":"wx-mp-app","jsapiEnabled":"false"}`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			_, err := client.PaymentProviderInstance.Create().
				SetProviderKey(payment.TypeWxpay).
				SetName("Official WeChat").
				SetConfig(tt.config).
				SetSupportedTypes("wxpay").
				SetEnabled(true).
				SetSortOrder(1).
				Save(ctx)
			if err != nil {
				t.Fatalf("create official wxpay instance: %v", err)
			}

			svc := &PaymentService{configService: &PaymentConfigService{entClient: client}}
			got, err := svc.officialWxpayVisibleMethodSupportsJSAPI(ctx)
			if err != nil {
				t.Fatalf("officialWxpayVisibleMethodSupportsJSAPI() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("officialWxpayVisibleMethodSupportsJSAPI() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUsesOfficialWxpayVisibleMethodRespectsConfiguredSourceWhenMultipleProvidersEnabled(t *testing.T) {
	tests := []struct {
		name         string
		source       string
		wantOfficial bool
	}{
		{
			name:         "official source selected",
			source:       VisibleMethodSourceOfficialWechat,
			wantOfficial: true,
		},
		{
			name:         "easypay source selected",
			source:       VisibleMethodSourceEasyPayWechat,
			wantOfficial: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)

			_, err := client.PaymentProviderInstance.Create().
				SetProviderKey(payment.TypeWxpay).
				SetName("Official WeChat").
				SetConfig("{}").
				SetSupportedTypes("wxpay").
				SetEnabled(true).
				SetSortOrder(1).
				Save(ctx)
			if err != nil {
				t.Fatalf("create official wxpay instance: %v", err)
			}

			_, err = client.PaymentProviderInstance.Create().
				SetProviderKey(payment.TypeEasyPay).
				SetName("EasyPay WeChat").
				SetConfig("{}").
				SetSupportedTypes("wxpay").
				SetEnabled(true).
				SetSortOrder(2).
				Save(ctx)
			if err != nil {
				t.Fatalf("create easypay wxpay instance: %v", err)
			}

			svc := &PaymentService{
				configService: &PaymentConfigService{
					entClient: client,
					settingRepo: &paymentConfigSettingRepoStub{
						values: map[string]string{
							SettingPaymentVisibleMethodWxpaySource: tt.source,
						},
					},
				},
			}

			if got := svc.usesOfficialWxpayVisibleMethod(ctx); got != tt.wantOfficial {
				t.Fatalf("usesOfficialWxpayVisibleMethod() = %v, want %v", got, tt.wantOfficial)
			}
		})
	}
}

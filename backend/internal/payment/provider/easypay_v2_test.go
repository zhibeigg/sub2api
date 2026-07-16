package provider

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

var easyPayV2FixedTime = time.Unix(1721050000, 0)

func TestEasyPayV2CreatePaymentContractAndResultMapping(t *testing.T) {
	tests := []struct {
		name            string
		paymentMode     string
		paymentType     string
		isMobile        bool
		responsePayType string
		responsePayInfo string
		wantMethod      string
		wantDevice      string
		wantPayURL      string
		wantQRCode      string
	}{
		{name: "qqpay qrcode without response sign type", paymentMode: "qrcode", paymentType: payment.TypeQQPay, responsePayType: "qrcode", responsePayInfo: "https://qr.example/qqpay", wantMethod: "web", wantDevice: "pc", wantQRCode: "https://qr.example/qqpay"},
		{name: "wxpay popup without response sign type", paymentMode: "popup", paymentType: payment.TypeWxpay, isMobile: true, responsePayType: "jump", responsePayInfo: "https://cashier.example/wxpay", wantMethod: "jump", wantDevice: "mobile", wantPayURL: "https://cashier.example/wxpay"},
		{name: "qqpay urlscheme", paymentMode: "qrcode", paymentType: payment.TypeQQPay, isMobile: true, responsePayType: "urlscheme", responsePayInfo: "mqqapi://wallet/pay?token=abc", wantMethod: "web", wantDevice: "mobile", wantPayURL: "mqqapi://wallet/pay?token=abc"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			merchantKey := mustGenerateEasyPayV2RSAKey(t)
			platformKey := mustGenerateEasyPayV2RSAKey(t)
			var requestForm url.Values
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != easyPayV2CreatePath {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if err := r.ParseForm(); err != nil {
					t.Errorf("ParseForm: %v", err)
					return
				}
				requestForm = r.PostForm
				if err := easyPayV2RSAVerify(formToEasyPayV2Map(r.PostForm), r.PostForm.Get("sign"), &merchantKey.PublicKey); err != nil {
					t.Errorf("request signature: %v", err)
				}
				writeEasyPayV2SignedJSON(t, w, platformKey, map[string]string{
					"code": "0", "msg": "success",
					"timestamp": "1721050000",
					"trade_no":  "gateway-123", "pay_type": test.responsePayType, "pay_info": test.responsePayInfo,
				})
			}))
			defer server.Close()

			config := easyPayV2TestConfig(t, server.URL+"/", merchantKey, &platformKey.PublicKey)
			config["paymentMode"] = test.paymentMode
			provider, err := NewEasyPayV2("instance-v2", config)
			if err != nil {
				t.Fatalf("NewEasyPayV2: %v", err)
			}
			provider.now = func() time.Time { return easyPayV2FixedTime }

			response, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
				OrderID: "order-123", Amount: "12.34", PaymentType: test.paymentType,
				Subject: "测试 & 商品", ClientIP: "203.0.113.10", IsMobile: test.isMobile,
			})
			if err != nil {
				t.Fatalf("CreatePayment: %v", err)
			}
			if response.TradeNo != "gateway-123" || response.PayURL != test.wantPayURL || response.QRCode != test.wantQRCode {
				t.Fatalf("response = %+v", response)
			}
			for key, want := range map[string]string{
				"pid": "pid-v2", "sign_type": "RSA", "timestamp": "1721050000",
				"method": test.wantMethod, "device": test.wantDevice, "type": test.paymentType,
				"out_trade_no": "order-123", "notify_url": config["notifyUrl"], "return_url": config["returnUrl"],
				"name": "测试 & 商品", "money": "12.34", "clientip": "203.0.113.10",
			} {
				if got := requestForm.Get(key); got != want {
					t.Fatalf("form[%s] = %q, want %q (form=%v)", key, got, want, requestForm)
				}
			}
		})
	}
}

func TestEasyPayV2CreatePaymentIgnoresResponseSignTypeLabel(t *testing.T) {
	merchantKey := mustGenerateEasyPayV2RSAKey(t)
	platformKey := mustGenerateEasyPayV2RSAKey(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeEasyPayV2SignedJSON(t, w, platformKey, map[string]string{
			"code": "0", "timestamp": "1721050000", "sign_type": "UNTRUSTED-FUTURE-LABEL",
			"trade_no": "gateway-fixed-rsa", "pay_type": "qrcode", "pay_info": "https://qr.example/wxpay",
		})
	}))
	defer server.Close()

	provider := newEasyPayV2TestProvider(t, server.URL, merchantKey, &platformKey.PublicKey)
	response, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "order-fixed-rsa", Amount: "1.00", PaymentType: payment.TypeWxpay, Subject: "test",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if response.TradeNo != "gateway-fixed-rsa" || response.QRCode != "https://qr.example/wxpay" {
		t.Fatalf("response = %+v", response)
	}
}

func TestEasyPayV2CreatePaymentUsesCustomMethodAndRequestURLs(t *testing.T) {
	merchantKey := mustGenerateEasyPayV2RSAKey(t)
	platformKey := mustGenerateEasyPayV2RSAKey(t)
	var requestForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		requestForm = r.PostForm
		writeEasyPayV2SignedJSON(t, w, platformKey, map[string]string{
			"code": "0", "pid": "pid-v2", "timestamp": "1721050000",
			"trade_no": "custom-trade", "pay_type": "qrcode", "pay_info": "custom-qr",
		})
	}))
	defer server.Close()

	config := easyPayV2TestConfig(t, server.URL, merchantKey, &platformKey.PublicKey)
	config["customMethods"] = `[{"type":"usdt_trc20","upstreamType":"usdt","displayName":"USDT"}]`
	provider, err := NewEasyPayV2("instance-v2", config)
	if err != nil {
		t.Fatalf("NewEasyPayV2: %v", err)
	}
	provider.now = func() time.Time { return easyPayV2FixedTime }
	_, err = provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "custom-order", Amount: "1.00", PaymentType: "usdt_trc20", Subject: "USDT",
		NotifyURL: "https://override.example/notify", ReturnURL: "https://override.example/return",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if requestForm.Get("type") != "usdt" || requestForm.Get("notify_url") != "https://override.example/notify" || requestForm.Get("return_url") != "https://override.example/return" {
		t.Fatalf("form = %v", requestForm)
	}

	_, err = provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{PaymentType: "unsupported"})
	if err == nil || !strings.Contains(err.Error(), "unsupported payment type") {
		t.Fatalf("unsupported type error = %v", err)
	}
}

func TestEasyPayV2CreateRejectsUnsupportedOrUnsafePayType(t *testing.T) {
	tests := []struct {
		name     string
		payType  string
		payInfo  string
		wantText string
	}{
		{name: "html", payType: "html", payInfo: "<form>unsafe</form>", wantText: "unsupported pay_type"},
		{name: "jsapi", payType: "jsapi", payInfo: "{}", wantText: "unsupported pay_type"},
		{name: "missing pay info", payType: "qrcode", payInfo: "", wantText: "missing pay_info"},
		{name: "unsafe jump", payType: "jump", payInfo: "javascript:alert(1)", wantText: "unsafe jump"},
		{name: "unsafe scheme", payType: "urlscheme", payInfo: "data:text/html,bad", wantText: "unsafe urlscheme"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			merchantKey := mustGenerateEasyPayV2RSAKey(t)
			platformKey := mustGenerateEasyPayV2RSAKey(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeEasyPayV2SignedJSON(t, w, platformKey, map[string]string{
					"code": "0", "pid": "pid-v2", "timestamp": "1721050000",
					"trade_no": "trade", "pay_type": test.payType, "pay_info": test.payInfo,
				})
			}))
			defer server.Close()
			provider := newEasyPayV2TestProvider(t, server.URL, merchantKey, &platformKey.PublicKey)
			_, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
				OrderID: "order", Amount: "1.00", PaymentType: payment.TypeAlipay, Subject: "test",
			})
			if err == nil || !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("error = %v, want %q", err, test.wantText)
			}
		})
	}
}

func TestEasyPayV2ResponseVerification(t *testing.T) {
	tests := []struct {
		name          string
		mutate        func(map[string]string)
		signingKey    func(platformKey *rsa.PrivateKey) *rsa.PrivateKey
		omitSignature bool
		wantText      string
	}{
		{name: "invalid signature", signingKey: func(_ *rsa.PrivateKey) *rsa.PrivateKey { return mustGenerateEasyPayV2RSAKey(t) }, wantText: "invalid signature"},
		{name: "missing signature", omitSignature: true, wantText: "missing sign"},
		{name: "pid mismatch", mutate: func(values map[string]string) { values["pid"] = "other" }, wantText: "pid mismatch"},
		{name: "expired timestamp", mutate: func(values map[string]string) { values["timestamp"] = "1721049699" }, wantText: "outside 300 second"},
		{name: "future timestamp", mutate: func(values map[string]string) { values["timestamp"] = "1721050301" }, wantText: "outside 300 second"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			merchantKey := mustGenerateEasyPayV2RSAKey(t)
			platformKey := mustGenerateEasyPayV2RSAKey(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				values := map[string]string{
					"code": "0", "pid": "pid-v2", "timestamp": "1721050000", "sign_type": "RSA",
					"trade_no": "trade", "pay_type": "qrcode", "pay_info": "qr",
				}
				if test.mutate != nil {
					test.mutate(values)
				}
				if test.omitSignature {
					writeEasyPayV2JSON(t, w, values)
					return
				}
				key := platformKey
				if test.signingKey != nil {
					key = test.signingKey(platformKey)
				}
				writeEasyPayV2SignedJSON(t, w, key, values)
			}))
			defer server.Close()
			provider := newEasyPayV2TestProvider(t, server.URL, merchantKey, &platformKey.PublicKey)
			_, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
				OrderID: "order", Amount: "1.00", PaymentType: payment.TypeAlipay, Subject: "test",
			})
			if err == nil || !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("error = %v, want %q", err, test.wantText)
			}
		})
	}
}

func TestEasyPayV2UpstreamAndTransportErrors(t *testing.T) {
	merchantKey := mustGenerateEasyPayV2RSAKey(t)
	platformKey := mustGenerateEasyPayV2RSAKey(t)
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantText   string
	}{
		{name: "unsigned business error", statusCode: http.StatusOK, body: `{"code":1,"msg":"merchant failed"}`, wantText: "upstream error: merchant failed"},
		{name: "invalid json", statusCode: http.StatusOK, body: `not-json`, wantText: "invalid JSON response"},
		{name: "non 2xx", statusCode: http.StatusBadGateway, body: `bad gateway`, wantText: "HTTP 502"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.statusCode)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			provider := newEasyPayV2TestProvider(t, server.URL, merchantKey, &platformKey.PublicKey)
			_, err := provider.QueryOrder(context.Background(), "order")
			if err == nil || !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("error = %v, want %q", err, test.wantText)
			}
		})
	}
}

func TestEasyPayV2UpstreamErrorRedactsRequestSignatureAndPrivateKey(t *testing.T) {
	merchantKey := mustGenerateEasyPayV2RSAKey(t)
	platformKey := mustGenerateEasyPayV2RSAKey(t)
	config := easyPayV2TestConfig(t, "https://pay.example.com", merchantKey, &platformKey.PublicKey)
	provider, err := NewEasyPayV2("instance-v2", config)
	if err != nil {
		t.Fatalf("NewEasyPayV2: %v", err)
	}
	provider.now = func() time.Time { return easyPayV2FixedTime }
	signedParams, err := provider.buildRequestParams(map[string]string{"out_trade_no": "order"})
	if err != nil {
		t.Fatalf("buildRequestParams: %v", err)
	}
	signature := signedParams["sign"]
	privateKeyText := config["merchantPrivateKey"]
	message := provider.summarizeUpstreamMessage("sign="+signature+" key="+privateKeyText, signature)
	if strings.Contains(message, privateKeyText) || strings.Contains(message, signature) {
		t.Fatalf("sensitive values were not redacted: %q", message)
	}
}

func TestEasyPayV2QueryUsesOutTradeNoAndMapsStatus(t *testing.T) {
	for _, test := range []struct {
		status string
		want   string
	}{{status: "1", want: payment.ProviderStatusPaid}, {status: "0", want: payment.ProviderStatusPending}, {status: "2", want: payment.ProviderStatusRefunded}, {status: "3", want: payment.ProviderStatusPending}, {status: "4", want: payment.ProviderStatusPending}} {
		t.Run(test.status, func(t *testing.T) {
			merchantKey := mustGenerateEasyPayV2RSAKey(t)
			platformKey := mustGenerateEasyPayV2RSAKey(t)
			var requestForm url.Values
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != easyPayV2QueryPath {
					t.Errorf("path = %q", r.URL.Path)
				}
				if err := r.ParseForm(); err != nil {
					t.Errorf("ParseForm: %v", err)
				}
				requestForm = r.PostForm
				writeEasyPayV2SignedJSON(t, w, platformKey, map[string]string{
					"code": "0", "pid": "pid-v2", "timestamp": "1721050000",
					"status": test.status, "trade_no": "gateway-query", "out_trade_no": "out-order", "money": "12.34",
				})
			}))
			defer server.Close()
			provider := newEasyPayV2TestProvider(t, server.URL, merchantKey, &platformKey.PublicKey)
			response, err := provider.QueryOrder(context.Background(), "out-order")
			if err != nil {
				t.Fatalf("QueryOrder: %v", err)
			}
			if requestForm.Get("out_trade_no") != "out-order" || requestForm.Get("trade_no") != "" {
				t.Fatalf("query form = %v", requestForm)
			}
			if response.Status != test.want || response.TradeNo != "gateway-query" || response.Amount != 12.34 {
				t.Fatalf("response = %+v", response)
			}
			if response.Metadata["protocol_version"] != "2" {
				t.Fatalf("metadata = %#v", response.Metadata)
			}
		})
	}
}

func TestEasyPayV2QueryOrderRequiresResponsePID(t *testing.T) {
	merchantKey := mustGenerateEasyPayV2RSAKey(t)
	platformKey := mustGenerateEasyPayV2RSAKey(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeEasyPayV2SignedJSON(t, w, platformKey, map[string]string{
			"code": "0", "timestamp": "1721050000", "status": "1", "trade_no": "trade", "money": "1.00",
		})
	}))
	defer server.Close()
	provider := newEasyPayV2TestProvider(t, server.URL, merchantKey, &platformKey.PublicKey)
	_, err := provider.QueryOrder(context.Background(), "order")
	if err == nil || !strings.Contains(err.Error(), "missing pid") {
		t.Fatalf("missing pid error = %v", err)
	}
}

func TestEasyPayV2QueryRejectsMismatchedOrderAndUnknownStatus(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(map[string]string)
		wantText string
	}{
		{name: "missing out trade number", mutate: func(values map[string]string) { delete(values, "out_trade_no") }, wantText: "missing out_trade_no"},
		{name: "mismatched out trade number", mutate: func(values map[string]string) { values["out_trade_no"] = "other-order" }, wantText: "out_trade_no mismatch"},
		{name: "missing trade number", mutate: func(values map[string]string) { delete(values, "trade_no") }, wantText: "missing trade_no"},
		{name: "unknown status", mutate: func(values map[string]string) { values["status"] = "99" }, wantText: "unsupported status: 99"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			merchantKey := mustGenerateEasyPayV2RSAKey(t)
			platformKey := mustGenerateEasyPayV2RSAKey(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				values := map[string]string{
					"code": "0", "pid": "pid-v2", "timestamp": "1721050000",
					"status": "1", "trade_no": "gateway-query", "out_trade_no": "out-order", "money": "12.34",
				}
				test.mutate(values)
				writeEasyPayV2SignedJSON(t, w, platformKey, values)
			}))
			defer server.Close()
			provider := newEasyPayV2TestProvider(t, server.URL, merchantKey, &platformKey.PublicKey)
			_, err := provider.QueryOrder(context.Background(), "out-order")
			if err == nil || !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("error = %v, want %q", err, test.wantText)
			}
		})
	}
}

func TestEasyPayV2RefundPrefersTradeNoAndReusesStableOutRefundNo(t *testing.T) {
	merchantKey := mustGenerateEasyPayV2RSAKey(t)
	platformKey := mustGenerateEasyPayV2RSAKey(t)
	var forms []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != easyPayV2RefundPath {
			t.Errorf("path = %q", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		copyForm := make(url.Values, len(r.PostForm))
		for key, values := range r.PostForm {
			copyForm[key] = append([]string(nil), values...)
		}
		forms = append(forms, copyForm)
		writeEasyPayV2SignedJSON(t, w, platformKey, map[string]string{
			"code": "0", "timestamp": "1721050000", "money": "1.50", "refund_no": "refund-platform",
			"out_refund_no": r.PostForm.Get("out_refund_no"), "trade_no": "gateway-trade",
		})
	}))
	defer server.Close()
	provider := newEasyPayV2TestProvider(t, server.URL, merchantKey, &platformKey.PublicKey)
	req := payment.RefundRequest{TradeNo: "gateway-trade", OrderID: "out-order", Amount: "1.5"}
	for index := 0; index < 2; index++ {
		response, err := provider.Refund(context.Background(), req)
		if err != nil {
			t.Fatalf("Refund %d: %v", index, err)
		}
		if response.RefundID != "refund-platform" || response.Status != payment.ProviderStatusSuccess || response.Metadata["protocol_version"] != "2" {
			t.Fatalf("response = %+v", response)
		}
	}
	if len(forms) != 2 || forms[0].Get("out_refund_no") == "" || forms[0].Get("out_refund_no") != forms[1].Get("out_refund_no") {
		t.Fatalf("refund forms = %v", forms)
	}
	if forms[0].Get("trade_no") != "gateway-trade" || forms[0].Get("out_trade_no") != "" || forms[0].Get("money") != "1.50" {
		t.Fatalf("refund form = %v", forms[0])
	}
}

func TestEasyPayV2RefundFallsBackToOutTradeNo(t *testing.T) {
	merchantKey := mustGenerateEasyPayV2RSAKey(t)
	platformKey := mustGenerateEasyPayV2RSAKey(t)
	var form url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		form = r.PostForm
		writeEasyPayV2SignedJSON(t, w, platformKey, map[string]string{
			"code": "0", "timestamp": "1721050000", "money": "1.00",
			"out_refund_no": r.PostForm.Get("out_refund_no"), "trade_no": "gateway-out-only",
		})
	}))
	defer server.Close()
	provider := newEasyPayV2TestProvider(t, server.URL, merchantKey, &platformKey.PublicKey)
	response, err := provider.Refund(context.Background(), payment.RefundRequest{OrderID: "out-only", Amount: "1.00"})
	if err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if form.Get("out_trade_no") != "out-only" || form.Get("trade_no") != "" || response.RefundID != form.Get("out_refund_no") {
		t.Fatalf("form=%v response=%+v", form, response)
	}
}

func TestEasyPayV2RefundRejectsMismatchedResponseFields(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(map[string]string)
		wantText string
	}{
		{name: "missing out refund number", mutate: func(values map[string]string) { delete(values, "out_refund_no") }, wantText: "missing out_refund_no"},
		{name: "mismatched out refund number", mutate: func(values map[string]string) { values["out_refund_no"] = "other-refund" }, wantText: "out_refund_no mismatch"},
		{name: "mismatched trade number", mutate: func(values map[string]string) { values["trade_no"] = "other-trade" }, wantText: "trade_no mismatch"},
		{name: "mismatched out trade number", mutate: func(values map[string]string) { values["out_trade_no"] = "other-order" }, wantText: "out_trade_no mismatch"},
		{name: "mismatched money", mutate: func(values map[string]string) { values["money"] = "2.00" }, wantText: "money mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			merchantKey := mustGenerateEasyPayV2RSAKey(t)
			platformKey := mustGenerateEasyPayV2RSAKey(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					t.Errorf("ParseForm: %v", err)
					return
				}
				values := map[string]string{
					"code": "0", "timestamp": "1721050000", "money": "1.50", "refund_no": "refund-platform",
					"out_refund_no": r.PostForm.Get("out_refund_no"), "trade_no": "gateway-trade", "out_trade_no": "out-order",
				}
				test.mutate(values)
				writeEasyPayV2SignedJSON(t, w, platformKey, values)
			}))
			defer server.Close()
			provider := newEasyPayV2TestProvider(t, server.URL, merchantKey, &platformKey.PublicKey)
			_, err := provider.Refund(context.Background(), payment.RefundRequest{
				TradeNo: "gateway-trade", OrderID: "out-order", Amount: "1.50",
			})
			if err == nil || !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("error = %v, want %q", err, test.wantText)
			}
		})
	}
}

func TestEasyPayV2QueryRefundContract(t *testing.T) {
	tests := []struct {
		name             string
		request          payment.RefundQueryRequest
		responseStatus   string
		responseRefundID string
		wantRequestKey   string
		wantRequestValue string
		wantStatus       string
	}{
		{
			name: "platform refund number",
			request: payment.RefundQueryRequest{
				RefundID: "refund-platform", TradeNo: "trade-123", OrderID: "order-123", Amount: "1.50",
			},
			responseStatus: "1", responseRefundID: "refund-platform",
			wantRequestKey: "refund_no", wantRequestValue: "refund-platform", wantStatus: payment.ProviderStatusSuccess,
		},
		{
			name: "derived merchant refund number",
			request: payment.RefundQueryRequest{
				TradeNo: "trade-456", OrderID: "order-456", Amount: "2.00",
			},
			responseStatus: "0",
			wantRequestKey: "out_refund_no", wantRequestValue: easyPayV2StableRefundNo("order-456", "trade-456", "2.00"),
			wantStatus: payment.ProviderStatusFailed,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			merchantKey := mustGenerateEasyPayV2RSAKey(t)
			platformKey := mustGenerateEasyPayV2RSAKey(t)
			var requestForm url.Values
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != easyPayV2RefundQueryPath {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if err := r.ParseForm(); err != nil {
					t.Errorf("ParseForm: %v", err)
					return
				}
				requestForm = r.PostForm
				if err := easyPayV2RSAVerify(formToEasyPayV2Map(r.PostForm), r.PostForm.Get("sign"), &merchantKey.PublicKey); err != nil {
					t.Errorf("request signature: %v", err)
				}
				response := map[string]string{
					"code": "0", "timestamp": "1721050000", "status": test.responseStatus,
					"trade_no": test.request.TradeNo, "out_trade_no": test.request.OrderID, "money": test.request.Amount,
				}
				if test.responseRefundID != "" {
					response["refund_no"] = test.responseRefundID
				} else {
					response["out_refund_no"] = test.wantRequestValue
				}
				writeEasyPayV2SignedJSONWithNumericStatus(t, w, platformKey, response)
			}))
			defer server.Close()
			provider := newEasyPayV2TestProvider(t, server.URL, merchantKey, &platformKey.PublicKey)

			response, err := provider.QueryRefund(context.Background(), test.request)
			if err != nil {
				t.Fatalf("QueryRefund: %v", err)
			}
			if got := requestForm.Get(test.wantRequestKey); got != test.wantRequestValue {
				t.Fatalf("form[%s] = %q, want %q (form=%v)", test.wantRequestKey, got, test.wantRequestValue, requestForm)
			}
			otherKey := "refund_no"
			if test.wantRequestKey == otherKey {
				otherKey = "out_refund_no"
			}
			if requestForm.Get(otherKey) != "" {
				t.Fatalf("form must send only one refund identifier: %v", requestForm)
			}
			for _, forbidden := range []string{"trade_no", "out_trade_no", "money"} {
				if requestForm.Get(forbidden) != "" {
					t.Fatalf("form[%s] must be empty: %v", forbidden, requestForm)
				}
			}
			if response.Status != test.wantStatus || response.RefundID != test.wantRequestValue {
				t.Fatalf("response = %+v", response)
			}
			if response.Metadata["pid"] != "pid-v2" || response.Metadata["protocol_version"] != "2" {
				t.Fatalf("metadata = %#v", response.Metadata)
			}
		})
	}
}

func TestEasyPayV2QueryRefundRejectsMismatchedResponseFields(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(map[string]string)
		wantText string
	}{
		{name: "mismatched refund number", mutate: func(values map[string]string) { values["refund_no"] = "other-refund" }, wantText: "refund_no mismatch"},
		{name: "mismatched trade number", mutate: func(values map[string]string) { values["trade_no"] = "other-trade" }, wantText: "trade_no mismatch"},
		{name: "mismatched out trade number", mutate: func(values map[string]string) { values["out_trade_no"] = "other-order" }, wantText: "out_trade_no mismatch"},
		{name: "missing money", mutate: func(values map[string]string) { delete(values, "money") }, wantText: "invalid money"},
		{name: "mismatched money", mutate: func(values map[string]string) { values["money"] = "2.00" }, wantText: "money mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			merchantKey := mustGenerateEasyPayV2RSAKey(t)
			platformKey := mustGenerateEasyPayV2RSAKey(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				values := map[string]string{
					"code": "0", "timestamp": "1721050000", "status": "1", "refund_no": "refund-platform",
					"trade_no": "trade-123", "out_trade_no": "order-123", "money": "1.50",
				}
				test.mutate(values)
				writeEasyPayV2SignedJSON(t, w, platformKey, values)
			}))
			defer server.Close()
			provider := newEasyPayV2TestProvider(t, server.URL, merchantKey, &platformKey.PublicKey)
			_, err := provider.QueryRefund(context.Background(), payment.RefundQueryRequest{
				RefundID: "refund-platform", TradeNo: "trade-123", OrderID: "order-123", Amount: "1.50",
			})
			if err == nil || !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("error = %v, want %q", err, test.wantText)
			}
		})
	}
}

func TestEasyPayV2QueryRefundRejectsMissingIdentifierAndUnknownStatus(t *testing.T) {
	merchantKey := mustGenerateEasyPayV2RSAKey(t)
	platformKey := mustGenerateEasyPayV2RSAKey(t)
	provider := newEasyPayV2TestProvider(t, "https://pay.example.com", merchantKey, &platformKey.PublicKey)
	if _, err := provider.QueryRefund(context.Background(), payment.RefundQueryRequest{}); err == nil || !strings.Contains(err.Error(), "missing refund_no or out_refund_no") {
		t.Fatalf("missing identifier error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeEasyPayV2SignedJSON(t, w, platformKey, map[string]string{
			"code": "0", "timestamp": "1721050000", "status": "2", "refund_no": "refund-platform",
		})
	}))
	defer server.Close()
	provider = newEasyPayV2TestProvider(t, server.URL, merchantKey, &platformKey.PublicKey)
	_, err := provider.QueryRefund(context.Background(), payment.RefundQueryRequest{RefundID: "refund-platform"})
	if err == nil || !strings.Contains(err.Error(), "unsupported status: 2") {
		t.Fatalf("unknown status error = %v", err)
	}
}

func TestEasyPayV2QueryRefundVerifiesSignatureTimestampAndOptionalPID(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(map[string]string)
		wrongSigner bool
		wantText    string
	}{
		{name: "invalid signature", wrongSigner: true, wantText: "invalid signature"},
		{name: "expired timestamp", mutate: func(values map[string]string) { values["timestamp"] = "1721049699" }, wantText: "outside 300 second"},
		{name: "wrong optional pid", mutate: func(values map[string]string) { values["pid"] = "other" }, wantText: "pid mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			merchantKey := mustGenerateEasyPayV2RSAKey(t)
			platformKey := mustGenerateEasyPayV2RSAKey(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				values := map[string]string{
					"code": "0", "timestamp": "1721050000", "status": "1", "refund_no": "refund-platform",
				}
				if test.mutate != nil {
					test.mutate(values)
				}
				signer := platformKey
				if test.wrongSigner {
					signer = mustGenerateEasyPayV2RSAKey(t)
				}
				writeEasyPayV2SignedJSON(t, w, signer, values)
			}))
			defer server.Close()
			provider := newEasyPayV2TestProvider(t, server.URL, merchantKey, &platformKey.PublicKey)
			_, err := provider.QueryRefund(context.Background(), payment.RefundQueryRequest{RefundID: "refund-platform"})
			if err == nil || !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("error = %v, want %q", err, test.wantText)
			}
		})
	}
}

func TestEasyPayV2VerifyNotification(t *testing.T) {
	merchantKey := mustGenerateEasyPayV2RSAKey(t)
	platformKey := mustGenerateEasyPayV2RSAKey(t)
	provider := newEasyPayV2TestProvider(t, "https://pay.example.com", merchantKey, &platformKey.PublicKey)
	baseParams := map[string]string{
		"pid": "pid-v2", "timestamp": "1721050000", "trade_status": tradeStatusSuccess,
		"trade_no": "gateway-notify", "out_trade_no": "out-notify", "money": "9.99", "type": "qqpay",
		"future_extension": "signed extension & value",
	}
	signature, err := easyPayV2RSASign(baseParams, platformKey)
	if err != nil {
		t.Fatalf("sign callback: %v", err)
	}

	var rawBodyWithoutSignType string
	for _, signType := range []struct {
		name  string
		value string
	}{
		{name: "missing sign type"},
		{name: "arbitrary sign type label", value: "UNTRUSTED-FUTURE-LABEL"},
	} {
		t.Run(signType.name, func(t *testing.T) {
			params := cloneStringMap(baseParams)
			params["sign"] = signature
			if signType.value != "" {
				params["sign_type"] = signType.value
			}
			rawBody := easyPayV2MapToForm(params).Encode()
			if signType.value == "" {
				rawBodyWithoutSignType = rawBody
			}

			for _, callbackMethod := range []string{"GET", "POST"} {
				t.Run(callbackMethod, func(t *testing.T) {
					notification, err := provider.VerifyNotification(context.Background(), rawBody, map[string]string{"x-callback-method": callbackMethod})
					if err != nil {
						t.Fatalf("VerifyNotification: %v", err)
					}
					if notification.OrderID != "out-notify" || notification.TradeNo != "gateway-notify" || notification.Amount != 9.99 || notification.Status != payment.NotificationStatusSuccess {
						t.Fatalf("notification = %+v", notification)
					}
					if notification.Metadata["pid"] != "pid-v2" || notification.Metadata["protocol_version"] != "2" {
						t.Fatalf("metadata = %#v", notification.Metadata)
					}
				})
			}
		})
	}

	if _, err := provider.VerifyNotification(context.Background(), rawBodyWithoutSignType+"&money=10.00", nil); err == nil || !strings.Contains(err.Error(), "duplicate parameter") {
		t.Fatalf("duplicate callback error = %v", err)
	}
	tampered := strings.Replace(rawBodyWithoutSignType, url.QueryEscape("signed extension & value"), url.QueryEscape("tampered"), 1)
	if _, err := provider.VerifyNotification(context.Background(), tampered, nil); err == nil || !strings.Contains(err.Error(), "invalid signature") {
		t.Fatalf("tampered extension error = %v", err)
	}
}

func TestEasyPayV2VerifyNotificationRejectsInvalidSecurityFields(t *testing.T) {
	merchantKey := mustGenerateEasyPayV2RSAKey(t)
	platformKey := mustGenerateEasyPayV2RSAKey(t)
	provider := newEasyPayV2TestProvider(t, "https://pay.example.com", merchantKey, &platformKey.PublicKey)
	tests := []struct {
		name        string
		mutate      func(map[string]string)
		skipSigning bool
		wantText    string
	}{
		{name: "missing pid", mutate: func(params map[string]string) { delete(params, "pid") }, wantText: "missing pid"},
		{name: "wrong pid", mutate: func(params map[string]string) { params["pid"] = "other" }, wantText: "pid mismatch"},
		{name: "missing sign", skipSigning: true, wantText: "missing sign"},
		{name: "expired", mutate: func(params map[string]string) { params["timestamp"] = "1721049699" }, wantText: "outside 300 second"},
		{name: "future", mutate: func(params map[string]string) { params["timestamp"] = "1721050301" }, wantText: "outside 300 second"},
		{name: "invalid base64", mutate: func(params map[string]string) { params["sign"] = "%%%" }, wantText: "invalid base64"},
		{name: "failed trade", mutate: func(params map[string]string) { params["trade_status"] = "WAIT_BUYER_PAY" }, wantText: "unsuccessful trade_status"},
		{name: "missing order", mutate: func(params map[string]string) { params["out_trade_no"] = "" }, wantText: "missing order identifier"},
		{name: "invalid amount", mutate: func(params map[string]string) { params["money"] = "0" }, wantText: "amount must be positive"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := map[string]string{
				"pid": "pid-v2", "timestamp": "1721050000", "sign_type": "RSA",
				"trade_status": tradeStatusSuccess, "trade_no": "trade", "out_trade_no": "order", "money": "1.00",
			}
			if test.mutate != nil {
				test.mutate(params)
			}
			if !test.skipSigning && params["sign"] == "" {
				signature, err := easyPayV2RSASign(params, platformKey)
				if err != nil {
					t.Fatalf("sign: %v", err)
				}
				params["sign"] = signature
			}
			_, err := provider.VerifyNotification(context.Background(), easyPayV2MapToForm(params).Encode(), nil)
			if err == nil || !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("error = %v, want %q", err, test.wantText)
			}
		})
	}

	boundary := map[string]string{
		"pid": "pid-v2", "timestamp": "1721050300", "sign_type": "RSA",
		"trade_status": tradeStatusSuccess, "trade_no": "trade", "out_trade_no": "order", "money": "1.00",
	}
	signature, err := easyPayV2RSASign(boundary, platformKey)
	if err != nil {
		t.Fatalf("sign boundary: %v", err)
	}
	boundary["sign"] = signature
	if _, err := provider.VerifyNotification(context.Background(), easyPayV2MapToForm(boundary).Encode(), nil); err != nil {
		t.Fatalf("300-second boundary should be accepted: %v", err)
	}
}

func TestEasyPayV2TransportLimitsAndContext(t *testing.T) {
	merchantKey := mustGenerateEasyPayV2RSAKey(t)
	platformKey := mustGenerateEasyPayV2RSAKey(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxEasypayResponseSize+1)))
	}))
	defer server.Close()
	provider := newEasyPayV2TestProvider(t, server.URL, merchantKey, &platformKey.PublicKey)
	_, err := provider.QueryOrder(context.Background(), "order")
	if err == nil || !strings.Contains(err.Error(), "exceeds 1MB") {
		t.Fatalf("oversized response error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = provider.QueryOrder(ctx, "order")
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("canceled context error = %v", err)
	}
}

func newEasyPayV2TestProvider(t *testing.T, apiBase string, merchantKey *rsa.PrivateKey, platformKey *rsa.PublicKey) *EasyPayV2 {
	t.Helper()
	provider, err := NewEasyPayV2("instance-v2", easyPayV2TestConfig(t, apiBase, merchantKey, platformKey))
	if err != nil {
		t.Fatalf("NewEasyPayV2: %v", err)
	}
	provider.now = func() time.Time { return easyPayV2FixedTime }
	return provider
}

func writeEasyPayV2SignedJSON(t *testing.T, w http.ResponseWriter, privateKey *rsa.PrivateKey, values map[string]string) {
	t.Helper()
	params := cloneStringMap(values)
	signature, err := easyPayV2RSASign(params, privateKey)
	if err != nil {
		t.Fatalf("sign response: %v", err)
	}
	params["sign"] = signature
	writeEasyPayV2JSON(t, w, params)
}

func writeEasyPayV2JSON(t *testing.T, w http.ResponseWriter, values map[string]string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(values); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func writeEasyPayV2SignedJSONWithNumericStatus(t *testing.T, w http.ResponseWriter, privateKey *rsa.PrivateKey, values map[string]string) {
	t.Helper()
	params := cloneStringMap(values)
	signature, err := easyPayV2RSASign(params, privateKey)
	if err != nil {
		t.Fatalf("sign response: %v", err)
	}
	params["sign"] = signature
	status, err := strconv.Atoi(params["status"])
	if err != nil {
		t.Fatalf("parse numeric status: %v", err)
	}
	payload := make(map[string]any, len(params))
	for key, value := range params {
		payload[key] = value
	}
	payload["code"] = 0
	payload["status"] = status
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func formToEasyPayV2Map(values url.Values) map[string]string {
	result := make(map[string]string, len(values))
	for key := range values {
		result[key] = values.Get(key)
	}
	return result
}

func easyPayV2MapToForm(values map[string]string) url.Values {
	result := make(url.Values, len(values))
	for key, value := range values {
		result.Set(key, value)
	}
	return result
}

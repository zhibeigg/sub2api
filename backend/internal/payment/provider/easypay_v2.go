package provider

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
)

const (
	easyPayV2ProtocolVersion       = "2"
	easyPayV2SignType              = "RSA"
	easyPayV2TimestampWindow       = 300 * time.Second
	easyPayV2MillisecondsThreshold = int64(1_000_000_000_000)
	easyPayV2SubmitPath            = "/api/pay/submit"
	easyPayV2QueryPath             = "/api/pay/query"
	easyPayV2RefundPath            = "/api/pay/refund"
	easyPayV2RefundQueryPath       = "/api/pay/refundquery"
	easyPayV2MethodJump            = "jump"
	easyPayV2MethodWeb             = "web"
	easyPayV2DevicePC              = "pc"
	easyPayV2DeviceMobile          = "mobile"
)

var _ payment.RefundQueryProvider = (*EasyPayV2)(nil)

type easyPayV2PIDPolicy bool

const (
	easyPayV2PIDOptional easyPayV2PIDPolicy = false
	easyPayV2PIDRequired easyPayV2PIDPolicy = true
)

// EasyPayV2 implements the EasyPay 2.0 RSA-SHA256 protocol.
type EasyPayV2 struct {
	instanceID           string
	config               map[string]string
	merchantPrivateKey   *rsa.PrivateKey
	platformPublicKey    *rsa.PublicKey
	httpClient           *http.Client
	now                  func() time.Time
	customPaymentMethods []easyPayCustomMethod
}

// NewEasyPayV2 creates an EasyPay 2.0 provider and validates its protocol configuration.
func NewEasyPayV2(instanceID string, config map[string]string) (*EasyPayV2, error) {
	if strings.TrimSpace(config["protocolVersion"]) != easyPayV2ProtocolVersion {
		return nil, newProviderConfigError(payment.TypeEasyPay, "protocolVersion", "must be 2 for the RSA provider", nil)
	}
	for _, key := range []string{"pid", "apiBase", "merchantPrivateKey", "platformPublicKey", "notifyUrl", "returnUrl"} {
		if strings.TrimSpace(config[key]) == "" {
			return nil, newProviderConfigError(payment.TypeEasyPay, key, "is required", nil)
		}
	}

	apiBase, err := normalizeEasyPayV2APIBase(config["apiBase"])
	if err != nil {
		return nil, newProviderConfigError(payment.TypeEasyPay, "apiBase", "must be an absolute HTTP(S) URL", err)
	}
	for _, key := range []string{"notifyUrl", "returnUrl"} {
		if err := validateEasyPayV2CallbackURL(config[key]); err != nil {
			return nil, newProviderConfigError(payment.TypeEasyPay, key, "must be an absolute HTTP(S) URL", err)
		}
	}

	merchantPrivateKey, err := parseEasyPayV2PrivateKey(config["merchantPrivateKey"])
	if err != nil {
		return nil, newProviderConfigError(payment.TypeEasyPay, "merchantPrivateKey", "is not a valid RSA private key", err)
	}
	platformPublicKey, err := parseEasyPayV2PublicKey(config["platformPublicKey"])
	if err != nil {
		return nil, newProviderConfigError(payment.TypeEasyPay, "platformPublicKey", "is not a valid RSA public key", err)
	}
	customMethods, err := parseEasyPayV2CustomMethods(config["customMethods"])
	if err != nil {
		return nil, newProviderConfigError(payment.TypeEasyPay, "customMethods", "must be a valid method list", err)
	}

	cfg := cloneStringMap(config)
	cfg["apiBase"] = apiBase
	cfg["protocolVersion"] = easyPayV2ProtocolVersion
	return &EasyPayV2{
		instanceID:           instanceID,
		config:               cfg,
		merchantPrivateKey:   merchantPrivateKey,
		platformPublicKey:    platformPublicKey,
		httpClient:           &http.Client{Timeout: easypayHTTPTimeout},
		now:                  time.Now,
		customPaymentMethods: customMethods,
	}, nil
}

func (e *EasyPayV2) Name() string        { return "EasyPay" }
func (e *EasyPayV2) ProviderKey() string { return payment.TypeEasyPay }

func (e *EasyPayV2) SupportedTypes() []payment.PaymentType {
	result := []payment.PaymentType{payment.TypeAlipay, payment.TypeWxpay, payment.TypeQQPay}
	seen := map[string]struct{}{
		payment.TypeAlipay: {},
		payment.TypeWxpay:  {},
		payment.TypeQQPay:  {},
	}
	for _, method := range e.customPaymentMethods {
		if _, exists := seen[method.Type]; exists {
			continue
		}
		seen[method.Type] = struct{}{}
		result = append(result, method.Type)
	}
	return result
}

func (e *EasyPayV2) MerchantIdentityMetadata() map[string]string {
	if e == nil {
		return nil
	}
	pid := strings.TrimSpace(e.config["pid"])
	if pid == "" {
		return nil
	}
	return map[string]string{
		"pid":              pid,
		"protocol_version": easyPayV2ProtocolVersion,
	}
}

func (e *EasyPayV2) CreatePayment(_ context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	paymentType, ok := e.resolvePaymentType(req.PaymentType)
	if !ok {
		return nil, fmt.Errorf("easypay v2 unsupported payment type: %s", strings.TrimSpace(req.PaymentType))
	}
	method, err := e.createMethod()
	if err != nil {
		return nil, err
	}
	notifyURL, returnURL := e.resolveURLs(req)
	if err := validateEasyPayV2CallbackURL(notifyURL); err != nil {
		return nil, fmt.Errorf("easypay v2 invalid notify URL: %w", err)
	}
	if err := validateEasyPayV2CallbackURL(returnURL); err != nil {
		return nil, fmt.Errorf("easypay v2 invalid return URL: %w", err)
	}

	device := easyPayV2DevicePC
	if method == easyPayV2MethodJump && req.IsMobile {
		device = easyPayV2DeviceMobile
	}
	payURL, err := e.buildHostedPaymentURL(map[string]string{
		"method":       method,
		"device":       device,
		"type":         paymentType,
		"out_trade_no": req.OrderID,
		"notify_url":   notifyURL,
		"return_url":   returnURL,
		"name":         req.Subject,
		"money":        req.Amount,
		"clientip":     req.ClientIP,
	})
	if err != nil {
		return nil, fmt.Errorf("easypay v2 build hosted payment URL: %w", err)
	}

	result := &payment.CreatePaymentResponse{
		PayURL:     payURL,
		ResultType: payment.CreatePaymentResultOrderCreated,
	}
	if method == easyPayV2MethodWeb {
		result.QRCode = payURL
	}
	return result, nil
}

func (e *EasyPayV2) buildHostedPaymentURL(params map[string]string) (string, error) {
	signedParams, err := e.buildRequestParams(params)
	if err != nil {
		return "", err
	}
	apiBase, err := url.Parse(e.config["apiBase"])
	if err != nil {
		return "", fmt.Errorf("parse api base: %w", err)
	}

	query := make(url.Values, len(signedParams))
	for key, value := range signedParams {
		query.Set(key, value)
	}
	hostedURL := &url.URL{
		Scheme:   apiBase.Scheme,
		Host:     apiBase.Host,
		Path:     easyPayV2SubmitPath,
		RawQuery: query.Encode(),
	}
	if err := validateEasyPayV2HostedPaymentURL(apiBase, hostedURL); err != nil {
		return "", err
	}
	return hostedURL.String(), nil
}

func (e *EasyPayV2) QueryOrder(ctx context.Context, outTradeNo string) (*payment.QueryOrderResponse, error) {
	outTradeNo = strings.TrimSpace(outTradeNo)
	if outTradeNo == "" {
		return nil, fmt.Errorf("easypay v2 query missing out_trade_no")
	}
	response, err := e.execute(ctx, easyPayV2QueryPath, map[string]string{"out_trade_no": outTradeNo}, easyPayV2PIDRequired)
	if err != nil {
		return nil, fmt.Errorf("easypay v2 query: %w", err)
	}

	if err := requireEasyPayV2ResponseFieldMatch(response, "out_trade_no", outTradeNo); err != nil {
		return nil, fmt.Errorf("easypay v2 query response: %w", err)
	}
	statusValue, err := strconv.Atoi(strings.TrimSpace(response["status"]))
	if err != nil {
		return nil, fmt.Errorf("easypay v2 query invalid status")
	}
	var status string
	switch statusValue {
	case 0, 3, 4:
		status = payment.ProviderStatusPending
	case easypayStatusPaid:
		status = payment.ProviderStatusPaid
	case 2:
		status = payment.ProviderStatusRefunded
	default:
		return nil, fmt.Errorf("easypay v2 query unsupported status: %d", statusValue)
	}

	amount, err := parseEasyPayV2Amount(response["money"], false)
	if err != nil {
		return nil, fmt.Errorf("easypay v2 query invalid money: %w", err)
	}
	tradeNo := strings.TrimSpace(response["trade_no"])
	if tradeNo == "" {
		return nil, fmt.Errorf("easypay v2 query response missing trade_no")
	}
	return &payment.QueryOrderResponse{
		TradeNo:  tradeNo,
		Status:   status,
		Amount:   amount.InexactFloat64(),
		Metadata: e.MerchantIdentityMetadata(),
	}, nil
}

func (e *EasyPayV2) VerifyNotification(_ context.Context, rawBody string, _ map[string]string) (*payment.PaymentNotification, error) {
	values, err := url.ParseQuery(rawBody)
	if err != nil {
		return nil, fmt.Errorf("easypay v2 parse notify: %w", err)
	}
	params := make(map[string]string, len(values))
	for key, items := range values {
		if len(items) != 1 {
			return nil, fmt.Errorf("easypay v2 notify duplicate parameter: %s", key)
		}
		params[key] = items[0]
	}
	if err := e.verifySignedParams(params, easyPayV2PIDRequired); err != nil {
		return nil, fmt.Errorf("easypay v2 notify verification failed: %w", err)
	}

	orderID := strings.TrimSpace(params["out_trade_no"])
	tradeNo := strings.TrimSpace(params["trade_no"])
	if orderID == "" || tradeNo == "" {
		return nil, fmt.Errorf("easypay v2 notify missing order identifier")
	}
	if params["trade_status"] != tradeStatusSuccess {
		return nil, fmt.Errorf("easypay v2 notify unsuccessful trade_status: %s", params["trade_status"])
	}
	amount, err := parseEasyPayV2Amount(params["money"], true)
	if err != nil {
		return nil, fmt.Errorf("easypay v2 notify invalid money: %w", err)
	}
	return &payment.PaymentNotification{
		TradeNo:  tradeNo,
		OrderID:  orderID,
		Amount:   amount.InexactFloat64(),
		Status:   payment.NotificationStatusSuccess,
		RawData:  rawBody,
		Metadata: e.MerchantIdentityMetadata(),
	}, nil
}

func (e *EasyPayV2) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	money, err := parseEasyPayV2Amount(req.Amount, true)
	if err != nil {
		return nil, fmt.Errorf("easypay v2 refund invalid money: %w", err)
	}
	normalizedMoney := money.StringFixed(2)
	tradeNo := strings.TrimSpace(req.TradeNo)
	orderID := strings.TrimSpace(req.OrderID)
	if tradeNo == "" && orderID == "" {
		return nil, fmt.Errorf("easypay v2 refund missing order identifier")
	}

	outRefundNo := easyPayV2StableRefundNo(orderID, tradeNo, normalizedMoney)
	params := map[string]string{
		"money":         normalizedMoney,
		"out_refund_no": outRefundNo,
	}
	if tradeNo != "" {
		params["trade_no"] = tradeNo
	} else {
		params["out_trade_no"] = orderID
	}
	response, err := e.execute(ctx, easyPayV2RefundPath, params, easyPayV2PIDOptional)
	if err != nil {
		return nil, fmt.Errorf("easypay v2 refund: %w", err)
	}

	responseMoney, err := parseEasyPayV2Amount(response["money"], true)
	if err != nil {
		return nil, fmt.Errorf("easypay v2 refund response invalid money: %w", err)
	}
	if !responseMoney.Equal(money) {
		return nil, fmt.Errorf("easypay v2 refund response money mismatch")
	}
	if err := requireEasyPayV2ResponseFieldMatch(response, "out_refund_no", outRefundNo); err != nil {
		return nil, fmt.Errorf("easypay v2 refund response: %w", err)
	}
	if tradeNo != "" {
		if err := requireEasyPayV2ResponseFieldMatch(response, "trade_no", tradeNo); err != nil {
			return nil, fmt.Errorf("easypay v2 refund response: %w", err)
		}
	}
	if orderID != "" {
		if returnedOrderID := strings.TrimSpace(response["out_trade_no"]); returnedOrderID != "" && returnedOrderID != orderID {
			return nil, fmt.Errorf("easypay v2 refund response out_trade_no mismatch")
		}
	}
	refundID := strings.TrimSpace(response["refund_no"])
	if refundID == "" {
		refundID = outRefundNo
	}
	return &payment.RefundResponse{
		RefundID: refundID,
		Status:   payment.ProviderStatusSuccess,
		Metadata: e.MerchantIdentityMetadata(),
	}, nil
}

func (e *EasyPayV2) QueryRefund(ctx context.Context, req payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	params, fallbackRefundID, err := easyPayV2RefundQueryParams(req)
	if err != nil {
		return nil, fmt.Errorf("easypay v2 query refund: %w", err)
	}
	response, err := e.execute(ctx, easyPayV2RefundQueryPath, params, easyPayV2PIDOptional)
	if err != nil {
		return nil, fmt.Errorf("easypay v2 query refund: %w", err)
	}
	for field, expected := range params {
		if err := requireEasyPayV2ResponseFieldMatch(response, field, expected); err != nil {
			return nil, fmt.Errorf("easypay v2 query refund response: %w", err)
		}
	}
	if tradeNo := strings.TrimSpace(req.TradeNo); tradeNo != "" {
		if err := requireEasyPayV2ResponseFieldMatch(response, "trade_no", tradeNo); err != nil {
			return nil, fmt.Errorf("easypay v2 query refund response: %w", err)
		}
	}
	if orderID := strings.TrimSpace(req.OrderID); orderID != "" {
		if err := requireEasyPayV2ResponseFieldMatch(response, "out_trade_no", orderID); err != nil {
			return nil, fmt.Errorf("easypay v2 query refund response: %w", err)
		}
	}
	if rawAmount := strings.TrimSpace(req.Amount); rawAmount != "" {
		expectedAmount, err := parseEasyPayV2Amount(rawAmount, true)
		if err != nil {
			return nil, fmt.Errorf("easypay v2 query refund invalid requested money: %w", err)
		}
		responseAmount, err := parseEasyPayV2Amount(response["money"], true)
		if err != nil {
			return nil, fmt.Errorf("easypay v2 query refund response invalid money: %w", err)
		}
		if !responseAmount.Equal(expectedAmount) {
			return nil, fmt.Errorf("easypay v2 query refund response money mismatch")
		}
	}

	statusValue, err := strconv.Atoi(strings.TrimSpace(response["status"]))
	if err != nil {
		return nil, fmt.Errorf("easypay v2 query refund invalid status")
	}
	var status string
	switch statusValue {
	case 0:
		status = payment.ProviderStatusFailed
	case 1:
		status = payment.ProviderStatusSuccess
	default:
		return nil, fmt.Errorf("easypay v2 query refund unsupported status: %d", statusValue)
	}

	refundID := strings.TrimSpace(response["refund_no"])
	if refundID == "" {
		refundID = strings.TrimSpace(response["out_refund_no"])
	}
	if refundID == "" {
		refundID = fallbackRefundID
	}
	return &payment.RefundResponse{
		RefundID: refundID,
		Status:   status,
		Metadata: e.MerchantIdentityMetadata(),
	}, nil
}

func easyPayV2RefundQueryParams(req payment.RefundQueryRequest) (map[string]string, string, error) {
	refundID := strings.TrimSpace(req.RefundID)
	orderID := strings.TrimSpace(req.OrderID)
	tradeNo := strings.TrimSpace(req.TradeNo)

	stableRefundID := ""
	if strings.TrimSpace(req.Amount) != "" && (orderID != "" || tradeNo != "") {
		amount, err := parseEasyPayV2Amount(req.Amount, true)
		if err != nil {
			if refundID == "" {
				return nil, "", fmt.Errorf("invalid money: %w", err)
			}
		} else {
			stableRefundID = easyPayV2StableRefundNo(orderID, tradeNo, amount.StringFixed(2))
		}
	}

	params := make(map[string]string, 1)
	switch {
	case refundID != "" && (refundID == stableRefundID || strings.HasPrefix(refundID, "sub2api_")):
		params["out_refund_no"] = refundID
	case refundID != "":
		params["refund_no"] = refundID
	case stableRefundID != "":
		params["out_refund_no"] = stableRefundID
		refundID = stableRefundID
	default:
		return nil, "", fmt.Errorf("missing refund_no or out_refund_no")
	}
	return params, refundID, nil
}

func (e *EasyPayV2) execute(ctx context.Context, path string, params map[string]string, pidPolicy easyPayV2PIDPolicy) (map[string]string, error) {
	signedParams, err := e.buildRequestParams(params)
	if err != nil {
		return nil, err
	}
	body, err := e.postForm(ctx, e.config["apiBase"]+path, signedParams)
	if err != nil {
		return nil, err
	}
	rawResponse, err := decodeEasyPayV2JSON(body)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}
	if !easyPayV2CodeIsSuccess(rawResponse["code"]) {
		message, _ := easyPayV2ScalarString(rawResponse["msg"])
		message = e.summarizeUpstreamMessage(message, signedParams["sign"])
		return nil, fmt.Errorf("upstream error: %s", message)
	}
	response := easyPayV2ScalarParams(rawResponse)
	if err := e.verifySignedParams(response, pidPolicy); err != nil {
		return nil, fmt.Errorf("response verification failed: %w", err)
	}
	return response, nil
}

func (e *EasyPayV2) buildRequestParams(params map[string]string) (map[string]string, error) {
	result := cloneStringMap(params)
	result["pid"] = e.config["pid"]
	result["timestamp"] = strconv.FormatInt(e.currentTime().Unix(), 10)
	signature, err := easyPayV2RSASign(result, e.merchantPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("sign request: %w", err)
	}
	result["sign"] = signature
	result["sign_type"] = easyPayV2SignType
	return result, nil
}

func (e *EasyPayV2) verifySignedParams(params map[string]string, pidPolicy easyPayV2PIDPolicy) error {
	// EasyPay V2 response authentication is fixed to RSA PKCS#1 v1.5 with
	// SHA-256. sign_type is request metadata excluded from the signed content,
	// so an absent or attacker-controlled response label must never select,
	// downgrade, or otherwise affect the cryptographic verification algorithm.
	responsePID := strings.TrimSpace(params["pid"])
	if pidPolicy == easyPayV2PIDRequired && responsePID == "" {
		return fmt.Errorf("missing pid")
	}
	if responsePID != "" && responsePID != e.config["pid"] {
		return fmt.Errorf("pid mismatch")
	}
	timestamp := params["timestamp"]
	if strings.TrimSpace(timestamp) == "" {
		return fmt.Errorf("missing timestamp")
	}
	parsedTimestamp, err := parseEasyPayV2Timestamp(timestamp)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}
	// Only the parsed instant is used for replay-window enforcement. The original
	// timestamp string remains untouched in params and is therefore included
	// verbatim in RSA-SHA256 verification. This supports both legacy SDK Unix
	// timestamps and the current RFC3339/Unix-millisecond protocol forms without
	// normalizing signed response data.
	delta := parsedTimestamp.Sub(e.currentTime())
	if delta < -easyPayV2TimestampWindow || delta > easyPayV2TimestampWindow {
		return fmt.Errorf("timestamp outside 300 second window")
	}
	signature := strings.TrimSpace(params["sign"])
	if signature == "" {
		return fmt.Errorf("missing sign")
	}
	if err := easyPayV2RSAVerify(params, signature, e.platformPublicKey); err != nil {
		return fmt.Errorf("invalid signature: %w", err)
	}
	return nil
}

func (e *EasyPayV2) postForm(ctx context.Context, endpoint string, params map[string]string) ([]byte, error) {
	form := url.Values{}
	for key, value := range params {
		form.Set(key, value)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	client := e.httpClient
	if client == nil {
		client = &http.Client{Timeout: easypayHTTPTimeout}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxEasypayResponseSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxEasypayResponseSize {
		return nil, fmt.Errorf("response exceeds 1MB limit")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return body, nil
}

func (e *EasyPayV2) resolvePaymentType(paymentType string) (string, bool) {
	paymentType = strings.TrimSpace(paymentType)
	switch paymentType {
	case payment.TypeAlipay, payment.TypeWxpay, payment.TypeQQPay:
		return paymentType, true
	}
	for _, method := range e.customPaymentMethods {
		if method.Type == paymentType {
			return method.UpstreamType, true
		}
	}
	return "", false
}

func (e *EasyPayV2) createMethod() (string, error) {
	switch strings.TrimSpace(e.config["paymentMode"]) {
	case "", "qrcode":
		return easyPayV2MethodWeb, nil
	case paymentModePopup:
		return easyPayV2MethodJump, nil
	default:
		return "", newProviderConfigError(payment.TypeEasyPay, "paymentMode", "must be qrcode or popup for protocol version 2", nil)
	}
}

func (e *EasyPayV2) resolveURLs(req payment.CreatePaymentRequest) (string, string) {
	notifyURL := strings.TrimSpace(req.NotifyURL)
	if notifyURL == "" {
		notifyURL = e.config["notifyUrl"]
	}
	returnURL := strings.TrimSpace(req.ReturnURL)
	if returnURL == "" {
		returnURL = e.config["returnUrl"]
	}
	return notifyURL, returnURL
}

func (e *EasyPayV2) currentTime() time.Time {
	if e != nil && e.now != nil {
		return e.now()
	}
	return time.Now()
}

func normalizeEasyPayV2APIBase(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Opaque != "" {
		return "", fmt.Errorf("unsupported URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("URL must not contain user info, query, or fragment")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", fmt.Errorf("URL must not contain a path")
	}
	if parsed.RawPath != "" && parsed.RawPath != "/" {
		return "", fmt.Errorf("URL must not contain an escaped path")
	}
	parsed.Path = ""
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func validateEasyPayV2CallbackURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	scheme := strings.ToLower(parsed.Scheme)
	if (scheme != "http" && scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" {
		return fmt.Errorf("unsupported URL")
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("URL must not contain a fragment")
	}
	return nil
}

func validateEasyPayV2HostedPaymentURL(apiBase, hostedURL *url.URL) error {
	if apiBase == nil || hostedURL == nil {
		return fmt.Errorf("payment URL is missing")
	}
	if hostedURL.User != nil || hostedURL.Opaque != "" {
		return fmt.Errorf("payment URL must not contain user info")
	}
	if !strings.EqualFold(hostedURL.Scheme, apiBase.Scheme) || !strings.EqualFold(hostedURL.Host, apiBase.Host) {
		return fmt.Errorf("payment URL origin mismatch")
	}
	if hostedURL.Path != easyPayV2SubmitPath || hostedURL.RawPath != "" {
		return fmt.Errorf("payment URL path mismatch")
	}
	if hostedURL.Fragment != "" {
		return fmt.Errorf("payment URL must not contain a fragment")
	}
	return nil
}

func parseEasyPayV2CustomMethods(raw string) ([]easyPayCustomMethod, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var methods []easyPayCustomMethod
	if err := json.Unmarshal([]byte(raw), &methods); err != nil {
		return nil, err
	}
	result := make([]easyPayCustomMethod, 0, len(methods))
	seen := make(map[string]struct{}, len(methods))
	for index, method := range methods {
		method.Type = strings.TrimSpace(method.Type)
		method.UpstreamType = strings.TrimSpace(method.UpstreamType)
		method.DisplayName = strings.TrimSpace(method.DisplayName)
		if method.Type == "" || method.UpstreamType == "" {
			return nil, fmt.Errorf("method %d requires type and upstreamType", index)
		}
		if _, exists := seen[method.Type]; exists {
			return nil, fmt.Errorf("duplicate method type: %s", method.Type)
		}
		seen[method.Type] = struct{}{}
		result = append(result, method)
	}
	return result, nil
}

func easyPayV2SignContent(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if key == "sign" || key == "sign_type" || easyPayV2ValueIsEmpty(value) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for index, key := range keys {
		if index > 0 {
			_ = builder.WriteByte('&')
		}
		_, _ = builder.WriteString(key)
		_ = builder.WriteByte('=')
		_, _ = builder.WriteString(params[key])
	}
	return builder.String()
}

func easyPayV2ValueIsEmpty(value string) bool {
	return strings.Trim(value, " \t\n\r\x00\v") == ""
}

func easyPayV2RSASign(params map[string]string, privateKey *rsa.PrivateKey) (string, error) {
	if privateKey == nil {
		return "", fmt.Errorf("RSA private key is nil")
	}
	digest := sha256.Sum256([]byte(easyPayV2SignContent(params)))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func easyPayV2RSAVerify(params map[string]string, signature string, publicKey *rsa.PublicKey) error {
	if publicKey == nil {
		return fmt.Errorf("RSA public key is nil")
	}
	decoded, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("invalid base64 signature: %w", err)
	}
	digest := sha256.Sum256([]byte(easyPayV2SignContent(params)))
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], decoded); err != nil {
		return err
	}
	return nil
}

func parseEasyPayV2PrivateKey(raw string) (*rsa.PrivateKey, error) {
	der, blockType, err := decodeEasyPayV2Key(raw, "PRIVATE KEY")
	if err != nil {
		return nil, err
	}
	if blockType == "RSA PRIVATE KEY" {
		return x509.ParsePKCS1PrivateKey(der)
	}
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not RSA")
		}
		return rsaKey, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	return nil, fmt.Errorf("unsupported private key encoding")
}

func parseEasyPayV2PublicKey(raw string) (*rsa.PublicKey, error) {
	der, blockType, err := decodeEasyPayV2Key(raw, "PUBLIC KEY")
	if err != nil {
		return nil, err
	}
	if blockType == "RSA PUBLIC KEY" {
		return x509.ParsePKCS1PublicKey(der)
	}
	if key, err := x509.ParsePKIXPublicKey(der); err == nil {
		rsaKey, ok := key.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("public key is not RSA")
		}
		return rsaKey, nil
	}
	if key, err := x509.ParsePKCS1PublicKey(der); err == nil {
		return key, nil
	}
	return nil, fmt.Errorf("unsupported public key encoding")
}

func decodeEasyPayV2Key(raw, defaultBlockType string) ([]byte, string, error) {
	trimmed := strings.TrimSpace(raw)
	if block, _ := pem.Decode([]byte(trimmed)); block != nil {
		return block.Bytes, block.Type, nil
	}
	compact := strings.Join(strings.Fields(trimmed), "")
	decoded, err := base64.StdEncoding.DecodeString(compact)
	if err != nil {
		return nil, "", fmt.Errorf("invalid base64 key: %w", err)
	}
	return decoded, defaultBlockType, nil
}

func decodeEasyPayV2JSON(body []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var result map[string]any
	if err := decoder.Decode(&result); err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("response must be a JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("response contains multiple JSON values")
		}
		return nil, err
	}
	return result, nil
}

func easyPayV2ScalarParams(raw map[string]any) map[string]string {
	result := make(map[string]string, len(raw))
	for key, value := range raw {
		if scalar, ok := easyPayV2ScalarString(value); ok {
			result[key] = scalar
		}
	}
	return result
}

func easyPayV2ScalarString(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case json.Number:
		return typed.String(), true
	case bool:
		if typed {
			return "1", true
		}
		return "", true
	case nil:
		return "", false
	default:
		return "", false
	}
}

func easyPayV2CodeIsSuccess(value any) bool {
	scalar, ok := easyPayV2ScalarString(value)
	if !ok {
		return false
	}
	code, err := strconv.Atoi(strings.TrimSpace(scalar))
	return err == nil && code == 0
}

func requireEasyPayV2ResponseFieldMatch(response map[string]string, field, expected string) error {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return nil
	}
	actual := strings.TrimSpace(response[field])
	if actual == "" {
		return fmt.Errorf("missing %s", field)
	}
	if actual != expected {
		return fmt.Errorf("%s mismatch", field)
	}
	return nil
}

func parseEasyPayV2Amount(raw string, requirePositive bool) (decimal.Decimal, error) {
	amount, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil {
		return decimal.Zero, err
	}
	if requirePositive && !amount.GreaterThan(decimal.Zero) {
		return decimal.Zero, fmt.Errorf("amount must be positive")
	}
	if !requirePositive && amount.IsNegative() {
		return decimal.Zero, fmt.Errorf("amount must not be negative")
	}
	if !amount.Equal(amount.Round(2)) {
		return decimal.Zero, fmt.Errorf("amount must have at most 2 decimal places")
	}
	return amount, nil
}

func easyPayV2StableRefundNo(orderID, tradeNo, money string) string {
	identifier := strings.TrimSpace(orderID)
	if identifier == "" {
		identifier = strings.TrimSpace(tradeNo)
	}
	digest := sha256.Sum256([]byte(identifier + "\n" + strings.TrimSpace(money)))
	return "sub2api_" + hex.EncodeToString(digest[:16])
}

func parseEasyPayV2Timestamp(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("timestamp is empty")
	}
	if isEasyPayV2DecimalDigits(trimmed) {
		value, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return time.Time{}, err
		}
		if value <= 0 {
			return time.Time{}, fmt.Errorf("timestamp must be positive")
		}
		if len(trimmed) == 13 || value >= easyPayV2MillisecondsThreshold {
			seconds := value / int64(time.Second/time.Millisecond)
			nanoseconds := (value % int64(time.Second/time.Millisecond)) * int64(time.Millisecond)
			return time.Unix(seconds, nanoseconds), nil
		}
		return time.Unix(value, 0), nil
	}

	parsed, err := time.Parse(time.RFC3339Nano, trimmed)
	if err != nil {
		return time.Time{}, err
	}
	if parsed.Unix() <= 0 {
		return time.Time{}, fmt.Errorf("timestamp must be positive")
	}
	return parsed, nil
}

func isEasyPayV2DecimalDigits(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return value != ""
}

func (e *EasyPayV2) summarizeUpstreamMessage(message string, requestSignature string) string {
	for _, sensitive := range []string{
		requestSignature,
		e.config["merchantPrivateKey"],
		e.config["pkey"],
		e.config["secretKey"],
	} {
		if sensitive != "" {
			message = strings.ReplaceAll(message, sensitive, "[REDACTED]")
		}
	}
	return summarizeEasyPayV2Message(message)
}

func summarizeEasyPayV2Message(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	if message == "" {
		return "request failed"
	}
	if len(message) > maxEasypayErrorSummary {
		return message[:maxEasypayErrorSummary] + "..."
	}
	return message
}

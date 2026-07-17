package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/h5"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/jsapi"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"github.com/wechatpay-apiv3/wechatpay-go/services/refunddomestic"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"
)

// WeChat Pay constants.
const (
	wxpayCurrency   = "CNY"
	wxpayH5Type     = "Wap"
	wxpayResultPath = "/payment/result"
)

const (
	wxpayMetadataAppID      = "appid"
	wxpayMetadataMerchantID = "mchid"
	wxpayMetadataCurrency   = "currency"
	wxpayMetadataTradeState = "trade_state"
)

// WeChat Pay create-payment modes.
const (
	wxpayModeNative = "native"
	wxpayModeH5     = "h5"
	wxpayModeJSAPI  = "jsapi"
)

const (
	wxpayConfigNativeEnabled = "nativeEnabled"
	wxpayConfigH5Enabled     = "h5Enabled"
	wxpayConfigJSAPIEnabled  = "jsapiEnabled"
)

// WxpayCapabilityStatus is the non-sensitive local capability self-check result.
type WxpayCapabilityStatus struct {
	NativeEnabled bool
	H5Enabled     bool
	JSAPIEnabled  bool
}

// WeChat Pay trade states.
const (
	wxpayTradeStateSuccess  = "SUCCESS"
	wxpayTradeStateRefund   = "REFUND"
	wxpayTradeStateClosed   = "CLOSED"
	wxpayTradeStatePayError = "PAYERROR"
)

// WeChat Pay notification event types.
const (
	wxpayEventTransactionSuccess = "TRANSACTION.SUCCESS"
)

var (
	wxpayNativePrepay = func(ctx context.Context, svc native.NativeApiService, req native.PrepayRequest) (*native.PrepayResponse, *core.APIResult, error) {
		return svc.Prepay(ctx, req)
	}
	wxpayH5Prepay = func(ctx context.Context, svc h5.H5ApiService, req h5.PrepayRequest) (*h5.PrepayResponse, *core.APIResult, error) {
		return svc.Prepay(ctx, req)
	}
	wxpayJSAPIPrepayWithRequestPayment = func(ctx context.Context, svc jsapi.JsapiApiService, req jsapi.PrepayRequest) (*jsapi.PrepayWithRequestPaymentResponse, *core.APIResult, error) {
		return svc.PrepayWithRequestPayment(ctx, req)
	}
)

type Wxpay struct {
	instanceID    string
	config        map[string]string
	mu            sync.Mutex
	coreClient    *core.Client
	notifyHandler *notify.Handler
}

const (
	wxpayAPIv3KeyLength   = 32
	wxpayAppIDShortLength = 18
	wxpayAppIDLongLength  = 20
)

// IsValidWxpayAppID validates the documented WeChat AppID shape: a lowercase
// "wx" prefix followed by 16 or 18 ASCII alphanumeric characters.
func IsValidWxpayAppID(raw string) bool {
	appID := strings.TrimSpace(raw)
	if len(appID) != wxpayAppIDShortLength && len(appID) != wxpayAppIDLongLength {
		return false
	}
	if !strings.HasPrefix(appID, "wx") {
		return false
	}
	for i := len("wx"); i < len(appID); i++ {
		ch := appID[i]
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') {
			return false
		}
	}
	return true
}

// ValidateWxpayAppIDConfig is the strict save-time validation used by the admin
// configuration service. It is intentionally separate from NewWxpay so legacy
// enabled instances remain loadable after an upgrade and fail locally only when
// a payment mode would actually use an invalid AppID.
func ValidateWxpayAppIDConfig(config map[string]string) error {
	if appID := strings.TrimSpace(config["appId"]); appID != "" && !IsValidWxpayAppID(appID) {
		return invalidWxpayBaseAppIDError()
	}
	if appID := strings.TrimSpace(config["mpAppId"]); appID != "" && !IsValidWxpayAppID(appID) {
		return invalidWxpayJSAPIAppIDError()
	}
	return nil
}

// InspectWxpayCapabilities resolves explicit capability switches and applies
// backward-compatible defaults for historical provider instances.
func InspectWxpayCapabilities(config map[string]string) (WxpayCapabilityStatus, error) {
	h5Configured := strings.TrimSpace(config["h5AppName"]) != "" && strings.TrimSpace(config["h5AppUrl"]) != ""
	jsapiConfigured := strings.TrimSpace(config["mpAppId"]) != ""

	nativeEnabled, err := resolveWxpayCapabilityFlag(config, wxpayConfigNativeEnabled, true)
	if err != nil {
		return WxpayCapabilityStatus{}, err
	}
	h5Enabled, err := resolveWxpayCapabilityFlag(config, wxpayConfigH5Enabled, h5Configured)
	if err != nil {
		return WxpayCapabilityStatus{}, err
	}
	jsapiEnabled, err := resolveWxpayCapabilityFlag(config, wxpayConfigJSAPIEnabled, jsapiConfigured)
	if err != nil {
		return WxpayCapabilityStatus{}, err
	}

	status := WxpayCapabilityStatus{
		NativeEnabled: nativeEnabled,
		H5Enabled:     h5Enabled,
		JSAPIEnabled:  jsapiEnabled,
	}
	if err := validateWxpayCapabilityConfig(config, status); err != nil {
		return WxpayCapabilityStatus{}, err
	}
	return status, nil
}

func resolveWxpayCapabilityFlag(config map[string]string, key string, fallback bool) (bool, error) {
	raw, exists := config[key]
	if !exists {
		return fallback, nil
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, infraerrors.BadRequest("WXPAY_CONFIG_INVALID_BOOLEAN", "invalid_boolean").
			WithMetadata(map[string]string{"action": "set_" + key + "_to_true_or_false"})
	}
}

func validateWxpayCapabilityConfig(config map[string]string, status WxpayCapabilityStatus) error {
	if status.H5Enabled {
		if strings.TrimSpace(config["h5AppName"]) == "" {
			return infraerrors.BadRequest("WXPAY_CONFIG_H5_APP_REQUIRED", "h5_app_name_required").
				WithMetadata(map[string]string{"action": "configure_h5_app_name"})
		}
		h5AppURL := strings.TrimSpace(config["h5AppUrl"])
		parsed, err := url.Parse(h5AppURL)
		if err != nil || !parsed.IsAbs() || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return infraerrors.BadRequest("WXPAY_CONFIG_H5_URL_INVALID", "h5_app_url_must_be_absolute_https").
				WithMetadata(map[string]string{"action": "configure_absolute_https_h5_app_url"})
		}
	}
	if status.JSAPIEnabled && ResolveWxpayJSAPIAppID(config) == "" {
		return infraerrors.BadRequest("WXPAY_CONFIG_JSAPI_APPID_REQUIRED", "jsapi_app_id_required").
			WithMetadata(map[string]string{"action": "configure_jsapi_app_id"})
	}
	return nil
}

func NewWxpay(instanceID string, config map[string]string) (*Wxpay, error) {
	// All fields are required. Platform-certificate mode is intentionally unsupported —
	// WeChat has been migrating all merchants to the pubkey verifier since 2024-10,
	// and newly-provisioned merchants cannot download platform certificates at all.
	required := []string{"appId", "mchId", "privateKey", "apiV3Key", "certSerial", "publicKey", "publicKeyId"}
	for _, k := range required {
		if config[k] == "" {
			return nil, infraerrors.BadRequest("WXPAY_CONFIG_MISSING_KEY", "missing_required_key").
				WithMetadata(map[string]string{"action": "configure_required_wxpay_credentials"})
		}
	}
	if len(config["apiV3Key"]) != wxpayAPIv3KeyLength {
		return nil, infraerrors.BadRequest("WXPAY_CONFIG_INVALID_KEY_LENGTH", "invalid_key_length").
			WithMetadata(map[string]string{"action": "set_api_v3_key_to_32_bytes"})
	}
	// Parse PEMs eagerly so malformed keys surface at save time, not at order creation.
	if _, err := utils.LoadPrivateKey(formatPEM(config["privateKey"], "PRIVATE KEY")); err != nil {
		return nil, infraerrors.BadRequest("WXPAY_CONFIG_INVALID_KEY", "invalid_key").
			WithMetadata(map[string]string{"action": "verify_merchant_private_key"})
	}
	if _, err := utils.LoadPublicKey(formatPEM(config["publicKey"], "PUBLIC KEY")); err != nil {
		return nil, infraerrors.BadRequest("WXPAY_CONFIG_INVALID_KEY", "invalid_key").
			WithMetadata(map[string]string{"action": "verify_wechat_pay_public_key"})
	}
	if _, err := InspectWxpayCapabilities(config); err != nil {
		return nil, err
	}
	return &Wxpay{instanceID: instanceID, config: config}, nil
}

func (w *Wxpay) Name() string        { return "Wxpay" }
func (w *Wxpay) ProviderKey() string { return payment.TypeWxpay }
func (w *Wxpay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeWxpay}
}

// ResolveWxpayJSAPIAppID returns the AppID that JSAPI prepay will use for a
// given provider config. A dedicated MP AppID takes precedence over the base
// merchant AppID.
func ResolveWxpayJSAPIAppID(config map[string]string) string {
	if appID := strings.TrimSpace(config["mpAppId"]); appID != "" {
		return appID
	}
	return strings.TrimSpace(config["appId"])
}

func validateWxpayAppIDForMode(config map[string]string, mode string) error {
	if mode == wxpayModeJSAPI {
		if !IsValidWxpayAppID(ResolveWxpayJSAPIAppID(config)) {
			return invalidWxpayJSAPIAppIDError()
		}
		return nil
	}
	if !IsValidWxpayAppID(config["appId"]) {
		return invalidWxpayBaseAppIDError()
	}
	return nil
}

func invalidWxpayBaseAppIDError() error {
	return infraerrors.BadRequest("WXPAY_CONFIG_APPID_INVALID", "wechat_app_id_invalid").
		WithMetadata(map[string]string{"action": "configure_valid_wechat_app_id"})
}

func invalidWxpayJSAPIAppIDError() error {
	return infraerrors.BadRequest("WXPAY_CONFIG_JSAPI_APPID_INVALID", "wechat_jsapi_app_id_invalid").
		WithMetadata(map[string]string{"action": "configure_valid_wechat_mp_app_id"})
}

func formatPEM(key, keyType string) string {
	key = strings.TrimSpace(key)
	if strings.HasPrefix(key, "-----BEGIN") {
		return key
	}
	return fmt.Sprintf("-----BEGIN %s-----\n%s\n-----END %s-----", keyType, key, keyType)
}

func (w *Wxpay) ensureClient() (*core.Client, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.coreClient != nil {
		return w.coreClient, nil
	}
	privateKey, err := utils.LoadPrivateKey(formatPEM(w.config["privateKey"], "PRIVATE KEY"))
	if err != nil {
		return nil, infraerrors.BadRequest("WXPAY_CONFIG_INVALID_KEY", "invalid_key").
			WithMetadata(map[string]string{"action": "verify_merchant_private_key"})
	}
	publicKey, err := utils.LoadPublicKey(formatPEM(w.config["publicKey"], "PUBLIC KEY"))
	if err != nil {
		return nil, infraerrors.BadRequest("WXPAY_CONFIG_INVALID_KEY", "invalid_key").
			WithMetadata(map[string]string{"action": "verify_wechat_pay_public_key"})
	}
	verifier := verifiers.NewSHA256WithRSAPubkeyVerifier(w.config["publicKeyId"], *publicKey)
	client, err := core.NewClient(context.Background(),
		option.WithMerchantCredential(w.config["mchId"], w.config["certSerial"], privateKey),
		option.WithVerifier(verifier))
	if err != nil {
		return nil, fmt.Errorf("wxpay init client: %w", err)
	}
	handler, err := notify.NewRSANotifyHandler(w.config["apiV3Key"], verifier)
	if err != nil {
		return nil, fmt.Errorf("wxpay init notify handler: %w", err)
	}
	w.notifyHandler = handler
	w.coreClient = client
	return w.coreClient, nil
}

func (w *Wxpay) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	capabilities, err := InspectWxpayCapabilities(w.config)
	if err != nil {
		return nil, err
	}
	mode, err := resolveWxpayCreateMode(req, capabilities)
	if err != nil {
		return nil, err
	}
	if err := validateWxpayAppIDForMode(w.config, mode); err != nil {
		return nil, err
	}
	client, err := w.ensureClient()
	if err != nil {
		return nil, err
	}
	// Request-first, config-fallback (consistent with EasyPay/Alipay)
	notifyURL := req.NotifyURL
	if notifyURL == "" {
		notifyURL = w.config["notifyUrl"]
	}
	if notifyURL == "" {
		return nil, fmt.Errorf("wxpay notifyUrl is required")
	}
	totalFen, err := payment.YuanToFen(req.Amount)
	if err != nil {
		return nil, fmt.Errorf("wxpay create payment: %w", err)
	}

	switch mode {
	case wxpayModeJSAPI:
		return w.prepayJSAPI(ctx, client, req, notifyURL, totalFen)
	case wxpayModeH5:
		return w.prepayH5(ctx, client, req, notifyURL, totalFen)
	case wxpayModeNative:
		return w.prepayNative(ctx, client, req, notifyURL, totalFen)
	default:
		return nil, fmt.Errorf("wxpay create payment: unsupported mode %q", mode)
	}
}

func (w *Wxpay) prepayJSAPI(ctx context.Context, c *core.Client, req payment.CreatePaymentRequest, notifyURL string, totalFen int64) (*payment.CreatePaymentResponse, error) {
	svc := jsapi.JsapiApiService{Client: c}
	cur := wxpayCurrency
	appID := ResolveWxpayJSAPIAppID(w.config)
	prepayReq := jsapi.PrepayRequest{
		Appid:       core.String(appID),
		Mchid:       core.String(w.config["mchId"]),
		Description: core.String(req.Subject),
		OutTradeNo:  core.String(req.OrderID),
		NotifyUrl:   core.String(notifyURL),
		Amount:      &jsapi.Amount{Total: core.Int64(totalFen), Currency: &cur},
		Payer:       &jsapi.Payer{Openid: core.String(strings.TrimSpace(req.OpenID))},
	}
	if clientIP := strings.TrimSpace(req.ClientIP); clientIP != "" {
		prepayReq.SceneInfo = &jsapi.SceneInfo{PayerClientIp: core.String(clientIP)}
	}
	resp, result, err := wxpayJSAPIPrepayWithRequestPayment(ctx, svc, prepayReq)
	if err != nil {
		return nil, mapWxpayPrepayError(wxpayModeJSAPI, result, err)
	}
	return &payment.CreatePaymentResponse{
		TradeNo:    req.OrderID,
		ResultType: payment.CreatePaymentResultJSAPIReady,
		JSAPI: &payment.WechatJSAPIPayload{
			AppID:     wxSV(resp.Appid),
			TimeStamp: wxSV(resp.TimeStamp),
			NonceStr:  wxSV(resp.NonceStr),
			Package:   wxSV(resp.Package),
			SignType:  wxSV(resp.SignType),
			PaySign:   wxSV(resp.PaySign),
		},
	}, nil
}

func (w *Wxpay) prepayNative(ctx context.Context, c *core.Client, req payment.CreatePaymentRequest, notifyURL string, totalFen int64) (*payment.CreatePaymentResponse, error) {
	svc := native.NativeApiService{Client: c}
	cur := wxpayCurrency
	resp, result, err := wxpayNativePrepay(ctx, svc, native.PrepayRequest{
		Appid: core.String(strings.TrimSpace(w.config["appId"])), Mchid: core.String(w.config["mchId"]),
		Description: core.String(req.Subject), OutTradeNo: core.String(req.OrderID),
		NotifyUrl: core.String(notifyURL),
		Amount:    &native.Amount{Total: core.Int64(totalFen), Currency: &cur},
	})
	if err != nil {
		return nil, mapWxpayPrepayError(wxpayModeNative, result, err)
	}
	codeURL := ""
	if resp.CodeUrl != nil {
		codeURL = *resp.CodeUrl
	}
	return &payment.CreatePaymentResponse{TradeNo: req.OrderID, QRCode: codeURL}, nil
}

func (w *Wxpay) prepayH5(ctx context.Context, c *core.Client, req payment.CreatePaymentRequest, notifyURL string, totalFen int64) (*payment.CreatePaymentResponse, error) {
	svc := h5.H5ApiService{Client: c}
	cur := wxpayCurrency
	resp, result, err := wxpayH5Prepay(ctx, svc, h5.PrepayRequest{
		Appid: core.String(strings.TrimSpace(w.config["appId"])), Mchid: core.String(w.config["mchId"]),
		Description: core.String(req.Subject), OutTradeNo: core.String(req.OrderID),
		NotifyUrl: core.String(notifyURL),
		Amount:    &h5.Amount{Total: core.Int64(totalFen), Currency: &cur},
		SceneInfo: &h5.SceneInfo{PayerClientIp: core.String(req.ClientIP), H5Info: buildWxpayH5Info(w.config)},
	})
	if err != nil {
		return nil, mapWxpayPrepayError(wxpayModeH5, result, err)
	}
	h5URL := ""
	if resp.H5Url != nil {
		h5URL = *resp.H5Url
	}
	h5URL, err = appendWxpayRedirectURL(h5URL, req)
	if err != nil {
		return nil, err
	}
	return &payment.CreatePaymentResponse{TradeNo: req.OrderID, PayURL: h5URL}, nil
}

func buildWxpayH5Info(config map[string]string) *h5.H5Info {
	tp := wxpayH5Type
	info := &h5.H5Info{Type: &tp}
	if appName := strings.TrimSpace(config["h5AppName"]); appName != "" {
		info.AppName = core.String(appName)
	}
	if appURL := strings.TrimSpace(config["h5AppUrl"]); appURL != "" {
		info.AppUrl = core.String(appURL)
	}
	return info
}

func resolveWxpayCreateMode(req payment.CreatePaymentRequest, capabilities WxpayCapabilityStatus) (string, error) {
	if strings.TrimSpace(req.OpenID) != "" {
		if capabilities.JSAPIEnabled {
			return wxpayModeJSAPI, nil
		}
		return "", noAvailableWxpayCapabilityError(wxpayModeJSAPI, "enable_jsapi_payment")
	}
	if req.IsWeChatBrowser {
		if capabilities.NativeEnabled {
			return wxpayModeNative, nil
		}
		return "", noAvailableWxpayCapabilityError(wxpayModeNative, "enable_native_payment")
	}
	if req.IsMobile {
		if capabilities.H5Enabled && strings.TrimSpace(req.ClientIP) != "" {
			return wxpayModeH5, nil
		}
		if capabilities.NativeEnabled {
			return wxpayModeNative, nil
		}
		action := "enable_h5_or_native_payment"
		if capabilities.H5Enabled {
			action = "provide_client_ip_or_enable_native_payment"
		}
		return "", noAvailableWxpayCapabilityError(wxpayModeH5, action)
	}
	if capabilities.NativeEnabled {
		return wxpayModeNative, nil
	}
	return "", noAvailableWxpayCapabilityError(wxpayModeNative, "enable_native_payment")
}

func noAvailableWxpayCapabilityError(mode, action string) error {
	return infraerrors.ServiceUnavailable(
		"NO_AVAILABLE_WXPAY_CAPABILITY",
		"no available WeChat Pay capability for the current payment scenario",
	).WithMetadata(map[string]string{
		"mode":   mode,
		"action": action,
	})
}

func mapWxpayPrepayError(mode string, result *core.APIResult, err error) error {
	if err == nil {
		return nil
	}

	httpStatus := 0
	wechatCode := ""
	requestID := ""
	var apiErr *core.APIError
	if errors.As(err, &apiErr) {
		httpStatus = apiErr.StatusCode
		wechatCode = strings.TrimSpace(apiErr.Code)
		requestID = strings.TrimSpace(apiErr.Header.Get("Request-Id"))
	}
	if result != nil && result.Response != nil {
		if httpStatus == 0 {
			httpStatus = result.Response.StatusCode
		}
		if requestID == "" {
			requestID = strings.TrimSpace(result.Response.Header.Get("Request-Id"))
		}
	}

	reason, message, action := classifyWxpayAPIError(mode, wechatCode)
	metadata := map[string]string{
		"mode":   mode,
		"action": action,
	}
	if httpStatus > 0 {
		metadata["http_status"] = strconv.Itoa(httpStatus)
	}
	if wechatCode != "" {
		metadata["wechat_code"] = wechatCode
	}
	if requestID != "" {
		metadata["request_id"] = requestID
	}
	return infraerrors.ServiceUnavailable(reason, message).WithMetadata(metadata)
}

func classifyWxpayAPIError(mode, wechatCode string) (reason, message, action string) {
	switch strings.ToUpper(strings.TrimSpace(wechatCode)) {
	case "NO_AUTH":
		switch mode {
		case wxpayModeNative:
			return "WECHAT_NATIVE_NOT_AUTHORIZED", "wechat native payment is not authorized for this merchant", "enable_native_payment"
		case wxpayModeH5:
			return "WECHAT_H5_NOT_AUTHORIZED", "wechat h5 payment is not authorized for this merchant", "enable_h5_payment_or_use_native"
		case wxpayModeJSAPI:
			return "WECHAT_JSAPI_NOT_AUTHORIZED", "wechat jsapi payment is not authorized for this merchant", "enable_jsapi_and_configure_mp"
		}
	case "APPID_MCHID_NOT_MATCH":
		return "WECHAT_APPID_MCHID_MISMATCH", "wechat app id and merchant id do not match", "verify_appid_mchid_binding"
	case "SIGN_ERROR":
		return "WECHAT_SIGN_ERROR", "wechat pay rejected the merchant signature", "verify_merchant_signature_config"
	}
	return "WECHAT_PAYMENT_API_ERROR", "wechat payment api request failed", "check_wechat_pay_configuration"
}

func appendWxpayRedirectURL(h5URL string, req payment.CreatePaymentRequest) (string, error) {
	h5URL = strings.TrimSpace(h5URL)
	returnURL := strings.TrimSpace(req.ReturnURL)
	if h5URL == "" || returnURL == "" {
		return h5URL, nil
	}

	redirectURL, err := buildWxpayResultURL(returnURL, req)
	if err != nil {
		return "", err
	}

	sep := "&"
	if !strings.Contains(h5URL, "?") {
		sep = "?"
	}
	return h5URL + sep + "redirect_url=" + url.QueryEscape(redirectURL), nil
}

func buildWxpayResultURL(returnURL string, req payment.CreatePaymentRequest) (string, error) {
	u, err := url.Parse(returnURL)
	if err != nil || !u.IsAbs() || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("return URL must be an absolute http(s) URL")
	}

	values := u.Query()
	values.Set("out_trade_no", strings.TrimSpace(req.OrderID))
	if paymentType := strings.TrimSpace(req.PaymentType); paymentType != "" {
		values.Set("payment_type", paymentType)
	}
	if strings.TrimSpace(u.Path) == "" {
		u.Path = wxpayResultPath
	}
	u.RawPath = ""
	u.RawQuery = values.Encode()
	u.Fragment = ""
	return u.String(), nil
}

func wxSV(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func mapWxState(s string) string {
	switch s {
	case wxpayTradeStateSuccess:
		return payment.ProviderStatusPaid
	case wxpayTradeStateRefund:
		return payment.ProviderStatusRefunded
	case wxpayTradeStateClosed, wxpayTradeStatePayError:
		return payment.ProviderStatusFailed
	default:
		return payment.ProviderStatusPending
	}
}

func buildWxpayTransactionMetadata(tx *payments.Transaction) map[string]string {
	if tx == nil {
		return nil
	}

	metadata := map[string]string{}
	if appID := wxSV(tx.Appid); appID != "" {
		metadata[wxpayMetadataAppID] = appID
	}
	if merchantID := wxSV(tx.Mchid); merchantID != "" {
		metadata[wxpayMetadataMerchantID] = merchantID
	}
	if tradeState := wxSV(tx.TradeState); tradeState != "" {
		metadata[wxpayMetadataTradeState] = tradeState
	}
	if tx.Amount != nil {
		if currency := wxSV(tx.Amount.Currency); currency != "" {
			metadata[wxpayMetadataCurrency] = currency
		}
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func (w *Wxpay) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	c, err := w.ensureClient()
	if err != nil {
		return nil, err
	}
	svc := native.NativeApiService{Client: c}
	tx, _, err := svc.QueryOrderByOutTradeNo(ctx, native.QueryOrderByOutTradeNoRequest{
		OutTradeNo: core.String(tradeNo), Mchid: core.String(w.config["mchId"]),
	})
	if err != nil {
		return nil, fmt.Errorf("wxpay query order: %w", err)
	}
	var amt float64
	if tx.Amount != nil && tx.Amount.Total != nil {
		amt = payment.FenToYuan(*tx.Amount.Total)
	}
	id := tradeNo
	if tx.TransactionId != nil {
		id = *tx.TransactionId
	}
	pa := ""
	if tx.SuccessTime != nil {
		pa = *tx.SuccessTime
	}
	return &payment.QueryOrderResponse{
		TradeNo:  id,
		Status:   mapWxState(wxSV(tx.TradeState)),
		Amount:   amt,
		PaidAt:   pa,
		Metadata: buildWxpayTransactionMetadata(tx),
	}, nil
}

func (w *Wxpay) VerifyNotification(ctx context.Context, rawBody string, headers map[string]string) (*payment.PaymentNotification, error) {
	if _, err := w.ensureClient(); err != nil {
		return nil, err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "/", io.NopCloser(bytes.NewBufferString(rawBody)))
	if err != nil {
		return nil, fmt.Errorf("wxpay construct request: %w", err)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	var tx payments.Transaction
	nr, err := w.notifyHandler.ParseNotifyRequest(ctx, r, &tx)
	if err != nil {
		return nil, fmt.Errorf("wxpay verify notification: %w", err)
	}
	if nr.EventType != wxpayEventTransactionSuccess {
		return nil, nil
	}
	var amt float64
	if tx.Amount != nil && tx.Amount.Total != nil {
		amt = payment.FenToYuan(*tx.Amount.Total)
	}
	st := payment.ProviderStatusFailed
	if wxSV(tx.TradeState) == wxpayTradeStateSuccess {
		st = payment.ProviderStatusSuccess
	}
	return &payment.PaymentNotification{
		TradeNo: wxSV(tx.TransactionId), OrderID: wxSV(tx.OutTradeNo),
		Amount: amt, Status: st, RawData: rawBody, Metadata: buildWxpayTransactionMetadata(&tx),
	}, nil
}

func (w *Wxpay) Refund(ctx context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	c, err := w.ensureClient()
	if err != nil {
		return nil, err
	}
	rf, err := payment.YuanToFen(req.Amount)
	if err != nil {
		return nil, fmt.Errorf("wxpay refund amount: %w", err)
	}
	tf, err := w.queryOrderTotalFen(ctx, c, req.OrderID)
	if err != nil {
		return nil, err
	}
	rs := refunddomestic.RefundsApiService{Client: c}
	cur := wxpayCurrency
	outRefundNo := wxpayRefundID(req.OrderID, req.Amount)
	res, _, err := rs.Create(ctx, refunddomestic.CreateRequest{
		OutTradeNo:  core.String(req.OrderID),
		OutRefundNo: core.String(outRefundNo),
		Reason:      core.String(req.Reason),
		Amount:      &refunddomestic.AmountReq{Refund: core.Int64(rf), Total: core.Int64(tf), Currency: &cur},
	})
	if err != nil {
		return nil, fmt.Errorf("wxpay refund: %w", err)
	}
	st := payment.ProviderStatusPending
	if res.Status != nil && *res.Status == refunddomestic.STATUS_SUCCESS {
		st = payment.ProviderStatusSuccess
	}
	return &payment.RefundResponse{RefundID: outRefundNo, Status: st}, nil
}

func (w *Wxpay) QueryRefund(ctx context.Context, req payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	c, err := w.ensureClient()
	if err != nil {
		return nil, err
	}
	outRefundNo := strings.TrimSpace(req.RefundID)
	if outRefundNo == "" {
		outRefundNo = wxpayRefundID(req.OrderID, req.Amount)
	}
	if outRefundNo == "" {
		return nil, fmt.Errorf("wxpay query refund: missing refund id")
	}
	rs := refunddomestic.RefundsApiService{Client: c}
	res, _, err := rs.QueryByOutRefundNo(ctx, refunddomestic.QueryByOutRefundNoRequest{
		OutRefundNo: core.String(outRefundNo),
	})
	if err != nil {
		return nil, fmt.Errorf("wxpay query refund: %w", err)
	}
	status := payment.ProviderStatusPending
	if res != nil && res.Status != nil {
		switch *res.Status {
		case refunddomestic.STATUS_SUCCESS:
			status = payment.ProviderStatusSuccess
		case refunddomestic.STATUS_CLOSED, refunddomestic.STATUS_ABNORMAL:
			status = payment.ProviderStatusFailed
		default:
			status = payment.ProviderStatusPending
		}
	}
	return &payment.RefundResponse{RefundID: outRefundNo, Status: status}, nil
}

func wxpayRefundID(orderID, amount string) string {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return ""
	}
	amount = strings.NewReplacer(".", "", "-", "").Replace(strings.TrimSpace(amount))
	if amount == "" {
		return orderID + "-refund"
	}
	return orderID + "-refund-" + amount
}

func (w *Wxpay) queryOrderTotalFen(ctx context.Context, c *core.Client, orderID string) (int64, error) {
	svc := native.NativeApiService{Client: c}
	tx, _, err := svc.QueryOrderByOutTradeNo(ctx, native.QueryOrderByOutTradeNoRequest{
		OutTradeNo: core.String(orderID), Mchid: core.String(w.config["mchId"]),
	})
	if err != nil {
		return 0, fmt.Errorf("wxpay refund query order: %w", err)
	}
	var tf int64
	if tx.Amount != nil && tx.Amount.Total != nil {
		tf = *tx.Amount.Total
	}
	return tf, nil
}

func (w *Wxpay) CancelPayment(ctx context.Context, tradeNo string) error {
	c, err := w.ensureClient()
	if err != nil {
		return err
	}
	svc := native.NativeApiService{Client: c}
	_, err = svc.CloseOrder(ctx, native.CloseOrderRequest{
		OutTradeNo: core.String(tradeNo), Mchid: core.String(w.config["mchId"]),
	})
	if err != nil {
		return fmt.Errorf("wxpay cancel payment: %w", err)
	}
	return nil
}

var (
	_ payment.Provider           = (*Wxpay)(nil)
	_ payment.CancelableProvider = (*Wxpay)(nil)
)

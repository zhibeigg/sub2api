package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// KiroOAuthHandler exposes Kiro interactive login (Builder ID device code,
// IAM Identity Center PKCE, SSO token import), account creation from a login
// session, token refresh, and usage/overage management.
type KiroOAuthHandler struct {
	kiroOAuthService *service.KiroOAuthService
	kiroUsageService *service.KiroUsageService
	adminService     service.AdminService
}

func NewKiroOAuthHandler(
	kiroOAuthService *service.KiroOAuthService,
	kiroUsageService *service.KiroUsageService,
	adminService service.AdminService,
) *KiroOAuthHandler {
	return &KiroOAuthHandler{
		kiroOAuthService: kiroOAuthService,
		kiroUsageService: kiroUsageService,
		adminService:     adminService,
	}
}

// ---- Builder ID device-code flow ----

type kiroBuilderIDStartRequest struct {
	Region  string `json:"region"`
	ProxyID *int64 `json:"proxy_id"`
}

func (h *KiroOAuthHandler) StartBuilderID(c *gin.Context) {
	var req kiroBuilderIDStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = kiroBuilderIDStartRequest{}
	}
	result, err := h.kiroOAuthService.StartBuilderIDLogin(c.Request.Context(), req.Region, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type kiroBuilderIDPollRequest struct {
	SessionID string `json:"session_id" binding:"required"`
}

func (h *KiroOAuthHandler) PollBuilderID(c *gin.Context) {
	var req kiroBuilderIDPollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.kiroOAuthService.PollBuilderIDLogin(c.Request.Context(), req.SessionID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// When completed, surface the token info alongside the status so the client
	// can immediately create an account.
	payload := gin.H{"status": result.Status}
	if result.TokenInfo != nil {
		payload["token_info"] = result.TokenInfo
		payload["credentials"] = h.kiroOAuthService.BuildAccountCredentials(result.TokenInfo)
	}
	response.Success(c, payload)
}

// ---- IAM Identity Center PKCE flow ----

type kiroIAMSSOStartRequest struct {
	StartURL string `json:"start_url"`
	Region   string `json:"region"`
	ProxyID  *int64 `json:"proxy_id"`
}

func (h *KiroOAuthHandler) StartIAMSSO(c *gin.Context) {
	var req kiroIAMSSOStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = kiroIAMSSOStartRequest{}
	}
	result, err := h.kiroOAuthService.StartIAMSSOLogin(c.Request.Context(), req.StartURL, req.Region, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type kiroIAMSSOCompleteRequest struct {
	SessionID   string `json:"session_id" binding:"required"`
	CallbackURL string `json:"callback_url" binding:"required"`
}

func (h *KiroOAuthHandler) CompleteIAMSSO(c *gin.Context) {
	var req kiroIAMSSOCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	tokenInfo, err := h.kiroOAuthService.CompleteIAMSSOLogin(c.Request.Context(), req.SessionID, req.CallbackURL)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"token_info":  tokenInfo,
		"credentials": h.kiroOAuthService.BuildAccountCredentials(tokenInfo),
	})
}

// ---- SSO token import ----

type kiroSSOTokenRequest struct {
	BearerToken string `json:"bearer_token" binding:"required"`
	Region      string `json:"region"`
	ProxyID     *int64 `json:"proxy_id"`
}

func (h *KiroOAuthHandler) ImportSSOToken(c *gin.Context) {
	var req kiroSSOTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	tokenInfo, err := h.kiroOAuthService.ImportFromSSOToken(c.Request.Context(), req.BearerToken, req.Region, req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"token_info":  tokenInfo,
		"credentials": h.kiroOAuthService.BuildAccountCredentials(tokenInfo),
	})
}

// ---- Account creation from resolved credentials ----

type kiroCreateAccountRequest struct {
	Credentials  map[string]any    `json:"credentials" binding:"required"`
	Name         string            `json:"name"`
	ProxyID      *int64            `json:"proxy_id"`
	Concurrency  int               `json:"concurrency"`
	Priority     int               `json:"priority"`
	GroupIDs     []int64           `json:"group_ids"`
	ModelMapping map[string]string `json:"model_mapping"`
}

func (h *KiroOAuthHandler) CreateAccount(c *gin.Context) {
	var req kiroCreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if len(req.Credentials) == 0 {
		response.BadRequest(c, "credentials are required")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		if email, _ := req.Credentials["email"].(string); email != "" {
			name = email
		}
	}
	if name == "" {
		name = "Kiro OAuth Account"
	}

	credentials := req.Credentials
	if len(req.ModelMapping) > 0 {
		// model_mapping is persisted inside credentials (see DefaultKiroModelMapping).
		mapping := make(map[string]any, len(req.ModelMapping))
		for k, v := range req.ModelMapping {
			mapping[k] = v
		}
		credentials["model_mapping"] = mapping
	}

	account, err := h.adminService.CreateAccount(c.Request.Context(), &service.CreateAccountInput{
		Name:        name,
		Platform:    service.PlatformKiro,
		Type:        service.AccountTypeOAuth,
		Credentials: credentials,
		ProxyID:     req.ProxyID,
		Concurrency: req.Concurrency,
		Priority:    req.Priority,
		GroupIDs:    req.GroupIDs,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(account))
}

// ---- Manual token refresh ----

func (h *KiroOAuthHandler) RefreshAccountToken(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if account.Platform != service.PlatformKiro {
		response.BadRequest(c, "Account platform does not match Kiro OAuth endpoint")
		return
	}
	if !account.IsOAuth() {
		response.BadRequest(c, "Cannot refresh non-OAuth account credentials")
		return
	}
	tokenInfo, err := h.kiroOAuthService.RefreshAccountToken(c.Request.Context(), account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	newCredentials := h.kiroOAuthService.BuildAccountCredentials(tokenInfo)
	newCredentials = service.MergeCredentials(account.Credentials, newCredentials)
	updatedAccount, err := h.adminService.UpdateAccount(c.Request.Context(), accountID, &service.UpdateAccountInput{
		Credentials: newCredentials,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(updatedAccount))
}

// ---- Usage / overage ----

func (h *KiroOAuthHandler) QueryUsage(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if h.kiroUsageService == nil {
		response.BadRequest(c, "kiro usage service is not enabled")
		return
	}
	result, err := h.kiroUsageService.ProbeUsage(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type kiroSetOverageRequest struct {
	Enabled bool `json:"enabled"`
}

func (h *KiroOAuthHandler) SetOverage(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if h.kiroUsageService == nil {
		response.BadRequest(c, "kiro usage service is not enabled")
		return
	}
	var req kiroSetOverageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.kiroUsageService.SetOverage(c.Request.Context(), accountID, req.Enabled)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

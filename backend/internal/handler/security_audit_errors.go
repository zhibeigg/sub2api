package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/modelerror"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

func securityAuditDescriptor(decision *securityaudit.Decision) modelerror.Descriptor {
	if decision == nil {
		return modelerror.Descriptor{Code: modelerror.CodeServiceUnavailable}
	}
	blocked := decision.Kind == securityaudit.DecisionBlock || decision.Legacy != nil && decision.Legacy.Blocked
	if !blocked {
		return modelerror.Descriptor{Code: modelerror.CodeServiceUnavailable}
	}
	descriptor := modelerror.Descriptor{Code: modelerror.CodeContentPolicy}
	if message := strings.TrimSpace(securityAuditMessage(decision)); message != "" {
		descriptor = modelerror.WithCustomMessage(descriptor, message)
	}
	return descriptor
}

func (h *OpenAIGatewayHandler) openAISecurityAuditError(c *gin.Context, decision *securityaudit.Decision) {
	if decision == nil {
		return
	}
	errType := "api_error"
	if decision.Kind == securityaudit.DecisionBlock || decision.Legacy != nil && decision.Legacy.Blocked {
		errType = "permission_error"
	}
	modelerror.WriteOpenAIDescriptor(c, securityAuditStatus(decision), errType, securityAuditErrorCode(decision), securityAuditDescriptor(decision))
}

func (h *GatewayHandler) openAISecurityAuditError(c *gin.Context, decision *securityaudit.Decision) {
	if decision == nil {
		return
	}
	errType := "api_error"
	if decision.Kind == securityaudit.DecisionBlock || decision.Legacy != nil && decision.Legacy.Blocked {
		errType = "permission_error"
	}
	modelerror.WriteOpenAIDescriptor(c, securityAuditStatus(decision), errType, securityAuditErrorCode(decision), securityAuditDescriptor(decision))
}

func (h *GatewayHandler) responsesSecurityAuditError(c *gin.Context, decision *securityaudit.Decision) {
	if decision == nil {
		return
	}
	modelerror.WriteResponsesDescriptorWithType(c, securityAuditStatus(decision), "api_error", securityAuditErrorCode(decision), securityAuditDescriptor(decision))
}

func (h *GatewayHandler) anthropicSecurityAuditError(c *gin.Context, decision *securityaudit.Decision) {
	if decision == nil {
		return
	}
	errType := "api_error"
	if decision.Kind == securityaudit.DecisionBlock || decision.Legacy != nil && decision.Legacy.Blocked {
		errType = "permission_error"
	}
	modelerror.WriteAnthropicDescriptorWithCode(c, securityAuditStatus(decision), errType, securityAuditErrorCode(decision), securityAuditDescriptor(decision))
}

func (h *OpenAIGatewayHandler) anthropicSecurityAuditError(c *gin.Context, decision *securityaudit.Decision) {
	if decision == nil {
		return
	}
	errType := "api_error"
	if decision.Kind == securityaudit.DecisionBlock || decision.Legacy != nil && decision.Legacy.Blocked {
		errType = "permission_error"
	}
	modelerror.WriteAnthropicDescriptorWithCode(c, securityAuditStatus(decision), errType, securityAuditErrorCode(decision), securityAuditDescriptor(decision))
}

func googleSecurityAuditError(c *gin.Context, decision *securityaudit.Decision) {
	if decision == nil {
		return
	}
	status := securityAuditStatus(decision)
	requestID := ""
	if c != nil && c.Request != nil {
		requestID = contentModerationRequestID(c.Request.Context())
	}
	details := []gin.H{{
		"@type":  "type.googleapis.com/google.rpc.ErrorInfo",
		"reason": securityAuditErrorCode(decision), "domain": "sub2api.securityaudit",
		"metadata": gin.H{"request_id": requestID},
	}}
	googleStatus := ""
	if status == http.StatusServiceUnavailable {
		googleStatus = "UNAVAILABLE"
	}
	modelerror.WriteGoogleDescriptorWithDetails(c, status, googleStatus, details, securityAuditDescriptor(decision))
}

func writeSecurityAuditWSError(ctx context.Context, conn *coderws.Conn, decision *securityaudit.Decision) {
	if conn == nil || decision == nil {
		return
	}
	if decision.Legacy != nil && decision.Legacy.Blocked {
		legacy := decision.Legacy
		writeContentModerationWSError(ctx, conn, (legacyContentModerationDecision{legacy}).toService())
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	clientMessage := modelerror.Present(ctx, securityAuditDescriptor(decision)).Message
	payload, err := json.Marshal(gin.H{
		"event_id": "evt_prompt_guard_rejected", "type": "error",
		"error": gin.H{"type": "invalid_request_error", "code": securityAuditErrorCode(decision), "message": clientMessage},
	})
	if err != nil {
		return
	}
	writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_ = conn.Write(writeCtx, coderws.MessageText, payload)
}

type legacyContentModerationDecision struct{ value *securityaudit.LegacyDecision }

func (d legacyContentModerationDecision) toService() *service.ContentModerationDecision {
	if d.value == nil {
		return nil
	}
	return &service.ContentModerationDecision{Allowed: d.value.Allowed, Blocked: d.value.Blocked, Flagged: d.value.Flagged, Message: d.value.Message, StatusCode: d.value.StatusCode, Action: d.value.Action}
}

func securityAuditWSCloseStatus(decision *securityaudit.Decision) coderws.StatusCode {
	if decision == nil {
		return coderws.StatusInternalError
	}
	if decision.Legacy != nil && decision.Legacy.Blocked {
		return coderws.StatusPolicyViolation
	}
	if decision.Kind == securityaudit.DecisionBlock {
		return coderws.StatusCode(4403)
	}
	return coderws.StatusTryAgainLater
}

func securityAuditWSCloseReason(decision *securityaudit.Decision) string {
	if decision == nil {
		return securityaudit.ErrorCodeUnavailable
	}
	if decision.Legacy != nil && decision.Legacy.Blocked {
		message := strings.TrimSpace(decision.Legacy.Message)
		if message != "" {
			return message
		}
		return "content_policy_violation"
	}
	code := securityAuditErrorCode(decision)
	if code == "" {
		return securityaudit.ErrorCodeUnavailable
	}
	return code
}

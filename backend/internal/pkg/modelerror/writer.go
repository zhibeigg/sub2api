package modelerror

import (
	"context"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	"github.com/gin-gonic/gin"
)

const (
	ProtocolAnthropicMessages     = "anthropic_messages"
	ProtocolOpenAIChatCompletions = "openai_chat_completions"
	ProtocolOpenAIResponses       = "openai_responses"
	ProtocolGeminiGenerateContent = "gemini_generate_content"
	ProtocolOpenAIEmbeddings      = "openai_embeddings"
	ProtocolOpenAIAlphaSearch     = "openai_alpha_search"
	ProtocolOpenAIImages          = "openai_images"
	ProtocolOpenAIVideos          = "openai_videos"
)

func PresentForGin(c *gin.Context, descriptor Descriptor) Presentation {
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	return Present(ctx, descriptor)
}

func ApplyGinErrorHeaders(c *gin.Context, presentation Presentation) {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return
	}
	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}
	ApplyErrorHeaders(ctx, c.Writer.Header(), presentation)
}

func WriteStandard(c *gin.Context, status int, legacyCode, message string) {
	presentation := PresentForGin(c, LegacyDescriptor(status, legacyCode, message))
	ApplyGinErrorHeaders(c, presentation)
	c.JSON(status, gin.H{"code": legacyCode, "message": presentation.Message})
}

func WriteAnthropic(c *gin.Context, status int, errType, message string) {
	WriteAnthropicDescriptor(c, status, errType, LegacyDescriptor(status, errType, message))
}

func WriteAnthropicDescriptor(c *gin.Context, status int, errType string, descriptor Descriptor) {
	WriteAnthropicDescriptorWithCode(c, status, errType, "", descriptor)
}

func WriteAnthropicDescriptorWithCode(c *gin.Context, status int, errType, protocolCode string, descriptor Descriptor) {
	presentation := PresentForGin(c, descriptor)
	ApplyGinErrorHeaders(c, presentation)
	errorObject := gin.H{
		"type":    errType,
		"message": presentation.Message,
	}
	if strings.TrimSpace(protocolCode) != "" {
		errorObject["code"] = protocolCode
	}
	c.JSON(status, gin.H{
		"type":  "error",
		"error": errorObject,
	})
}

func WriteOpenAI(c *gin.Context, status int, errType, message string) {
	WriteOpenAIDescriptor(c, status, errType, "", LegacyDescriptor(status, errType, message))
}

func WriteOpenAIWithCode(c *gin.Context, status int, errType, protocolCode, message string) {
	WriteOpenAIDescriptor(c, status, errType, protocolCode, LegacyDescriptor(status, protocolCode, message))
}

func WriteOpenAIDescriptor(c *gin.Context, status int, errType, protocolCode string, descriptor Descriptor) {
	presentation := PresentForGin(c, descriptor)
	ApplyGinErrorHeaders(c, presentation)
	errorObject := gin.H{
		"type":    errType,
		"message": presentation.Message,
	}
	if strings.TrimSpace(protocolCode) != "" {
		errorObject["code"] = protocolCode
	}
	c.JSON(status, gin.H{"error": errorObject})
}

func WriteResponses(c *gin.Context, status int, protocolCode, message string) {
	WriteResponsesDescriptor(c, status, protocolCode, LegacyDescriptor(status, protocolCode, message))
}

func WriteResponsesDescriptor(c *gin.Context, status int, protocolCode string, descriptor Descriptor) {
	WriteResponsesDescriptorWithType(c, status, "", protocolCode, descriptor)
}

func WriteResponsesDescriptorWithType(c *gin.Context, status int, errType, protocolCode string, descriptor Descriptor) {
	presentation := PresentForGin(c, descriptor)
	ApplyGinErrorHeaders(c, presentation)
	errorObject := gin.H{
		"code":    protocolCode,
		"message": presentation.Message,
	}
	if strings.TrimSpace(errType) != "" {
		errorObject["type"] = errType
	}
	c.JSON(status, gin.H{"error": errorObject})
}

func WriteGoogle(c *gin.Context, status int, message string) {
	WriteGoogleDescriptor(c, status, LegacyDescriptor(status, googleapi.HTTPStatusToGoogleStatus(status), message))
}

func WriteGoogleDescriptor(c *gin.Context, status int, descriptor Descriptor) {
	WriteGoogleDescriptorWithDetails(c, status, "", nil, descriptor)
}

func WriteGoogleDescriptorWithDetails(c *gin.Context, status int, googleStatus string, details any, descriptor Descriptor) {
	presentation := PresentForGin(c, descriptor)
	ApplyGinErrorHeaders(c, presentation)
	if strings.TrimSpace(googleStatus) == "" {
		googleStatus = googleapi.HTTPStatusToGoogleStatus(status)
	}
	errorObject := gin.H{
		"code":    status,
		"message": presentation.Message,
		"status":  googleStatus,
	}
	if details != nil {
		errorObject["details"] = details
	}
	c.JSON(status, gin.H{"error": errorObject})
}

// WriteGatewayProtocol writes an early gateway error in the public ingress
// protocol. It returns false for non-model routes so callers can keep their
// ordinary REST error envelope.
func WriteGatewayProtocol(c *gin.Context, protocol string, status int, legacyCode, message string) bool {
	return WriteGatewayProtocolDescriptor(c, protocol, status, "", legacyCode, LegacyDescriptor(status, legacyCode, message))
}

// WriteGatewayProtocolDescriptor preserves the public protocol envelope while
// allowing callers to provide a stable, already classified client error.
func WriteGatewayProtocolDescriptor(c *gin.Context, protocol string, status int, errType, protocolCode string, descriptor Descriptor) bool {
	if strings.TrimSpace(errType) == "" {
		errType = protocolErrorType(status)
	}
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case ProtocolAnthropicMessages:
		WriteAnthropicDescriptorWithCode(c, status, errType, protocolCode, descriptor)
	case ProtocolOpenAIResponses:
		if strings.TrimSpace(protocolCode) == "" {
			protocolCode = protocolErrorCode(status, "")
		}
		WriteResponsesDescriptorWithType(c, status, errType, protocolCode, descriptor)
	case ProtocolGeminiGenerateContent:
		WriteGoogleDescriptor(c, status, descriptor)
	case ProtocolOpenAIChatCompletions, ProtocolOpenAIEmbeddings, ProtocolOpenAIAlphaSearch, ProtocolOpenAIImages, ProtocolOpenAIVideos:
		WriteOpenAIDescriptor(c, status, errType, protocolCode, descriptor)
	default:
		return false
	}
	return true
}

func protocolErrorType(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity:
		return "invalid_request_error"
	case http.StatusNotFound:
		return "not_found_error"
	default:
		if status >= 500 {
			return "api_error"
		}
		return "api_error"
	}
}

func protocolErrorCode(status int, legacyCode string) string {
	legacyCode = strings.ToLower(strings.TrimSpace(legacyCode))
	if legacyCode != "" {
		return legacyCode
	}
	switch status {
	case http.StatusUnauthorized:
		return "authentication_failed"
	case http.StatusForbidden:
		return "permission_denied"
	case http.StatusTooManyRequests:
		return "rate_limit_exceeded"
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity:
		return "invalid_request"
	case http.StatusNotFound:
		return "not_found"
	default:
		return "server_error"
	}
}

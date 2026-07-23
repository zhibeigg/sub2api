package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/modelerror"
	"github.com/gin-gonic/gin"
)

func presentServiceModelError(c *gin.Context, descriptor modelerror.Descriptor) modelerror.Presentation {
	presentation := modelerror.PresentForGin(c, descriptor)
	modelerror.ApplyGinErrorHeaders(c, presentation)
	return presentation
}

func presentLegacyServiceModelError(c *gin.Context, status int, legacyCode, message string) modelerror.Presentation {
	descriptor := modelerror.FromLegacy(status, legacyCode, message)
	if strings.HasPrefix(strings.TrimSpace(message), modelerror.BrandPrefix) {
		descriptor = modelerror.WithCustomMessage(descriptor, message)
	}
	return presentServiceModelError(c, descriptor)
}

func presentUpstreamServiceModelError(c *gin.Context, status int, body []byte, message string, err error, model string) modelerror.Presentation {
	return presentServiceModelError(c, modelerror.ClassifyUpstream(modelerror.UpstreamInput{
		Status:  status,
		Body:    body,
		Message: message,
		Err:     err,
		Model:   model,
	}))
}

func presentUpstreamCustomServiceModelError(c *gin.Context, status int, body []byte, message, model string) modelerror.Presentation {
	descriptor := modelerror.ClassifyUpstream(modelerror.UpstreamInput{
		Status: status,
		Body:   body,
		Model:  model,
	})
	if strings.TrimSpace(message) != "" {
		descriptor = modelerror.WithCustomMessage(descriptor, message)
	}
	return presentServiceModelError(c, descriptor)
}

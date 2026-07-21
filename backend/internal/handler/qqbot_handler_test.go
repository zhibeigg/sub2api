package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLegacyUpdateSettingsRejectsChannelCheckField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/integrations/qqbot/settings", bytes.NewBufferString(`{"channel_check_enabled":true}`))
	context.Request.Header.Set("Content-Type", "application/json")

	NewQQBotHandler(nil, nil, nil, nil, nil).UpdateSettings(context)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestChannelCheckSignatureQueryRejectsDuplicateAndExtraParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	validURL := "/image?v=1&exp=123&nonce=nonce&sig=signature"
	for _, test := range []struct {
		name string
		url  string
		ok   bool
	}{
		{name: "valid", url: validURL, ok: true},
		{name: "duplicate", url: validURL + "&sig=second", ok: false},
		{name: "extra", url: validURL + "&debug=1", ok: false},
		{name: "malformed escape", url: validURL + "&debug=%zz", ok: false},
		{name: "semicolon", url: validURL + ";debug=1", ok: false},
		{name: "empty key", url: validURL + "&=debug", ok: false},
		{name: "missing", url: "/image?v=1&exp=123&nonce=nonce", ok: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest("GET", test.url, nil)
			_, _, _, _, ok := channelCheckSignatureQuery(context)
			if ok != test.ok {
				t.Fatalf("query accepted=%v want=%v", ok, test.ok)
			}
		})
	}
}

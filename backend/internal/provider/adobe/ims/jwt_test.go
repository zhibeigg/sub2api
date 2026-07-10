package ims

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func makeJWT(v map[string]any) string {
	b, _ := json.Marshal(v)
	return "a." + base64.RawURLEncoding.EncodeToString(b) + ".c"
}
func TestJWTHelpers(t *testing.T) {
	tok := makeJWT(map[string]any{"created_at": 1700000000000, "expires_in": 3600000, "user_id": "abc@AdobeID"})
	if got := ExtractJWTExpiry(tok); got != 1700003600 {
		t.Fatalf("expiry=%d", got)
	}
	if got := ExtractAccountIDFromJWT(tok); got != "abc@AdobeID" {
		t.Fatalf("id=%s", got)
	}
	if !IsExpiringWithin(0, time.Hour) {
		t.Fatal("unknown expiry must refresh")
	}
}

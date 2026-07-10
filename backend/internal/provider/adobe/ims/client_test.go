package ims

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCookieDeviceRefreshAndCredits(t *testing.T) {
	jwt := makeJWT(map[string]any{"exp": 1900000000, "user_id": "u@AdobeID"})
	var cookieSeen, deviceSeen bool
	creditsMode := "zero"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cookie":
			cookieSeen = r.Header.Get("Cookie") == "ims_sid=secret"
			fmt.Fprintf(w, `{"access_token":%q,"expires_in":3600}`, jwt)
		case "/device":
			b, _ := io.ReadAll(r.Body)
			form, _ := url.ParseQuery(string(b))
			deviceSeen = form.Get("device_token") == "device-secret" && form.Get("device_id") == "device-id"
			fmt.Fprintf(w, `{"access_token":%q,"expires_in":"3600"}`, jwt)
		case "/profile":
			fmt.Fprint(w, `{"displayName":"Test","email":"a@example.com","userId":"u@AdobeID"}`)
		case "/credits":
			if creditsMode == "zero" {
				fmt.Fprint(w, `{"total":{"quota":{"available":0}}}`)
			} else {
				fmt.Fprint(w, `{"total":{}}`)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old1, old2, old3, old4 := imsTokenURL, imsDeviceTokenURL, imsProfileURL, creditsURL
	imsTokenURL, imsDeviceTokenURL, imsProfileURL, creditsURL = srv.URL+"/cookie", srv.URL+"/device", srv.URL+"/profile", srv.URL+"/credits"
	defer func() { imsTokenURL, imsDeviceTokenURL, imsProfileURL, creditsURL = old1, old2, old3, old4 }()
	opt := RefreshOptions{}
	a, err := RefreshAccessTokenViaCookie(context.Background(), "ims_sid=secret", opt)
	if err != nil || a.AccessToken != jwt || !cookieSeen {
		t.Fatalf("cookie refresh: %+v %v", a, err)
	}
	d, err := RefreshAccessTokenViaDeviceToken(context.Background(), "device-secret", "device-id", opt)
	if err != nil || d.AccessToken != jwt || !deviceSeen {
		t.Fatalf("device refresh: %+v %v", d, err)
	}
	if got := FetchCredits(context.Background(), jwt, "u@AdobeID", opt); got != 0 {
		t.Fatalf("zero credits=%v", got)
	}
	creditsMode = "unknown"
	if got := FetchCredits(context.Background(), jwt, "u@AdobeID", opt); got != -1 {
		t.Fatalf("unknown credits=%v", got)
	}
}
func TestRefreshErrorsDoNotLeakSecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"token":"secret-token","cookie":"secret-cookie"}`)
	}))
	defer srv.Close()
	old := imsTokenURL
	imsTokenURL = srv.URL
	defer func() { imsTokenURL = old }()
	_, err := RefreshAccessTokenViaCookie(context.Background(), "secret-cookie", RefreshOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	s := err.Error()
	for _, secret := range []string{"secret-cookie", "secret-token"} {
		if strings.Contains(s, secret) {
			t.Fatalf("leaked %s", secret)
		}
	}
}

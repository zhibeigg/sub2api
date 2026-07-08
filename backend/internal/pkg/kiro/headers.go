package kiro

import (
	"fmt"
	"net/http"
)

const (
	kiroStreamingSDKVersion = "1.0.34"
	kiroRuntimeSDKVersion   = "1.0.0"
)

// ClientConfig holds the Kiro IDE client identity embedded in the User-Agent.
// These defaults mirror the values Kiro-Go ships; they can be overridden via
// SetClientConfig if AWS starts rejecting stale client versions.
type ClientConfig struct {
	SystemVersion string
	NodeVersion   string
	KiroVersion   string
}

var defaultClientConfig = ClientConfig{
	SystemVersion: "darwin#24.5.0",
	NodeVersion:   "20.18.1",
	KiroVersion:   "0.11.107",
}

// SetClientConfig overrides the embedded Kiro client identity at runtime.
func SetClientConfig(cfg ClientConfig) {
	if cfg.SystemVersion != "" {
		defaultClientConfig.SystemVersion = cfg.SystemVersion
	}
	if cfg.NodeVersion != "" {
		defaultClientConfig.NodeVersion = cfg.NodeVersion
	}
	if cfg.KiroVersion != "" {
		defaultClientConfig.KiroVersion = cfg.KiroVersion
	}
}

type kiroHeaderValues struct {
	UserAgent    string
	AmzUserAgent string
	Host         string
}

func buildStreamingHeaderValues(cred *Credential, host string) kiroHeaderValues {
	return buildKiroHeaderValues(cred, host, "codewhispererstreaming", kiroStreamingSDKVersion, "m/E")
}

// buildRuntimeHeaderValues builds the User-Agent header values for the
// non-streaming CodeWhisperer runtime REST APIs (usage limits, models,
// profiles, user info, overage preference).
func buildRuntimeHeaderValues(cred *Credential, host string) kiroHeaderValues {
	return buildKiroHeaderValues(cred, host, "codewhispererruntime", kiroRuntimeSDKVersion, "m/N,E")
}

func buildKiroHeaderValues(cred *Credential, host, apiName, sdkVersion, mode string) kiroHeaderValues {
	cfg := defaultClientConfig
	machineID := ""
	if cred != nil {
		machineID = cred.MachineID
	}

	userAgent := fmt.Sprintf(
		"aws-sdk-js/%s ua/2.1 os/%s lang/js md/nodejs#%s api/%s#%s %s KiroIDE-%s",
		sdkVersion,
		cfg.SystemVersion,
		cfg.NodeVersion,
		apiName,
		sdkVersion,
		mode,
		cfg.KiroVersion,
	)
	amzUserAgent := fmt.Sprintf("aws-sdk-js/%s KiroIDE-%s", sdkVersion, cfg.KiroVersion)
	if machineID != "" {
		userAgent += "-" + machineID
		amzUserAgent += "-" + machineID
	}

	return kiroHeaderValues{
		UserAgent:    userAgent,
		AmzUserAgent: amzUserAgent,
		Host:         host,
	}
}

func applyKiroBaseHeaders(req *http.Request, cred *Credential, values kiroHeaderValues) {
	if cred != nil && cred.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+cred.AccessToken)
	}
	req.Header.Set("User-Agent", values.UserAgent)
	req.Header.Set("x-amz-user-agent", values.AmzUserAgent)
	req.Header.Set("x-amzn-codewhisperer-optout", "true")
	if values.Host != "" {
		req.Host = values.Host
	}
}

// setKiroRuntimeHeaders applies the standard headers for a REST runtime request
// (usage limits, models, profiles, user info, overage). It derives the Host from
// the request URL and sets Accept: application/json.
func setKiroRuntimeHeaders(req *http.Request, cred *Credential) {
	host := ""
	if req.URL != nil {
		host = req.URL.Host
	}
	values := buildRuntimeHeaderValues(cred, host)
	req.Header.Set("Accept", "application/json")
	applyKiroBaseHeaders(req, cred, values)
}

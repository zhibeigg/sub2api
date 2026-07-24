package qqbot

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

const (
	headerSignature = "X-Signature-Ed25519"
	headerTimestamp = "X-Signature-Timestamp"
)

func (r *Runtime) ServeWebhook(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		http.Error(writer, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, MaxWebhookBodyBytes+1))
	if err != nil {
		http.Error(writer, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if int64(len(body)) > MaxWebhookBodyBytes {
		http.Error(writer, `{"error":"request body too large"}`, http.StatusRequestEntityTooLarge)
		return
	}
	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(writer, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}
	cfg, ok := r.manager.Active()
	if !ok || normalizeTransportMode(cfg.TransportMode) != TransportModeBotGo || cfg.AppID == "" || cfg.WebhookSecret == "" {
		http.Error(writer, `{"error":"qqbot credentials are not configured"}`, http.StatusServiceUnavailable)
		return
	}
	if payload.OPCode == 13 {
		if appID := strings.TrimSpace(request.Header.Get("X-Bot-Appid")); appID != "" && appID != cfg.AppID {
			// Tencent's validation protocol authenticates the callback by checking the
			// Ed25519 signature generated from the configured Bot Secret. Keep the
			// header mismatch observable, but do not reject before that proof can run.
			slog.Warn("qqbot webhook validation app id mismatch", "expected_app_id", cfg.AppID, "received_app_id", appID)
		}
		var validation struct {
			PlainToken string `json:"plain_token"`
			EventTS    string `json:"event_ts"`
		}
		if err := json.Unmarshal(payload.Data, &validation); err != nil || validation.PlainToken == "" || validation.EventTS == "" {
			http.Error(writer, `{"error":"invalid validation payload"}`, http.StatusBadRequest)
			return
		}
		signature, err := generateSignature(cfg.WebhookSecret, validation.EventTS, []byte(validation.PlainToken))
		if err != nil {
			http.Error(writer, `{"error":"validation failed"}`, http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]string{"plain_token": validation.PlainToken, "signature": signature})
		r.MarkWebhook()
		return
	}
	generation := r.generation.Load()
	if generation == nil || !generation.config.Enabled {
		http.Error(writer, `{"error":"qq bot is disabled"}`, http.StatusServiceUnavailable)
		return
	}
	cfg = generation.config
	verified, err := verifySignature(cfg.WebhookSecret, request.Header.Get(headerTimestamp), request.Header.Get(headerSignature), body)
	if err != nil || !verified {
		http.Error(writer, `{"error":"invalid signature"}`, http.StatusUnauthorized)
		return
	}
	r.MarkWebhook()
	switch payload.OPCode {
	case 1:
		var sequence uint32
		if err := json.Unmarshal(payload.Data, &sequence); err != nil {
			http.Error(writer, `{"error":"invalid heartbeat"}`, http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"op": 11, "d": sequence})
	case 0:
		if err := dispatchWebhookPayload(payload); err != nil {
			_ = json.NewEncoder(writer).Encode(map[string]any{"op": 12, "d": 1})
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"op": 12, "d": 0})
	default:
		_ = json.NewEncoder(writer).Encode(map[string]any{"op": 12, "d": 0})
	}
}

func generateSignature(secret, timestamp string, body []byte) (string, error) {
	privateKey, _, err := signatureKeys(secret)
	if err != nil {
		return "", err
	}
	content := append([]byte(timestamp), body...)
	return hex.EncodeToString(ed25519.Sign(privateKey, content)), nil
}
func verifySignature(secret, timestamp, signature string, body []byte) (bool, error) {
	if timestamp == "" || signature == "" {
		return false, errors.New("signature headers missing")
	}
	_, publicKey, err := signatureKeys(secret)
	if err != nil {
		return false, err
	}
	sig, err := hex.DecodeString(signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false, errors.New("invalid signature encoding")
	}
	content := append([]byte(timestamp), body...)
	return ed25519.Verify(publicKey, content, sig), nil
}
func signatureKeys(secret string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	if secret == "" {
		return nil, nil, errors.New("secret invalid")
	}
	seed := secret
	for len(seed) < ed25519.SeedSize {
		seed += seed
	}
	seed = seed[:ed25519.SeedSize]
	publicKey, privateKey, err := ed25519.GenerateKey(bytes.NewReader([]byte(seed)))
	return privateKey, publicKey, err
}

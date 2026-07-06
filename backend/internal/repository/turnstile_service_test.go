package repository

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const testCapVerifyURL = "http://in-process/site-key/siteverify"

type TurnstileServiceSuite struct {
	suite.Suite
	ctx      context.Context
	verifier *turnstileVerifier
	received chan capVerifyRequest
}

func (s *TurnstileServiceSuite) SetupTest() {
	s.ctx = context.Background()
	s.received = make(chan capVerifyRequest, 1)
	verifier, ok := NewTurnstileVerifier().(*turnstileVerifier)
	require.True(s.T(), ok, "type assertion failed")
	s.verifier = verifier
}

func (s *TurnstileServiceSuite) setupTransport(handler http.HandlerFunc) {
	s.verifier.httpClient = &http.Client{
		Transport: newInProcessTransport(handler, nil),
	}
}

func (s *TurnstileServiceSuite) TestVerifyToken_SendsJSONAndDecodes() {
	s.setupTransport(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req capVerifyRequest
		_ = json.Unmarshal(body, &req)
		s.received <- req

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.TurnstileVerifyResponse{Success: true})
	}))

	resp, err := s.verifier.VerifyToken(s.ctx, testCapVerifyURL, "sk", "token", "1.1.1.1")
	require.NoError(s.T(), err, "VerifyToken")
	require.NotNil(s.T(), resp)
	require.True(s.T(), resp.Success, "expected success response")

	select {
	case req := <-s.received:
		require.Equal(s.T(), "sk", req.Secret)
		require.Equal(s.T(), "token", req.Response)
		require.Equal(s.T(), "1.1.1.1", req.RemoteIP)
	default:
		require.Fail(s.T(), "expected server to receive request")
	}
}

func (s *TurnstileServiceSuite) TestVerifyToken_ContentType() {
	var contentType string
	s.setupTransport(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.TurnstileVerifyResponse{Success: true})
	}))

	_, err := s.verifier.VerifyToken(s.ctx, testCapVerifyURL, "sk", "token", "1.1.1.1")
	require.NoError(s.T(), err)
	require.Equal(s.T(), "application/json", contentType, "unexpected content-type: %s", contentType)
}

func (s *TurnstileServiceSuite) TestVerifyToken_EmptyRemoteIP_NotSent() {
	var rawBody string
	s.setupTransport(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rawBody = string(body)
		var req capVerifyRequest
		_ = json.Unmarshal(body, &req)
		s.received <- req

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.TurnstileVerifyResponse{Success: true})
	}))

	_, err := s.verifier.VerifyToken(s.ctx, testCapVerifyURL, "sk", "token", "")
	require.NoError(s.T(), err)

	select {
	case req := <-s.received:
		require.Equal(s.T(), "", req.RemoteIP, "remoteip should be empty")
		require.NotContains(s.T(), rawBody, "remoteip", "remoteip should be omitted from JSON")
	default:
		require.Fail(s.T(), "expected server to receive request")
	}
}

func (s *TurnstileServiceSuite) TestVerifyToken_RequestError() {
	s.verifier.httpClient = &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial failed")
		}),
	}

	_, err := s.verifier.VerifyToken(s.ctx, testCapVerifyURL, "sk", "token", "1.1.1.1")
	require.Error(s.T(), err, "expected error when server is unreachable")
}

func (s *TurnstileServiceSuite) TestVerifyToken_InvalidJSON() {
	s.setupTransport(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "not-valid-json")
	}))

	_, err := s.verifier.VerifyToken(s.ctx, testCapVerifyURL, "sk", "token", "1.1.1.1")
	require.Error(s.T(), err, "expected error for invalid JSON response")
}

func (s *TurnstileServiceSuite) TestVerifyToken_SuccessFalse() {
	s.setupTransport(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.TurnstileVerifyResponse{
			Success:    false,
			ErrorCodes: []string{"invalid-input-response"},
		})
	}))

	resp, err := s.verifier.VerifyToken(s.ctx, testCapVerifyURL, "sk", "token", "1.1.1.1")
	require.NoError(s.T(), err, "VerifyToken should not error on success=false")
	require.NotNil(s.T(), resp)
	require.False(s.T(), resp.Success)
	require.Contains(s.T(), resp.ErrorCodes, "invalid-input-response")
}

func TestTurnstileServiceSuite(t *testing.T) {
	suite.Run(t, new(TurnstileServiceSuite))
}

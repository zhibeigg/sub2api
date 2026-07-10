package cursor

import (
	"errors"
	"fmt"
	"net/http"
)

type ErrorKind string

const (
	ErrorBadRequest   ErrorKind = "bad_request"
	ErrorUnauthorized ErrorKind = "unauthorized"
	ErrorForbidden    ErrorKind = "forbidden"
	ErrorRateLimited  ErrorKind = "rate_limited"
	ErrorUpstream     ErrorKind = "upstream"
	ErrorTransport    ErrorKind = "transport"
	ErrorProtocol     ErrorKind = "protocol"
)

type Error struct {
	Kind         ErrorKind
	StatusCode   int
	Operation    string
	Body         string
	FailoverSafe bool
	Err          error
}

func (e *Error) Error() string {
	if e == nil {
		return "cursor: <nil>"
	}
	message := string(e.Kind)
	if e.StatusCode != 0 {
		message = fmt.Sprintf("HTTP %d", e.StatusCode)
	}
	if e.Body != "" {
		message += ": " + e.Body
	}
	if e.Err != nil {
		message += ": " + e.Err.Error()
	}
	if e.Operation != "" {
		return "cursor " + e.Operation + ": " + message
	}
	return "cursor: " + message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func IsKind(err error, kind ErrorKind) bool {
	var target *Error
	return errors.As(err, &target) && target.Kind == kind
}

func HTTPError(status int, operation, body string) *Error {
	result := &Error{StatusCode: status, Operation: operation, Body: body}
	switch {
	case status == http.StatusBadRequest:
		result.Kind = ErrorBadRequest
	case status == http.StatusUnauthorized:
		result.Kind = ErrorUnauthorized
	case status == http.StatusForbidden:
		result.Kind = ErrorForbidden
	case status == http.StatusTooManyRequests:
		result.Kind = ErrorRateLimited
		result.FailoverSafe = true
	case status >= 500:
		result.Kind = ErrorUpstream
		result.FailoverSafe = true
	default:
		result.Kind = ErrorUpstream
	}
	return result
}

func transportError(operation string, err error) *Error {
	return &Error{Kind: ErrorTransport, Operation: operation, FailoverSafe: true, Err: err}
}

func protocolError(operation string, err error) *Error {
	return &Error{Kind: ErrorProtocol, Operation: operation, Err: err}
}

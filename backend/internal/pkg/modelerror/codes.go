package modelerror

// Code is a stable, protocol-independent error identifier exposed through
// X-PokeAPI-Error-Code. Protocol-specific JSON fields keep their existing
// semantics for SDK compatibility.
type Code string

const (
	CodeAuthRequired         Code = "POKE_AUTH_REQUIRED"
	CodeAuthInvalid          Code = "POKE_AUTH_INVALID"
	CodeAuthDisabled         Code = "POKE_AUTH_DISABLED"
	CodeAuthExpired          Code = "POKE_AUTH_EXPIRED"
	CodeAuthUnavailable      Code = "POKE_AUTH_UNAVAILABLE"
	CodePermissionDenied     Code = "POKE_PERMISSION_DENIED"
	CodeQuotaExhausted       Code = "POKE_QUOTA_EXHAUSTED"
	CodeBalanceInsufficient  Code = "POKE_BALANCE_INSUFFICIENT"
	CodeSubscriptionRequired Code = "POKE_SUBSCRIPTION_REQUIRED"
	CodeUsageLimitExceeded   Code = "POKE_USAGE_LIMIT_EXCEEDED"
	CodeGroupUnavailable     Code = "POKE_GROUP_UNAVAILABLE"
	CodeEndpointNotAllowed   Code = "POKE_ENDPOINT_NOT_ALLOWED"
	CodeInvalidRequest       Code = "POKE_INVALID_REQUEST"
	CodePayloadTooLarge      Code = "POKE_PAYLOAD_TOO_LARGE"
	CodeModelRequired        Code = "POKE_MODEL_REQUIRED"
	CodeModelUnsupported     Code = "POKE_MODEL_UNSUPPORTED"
	CodeModelNotFound        Code = "POKE_MODEL_NOT_FOUND"
	CodeContextTooLarge      Code = "POKE_CONTEXT_TOO_LARGE"
	CodeConcurrencyLimit     Code = "POKE_CONCURRENCY_LIMIT"
	CodeQueueFull            Code = "POKE_QUEUE_FULL"
	CodeRateLimited          Code = "POKE_RATE_LIMITED"
	CodeContentPolicy        Code = "POKE_CONTENT_POLICY"
	CodeUpstreamAuthFailed   Code = "POKE_UPSTREAM_AUTH_FAILED"
	CodeUpstreamForbidden    Code = "POKE_UPSTREAM_FORBIDDEN"
	CodeUpstreamRateLimited  Code = "POKE_UPSTREAM_RATE_LIMITED"
	CodeUpstreamOverloaded   Code = "POKE_UPSTREAM_OVERLOADED"
	CodeUpstreamTimeout      Code = "POKE_UPSTREAM_TIMEOUT"
	CodeUpstreamUnavailable  Code = "POKE_UPSTREAM_UNAVAILABLE"
	CodeUpstreamBadResponse  Code = "POKE_UPSTREAM_BAD_RESPONSE"
	CodeServiceUnavailable   Code = "POKE_SERVICE_UNAVAILABLE"
	CodeInternalError        Code = "POKE_INTERNAL_ERROR"
)

// Params contains only values that are safe to interpolate into a client-facing
// message after normalization.
type Params struct {
	Model      string
	LimitBytes int64
	RetryAfter int
	Scope      string
}

// Descriptor describes an error before locale-specific presentation.
type Descriptor struct {
	Code          Code
	Params        Params
	CustomMessage string
}

// Presentation is the final client-facing representation.
type Presentation struct {
	Code    Code
	Locale  Locale
	Message string
}

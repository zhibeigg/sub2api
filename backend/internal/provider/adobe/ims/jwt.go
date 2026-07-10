package ims

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

type jwtClaims struct {
	Exp       any    `json:"exp"`
	CreatedAt any    `json:"created_at"`
	ExpiresIn any    `json:"expires_in"`
	UserID    string `json:"user_id"`
	AaID      string `json:"aa_id"`
	Sub       string `json:"sub"`
}

func decodeJWT(token string) *jwtClaims {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var c jwtClaims
	if json.Unmarshal(raw, &c) != nil {
		return nil
	}
	return &c
}
func ExtractJWTExpiry(token string) int64 {
	c := decodeJWT(token)
	if c == nil {
		return 0
	}
	if exp := toInt64(c.Exp); exp > 0 {
		if exp > 10_000_000_000 {
			exp /= 1000
		}
		return exp
	}
	created, ttl := toInt64(c.CreatedAt), toInt64(c.ExpiresIn)
	if created > 10_000_000_000 {
		created /= 1000
	}
	if ttl > 172800 {
		ttl /= 1000
	}
	if created > 0 && ttl > 0 {
		return created + ttl
	}
	return 0
}
func ExtractAccountIDFromJWT(token string) string {
	c := decodeJWT(token)
	if c == nil {
		return ""
	}
	for _, v := range []string{c.UserID, c.AaID, c.Sub} {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
func IsExpiringWithin(expAt int64, within time.Duration) bool {
	if expAt <= 0 {
		return true
	}
	return time.Until(time.Unix(expAt, 0)) <= within
}
func toInt64(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int:
		return int64(x)
	case int64:
		return x
	case json.Number:
		n, _ := x.Int64()
		return n
	case string:
		n := json.Number(strings.TrimSpace(x))
		i, _ := n.Int64()
		return i
	}
	return 0
}

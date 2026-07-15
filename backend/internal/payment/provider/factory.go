package provider

import (
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// ProviderConfigError identifies an invalid provider configuration field.
type ProviderConfigError struct {
	ProviderKey string
	Field       string
	Reason      string
	Err         error
}

// ConfigError is kept as a concise alias for callers using errors.As.
type ConfigError = ProviderConfigError

func (e *ProviderConfigError) Error() string {
	if e == nil {
		return "provider config error"
	}
	message := fmt.Sprintf("%s config invalid", e.ProviderKey)
	if e.Field != "" {
		message += ": " + e.Field
	}
	if e.Reason != "" {
		message += " " + e.Reason
	}
	if e.Err != nil {
		message += ": " + e.Err.Error()
	}
	return message
}

func (e *ProviderConfigError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newProviderConfigError(providerKey, field, reason string, err error) error {
	return &ProviderConfigError{ProviderKey: providerKey, Field: field, Reason: reason, Err: err}
}

// CreateProvider creates a Provider from a provider key, instance ID and decrypted config.
func CreateProvider(providerKey string, instanceID string, config map[string]string) (payment.Provider, error) {
	switch providerKey {
	case payment.TypeEasyPay:
		protocolVersion := strings.TrimSpace(config["protocolVersion"])
		switch protocolVersion {
		case "", "1":
			return NewEasyPay(instanceID, config)
		case "2":
			return NewEasyPayV2(instanceID, config)
		default:
			return nil, newProviderConfigError(payment.TypeEasyPay, "protocolVersion", "must be 1 or 2", nil)
		}
	case payment.TypeAlipay:
		return NewAlipay(instanceID, config)
	case payment.TypeWxpay:
		return NewWxpay(instanceID, config)
	case payment.TypeStripe:
		return NewStripe(instanceID, config)
	case payment.TypeAirwallex:
		return NewAirwallex(instanceID, config)
	default:
		return nil, fmt.Errorf("unknown provider key: %s", providerKey)
	}
}

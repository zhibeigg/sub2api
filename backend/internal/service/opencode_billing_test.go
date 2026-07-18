package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBillingService_OpenCodeGoPricingIsProviderScoped(t *testing.T) {
	svc := NewBillingService(nil, nil)

	openCode, err := svc.GetModelPricing("opencode-go/glm-5.2")
	require.NoError(t, err)
	require.InDelta(t, 1.40e-6, openCode.InputPricePerToken, 1e-12)
	require.InDelta(t, 4.40e-6, openCode.OutputPricePerToken, 1e-12)
	require.InDelta(t, 0.26e-6, openCode.CacheReadPricePerToken, 1e-12)

	global, err := svc.GetModelPricing("glm-5.2")
	require.NoError(t, err)
	require.InDelta(t, 1.00e-6, global.InputPricePerToken, 1e-12)
	require.InDelta(t, 3.20e-6, global.OutputPricePerToken, 1e-12)
}

func TestBillingService_OpenCodeGoUnknownModelFailsClosed(t *testing.T) {
	svc := NewBillingService(nil, nil)

	_, err := svc.GetModelPricing("opencode-go/future-model-without-price")
	require.ErrorIs(t, err, ErrModelPricingUnavailable)
}

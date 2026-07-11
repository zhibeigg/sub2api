//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestResolveUserPromoSnapshotAndCreateOrder(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	promo, err := client.PromoCode.Create().
		SetCode("WELCOME25").
		SetRechargeBonusMultiplier(1.25).
		Save(ctx)
	require.NoError(t, err)
	userEntity, err := client.User.Create().
		SetEmail("promo-order@example.com").
		SetPasswordHash("hash").
		SetUsername("promo-order-user").
		SetPromoCodeID(promo.ID).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	user := &User{
		ID:          userEntity.ID,
		Email:       userEntity.Email,
		Username:    userEntity.Username,
		PromoCodeID: &promo.ID,
	}
	snapshot := svc.resolveUserPromoSnapshot(ctx, user)
	require.Equal(t, PromoAttributionAttributed, snapshot.Attribution)
	require.Equal(t, promo.ID, *snapshot.PromoCodeID)
	require.Equal(t, "WELCOME25", *snapshot.PromoCode)
	require.Equal(t, 1.25, snapshot.RechargeBonusMultiplier)

	order, err := svc.createOrderInTx(
		ctx,
		CreateOrderRequest{
			UserID:      userEntity.ID,
			PaymentType: payment.TypeAlipay,
			OrderType:   payment.OrderTypeBalance,
			ClientIP:    "127.0.0.1",
			SrcHost:     "app.example.com",
		},
		user,
		nil,
		&PaymentConfig{MaxPendingOrders: 3, OrderTimeoutMin: 30},
		125,
		100,
		100,
		1.25,
		0,
		100,
		nil,
		snapshot,
	)
	require.NoError(t, err)
	require.Equal(t, PromoAttributionAttributed, order.SignupPromoAttribution)
	require.Equal(t, promo.ID, *order.SignupPromoCodeID)
	require.Equal(t, "WELCOME25", *order.SignupPromoCode)

	user.FirstRechargeBonusUsed = true
	usedSnapshot := svc.resolveUserPromoSnapshot(ctx, user)
	require.Equal(t, PromoAttributionAttributed, usedSnapshot.Attribution)
	require.Equal(t, 1.0, usedSnapshot.RechargeBonusMultiplier)
}

func TestResolveUserPromoSnapshotWithoutPromo(t *testing.T) {
	svc := &PaymentService{}
	snapshot := svc.resolveUserPromoSnapshot(context.Background(), &User{})
	require.Equal(t, PromoAttributionNone, snapshot.Attribution)
	require.Nil(t, snapshot.PromoCodeID)
	require.Nil(t, snapshot.PromoCode)
	require.Equal(t, 1.0, snapshot.RechargeBonusMultiplier)
}

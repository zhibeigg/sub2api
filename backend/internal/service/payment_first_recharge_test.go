package service

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

var firstRechargeTestNonce atomic.Int64

func TestPrepareBalanceFulfillmentOrderAppliesPromoBonusOnlyOnce(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	userEntity, err := client.User.Create().
		SetEmail("first-recharge-once@example.com").
		SetPasswordHash("hash").
		SetUsername("first-recharge-once").
		Save(ctx)
	require.NoError(t, err)

	first := createFirstRechargeCandidateOrder(t, ctx, client, userEntity, 120, 100, 1.2)
	second := createFirstRechargeCandidateOrder(t, ctx, client, userEntity, 240, 200, 1.2)
	svc := &PaymentService{entClient: client}

	first, err = svc.prepareBalanceFulfillmentOrder(ctx, first, &paymentFulfillmentLease{version: first.UpdatedAt})
	require.NoError(t, err)
	require.True(t, first.FirstRechargeBonusApplied)
	require.InDelta(t, 120, first.Amount, 1e-12)

	second, err = svc.prepareBalanceFulfillmentOrder(ctx, second, &paymentFulfillmentLease{version: second.UpdatedAt})
	require.NoError(t, err)
	require.False(t, second.FirstRechargeBonusApplied)
	require.InDelta(t, 200, second.Amount, 1e-12)

	reloadedUser, err := client.User.Get(ctx, userEntity.ID)
	require.NoError(t, err)
	require.True(t, reloadedUser.FirstRechargeBonusUsed)
}

func TestPrepareBalanceFulfillmentOrderBaseRechargeConsumesEligibility(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	userEntity, err := client.User.Create().
		SetEmail("first-recharge-base@example.com").
		SetPasswordHash("hash").
		SetUsername("first-recharge-base").
		Save(ctx)
	require.NoError(t, err)

	baseOrder := createFirstRechargeCandidateOrder(t, ctx, client, userEntity, 100, 100, 1)
	bonusOrder := createFirstRechargeCandidateOrder(t, ctx, client, userEntity, 120, 100, 1.2)
	svc := &PaymentService{entClient: client}

	baseOrder, err = svc.prepareBalanceFulfillmentOrder(ctx, baseOrder, &paymentFulfillmentLease{version: baseOrder.UpdatedAt})
	require.NoError(t, err)
	require.False(t, baseOrder.FirstRechargeBonusApplied)
	require.InDelta(t, 100, baseOrder.Amount, 1e-12)

	bonusOrder, err = svc.prepareBalanceFulfillmentOrder(ctx, bonusOrder, &paymentFulfillmentLease{version: bonusOrder.UpdatedAt})
	require.NoError(t, err)
	require.False(t, bonusOrder.FirstRechargeBonusApplied)
	require.InDelta(t, 100, bonusOrder.Amount, 1e-12)
}

func TestPrepareBalanceFulfillmentOrderPreservesLegacyOrderAmount(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	userEntity, err := client.User.Create().
		SetEmail("first-recharge-legacy@example.com").
		SetPasswordHash("hash").
		SetUsername("first-recharge-legacy").
		Save(ctx)
	require.NoError(t, err)

	legacyOrder := createFirstRechargeCandidateOrder(t, ctx, client, userEntity, 120, 0, 1)
	svc := &PaymentService{entClient: client}

	legacyOrder, err = svc.prepareBalanceFulfillmentOrder(ctx, legacyOrder, &paymentFulfillmentLease{version: legacyOrder.UpdatedAt})
	require.NoError(t, err)
	require.InDelta(t, 120, legacyOrder.Amount, 1e-12)

	reloadedUser, err := client.User.Get(ctx, userEntity.ID)
	require.NoError(t, err)
	require.True(t, reloadedUser.FirstRechargeBonusUsed)
}

func createFirstRechargeCandidateOrder(
	t *testing.T,
	ctx context.Context,
	client *dbent.Client,
	userEntity *dbent.User,
	amount float64,
	baseAmount float64,
	bonusMultiplier float64,
) *dbent.PaymentOrder {
	t.Helper()
	nonce := strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + strconv.FormatInt(firstRechargeTestNonce.Add(1), 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(userEntity.ID).
		SetUserEmail(userEntity.Email).
		SetUserName(userEntity.Username).
		SetAmount(amount).
		SetPayAmount(baseAmount).
		SetFeeRate(0).
		SetRechargeBaseAmount(baseAmount).
		SetRechargeBonusMultiplier(bonusMultiplier).
		SetRechargeCode("PAY-FIRST-" + nonce).
		SetOutTradeNo("sub2_first_recharge_" + nonce).
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-first-" + nonce).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusRecharging).
		SetPaidAt(time.Now().Add(-time.Minute)).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)
	return order
}

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type groupPredictedBalanceReaderStub struct {
	mu        sync.Mutex
	summaries map[int64]*service.GroupPredictedBalanceSummary
	errors    map[int64]error
	calls     map[int64]int
}

func (s *groupPredictedBalanceReaderStub) EstimateGroupPredictedBalance(_ context.Context, groupID int64) (*service.GroupPredictedBalanceSummary, error) {
	s.mu.Lock()
	if s.calls == nil {
		s.calls = make(map[int64]int)
	}
	s.calls[groupID]++
	summary := s.summaries[groupID]
	err := s.errors[groupID]
	s.mu.Unlock()
	return summary, err
}

func TestParsePredictedCapacityGroupIDs(t *testing.T) {
	groupIDs, err := parsePredictedCapacityGroupIDs(" 3,1,3,2 ")
	require.NoError(t, err)
	require.Equal(t, []int64{3, 1, 2}, groupIDs)

	for _, raw := range []string{"", "0", "-1", "1,nope", "1,,2"} {
		_, err := parsePredictedCapacityGroupIDs(raw)
		require.Error(t, err, raw)
	}

	parts := make([]string, 0, maxPredictedCapacityGroupIDs+1)
	for i := 1; i <= maxPredictedCapacityGroupIDs+1; i++ {
		parts = append(parts, strconv.Itoa(i))
	}
	_, err = parsePredictedCapacityGroupIDs(strings.Join(parts, ","))
	require.Error(t, err)
}

func TestGroupHandlerGetPredictedCapacitySummaryReturnsPartialRows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	remaining := 12.5
	requests := int64(42)
	predictedQuantity := "42"
	evaluatedAt := time.Date(2026, time.July, 22, 15, 0, 0, 0, time.UTC)
	reader := &groupPredictedBalanceReaderStub{
		summaries: map[int64]*service.GroupPredictedBalanceSummary{
			2: {
				PredictionMode:                service.PredictedCapacityModeHistoricalRequests,
				PredictionUnit:                "request",
				PredictionConfigured:          true,
				PredictionComplete:            false,
				PredictedQuantity:             &predictedQuantity,
				KnownPredictionAccountCount:   1,
				UnknownPredictionAccountCount: 1,
				Complete:                      true,
				RemainingBalanceUSD:           &remaining,
				KnownRemainingBalanceUSD:      &remaining,
				PoolAuthoritativeBalanceUSD:   10,
				NormalEstimatedBalanceUSD:     2.5,
				KnownBalanceAccountCount:      2,
				RequestsComplete:              false,
				EstimatedRemainingRequests:    &requests,
				KnownRequestAccountCount:      1,
				UnknownRequestAccountCount:    1,
				UnknownAccountCount:           0,
				StaleAccountCount:             1,
				IncompatibleUnitAccountCount:  0,
				EvaluatedAt:                   evaluatedAt,
			},
		},
		errors: map[int64]error{3: errors.New("upstream unavailable")},
	}
	handler := NewGroupHandler(nil, nil, nil, reader)
	router := gin.New()
	router.GET("/api/v1/admin/groups/predicted-capacity-summary", handler.GetPredictedCapacitySummary)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/groups/predicted-capacity-summary?ids=2,2,3", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var envelope struct {
		Data []groupPredictedCapacitySummaryResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data, 2)
	require.Equal(t, int64(2), envelope.Data[0].GroupID)
	require.True(t, envelope.Data[0].Available)
	require.Equal(t, service.PredictedCapacityModeHistoricalRequests, envelope.Data[0].PredictionMode)
	require.Equal(t, "request", envelope.Data[0].PredictionUnit)
	require.True(t, envelope.Data[0].PredictionConfigured)
	require.False(t, envelope.Data[0].PredictionComplete)
	require.False(t, envelope.Data[0].PredictionUnlimited)
	require.NotNil(t, envelope.Data[0].PredictedQuantity)
	require.Equal(t, "42", *envelope.Data[0].PredictedQuantity)
	require.Equal(t, 1, envelope.Data[0].KnownPredictionAccountCount)
	require.Equal(t, 1, envelope.Data[0].UnknownPredictionAccountCount)
	require.True(t, envelope.Data[0].BalanceComplete)
	require.NotNil(t, envelope.Data[0].RemainingBalanceUSD)
	require.InDelta(t, 12.5, *envelope.Data[0].RemainingBalanceUSD, 1e-12)
	require.NotNil(t, envelope.Data[0].KnownRemainingBalanceUSD)
	require.InDelta(t, 12.5, *envelope.Data[0].KnownRemainingBalanceUSD, 1e-12)
	require.False(t, envelope.Data[0].RequestsComplete)
	require.NotNil(t, envelope.Data[0].EstimatedRemainingRequests)
	require.Equal(t, int64(42), *envelope.Data[0].EstimatedRemainingRequests)
	require.Equal(t, evaluatedAt, *envelope.Data[0].EvaluatedAt)
	require.Equal(t, int64(3), envelope.Data[1].GroupID)
	require.False(t, envelope.Data[1].Available)

	var rawEnvelope struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &rawEnvelope))
	require.Equal(t, `"42"`, string(rawEnvelope.Data[0]["predicted_quantity"]), "new generic quantity remains a decimal string")
	require.Equal(t, "42", string(rawEnvelope.Data[0]["estimated_remaining_requests"]), "legacy request quantity remains a JSON number")
	for _, field := range []string{"remaining_balance_usd", "known_remaining_balance_usd", "estimated_remaining_requests", "evaluated_at"} {
		require.Equal(t, "null", string(rawEnvelope.Data[1][field]), field)
	}

	require.Equal(t, 1, reader.calls[2])
	require.Equal(t, 1, reader.calls[3])
}

func TestGroupHandlerGetPredictedCapacitySummaryValidatesInputAndDependency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewGroupHandler(nil, nil, nil, nil)
	router := gin.New()
	router.GET("/api/v1/admin/groups/predicted-capacity-summary", handler.GetPredictedCapacitySummary)

	for _, path := range []string{
		"/api/v1/admin/groups/predicted-capacity-summary",
		"/api/v1/admin/groups/predicted-capacity-summary?group_ids=1",
		"/api/v1/admin/groups/predicted-capacity-summary?ids=invalid",
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusBadRequest, recorder.Code, path)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/groups/predicted-capacity-summary?ids=1", nil)
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
}

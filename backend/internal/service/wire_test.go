package service

import (
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/zeromicro/go-zero/core/collection"
)

func TestProvideTimingWheelService_ReturnsError(t *testing.T) {
	original := newTimingWheel
	t.Cleanup(func() { newTimingWheel = original })

	newTimingWheel = func(_ time.Duration, _ int, _ collection.Execute) (*collection.TimingWheel, error) {
		return nil, errors.New("boom")
	}

	svc, err := ProvideTimingWheelService()
	if err == nil {
		t.Fatalf("期望返回 error，但得到 nil")
	}
	if svc != nil {
		t.Fatalf("期望返回 nil svc，但得到非空")
	}
}

func TestProvideTimingWheelService_Success(t *testing.T) {
	svc, err := ProvideTimingWheelService()
	if err != nil {
		t.Fatalf("期望 err 为 nil，但得到: %v", err)
	}
	if svc == nil {
		t.Fatalf("期望 svc 非空，但得到 nil")
	}
	svc.Stop()
}

func TestProvideAccountUsageServiceInjectsKiroUsageService(t *testing.T) {
	kiroUsageService := &KiroUsageService{}
	svc := ProvideAccountUsageService(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, kiroUsageService,
	)
	if svc.kiroUsageService != kiroUsageService {
		t.Fatal("KiroUsageService was not injected into AccountUsageService")
	}
}

func TestProvideAccountTestServiceInjectsKiroUsageService(t *testing.T) {
	kiroUsageService := &KiroUsageService{}
	svc := ProvideAccountTestService(
		nil, nil, nil, nil, nil, nil, &config.Config{}, nil, nil, nil, nil, kiroUsageService,
	)
	if svc.kiroUsageService != kiroUsageService {
		t.Fatal("KiroUsageService was not injected into AccountTestService")
	}
}

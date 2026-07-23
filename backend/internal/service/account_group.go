package service

import (
	"context"
	"time"
)

type AccountGroup struct {
	AccountID                    int64
	GroupID                      int64
	Priority                     int
	EndpointCompatibilityEnabled bool
	CreatedAt                    time.Time

	Account *Account
	Group   *Group
}

// AccountGroupBindingRepository is an optional backward-compatible extension
// for callers that need to persist per-binding endpoint compatibility policy.
// Legacy AccountRepository.BindGroups callers continue to create safe, disabled
// compatibility bindings.
type AccountGroupBindingRepository interface {
	BindAccountGroups(ctx context.Context, accountID int64, groups []AccountGroup) error
}

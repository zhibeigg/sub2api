package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// BuildCompatibleAccountGroupBindings validates provider-independent endpoint
// compatibility and builds the relation rows that must be persisted. A
// cross-provider relation is an explicit opt-in only after this validation
// succeeds; same-provider relations retain the legacy disabled flag.
func BuildCompatibleAccountGroupBindings(
	ctx context.Context,
	groupRepo GroupRepository,
	account *Account,
	groupIDs []int64,
) ([]AccountGroup, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	if groupRepo == nil {
		return nil, fmt.Errorf("group repository not configured")
	}
	if account == nil {
		return nil, fmt.Errorf("account is required")
	}

	existingBindings := make(map[int64]AccountGroup, len(account.AccountGroups))
	for i := range account.AccountGroups {
		existingBindings[account.AccountGroups[i].GroupID] = account.AccountGroups[i]
	}

	bindings := make([]AccountGroup, 0, len(groupIDs))
	for i, groupID := range groupIDs {
		group, err := groupRepo.GetByID(ctx, groupID)
		if err != nil {
			return nil, err
		}
		if group == nil {
			return nil, ErrGroupNotFound
		}

		groupPlatform := NormalizePlatform(group.Platform)
		crossProvider := groupPlatform != PlatformComposite && NormalizePlatform(account.Platform) != groupPlatform
		existing, existed := existingBindings[group.ID]
		compatibilityEnabled := crossProvider && (!existed || existing.EndpointCompatibilityEnabled)

		// Preserve already-bound, explicitly disabled cross-provider relations.
		// A normal account edit must not silently activate them merely because the
		// group_ids array was submitted again. Removing and later adding the group is
		// the explicit opt-in path and is validated below.
		if !crossProvider || compatibilityEnabled {
			if !accountSupportsAnyGroupEndpoint(ctx, account, group, true, compatibilityEnabled) {
				return nil, infraerrors.Newf(
					http.StatusBadRequest,
					"ACCOUNT_GROUP_ENDPOINT_MISMATCH",
					"account platform %q cannot serve group %d endpoint protocols %q",
					account.Platform,
					group.ID,
					strings.Join(GroupEndpointProtocols(group), ","),
				)
			}
		}

		bindings = append(bindings, AccountGroup{
			AccountID:                    account.ID,
			GroupID:                      group.ID,
			Priority:                     i + 1,
			EndpointCompatibilityEnabled: compatibilityEnabled,
			Group:                        group,
		})
	}
	return bindings, nil
}

func accountSupportsAnyGroupEndpoint(ctx context.Context, account *Account, group *Group, hasBinding, compatibilityEnabled bool) bool {
	protocols := GroupEndpointProtocols(group)
	if len(protocols) == 0 && group != nil && NormalizePlatform(group.Platform) == PlatformComposite {
		protocols = compositeDefaultEndpointProtocols()
	}
	if len(protocols) == 0 {
		return false
	}

	models := []string{""}
	if group.ModelsListConfig.Enabled && len(group.ModelsListConfig.Models) > 0 {
		models = group.ModelsListConfig.Models
	}

	for _, rawProtocol := range protocols {
		protocol := NormalizeEndpointProtocol(EndpointProtocol(rawProtocol))
		if !IsValidEndpointProtocol(protocol) {
			continue
		}
		for _, model := range models {
			if IsAccountCompatibleForRequest(account, RequestDescriptor{
				Protocol: protocol,
				Model:    strings.TrimSpace(model),
			}, AccountGroupCompatibilityOptions{
				Context:                      ctx,
				Group:                        group,
				HasAccountGroupBinding:       hasBinding,
				EndpointCompatibilityEnabled: compatibilityEnabled,
				AllowMixedScheduling:         account.IsMixedSchedulingEnabled(),
				RequireOAuthOnly:             group.RequireOAuthOnly,
				// Privacy setup for newly created OAuth accounts is asynchronous.
				// Runtime selection still enforces require_privacy_set.
				RequirePrivacySet:        false,
				SkipSchedulabilityChecks: true,
			}) {
				return true
			}
		}
	}
	return false
}

// BindCompatibleAccountGroups replaces account bindings using the relation-aware
// repository extension. Falling back to the legacy method is safe only when no
// cross-provider compatibility flag is required.
func BindCompatibleAccountGroups(
	ctx context.Context,
	accountRepo AccountRepository,
	account *Account,
	groupRepo GroupRepository,
	groupIDs []int64,
) error {
	bindings, err := BuildCompatibleAccountGroupBindings(ctx, groupRepo, account, groupIDs)
	if err != nil {
		return err
	}
	if binder, ok := accountRepo.(AccountGroupBindingRepository); ok {
		return binder.BindAccountGroups(ctx, account.ID, bindings)
	}
	for _, binding := range bindings {
		if binding.EndpointCompatibilityEnabled {
			return infraerrors.New(
				http.StatusServiceUnavailable,
				"ACCOUNT_GROUP_COMPATIBILITY_UNAVAILABLE",
				"account repository does not support endpoint-compatible group bindings",
			)
		}
	}
	return accountRepo.BindGroups(ctx, account.ID, groupIDs)
}

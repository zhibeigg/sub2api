import { describe, expect, it } from 'vitest'
import type { UserSubscription } from '@/types'
import { getEffectiveSubscriptionQuotaLimit } from '@/utils/subscriptionQuota'

function subscription(overrides: Partial<UserSubscription> = {}): UserSubscription {
  return {
    id: 1,
    user_id: 64,
    group_id: 11,
    group_ids: [11],
    groups: [],
    source_plan_id: 2,
    quota_snapshotted: false,
    status: 'active',
    starts_at: '2026-07-14T00:00:00Z',
    expires_at: '2026-07-21T00:00:00Z',
    daily_usage_usd: 1,
    weekly_usage_usd: 1,
    monthly_usage_usd: 1,
    daily_window_start: null,
    weekly_window_start: null,
    monthly_window_start: null,
    created_at: '2026-07-14T00:00:00Z',
    updated_at: '2026-07-14T00:00:00Z',
    ...overrides
  }
}

describe('getEffectiveSubscriptionQuotaLimit', () => {
  it('falls back to the group limit for legacy non-snapshotted subscriptions', () => {
    const value = getEffectiveSubscriptionQuotaLimit(
      subscription({
        daily_limit_usd: null,
        group: { daily_limit_usd: 60 } as UserSubscription['group']
      }),
      'daily'
    )

    expect(value).toBe(60)
  })

  it('uses the subscription snapshot even when it explicitly has no limit', () => {
    const value = getEffectiveSubscriptionQuotaLimit(
      subscription({
        quota_snapshotted: true,
        daily_limit_usd: null,
        group: { daily_limit_usd: 60 } as UserSubscription['group']
      }),
      'daily'
    )

    expect(value).toBeNull()
  })

  it('uses a finite snapshotted subscription limit instead of the group limit', () => {
    const value = getEffectiveSubscriptionQuotaLimit(
      subscription({
        quota_snapshotted: true,
        daily_limit_usd: 30,
        group: { daily_limit_usd: 60 } as UserSubscription['group']
      }),
      'daily'
    )

    expect(value).toBe(30)
  })
})

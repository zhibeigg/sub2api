import { describe, expect, it } from 'vitest'
import type { UserSubscription } from '@/types'
import {
  getEffectiveSubscriptionQuotaLimit,
  getSubscriptionQuotaResetAt
} from '@/utils/subscriptionQuota'

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

describe('getSubscriptionQuotaResetAt', () => {
  it('keeps daily, weekly, and monthly reset times anchored to the purchase time', () => {
    const value = subscription({
      starts_at: '2026-07-01T10:30:00Z',
      expires_at: '2026-08-30T10:30:00Z',
      daily_window_start: '2026-07-22T00:00:00Z',
      weekly_window_start: '2026-07-20T00:00:00Z',
      monthly_window_start: '2026-07-01T00:00:00Z'
    })
    const now = new Date('2026-07-22T12:00:00Z')

    expect(getSubscriptionQuotaResetAt(value, 'daily', now)?.toISOString()).toBe('2026-07-23T10:30:00.000Z')
    expect(getSubscriptionQuotaResetAt(value, 'weekly', now)?.toISOString()).toBe('2026-07-29T10:30:00.000Z')
    expect(getSubscriptionQuotaResetAt(value, 'monthly', now)?.toISOString()).toBe('2026-07-31T10:30:00.000Z')
  })

  it('returns the next anchored period when called exactly on a boundary', () => {
    const value = subscription({
      starts_at: '2026-07-01T10:30:00Z',
      expires_at: '2026-08-30T10:30:00Z'
    })

    expect(
      getSubscriptionQuotaResetAt(value, 'daily', new Date('2026-07-22T10:30:00Z'))?.toISOString()
    ).toBe('2026-07-23T10:30:00.000Z')
  })

  it('uses subscription expiry as the end of a one-day daily quota', () => {
    const value = subscription({
      starts_at: '2026-07-22T10:30:00Z',
      expires_at: '2026-07-23T10:30:00Z'
    })

    expect(
      getSubscriptionQuotaResetAt(value, 'daily', new Date('2026-07-22T12:00:00Z'))?.toISOString()
    ).toBe('2026-07-23T10:30:00.000Z')
  })
})

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

  it('falls back to the matching group from groups when the legacy group field is omitted', () => {
    const value = getEffectiveSubscriptionQuotaLimit(
      subscription({
        daily_limit_usd: null,
        group: undefined,
        groups: [
          { id: 12, daily_limit_usd: 90 },
          { id: 11, daily_limit_usd: 180 }
        ] as UserSubscription['groups']
      }),
      'daily'
    )

    expect(value).toBe(180)
  })

  it('uses the first group as a compatibility fallback when the primary group id is absent', () => {
    const value = getEffectiveSubscriptionQuotaLimit(
      subscription({
        daily_limit_usd: null,
        group: undefined,
        groups: [{ id: 12, weekly_limit_usd: 360 }] as UserSubscription['groups']
      }),
      'weekly'
    )

    expect(value).toBe(360)
  })

  it('uses the subscription snapshot even when it explicitly has no limit', () => {
    const value = getEffectiveSubscriptionQuotaLimit(
      subscription({
        quota_snapshotted: true,
        daily_limit_usd: null,
        group: undefined,
        groups: [{ id: 11, daily_limit_usd: 60 }] as UserSubscription['groups']
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

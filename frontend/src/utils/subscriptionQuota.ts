import type { UserSubscription } from '@/types'

const ONE_DAY_MS = 24 * 60 * 60 * 1000

export interface RemainingDurationParts {
  days: number
  hours: number
  minutes: number
}

export type SubscriptionQuotaPeriod = 'daily' | 'weekly' | 'monthly'

const QUOTA_PERIOD_MS: Record<SubscriptionQuotaPeriod, number> = {
  daily: ONE_DAY_MS,
  weekly: 7 * ONE_DAY_MS,
  monthly: 30 * ONE_DAY_MS
}

export function isOneTimeDailyQuota(
  subscription: Pick<UserSubscription, 'starts_at' | 'expires_at'>
): boolean {
  if (!subscription.starts_at || !subscription.expires_at) return false

  const startsAt = new Date(subscription.starts_at).getTime()
  const expiresAt = new Date(subscription.expires_at).getTime()

  if (!Number.isFinite(startsAt) || !Number.isFinite(expiresAt)) return false

  return expiresAt <= startsAt + ONE_DAY_MS
}

export function getSubscriptionQuotaResetAt(
  subscription: UserSubscription,
  period: SubscriptionQuotaPeriod,
  now: Date = new Date()
): Date | null {
  if (period === 'daily' && subscription.expires_at && isOneTimeDailyQuota(subscription)) {
    const expiresAt = new Date(subscription.expires_at)
    return Number.isFinite(expiresAt.getTime()) ? expiresAt : null
  }

  const periodMs = QUOTA_PERIOD_MS[period]
  const anchorMs = new Date(subscription.starts_at).getTime()
  const nowMs = now.getTime()
  if (Number.isFinite(anchorMs) && Number.isFinite(nowMs)) {
    const elapsedMs = Math.max(0, nowMs - anchorMs)
    const completedWindows = Math.floor(elapsedMs / periodMs)
    return new Date(anchorMs + (completedWindows + 1) * periodMs)
  }

  const windowStartKey = `${period}_window_start` as
    | 'daily_window_start'
    | 'weekly_window_start'
    | 'monthly_window_start'
  const windowStart = subscription[windowStartKey]
  if (!windowStart) return null
  const windowStartMs = new Date(windowStart).getTime()
  return Number.isFinite(windowStartMs) ? new Date(windowStartMs + periodMs) : null
}

export function getRemainingDurationParts(
  targetAt: Date | string,
  now: Date = new Date()
): RemainingDurationParts | null {
  const targetTime = targetAt instanceof Date ? targetAt.getTime() : new Date(targetAt).getTime()
  const nowTime = now.getTime()

  if (!Number.isFinite(targetTime) || !Number.isFinite(nowTime)) return null

  const diffMs = targetTime - nowTime
  if (diffMs <= 0) return null

  const totalMinutes = Math.floor(diffMs / (1000 * 60))
  const days = Math.floor(totalMinutes / (24 * 60))
  const hours = Math.floor((totalMinutes % (24 * 60)) / 60)
  const minutes = totalMinutes % 60

  return { days, hours, minutes }
}

export function getEffectiveSubscriptionQuotaLimit(
  subscription: UserSubscription,
  period: SubscriptionQuotaPeriod
): number | null {
  const key = `${period}_limit_usd` as
    | 'daily_limit_usd'
    | 'weekly_limit_usd'
    | 'monthly_limit_usd'

  if (subscription.quota_snapshotted) {
    return subscription[key] ?? null
  }

  const quotaGroup =
    subscription.group ??
    subscription.groups?.find((group) => group.id === subscription.group_id) ??
    subscription.groups?.[0]

  return quotaGroup?.[key] ?? null
}

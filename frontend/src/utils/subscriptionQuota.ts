import type { UserSubscription } from '@/types'

const ONE_DAY_MS = 24 * 60 * 60 * 1000

export interface RemainingDurationParts {
  days: number
  hours: number
  minutes: number
}

export type SubscriptionQuotaPeriod = 'daily' | 'weekly' | 'monthly'

export function isOneTimeDailyQuota(
  subscription: Pick<UserSubscription, 'starts_at' | 'expires_at'>
): boolean {
  if (!subscription.starts_at || !subscription.expires_at) return false

  const startsAt = new Date(subscription.starts_at).getTime()
  const expiresAt = new Date(subscription.expires_at).getTime()

  if (!Number.isFinite(startsAt) || !Number.isFinite(expiresAt)) return false

  return expiresAt <= startsAt + ONE_DAY_MS
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

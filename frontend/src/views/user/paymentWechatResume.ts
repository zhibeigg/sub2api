import type { LocationQuery, LocationQueryRaw } from 'vue-router'
import type { SubscriptionPlan } from '@/types/payment'
import { normalizeVisibleMethod } from '@/components/payment/paymentFlow'

export const WECHAT_PAYMENT_RESUME_HANDOFF_KEY = 'payment.wechat.resume.handoff'
const WECHAT_PAYMENT_RESUME_HANDOFF_MAX_AGE_MS = 10 * 60 * 1000

export interface ParsedWechatResumeRoute {
  orderAmount: number
  orderType: 'balance' | 'subscription'
  paymentType: string
  planId?: number
  openid?: string
  wechatResumeToken?: string
}

export interface WechatPaymentResumeHandoff {
  wechat_resume_token?: string
  openid?: string
  state?: string
  scope?: string
  payment_type?: string
  amount?: string
  order_type?: string
  plan_id?: string
  created_at: number
}

function readQueryString(query: LocationQuery, key: string): string {
  const value = query[key]
  if (Array.isArray(value)) {
    return typeof value[0] === 'string' ? value[0] : ''
  }
  return typeof value === 'string' ? value : ''
}

function readResumeString(
  query: LocationQuery,
  handoff: WechatPaymentResumeHandoff | null | undefined,
  key: keyof Omit<WechatPaymentResumeHandoff, 'created_at'>,
): string {
  const handoffValue = handoff?.[key]
  return typeof handoffValue === 'string' && handoffValue !== ''
    ? handoffValue
    : readQueryString(query, key)
}

export function writeWechatPaymentResumeHandoff(
  storage: Pick<Storage, 'setItem'>,
  handoff: Omit<WechatPaymentResumeHandoff, 'created_at'>,
  now = Date.now(),
): void {
  storage.setItem(WECHAT_PAYMENT_RESUME_HANDOFF_KEY, JSON.stringify({
    ...handoff,
    created_at: now,
  }))
}

export function consumeWechatPaymentResumeHandoff(
  storage: Pick<Storage, 'getItem' | 'removeItem'>,
  now = Date.now(),
): WechatPaymentResumeHandoff | null {
  const raw = storage.getItem(WECHAT_PAYMENT_RESUME_HANDOFF_KEY)
  storage.removeItem(WECHAT_PAYMENT_RESUME_HANDOFF_KEY)
  if (!raw) return null

  try {
    const parsed = JSON.parse(raw) as Partial<WechatPaymentResumeHandoff>
    if (
      typeof parsed.created_at !== 'number'
      || parsed.created_at <= 0
      || now - parsed.created_at > WECHAT_PAYMENT_RESUME_HANDOFF_MAX_AGE_MS
      || now < parsed.created_at
    ) {
      return null
    }

    const handoff: WechatPaymentResumeHandoff = { created_at: parsed.created_at }
    const keys: (keyof Omit<WechatPaymentResumeHandoff, 'created_at'>)[] = [
      'wechat_resume_token',
      'openid',
      'state',
      'scope',
      'payment_type',
      'amount',
      'order_type',
      'plan_id',
    ]
    for (const key of keys) {
      const value = parsed[key]
      if (typeof value === 'string' && value !== '') {
        handoff[key] = value
      }
    }
    return handoff
  } catch {
    return null
  }
}

export function hasWechatResumeQuery(query: LocationQuery): boolean {
  if (readQueryString(query, 'wechat_resume') === '1') {
    return true
  }
  return readQueryString(query, 'wechat_resume_token') !== ''
    || readQueryString(query, 'openid') !== ''
}

export function parseWechatResumeRoute(
  query: LocationQuery,
  plans: SubscriptionPlan[],
  fallbackBalanceAmount: number,
  handoff?: WechatPaymentResumeHandoff | null,
): ParsedWechatResumeRoute | null {
  if (!hasWechatResumeQuery(query) && !handoff) {
    return null
  }

  const wechatResumeToken = readResumeString(query, handoff, 'wechat_resume_token')
  const paymentType = normalizeVisibleMethod(readResumeString(query, handoff, 'payment_type')) || 'wxpay'
  const planId = Number.parseInt(readResumeString(query, handoff, 'plan_id'), 10)
  const hasPlanId = Number.isFinite(planId) && planId > 0
  const orderType = readResumeString(query, handoff, 'order_type') === 'subscription' || hasPlanId
    ? 'subscription'
    : 'balance'

  if (wechatResumeToken) {
    return {
      wechatResumeToken,
      paymentType,
      orderType,
      orderAmount: 0,
      planId: hasPlanId ? planId : undefined,
    }
  }

  const openid = readResumeString(query, handoff, 'openid')
  if (!openid) {
    return null
  }

  const rawAmount = Number.parseFloat(readResumeString(query, handoff, 'amount'))
  const orderAmount = Number.isFinite(rawAmount) && rawAmount > 0
    ? rawAmount
    : (orderType === 'subscription'
      ? (plans.find(plan => plan.id === planId)?.price ?? 0)
      : fallbackBalanceAmount)

  return {
    openid,
    paymentType,
    orderType,
    orderAmount,
    planId: hasPlanId ? planId : undefined,
  }
}

export function stripWechatResumeQuery(query: LocationQuery): LocationQueryRaw {
  const nextQuery: LocationQueryRaw = { ...query }
  delete nextQuery.wechat_resume
  delete nextQuery.wechat_resume_token
  delete nextQuery.openid
  delete nextQuery.state
  delete nextQuery.scope
  delete nextQuery.payment_type
  delete nextQuery.amount
  delete nextQuery.order_type
  delete nextQuery.plan_id
  return nextQuery
}

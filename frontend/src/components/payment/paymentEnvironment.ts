import type { OrderType } from '@/types/payment'

export type WechatClientEnvironment = 'wecom' | 'wechat' | 'other'

export interface WechatOAuthResumeContext {
  paymentType: string
  orderType: OrderType
  planId?: number
  orderAmount: number
}

const OAUTH_PAYMENT_TYPE_ALIASES: Record<string, string> = {
  alipay_direct: 'alipay',
  wxpay_direct: 'wxpay',
}

export function classifyWechatClient(userAgent: string): WechatClientEnvironment {
  const normalized = userAgent.toLowerCase()
  if (normalized.includes('wxwork')) return 'wecom'
  if (normalized.includes('micromessenger')) return 'wechat'
  return 'other'
}

export function isWechatInAppEnvironment(environment: WechatClientEnvironment): boolean {
  return environment === 'wecom' || environment === 'wechat'
}

export function stripUrlFragment(rawUrl: string): string {
  const fragmentIndex = rawUrl.indexOf('#')
  return (fragmentIndex >= 0 ? rawUrl.slice(0, fragmentIndex) : rawUrl).trim()
}

function normalizeOAuthPaymentType(paymentType: string): string {
  const normalized = paymentType.trim().toLowerCase()
  return OAUTH_PAYMENT_TYPE_ALIASES[normalized] || normalized || 'wxpay'
}

function isRelativeURL(rawUrl: string): boolean {
  return !/^[a-z][a-z\d+.-]*:/i.test(rawUrl)
}

function serializeSafeURL(target: URL, relative: boolean): string {
  if (relative) {
    return `${target.pathname}${target.search}${target.hash}`
  }
  return target.toString()
}

function safeRedirectURL(rawRedirect: string, origin: string): URL {
  const fallback = new URL('/purchase', origin)
  const normalized = rawRedirect.trim()
  if (!normalized.startsWith('/') || normalized.startsWith('//') || normalized.includes('://')) {
    return fallback
  }

  try {
    const target = new URL(normalized, origin)
    return target.origin === fallback.origin ? target : fallback
  } catch {
    return fallback
  }
}

export function buildWechatOAuthAuthorizeUrl(
  authorizeUrl: string,
  context: WechatOAuthResumeContext,
  origin: string,
): string {
  const normalizedUrl = authorizeUrl.trim()
  if (!normalizedUrl || !origin) return ''
  if (normalizedUrl.startsWith('//')) return ''

  try {
    const base = new URL(origin)
    const relative = isRelativeURL(normalizedUrl)
    const targetUrl = new URL(normalizedUrl, base)
    if (targetUrl.protocol !== 'http:' && targetUrl.protocol !== 'https:') return ''
    if (targetUrl.origin !== base.origin) return ''

    // New OAuth URLs carry an opaque server-side context token. Do not copy
    // legacy order data into that URL or otherwise rewrite the token-bearing URL.
    if (targetUrl.searchParams.has('context_token')) {
      return normalizedUrl
    }

    const redirectUrl = safeRedirectURL(targetUrl.searchParams.get('redirect') || '/purchase', base.origin)
    redirectUrl.hash = ''
    redirectUrl.searchParams.set('payment_type', normalizeOAuthPaymentType(context.paymentType))
    redirectUrl.searchParams.set('order_type', context.orderType)

    if (context.planId && context.planId > 0) {
      redirectUrl.searchParams.set('plan_id', String(context.planId))
    } else {
      redirectUrl.searchParams.delete('plan_id')
    }

    if (Number.isFinite(context.orderAmount) && context.orderAmount > 0) {
      redirectUrl.searchParams.set('amount', String(context.orderAmount))
    } else {
      redirectUrl.searchParams.delete('amount')
    }

    targetUrl.searchParams.set('redirect', `${redirectUrl.pathname}${redirectUrl.search}`)
    return serializeSafeURL(targetUrl, relative)
  } catch {
    return ''
  }
}

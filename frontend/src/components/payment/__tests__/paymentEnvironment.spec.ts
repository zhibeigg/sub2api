import { describe, expect, it } from 'vitest'
import {
  buildWechatOAuthAuthorizeUrl,
  classifyWechatClient,
  stripUrlFragment,
} from '../paymentEnvironment'

describe('classifyWechatClient', () => {
  it.each([
    ['Mozilla/5.0 wxwork/4.1.36 MicroMessenger/7.0.1', 'wecom'],
    ['Mozilla/5.0 MicroMessenger/8.0.50 Mobile', 'wechat'],
    ['Mozilla/5.0 Chrome/126.0 Safari/537.36', 'other'],
    ['', 'other'],
  ] as const)('classifies %s as %s', (userAgent, expected) => {
    expect(classifyWechatClient(userAgent)).toBe(expected)
  })
})

describe('stripUrlFragment', () => {
  it('keeps the real page query while removing the fragment', () => {
    expect(stripUrlFragment('https://app.example.com/purchase?tab=balance#payment-panel')).toBe(
      'https://app.example.com/purchase?tab=balance',
    )
  })
})

describe('buildWechatOAuthAuthorizeUrl', () => {
  const context = {
    paymentType: 'wxpay_direct',
    orderType: 'subscription' as const,
    planId: 7,
    orderAmount: 128,
  }

  it('returns a context-token URL unchanged without appending legacy order data', () => {
    const authorizeUrl = '/api/v1/auth/oauth/wechat/payment/start?context_token=opaque-context&redirect=%2Fpurchase%3Ffrom%3Dwecom'

    expect(buildWechatOAuthAuthorizeUrl(authorizeUrl, context, 'https://app.example.com')).toBe(authorizeUrl)
  })

  it('keeps legacy URLs compatible by appending resume context to the safe redirect path', () => {
    const result = buildWechatOAuthAuthorizeUrl(
      '/api/v1/auth/oauth/wechat/payment/start?redirect=%2Fpurchase%3Ffrom%3Dwechat',
      context,
      'https://app.example.com',
    )
    const parsed = new URL(result, 'https://app.example.com')

    expect(parsed.searchParams.get('redirect')).toBe(
      '/purchase?from=wechat&payment_type=wxpay&order_type=subscription&plan_id=7&amount=128',
    )
  })

  it('accepts only same-origin OAuth URLs, including context-token URLs', () => {
    const sameOrigin = 'https://app.example.com/api/v1/auth/oauth/wechat/payment/start?context_token=opaque-context'

    expect(buildWechatOAuthAuthorizeUrl(sameOrigin, context, 'https://app.example.com')).toBe(sameOrigin)
    expect(buildWechatOAuthAuthorizeUrl(
      'https://evil.example/api/v1/auth/oauth/wechat/payment/start?context_token=opaque-context',
      context,
      'https://app.example.com',
    )).toBe('')
    expect(buildWechatOAuthAuthorizeUrl('//evil.example/oauth', context, 'https://app.example.com')).toBe('')
    expect(buildWechatOAuthAuthorizeUrl('javascript:alert(1)', context, 'https://app.example.com')).toBe('')
  })
})

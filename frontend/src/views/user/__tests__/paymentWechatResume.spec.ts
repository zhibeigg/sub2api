import { describe, expect, it } from 'vitest'
import {
  WECHAT_PAYMENT_RESUME_HANDOFF_KEY,
  consumeWechatPaymentResumeHandoff,
  parseWechatResumeRoute,
  stripWechatResumeQuery,
  writeWechatPaymentResumeHandoff,
} from '../paymentWechatResume'

describe('parseWechatResumeRoute', () => {
  it('prefers the opaque resume token over legacy openid query params', () => {
    expect(parseWechatResumeRoute({
      wechat_resume: '1',
      wechat_resume_token: 'resume-token-123',
      openid: 'openid-123',
      payment_type: 'wxpay',
      amount: '12.5',
      order_type: 'subscription',
      plan_id: '7',
    }, [], 88)).toEqual({
      wechatResumeToken: 'resume-token-123',
      paymentType: 'wxpay',
      orderType: 'subscription',
      orderAmount: 0,
      planId: 7,
    })
  })

  it('falls back to legacy openid-based resume when opaque token is absent', () => {
    expect(parseWechatResumeRoute({
      wechat_resume: '1',
      openid: 'openid-123',
      payment_type: 'wxpay',
      amount: '12.5',
      order_type: 'balance',
    }, [], 88)).toEqual({
      openid: 'openid-123',
      paymentType: 'wxpay',
      orderType: 'balance',
      orderAmount: 12.5,
      planId: undefined,
    })
  })

  it('consumes an opaque resume handoff without requiring the token in the route URL', () => {
    window.sessionStorage.clear()
    writeWechatPaymentResumeHandoff(window.sessionStorage, {
      wechat_resume_token: 'resume-token-private',
      payment_type: 'wxpay_direct',
      order_type: 'subscription',
      plan_id: '7',
    }, 1000)

    const handoff = consumeWechatPaymentResumeHandoff(window.sessionStorage, 1500)
    expect(parseWechatResumeRoute({ wechat_resume: '1' }, [], 0, handoff)).toEqual({
      wechatResumeToken: 'resume-token-private',
      paymentType: 'wxpay',
      orderType: 'subscription',
      orderAmount: 0,
      planId: 7,
    })
    expect(window.sessionStorage.getItem(WECHAT_PAYMENT_RESUME_HANDOFF_KEY)).toBeNull()
  })
})

describe('stripWechatResumeQuery', () => {
  it('removes both opaque-token and legacy resume params from the route query', () => {
    expect(stripWechatResumeQuery({
      foo: 'bar',
      wechat_resume: '1',
      wechat_resume_token: 'resume-token-123',
      openid: 'openid-123',
      payment_type: 'wxpay',
      amount: '12.5',
      order_type: 'subscription',
      plan_id: '7',
      state: 'state-123',
      scope: 'snsapi_base',
    })).toEqual({
      foo: 'bar',
    })
  })
})

import { describe, expect, it } from 'vitest'
import {
  buildPaymentErrorToastMessage,
  describePaymentScenarioError,
  normalizePaymentMethodForDisplay,
} from '../paymentUx'

describe('normalizePaymentMethodForDisplay', () => {
  it('collapses visible payment aliases to canonical method ids', () => {
    expect(normalizePaymentMethodForDisplay(' alipay_direct ')).toBe('alipay')
    expect(normalizePaymentMethodForDisplay('wxpay_direct')).toBe('wxpay')
    expect(normalizePaymentMethodForDisplay('wechat_pay')).toBe('wxpay')
  })

  it('leaves non-aliased methods untouched', () => {
    expect(normalizePaymentMethodForDisplay('stripe')).toBe('stripe')
  })
})

describe('describePaymentScenarioError', () => {
  it('maps WeChat H5 authorization errors to explicit in-app guidance', () => {
    expect(describePaymentScenarioError(
      { reason: 'WECHAT_H5_NOT_AUTHORIZED' },
      { paymentMethod: 'wxpay', isMobile: true, isWechatBrowser: false },
    )).toEqual({
      messageKey: 'payment.errors.wechatH5NotAuthorized',
      hintKey: 'payment.errors.wechatScanOnDesktopHint',
    })
  })

  it('maps WeChat H5 authorization errors when provider aliases use wxpay_direct', () => {
    expect(describePaymentScenarioError(
      { reason: 'WECHAT_H5_NOT_AUTHORIZED' },
      { paymentMethod: 'wxpay_direct', isMobile: true, isWechatBrowser: false },
    )).toEqual({
      messageKey: 'payment.errors.wechatH5NotAuthorized',
      hintKey: 'payment.errors.wechatScanOnDesktopHint',
    })
  })

  it('maps structured WeChat capability and API errors to actionable prompts', () => {
    const cases = [
      ['WECHAT_NATIVE_NOT_AUTHORIZED', 'payment.errors.wechatNativeNotAuthorized', 'payment.errors.wechatContactAdminHint'],
      ['WECHAT_JSAPI_NOT_AUTHORIZED', 'payment.errors.wechatJsapiNotAuthorized', 'payment.errors.wechatSwitchBrowserHint'],
      ['NO_AVAILABLE_WXPAY_CAPABILITY', 'payment.errors.wechatNoAvailableCapability', 'payment.errors.wechatOpenInWeChatHint'],
      ['WECHAT_APPID_MCHID_MISMATCH', 'payment.errors.wechatAppIdMchIdMismatch', 'payment.errors.wechatContactAdminHint'],
      ['WECHAT_SIGN_ERROR', 'payment.errors.wechatSignError', 'payment.errors.wechatContactAdminHint'],
      ['WECHAT_PAYMENT_API_ERROR', 'payment.errors.wechatApiError', 'payment.errors.wechatContactAdminHint'],
    ] as const

    for (const [reason, messageKey, hintKey] of cases) {
      expect(describePaymentScenarioError(
        { reason },
        { paymentMethod: 'wxpay', isMobile: true, isWechatBrowser: false },
      )).toEqual({ messageKey, hintKey })
    }
  })

  it('maps missing WeixinJSBridge to a JSAPI-specific prompt', () => {
    expect(describePaymentScenarioError(
      new Error('WeixinJSBridge is unavailable'),
      { paymentMethod: 'wxpay', isMobile: true, isWechatBrowser: true },
    )).toEqual({
      messageKey: 'payment.errors.wechatJsapiUnavailable',
      hintKey: 'payment.errors.wechatOpenInWeChatHint',
    })
  })

  it('maps the internal JSAPI unavailable marker to the same prompt', () => {
    expect(describePaymentScenarioError(
      new Error('WECHAT_JSAPI_UNAVAILABLE'),
      { paymentMethod: 'wxpay', isMobile: true, isWechatBrowser: true },
    )).toEqual({
      messageKey: 'payment.errors.wechatJsapiUnavailable',
      hintKey: 'payment.errors.wechatOpenInWeChatHint',
    })
  })

  it('maps ordinary WeChat and QQ gateway failures to the same non-retry guidance', () => {
    for (const paymentMethod of ['wxpay', 'qqpay']) {
      expect(describePaymentScenarioError(
        { reason: 'PAYMENT_GATEWAY_ERROR' },
        { paymentMethod, isMobile: true, isWechatBrowser: false },
      )).toEqual({
        messageKey: 'payment.errors.gatewayResponseInvalid',
      })
    }
  })

  it('maps generic desktop Alipay failures to QR guidance', () => {
    expect(describePaymentScenarioError(
      { reason: 'PAYMENT_GATEWAY_ERROR' },
      { paymentMethod: 'alipay', isMobile: false, isWechatBrowser: false },
    )).toEqual({
      messageKey: 'payment.errors.alipayDesktopUnavailable',
      hintKey: 'payment.errors.alipayDesktopQrHint',
    })
  })
})

describe('buildPaymentErrorToastMessage', () => {
  it('returns the main message when no hint is present', () => {
    expect(buildPaymentErrorToastMessage('Payment failed')).toBe('Payment failed')
  })

  it('appends the hint to the toast body when present', () => {
    expect(buildPaymentErrorToastMessage('Payment failed', 'Open WeChat to continue.')).toBe(
      'Payment failed Open WeChat to continue.'
    )
  })
})

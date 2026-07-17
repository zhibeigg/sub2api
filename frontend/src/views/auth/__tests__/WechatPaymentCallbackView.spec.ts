import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import WechatPaymentCallbackView from '@/views/auth/WechatPaymentCallbackView.vue'
import { WECHAT_PAYMENT_RESUME_HANDOFF_KEY } from '@/views/user/paymentWechatResume'

const { replaceMock, routeState, locationState, showErrorMock } = vi.hoisted(() => ({
  replaceMock: vi.fn(),
  routeState: {
    query: {} as Record<string, unknown>,
  },
  locationState: {
    current: {
      href: 'http://localhost/auth/wechat/payment/callback',
      hash: '',
      search: '',
      pathname: '/auth/wechat/payment/callback',
      origin: 'http://localhost',
    } as Location & { origin: string },
  },
  showErrorMock: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({
    replace: replaceMock,
  }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => {
      if (key === 'auth.wechatPayment.callbackTitle') return '正在恢复微信支付'
      if (key === 'auth.wechatPayment.callbackProcessing') return '正在恢复微信支付...'
      if (key === 'auth.wechatPayment.backToPayment') return '返回支付页'
      if (key === 'auth.wechatPayment.callbackMissingResumeToken') return '微信支付回调缺少恢复令牌。'
      if (key === 'payment.errors.wechatOAuthFailed') return '微信授权未能完成。'
      return key
    },
    locale: { value: 'zh-CN' },
  }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: (...args: unknown[]) => showErrorMock(...args),
  }),
}))

describe('WechatPaymentCallbackView', () => {
  beforeEach(() => {
    replaceMock.mockReset()
    showErrorMock.mockReset()
    routeState.query = {}
    window.sessionStorage.clear()
    locationState.current = {
      href: 'http://localhost/auth/wechat/payment/callback',
      hash: '',
      search: '',
      pathname: '/auth/wechat/payment/callback',
      origin: 'http://localhost',
    } as Location & { origin: string }
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: locationState.current,
    })
  })

  it('stores the resume token in sessionStorage and redirects without exposing it in the URL', async () => {
    locationState.current.hash = '#wechat_resume_token=resume-token-123&redirect=%2Fpurchase%3Ffrom%3Dwechat'

    mount(WechatPaymentCallbackView)
    await flushPromises()

    // Redirect must NOT include the sensitive token in the URL
    expect(replaceMock).toHaveBeenCalledWith({
      path: '/purchase',
      query: {
        from: 'wechat',
        wechat_resume: '1',
      },
    })
    const callArgs = replaceMock.mock.calls[0]?.[0]
    expect(JSON.stringify(callArgs)).not.toContain('resume-token-123')

    // Token must be safely stored in sessionStorage instead
    const stored = window.sessionStorage.getItem(WECHAT_PAYMENT_RESUME_HANDOFF_KEY)
    expect(stored).not.toBeNull()
    const payload = JSON.parse(stored!) as Record<string, unknown>
    expect(payload.wechat_resume_token).toBe('resume-token-123')
  })

  it('stores openid and context in sessionStorage and redirects without exposing sensitive fields in the URL', async () => {
    locationState.current.hash =
      '#openid=openid-123&state=oauth-state&scope=snsapi_base&payment_type=wxpay_direct&amount=128&order_type=subscription&plan_id=7&redirect=%2Fpayment%3Ffrom%3Dwechat'

    mount(WechatPaymentCallbackView)
    await flushPromises()

    // Redirect normalizes /payment → /purchase and must NOT include openid in URL
    expect(replaceMock).toHaveBeenCalledWith({
      path: '/purchase',
      query: {
        from: 'wechat',
        wechat_resume: '1',
      },
    })
    const callArgs = replaceMock.mock.calls[0]?.[0]
    expect(JSON.stringify(callArgs)).not.toContain('openid-123')

    // openid and payment context must be stored securely in sessionStorage
    const stored = window.sessionStorage.getItem(WECHAT_PAYMENT_RESUME_HANDOFF_KEY)
    expect(stored).not.toBeNull()
    const payload = JSON.parse(stored!) as Record<string, unknown>
    expect(payload).toMatchObject({
      openid: 'openid-123',
      payment_type: 'wxpay_direct',
      amount: '128',
      order_type: 'subscription',
      plan_id: '7',
    })
  })

  it('shows an error when the callback payload is missing the resume token', async () => {
    locationState.current.hash = '#payment_type=wxpay'

    const wrapper = mount(WechatPaymentCallbackView)
    await flushPromises()

    expect(replaceMock).not.toHaveBeenCalled()
    expect(showErrorMock).toHaveBeenCalledWith('微信支付回调缺少恢复令牌。')
    expect(wrapper.text()).toContain('微信支付回调缺少恢复令牌。')
    expect(wrapper.find('.bg-red-50').exists()).toBe(false)
  })
})

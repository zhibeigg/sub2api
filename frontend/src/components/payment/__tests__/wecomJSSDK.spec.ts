import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  WECOM_PAYMENT_JS_API_LIST,
  classifyWechatBridgePaymentResult,
  configureWecomJSSDK,
  invokeWechatJsapiPayment,
  waitForWeixinJSBridge,
  type WeixinJSBridgeLike,
  type WeixinJSSDKLike,
} from '../wecomJSSDK'

function installBridge(invoke: WeixinJSBridgeLike['invoke']) {
  (window as Window & { WeixinJSBridge?: WeixinJSBridgeLike }).WeixinJSBridge = { invoke }
}

afterEach(() => {
  vi.useRealTimers()
  ;(window as Window & { WeixinJSBridge?: WeixinJSBridgeLike }).WeixinJSBridge = undefined
  vi.restoreAllMocks()
})

describe('configureWecomJSSDK', () => {
  it('uses the fixed payment API list and resolves only after wx.ready', async () => {
    let readyCallback: (() => void) | undefined
    const sdk: WeixinJSSDKLike = {
      config: vi.fn(),
      ready: vi.fn(callback => { readyCallback = callback }),
      error: vi.fn(),
    }

    const configured = configureWecomJSSDK({
      appId: 'ww1234567890abcdef',
      timestamp: 1712345678,
      nonceStr: 'nonce-value',
      signature: 'signature-sensitive',
      jsApiList: ['openLocation'],
    }, { sdk })

    expect(sdk.config).toHaveBeenCalledWith({
      debug: false,
      appId: 'ww1234567890abcdef',
      timestamp: 1712345678,
      nonceStr: 'nonce-value',
      signature: 'signature-sensitive',
      jsApiList: [...WECOM_PAYMENT_JS_API_LIST],
    })

    let settled = false
    void configured.finally(() => { settled = true })
    await Promise.resolve()
    expect(settled).toBe(false)

    readyCallback?.()
    await expect(configured).resolves.toBeUndefined()
  })

  it('returns a structured redacted error when wx.error fires', async () => {
    let errorCallback: ((error: unknown) => void) | undefined
    const sdk: WeixinJSSDKLike = {
      config: vi.fn(),
      ready: vi.fn(),
      error: vi.fn(callback => { errorCallback = callback }),
    }

    const configured = configureWecomJSSDK({
      appId: 'ww1234567890abcdef',
      timestamp: 1712345678,
      nonceStr: 'nonce-value',
      signature: 'signature-sensitive',
    }, { sdk })
    errorCallback?.({ errMsg: 'config:fail signature-sensitive' })

    await expect(configured).rejects.toMatchObject({ reason: 'WECOM_JS_SDK_CONFIG_FAILED' })
    await configured.catch((error: Error) => {
      expect(error.message).not.toContain('signature-sensitive')
    })
  })
})

describe('WeixinJSBridge payment', () => {
  it.each([
    ['get_brand_wcpay_request:ok', 'success'],
    ['get_brand_wcpay_request:cancel', 'cancel'],
    ['get_brand_wcpay_request:fail', 'failure'],
  ] as const)('classifies %s as %s', (errMsg, expected) => {
    expect(classifyWechatBridgePaymentResult({ err_msg: errMsg })).toBe(expected)
  })

  it('invokes getBrandWCPayRequest and returns the bridge result', async () => {
    const invoke = vi.fn((_action, _payload, callback) => {
      callback({ err_msg: 'get_brand_wcpay_request:ok' })
    })
    installBridge(invoke)

    const result = await invokeWechatJsapiPayment({
      appId: 'wx123',
      timeStamp: '1712345678',
      nonceStr: 'nonce',
      package: 'prepay_id=wx123',
      signType: 'RSA',
      paySign: 'payment-signature-sensitive',
      auth_type: 'wecom',
      js_config: {
        appId: 'ww1234567890abcdef',
        timestamp: 1712345678,
        nonceStr: 'config-nonce',
        signature: 'config-signature-sensitive',
      },
    })

    expect(invoke).toHaveBeenCalledWith(
      'getBrandWCPayRequest',
      {
        appId: 'wx123',
        timeStamp: '1712345678',
        nonceStr: 'nonce',
        package: 'prepay_id=wx123',
        signType: 'RSA',
        paySign: 'payment-signature-sensitive',
      },
      expect.any(Function),
    )
    expect(result).toEqual({ err_msg: 'get_brand_wcpay_request:ok' })
  })

  it('cleans bridge-ready listeners after the bridge becomes available', async () => {
    const removeSpy = vi.spyOn(document, 'removeEventListener')
    const promise = waitForWeixinJSBridge(1000)
    installBridge(vi.fn())
    document.dispatchEvent(new Event('WeixinJSBridgeReady'))

    await expect(promise).resolves.toBeDefined()
    expect(removeSpy).toHaveBeenCalledWith('WeixinJSBridgeReady', expect.any(Function))
    expect(removeSpy).toHaveBeenCalledWith('onWeixinJSBridgeReady', expect.any(Function))
  })
})

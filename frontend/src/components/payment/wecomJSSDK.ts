import wx from 'weixin-js-sdk'
import type { WechatJSAPIConfig, WechatJSAPIPayload } from '@/types/payment'

export const WECOM_PAYMENT_JS_API_LIST = ['chooseWXPay'] as const

export type WechatPaymentClientErrorReason =
  | 'WECHAT_OAUTH_URL_INVALID'
  | 'WECHAT_JSAPI_UNAVAILABLE'
  | 'WECHAT_JSAPI_INVOKE_TIMEOUT'
  | 'WECHAT_JSAPI_FAILED'
  | 'WECOM_JS_SDK_CONFIG_INVALID'
  | 'WECOM_JS_SDK_CONFIG_FAILED'
  | 'WECOM_JS_SDK_CONFIG_TIMEOUT'

export class WechatPaymentClientError extends Error {
  readonly reason: WechatPaymentClientErrorReason

  constructor(reason: WechatPaymentClientErrorReason) {
    super(reason)
    this.name = 'WechatPaymentClientError'
    this.reason = reason
  }
}

export interface WeixinJSBridgeLike {
  invoke(
    action: string,
    payload: Record<string, unknown>,
    callback: (result: Record<string, unknown>) => void,
  ): void
}

export interface WeixinJSSDKLike {
  config(config: {
    debug: boolean
    appId: string
    timestamp: number
    nonceStr: string
    signature: string
    jsApiList: string[]
  }): void
  ready(callback: () => void): void
  error(callback: (error: unknown) => void): void
}

export type WechatBridgePaymentStatus = 'success' | 'cancel' | 'failure'

function isNonEmptyString(value: unknown): value is string {
  return typeof value === 'string' && value.trim() !== ''
}

function normalizeTimestamp(value: unknown): number {
  const timestamp = typeof value === 'number' ? value : Number(value)
  return Number.isSafeInteger(timestamp) && timestamp > 0 ? timestamp : 0
}

function normalizeWecomJSConfig(config: WechatJSAPIConfig | undefined): {
  appId: string
  timestamp: number
  nonceStr: string
  signature: string
} | null {
  if (!config) return null
  const timestamp = normalizeTimestamp(config.timestamp)
  if (
    !isNonEmptyString(config.appId)
    || timestamp <= 0
    || !isNonEmptyString(config.nonceStr)
    || !isNonEmptyString(config.signature)
  ) {
    return null
  }
  return {
    appId: config.appId.trim(),
    timestamp,
    nonceStr: config.nonceStr.trim(),
    signature: config.signature.trim(),
  }
}

export function isWecomJSAPIPayload(payload: WechatJSAPIPayload): boolean {
  return payload.auth_type?.trim().toLowerCase() === 'wecom'
}

export function configureWecomJSSDK(
  config: WechatJSAPIConfig | undefined,
  options: { sdk?: WeixinJSSDKLike; timeoutMs?: number } = {},
): Promise<void> {
  const normalized = normalizeWecomJSConfig(config)
  if (!normalized) {
    return Promise.reject(new WechatPaymentClientError('WECOM_JS_SDK_CONFIG_INVALID'))
  }

  const sdk = options.sdk ?? (wx as WeixinJSSDKLike)
  const timeoutMs = options.timeoutMs ?? 6000

  return new Promise((resolve, reject) => {
    let active = true
    const finish = (error?: WechatPaymentClientError) => {
      if (!active) return
      active = false
      window.clearTimeout(timer)
      if (error) {
        reject(error)
      } else {
        resolve()
      }
    }
    const timer = window.setTimeout(
      () => finish(new WechatPaymentClientError('WECOM_JS_SDK_CONFIG_TIMEOUT')),
      timeoutMs,
    )

    sdk.ready(() => finish())
    sdk.error(() => finish(new WechatPaymentClientError('WECOM_JS_SDK_CONFIG_FAILED')))

    try {
      sdk.config({
        debug: false,
        ...normalized,
        jsApiList: [...WECOM_PAYMENT_JS_API_LIST],
      })
    } catch {
      finish(new WechatPaymentClientError('WECOM_JS_SDK_CONFIG_FAILED'))
    }
  })
}

export async function prepareWechatJSAPI(
  payload: WechatJSAPIPayload,
  options: { sdk?: WeixinJSSDKLike; timeoutMs?: number } = {},
): Promise<void> {
  if (!isWecomJSAPIPayload(payload)) return
  await configureWecomJSSDK(payload.js_config, options)
}

function getWeixinJSBridge(): WeixinJSBridgeLike | undefined {
  if (typeof window === 'undefined') return undefined
  return (window as Window & { WeixinJSBridge?: WeixinJSBridgeLike }).WeixinJSBridge
}

export function waitForWeixinJSBridge(timeoutMs = 4000): Promise<WeixinJSBridgeLike> {
  const existing = getWeixinJSBridge()
  if (existing) return Promise.resolve(existing)
  if (typeof document === 'undefined' || typeof window === 'undefined') {
    return Promise.reject(new WechatPaymentClientError('WECHAT_JSAPI_UNAVAILABLE'))
  }

  return new Promise((resolve, reject) => {
    let active = true
    const cleanup = () => {
      document.removeEventListener('WeixinJSBridgeReady', handleReady)
      document.removeEventListener('onWeixinJSBridgeReady', handleReady)
      window.clearTimeout(timer)
    }
    const finish = (bridge?: WeixinJSBridgeLike) => {
      if (!active) return
      active = false
      cleanup()
      if (bridge) {
        resolve(bridge)
      } else {
        reject(new WechatPaymentClientError('WECHAT_JSAPI_UNAVAILABLE'))
      }
    }
    const handleReady = () => finish(getWeixinJSBridge())
    const timer = window.setTimeout(() => finish(getWeixinJSBridge()), timeoutMs)

    document.addEventListener('WeixinJSBridgeReady', handleReady, false)
    document.addEventListener('onWeixinJSBridgeReady', handleReady, false)
  })
}

export async function invokeWechatJsapiPayment(
  payload: WechatJSAPIPayload,
  options: { bridgeTimeoutMs?: number; invokeTimeoutMs?: number } = {},
): Promise<Record<string, unknown>> {
  const bridge = await waitForWeixinJSBridge(options.bridgeTimeoutMs)
  const invokeTimeoutMs = options.invokeTimeoutMs ?? 10000

  return new Promise((resolve, reject) => {
    let active = true
    const finish = (result?: Record<string, unknown>, error?: WechatPaymentClientError) => {
      if (!active) return
      active = false
      window.clearTimeout(timer)
      if (error) {
        reject(error)
      } else {
        resolve(result || {})
      }
    }
    const timer = window.setTimeout(
      () => finish(undefined, new WechatPaymentClientError('WECHAT_JSAPI_INVOKE_TIMEOUT')),
      invokeTimeoutMs,
    )

    try {
      const paymentPayload: Record<string, unknown> = {
        appId: payload.appId,
        timeStamp: payload.timeStamp,
        nonceStr: payload.nonceStr,
        package: payload.package,
        signType: payload.signType,
        paySign: payload.paySign,
      }
      bridge.invoke('getBrandWCPayRequest', paymentPayload, (result) => finish(result || {}))
    } catch {
      finish(undefined, new WechatPaymentClientError('WECHAT_JSAPI_FAILED'))
    }
  })
}

export function classifyWechatBridgePaymentResult(result: Record<string, unknown>): WechatBridgePaymentStatus {
  const rawMessage = typeof result.err_msg === 'string'
    ? result.err_msg
    : (typeof result.errMsg === 'string' ? result.errMsg : '')
  const normalized = rawMessage.trim().toLowerCase()
  if (normalized.includes(':ok') || normalized.endsWith('ok')) return 'success'
  if (normalized.includes('cancel')) return 'cancel'
  return 'failure'
}

import { normalizeVisibleMethod } from '@/components/payment/paymentFlow'
import type { WechatClientEnvironment } from '@/components/payment/paymentEnvironment'
import { extractApiErrorCode } from '@/utils/apiError'

const DISPLAY_METHOD_ALIASES: Record<string, string> = {
  wechat: 'wxpay',
  wechat_pay: 'wxpay',
}

export interface PaymentScenarioContext {
  paymentMethod: string
  isMobile: boolean
  isWechatBrowser: boolean
  wechatEnvironment?: WechatClientEnvironment
}

export interface PaymentScenarioErrorDescriptor {
  messageKey: string
  hintKey?: string
}

export function normalizePaymentMethodForDisplay(paymentType: string): string {
  const trimmed = paymentType.trim().toLowerCase()
  const visibleMethod = normalizeVisibleMethod(trimmed)
  if (visibleMethod) return visibleMethod
  return DISPLAY_METHOD_ALIASES[trimmed] ?? trimmed
}

export function paymentMethodI18nKey(paymentType: string): string {
  return `payment.methods.${normalizePaymentMethodForDisplay(paymentType)}`
}

export function buildPaymentErrorToastMessage(message: string, hint?: string): string {
  if (!hint) return message
  return `${message} ${hint}`.trim()
}

function defaultWechatHint(context: PaymentScenarioContext): string {
  if (context.wechatEnvironment === 'wecom') return 'payment.errors.wecomRetryHint'
  if (!context.isMobile) return 'payment.errors.wechatScanOnDesktopHint'
  return 'payment.errors.wechatOpenInWeChatHint'
}

function defaultAlipayHint(context: PaymentScenarioContext): string {
  if (context.isMobile) return 'payment.errors.alipayMobileOpenHint'
  return 'payment.errors.alipayDesktopQrHint'
}

export function describePaymentScenarioError(
  error: unknown,
  context: PaymentScenarioContext,
): PaymentScenarioErrorDescriptor | null {
  const method = normalizePaymentMethodForDisplay(context.paymentMethod)
  const code = extractApiErrorCode(error)
  const message = error instanceof Error
    ? error.message
    : (typeof error === 'object' && error && 'message' in error && typeof error.message === 'string'
      ? error.message
      : String(error || ''))
  const normalizedMessage = message.toLowerCase()

  if (code === 'PAYMENT_GATEWAY_ERROR' && (method === 'wxpay' || method === 'qqpay')) {
    return {
      messageKey: 'payment.errors.gatewayResponseInvalid',
    }
  }

  if (method === 'wxpay') {
    const isWecom = context.wechatEnvironment === 'wecom'
    if (code === 'WECHAT_OAUTH_URL_INVALID' || code === 'WECOM_OAUTH_CODE_INVALID') {
      return {
        messageKey: isWecom ? 'payment.errors.wecomOAuthFailed' : 'payment.errors.wechatOAuthFailed',
        hintKey: defaultWechatHint(context),
      }
    }
    if (code === 'WECOM_JS_SDK_CONFIG_INVALID') {
      return {
        messageKey: 'payment.errors.wecomJsSdkConfigInvalid',
        hintKey: 'payment.errors.wecomRetryHint',
      }
    }
    if (code === 'WECOM_JS_SDK_CONFIG_FAILED' || code === 'WECOM_JS_CONFIG_URL_INVALID') {
      return {
        messageKey: 'payment.errors.wecomJsSdkConfigFailed',
        hintKey: 'payment.errors.wecomRetryHint',
      }
    }
    if (code === 'WECOM_JS_SDK_CONFIG_TIMEOUT') {
      return {
        messageKey: 'payment.errors.wecomJsSdkConfigTimeout',
        hintKey: 'payment.errors.wecomRetryHint',
      }
    }
    if (code === 'WECHAT_JSAPI_INVOKE_TIMEOUT') {
      return {
        messageKey: isWecom ? 'payment.errors.wecomJsapiTimeout' : 'payment.errors.wechatJsapiTimeout',
        hintKey: defaultWechatHint(context),
      }
    }
    if (code?.startsWith('WECOM_')) {
      return {
        messageKey: 'payment.errors.wecomOAuthFailed',
        hintKey: 'payment.errors.wecomRetryHint',
      }
    }
    if (code === 'WECHAT_NATIVE_NOT_AUTHORIZED') {
      return {
        messageKey: 'payment.errors.wechatNativeNotAuthorized',
        hintKey: 'payment.errors.wechatContactAdminHint',
      }
    }
    if (code === 'WECHAT_H5_NOT_AUTHORIZED') {
      return {
        messageKey: 'payment.errors.wechatH5NotAuthorized',
        hintKey: context.isMobile
          ? 'payment.errors.wechatScanOnDesktopHint'
          : 'payment.errors.wechatContactAdminHint',
      }
    }
    if (code === 'WECHAT_JSAPI_NOT_AUTHORIZED') {
      return {
        messageKey: 'payment.errors.wechatJsapiNotAuthorized',
        hintKey: 'payment.errors.wechatSwitchBrowserHint',
      }
    }
    if (code === 'NO_AVAILABLE_WXPAY_CAPABILITY') {
      return {
        messageKey: 'payment.errors.wechatNoAvailableCapability',
        hintKey: defaultWechatHint(context),
      }
    }
    if (code === 'WECHAT_APPID_MCHID_MISMATCH') {
      return {
        messageKey: 'payment.errors.wechatAppIdMchIdMismatch',
        hintKey: 'payment.errors.wechatContactAdminHint',
      }
    }
    if (code === 'WECHAT_SIGN_ERROR') {
      return {
        messageKey: 'payment.errors.wechatSignError',
        hintKey: 'payment.errors.wechatContactAdminHint',
      }
    }
    if (code === 'WECHAT_PAYMENT_API_ERROR') {
      return {
        messageKey: 'payment.errors.wechatApiError',
        hintKey: 'payment.errors.wechatContactAdminHint',
      }
    }
    if (code === 'WECHAT_PAYMENT_MP_NOT_CONFIGURED') {
      return {
        messageKey: 'payment.errors.wechatPaymentMpNotConfigured',
        hintKey: context.isWechatBrowser
          ? 'payment.errors.wechatSwitchBrowserHint'
          : defaultWechatHint(context),
      }
    }
    if (code === 'NO_AVAILABLE_INSTANCE') {
      return {
        messageKey: 'payment.errors.wechatUnavailable',
        hintKey: defaultWechatHint(context),
      }
    }
    if (code === 'WECHAT_JSAPI_FAILED' || normalizedMessage.includes('get_brand_wcpay_request:fail')) {
      return {
        messageKey: isWecom ? 'payment.errors.wecomJsapiFailed' : 'payment.errors.wechatJsapiFailed',
        hintKey: defaultWechatHint(context),
      }
    }
    if (
      code === 'WECHAT_JSAPI_UNAVAILABLE'
      || normalizedMessage.includes('weixinjsbridge is unavailable')
      || normalizedMessage.includes('wechat_jsapi_unavailable')
    ) {
      return {
        messageKey: isWecom ? 'payment.errors.wecomJsapiUnavailable' : 'payment.errors.wechatJsapiUnavailable',
        hintKey: defaultWechatHint(context),
      }
    }
    if (code === 'UNHANDLED_PAYMENT_SCENARIO') {
      return {
        messageKey: 'payment.errors.wechatUnavailable',
        hintKey: defaultWechatHint(context),
      }
    }
  }

  if (method === 'alipay' && (code === 'PAYMENT_GATEWAY_ERROR' || code === 'UNHANDLED_PAYMENT_SCENARIO')) {
    return {
      messageKey: context.isMobile
        ? 'payment.errors.alipayMobileUnavailable'
        : 'payment.errors.alipayDesktopUnavailable',
      hintKey: defaultAlipayHint(context),
    }
  }

  return null
}

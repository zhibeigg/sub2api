/**
 * Shared constants and types for payment provider management.
 */

// --- Types ---

export type ConfigFieldInputType = 'text' | 'password' | 'textarea' | 'select'

export interface ConfigFieldDef {
  key: string
  label: string
  sensitive: boolean
  inputType?: ConfigFieldInputType
  optional?: boolean
  clearable?: boolean
  defaultValue?: string
  hintKey?: string
  options?: TypeOption[]
}

export interface TypeOption {
  value: string
  label: string
  [key: string]: unknown
}

export interface EasyPayCustomMethod {
  type: string
  upstreamType: string
  displayName: string
}

export type EasyPayProtocolVersion = '1' | '2'

export const EASYPAY_PROTOCOL_V1: EasyPayProtocolVersion = '1'
export const EASYPAY_PROTOCOL_V2: EasyPayProtocolVersion = '2'

export type WxpayJsapiAuthType = 'mp' | 'wecom'
export type WxpayCapabilityToggleKey = 'nativeEnabled' | 'h5Enabled' | 'jsapiEnabled'

export const WXPAY_JSAPI_AUTH_MP: WxpayJsapiAuthType = 'mp'
export const WXPAY_JSAPI_AUTH_WECOM: WxpayJsapiAuthType = 'wecom'

export interface WxpayCapabilityConfig {
  nativeEnabled: boolean
  h5Enabled: boolean
  jsapiEnabled: boolean
  jsapiAuthType: WxpayJsapiAuthType
}

export const DEFAULT_WXPAY_CAPABILITIES: WxpayCapabilityConfig = {
  nativeEnabled: true,
  h5Enabled: false,
  jsapiEnabled: false,
  jsapiAuthType: WXPAY_JSAPI_AUTH_MP,
}

export const EASYPAY_PROTOCOL_OPTIONS: TypeOption[] = [
  { value: EASYPAY_PROTOCOL_V1, label: 'V1 / MD5' },
  { value: EASYPAY_PROTOCOL_V2, label: 'V2 / RSA-SHA256' },
]

/** Callback URL paths for a provider. */
export interface CallbackPaths {
  notifyUrl?: string
  returnUrl?: string
}

// --- Constants ---

/** Maps provider key → available payment types. */
export const PROVIDER_SUPPORTED_TYPES: Record<string, string[]> = {
  easypay: ['alipay', 'wxpay'],
  alipay: ['alipay'],
  wxpay: ['wxpay'],
  stripe: ['card', 'alipay', 'wxpay', 'link'],
  airwallex: ['airwallex'],
}

export const EASYPAY_V2_SUPPORTED_TYPES = ['alipay', 'wxpay', 'qqpay'] as const

/** Available payment modes for EasyPay providers. */
export const EASYPAY_PAYMENT_MODES = ['qrcode', 'popup'] as const

/** Fixed display order for user-facing payment methods */
export const METHOD_ORDER = ['alipay', 'alipay_direct', 'wxpay', 'wxpay_direct', 'qqpay', 'stripe', 'airwallex'] as const

export function isBuiltInAlipayMethod(type: string): boolean {
  return type === 'alipay' || type === 'alipay_direct'
}

export function isBuiltInWxpayMethod(type: string): boolean {
  return type === 'wxpay' || type === 'wxpay_direct'
}

export function isBuiltInQqpayMethod(type: string): boolean {
  return type === 'qqpay'
}

/** Payment mode constants */
export const PAYMENT_MODE_QRCODE = 'qrcode'
export const PAYMENT_MODE_POPUP = 'popup'
/** Alipay-only: skip FACE_TO_FACE_PAYMENT precreate and open the Alipay
 * checkout page in a new tab instead. Backend `alipay.go` matches on this
 * literal (case-insensitive); other values fall back to the default
 * precreate→pagepay flow. */
export const PAYMENT_MODE_REDIRECT = 'redirect'

export const PAYMENT_CURRENCY_OPTIONS: TypeOption[] = [
  { value: 'CNY', label: 'CNY' },
  { value: 'HKD', label: 'HKD' },
  { value: 'USD', label: 'USD' },
  { value: 'EUR', label: 'EUR' },
  { value: 'GBP', label: 'GBP' },
  { value: 'AUD', label: 'AUD' },
  { value: 'CAD', label: 'CAD' },
  { value: 'SGD', label: 'SGD' },
  { value: 'JPY', label: 'JPY' },
  { value: 'KRW', label: 'KRW' },
  { value: 'NZD', label: 'NZD' },
]

// 与后端当前集成的 stripe-go v85.0.0 的 stripe.APIVersion 保持一致。
export const STRIPE_SDK_API_VERSION = '2026-03-25.dahlia'

/** Preferred popup size for payment gateways. Alipay's standard checkout
 * (QR + account login panel) needs ~1200×900 to render without any scrolling. */
const PAYMENT_POPUP_PREFERRED_WIDTH = 1250
const PAYMENT_POPUP_PREFERRED_HEIGHT = 900

/** Build a window.open features string sized to fit within the current screen
 * while preferring the above dimensions. Centers the popup on the available
 * work area so nothing is clipped on smaller laptop displays. */
export function getPaymentPopupFeatures(): string {
  const screen = typeof window !== 'undefined' ? window.screen : null
  const availW = screen?.availWidth ?? PAYMENT_POPUP_PREFERRED_WIDTH
  const availH = screen?.availHeight ?? PAYMENT_POPUP_PREFERRED_HEIGHT
  const width = Math.min(PAYMENT_POPUP_PREFERRED_WIDTH, availW - 40)
  const height = Math.min(PAYMENT_POPUP_PREFERRED_HEIGHT, availH - 40)
  const left = Math.max(0, Math.floor((availW - width) / 2))
  const top = Math.max(0, Math.floor((availH - height) / 2))
  return `width=${width},height=${height},left=${left},top=${top},scrollbars=yes,resizable=yes`
}

/** Webhook paths for each provider (relative to origin). */
export const WEBHOOK_PATHS: Record<string, string> = {
  easypay: '/api/v1/payment/webhook/easypay',
  alipay: '/api/v1/payment/webhook/alipay',
  wxpay: '/api/v1/payment/webhook/wxpay',
  stripe: '/api/v1/payment/webhook/stripe',
  airwallex: '/api/v1/payment/webhook/airwallex',
}

export const RETURN_PATH = '/payment/result'

/** Fixed callback paths per provider — displayed as read-only after base URL. */
export const PROVIDER_CALLBACK_PATHS: Record<string, CallbackPaths> = {
  easypay: { notifyUrl: WEBHOOK_PATHS.easypay, returnUrl: RETURN_PATH },
  alipay: { notifyUrl: WEBHOOK_PATHS.alipay, returnUrl: RETURN_PATH },
  wxpay: { notifyUrl: WEBHOOK_PATHS.wxpay },
  // stripe: 不需要回调 URL 配置，Webhook 单独配置。
  // airwallex: 不需要回调 URL 配置，Webhook 在空中云汇后台配置。
}

/** Per-provider config fields (excludes notifyUrl/returnUrl which are handled separately). */
export const PROVIDER_CONFIG_FIELDS: Record<string, ConfigFieldDef[]> = {
  easypay: [
    { key: 'protocolVersion', label: '', sensitive: false, defaultValue: EASYPAY_PROTOCOL_V2, options: EASYPAY_PROTOCOL_OPTIONS },
    { key: 'pid', label: 'PID', sensitive: false },
    { key: 'apiBase', label: '', sensitive: false, hintKey: 'admin.settings.payment.field_easypayApiBaseHint' },
    { key: 'merchantPrivateKey', label: '', sensitive: true, inputType: 'textarea' },
    { key: 'platformPublicKey', label: '', sensitive: true, inputType: 'textarea' },
  ],
  alipay: [
    { key: 'appId', label: 'App ID', sensitive: false },
    { key: 'privateKey', label: '', sensitive: true, inputType: 'textarea' },
    { key: 'publicKey', label: '', sensitive: true, inputType: 'textarea' },
  ],
  wxpay: [
    { key: 'appId', label: 'App ID', sensitive: false },
    { key: 'mchId', label: '', sensitive: false },
    { key: 'privateKey', label: '', sensitive: true, inputType: 'textarea' },
    { key: 'apiV3Key', label: '', sensitive: true, inputType: 'password' },
    { key: 'certSerial', label: '', sensitive: false },
    { key: 'publicKey', label: '', sensitive: true, inputType: 'textarea' },
    { key: 'publicKeyId', label: '', sensitive: false },
  ],
  stripe: [
    { key: 'secretKey', label: '', sensitive: true, inputType: 'password' },
    { key: 'publishableKey', label: '', sensitive: false },
    { key: 'webhookSecret', label: '', sensitive: true, inputType: 'password' },
    { key: 'currency', label: '', sensitive: false, defaultValue: 'CNY', hintKey: 'admin.settings.payment.field_paymentCurrencyHint', options: PAYMENT_CURRENCY_OPTIONS },
  ],
  airwallex: [
    { key: 'clientId', label: '', sensitive: false },
    { key: 'apiKey', label: '', sensitive: true, inputType: 'password' },
    { key: 'webhookSecret', label: '', sensitive: true, inputType: 'password' },
    { key: 'apiBase', label: '', sensitive: false, defaultValue: 'https://api.airwallex.com/api/v1', hintKey: 'admin.settings.payment.field_airwallexApiBaseHint' },
    { key: 'countryCode', label: '', sensitive: false, defaultValue: 'CN' },
    { key: 'currency', label: '', sensitive: false, defaultValue: 'CNY', hintKey: 'admin.settings.payment.field_paymentCurrencyHint', options: PAYMENT_CURRENCY_OPTIONS },
    { key: 'accountId', label: '', sensitive: false, optional: true, clearable: true, hintKey: 'admin.settings.payment.field_accountIdHint' },
  ],
}

// --- Helpers ---

function hasOwnConfigValue(config: Record<string, string> | null | undefined, key: string): boolean {
  return Boolean(config && Object.prototype.hasOwnProperty.call(config, key))
}

export function normalizeProviderConfigBoolean(value: unknown, fallback: boolean): boolean {
  const normalized = String(value ?? '').trim().toLowerCase()
  if (normalized === 'true') return true
  if (normalized === 'false') return false
  return fallback
}

export function normalizeWxpayJsapiAuthType(
  value: unknown,
  fallback: WxpayJsapiAuthType = WXPAY_JSAPI_AUTH_MP,
): WxpayJsapiAuthType {
  const normalized = String(value ?? '').trim().toLowerCase()
  if (normalized === WXPAY_JSAPI_AUTH_MP || normalized === WXPAY_JSAPI_AUTH_WECOM) {
    return normalized
  }
  return fallback
}

export function resolveWxpayCapabilities(
  config: Record<string, string> | null | undefined,
  defaults: WxpayCapabilityConfig = DEFAULT_WXPAY_CAPABILITIES,
): WxpayCapabilityConfig {
  const h5Fallback = Boolean(config?.h5AppName?.trim() && config?.h5AppUrl?.trim())
  // Only the historical Official Account field implied JSAPI enablement.
  // WeCom was introduced with an explicit capability switch, so its fields
  // must never silently enable JSAPI when jsapiEnabled is absent.
  const jsapiFallback = Boolean(config?.mpAppId?.trim())
  return {
    nativeEnabled: hasOwnConfigValue(config, 'nativeEnabled')
      ? normalizeProviderConfigBoolean(config?.nativeEnabled, defaults.nativeEnabled)
      : defaults.nativeEnabled,
    h5Enabled: hasOwnConfigValue(config, 'h5Enabled')
      ? normalizeProviderConfigBoolean(config?.h5Enabled, h5Fallback)
      : h5Fallback,
    jsapiEnabled: hasOwnConfigValue(config, 'jsapiEnabled')
      ? normalizeProviderConfigBoolean(config?.jsapiEnabled, jsapiFallback)
      : jsapiFallback,
    // Historical configurations did not store the auth mode. Always interpret
    // those as Official Account mode; a ww-prefixed base AppID is only a UI
    // suggestion and must never silently migrate an existing provider.
    jsapiAuthType: normalizeWxpayJsapiAuthType(config?.jsapiAuthType, defaults.jsapiAuthType),
  }
}

export function writeWxpayCapabilities(
  config: Record<string, string>,
  capabilities: WxpayCapabilityConfig,
): void {
  config.nativeEnabled = String(capabilities.nativeEnabled)
  config.h5Enabled = String(capabilities.h5Enabled)
  config.jsapiEnabled = String(capabilities.jsapiEnabled)
  config.jsapiAuthType = normalizeWxpayJsapiAuthType(capabilities.jsapiAuthType)
}

export function getWxpayJsapiConfigFields(authType: unknown): ConfigFieldDef[] {
  if (normalizeWxpayJsapiAuthType(authType) === WXPAY_JSAPI_AUTH_WECOM) {
    return [
      {
        key: 'wecomAppSecret',
        label: '',
        sensitive: true,
        inputType: 'password',
        hintKey: 'admin.settings.payment.field_wecomAppSecretHint',
      },
      {
        key: 'wecomAgentId',
        label: '',
        sensitive: false,
        inputType: 'text',
        optional: true,
        clearable: true,
        hintKey: 'admin.settings.payment.field_wecomAgentIdHint',
      },
    ]
  }

  return [
    {
      key: 'mpAppId',
      label: '',
      sensitive: false,
      inputType: 'text',
      optional: true,
      clearable: true,
      hintKey: 'admin.settings.payment.field_mpAppIdHint',
    },
  ]
}

export function normalizeEasyPayProtocolVersion(
  value: unknown,
  fallback: EasyPayProtocolVersion = EASYPAY_PROTOCOL_V1,
): EasyPayProtocolVersion {
  const normalized = String(value ?? '').trim()
  if (normalized === EASYPAY_PROTOCOL_V1 || normalized === EASYPAY_PROTOCOL_V2) {
    return normalized
  }
  return fallback
}

export function getEasyPayProtocolVersion(
  config: Record<string, string> | null | undefined,
  fallback: EasyPayProtocolVersion = EASYPAY_PROTOCOL_V1,
): EasyPayProtocolVersion {
  return normalizeEasyPayProtocolVersion(config?.protocolVersion, fallback)
}

export function getProviderSupportedTypes(
  providerKey: string,
  protocolVersion?: unknown,
): string[] {
  if (providerKey === 'easypay') {
    const version = normalizeEasyPayProtocolVersion(protocolVersion, EASYPAY_PROTOCOL_V1)
    return version === EASYPAY_PROTOCOL_V2
      ? [...EASYPAY_V2_SUPPORTED_TYPES]
      : [...PROVIDER_SUPPORTED_TYPES.easypay]
  }
  return [...(PROVIDER_SUPPORTED_TYPES[providerKey] || [])]
}

export function getProviderConfigFields(
  providerKey: string,
  protocolVersion?: unknown,
): ConfigFieldDef[] {
  if (providerKey !== 'easypay') {
    return PROVIDER_CONFIG_FIELDS[providerKey] || []
  }

  const version = normalizeEasyPayProtocolVersion(protocolVersion, EASYPAY_PROTOCOL_V2)
  const commonFields: ConfigFieldDef[] = [
    { key: 'protocolVersion', label: '', sensitive: false, defaultValue: version, options: EASYPAY_PROTOCOL_OPTIONS },
    { key: 'pid', label: 'PID', sensitive: false },
    { key: 'apiBase', label: '', sensitive: false, hintKey: 'admin.settings.payment.field_easypayApiBaseHint' },
  ]
  if (version === EASYPAY_PROTOCOL_V1) {
    return [
      ...commonFields,
      { key: 'pkey', label: 'PKey', sensitive: true },
      { key: 'cidAlipay', label: '', sensitive: false, optional: true },
      { key: 'cidWxpay', label: '', sensitive: false, optional: true },
    ]
  }
  return [
    ...commonFields,
    { key: 'merchantPrivateKey', label: '', sensitive: true, inputType: 'textarea' },
    { key: 'platformPublicKey', label: '', sensitive: true, inputType: 'textarea' },
  ]
}

/** Resolve type label for display. */
export function resolveTypeLabel(
  typeVal: string,
  _providerKey: string,
  allTypes: TypeOption[],
  _redirectLabel: string,
): TypeOption {
  return allTypes.find(pt => pt.value === typeVal) || { value: typeVal, label: typeVal }
}

/** Get available type options for a provider key. */
export function getAvailableTypes(
  providerKey: string,
  allTypes: TypeOption[],
  redirectLabel: string,
  protocolVersion?: unknown,
): TypeOption[] {
  const types = getProviderSupportedTypes(providerKey, protocolVersion)
  return types.map(t => resolveTypeLabel(t, providerKey, allTypes, redirectLabel))
}

export function parseEasyPayCustomMethods(raw: string | undefined): EasyPayCustomMethod[] {
  if (!raw || !raw.trim()) return []
  try {
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed
      .map(item => ({
        type: String(item?.type || '').trim(),
        upstreamType: String(item?.upstreamType || '').trim(),
        displayName: String(item?.displayName || '').trim(),
      }))
      .filter(item => item.type && item.upstreamType)
  } catch {
    return []
  }
}

export function serializeEasyPayCustomMethods(methods: EasyPayCustomMethod[]): string {
  const clean = methods
    .map(method => ({
      type: method.type.trim(),
      upstreamType: method.upstreamType.trim(),
      displayName: method.displayName.trim(),
    }))
    .filter(method => method.type && method.upstreamType)
  return clean.length ? JSON.stringify(clean) : ''
}

/** Extract base URL from a full callback URL by removing the known path suffix. */
export function extractBaseUrl(fullUrl: string, path: string): string {
  if (!fullUrl) return ''
  if (fullUrl.endsWith(path)) return fullUrl.slice(0, -path.length)
  // Fallback: try to extract origin
  try { return new URL(fullUrl).origin } catch { return fullUrl }
}

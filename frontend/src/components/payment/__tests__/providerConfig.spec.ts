import { describe, expect, it } from 'vitest'
import {
  EASYPAY_PROTOCOL_V1,
  EASYPAY_PROTOCOL_V2,
  PAYMENT_CURRENCY_OPTIONS,
  PROVIDER_CONFIG_FIELDS,
  getEasyPayProtocolVersion,
  getProviderConfigFields,
  getProviderSupportedTypes,
  isBuiltInAlipayMethod,
  isBuiltInQqpayMethod,
  isBuiltInWxpayMethod,
  parseEasyPayCustomMethods,
  resolveWxpayCapabilities,
  serializeEasyPayCustomMethods,
  writeWxpayCapabilities,
} from '@/components/payment/providerConfig'

function findField(providerKey: string, key: string) {
  const fields = PROVIDER_CONFIG_FIELDS[providerKey] || []
  return fields.find(field => field.key === key)
}

describe('PROVIDER_CONFIG_FIELDS.wxpay', () => {
  it('keeps admin form validation aligned with backend-required credentials', () => {
    expect(findField('wxpay', 'publicKeyId')?.optional).toBeFalsy()
    expect(findField('wxpay', 'certSerial')?.optional).toBeFalsy()
  })

  it('only keeps the simplified visible credential set in the admin form', () => {
    expect(findField('wxpay', 'mpAppId')).toBeUndefined()
    expect(findField('wxpay', 'h5AppName')).toBeUndefined()
    expect(findField('wxpay', 'h5AppUrl')).toBeUndefined()
  })
})

describe('wxpay capability config', () => {
  it('uses backward-compatible inference only when explicit booleans are missing', () => {
    expect(resolveWxpayCapabilities({})).toEqual({
      nativeEnabled: true,
      h5Enabled: false,
      jsapiEnabled: false,
    })
    expect(resolveWxpayCapabilities({
      h5AppName: 'Sub2API',
      h5AppUrl: 'https://pay.example.com',
      mpAppId: 'wx-mp-app',
    })).toEqual({
      nativeEnabled: true,
      h5Enabled: true,
      jsapiEnabled: true,
    })
    expect(resolveWxpayCapabilities({
      nativeEnabled: 'false',
      h5Enabled: 'false',
      jsapiEnabled: 'false',
      h5AppName: 'Sub2API',
      h5AppUrl: 'https://pay.example.com',
      mpAppId: 'wx-mp-app',
    })).toEqual({
      nativeEnabled: false,
      h5Enabled: false,
      jsapiEnabled: false,
    })
  })

  it('writes stable explicit boolean strings without touching sensitive values', () => {
    const config = {
      privateKey: 'masked-secret-value',
      publicKey: 'masked-public-value',
    }
    writeWxpayCapabilities(config, {
      nativeEnabled: true,
      h5Enabled: false,
      jsapiEnabled: true,
    })

    expect(config).toEqual({
      privateKey: 'masked-secret-value',
      publicKey: 'masked-public-value',
      nativeEnabled: 'true',
      h5Enabled: 'false',
      jsapiEnabled: 'true',
    })
  })
})

describe('PROVIDER_CONFIG_FIELDS.airwallex', () => {
  it('adds currency config with CNY as the default', () => {
    const currency = findField('airwallex', 'currency')

    expect(currency?.defaultValue).toBe('CNY')
    expect(currency?.hintKey).toBe('admin.settings.payment.field_paymentCurrencyHint')
    expect(currency?.options).toBe(PAYMENT_CURRENCY_OPTIONS)
  })

  it('marks accountId as optional and explains when it can be left blank', () => {
    const accountId = findField('airwallex', 'accountId')

    expect(accountId?.optional).toBe(true)
    expect(accountId?.clearable).toBe(true)
    expect(accountId?.hintKey).toBe('admin.settings.payment.field_accountIdHint')
  })

  it('explains that apiBase must match the Airwallex key environment', () => {
    expect(findField('airwallex', 'apiBase')?.hintKey).toBe('admin.settings.payment.field_airwallexApiBaseHint')
  })
})

describe('PROVIDER_CONFIG_FIELDS.stripe', () => {
  it('adds currency config with CNY as the default', () => {
    const currency = findField('stripe', 'currency')

    expect(currency?.defaultValue).toBe('CNY')
    expect(currency?.hintKey).toBe('admin.settings.payment.field_paymentCurrencyHint')
    expect(currency?.options).toBe(PAYMENT_CURRENCY_OPTIONS)
  })
})

describe('EasyPay protocol config', () => {
  it('treats historical config without protocolVersion as V1', () => {
    expect(getEasyPayProtocolVersion({ pid: '1001' })).toBe(EASYPAY_PROTOCOL_V1)
  })

  it('uses V2 credentials and QQ Wallet as a built-in method', () => {
    const fields = getProviderConfigFields('easypay', EASYPAY_PROTOCOL_V2)

    expect(fields.find(field => field.key === 'apiBase')?.hintKey).toBe('admin.settings.payment.field_easypayApiBaseHint')
    expect(fields.some(field => field.key === 'merchantPrivateKey')).toBe(true)
    expect(fields.some(field => field.key === 'platformPublicKey')).toBe(true)
    expect(fields.some(field => field.key === 'pkey')).toBe(false)
    expect(getProviderSupportedTypes('easypay', EASYPAY_PROTOCOL_V2)).toEqual(['alipay', 'wxpay', 'qqpay'])
  })

  it('keeps V1 pkey and channel IDs without adding QQ Wallet as built-in', () => {
    const fields = getProviderConfigFields('easypay', EASYPAY_PROTOCOL_V1)

    expect(fields.some(field => field.key === 'pkey')).toBe(true)
    expect(fields.some(field => field.key === 'cidAlipay')).toBe(true)
    expect(fields.some(field => field.key === 'cidWxpay')).toBe(true)
    expect(fields.some(field => field.key === 'merchantPrivateKey')).toBe(false)
    expect(getProviderSupportedTypes('easypay', EASYPAY_PROTOCOL_V1)).toEqual(['alipay', 'wxpay'])
  })
})

describe('EasyPay custom methods config', () => {
  it('parses customMethods from the JSON string stored in provider config', () => {
    expect(parseEasyPayCustomMethods(
      '[{"type":"ldc","upstreamType":"epay","displayName":"LDC"},{"type":"usdt_trc20","upstreamType":"usdt","displayName":"USDT-TRC20"}]',
    )).toEqual([
      { type: 'ldc', upstreamType: 'epay', displayName: 'LDC' },
      { type: 'usdt_trc20', upstreamType: 'usdt', displayName: 'USDT-TRC20' },
    ])
  })

  it('serializes non-empty custom methods into the config string format', () => {
    expect(serializeEasyPayCustomMethods([
      { type: 'ldc', upstreamType: 'epay', displayName: 'LDC' },
      { type: '  ', upstreamType: 'ignored', displayName: 'Ignored' },
      { type: 'usdt_trc20', upstreamType: 'usdt', displayName: '' },
    ])).toBe('[{"type":"ldc","upstreamType":"epay","displayName":"LDC"},{"type":"usdt_trc20","upstreamType":"usdt","displayName":""}]')
  })

  it('returns an empty string for invalid or empty custom methods', () => {
    expect(parseEasyPayCustomMethods('not-json')).toEqual([])
    expect(serializeEasyPayCustomMethods([{ type: '', upstreamType: 'epay', displayName: 'LDC' }])).toBe('')
  })
})

describe('built-in payment method helpers', () => {
  it('only treats exact built-in aliases as Alipay or WeChat Pay', () => {
    expect(isBuiltInAlipayMethod('alipay')).toBe(true)
    expect(isBuiltInAlipayMethod('alipay_direct')).toBe(true)
    expect(isBuiltInAlipayMethod('card_alipay')).toBe(false)

    expect(isBuiltInWxpayMethod('wxpay')).toBe(true)
    expect(isBuiltInWxpayMethod('wxpay_direct')).toBe(true)
    expect(isBuiltInWxpayMethod('card_wxpay')).toBe(false)

    expect(isBuiltInQqpayMethod('qqpay')).toBe(true)
    expect(isBuiltInQqpayMethod('easypay_qqpay')).toBe(false)
  })
})

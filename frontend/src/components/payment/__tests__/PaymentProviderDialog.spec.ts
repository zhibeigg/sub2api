import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import PaymentProviderDialog from '@/components/payment/PaymentProviderDialog.vue'
import { STRIPE_SDK_API_VERSION } from '@/components/payment/providerConfig'
import type { ProviderInstance } from '@/types/payment'

const messages: Record<string, string> = {
  'admin.settings.payment.providerConfig': 'Credentials',
  'admin.settings.payment.easypayCustomMethods': 'Custom EasyPay methods',
  'admin.settings.payment.easypayCustomMethodsHint': 'Add provider-specific EasyPay type values.',
  'admin.settings.payment.addCustomMethod': 'Add method',
  'admin.settings.payment.customMethodType': 'Payment type',
  'admin.settings.payment.customMethodUpstreamType': 'Upstream type',
  'admin.settings.payment.customMethodDisplayName': 'Display name',
  'admin.settings.payment.paymentGuideTrigger': 'View payment guide',
  'admin.settings.payment.alipayGuideSummary': 'Desktop prefers QR precreate and falls back to cashier; mobile prefers WAP checkout.',
  'admin.settings.payment.wxpayGuideSummary': 'Desktop prefers Native QR; mobile routes to JSAPI or H5 based on browser context.',
  'admin.settings.payment.wxpayH5Enabled': 'H5 payment',
  'admin.settings.payment.wxpayJsapiEnabled': 'JSAPI payment',
  'admin.settings.payment.wxpayJsapiAuthType': 'JSAPI authentication type',
  'admin.settings.payment.wxpayJsapiAuthMp': 'WeChat Official Account',
  'admin.settings.payment.wxpayJsapiAuthWecom': 'WeCom custom app',
  'admin.settings.payment.wxpayWecomSuggested': 'WeCom mode is recommended for a ww-prefixed CorpID.',
  'admin.settings.payment.wxpayWecomSetupTitle': 'WeCom JSAPI configuration checklist',
  'admin.settings.payment.field_mpAppId': 'Official Account AppID',
  'admin.settings.payment.field_wecomAppSecret': 'WeCom App Secret',
  'admin.settings.payment.field_wecomAgentId': 'WeCom App AgentId',
  'admin.settings.payment.airwallexGuideSummary': 'Use Payment Acceptance read/write only.',
  'admin.settings.payment.stripeWebhookHint': 'Configure Stripe webhook.',
  'admin.settings.payment.stripeWebhookApiVersionHint': 'Use Stripe API version {version}.',
  'admin.settings.payment.airwallexWebhookHint': 'Select payment_intent.succeeded and use the latest stable API version.',
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, string>) => {
      const message = messages[key] ?? key
      if (!params) return message
      return Object.entries(params).reduce(
        (value, [name, replacement]) => value.replaceAll(`{${name}}`, replacement),
        message,
      )
    },
  }),
}))

function providerFactory(overrides: Partial<ProviderInstance> = {}): ProviderInstance {
  return {
    id: 1,
    provider_key: 'airwallex',
    name: 'Airwallex',
    config: {},
    supported_types: ['airwallex'],
    enabled: true,
    payment_mode: '',
    refund_enabled: false,
    allow_user_refund: false,
    limits: '',
    sort_order: 0,
    ...overrides,
  }
}

function mountDialog(options: { editing?: ProviderInstance | null } = {}) {
  return mount(PaymentProviderDialog, {
    props: {
      show: true,
      saving: false,
      editing: options.editing ?? null,
      allKeyOptions: [
        { value: 'easypay', label: 'EasyPay' },
        { value: 'alipay', label: 'Alipay' },
        { value: 'wxpay', label: 'WeChat Pay' },
        { value: 'stripe', label: 'Stripe' },
        { value: 'airwallex', label: 'Airwallex' },
      ],
      enabledKeyOptions: [
        { value: 'easypay', label: 'EasyPay' },
        { value: 'alipay', label: 'Alipay' },
        { value: 'wxpay', label: 'WeChat Pay' },
        { value: 'airwallex', label: 'Airwallex' },
      ],
      allPaymentTypes: [
        { value: 'alipay', label: 'Alipay' },
        { value: 'wxpay', label: 'WeChat Pay' },
      ],
      redirectLabel: 'Redirect',
    },
    global: {
      stubs: {
        BaseDialog: {
          template: '<div><slot /><slot name="footer" /></div>',
        },
        Select: {
          name: 'Select',
          props: ['modelValue', 'options', 'disabled'],
          emits: ['update:modelValue', 'change'],
          template: '<div class="select-stub" :data-value="modelValue" />',
        },
        ToggleSwitch: {
          name: 'ToggleSwitch',
          props: ['label', 'checked'],
          emits: ['toggle'],
          template: '<button type="button" class="toggle-stub" :data-label="label" :data-checked="checked" @click="$emit(\'toggle\')">{{ label }}</button>',
        },
      },
    },
  })
}

describe('PaymentProviderDialog payment guide', () => {
  it('shows no payment guide for providers without a flow guide', () => {
    const wrapper = mountDialog()

    expect(wrapper.text()).not.toContain(messages['admin.settings.payment.alipayGuideSummary'])
    expect(wrapper.text()).not.toContain(messages['admin.settings.payment.wxpayGuideSummary'])
    expect(wrapper.find('button[title="View payment guide"]').exists()).toBe(false)
  })

  it.each([
    ['alipay', 'admin.settings.payment.alipayGuideSummary'],
    ['wxpay', 'admin.settings.payment.wxpayGuideSummary'],
    ['airwallex', 'admin.settings.payment.airwallexGuideSummary'],
  ])('shows the payment guide summary for %s', async (providerKey, summaryKey) => {
    const wrapper = mountDialog()

    ;(wrapper.vm as unknown as { reset: (key: string) => void }).reset(providerKey)
    await nextTick()

    expect(wrapper.text()).toContain(messages[summaryKey])
    expect(wrapper.find('button[title="View payment guide"]').exists()).toBe(true)
  })

  it('shows Airwallex webhook event and API version guidance with the webhook URL', async () => {
    const wrapper = mountDialog()

    ;(wrapper.vm as unknown as { reset: (key: string) => void }).reset('airwallex')
    await nextTick()

    expect(wrapper.text()).toContain(messages['admin.settings.payment.airwallexWebhookHint'])
    expect(wrapper.text()).toContain('/api/v1/payment/webhook/airwallex')
  })

  it('shows Stripe webhook API version guidance with the integrated SDK version', async () => {
    const wrapper = mountDialog()

    ;(wrapper.vm as unknown as { reset: (key: string) => void }).reset('stripe')
    await nextTick()

    expect(wrapper.text()).toContain(messages['admin.settings.payment.stripeWebhookHint'])
    expect(wrapper.text()).toContain(`Use Stripe API version ${STRIPE_SDK_API_VERSION}.`)
    expect(wrapper.text()).toContain('/api/v1/payment/webhook/stripe')
  })

  it('emits an empty Airwallex accountId when the admin clears it', async () => {
    const provider = providerFactory({
      config: {
        clientId: 'cid_123',
        apiBase: 'https://api.airwallex.com/api/v1',
        countryCode: 'CN',
        currency: 'CNY',
        accountId: 'acct_123',
      },
    })
    const wrapper = mountDialog({ editing: provider })

    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()

    const accountIdInput = wrapper
      .findAll('input[type="text"]')
      .find(input => (input.element as HTMLInputElement).value === 'acct_123')
    if (!accountIdInput) throw new Error('accountId input not found')

    await accountIdInput.setValue('')
    await wrapper.find('form').trigger('submit.prevent')

    const payload = wrapper.emitted('save')?.[0]?.[0] as { config: Record<string, string> }
    expect(payload.config.accountId).toBe('')
  })

  it('infers historical H5 and JSAPI fields while saving explicit capability flags and mp auth mode', async () => {
    const provider = providerFactory({
      provider_key: 'wxpay',
      name: 'WeChat Pay',
      config: {
        appId: 'wx-app',
        mchId: 'mch-1',
        publicKeyId: 'PUB_KEY_ID_1',
        certSerial: 'CERT_1',
        h5AppName: 'Sub2API',
        h5AppUrl: 'https://pay.example.com',
        mpAppId: 'wx-mp-app',
      },
      supported_types: ['wxpay'],
      payment_mode: 'qrcode',
    })
    const wrapper = mountDialog({ editing: provider })

    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()
    await wrapper.find('form').trigger('submit.prevent')

    const payload = wrapper.emitted('save')?.[0]?.[0] as { config: Record<string, string> }
    expect(payload.config.nativeEnabled).toBe('true')
    expect(payload.config.h5Enabled).toBe('true')
    expect(payload.config.jsapiEnabled).toBe('true')
    expect(payload.config.jsapiAuthType).toBe('mp')
    expect(payload.config.h5AppName).toBe('Sub2API')
    expect(payload.config.h5AppUrl).toBe('https://pay.example.com')
    expect(payload.config.mpAppId).toBe('wx-mp-app')
    expect(payload.config).not.toHaveProperty('privateKey')
    expect(payload.config).not.toHaveProperty('publicKey')
    expect(payload.config).not.toHaveProperty('apiV3Key')
  })

  it('keeps explicit disabled wxpay capabilities when historical fields remain', async () => {
    const provider = providerFactory({
      provider_key: 'wxpay',
      name: 'WeChat Pay',
      config: {
        appId: 'wx-app',
        mchId: 'mch-1',
        publicKeyId: 'PUB_KEY_ID_1',
        certSerial: 'CERT_1',
        h5AppName: 'Sub2API',
        h5AppUrl: 'https://pay.example.com',
        mpAppId: 'wx-mp-app',
        nativeEnabled: 'true',
        h5Enabled: 'false',
        jsapiEnabled: 'false',
      },
      supported_types: ['wxpay'],
      payment_mode: 'qrcode',
    })
    const wrapper = mountDialog({ editing: provider })

    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()
    await wrapper.find('form').trigger('submit.prevent')

    const payload = wrapper.emitted('save')?.[0]?.[0] as { config: Record<string, string> }
    expect(payload.config.nativeEnabled).toBe('true')
    expect(payload.config.h5Enabled).toBe('false')
    expect(payload.config.jsapiEnabled).toBe('false')
    expect(payload.config.jsapiAuthType).toBe('mp')
    expect(wrapper.text()).not.toContain(messages['admin.settings.payment.wxpayJsapiAuthType'])
    expect(wrapper.text()).not.toContain(messages['admin.settings.payment.field_mpAppId'])
  })

  it('keeps new wxpay H5 and JSAPI disabled, then reveals mp mode only after JSAPI is enabled', async () => {
    const wrapper = mountDialog()

    ;(wrapper.vm as unknown as { reset: (key: string) => void }).reset('wxpay')
    await nextTick()

    const h5Toggle = wrapper.find(`button[data-label="${messages['admin.settings.payment.wxpayH5Enabled']}"]`)
    const jsapiToggle = wrapper.find(`button[data-label="${messages['admin.settings.payment.wxpayJsapiEnabled']}"]`)
    expect(h5Toggle.attributes('data-checked')).toBe('false')
    expect(jsapiToggle.attributes('data-checked')).toBe('false')
    expect(wrapper.text()).not.toContain(messages['admin.settings.payment.wxpayJsapiAuthType'])
    expect(wrapper.text()).not.toContain(messages['admin.settings.payment.field_mpAppId'])

    await jsapiToggle.trigger('click')
    await nextTick()

    expect(wrapper.text()).toContain(messages['admin.settings.payment.wxpayJsapiAuthType'])
    expect(wrapper.text()).toContain(messages['admin.settings.payment.field_mpAppId'])
    expect(wrapper.text()).not.toContain(messages['admin.settings.payment.field_wecomAppSecret'])
    const authSelect = wrapper.findAllComponents({ name: 'Select' })
      .find(select => select.props('modelValue') === 'mp')
    expect(authSelect?.props('options')).toEqual([
      { value: 'mp', label: messages['admin.settings.payment.wxpayJsapiAuthMp'] },
      { value: 'wecom', label: messages['admin.settings.payment.wxpayJsapiAuthWecom'] },
    ])
  })

  it('suggests WeCom for ww-prefixed historical providers without silently changing mp mode', async () => {
    const provider = providerFactory({
      provider_key: 'wxpay',
      name: 'WeChat Pay',
      config: {
        appId: 'ww-corp-id',
        mchId: 'mch-1',
        publicKeyId: 'PUB_KEY_ID_1',
        certSerial: 'CERT_1',
        mpAppId: 'wx-official-account',
        jsapiEnabled: 'true',
      },
      supported_types: ['wxpay'],
    })
    const wrapper = mountDialog({ editing: provider })

    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()

    const authSelect = wrapper.findAllComponents({ name: 'Select' })
      .find(select => select.props('modelValue') === 'mp')
    expect(authSelect).toBeDefined()
    expect(wrapper.text()).toContain(messages['admin.settings.payment.wxpayWecomSuggested'])
    expect(wrapper.text()).toContain(messages['admin.settings.payment.field_mpAppId'])
    expect(wrapper.text()).not.toContain(messages['admin.settings.payment.field_wecomAppSecret'])
  })

  it('shows WeCom fields and preserves a blank stored app secret while editing', async () => {
    const provider = providerFactory({
      provider_key: 'wxpay',
      name: 'WeCom Pay',
      config: {
        appId: 'ww-corp-id',
        mchId: 'mch-1',
        publicKeyId: 'PUB_KEY_ID_1',
        certSerial: 'CERT_1',
        jsapiEnabled: 'true',
        jsapiAuthType: 'wecom',
        wecomAgentId: '1000002',
      },
      supported_types: ['wxpay'],
    })
    const wrapper = mountDialog({ editing: provider })

    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()

    expect(wrapper.text()).toContain(messages['admin.settings.payment.field_wecomAppSecret'])
    expect(wrapper.text()).toContain(messages['admin.settings.payment.field_wecomAgentId'])
    expect(wrapper.text()).toContain(messages['admin.settings.payment.wxpayWecomSetupTitle'])
    expect(wrapper.text()).not.toContain(messages['admin.settings.payment.field_mpAppId'])
    const passwordInputs = wrapper.findAll('input[type="password"]')
    expect(passwordInputs).toHaveLength(2)
    expect(passwordInputs.every(input => input.attributes('placeholder') === 'admin.accounts.leaveEmptyToKeep')).toBe(true)

    await wrapper.find('form').trigger('submit.prevent')

    const payload = wrapper.emitted('save')?.[0]?.[0] as { config: Record<string, string> }
    expect(payload.config.jsapiAuthType).toBe('wecom')
    expect(payload.config.wecomAgentId).toBe('1000002')
    expect(payload.config).not.toHaveProperty('wecomAppSecret')
    expect(payload.config).not.toHaveProperty('mpAppId')
  })

  it.each([
    {
      name: 'mp mode with only a WeCom CorpID',
      config: {
        appId: 'ww-corp-id',
        mchId: 'mch-1',
        publicKeyId: 'PUB_KEY_ID_1',
        certSerial: 'CERT_1',
        jsapiEnabled: 'true',
        jsapiAuthType: 'mp',
      },
    },
    {
      name: 'WeCom mode with a non-numeric AgentId',
      config: {
        appId: 'ww-corp-id',
        mchId: 'mch-1',
        publicKeyId: 'PUB_KEY_ID_1',
        certSerial: 'CERT_1',
        jsapiEnabled: 'true',
        jsapiAuthType: 'wecom',
        wecomAgentId: 'agent-1',
      },
    },
  ])('blocks the clear invalid JSAPI combination: $name', async ({ config }) => {
    const provider = providerFactory({
      provider_key: 'wxpay',
      name: 'Invalid JSAPI configuration',
      config,
      supported_types: ['wxpay'],
    })
    const wrapper = mountDialog({ editing: provider })

    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()
    await wrapper.find('form').trigger('submit.prevent')

    expect(wrapper.emitted('save')).toBeUndefined()
  })

  it('defaults new EasyPay providers to V2 with QQ Wallet and V2 key fields', async () => {
    const wrapper = mountDialog()

    ;(wrapper.vm as unknown as { reset: (key: string) => void }).reset('easypay')
    await nextTick()

    expect(wrapper.text()).toContain('admin.settings.payment.field_merchantPrivateKey')
    expect(wrapper.text()).toContain('admin.settings.payment.field_platformPublicKey')
    expect(wrapper.text()).not.toContain('PKey')
    expect(wrapper.text()).toContain('payment.methods.qqpay')

    const protocolSelect = wrapper.findAllComponents({ name: 'Select' })
      .find(select => select.props('modelValue') === '2')
    expect(protocolSelect?.props('disabled')).toBe(false)
  })

  it('omits blank V2 sensitive fields while preserving the selected protocol on edit', async () => {
    const provider = providerFactory({
      provider_key: 'easypay',
      name: 'EasyPay V2',
      config: {
        protocolVersion: '2',
        pid: 'pid-v2',
        apiBase: 'https://pay.example.com',
        notifyUrl: 'https://example.com/api/v1/payment/webhook/easypay',
        returnUrl: 'https://example.com/payment/result',
      },
      supported_types: ['alipay', 'wxpay', 'qqpay'],
      payment_mode: 'qrcode',
    })
    const wrapper = mountDialog({ editing: provider })

    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()
    await wrapper.find('form').trigger('submit.prevent')

    const payload = wrapper.emitted('save')?.[0]?.[0] as { config: Record<string, string> }
    expect(payload.config.protocolVersion).toBe('2')
    expect(payload.config).not.toHaveProperty('merchantPrivateKey')
    expect(payload.config).not.toHaveProperty('platformPublicKey')
  })

  it('loads historical EasyPay providers as locked V1 and keeps custom qqpay', async () => {
    const provider = providerFactory({
      provider_key: 'easypay',
      name: 'Legacy EasyPay',
      config: {
        pid: 'pid-legacy',
        apiBase: 'https://pay.example.com',
        customMethods: '[{"type":"qqpay","upstreamType":"qqpay","displayName":"QQ Wallet"}]',
        notifyUrl: 'https://example.com/api/v1/payment/webhook/easypay',
        returnUrl: 'https://example.com/payment/result',
      },
      supported_types: ['alipay', 'wxpay', 'qqpay'],
      payment_mode: 'qrcode',
    })
    const wrapper = mountDialog({ editing: provider })

    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()

    expect(wrapper.text()).toContain('PKey')
    expect(wrapper.text()).not.toContain('admin.settings.payment.field_merchantPrivateKey')
    expect(wrapper.text()).toContain('QQ Wallet')
    const protocolSelect = wrapper.findAllComponents({ name: 'Select' })
      .find(select => select.props('modelValue') === '1')
    expect(protocolSelect?.props('disabled')).toBe(true)

    await wrapper.find('form').trigger('submit.prevent')
    const payload = wrapper.emitted('save')?.[0]?.[0] as {
      config: Record<string, string>
      supported_types: string[]
    }
    expect(payload.config.protocolVersion).toBe('1')
    expect(payload.config.customMethods).toContain('"type":"qqpay"')
    expect(payload.supported_types).toContain('qqpay')
  })

  it('serializes EasyPay custom methods and adds them to supported_types', async () => {
    const provider = providerFactory({
      provider_key: 'easypay',
      name: 'EasyPay',
      config: {
        pid: 'pid-1',
        apiBase: 'https://pay.example.com',
        notifyUrl: 'https://example.com/api/v1/payment/webhook/easypay',
        returnUrl: 'https://example.com/payment/result',
      },
      supported_types: ['alipay', 'wxpay'],
      payment_mode: 'qrcode',
    })
    const wrapper = mountDialog({ editing: provider })

    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()

    await wrapper.find('button.btn-sm').trigger('click')
    await nextTick()

    const inputs = wrapper.findAll('input[type="text"]')
    const customTypeInputs = inputs.filter(input => (input.element as HTMLInputElement).placeholder === 'credit_card')
    const ldcTypeInput = customTypeInputs[0]
    const upstreamTypeInput = customTypeInputs[1]
    const displayNameInput = inputs.find(input => (input.element as HTMLInputElement).placeholder === '信用卡')
    if (!ldcTypeInput || !upstreamTypeInput || !displayNameInput) {
      throw new Error('custom method inputs not found')
    }

    await ldcTypeInput.setValue('ldc')
    await upstreamTypeInput.setValue('epay')
    await displayNameInput.setValue('LDC')
    await wrapper.find('form').trigger('submit.prevent')

    const payload = wrapper.emitted('save')?.[0]?.[0] as {
      config: Record<string, string>
      supported_types: string[]
    }
    expect(payload.config.customMethods).toBe('[{"type":"ldc","upstreamType":"epay","displayName":"LDC"}]')
    expect(payload.supported_types).toEqual(['alipay', 'wxpay', 'ldc'])
  })

  it('rejects custom EasyPay method types with built-in payment prefixes', async () => {
    const provider = providerFactory({
      provider_key: 'easypay',
      name: 'EasyPay',
      config: {
        pid: 'pid-1',
        apiBase: 'https://pay.example.com',
        notifyUrl: 'https://example.com/api/v1/payment/webhook/easypay',
        returnUrl: 'https://example.com/payment/result',
      },
      supported_types: ['alipay', 'wxpay'],
      payment_mode: 'qrcode',
    })
    const wrapper = mountDialog({ editing: provider })

    ;(wrapper.vm as unknown as { loadProvider: (provider: ProviderInstance) => void }).loadProvider(provider)
    await nextTick()

    await wrapper.find('button.btn-sm').trigger('click')
    await nextTick()

    const inputs = wrapper.findAll('input[type="text"]')
    const customTypeInputs = inputs.filter(input => (input.element as HTMLInputElement).placeholder === 'credit_card')
    const typeInput = customTypeInputs[0]
    const upstreamTypeInput = customTypeInputs[1]
    const displayNameInput = inputs.find(input => (input.element as HTMLInputElement).placeholder === '信用卡')
    if (!typeInput || !upstreamTypeInput || !displayNameInput) {
      throw new Error('custom method inputs not found')
    }

    await typeInput.setValue('alipay_hk')
    await upstreamTypeInput.setValue('hkpay')
    await displayNameInput.setValue('Hong Kong Alipay')
    await wrapper.find('form').trigger('submit.prevent')

    expect(wrapper.emitted('save')).toBeUndefined()
  })
})

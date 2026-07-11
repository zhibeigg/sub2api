import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { mount } from '@vue/test-utils'

const {
  createAccountMock,
  validateCredentialsMock,
  showErrorMock,
  makeOAuthMock,
  makeRef
} = vi.hoisted(() => {
  const makeRef = <T>(value: T) => ({ __v_isRef: true, value })
  return {
    createAccountMock: vi.fn(),
    validateCredentialsMock: vi.fn(),
    showErrorMock: vi.fn(),
    makeRef,
    makeOAuthMock: () => ({
      authUrl: { value: '' },
      sessionId: { value: '' },
      state: { value: '' },
      loading: { value: false },
      error: { value: '' },
      resetState: vi.fn(),
      generateAuthUrl: vi.fn(),
      validateRefreshToken: vi.fn(),
      exchangeAuthCode: vi.fn(),
      buildCredentials: vi.fn(() => ({})),
      buildExtraInfo: vi.fn(() => ({})),
      parseSessionKeys: vi.fn(() => [])
    })
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: showErrorMock,
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
    showWarning: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isSimpleMode: false })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      create: createAccountMock,
      validateCredentials: validateCredentialsMock,
      checkMixedChannelRisk: vi.fn().mockResolvedValue({ has_risk: false })
    },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] })
    },
    tlsFingerprintProfiles: {
      list: vi.fn().mockResolvedValue([])
    }
  }
}))

vi.mock('@/api/admin/accounts', () => ({
  accountsAPI: { syncUpstreamModelsPreview: vi.fn() },
  getAntigravityDefaultModelMapping: vi.fn().mockResolvedValue([])
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

vi.mock('@/composables/useAccountOAuth', () => ({ useAccountOAuth: makeOAuthMock }))
vi.mock('@/composables/useOpenAIOAuth', () => ({ useOpenAIOAuth: makeOAuthMock }))
vi.mock('@/composables/useGeminiOAuth', () => ({
  useGeminiOAuth: () => ({ ...makeOAuthMock(), getCapabilities: vi.fn().mockResolvedValue({ ai_studio_oauth_enabled: false }) })
}))
vi.mock('@/composables/useAntigravityOAuth', () => ({ useAntigravityOAuth: makeOAuthMock }))
vi.mock('@/composables/useGrokOAuth', () => ({ useGrokOAuth: makeOAuthMock }))
vi.mock('@/composables/useKiroOAuth', () => ({
  useKiroOAuth: () => ({
    loading: makeRef(false),
    polling: makeRef(false),
    userCode: makeRef(''),
    verificationUri: makeRef(''),
    authUrl: makeRef(''),
    ssoSessionId: makeRef(''),
    resetState: vi.fn(),
    startBuilderID: vi.fn(),
    pollBuilderID: vi.fn(),
    startIAMSSO: vi.fn(),
    completeIAMSSO: vi.fn(),
    importSSOToken: vi.fn()
  })
}))
vi.mock('@/composables/useQuotaNotifyState', () => ({
  useQuotaNotifyState: () => ({
    globalEnabled: makeRef(false),
    state: makeRef({
      daily: { enabled: false, threshold: null, thresholdType: 'percent' },
      weekly: { enabled: false, threshold: null, thresholdType: 'percent' },
      total: { enabled: false, threshold: null, thresholdType: 'percent' }
    }),
    loadGlobalState: vi.fn(),
    writeToExtra: vi.fn()
  })
}))

import CreateAccountModal from '../CreateAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show" data-testid="dialog"><slot /><slot name="footer" /></div>'
})

const PlainStub = defineComponent({
  name: 'PlainStub',
  template: '<div><slot /></div>'
})

const ModelWhitelistSelectorStub = defineComponent({
  name: 'ModelWhitelistSelector',
  props: {
    modelValue: { type: Array, default: () => [] },
    platform: { type: String, default: '' },
    syncCredentials: { type: Object, default: undefined }
  },
  emits: ['update:modelValue'],
  template: `
    <div data-testid="model-whitelist-selector">
      <span data-testid="model-sync-platform">{{ platform }}</span>
      <span data-testid="model-sync-ready">{{ syncCredentials ? 'ready' : 'missing' }}</span>
      <button type="button" data-testid="set-cursor-models" @click="$emit('update:modelValue', ['claude-sonnet-4-6'])">
        sync models
      </button>
    </div>
  `
})

const mountModal = () => mount(CreateAccountModal, {
  props: { show: true, proxies: [], groups: [] },
  global: {
    stubs: {
      BaseDialog: BaseDialogStub,
      ConfirmDialog: PlainStub,
      Select: PlainStub,
      PlatformIcon: PlainStub,
      Icon: PlainStub,
      ProxySelector: PlainStub,
      ProxyAdBanner: PlainStub,
      GroupSelector: PlainStub,
      ModelWhitelistSelector: ModelWhitelistSelectorStub,
      QuotaLimitCard: PlainStub,
      OAuthAuthorizationFlow: PlainStub
    }
  }
})

const selectPlatform = async (wrapper: ReturnType<typeof mountModal>, platform: 'Adobe' | 'Cursor') => {
  const button = wrapper.findAll('button').find(item => item.text().includes(platform))
  expect(button).toBeTruthy()
  await button!.trigger('click')
  await nextTick()
}

const enterNameAndContinue = async (wrapper: ReturnType<typeof mountModal>) => {
  await wrapper.get('[data-tour="account-form-name"]').setValue('test-account')
  await wrapper.get('#create-account-form').trigger('submit')
  await nextTick()
}

beforeEach(() => {
  vi.clearAllMocks()
  createAccountMock.mockResolvedValue({ id: 1 })
})

describe('CreateAccountModal Adobe/Cursor credential validation flow', () => {
  it.each([
    ['Adobe', '[data-testid="adobe-credentials-form"]', 'admin.accounts.credentialsValidation.adobeStepTitle'],
    ['Cursor', '[data-testid="cursor-credentials-form"]', 'admin.accounts.credentialsValidation.cursorStepTitle']
  ] as const)('hides %s credentials on step 1 and shows them after continuing', async (platform, selector, stepTitle) => {
    const wrapper = mountModal()
    await selectPlatform(wrapper, platform)

    expect(wrapper.find(selector).exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.accounts.credentialsValidation.basicConfiguration')
    expect(wrapper.text()).toContain(stepTitle)
    expect(wrapper.text()).toContain('common.next')

    await enterNameAndContinue(wrapper)
    expect(wrapper.find(selector).exists()).toBe(true)
    expect(wrapper.get('[data-testid="validate-and-create-button"]').attributes()).toMatchObject({
      type: 'submit',
      form: 'credential-validation-form'
    })
  })

  it('renders Cursor as a dedicated Cloud Agents API Key flow without duplicated generic API Key fields', async () => {
    const wrapper = mountModal()
    await selectPlatform(wrapper, 'Cursor')

    expect(wrapper.find('[data-testid="cursor-account-type"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="credential-model-restriction"]')).toHaveLength(0)
    expect(wrapper.find('[data-testid="mixed-scheduling-checkbox"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('admin.accounts.cursor.cloudAgentsApiKey')
    expect(wrapper.text()).not.toContain('admin.accounts.mapRequestModels')
    expect(wrapper.text()).not.toContain('admin.accounts.baseUrl')
    expect(wrapper.text()).not.toContain('admin.accounts.apiKeyHint')
  })

  it('uses the Cursor API Key for upstream model sync and preserves synchronized models', async () => {
    validateCredentialsMock.mockResolvedValue({ success: true, platform: 'cursor', message: 'ok' })
    const wrapper = mountModal()
    await selectPlatform(wrapper, 'Cursor')
    await enterNameAndContinue(wrapper)

    await wrapper.get('[data-testid="cursor-api-key-input"]').setValue('cursor-api-key')
    await nextTick()
    expect(wrapper.get('[data-testid="model-sync-platform"]').text()).toBe('cursor')
    expect(wrapper.get('[data-testid="model-sync-ready"]').text()).toBe('ready')
    expect(wrapper.findComponent(ModelWhitelistSelectorStub).props('syncCredentials')).toEqual({
      platform: 'cursor',
      type: 'apikey',
      api_key: 'cursor-api-key'
    })
    await wrapper.get('[data-testid="set-cursor-models"]').trigger('click')
    await nextTick()
    await wrapper.get('#credential-validation-form').trigger('submit')

    expect(validateCredentialsMock).toHaveBeenCalledWith(expect.objectContaining({
      credentials: expect.objectContaining({
        api_key: 'cursor-api-key',
        model_mapping: { 'claude-sonnet-4-6': 'claude-sonnet-4-6' }
      })
    }))
    expect(createAccountMock).toHaveBeenCalledWith(expect.objectContaining({
      credentials: expect.objectContaining({
        api_key: 'cursor-api-key',
        model_mapping: { 'claude-sonnet-4-6': 'claude-sonnet-4-6' }
      })
    }))
  })

  it('persists the Cursor /v1/messages scheduling switch when creating the account', async () => {
    validateCredentialsMock.mockResolvedValue({ success: true, platform: 'cursor', message: 'ok' })
    const wrapper = mountModal()
    await selectPlatform(wrapper, 'Cursor')
    await wrapper.get('[data-testid="mixed-scheduling-checkbox"]').setValue(true)
    await enterNameAndContinue(wrapper)
    await wrapper.get('[data-testid="cursor-api-key-input"]').setValue('cursor-api-key')
    await wrapper.get('#credential-validation-form').trigger('submit')

    expect(createAccountMock).toHaveBeenCalledWith(expect.objectContaining({
      platform: 'cursor',
      extra: { mixed_scheduling: true }
    }))
  })

  it('does not create a Cursor account when credential validation fails', async () => {
    validateCredentialsMock.mockResolvedValue({ success: false, platform: 'cursor', message: 'invalid' })
    const wrapper = mountModal()
    await selectPlatform(wrapper, 'Cursor')
    await enterNameAndContinue(wrapper)
    await wrapper.get('[data-testid="cursor-api-key-input"]').setValue('cursor-api-key')
    await wrapper.get('#credential-validation-form').trigger('submit')

    expect(validateCredentialsMock).toHaveBeenCalledOnce()
    expect(createAccountMock).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="cursor-credentials-form"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('cursor-api-key')
  })

  it('creates an Adobe account only after successful validation with the same credentials', async () => {
    validateCredentialsMock.mockResolvedValue({
      success: true,
      platform: 'adobe',
      message: 'ok',
      email: 'safe@example.com'
    })
    const wrapper = mountModal()
    await selectPlatform(wrapper, 'Adobe')
    await enterNameAndContinue(wrapper)
    await wrapper.get('[data-testid="adobe-access-token-input"]').setValue('secret-token')
    await wrapper.get('#credential-validation-form').trigger('submit')

    expect(validateCredentialsMock).toHaveBeenCalledOnce()
    expect(createAccountMock).toHaveBeenCalledOnce()
    const validationPayload = validateCredentialsMock.mock.calls[0][0]
    const createPayload = createAccountMock.mock.calls[0][0]
    expect(validationPayload.type).toBe('oauth')
    expect(createPayload.type).toBe('oauth')
    expect(createPayload.credentials).toEqual(validationPayload.credentials)
  })

  it('validates and creates Cursor as an API Key account with concurrency 1', async () => {
    validateCredentialsMock.mockResolvedValue({ success: true, platform: 'cursor', message: 'ok' })
    const wrapper = mountModal()
    await selectPlatform(wrapper, 'Cursor')
    await enterNameAndContinue(wrapper)
    await wrapper.get('[data-testid="cursor-api-key-input"]').setValue('cursor-api-key')
    await wrapper.get('#credential-validation-form').trigger('submit')

    expect(validateCredentialsMock).toHaveBeenCalledWith(expect.objectContaining({
      platform: 'cursor',
      type: 'apikey',
      credentials: expect.objectContaining({ api_key: 'cursor-api-key' })
    }))
    expect(createAccountMock).toHaveBeenCalledWith(expect.objectContaining({
      platform: 'cursor',
      type: 'apikey',
      concurrency: 1,
      credentials: expect.objectContaining({ api_key: 'cursor-api-key' })
    }))
  })

  it('persists optional Cursor Dashboard tokens with the validated account', async () => {
    validateCredentialsMock.mockResolvedValue({ success: true, platform: 'cursor', message: 'ok' })
    const wrapper = mountModal()
    await selectPlatform(wrapper, 'Cursor')
    await enterNameAndContinue(wrapper)
    await wrapper.get('[data-testid="cursor-api-key-input"]').setValue('cursor-api-key')
    await wrapper.get('#cursor-dashboard-access-token').setValue('dashboard-access')
    await wrapper.get('#cursor-dashboard-refresh-token').setValue('dashboard-refresh')
    await wrapper.get('#credential-validation-form').trigger('submit')

    expect(createAccountMock).toHaveBeenCalledWith(expect.objectContaining({
      credentials: expect.objectContaining({
        api_key: 'cursor-api-key',
        dashboard_access_token: 'dashboard-access',
        dashboard_refresh_token: 'dashboard-refresh'
      })
    }))
  })

  it('requires a Cursor API Key before validation', async () => {
    const wrapper = mountModal()
    await selectPlatform(wrapper, 'Cursor')
    await enterNameAndContinue(wrapper)

    await wrapper.get('#credential-validation-form').trigger('submit')

    expect(validateCredentialsMock).not.toHaveBeenCalled()
    expect(createAccountMock).not.toHaveBeenCalled()
    expect(showErrorMock).toHaveBeenCalledWith('admin.accounts.cursor.apiKeyRequired')
  })

  it('preserves sensitive input when returning to step 1 and showing step 2 again', async () => {
    const wrapper = mountModal()
    await selectPlatform(wrapper, 'Cursor')
    await enterNameAndContinue(wrapper)
    await wrapper.get('[data-testid="cursor-api-key-input"]').setValue('cursor-api-key')

    const backButton = wrapper.findAll('button').find(item => item.text().includes('common.back'))
    expect(backButton).toBeTruthy()
    await backButton!.trigger('click')
    await wrapper.get('#create-account-form').trigger('submit')
    await nextTick()

    expect((wrapper.get('[data-testid="cursor-api-key-input"]').element as HTMLInputElement).value).toBe('cursor-api-key')
    expect(wrapper.text()).not.toContain('cursor-api-key')
  })
})

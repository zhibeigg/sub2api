import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const {
  createAccountMock,
  validateCredentialsMock,
  showErrorMock,
  makeOAuthMock,
  makeRef,
  cursorBrowserLoginMock
} = vi.hoisted(() => {
  const makeRef = <T>(value: T) => ({ __v_isRef: true, value })
  return {
    createAccountMock: vi.fn(),
    validateCredentialsMock: vi.fn(),
    showErrorMock: vi.fn(),
    makeRef,
    cursorBrowserLoginMock: {
      state: makeRef<'ready' | 'unavailable' | 'starting' | 'waiting_for_login' | 'reading_cookie' | 'received' | 'error'>('ready'),
      available: makeRef(true),
      busy: makeRef(false),
      extensionVersion: makeRef('0.34.5'),
      errorCode: makeRef<string | null>(null),
      initialize: vi.fn(),
      ping: vi.fn(),
      start: vi.fn(),
      cancel: vi.fn(),
      dispose: vi.fn()
    },
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
vi.mock('@/composables/useCursorBrowserLogin', () => ({
  CURSOR_EXTENSION_DOWNLOAD_URL: '/downloads/cursor-cookie-importer.zip',
  CursorBrowserLoginError: class CursorBrowserLoginError extends Error {
    constructor(public readonly code: string) { super(code) }
  },
  useCursorBrowserLogin: () => cursorBrowserLoginMock
}))
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
      ModelWhitelistSelector: PlainStub,
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
  cursorBrowserLoginMock.state.value = 'ready'
  cursorBrowserLoginMock.available.value = true
  cursorBrowserLoginMock.busy.value = false
  cursorBrowserLoginMock.extensionVersion.value = '0.34.5'
  cursorBrowserLoginMock.errorCode.value = null
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

  it('does not create a Cursor account when credential validation fails', async () => {
    validateCredentialsMock.mockResolvedValue({ success: false, platform: 'cursor', message: 'invalid' })
    const wrapper = mountModal()
    await selectPlatform(wrapper, 'Cursor')
    await enterNameAndContinue(wrapper)
    await wrapper.get('[data-testid="cursor-cookie-input"]').setValue('secret-cookie')
    await wrapper.get('#credential-validation-form').trigger('submit')

    expect(validateCredentialsMock).toHaveBeenCalledOnce()
    expect(createAccountMock).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="cursor-credentials-form"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('secret-cookie')
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

  it('creates Cursor with cookie type and concurrency 1 after successful validation', async () => {
    validateCredentialsMock.mockResolvedValue({ success: true, platform: 'cursor', message: 'ok' })
    const wrapper = mountModal()
    await selectPlatform(wrapper, 'Cursor')
    await enterNameAndContinue(wrapper)
    await wrapper.get('[data-testid="cursor-cookie-input"]').setValue('secret-cookie')
    await wrapper.get('#credential-validation-form').trigger('submit')

    expect(createAccountMock).toHaveBeenCalledWith(expect.objectContaining({
      platform: 'cursor',
      type: 'cookie',
      concurrency: 1
    }))
  })

  it('imports only _vcrcs through the extension and automatically validates before creating', async () => {
    validateCredentialsMock.mockResolvedValue({ success: true, platform: 'cursor', message: 'ok' })
    cursorBrowserLoginMock.start.mockResolvedValue({ value: 'browser-secret', expirationDate: 1_800_000_000 })
    const wrapper = mountModal()
    await selectPlatform(wrapper, 'Cursor')
    await enterNameAndContinue(wrapper)

    await wrapper.get('[data-testid="cursor-browser-login-button"]').trigger('click')
    await flushPromises()

    expect(cursorBrowserLoginMock.start).toHaveBeenCalledOnce()
    expect(validateCredentialsMock).toHaveBeenCalledWith(expect.objectContaining({
      platform: 'cursor',
      type: 'cookie',
      credentials: expect.objectContaining({ cookie: '_vcrcs=browser-secret' })
    }))
    expect(createAccountMock).toHaveBeenCalledOnce()
    expect(wrapper.text()).not.toContain('browser-secret')
  })

  it('offers the bundled extension download and manual fallback when the helper is unavailable', async () => {
    cursorBrowserLoginMock.state.value = 'unavailable'
    cursorBrowserLoginMock.available.value = false
    const wrapper = mountModal()
    await selectPlatform(wrapper, 'Cursor')
    await enterNameAndContinue(wrapper)

    expect(wrapper.get('[data-testid="cursor-extension-download"]').attributes('href')).toBe('/downloads/cursor-cookie-importer.zip')
    expect(wrapper.get('[data-testid="cursor-manual-import"]').attributes()).toHaveProperty('open')
    expect(wrapper.get('[data-testid="cursor-cookie-input"]').exists()).toBe(true)
  })

  it('keeps the internal model inside advanced settings with an empty safe default', async () => {
    const wrapper = mountModal()
    await selectPlatform(wrapper, 'Cursor')
    await enterNameAndContinue(wrapper)

    expect(wrapper.get('[data-testid="cursor-advanced-settings"]').text()).toContain('admin.accounts.cursor.advancedSettings')
    expect((wrapper.get('#cursor-upstream-model').element as HTMLInputElement).value).toBe('')
    expect(wrapper.text()).not.toContain('claude-sonnet-4-5')
  })

  it('preserves sensitive input when returning to step 1 and showing step 2 again', async () => {
    const wrapper = mountModal()
    await selectPlatform(wrapper, 'Cursor')
    await enterNameAndContinue(wrapper)
    await wrapper.get('[data-testid="cursor-cookie-input"]').setValue('secret-cookie')

    const backButton = wrapper.findAll('button').find(item => item.text().includes('common.back'))
    expect(backButton).toBeTruthy()
    await backButton!.trigger('click')
    await wrapper.get('#create-account-form').trigger('submit')
    await nextTick()

    expect((wrapper.get('[data-testid="cursor-cookie-input"]').element as HTMLInputElement).value).toBe('secret-cookie')
    expect(wrapper.text()).not.toContain('secret-cookie')
  })
})

import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick, ref } from 'vue'
import { mount } from '@vue/test-utils'

const { createAccountMock, makeOAuthMock, makeRef } = vi.hoisted(() => ({
  createAccountMock: vi.fn(),
  makeRef: <T>(value: T) => ({ __v_isRef: true, value }),
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
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
    showWarning: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    isSimpleMode: false
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      create: createAccountMock,
      validateCredentials: vi.fn(),
      checkMixedChannelRisk: vi.fn().mockResolvedValue({ needs_warning: false })
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
  accountsAPI: {
    syncUpstreamModelsPreview: vi.fn()
  },
  getAntigravityDefaultModelMapping: vi.fn().mockResolvedValue([])
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key
    })
  }
})

vi.mock('@/composables/useAccountOAuth', () => ({
  useAccountOAuth: makeOAuthMock
}))

vi.mock('@/composables/useOpenAIOAuth', () => ({
  useOpenAIOAuth: makeOAuthMock
}))

vi.mock('@/composables/useGeminiOAuth', () => ({
  useGeminiOAuth: () => ({
    ...makeOAuthMock(),
    getCapabilities: vi.fn().mockResolvedValue({ ai_studio_oauth_enabled: false })
  })
}))

vi.mock('@/composables/useAntigravityOAuth', () => ({
  useAntigravityOAuth: makeOAuthMock
}))

vi.mock('@/composables/useGrokOAuth', () => ({
  useGrokOAuth: makeOAuthMock
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
  props: {
    show: {
      type: Boolean,
      default: false
    }
  },
  template: '<div v-if="show" data-testid="dialog"><slot /><slot name="footer" /></div>'
})

const PlainStub = defineComponent({
  name: 'PlainStub',
  template: '<div><slot /></div>'
})

afterEach(() => {
  document.body.innerHTML = ''
  document.body.classList.remove('modal-open')
})

describe('CreateAccountModal Kiro platform', () => {
  it('shows the two-step wizard for Kiro without closing the modal', async () => {
    const wrapper = mount(CreateAccountModal, {
      props: {
        show: true,
        proxies: [],
        groups: []
      },
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

    const kiroButton = wrapper.findAll('button').find(button => button.text().includes('Kiro'))
    expect(kiroButton).toBeTruthy()

    await kiroButton!.trigger('click')
    await nextTick()

    // 选中 Kiro 后仍在第一步：显示步骤指示器与"添加方式"选择，尚未显示凭证输入
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(wrapper.find('[data-testid="dialog"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('admin.accounts.oauth.authMethod')
    expect(wrapper.text()).toContain('admin.accounts.oauth.kiro.method')
    expect(wrapper.find('[data-testid="kiro-credentials-input"]').exists()).toBe(false)
  })

  it('keeps the real dialog mounted and the page locked only while Kiro form is open', async () => {
    const wrapper = mount(CreateAccountModal, {
      attachTo: document.body,
      props: {
        show: true,
        proxies: [],
        groups: []
      },
      global: {
        stubs: {
          Transition: false
        }
      }
    })

    expect(document.body.classList.contains('modal-open')).toBe(true)

    const kiroButton = Array.from(document.body.querySelectorAll('button')).find(button =>
      button.textContent?.includes('Kiro')
    )
    expect(kiroButton).toBeTruthy()

    kiroButton!.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))
    await nextTick()

    expect(wrapper.emitted('close')).toBeUndefined()
    expect(document.body.querySelector('.modal-overlay')).toBeTruthy()
    // 两步流程：选中 Kiro 后仍在第一步（凭证输入在第二步才出现），弹窗保持挂载与锁定
    expect(document.body.querySelector('[data-testid="kiro-credentials-input"]')).toBeFalsy()
    expect(document.body.textContent).toContain('admin.accounts.oauth.kiro.method')
    expect(document.body.classList.contains('modal-open')).toBe(true)

    await wrapper.setProps({ show: false })
    await nextTick()
    wrapper.unmount()
    expect(document.body.classList.contains('modal-open')).toBe(false)
  })

  it('does not ask a parent-controlled modal to close when selecting Kiro', async () => {
    const ParentHarness = defineComponent({
      components: { CreateAccountModal },
      setup() {
        const show = ref(true)
        return { show }
      },
      template: `
        <CreateAccountModal
          :show="show"
          :proxies="[]"
          :groups="[]"
          @close="show = false"
        />
      `
    })

    const wrapper = mount(ParentHarness, {
      attachTo: document.body,
      global: {
        stubs: {
          Transition: false
        }
      }
    })

    await nextTick()
    expect(document.body.querySelector('.modal-overlay')).toBeTruthy()

    const kiroButton = Array.from(document.body.querySelectorAll('button')).find(button =>
      button.textContent?.includes('Kiro')
    )
    expect(kiroButton).toBeTruthy()

    kiroButton!.dispatchEvent(new Event('pointerdown', { bubbles: true, cancelable: true }))
    kiroButton!.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true }))
    kiroButton!.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, cancelable: true }))
    kiroButton!.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))
    await nextTick()

    expect((wrapper.vm as unknown as { show: boolean }).show).toBe(true)
    expect(document.body.querySelector('.modal-overlay')).toBeTruthy()
    // 两步流程：第一步显示添加方式选择，不因选中 Kiro 而关闭父级弹窗
    expect(document.body.textContent).toContain('admin.accounts.oauth.kiro.method')

    wrapper.unmount()
    expect(document.body.classList.contains('modal-open')).toBe(false)
  })
})

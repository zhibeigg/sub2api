import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'

const state = vi.hoisted(() => ({
  appStore: {
    publicSettingsLoaded: true,
    cachedPublicSettings: { payment_enabled: true, custom_menu_items: [] as unknown[] },
    contactInfo: '',
    toggleMobileSidebar: vi.fn(),
  },
  authStore: {
    user: {
      id: 1,
      username: 'tester',
      email: 'tester@example.com',
      role: 'user',
      balance: 12.34,
      frozen_balance: 0,
      avatar_url: '',
    },
    isSimpleMode: false,
    isAdmin: false,
    logout: vi.fn(),
  },
  route: {
    name: 'Keys',
    params: {},
    meta: { titleKey: 'keys.title', descriptionKey: 'keys.description' },
  },
}))

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return {
    ...actual,
    useRouter: () => ({ push: vi.fn() }),
    useRoute: () => state.route,
  }
})

vi.mock('@/stores', () => ({
  useAppStore: () => state.appStore,
  useAuthStore: () => state.authStore,
  useOnboardingStore: () => ({ replay: vi.fn() }),
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({ customMenuItems: [] }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => ({
        'keys.title': 'API 密钥',
        'keys.description': '管理密钥',
        'payment.quickRecharge': '充值',
        'payment.quickRechargeAria': '前往余额充值',
        'common.balance': '余额',
        'common.availableBalance': '可用余额',
        'common.frozenBalance': '冻结金额',
        'common.totalBalance': '总余额',
        'common.userMenu': 'User Menu',
      }[key] ?? key),
    }),
  }
})

import AppHeader from '../AppHeader.vue'

function mountHeader() {
  return mount(AppHeader, {
    global: {
      stubs: {
        RouterLink: RouterLinkStub,
        LocaleSwitcher: true,
        SubscriptionProgressMini: true,
        AnnouncementBell: true,
      },
    },
  })
}

describe('AppHeader recharge shortcut', () => {
  beforeEach(() => {
    state.appStore.publicSettingsLoaded = true
    state.appStore.cachedPublicSettings = { payment_enabled: true, custom_menu_items: [] }
  })

  it('支付启用时在桌面和移动余额区域提供充值快捷入口', async () => {
    const wrapper = mountHeader()
    await wrapper.find('button[aria-label="User Menu"]').trigger('click')

    const links = wrapper.findAllComponents(RouterLinkStub).filter((link) => {
      const to = link.props('to') as { path?: string; query?: Record<string, string> } | string
      return typeof to === 'object' && to.path === '/purchase'
    })

    expect(links).toHaveLength(2)
    expect(links[0].props('to')).toEqual({ path: '/purchase', query: { tab: 'balance' } })
    expect(wrapper.text()).toContain('充值')
  })

  it('支付关闭时隐藏充值入口', () => {
    state.appStore.cachedPublicSettings = { payment_enabled: false, custom_menu_items: [] }
    const wrapper = mountHeader()
    expect(wrapper.findAll('[aria-label="前往余额充值"]')).toHaveLength(0)
  })

  it('公开设置尚未加载时不提前显示充值入口', () => {
    state.appStore.publicSettingsLoaded = false
    const wrapper = mountHeader()
    expect(wrapper.findAll('[aria-label="前往余额充值"]')).toHaveLength(0)
  })
})

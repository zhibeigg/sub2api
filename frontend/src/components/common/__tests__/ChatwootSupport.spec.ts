import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { ref } from 'vue'
import ChatwootSupport from '../ChatwootSupport.vue'
import { useAppStore, useAuthStore } from '@/stores'

const { getChatwootIdentity, getCurrentUser } = vi.hoisted(() => ({
  getChatwootIdentity: vi.fn(),
  getCurrentUser: vi.fn(),
}))

const localeRef = ref('en')

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ locale: localeRef }),
  }
})

vi.mock('@/api', () => ({
  authAPI: {
    getChatwootIdentity,
    getCurrentUser,
    logout: vi.fn().mockResolvedValue(undefined),
  },
  isTotp2FARequired: () => false,
}))

function publicSettings(overrides: Record<string, unknown> = {}) {
  return {
    site_name: 'Test',
    chatwoot_enabled: true,
    chatwoot_base_url: 'https://support.example.com',
    chatwoot_website_token: 'website-token',
    ...overrides,
  } as any
}

function authenticatedUser(id = 7) {
  return {
    id,
    username: `user-${id}`,
    email: `user-${id}@example.com`,
    role: 'user',
    balance: 0,
    concurrency: 1,
    status: 'active',
    allowed_groups: null,
    balance_notify_enabled: false,
    balance_notify_threshold: null,
    balance_notify_extra_emails: [],
    created_at: '',
    updated_at: '',
  }
}

async function initializeSDK() {
  const run = vi.fn()
  window.chatwootSDK = { run }
  const script = document.querySelector<HTMLScriptElement>('script[data-sub2api-chatwoot-sdk]')
  expect(script).not.toBeNull()
  script?.dispatchEvent(new Event('load'))

  const client = {
    reset: vi.fn(),
    setLocale: vi.fn(),
    setUser: vi.fn(),
  }
  window.$chatwoot = client
  window.dispatchEvent(new Event('chatwoot:ready'))
  await flushPromises()
  return { run, client }
}

describe('ChatwootSupport', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    document.head.querySelectorAll('script[data-sub2api-chatwoot-sdk]').forEach((node) => node.remove())
    localeRef.value = 'en'
    getChatwootIdentity.mockReset()
    getCurrentUser.mockReset()
  })

  afterEach(() => {
    delete window.chatwootSDK
    delete window.$chatwoot
    delete window.chatwootSettings
    document.querySelectorAll('#chatwoot_live_chat_widget, .woot-widget-holder, .woot-widget-bubble').forEach((node) => node.remove())
  })

  it('does not load the SDK when disabled or incomplete', async () => {
    const appStore = useAppStore()
    window.__APP_CONFIG__ = publicSettings({ chatwoot_enabled: false })
    appStore.initFromInjectedConfig()

    mount(ChatwootSupport)
    await flushPromises()
    expect(document.querySelector('script[data-sub2api-chatwoot-sdk]')).toBeNull()

    window.__APP_CONFIG__ = publicSettings({ chatwoot_website_token: '' })
    appStore.clearPublicSettingsCache()
    appStore.initFromInjectedConfig()
    await flushPromises()
    expect(document.querySelector('script[data-sub2api-chatwoot-sdk]')).toBeNull()
  })

  it('loads once, applies locale, and uses only a signed identity', async () => {
    const user = authenticatedUser()
    localStorage.setItem('auth_token', 'token')
    localStorage.setItem('auth_user', JSON.stringify(user))
    getCurrentUser.mockResolvedValue({ data: user })
    getChatwootIdentity.mockResolvedValue({
      identifier: 'user-7',
      identifier_hash: 'signed-hash',
      name: 'Verified User',
      email: user.email,
    })

    const appStore = useAppStore()
    window.__APP_CONFIG__ = publicSettings()
    appStore.initFromInjectedConfig()
    useAuthStore().checkAuth()

    mount(ChatwootSupport)
    expect(document.querySelectorAll('script[data-sub2api-chatwoot-sdk]')).toHaveLength(1)

    const { run, client } = await initializeSDK()
    expect(run).toHaveBeenCalledWith({
      websiteToken: 'website-token',
      baseUrl: 'https://support.example.com',
    })
    expect(client.setLocale).toHaveBeenCalledWith('en')
    expect(client.setUser).toHaveBeenCalledWith('user-7', expect.objectContaining({
      identifier_hash: 'signed-hash',
      email: user.email,
    }))

    localeRef.value = 'zh'
    await flushPromises()
    expect(client.setLocale).toHaveBeenLastCalledWith('zh_CN')
  })

  it('keeps an authenticated visitor anonymous when the identity response has no HMAC', async () => {
    const user = authenticatedUser()
    localStorage.setItem('auth_token', 'token')
    localStorage.setItem('auth_user', JSON.stringify(user))
    getCurrentUser.mockResolvedValue({ data: user })
    getChatwootIdentity.mockResolvedValue({ identifier: 'user-7' })

    const appStore = useAppStore()
    window.__APP_CONFIG__ = publicSettings()
    appStore.initFromInjectedConfig()
    useAuthStore().checkAuth()

    mount(ChatwootSupport)
    const { client } = await initializeSDK()

    expect(client.setUser).not.toHaveBeenCalled()
  })

  it('ignores stale identity responses after logout', async () => {
    const user = authenticatedUser()
    localStorage.setItem('auth_token', 'token')
    localStorage.setItem('auth_user', JSON.stringify(user))
    getCurrentUser.mockResolvedValue({ data: user })

    let resolveIdentity!: (value: Record<string, string>) => void
    getChatwootIdentity.mockReturnValue(new Promise((resolve) => {
      resolveIdentity = resolve
    }))

    const appStore = useAppStore()
    window.__APP_CONFIG__ = publicSettings()
    appStore.initFromInjectedConfig()
    const authStore = useAuthStore()
    authStore.checkAuth()

    mount(ChatwootSupport)
    const { client } = await initializeSDK()

    await authStore.logout()
    resolveIdentity({ identifier: 'user-7', identifier_hash: 'late-hash' })
    await flushPromises()

    expect(client.reset).toHaveBeenCalled()
    expect(client.setUser).not.toHaveBeenCalled()
  })

  it('cleans and reloads when public configuration changes', async () => {
    const appStore = useAppStore()
    window.__APP_CONFIG__ = publicSettings()
    appStore.initFromInjectedConfig()
    mount(ChatwootSupport)

    const firstScript = document.querySelector('script[data-sub2api-chatwoot-sdk]')
    expect(firstScript).not.toBeNull()

    window.__APP_CONFIG__ = publicSettings({
      chatwoot_base_url: 'https://chat.example.net',
      chatwoot_website_token: 'new-token',
    })
    appStore.clearPublicSettingsCache()
    appStore.initFromInjectedConfig()
    await flushPromises()

    const scripts = document.querySelectorAll<HTMLScriptElement>('script[data-sub2api-chatwoot-sdk]')
    expect(scripts).toHaveLength(1)
    expect(scripts[0]).not.toBe(firstScript)
    expect(scripts[0]?.src).toBe('https://chat.example.net/packs/js/sdk.js')
  })
})

<script setup lang="ts">
import { computed, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { authAPI } from '@/api'
import { useAppStore, useAuthStore } from '@/stores'

const SCRIPT_MARKER = 'data-sub2api-chatwoot-sdk'
const WIDGET_SELECTORS = [
  '#chatwoot_live_chat_widget',
  '.woot-widget-holder',
  '.woot-widget-bubble',
]

const appStore = useAppStore()
const authStore = useAuthStore()
const { locale } = useI18n()

const config = computed(() => {
  const settings = appStore.cachedPublicSettings
  return {
    enabled: settings?.chatwoot_enabled === true,
    baseUrl: normalizeBaseUrl(settings?.chatwoot_base_url),
    websiteToken: settings?.chatwoot_website_token?.trim() ?? '',
  }
})

const configKey = computed(() =>
  `${config.value.enabled}:${config.value.baseUrl}:${config.value.websiteToken}`,
)

let lifecycle = 0
let identityRequest = 0
let sdkReady = false
let activeConfigKey = ''
let readyListener: (() => void) | null = null

function normalizeBaseUrl(value: string | undefined): string {
  const trimmed = value?.trim() ?? ''
  if (!trimmed) return ''

  try {
    const url = new URL(trimmed)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return ''
    return url.toString().replace(/\/$/, '')
  } catch {
    return ''
  }
}

function chatwootLocale(): string {
  return String(locale.value).toLowerCase().startsWith('zh') ? 'zh_CN' : 'en'
}

function isCompleteConfig(): boolean {
  return config.value.enabled && Boolean(config.value.baseUrl && config.value.websiteToken)
}

function removeReadyListener(): void {
  if (!readyListener) return
  window.removeEventListener('chatwoot:ready', readyListener)
  readyListener = null
}

function removeWidgetElements(): void {
  for (const selector of WIDGET_SELECTORS) {
    document.querySelectorAll(selector).forEach((element) => element.remove())
  }
}

function resetIdentity(): void {
  identityRequest += 1
  try {
    window.$chatwoot?.reset()
  } catch (error) {
    console.warn('Failed to reset Chatwoot identity:', error)
  }
}

function cleanupWidget(removeScript = true): void {
  lifecycle += 1
  sdkReady = false
  activeConfigKey = ''
  removeReadyListener()
  resetIdentity()
  removeWidgetElements()

  if (removeScript) {
    document.querySelectorAll<HTMLScriptElement>(`script[${SCRIPT_MARKER}]`).forEach((script) => script.remove())
    delete window.chatwootSDK
    delete window.$chatwoot
    delete window.chatwootSettings
  }
}

function applyLocale(): void {
  if (!sdkReady) return
  try {
    window.$chatwoot?.setLocale(chatwootLocale())
  } catch (error) {
    console.warn('Failed to set Chatwoot locale:', error)
  }
}

async function syncIdentity(): Promise<void> {
  const currentUserId = authStore.user?.id
  const currentLifecycle = lifecycle
  const request = ++identityRequest

  if (!sdkReady || !authStore.isAuthenticated || currentUserId === undefined) return

  try {
    const identity = await authAPI.getChatwootIdentity()
    if (
      request !== identityRequest ||
      currentLifecycle !== lifecycle ||
      !sdkReady ||
      authStore.user?.id !== currentUserId
    ) {
      return
    }

    const identifier = identity.identifier?.trim()
    const identifierHash = identity.identifier_hash?.trim()
    if (!identifier || !identifierHash) return

    window.$chatwoot?.setUser(identifier, {
      identifier_hash: identifierHash,
      name: identity.name || authStore.user?.username || undefined,
      email: identity.email || authStore.user?.email || undefined,
      avatar_url: identity.avatar_url || authStore.user?.avatar_url || undefined,
    })
  } catch {
    // Identity validation is optional; keep the visitor anonymous when unavailable.
  }
}

function handleSDKReady(expectedLifecycle: number): void {
  if (expectedLifecycle !== lifecycle || activeConfigKey !== configKey.value) return
  sdkReady = true
  removeReadyListener()
  applyLocale()
  void syncIdentity()
}

function runSDK(expectedLifecycle: number): void {
  if (expectedLifecycle !== lifecycle || !isCompleteConfig() || !window.chatwootSDK) return

  try {
    window.chatwootSettings = {
      ...(window.chatwootSettings ?? {}),
      position: 'right',
      locale: chatwootLocale(),
    }
    window.chatwootSDK.run({
      websiteToken: config.value.websiteToken,
      baseUrl: config.value.baseUrl,
    })
  } catch (error) {
    console.warn('Failed to initialize Chatwoot support widget:', error)
  }
}

function loadWidget(): void {
  cleanupWidget()
  if (!isCompleteConfig()) return

  const expectedLifecycle = lifecycle
  activeConfigKey = configKey.value
  readyListener = () => handleSDKReady(expectedLifecycle)
  window.addEventListener('chatwoot:ready', readyListener)

  const script = document.createElement('script')
  script.src = `${config.value.baseUrl}/packs/js/sdk.js`
  script.async = true
  script.defer = true
  script.setAttribute(SCRIPT_MARKER, 'true')
  script.addEventListener('load', () => runSDK(expectedLifecycle), { once: true })
  script.addEventListener('error', () => {
    if (expectedLifecycle === lifecycle) {
      removeReadyListener()
      console.warn('Failed to load Chatwoot support widget SDK')
    }
  }, { once: true })
  document.head.appendChild(script)
}

watch(configKey, loadWidget, { immediate: true })

watch(locale, applyLocale)

watch(
  () => authStore.user?.id,
  (userId, previousUserId) => {
    if (userId === previousUserId) return
    resetIdentity()
    if (userId !== undefined && sdkReady) void syncIdentity()
  },
)

onBeforeUnmount(() => cleanupWidget())
</script>

<template>
  <span v-if="false" aria-hidden="true"></span>
</template>

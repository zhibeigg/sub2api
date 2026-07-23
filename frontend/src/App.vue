<script setup lang="ts">
import { RouterView, useRouter, useRoute } from 'vue-router'
import { computed, onErrorCaptured, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Toast from '@/components/common/Toast.vue'
import NavigationProgress from '@/components/common/NavigationProgress.vue'
import ChatwootSupport from '@/components/common/ChatwootSupport.vue'
import AdminComplianceDialog from '@/components/admin/AdminComplianceDialog.vue'
import { applyRouteSEO, resolveRouteSEO } from '@/router/title'
import AnnouncementPopup from '@/components/common/AnnouncementPopup.vue'
import { useAppStore, useAuthStore, useSubscriptionStore, useAnnouncementStore, useAdminComplianceStore, useAdminSettingsStore } from '@/stores'
import { getSetupStatus } from '@/api/setup'
import { updateFavicon } from '@/utils/branding'

const router = useRouter()
const route = useRoute()
const { locale } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const subscriptionStore = useSubscriptionStore()
const announcementStore = useAnnouncementStore()
const adminComplianceStore = useAdminComplianceStore()
const adminSettingsStore = useAdminSettingsStore()
const renderError = ref(false)
const renderVersion = ref(0)
const errorBoundaryCopy = computed(() => {
  const language = String(locale.value).toLowerCase()
  if (language.startsWith('en')) {
    return {
      title: 'This page encountered an error',
      message: 'The failure was contained and no internal details were exposed.',
      retry: 'Retry this page',
      home: 'Return home'
    }
  }
  if (language.startsWith('ja')) {
    return {
      title: 'このページでエラーが発生しました',
      message: '障害は隔離され、内部情報は表示されていません。',
      retry: 'このページを再試行',
      home: 'ホームへ戻る'
    }
  }
  return {
    title: '当前页面发生异常',
    message: '异常已被隔离，内部错误详情不会暴露。',
    retry: '重试当前页面',
    home: '返回首页'
  }
})

onErrorCaptured((error, _instance, info) => {
  const errorName = error instanceof Error ? error.name : 'UnknownError'
  console.error('Component render error contained', { errorName, info })
  renderError.value = true
  return false
})

function retryCurrentPage() {
  renderError.value = false
  renderVersion.value += 1
}

function returnHome() {
  renderError.value = false
  router.push('/')
}

function updateRouteSEO() {
  const customMenuItems = [
    ...(appStore.cachedPublicSettings?.custom_menu_items ?? []),
    ...(authStore.isAdmin ? adminSettingsStore.customMenuItems : []),
  ]
  const seo = resolveRouteSEO(route, appStore.siteName, customMenuItems)
  applyRouteSEO(seo, String(locale.value), appStore.siteName)
}

// Watch for site settings changes and update favicon/title
watch(
  () => appStore.siteLogo,
  (newLogo) => {
    if (newLogo) {
      updateFavicon(newLogo)
    }
  },
  { immediate: true }
)

watch(
  [
    () => route.fullPath,
    () => route.meta.title,
    () => route.meta.titleKey,
    () => route.meta.seoTitleKey,
    () => route.meta.seoDescriptionKey,
    () => route.meta.indexable,
    () => route.meta.canonicalPath,
    () => locale.value,
    () => appStore.siteName,
    () => appStore.cachedPublicSettings?.custom_menu_items,
    () => authStore.isAdmin,
    () => adminSettingsStore.customMenuItems,
  ],
  updateRouteSEO,
  { deep: true }
)

// Watch for authentication state and manage subscription data + announcements
function onVisibilityChange() {
  if (document.visibilityState === 'visible' && authStore.isAuthenticated) {
    announcementStore.fetchAnnouncements()
  }
}

function onAdminComplianceRequired(event: Event) {
  const detail = (event as CustomEvent<Record<string, string>>).detail || {}
  adminComplianceStore.requireAcknowledgement(detail)
}

watch(
  () => authStore.isAuthenticated,
  (isAuthenticated, oldValue) => {
    if (isAuthenticated) {
      if (authStore.isAdmin) {
        adminComplianceStore.fetchStatus().catch((error) => {
          console.error('Failed to fetch admin compliance status:', error)
        })
      }

      // User logged in: preload subscriptions and start polling
      subscriptionStore.fetchActiveSubscriptions().catch((error) => {
        console.error('Failed to preload subscriptions:', error)
      })
      subscriptionStore.startPolling()

      // Announcements: new login vs page refresh restore
      if (oldValue === false) {
        // New login: delay 3s then force fetch
        setTimeout(() => announcementStore.fetchAnnouncements(true), 3000)
      } else {
        // Page refresh restore (oldValue was undefined)
        announcementStore.fetchAnnouncements()
      }

      // Register visibility change listener
      document.addEventListener('visibilitychange', onVisibilityChange)
    } else {
      // User logged out: clear data and stop polling
      subscriptionStore.clear()
      announcementStore.reset()
      adminComplianceStore.reset()
      document.removeEventListener('visibilitychange', onVisibilityChange)
    }
  },
  { immediate: true }
)

// Route change trigger (throttled by store)
router.afterEach(() => {
  if (authStore.isAuthenticated) {
    announcementStore.fetchAnnouncements()
  }
})

onBeforeUnmount(() => {
  document.removeEventListener('visibilitychange', onVisibilityChange)
  window.removeEventListener('admin-compliance-required', onAdminComplianceRequired)
})

onMounted(async () => {
  window.addEventListener('admin-compliance-required', onAdminComplianceRequired)

  // Check if setup is needed
  try {
    const status = await getSetupStatus()
    if (status.needs_setup && route.path !== '/setup') {
      router.replace('/setup')
      return
    }
  } catch {
    // If setup endpoint fails, assume normal mode and continue
  }

  // Load public settings into appStore (will be cached for other components)
  await appStore.fetchPublicSettings()

  // Re-resolve route SEO now that site settings are available
  updateRouteSEO()
})
</script>

<template>
  <NavigationProgress />
  <ChatwootSupport />
  <main v-if="renderError" class="app-error-boundary" role="alert">
    <div class="app-error-boundary__panel">
      <p class="app-error-boundary__eyebrow">PokeAPI</p>
      <h1>{{ errorBoundaryCopy.title }}</h1>
      <p>{{ errorBoundaryCopy.message }}</p>
      <div class="app-error-boundary__actions">
        <button type="button" @click="retryCurrentPage">{{ errorBoundaryCopy.retry }}</button>
        <button type="button" class="app-error-boundary__secondary" @click="returnHome">
          {{ errorBoundaryCopy.home }}
        </button>
      </div>
    </div>
  </main>
  <RouterView v-else :key="`${route.fullPath}:${renderVersion}`" />
  <Toast />
  <AnnouncementPopup />
  <AdminComplianceDialog />
</template>

<style scoped>
.app-error-boundary {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 2rem;
  background: var(--color-bg-primary, #f5f5f3);
  color: var(--color-text-primary, #161616);
}

.app-error-boundary__panel {
  width: min(34rem, 100%);
  padding: 2rem;
  border: 1px solid color-mix(in srgb, currentColor 18%, transparent);
  border-radius: 1rem;
  background: var(--color-bg-secondary, #ffffff);
  box-shadow: 0 1.25rem 4rem rgb(0 0 0 / 10%);
}

.app-error-boundary__eyebrow {
  margin: 0 0 0.75rem;
  font: 600 0.75rem/1.2 monospace;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  opacity: 0.6;
}

.app-error-boundary h1 {
  margin: 0;
  font-size: clamp(1.75rem, 4vw, 2.5rem);
}

.app-error-boundary p {
  margin: 1rem 0 0;
  line-height: 1.7;
  opacity: 0.78;
}

.app-error-boundary__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  margin-top: 1.5rem;
}

.app-error-boundary button {
  min-height: 2.75rem;
  padding: 0.65rem 1rem;
  border: 1px solid currentColor;
  border-radius: 0.6rem;
  background: currentColor;
  color: var(--color-bg-secondary, #ffffff);
  cursor: pointer;
}

.app-error-boundary__secondary {
  background: transparent !important;
  color: inherit !important;
}
</style>

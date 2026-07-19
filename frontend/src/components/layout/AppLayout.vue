<template>
  <div
    class="min-h-screen bg-gray-50 dark:bg-dark-950"
    :class="fullHeight ? 'lg:flex lg:h-screen lg:flex-col lg:overflow-hidden' : ''"
  >
    <!-- Background Decoration -->
    <div class="pointer-events-none fixed inset-0 bg-mesh-gradient"></div>

    <!-- Sidebar -->
    <AppSidebar />

    <!-- Main Content Area -->
    <div
      class="relative min-h-screen transition-all duration-300"
      :class="[
        sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64',
        fullHeight ? 'lg:flex lg:h-screen lg:flex-col lg:overflow-hidden' : ''
      ]"
    >
      <!-- Header -->
      <AppHeader :class="fullHeight ? 'lg:flex-none' : ''" />

      <!-- Persistent scrolling notice bar (admin-configured) -->
      <NoticeBar :class="fullHeight ? 'lg:flex-none' : ''" />

      <!-- Main Content -->
      <main
        class="p-4 md:p-6 lg:p-8"
        :class="fullHeight ? 'lg:min-h-0 lg:flex-1 lg:overflow-hidden' : ''"
      >
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'
import NoticeBar from '@/components/common/NoticeBar.vue'

withDefaults(defineProps<{ fullHeight?: boolean }>(), {
  fullHeight: false
})

const appStore = useAppStore()
const authStore = useAuthStore()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')

const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>

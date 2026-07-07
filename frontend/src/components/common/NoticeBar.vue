<template>
  <div
    v-if="items.length"
    class="pk-notice glass sticky top-0 z-20 flex items-center gap-2 overflow-hidden border-b border-gray-200/50 px-4 py-2 dark:border-dark-700/50"
    role="status"
    :aria-label="t('common.notice')"
  >
    <Icon name="bell" size="sm" class="flex-shrink-0 text-primary-500" />
    <div class="pk-notice-viewport min-w-0 flex-1 overflow-hidden">
      <div class="pk-notice-track" :class="{ 'pk-notice-track--animate': animate }">
        <!-- Two identical sequences for seamless looping. -->
        <span
          v-for="pass in 2"
          :key="pass"
          class="pk-notice-seq"
          :aria-hidden="pass === 2 ? 'true' : undefined"
        >
          <span v-for="(item, i) in items" :key="`${pass}-${i}`" class="pk-notice-item">
            {{ item }}
          </span>
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()

// Split the configured text into individual notices (by newline).
const items = computed<string[]>(() => {
  const raw = appStore.cachedPublicSettings?.notice_bar || ''
  return raw
    .split(/\r?\n/)
    .map((s) => s.trim())
    .filter(Boolean)
})

// Respect reduced-motion: render statically (user can still scroll the row).
const animate = computed(() => {
  if (typeof window === 'undefined' || !window.matchMedia) return true
  return !window.matchMedia('(prefers-reduced-motion: reduce)').matches
})
</script>

<style scoped>
.pk-notice-viewport {
  position: relative;
}

.pk-notice-track {
  display: inline-flex;
  white-space: nowrap;
  will-change: transform;
}

.pk-notice-track--animate {
  animation: pk-notice-scroll 40s linear infinite;
}
.pk-notice-track--animate:hover {
  animation-play-state: paused;
}

/* When not animating, allow horizontal scroll of the row instead. */
.pk-notice-track:not(.pk-notice-track--animate) {
  animation: none;
}

.pk-notice-seq {
  display: inline-flex;
  align-items: center;
}

.pk-notice-item {
  display: inline-flex;
  align-items: center;
  padding-inline: 1.75rem;
  font-size: 13px;
  color: var(--pk-notice-fg, inherit);
}
.pk-notice-item::before {
  content: '';
  display: inline-block;
  width: 4px;
  height: 4px;
  margin-right: 1.75rem;
  border-radius: 9999px;
  background: currentColor;
  opacity: 0.35;
}
.pk-notice-item:first-child::before {
  display: none;
}

@keyframes pk-notice-scroll {
  from {
    transform: translateX(0);
  }
  to {
    /* Two identical sequences → shifting by 50% loops seamlessly. */
    transform: translateX(-50%);
  }
}

@media (prefers-reduced-motion: reduce) {
  .pk-notice-track--animate {
    animation: none;
  }
  .pk-notice-viewport {
    overflow-x: auto;
  }
}
</style>

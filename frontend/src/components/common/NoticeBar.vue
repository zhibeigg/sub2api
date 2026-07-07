<template>
  <div
    v-if="items.length"
    class="pk-notice glass sticky top-0 z-20 flex items-center gap-2 overflow-hidden border-b border-gray-200/50 px-4 py-2 dark:border-dark-700/50"
    role="status"
    :aria-label="t('common.notice')"
  >
    <Icon name="bell" size="sm" class="flex-shrink-0 text-primary-500" />
    <div ref="viewportRef" class="pk-notice-viewport min-w-0 flex-1 overflow-hidden">
      <div
        ref="trackRef"
        class="pk-notice-track"
        :class="{ 'pk-notice-track--animate': animate }"
        :style="{ '--pk-notice-duration': `${durationSeconds}s` }"
      >
        <!-- Two identical sequences for seamless looping. -->
        <span
          v-for="pass in 2"
          :key="pass"
          class="pk-notice-seq"
          :aria-hidden="pass === 2 ? 'true' : undefined"
        >
          <template v-for="(item, i) in items" :key="`${pass}-${i}`">
            <a
              v-if="item.url"
              :href="item.url"
              target="_blank"
              rel="noopener noreferrer"
              class="pk-notice-item pk-notice-item--link"
            >
              {{ item.text }}
            </a>
            <span v-else class="pk-notice-item">{{ item.text }}</span>
          </template>
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()

interface NoticeItem {
  text: string
  url?: string
}

// Split the configured text into individual notices (by newline). Each line
// may carry an optional link using "文本 | https://..." syntax; only http(s)
// URLs are treated as links (anything else stays plain text, XSS-safe).
const items = computed<NoticeItem[]>(() => {
  const raw = appStore.cachedPublicSettings?.notice_bar || ''
  return raw
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const sep = line.lastIndexOf('|')
      if (sep > -1) {
        const text = line.slice(0, sep).trim()
        const url = line.slice(sep + 1).trim()
        if (/^https?:\/\//i.test(url)) {
          return { text: text || url, url }
        }
      }
      return { text: line }
    })
})

// Respect reduced-motion: render statically (user can still read the row).
const animate = computed(() => {
  if (typeof window === 'undefined' || !window.matchMedia) return true
  return !window.matchMedia('(prefers-reduced-motion: reduce)').matches
})

// Constant visual scroll speed (px/s). The animation shifts by 50% of the track
// (one full sequence), so duration is derived from a single sequence's width to
// keep speed uniform regardless of how much text is configured.
const SCROLL_SPEED_PX_PER_SEC = 70
const viewportRef = ref<HTMLElement | null>(null)
const trackRef = ref<HTMLElement | null>(null)
const durationSeconds = ref(20)

function recomputeDuration() {
  const track = trackRef.value
  if (!track) return
  // Track holds two identical sequences; one sequence is half the scrollWidth.
  const seqWidth = track.scrollWidth / 2
  if (seqWidth <= 0) return
  const seconds = seqWidth / SCROLL_SPEED_PX_PER_SEC
  // Clamp to a sensible range so very short notices aren't dizzyingly fast.
  durationSeconds.value = Math.min(Math.max(seconds, 8), 60)
}

let resizeObserver: ResizeObserver | null = null

onMounted(() => {
  nextTick(recomputeDuration)
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => recomputeDuration())
    if (viewportRef.value) resizeObserver.observe(viewportRef.value)
    if (trackRef.value) resizeObserver.observe(trackRef.value)
  }
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  resizeObserver = null
})

// Recompute when the notice content changes.
watch(items, () => nextTick(recomputeDuration))
</script>

<style scoped>
.pk-notice-viewport {
  position: relative;
  /* Fade both edges so text scrolls in/out through a soft mask (左右通透). */
  --pk-notice-fade: 48px;
  -webkit-mask-image: linear-gradient(
    to right,
    transparent 0,
    #000 var(--pk-notice-fade),
    #000 calc(100% - var(--pk-notice-fade)),
    transparent 100%
  );
  mask-image: linear-gradient(
    to right,
    transparent 0,
    #000 var(--pk-notice-fade),
    #000 calc(100% - var(--pk-notice-fade)),
    transparent 100%
  );
}

.pk-notice-track {
  display: inline-flex;
  white-space: nowrap;
  will-change: transform;
}

.pk-notice-track--animate {
  animation: pk-notice-scroll var(--pk-notice-duration, 20s) linear infinite;
}
.pk-notice-track--animate:hover {
  animation-play-state: paused;
}

/* When not animating, render statically (no native scrollbar). */
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
.pk-notice-item--link {
  text-decoration: none;
  color: inherit;
  cursor: pointer;
  transition: color 0.15s ease;
}
.pk-notice-item--link:hover {
  color: var(--pk-notice-link-hover, #14b8a6);
  text-decoration: underline;
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
}

/* On small screens, tighten the fade so more text stays readable. */
@media (max-width: 640px) {
  .pk-notice-viewport {
    --pk-notice-fade: 24px;
  }
}
</style>

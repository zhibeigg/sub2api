<template>
  <div v-if="apiEndpoint" class="cap-wrapper">
    <cap-widget
      ref="widgetRef"
      :data-cap-api-endpoint="apiEndpoint"
      :data-cap-theme="capTheme"
      @solve="onSolve"
      @error="onError"
      @reset="onReset"
      @expire="onExpire"
    ></cap-widget>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
// Bundled widget (self-hosted, CSP 'self'-friendly). Registers the
// <cap-widget> custom element as a side effect of import.
import '@cap.js/widget'

// Rename component so its kebab-case name does not collide with the
// <cap-widget> custom element (which would cause a self-reference).
defineOptions({ name: 'CapCaptcha' })

const props = withDefaults(
  defineProps<{
    // Cap instance base URL, e.g. https://cap.example.com (no trailing slash)
    endpoint: string
    // Cap site key from the dashboard
    siteKey: string
    theme?: 'light' | 'dark' | 'auto'
  }>(),
  {
    theme: 'auto'
  }
)

const emit = defineEmits<{
  (e: 'verify', token: string): void
  (e: 'expire'): void
  (e: 'error'): void
}>()

const widgetRef = ref<HTMLElement & { reset?: () => void }>()

// Cap expects the challenge endpoint as `{instance}/{siteKey}/`
const apiEndpoint = computed(() => {
  const base = (props.endpoint || '').trim().replace(/\/+$/, '')
  const key = (props.siteKey || '').trim().replace(/^\/+|\/+$/g, '')
  if (!base || !key) return ''
  return `${base}/${key}/`
})

// Resolve 'auto' against the site's html.dark convention
const capTheme = computed(() => {
  if (props.theme !== 'auto') return props.theme
  return document.documentElement.classList.contains('dark') ? 'dark' : 'light'
})

interface CapSolveEvent extends Event {
  detail?: { token?: string }
}

function onSolve(e: Event): void {
  const token = (e as CapSolveEvent).detail?.token
  if (token) emit('verify', token)
}

function onError(): void {
  emit('error')
}

function onExpire(): void {
  emit('expire')
}

function onReset(): void {
  // Widget was reset (token cleared); surface as expire so parent clears token
  emit('expire')
}

function reset(): void {
  try {
    widgetRef.value?.reset?.()
  } catch {
    // ignore
  }
}

defineExpose({ reset })

// Keep the widget theme in sync when the site toggles dark mode
let themeObserver: MutationObserver | null = null

onMounted(() => {
  if (props.theme !== 'auto') return
  themeObserver = new MutationObserver(() => {
    const el = widgetRef.value
    if (el) el.setAttribute('data-cap-theme', capTheme.value)
  })
  themeObserver.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class']
  })
})

onBeforeUnmount(() => {
  themeObserver?.disconnect()
})
</script>

<style scoped>
.cap-wrapper {
  width: 100%;
}

.cap-wrapper :deep(cap-widget) {
  width: 100%;
  --cap-widget-width: 100%;
}
</style>

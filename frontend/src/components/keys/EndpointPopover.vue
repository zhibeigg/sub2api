<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useClipboard } from '@/composables/useClipboard'
import Icon from '@/components/icons/Icon.vue'
import type { CustomEndpoint } from '@/types'
import {
  BUILT_IN_ENDPOINT_URL,
  buildEndpointDisplayItems,
  classifyEndpointLatency,
  measureEndpointLatency,
  runWithConcurrency,
  type EndpointDisplayItem,
  type EndpointLatencyLevel,
  type EndpointProbeResult,
} from '@/utils/endpointLatency'

const props = defineProps<{
  apiBaseUrl: string
  customEndpoints: CustomEndpoint[]
}>()

const { t, locale } = useI18n()
const { copyToClipboard } = useClipboard()
const ENDPOINT_AUTO_REFRESH_INTERVAL_MS = 3_000

const copiedEndpoint = ref<string | null>(null)
const probeResults = ref<Record<string, EndpointProbeResult>>({})
const testingAll = ref(false)

let copiedResetTimer: number | undefined
let autoRefreshTimer: number | undefined
let probeController: AbortController | null = null
let probeGeneration = 0

const builtInEndpoints = computed<CustomEndpoint[]>(() => [
  {
    name: t('keys.endpoints.builtInName'),
    endpoint: BUILT_IN_ENDPOINT_URL,
    description: t('keys.endpoints.builtInDescription'),
  },
])

const allEndpoints = computed(() => {
  const items = buildEndpointDisplayItems(
    props.apiBaseUrl,
    props.customEndpoints,
    builtInEndpoints.value,
  )

  return items.map((item) => ({
    ...item,
    name: item.name || t('keys.endpoints.primaryName'),
    description: item.description || t('keys.endpoints.defaultDescription'),
  }))
})

const testedCount = computed(() => allEndpoints.value.filter((item) => probeResults.value[item.key]?.status === 'success').length)

async function copy(url: string) {
  const success = await copyToClipboard(url, t('keys.endpoints.copied'))
  if (!success) return

  copiedEndpoint.value = url
  if (copiedResetTimer !== undefined) {
    window.clearTimeout(copiedResetTimer)
  }
  copiedResetTimer = window.setTimeout(() => {
    if (copiedEndpoint.value === url) copiedEndpoint.value = null
  }, 1800)
}

function tooltipHint(endpoint: string): string {
  return copiedEndpoint.value === endpoint
    ? t('keys.endpoints.copiedHint')
    : t('keys.endpoints.clickToCopy')
}

function resultFor(item: EndpointDisplayItem): EndpointProbeResult {
  return probeResults.value[item.key] ?? { status: 'idle', latencyMs: null, testedAt: null }
}

function latencyLevel(item: EndpointDisplayItem): EndpointLatencyLevel {
  const result = resultFor(item)
  return classifyEndpointLatency(result.latencyMs, result.status)
}

function latencyText(item: EndpointDisplayItem): string {
  const result = resultFor(item)
  if (result.status === 'testing' || result.status === 'idle') return t('keys.endpoints.testing')
  if (result.status === 'timeout') return t('keys.endpoints.timeout')
  if (result.status === 'error') return t('keys.endpoints.unreachable')
  return `${result.latencyMs}ms`
}

function latencyStatusText(item: EndpointDisplayItem): string {
  const result = resultFor(item)
  if (result.status === 'testing' || result.status === 'idle') return t('keys.endpoints.statusTesting')
  if (result.status === 'timeout') return t('keys.endpoints.statusTimeout')
  if (result.status === 'error') return t('keys.endpoints.statusUnreachable')
  return t(`keys.endpoints.status.${latencyLevel(item)}`)
}

function testedAtText(item: EndpointDisplayItem): string {
  const testedAt = resultFor(item).testedAt
  if (!testedAt) return t('keys.endpoints.notTested')
  return new Intl.DateTimeFormat(locale.value, {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(testedAt)
}

function latencyTextClass(item: EndpointDisplayItem): string {
  const level = latencyLevel(item)
  return {
    fast: 'text-emerald-600 dark:text-emerald-400',
    normal: 'text-sky-600 dark:text-sky-400',
    slow: 'text-amber-600 dark:text-amber-400',
    poor: 'text-rose-600 dark:text-rose-400',
    unavailable: 'text-gray-400 dark:text-dark-400',
  }[level]
}

function latencyDotClass(item: EndpointDisplayItem): string {
  const result = resultFor(item)
  if (result.status === 'testing' || result.status === 'idle') return 'bg-gray-300 dark:bg-dark-500'

  return {
    fast: 'bg-emerald-500',
    normal: 'bg-sky-500',
    slow: 'bg-amber-500',
    poor: 'bg-rose-500',
    unavailable: 'bg-gray-400 dark:bg-dark-400',
  }[latencyLevel(item)]
}

function latencyBarClass(item: EndpointDisplayItem): string {
  const result = resultFor(item)
  if (result.status === 'testing' || result.status === 'idle') return 'w-1/3 animate-pulse bg-gray-300 dark:bg-dark-500'

  return {
    fast: 'w-full bg-emerald-500',
    normal: 'w-4/5 bg-sky-500',
    slow: 'w-3/5 bg-amber-500',
    poor: 'w-2/5 bg-rose-500',
    unavailable: 'w-1/5 bg-gray-400 dark:bg-dark-400',
  }[latencyLevel(item)]
}

async function testAllEndpoints(preservePreviousResults = false) {
  probeController?.abort()
  probeController = new AbortController()
  const activeController = probeController
  const generation = ++probeGeneration
  const items = allEndpoints.value

  if (items.length === 0) {
    testingAll.value = false
    return
  }

  testingAll.value = true
  const nextResults = { ...probeResults.value }
  items.forEach((item) => {
    if (!preservePreviousResults || !nextResults[item.key]) {
      nextResults[item.key] = { status: 'testing', latencyMs: null, testedAt: null }
    }
  })
  probeResults.value = nextResults

  await runWithConcurrency(items, 4, async (item) => {
    const result = await measureEndpointLatency(item.endpoint, { signal: activeController.signal })
    if (generation !== probeGeneration) return
    probeResults.value = { ...probeResults.value, [item.key]: result }
  })

  if (generation === probeGeneration) testingAll.value = false
}

function autoRefreshEndpoints() {
  if (testingAll.value) return
  void testAllEndpoints(true)
}

watch(
  () => allEndpoints.value.map((item) => item.key).join('|'),
  () => void testAllEndpoints(),
  { immediate: true },
)

onMounted(() => {
  autoRefreshTimer = window.setInterval(autoRefreshEndpoints, ENDPOINT_AUTO_REFRESH_INTERVAL_MS)
})

onBeforeUnmount(() => {
  probeController?.abort()
  if (autoRefreshTimer !== undefined) window.clearInterval(autoRefreshTimer)
  if (copiedResetTimer !== undefined) window.clearTimeout(copiedResetTimer)
})
</script>

<template>
  <section v-if="allEndpoints.length > 0" class="endpoint-rail" :aria-label="t('keys.endpoints.availableTitle')">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <div class="flex items-center gap-2 text-xs">
        <span class="font-semibold text-gray-700 dark:text-gray-200">{{ t('keys.endpoints.availableTitle') }}</span>
        <span class="rounded-full bg-gray-100 px-2 py-0.5 font-medium tabular-nums text-gray-500 dark:bg-dark-700 dark:text-dark-300">
          {{ allEndpoints.length }}
        </span>
        <span class="hidden text-gray-400 dark:text-dark-400 sm:inline">
          {{ t('keys.endpoints.testedSummary', { tested: testedCount, total: allEndpoints.length }) }}
        </span>
      </div>

      <button
        type="button"
        class="inline-flex min-h-9 items-center gap-1.5 rounded-lg px-2.5 text-xs font-medium text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-white dark:focus-visible:ring-offset-dark-900"
        :disabled="testingAll"
        :aria-label="t('keys.endpoints.retest')"
        @click="testAllEndpoints()"
      >
        <Icon name="refresh" size="sm" :class="testingAll ? 'animate-spin' : ''" />
        <span>{{ testingAll ? t('keys.endpoints.testingAll') : t('keys.endpoints.retest') }}</span>
      </button>
    </div>

    <div class="mt-2 flex flex-wrap gap-2" role="list">
      <article
        v-for="(item, index) in allEndpoints"
        :key="item.key"
        class="group/node relative flex min-h-11 w-full items-center gap-2 rounded-xl border border-gray-200 bg-white px-2.5 py-2 transition-[border-color,background-color] hover:border-gray-300 focus-within:border-primary-300 dark:border-dark-600 dark:bg-dark-800 dark:hover:border-dark-500 dark:focus-within:border-primary-700 sm:w-auto sm:min-w-[15rem]"
        role="listitem"
      >
        <span class="relative flex h-4 w-4 flex-none items-center justify-center" aria-hidden="true">
          <span
            v-if="resultFor(item).status === 'testing' || resultFor(item).status === 'idle'"
            class="absolute h-4 w-4 animate-ping rounded-full bg-gray-300/50 dark:bg-dark-500/50"
          ></span>
          <span class="relative h-2 w-2 rounded-full" :class="latencyDotClass(item)"></span>
        </span>

        <button
          type="button"
          class="min-w-0 flex-1 text-left focus-visible:outline-none"
          :aria-label="`${item.name}，${tooltipHint(item.endpoint)}`"
          @click="copy(item.endpoint)"
        >
          <span class="flex min-w-0 items-center gap-1.5">
            <code class="truncate font-mono text-xs font-semibold text-gray-800 dark:text-gray-100">{{ item.host }}</code>
            <span
              v-if="item.isDefault"
              class="flex-none rounded bg-primary-50 px-1 py-px text-[10px] font-semibold text-primary-600 dark:bg-primary-900/30 dark:text-primary-300"
            >{{ t('keys.endpoints.default') }}</span>
            <span
              v-else-if="item.isBuiltIn"
              class="flex-none rounded bg-slate-100 px-1 py-px text-[10px] font-semibold text-slate-600 dark:bg-slate-700 dark:text-slate-200"
            >{{ t('keys.endpoints.backup') }}</span>
          </span>
          <span class="mt-1 flex items-center gap-2">
            <span class="font-mono text-[11px] font-semibold tabular-nums" :class="latencyTextClass(item)" aria-live="polite">{{ latencyText(item) }}</span>
            <span class="h-1 w-10 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
              <span class="block h-full rounded-full transition-[width,background-color] duration-300" :class="latencyBarClass(item)"></span>
            </span>
            <span class="truncate text-[10px] text-gray-400 dark:text-dark-400">{{ item.name }}</span>
          </span>
        </button>

        <button
          type="button"
          class="flex h-8 w-8 flex-none items-center justify-center rounded-lg transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
          :class="copiedEndpoint === item.endpoint
            ? 'bg-emerald-50 text-emerald-600 dark:bg-emerald-900/25 dark:text-emerald-300'
            : 'text-gray-400 hover:bg-gray-100 hover:text-primary-600 dark:text-dark-400 dark:hover:bg-dark-700 dark:hover:text-primary-300'"
          :aria-label="tooltipHint(item.endpoint)"
          @click="copy(item.endpoint)"
        >
          <Icon :name="copiedEndpoint === item.endpoint ? 'check' : 'copy'" size="sm" />
        </button>

        <div
          class="pointer-events-none absolute top-full z-40 mt-2 hidden w-[min(22rem,calc(100vw-2rem))] -translate-y-1 rounded-xl border border-gray-200 bg-white p-3 text-left opacity-0 shadow-[0_18px_50px_-24px_rgba(15,23,42,0.45)] transition-[opacity,transform] duration-150 group-hover/node:block group-hover/node:translate-y-0 group-hover/node:opacity-100 group-focus-within/node:block group-focus-within/node:translate-y-0 group-focus-within/node:opacity-100 dark:border-dark-600 dark:bg-dark-800"
          :class="index === allEndpoints.length - 1 ? 'right-0' : 'left-0'"
          role="tooltip"
        >
          <div class="flex items-start justify-between gap-4">
            <div class="min-w-0">
              <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ item.name }}</p>
              <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-300">{{ item.description }}</p>
            </div>
            <span class="flex-none rounded-md bg-gray-100 px-1.5 py-1 font-mono text-[10px] font-semibold text-gray-600 dark:bg-dark-700 dark:text-dark-200">{{ item.protocol }}</span>
          </div>

          <dl class="mt-3 grid grid-cols-[auto_1fr] gap-x-3 gap-y-2 border-t border-gray-100 pt-3 text-xs dark:border-dark-700">
            <dt class="text-gray-400 dark:text-dark-400">{{ t('keys.endpoints.nodeAddress') }}</dt>
            <dd class="min-w-0 truncate text-right font-mono text-gray-700 dark:text-gray-200">{{ item.endpoint }}</dd>
            <dt class="text-gray-400 dark:text-dark-400">{{ t('keys.endpoints.measuredLatency') }}</dt>
            <dd class="text-right font-semibold tabular-nums" :class="latencyTextClass(item)">{{ latencyText(item) }}</dd>
            <dt class="text-gray-400 dark:text-dark-400">{{ t('keys.endpoints.nodeStatus') }}</dt>
            <dd class="text-right font-medium text-gray-700 dark:text-gray-200">{{ latencyStatusText(item) }}</dd>
            <dt class="text-gray-400 dark:text-dark-400">{{ t('keys.endpoints.lastTestedAt') }}</dt>
            <dd class="text-right font-mono text-gray-700 dark:text-gray-200">{{ testedAtText(item) }}</dd>
          </dl>

          <p class="mt-3 flex items-center gap-1.5 text-[11px] text-primary-600 dark:text-primary-300">
            <span class="h-1.5 w-1.5 rounded-full bg-primary-500 dark:bg-primary-300"></span>
            {{ tooltipHint(item.endpoint) }}
          </p>
        </div>
      </article>
    </div>

    <p class="mt-2 text-[11px] leading-4 text-gray-400 dark:text-dark-400">
      {{ t('keys.endpoints.clientProbeHint') }}
    </p>
  </section>
</template>

<style scoped>
.endpoint-rail {
  @apply rounded-2xl border border-gray-200 bg-gray-50/70 p-3 dark:border-dark-700 dark:bg-dark-900/55;
}

@media (prefers-reduced-motion: reduce) {
  .endpoint-rail * {
    animation-duration: 0.01ms !important;
    transition-duration: 0.01ms !important;
  }
}
</style>

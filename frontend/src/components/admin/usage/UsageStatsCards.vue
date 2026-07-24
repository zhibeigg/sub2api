<template>
  <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
    <div class="card p-4 flex items-center gap-3">
      <div class="rounded-lg bg-blue-100 p-2 dark:bg-blue-900/30 text-blue-600">
        <Icon name="document" size="md" />
      </div>
      <div>
        <p class="text-xs font-medium text-gray-500">{{ t('usage.totalRequests') }}</p>
        <p class="text-xl font-bold">{{ stats?.total_requests?.toLocaleString() || '0' }}</p>
        <p class="text-xs text-gray-400">{{ t('usage.inSelectedRange') }}</p>
      </div>
    </div>
    <div class="card token-summary-card p-4">
      <div class="flex items-start gap-3">
        <div class="shrink-0 rounded-lg bg-amber-100 p-2 text-amber-600 dark:bg-amber-900/30">
          <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="m21 7.5-9-5.25L3 7.5m18 0-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9" />
          </svg>
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('usage.totalTokens') }}</p>
          <p class="mt-0.5 text-xl font-bold tracking-tight text-gray-900 tabular-nums dark:text-white">
            {{ formatTokens(totalTokens) }}
          </p>

          <div class="token-metric-grid mt-3 border-t border-gray-100 pt-3 dark:border-dark-700" data-testid="token-breakdown">
            <div class="min-w-0">
              <p class="text-[11px] leading-4 text-gray-500 dark:text-gray-400">{{ t('usage.in') }}</p>
              <p class="truncate text-sm font-semibold tabular-nums text-blue-700 dark:text-blue-300">{{ formatTokens(inputTokens) }}</p>
            </div>
            <div class="min-w-0">
              <p class="text-[11px] leading-4 text-gray-500 dark:text-gray-400">{{ t('usage.out') }}</p>
              <p class="truncate text-sm font-semibold tabular-nums text-violet-700 dark:text-violet-300">{{ formatTokens(outputTokens) }}</p>
            </div>
            <div class="min-w-0">
              <p class="text-[11px] leading-4 text-gray-500 dark:text-gray-400">{{ t('usage.cacheHit') }}</p>
              <p class="truncate text-sm font-semibold tabular-nums text-sky-700 dark:text-sky-300">{{ formatTokens(cacheReadTokens) }}</p>
            </div>
            <div class="min-w-0">
              <p class="text-[11px] leading-4 text-gray-500 dark:text-gray-400">{{ t('usage.cacheCreate') }}</p>
              <p class="truncate text-sm font-semibold tabular-nums text-amber-700 dark:text-amber-300">{{ formatTokens(cacheCreationTokens) }}</p>
            </div>
          </div>

          <div
            v-if="cacheHitRate != null"
            class="mt-3 flex items-center justify-between gap-3 text-xs"
            data-testid="cache-hit-rate"
          >
            <span class="text-gray-500 dark:text-gray-400">{{ t('usage.cacheHitRate') }}</span>
            <span class="flex min-w-0 items-center gap-1.5 whitespace-nowrap tabular-nums">
              <span class="text-gray-400 dark:text-gray-500">{{ formatTokens(cacheReadTokens) }} / {{ formatTokens(promptTokens) }}</span>
              <span class="font-semibold text-sky-700 dark:text-sky-300">{{ cacheHitRate }}</span>
            </span>
          </div>
        </div>
      </div>
    </div>
    <div class="card p-4 flex items-center gap-3">
      <div class="rounded-lg bg-green-100 p-2 dark:bg-green-900/30 text-green-600">
        <Icon name="dollar" size="md" />
      </div>
      <div class="min-w-0 flex-1">
        <p class="text-xs font-medium text-gray-500">{{ t('usage.totalCost') }}</p>
        <p class="text-xl font-bold text-green-600">
          ${{ (stats?.total_actual_cost || 0).toFixed(4) }}
        </p>
        <p class="text-xs text-gray-400">
          <template v-if="showAccountCost && totalAccountCost != null">
            <span class="text-orange-500">{{ t('usage.accountCost') }} ${{ totalAccountCost.toFixed(4) }}</span>
            <span> · </span>
          </template>
          <span>
            {{ t('usage.standardCost') }}
            <span :class="{ 'line-through': strikeStandardCost }">${{ (stats?.total_cost || 0).toFixed(4) }}</span>
          </span>
        </p>
      </div>
    </div>
    <div class="card p-4 flex items-center gap-3">
      <div class="rounded-lg bg-purple-100 p-2 dark:bg-purple-900/30 text-purple-600">
        <Icon name="clock" size="md" />
      </div>
      <div><p class="text-xs font-medium text-gray-500">{{ t('usage.avgDuration') }}</p><p class="text-xl font-bold">{{ formatDuration(stats?.average_duration_ms || 0) }}</p></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AdminUsageStatsResponse } from '@/api/admin/usage'
import type { UsageStatsResponse } from '@/types'
import Icon from '@/components/icons/Icon.vue'

const props = withDefaults(defineProps<{
  stats: (AdminUsageStatsResponse | UsageStatsResponse) | null
  showAccountCost?: boolean
  strikeStandardCost?: boolean
}>(), {
  showAccountCost: true,
  strikeStandardCost: false,
})

const { t } = useI18n()

const totalAccountCost = computed(() => {
  const stats = props.stats as (AdminUsageStatsResponse & { total_account_cost?: number }) | null
  return stats?.total_account_cost ?? null
})
const showAccountCost = computed(() => props.showAccountCost)
const strikeStandardCost = computed(() => props.strikeStandardCost)

const inputTokens = computed(() => props.stats?.total_input_tokens ?? 0)
const outputTokens = computed(() => props.stats?.total_output_tokens ?? 0)
const cacheReadTokens = computed(() => props.stats?.total_cache_read_tokens ?? 0)
const cacheCreationTokens = computed(() => props.stats?.total_cache_creation_tokens ?? 0)
const totalTokens = computed(() => props.stats?.total_tokens ?? 0)
const promptTokens = computed(() => inputTokens.value + cacheReadTokens.value + cacheCreationTokens.value)
const cacheHitRate = computed(() => {
  if (promptTokens.value <= 0) return null
  return `${((cacheReadTokens.value / promptTokens.value) * 100).toFixed(1)}%`
})

const formatDuration = (ms: number) =>
  ms < 1000 ? `${ms.toFixed(0)}ms` : `${(ms / 1000).toFixed(2)}s`

const formatTokens = (value: number) => {
  if (value >= 1e9) return (value / 1e9).toFixed(2) + 'B'
  if (value >= 1e6) return (value / 1e6).toFixed(2) + 'M'
  if (value >= 1e3) return (value / 1e3).toFixed(2) + 'K'
  return value.toLocaleString()
}
</script>

<style scoped>
.token-summary-card {
  container-type: inline-size;
}

.token-metric-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.5rem 0.75rem;
}

@container (min-width: 34rem) {
  .token-metric-grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}
</style>

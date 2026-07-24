<template>
  <dl class="usage-summary" data-testid="usage-summary">
    <div class="usage-summary__item usage-summary__requests" data-testid="usage-stat-requests">
      <dt class="usage-summary__label">{{ t('usage.totalRequests') }}</dt>
      <dd class="usage-summary__value">
        {{ stats?.total_requests?.toLocaleString() || '0' }}
      </dd>
      <dd class="usage-summary__meta">{{ t('usage.inSelectedRange') }}</dd>
    </div>

    <div class="usage-summary__item token-summary-card" data-testid="usage-stat-tokens">
      <dt class="usage-summary__label">{{ t('usage.totalTokens') }}</dt>
      <dd class="usage-summary__value">
        {{ formatTokens(totalTokens) }}
      </dd>

      <dd
        v-if="cacheHitRate != null"
        class="usage-cache-rate"
        data-testid="cache-hit-rate"
      >
        <span class="usage-cache-rate__label">{{ t('usage.cacheHitRate') }}</span>
        <span class="usage-cache-rate__value">{{ cacheHitRate }}</span>
        <span class="usage-cache-rate__formula">
          {{ formatTokens(cacheReadTokens) }} / {{ formatTokens(promptTokens) }}
        </span>
      </dd>

      <dd class="token-summary-card__breakdown">
        <dl class="token-metric-grid" data-testid="token-breakdown">
          <div class="min-w-0">
            <dt class="usage-token-label">{{ t('usage.in') }}</dt>
            <dd class="usage-token-value text-blue-700 dark:text-blue-300">
              {{ formatTokens(inputTokens) }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="usage-token-label">{{ t('usage.out') }}</dt>
            <dd class="usage-token-value text-violet-700 dark:text-violet-300">
              {{ formatTokens(outputTokens) }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="usage-token-label">{{ t('usage.cacheHit') }}</dt>
            <dd class="usage-token-value text-sky-700 dark:text-sky-300">
              {{ formatTokens(cacheReadTokens) }}
            </dd>
          </div>
          <div class="min-w-0">
            <dt class="usage-token-label">{{ t('usage.cacheCreate') }}</dt>
            <dd class="usage-token-value text-amber-700 dark:text-amber-300">
              {{ formatTokens(cacheCreationTokens) }}
            </dd>
          </div>
        </dl>
      </dd>
    </div>

    <div class="usage-summary__item usage-summary__cost" data-testid="usage-stat-cost">
      <dt class="usage-summary__label">{{ t('usage.totalCost') }}</dt>
      <dd class="usage-summary__value text-emerald-700 dark:text-emerald-300">
        ${{ (stats?.total_actual_cost || 0).toFixed(4) }}
      </dd>
      <dd class="usage-cost-detail">
        <span v-if="showAccountCost && totalAccountCost != null" data-testid="account-cost">
          <span class="usage-cost-detail__label">{{ t('usage.accountCost') }}</span>
          <span class="font-medium tabular-nums text-amber-700 dark:text-amber-300">
            ${{ totalAccountCost.toFixed(4) }}
          </span>
          <span class="usage-cost-detail__separator" aria-hidden="true">·</span>
        </span>
        <span>
          <span class="usage-cost-detail__label">{{ t('usage.standardCost') }}</span>
          <span
            data-testid="standard-cost"
            class="font-medium tabular-nums text-gray-500 dark:text-gray-300"
            :class="{ 'line-through': strikeStandardCost }"
          >
            ${{ (stats?.total_cost || 0).toFixed(4) }}
          </span>
        </span>
      </dd>
    </div>

    <div class="usage-summary__item usage-summary__duration" data-testid="usage-stat-duration">
      <dt class="usage-summary__label">{{ t('usage.avgDuration') }}</dt>
      <dd class="usage-summary__value">
        {{ formatDuration(stats?.average_duration_ms || 0) }}
      </dd>
      <dd class="usage-summary__meta">{{ t('usage.perRequest') }}</dd>
    </div>
  </dl>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AdminUsageStatsResponse } from '@/api/admin/usage'
import type { UsageStatsResponse } from '@/types'

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
.usage-summary {
  @apply grid overflow-hidden rounded-2xl border border-gray-200/80 bg-white shadow-card;
  @apply dark:border-dark-700 dark:bg-dark-800/70;
}

.usage-summary__item {
  @apply min-w-0 border-t border-gray-100 px-5 py-5 dark:border-dark-700;
}

.usage-summary__item:first-child {
  border-top: 0;
}

.usage-summary__label {
  @apply text-xs font-medium tracking-wide text-gray-500 dark:text-gray-400;
}

.usage-summary__value {
  @apply mt-1 text-2xl font-semibold leading-tight tracking-tight text-gray-900 tabular-nums dark:text-white;
}

.usage-summary__meta {
  @apply mt-2 text-xs text-gray-500 dark:text-gray-400;
}

.token-summary-card {
  container-type: inline-size;
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  align-content: start;
  align-items: start;
}

.token-summary-card > .usage-summary__label,
.token-summary-card > .usage-summary__value,
.token-summary-card > .usage-cache-rate {
  grid-column: 1;
}

.token-summary-card > .usage-cache-rate {
  justify-self: start;
  margin-top: 0.75rem;
  text-align: left;
}

.token-summary-card__breakdown {
  grid-column: 1 / -1;
  margin-top: 1rem;
}

.token-metric-grid {
  @apply grid grid-cols-2 gap-x-5 gap-y-3 border-t border-gray-100 pt-4 dark:border-dark-700;
}

.usage-token-label {
  @apply text-xs leading-4 text-gray-500 dark:text-gray-400;
}

.usage-token-value {
  @apply mt-0.5 truncate text-sm font-semibold tabular-nums;
}

.usage-cache-rate {
  @apply grid shrink-0 grid-cols-[auto_auto] items-baseline gap-x-2 rounded-lg bg-sky-50 px-3 py-2;
  @apply dark:bg-sky-950/40;
}

.usage-cache-rate__label {
  @apply text-xs text-sky-800 dark:text-sky-300;
}

.usage-cache-rate__value {
  @apply text-sm font-semibold tabular-nums text-sky-800 dark:text-sky-200;
}

.usage-cache-rate__formula {
  @apply col-span-2 mt-0.5 text-xs tabular-nums text-sky-700/70 dark:text-sky-300/70;
}

.usage-cost-detail {
  @apply mt-3 flex flex-wrap items-center gap-x-1.5 gap-y-1 text-xs text-gray-500 dark:text-gray-400;
}

.usage-cost-detail__label {
  @apply mr-1 text-gray-500 dark:text-gray-400;
}

.usage-cost-detail__separator {
  @apply ml-1 text-gray-300 dark:text-gray-600;
}

@media (min-width: 30rem) {
  .token-summary-card {
    grid-template-columns: minmax(0, 1fr) auto;
    column-gap: 1.25rem;
  }

  .token-summary-card > .usage-cache-rate {
    grid-column: 2;
    grid-row: 1 / span 2;
    justify-self: end;
    margin-top: 0;
    text-align: right;
  }
}

@container (min-width: 22rem) {
  .token-metric-grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}

@media (min-width: 48rem) {
  .usage-summary {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .usage-summary__item {
    border-top: 0;
    border-left: 1px solid rgb(243 244 246);
  }

  .usage-summary__item:nth-child(odd) {
    border-left: 0;
  }

  .usage-summary__item:nth-child(n + 3) {
    border-top: 1px solid rgb(243 244 246);
  }

  :global(.dark) .usage-summary__item {
    border-left-color: rgb(51 65 85);
  }

  :global(.dark) .usage-summary__item:nth-child(n + 3) {
    border-top-color: rgb(51 65 85);
  }
}

@media (min-width: 80rem) {
  .usage-summary {
    grid-template-columns:
      minmax(0, 0.85fr)
      minmax(26rem, 1.75fr)
      minmax(0, 1.15fr)
      minmax(0, 0.8fr);
  }

  .usage-summary__item,
  .usage-summary__item:nth-child(odd),
  .usage-summary__item:nth-child(n + 3) {
    border-top: 0;
    border-left: 1px solid rgb(243 244 246);
  }

  .usage-summary__item:first-child {
    border-left: 0;
  }

  :global(.dark) .usage-summary__item,
  :global(.dark) .usage-summary__item:nth-child(odd),
  :global(.dark) .usage-summary__item:nth-child(n + 3) {
    border-top: 0;
    border-left-color: rgb(51 65 85);
  }
}
</style>

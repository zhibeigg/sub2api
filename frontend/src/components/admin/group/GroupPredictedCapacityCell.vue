<template>
  <div
    v-if="loading"
    class="w-36 space-y-1 text-xs"
    data-testid="predicted-capacity-loading"
    aria-busy="true"
  >
    <div class="flex items-center justify-between gap-3">
      <span class="text-gray-400 dark:text-gray-500">{{ t('admin.groups.predictedCapacity.balance') }}</span>
      <span class="inline-block h-3 w-16 rounded-sm bg-gray-200 dark:bg-dark-600" />
    </div>
    <div class="flex items-center justify-between gap-3">
      <span class="text-gray-400 dark:text-gray-500">{{ t('admin.groups.predictedCapacity.requests') }}</span>
      <span class="inline-block h-3 w-16 rounded-sm bg-gray-200 dark:bg-dark-600" />
    </div>
  </div>

  <div
    v-else
    class="w-36 space-y-0.5 text-xs"
    :title="diagnosticTitle"
    data-testid="predicted-capacity-cell"
  >
    <div class="flex items-center justify-between gap-3 whitespace-nowrap">
      <span class="text-gray-400 dark:text-gray-500">{{ t('admin.groups.predictedCapacity.balance') }}</span>
      <span :class="balance.partial ? partialValueClass : valueClass" :title="balance.title">
        {{ balance.text }}
      </span>
    </div>
    <div class="flex items-center justify-between gap-3 whitespace-nowrap">
      <span class="text-gray-400 dark:text-gray-500">{{ t('admin.groups.predictedCapacity.requests') }}</span>
      <span :class="requests.partial ? partialValueClass : valueClass" :title="requests.title">
        {{ requests.text }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { GroupPredictedCapacitySummary } from '@/types'

const props = withDefaults(defineProps<{
  summary?: GroupPredictedCapacitySummary | null
  loading?: boolean
}>(), {
  summary: null,
  loading: false,
})

const { t } = useI18n()

const valueClass = 'font-medium tabular-nums text-gray-700 dark:text-gray-300'
const partialValueClass = 'font-medium tabular-nums text-amber-700 dark:text-amber-400'

const usdFormatter = new Intl.NumberFormat('en-US', {
  minimumFractionDigits: 0,
  maximumFractionDigits: 2,
})
const isFiniteNumber = (value: number | null | undefined): value is number =>
  typeof value === 'number' && Number.isFinite(value)

const parseRequestCount = (value: string | number | null | undefined): bigint | null => {
  if (typeof value === 'number') {
    return Number.isSafeInteger(value) && value >= 0 ? BigInt(value) : null
  }
  if (typeof value !== 'string' || !/^\d+$/.test(value.trim())) return null
  try {
    return BigInt(value.trim())
  } catch {
    return null
  }
}

const formatRequestCount = (value: bigint) =>
  value.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',')

const insufficient = () => ({
  text: t('admin.groups.predictedCapacity.insufficient'),
  partial: false,
  title: t('admin.groups.predictedCapacity.insufficient'),
})

const balance = computed(() => {
  const summary = props.summary
  if (!summary?.available) return insufficient()
  if (summary.balance_unlimited) {
    return {
      text: t('admin.groups.predictedCapacity.unlimited'),
      partial: false,
      title: t('admin.groups.predictedCapacity.unlimited'),
    }
  }
  if (summary.balance_complete && isFiniteNumber(summary.remaining_balance_usd)) {
    return {
      text: `≈ $${usdFormatter.format(summary.remaining_balance_usd)}`,
      partial: false,
      title: '',
    }
  }
  if (isFiniteNumber(summary.known_remaining_balance_usd)) {
    return {
      text: `≥ $${usdFormatter.format(summary.known_remaining_balance_usd)}`,
      partial: true,
      title: t('admin.groups.predictedCapacity.partialBalanceHint'),
    }
  }
  return insufficient()
})

const requests = computed(() => {
  const summary = props.summary
  if (!summary?.available) return insufficient()
  if (summary.requests_unlimited) {
    return {
      text: t('admin.groups.predictedCapacity.unlimited'),
      partial: false,
      title: t('admin.groups.predictedCapacity.unlimited'),
    }
  }
  const estimatedRequests = parseRequestCount(summary.estimated_remaining_requests)
  if (summary.requests_complete && estimatedRequests !== null) {
    return {
      text: `≈ ${formatRequestCount(estimatedRequests)} ${t('admin.groups.predictedCapacity.requestUnit')}`,
      partial: false,
      title: '',
    }
  }
  if (estimatedRequests !== null) {
    return {
      text: `≥ ${formatRequestCount(estimatedRequests)} ${t('admin.groups.predictedCapacity.requestUnit')}`,
      partial: true,
      title: t('admin.groups.predictedCapacity.partialRequestsHint'),
    }
  }
  return insufficient()
})

const diagnosticTitle = computed(() => {
  const summary = props.summary
  if (!summary?.available) return t('admin.groups.predictedCapacity.insufficient')

  const parts = [
    t('admin.groups.predictedCapacity.diagnostics', {
      knownRequests: summary.known_request_account_count,
      unknownRequests: summary.unknown_request_account_count,
      unknownAccounts: summary.unknown_account_count,
      staleAccounts: summary.stale_account_count,
      incompatibleAccounts: summary.incompatible_unit_account_count,
    }),
  ]
  if (!summary.balance_complete && balance.value.partial) {
    parts.unshift(t('admin.groups.predictedCapacity.partialBalanceHint'))
  }
  if (!summary.requests_complete && requests.value.partial) {
    parts.unshift(t('admin.groups.predictedCapacity.partialRequestsHint'))
  }
  if (summary.evaluated_at) {
    parts.push(t('admin.groups.predictedCapacity.evaluatedAt', {
      time: new Date(summary.evaluated_at).toLocaleString(),
    }))
  }
  return parts.join(' ')
})
</script>

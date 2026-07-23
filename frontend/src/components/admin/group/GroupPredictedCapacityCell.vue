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
      <span class="text-gray-400 dark:text-gray-500">{{ t('admin.groups.predictedCapacity.capacity') }}</span>
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
      <span :class="displayValueClass(balance)" :title="balance.title">
        {{ balance.text }}
      </span>
    </div>
    <div class="flex items-center justify-between gap-3 whitespace-nowrap">
      <span class="text-gray-400 dark:text-gray-500">{{ capacityLabel }}</span>
      <span :class="displayValueClass(capacity)" :title="capacity.title">
        {{ capacity.text }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { GroupPredictedCapacitySummary, PredictionUnit } from '@/types'

const props = withDefaults(defineProps<{
  summary?: GroupPredictedCapacitySummary | null
  loading?: boolean
  error?: boolean
}>(), {
  summary: null,
  loading: false,
  error: false,
})

const { t } = useI18n()

const valueClass = 'font-medium tabular-nums text-gray-700 dark:text-gray-300'
const partialValueClass = 'font-medium tabular-nums text-amber-700 dark:text-amber-400'
const errorValueClass = 'font-medium text-red-600 dark:text-red-400'

const usdFormatter = new Intl.NumberFormat('en-US', {
  minimumFractionDigits: 0,
  maximumFractionDigits: 2,
})
const isFiniteNumber = (value: number | null | undefined): value is number =>
  typeof value === 'number' && Number.isFinite(value)

const parseQuantity = (value: string | number | null | undefined): bigint | null => {
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

const formatQuantity = (value: bigint) =>
  value.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',')

type DisplayValue = {
  text: string
  partial: boolean
  error?: boolean
  title: string
}

const displayValueClass = (value: DisplayValue) => {
  if (value.error) return errorValueClass
  return value.partial ? partialValueClass : valueClass
}

const insufficient = (): DisplayValue => ({
  text: t('admin.groups.predictedCapacity.insufficient'),
  partial: false,
  title: t('admin.groups.predictedCapacity.insufficient'),
})

const failed = (): DisplayValue => ({
  text: t('admin.groups.predictedCapacity.error'),
  partial: false,
  error: true,
  title: t('admin.groups.predictedCapacity.errorHint'),
})

const balance = computed<DisplayValue>(() => {
  if (props.error) return failed()
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

const prediction = computed(() => {
  const summary = props.summary
  const hasGenericQuantity = summary
    ? Object.prototype.hasOwnProperty.call(summary, 'predicted_quantity')
    : false
  const quantity = parseQuantity(
    hasGenericQuantity ? summary?.predicted_quantity : summary?.estimated_remaining_requests,
  )
  const unit: PredictionUnit = summary?.prediction_unit === 'image' ? 'image' : 'request'
  const unlimited = summary?.prediction_unlimited ?? summary?.requests_unlimited ?? false

  return {
    unit,
    quantity,
    configured:
      summary?.prediction_configured ??
      (unlimited || quantity !== null),
    complete: summary?.prediction_complete ?? summary?.requests_complete ?? false,
    unlimited,
    knownAccountCount:
      summary?.known_prediction_account_count ?? summary?.known_request_account_count ?? 0,
    unknownAccountCount:
      summary?.unknown_prediction_account_count ?? summary?.unknown_request_account_count ?? 0,
  }
})

const capacityLabel = computed(() =>
  t(
    prediction.value.unit === 'image'
      ? 'admin.groups.predictedCapacity.images'
      : 'admin.groups.predictedCapacity.requests',
  ),
)

const capacityUnit = computed(() =>
  t(
    prediction.value.unit === 'image'
      ? 'admin.groups.predictedCapacity.imageUnit'
      : 'admin.groups.predictedCapacity.requestUnit',
  ),
)

const partialCapacityHint = computed(() =>
  t(
    prediction.value.unit === 'image'
      ? 'admin.groups.predictedCapacity.partialImagesHint'
      : 'admin.groups.predictedCapacity.partialRequestsHint',
  ),
)

const capacity = computed<DisplayValue>(() => {
  if (props.error) return failed()
  const summary = props.summary
  if (!summary?.available) return insufficient()
  if (prediction.value.unlimited) {
    return {
      text: t('admin.groups.predictedCapacity.unlimited'),
      partial: false,
      title: t('admin.groups.predictedCapacity.unlimited'),
    }
  }
  if (!prediction.value.configured) return insufficient()
  if (prediction.value.complete && prediction.value.quantity !== null) {
    return {
      text: `≈ ${formatQuantity(prediction.value.quantity)} ${capacityUnit.value}`,
      partial: false,
      title: '',
    }
  }
  if (prediction.value.quantity !== null) {
    return {
      text: `≥ ${formatQuantity(prediction.value.quantity)} ${capacityUnit.value}`,
      partial: true,
      title: partialCapacityHint.value,
    }
  }
  return insufficient()
})

const diagnosticTitle = computed(() => {
  if (props.error) return t('admin.groups.predictedCapacity.errorHint')
  const summary = props.summary
  if (!summary?.available) return t('admin.groups.predictedCapacity.insufficient')

  const parts = [
    t('admin.groups.predictedCapacity.diagnostics', {
      knownPredictions: prediction.value.knownAccountCount,
      unknownPredictions: prediction.value.unknownAccountCount,
      unknownAccounts: summary.unknown_account_count,
      staleAccounts: summary.stale_account_count,
      incompatibleAccounts: summary.incompatible_unit_account_count,
    }),
  ]
  if (!summary.balance_complete && balance.value.partial) {
    parts.unshift(t('admin.groups.predictedCapacity.partialBalanceHint'))
  }
  if (!prediction.value.complete && capacity.value.partial) {
    parts.unshift(partialCapacityHint.value)
  }
  if (isFiniteNumber(summary.prediction_unit_cost_usd)) {
    parts.push(t('admin.groups.predictedCapacity.unitCostDiagnostic', {
      cost: summary.prediction_unit_cost_usd,
      unit: capacityUnit.value,
    }))
  }
  if (summary.evaluated_at) {
    parts.push(t('admin.groups.predictedCapacity.evaluatedAt', {
      time: new Date(summary.evaluated_at).toLocaleString(),
    }))
  }
  return parts.join(' ')
})
</script>

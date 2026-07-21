<template>
  <div
    :class="[
      'group relative flex flex-col overflow-hidden rounded-2xl border transition-all',
      'hover:shadow-xl hover:-translate-y-0.5',
      borderClass,
      'bg-white dark:bg-dark-800',
    ]"
  >
    <!-- Colored top accent bar -->
    <div :class="['h-1.5', accentClass]" />

    <div class="flex flex-1 flex-col p-4">
      <!-- Header: name + badge + price -->
      <div class="mb-3 flex items-start justify-between gap-2">
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <h3 class="truncate text-base font-bold text-gray-900 dark:text-white">{{ plan.name }}</h3>
            <span :class="['shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium', badgeLightClass]">
              {{ pLabel }}
            </span>
          </div>
          <p v-if="plan.description" class="mt-0.5 text-xs leading-relaxed text-gray-500 dark:text-dark-400 line-clamp-2">
            {{ plan.description }}
          </p>
        </div>
        <div class="shrink-0 text-right">
          <div class="flex items-baseline gap-1">
            <span class="text-xs text-gray-400 dark:text-dark-500">{{ planCurrencySymbol }}</span>
            <span :class="['text-2xl font-extrabold tracking-tight', textClass]">{{ plan.price }}</span>
            <span v-if="plan.currency" class="text-xs font-medium text-gray-400 dark:text-dark-500">{{ plan.currency }}</span>
          </div>
          <span class="text-[11px] text-gray-400 dark:text-dark-500">/ {{ validitySuffix }}</span>
          <div v-if="plan.original_price" class="mt-0.5 flex items-center justify-end gap-1.5">
            <span class="text-xs text-gray-400 line-through dark:text-dark-500">{{ planCurrencySymbol }}{{ plan.original_price }}<template v-if="plan.currency"> {{ plan.currency }}</template></span>
            <span :class="['rounded px-1 py-0.5 text-[10px] font-semibold', discountClass]">{{ discountText }}</span>
          </div>
        </div>
      </div>

      <!-- Group quota info (compact) -->
      <div class="mb-3 grid grid-cols-2 gap-x-3 gap-y-1 rounded-lg bg-gray-50 px-3 py-2 text-xs dark:bg-dark-700/50">
        <div class="col-span-2 flex flex-wrap gap-1">
          <span
            v-for="group in displayGroups"
            :key="group.id"
            :class="['rounded px-1.5 py-0.5 text-[10px] font-medium', platformBadgeLightClass(group.platform)]"
          >
            {{ group.name }} · ×{{ group.rate_multiplier }}
          </span>
        </div>
        <div class="col-span-2 mt-1 text-[10px] font-medium uppercase tracking-wide text-gray-400 dark:text-dark-500">
          {{ quotaHeading }}
        </div>
        <div v-if="hasPeakRate" class="col-span-2 flex items-center justify-between gap-2">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.peakRate') }}</span>
          <span class="text-right font-medium text-amber-700 dark:text-amber-300">{{ peakRateDisplay }}</span>
        </div>
        <div v-if="quotaLimits.daily != null" class="flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.dailyLimit') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">${{ quotaLimits.daily }}</span>
        </div>
        <div v-if="quotaLimits.weekly != null" class="flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.weeklyLimit') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">${{ quotaLimits.weekly }}</span>
        </div>
        <div v-if="quotaLimits.monthly != null" class="flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.monthlyLimit') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">${{ quotaLimits.monthly }}</span>
        </div>
        <div v-if="planType === 'standard_quota'" data-test="plan-concurrency" class="col-span-2 flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.concurrency') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">
            {{ plan.concurrency_limit == null ? t('payment.planCard.noExtraConcurrencyLimit') : t('payment.planCard.concurrencyValue', { limit: plan.concurrency_limit }) }}
          </span>
        </div>
        <div v-if="quotaLimits.daily == null && quotaLimits.weekly == null && quotaLimits.monthly == null" class="flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.quota') }}</span>
          <span class="font-medium text-gray-700 dark:text-gray-300">{{ t('payment.planCard.unlimited') }}</span>
        </div>
        <div v-if="modelScopeLabels.length > 0" class="col-span-2 flex items-center justify-between">
          <span class="text-gray-400 dark:text-dark-500">{{ t('payment.planCard.models') }}</span>
          <div class="flex flex-wrap justify-end gap-1">
            <span v-for="scope in modelScopeLabels" :key="scope"
              class="rounded bg-gray-200/80 px-1.5 py-0.5 text-[10px] font-medium text-gray-600 dark:bg-dark-600 dark:text-gray-300">
              {{ scope }}
            </span>
          </div>
        </div>
      </div>

      <!-- Features list (compact) -->
      <div v-if="plan.features.length > 0" class="mb-3 space-y-1">
        <div v-for="feature in plan.features" :key="feature" class="flex items-start gap-1.5">
          <svg :class="['mt-0.5 h-3.5 w-3.5 flex-shrink-0', iconClass]" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
          </svg>
          <span class="text-xs text-gray-600 dark:text-gray-300">{{ feature }}</span>
        </div>
      </div>

      <div class="flex-1" />

      <!-- Subscribe Button -->
      <button
        type="button"
        :class="['w-full rounded-xl py-2.5 text-sm font-semibold transition-all active:scale-[0.98]', btnClass]"
        @click="emit('select', plan)"
      >
        {{ isRenewal ? t('payment.renewNow') : t('payment.subscribeNow') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SubscriptionPlan, SubscriptionPlanType } from '@/types/payment'
import type { UserSubscription } from '@/types'
import { useAppStore } from '@/stores/app'
import { hasPeakRate as groupHasPeakRate, formatPeakRateWindow, serverTimezoneLabel } from '@/utils/peak-rate'
import { currencySymbol } from '@/components/payment/currency'
import {
  platformAccentBarClass,
  platformBadgeLightClass,
  platformBorderClass,
  platformTextClass,
  platformIconClass,
  platformButtonClass,
  platformDiscountClass,
  platformLabel,
} from '@/utils/platformColors'

const props = defineProps<{ plan: SubscriptionPlan; activeSubscriptions?: UserSubscription[] }>()
const emit = defineEmits<{ select: [plan: SubscriptionPlan] }>()
const { t } = useI18n()

const planType = computed<SubscriptionPlanType>(() =>
  props.plan.plan_type || 'legacy_shared_subscription',
)

const planGroups = computed<SubscriptionPlan['groups']>(() => {
  if (props.plan.groups?.length) return props.plan.groups
  return [{
    id: props.plan.group_id,
    name: props.plan.group_name || t('payment.groupFallback', { id: props.plan.group_id }),
    platform: props.plan.group_platform || '',
    rate_multiplier: props.plan.rate_multiplier ?? 1,
    daily_limit_usd: props.plan.daily_limit_usd,
    weekly_limit_usd: props.plan.weekly_limit_usd,
    monthly_limit_usd: props.plan.monthly_limit_usd,
  }]
})
const displayGroups = computed(() =>
  planType.value === 'subscription' ? planGroups.value.slice(0, 1) : planGroups.value,
)
const planGroupIds = computed(() => {
  const ids = props.plan.group_ids?.length ? props.plan.group_ids : displayGroups.value.map(group => group.id)
  return planType.value === 'subscription' ? ids.slice(0, 1) : ids
})
const platform = computed(() => displayGroups.value[0]?.platform || '')
const quotaHeading = computed(() =>
  t(planType.value === 'subscription' ? 'payment.planCard.nativeQuota' : 'payment.planCard.sharedQuota'),
)
const quotaLimits = computed(() => {
  const source = planType.value === 'subscription' ? displayGroups.value[0] : props.plan
  return {
    daily: source?.daily_limit_usd ?? null,
    weekly: source?.weekly_limit_usd ?? null,
    monthly: source?.monthly_limit_usd ?? null,
  }
})

function sameGroupSet(left: number[], right: number[]): boolean {
  const normalizedLeft = [...new Set(left)].sort((a, b) => a - b)
  const normalizedRight = [...new Set(right)].sort((a, b) => a - b)
  return normalizedLeft.length === normalizedRight.length
    && normalizedLeft.every((id, index) => id === normalizedRight[index])
}

const isRenewal = computed(() =>
  props.activeSubscriptions?.some(subscription => {
    if (subscription.status !== 'active') return false
    if (subscription.source_plan_id != null) {
      return subscription.source_plan_id === props.plan.id
    }
    const subscriptionGroupIds = subscription.group_ids?.length
      ? subscription.group_ids
      : [subscription.group_id]
    return sameGroupSet(subscriptionGroupIds, planGroupIds.value)
  }) ?? false
)

// Derived color classes from central config
const accentClass = computed(() => platformAccentBarClass(platform.value))
const borderClass = computed(() => platformBorderClass(platform.value))
const badgeLightClass = computed(() => platformBadgeLightClass(platform.value))
const textClass = computed(() => platformTextClass(platform.value))
const iconClass = computed(() => platformIconClass(platform.value))
const btnClass = computed(() => platformButtonClass(platform.value))
const discountClass = computed(() => platformDiscountClass(platform.value))
const pLabel = computed(() => platformLabel(platform.value))

const discountText = computed(() => {
  if (!props.plan.original_price || props.plan.original_price <= 0) return ''
  const pct = Math.round((1 - props.plan.price / props.plan.original_price) * 100)
  return pct > 0 ? `-${pct}%` : ''
})

const appStore = useAppStore()
const planCurrencySymbol = computed(() => currencySymbol(props.plan.currency || 'USD'))

const hasPeakRate = computed(() => displayGroups.value.some(group => groupHasPeakRate(group)))

const peakRateDisplay = computed(() => {
  const timezone = serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset)
  return displayGroups.value
    .filter(group => groupHasPeakRate(group))
    .map(group => `${group.name}: ${formatPeakRateWindow(group, timezone)}`)
    .join(' · ')
})

const MODEL_SCOPE_LABELS: Record<string, string> = {
  claude: 'Claude',
  gemini_text: 'Gemini',
  gemini_image: 'Imagen',
}

const modelScopeLabels = computed(() => {
  if (platform.value !== 'antigravity') return []
  const scopes = planType.value === 'subscription'
    ? displayGroups.value[0]?.supported_model_scopes
    : props.plan.supported_model_scopes
  if (!scopes || scopes.length === 0) return []
  return scopes.map(s => MODEL_SCOPE_LABELS[s] || s)
})

const validitySuffix = computed(() => {
  const u = props.plan.validity_unit || 'day'
  if (u === 'month') return t('payment.perMonth')
  if (u === 'year') return t('payment.perYear')
  return `${props.plan.validity_days}${t('payment.days')}`
})
</script>

<template>
  <BaseDialog :show="show" :title="plan ? t('payment.admin.editPlan') : t('payment.admin.createPlan')" width="wide" @close="emit('close')">
    <form id="plan-form" @submit.prevent="handleSavePlan" class="space-y-4">
      <div>
        <label class="input-label">{{ t('payment.admin.planName') }} <span class="text-red-500">*</span></label>
        <input v-model="planForm.name" type="text" class="input" required />
      </div>

      <div>
        <label class="input-label">{{ t('payment.admin.planType') }} <span class="text-red-500">*</span></label>
        <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <label
            v-for="option in planTypeOptions"
            :key="option.value"
            :class="[
              'flex cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors',
              planForm.plan_type === option.value
                ? 'border-primary-500 bg-primary-50 dark:bg-primary-900/20'
                : 'border-gray-200 hover:border-gray-300 dark:border-dark-600 dark:hover:border-dark-500'
            ]"
          >
            <input
              :data-test="`plan-type-${option.value}`"
              type="radio"
              name="subscription-plan-type"
              :value="option.value"
              :checked="planForm.plan_type === option.value"
              class="mt-0.5 h-4 w-4 border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500"
              @change="handlePlanTypeChange(option.value)"
            />
            <span>
              <span class="block text-sm font-medium text-gray-900 dark:text-white">{{ option.label }}</span>
              <span class="mt-0.5 block text-xs text-gray-500 dark:text-gray-400">{{ option.description }}</span>
            </span>
          </label>
        </div>
      </div>

      <div
        v-if="isLegacyPlan"
        data-test="legacy-plan-warning"
        class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-200"
      >
        {{ t('payment.admin.legacyPlanConversionWarning') }}
      </div>

      <GroupSelector
        v-model="planForm.group_ids"
        :groups="availableGroups"
        :multiple="planForm.plan_type !== 'subscription'"
        searchable
      />

      <!-- Group Info Preview -->
      <div v-if="selectedGroupInfos.length" class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-800">
        <p class="mb-2 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('payment.admin.selectedGroupsHint') }}</p>
        <div class="flex flex-wrap gap-2">
          <GroupBadge
            v-for="group in selectedGroupInfos"
            :key="group.id"
            :name="group.name"
            :platform="group.platform"
            :rate-multiplier="group.rate_multiplier"
          />
        </div>
      </div>

      <div v-if="isStandardQuotaPlan" data-test="shared-quota-section">
        <label class="input-label">{{ t('payment.admin.sharedQuotaLimits') }}</label>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <div>
            <label class="mb-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.dailyLimit') }}</label>
            <input v-model.number="planForm.daily_limit_usd" data-test="daily-limit" type="number" step="0.01" min="0.01" class="input" :placeholder="t('payment.admin.unlimited')" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.weeklyLimit') }}</label>
            <input v-model.number="planForm.weekly_limit_usd" data-test="weekly-limit" type="number" step="0.01" min="0.01" class="input" :placeholder="t('payment.admin.unlimited')" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.monthlyLimit') }}</label>
            <input v-model.number="planForm.monthly_limit_usd" data-test="monthly-limit" type="number" step="0.01" min="0.01" class="input" :placeholder="t('payment.admin.unlimited')" />
          </div>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.sharedQuotaHint') }}</p>
      </div>

      <div><label class="input-label">{{ t('payment.admin.planDescription') }} <span class="text-red-500">*</span></label><textarea v-model="planForm.description" rows="2" class="input" required></textarea></div>
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label">{{ t('payment.admin.price') }} <span class="text-red-500">*</span></label>
          <input v-model.number="planForm.price" data-test="plan-price" type="number" step="0.01" min="0.01" class="input" required />
          <p v-if="subscriptionCnyPreview" class="mt-1 text-xs font-medium text-primary-600 dark:text-primary-400">
            {{ t('payment.admin.subscriptionCnyPayPreview', { amount: subscriptionCnyPreview.amount }) }}
            <span v-if="subscriptionCnyPreview.feeRate > 0">
              {{ t('payment.admin.subscriptionCnyPayPreviewWithFee', { feeRate: subscriptionCnyPreview.feeRate, total: subscriptionCnyPreview.total }) }}
            </span>
          </p>
        </div>
        <div><label class="input-label">{{ t('payment.admin.originalPrice') }}</label><input v-model.number="planForm.original_price" type="number" step="0.01" min="0.01" class="input" /></div>
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div><label class="input-label">{{ t('payment.admin.validity') }} <span class="text-red-500">*</span></label><input v-model.number="planForm.validity_days" type="number" min="1" class="input" required /></div>
        <div><label class="input-label">{{ t('payment.admin.validityUnit') }} <span class="text-red-500">*</span></label><Select v-model="planForm.validity_unit" :options="validityUnitOptions" /></div>
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div><label class="input-label">{{ t('payment.admin.sortOrder') }}</label><input v-model.number="planForm.sort_order" type="number" min="0" class="input" /></div>
        <div>
          <label class="input-label">{{ t('payment.admin.currency') }}</label>
          <input v-model="planForm.currency" type="text" maxlength="3" class="input uppercase" :placeholder="t('payment.admin.currencyPlaceholder')" />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.currencyHint') }}</p>
        </div>
      </div>
      <div>
        <label class="input-label">{{ t('payment.admin.features') }}</label>
        <textarea v-model="planFeaturesText" rows="3" class="input" :placeholder="t('payment.admin.featuresPlaceholder')"></textarea>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.featuresHint') }}</p>
      </div>
      <div class="flex items-center gap-3">
        <label class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.admin.forSale') }}</label>
        <button
          type="button"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            planForm.for_sale ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'
          ]"
          @click="planForm.for_sale = !planForm.for_sale"
        >
          <span :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            planForm.for_sale ? 'translate-x-5' : 'translate-x-0'
          ]" />
        </button>
      </div>
    </form>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" @click="emit('close')" class="btn btn-secondary">{{ t('common.cancel') }}</button>
        <button type="submit" form="plan-form" :disabled="saving" class="btn btn-primary">{{ saving ? t('common.saving') : t('common.save') }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import type { AdminPaymentConfig } from '@/api/admin/payment'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatPaymentAmount } from '@/components/payment/currency'
import type { SubscriptionPlan, SubscriptionPlanType } from '@/types/payment'
import type { AdminGroup } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'

const props = defineProps<{
  show: boolean
  plan: SubscriptionPlan | null
  groups: AdminGroup[]
  paymentConfig?: AdminPaymentConfig | null
}>()

const emit = defineEmits<{
  close: []
  saved: []
}>()

const { t } = useI18n()
const appStore = useAppStore()

const saving = ref(false)
type QuotaInput = number | null | ''
type FormalSubscriptionPlanType = Exclude<SubscriptionPlanType, 'legacy_shared_subscription'>
const planForm = reactive({
  name: '',
  plan_type: 'subscription' as SubscriptionPlanType,
  group_ids: [] as number[],
  daily_limit_usd: null as QuotaInput,
  weekly_limit_usd: null as QuotaInput,
  monthly_limit_usd: null as QuotaInput,
  description: '',
  price: 0,
  original_price: 0,
  currency: '',
  validity_days: 30,
  validity_unit: 'days',
  sort_order: 0,
  for_sale: true,
})
const planFeaturesText = ref('')

const validityUnitOptions = computed(() => [
  { value: 'days', label: t('payment.admin.days') },
  { value: 'weeks', label: t('payment.admin.weeks') },
  { value: 'months', label: t('payment.admin.months') },
])

const planTypeOptions = computed(() => [
  {
    value: 'subscription' as const,
    label: t('payment.admin.planTypes.subscription'),
    description: t('payment.admin.planTypeDescriptions.subscription'),
  },
  {
    value: 'standard_quota' as const,
    label: t('payment.admin.planTypes.standardQuota'),
    description: t('payment.admin.planTypeDescriptions.standardQuota'),
  },
])

const originalPlanType = computed<SubscriptionPlanType>(() =>
  props.plan?.plan_type || 'legacy_shared_subscription',
)
const isLegacyPlan = computed(() => Boolean(props.plan) && originalPlanType.value === 'legacy_shared_subscription')
const isStandardQuotaPlan = computed(() => planForm.plan_type === 'standard_quota')

const availableGroups = computed(() => {
  if (planForm.plan_type === 'subscription') {
    return props.groups.filter(group => group.subscription_type === 'subscription')
  }
  if (planForm.plan_type === 'standard_quota') {
    return props.groups.filter(group => group.subscription_type === 'standard')
  }
  return []
})

const selectedGroupInfos = computed(() => {
  const selected = new Set(planForm.group_ids)
  return props.groups.filter(g => selected.has(g.id))
})

function roundCnyAmount(value: number): number {
  return Math.round(value * 100) / 100
}

function ceilCnyAmount(value: number): number {
  return Math.ceil(value * 100) / 100
}

const subscriptionCnyPreview = computed(() => {
  const price = Number(planForm.price) || 0
  const rate = Number(props.paymentConfig?.subscription_usd_to_cny_rate) || 0
  if (price <= 0 || rate <= 0) return null

  const amount = roundCnyAmount(price * rate)
  const feeRate = Number(props.paymentConfig?.recharge_fee_rate) || 0
  const fee = feeRate > 0 ? ceilCnyAmount((amount * feeRate) / 100) : 0
  const total = feeRate > 0 ? roundCnyAmount(amount + fee) : amount

  return {
    amount: formatPaymentAmount(amount, 'CNY'),
    feeRate,
    total: formatPaymentAmount(total, 'CNY'),
  }
})

// Reset form when dialog opens
watch(() => props.show, (visible) => {
  if (!visible) return
  if (props.plan) {
    const planType = originalPlanType.value
    const groupIds = props.plan.group_ids?.length ? props.plan.group_ids : [props.plan.group_id].filter(id => id > 0)
    Object.assign(planForm, {
      name: props.plan.name,
      plan_type: planType,
      group_ids: planType === 'subscription' ? groupIds.slice(0, 1) : [...groupIds],
      daily_limit_usd: planType === 'subscription' ? null : (props.plan.daily_limit_usd ?? null),
      weekly_limit_usd: planType === 'subscription' ? null : (props.plan.weekly_limit_usd ?? null),
      monthly_limit_usd: planType === 'subscription' ? null : (props.plan.monthly_limit_usd ?? null),
      description: props.plan.description,
      price: props.plan.price,
      original_price: props.plan.original_price || 0,
      currency: props.plan.currency || '',
      validity_days: props.plan.validity_days,
      validity_unit: props.plan.validity_unit || 'days',
      sort_order: props.plan.sort_order || 0,
      for_sale: props.plan.for_sale,
    })
    planFeaturesText.value = (props.plan.features || []).join('\n')
  } else {
    Object.assign(planForm, {
      name: '',
      plan_type: 'subscription',
      group_ids: [],
      daily_limit_usd: null,
      weekly_limit_usd: null,
      monthly_limit_usd: null,
      description: '',
      price: 0,
      original_price: 0,
      currency: '',
      validity_days: 30,
      validity_unit: 'days',
      sort_order: 0,
      for_sale: true,
    })
    planFeaturesText.value = ''
  }
})

function handlePlanTypeChange(planType: FormalSubscriptionPlanType) {
  if (planForm.plan_type === planType) return

  planForm.plan_type = planType
  const compatibleGroupIds = new Set(
    props.groups
      .filter(group => group.subscription_type === (planType === 'subscription' ? 'subscription' : 'standard'))
      .map(group => group.id),
  )
  planForm.group_ids = planForm.group_ids.filter(groupId => compatibleGroupIds.has(groupId))
  if (planType === 'subscription' && planForm.group_ids.length > 1) {
    planForm.group_ids = planForm.group_ids.slice(0, 1)
  }
  planForm.daily_limit_usd = null
  planForm.weekly_limit_usd = null
  planForm.monthly_limit_usd = null
}

function normalizeQuota(value: QuotaInput): number | null {
  if (value === '' || value == null) return null
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : null
}

/** Build request payload with snake_case keys matching backend JSON tags */
function buildPlanPayload() {
  const features = planFeaturesText.value.split('\n').map(f => f.trim()).filter(Boolean).join('\n')
  const groupIds = planForm.plan_type === 'subscription'
    ? planForm.group_ids.slice(0, 1)
    : [...planForm.group_ids]
  const quotaLimits = planForm.plan_type === 'standard_quota'
    ? {
        daily_limit_usd: normalizeQuota(planForm.daily_limit_usd),
        weekly_limit_usd: normalizeQuota(planForm.weekly_limit_usd),
        monthly_limit_usd: normalizeQuota(planForm.monthly_limit_usd),
      }
    : {
        daily_limit_usd: null,
        weekly_limit_usd: null,
        monthly_limit_usd: null,
      }
  return {
    name: planForm.name,
    plan_type: planForm.plan_type,
    group_id: groupIds[0] ?? null,
    group_ids: groupIds,
    ...quotaLimits,
    ...(props.plan ? { quota_limits_set: true } : {}),
    description: planForm.description,
    price: planForm.price,
    original_price: planForm.original_price || 0,
    currency: planForm.currency.trim().toUpperCase(),
    validity_days: planForm.validity_days,
    validity_unit: planForm.validity_unit,
    sort_order: planForm.sort_order,
    for_sale: planForm.for_sale,
    features,
  }
}

async function handleSavePlan() {
  if (planForm.plan_type === 'legacy_shared_subscription') {
    appStore.showError(t('payment.admin.formalPlanTypeRequired'))
    return
  }
  if (planForm.group_ids.length === 0) {
    appStore.showError(t('payment.admin.groupRequired'))
    return
  }
  if (planForm.plan_type === 'standard_quota') {
    const quotaLimits = [
      normalizeQuota(planForm.daily_limit_usd),
      normalizeQuota(planForm.weekly_limit_usd),
      normalizeQuota(planForm.monthly_limit_usd),
    ]
    if (!quotaLimits.some(limit => limit != null && limit > 0)) {
      appStore.showError(t('payment.admin.standardQuotaRequired'))
      return
    }
  }
  if (!planForm.price || planForm.price <= 0) {
    appStore.showError(t('payment.admin.priceRequired'))
    return
  }
  if (!planForm.validity_days || planForm.validity_days < 1) {
    appStore.showError(t('payment.admin.validityRequired'))
    return
  }
  saving.value = true
  try {
    const data = buildPlanPayload()
    if (props.plan) { await adminPaymentAPI.updatePlan(props.plan.id, data) }
    else { await adminPaymentAPI.createPlan(data) }
    appStore.showSuccess(t('common.saved'))
    emit('close')
    emit('saved')
  } catch (err: unknown) { appStore.showError(extractApiErrorMessage(err, t('common.error'))) }
  finally { saving.value = false }
}
</script>

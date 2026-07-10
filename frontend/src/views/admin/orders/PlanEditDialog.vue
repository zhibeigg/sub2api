<template>
  <BaseDialog :show="show" :title="plan ? t('payment.admin.editPlan') : t('payment.admin.createPlan')" width="wide" @close="emit('close')">
    <form id="plan-form" @submit.prevent="handleSavePlan" class="space-y-4">
      <div>
        <label class="input-label">{{ t('payment.admin.planName') }} <span class="text-red-500">*</span></label>
        <input v-model="planForm.name" type="text" class="input" required />
      </div>

      <GroupSelector v-model="planForm.group_ids" :groups="subscriptionGroups" searchable />

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

      <div>
        <label class="input-label">{{ t('payment.admin.sharedQuotaLimits') }}</label>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <div>
            <label class="mb-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.dailyLimit') }}</label>
            <input v-model.number="planForm.daily_limit_usd" data-test="daily-limit" type="number" step="0.01" min="0" class="input" :placeholder="t('payment.admin.unlimited')" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.weeklyLimit') }}</label>
            <input v-model.number="planForm.weekly_limit_usd" data-test="weekly-limit" type="number" step="0.01" min="0" class="input" :placeholder="t('payment.admin.unlimited')" />
          </div>
          <div>
            <label class="mb-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.monthlyLimit') }}</label>
            <input v-model.number="planForm.monthly_limit_usd" data-test="monthly-limit" type="number" step="0.01" min="0" class="input" :placeholder="t('payment.admin.unlimited')" />
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
        <div><label class="input-label">{{ t('payment.admin.originalPrice') }}</label><input v-model.number="planForm.original_price" type="number" step="0.01" min="0" class="input" /></div>
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div><label class="input-label">{{ t('payment.admin.validityDays') }} <span class="text-red-500">*</span></label><input v-model.number="planForm.validity_days" type="number" min="1" class="input" required /></div>
        <div><label class="input-label">{{ t('payment.admin.validityUnit') }} <span class="text-red-500">*</span></label><Select v-model="planForm.validity_unit" :options="validityUnitOptions" /></div>
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div><label class="input-label">{{ t('payment.admin.sortOrder') }}</label><input v-model.number="planForm.sort_order" type="number" min="0" class="input" /></div>
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
import type { SubscriptionPlan } from '@/types/payment'
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
const planForm = reactive({
  name: '',
  group_ids: [] as number[],
  daily_limit_usd: null as QuotaInput,
  weekly_limit_usd: null as QuotaInput,
  monthly_limit_usd: null as QuotaInput,
  description: '',
  price: 0,
  original_price: 0,
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

const subscriptionGroups = computed(() =>
  props.groups.filter(g => g.subscription_type === 'subscription'),
)

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
    const groupIds = props.plan.group_ids?.length ? props.plan.group_ids : [props.plan.group_id].filter(id => id > 0)
    Object.assign(planForm, {
      name: props.plan.name,
      group_ids: [...groupIds],
      daily_limit_usd: props.plan.daily_limit_usd ?? null,
      weekly_limit_usd: props.plan.weekly_limit_usd ?? null,
      monthly_limit_usd: props.plan.monthly_limit_usd ?? null,
      description: props.plan.description,
      price: props.plan.price,
      original_price: props.plan.original_price || 0,
      validity_days: props.plan.validity_days,
      validity_unit: props.plan.validity_unit || 'days',
      sort_order: props.plan.sort_order || 0,
      for_sale: props.plan.for_sale,
    })
    planFeaturesText.value = (props.plan.features || []).join('\n')
  } else {
    Object.assign(planForm, {
      name: '',
      group_ids: [],
      daily_limit_usd: null,
      weekly_limit_usd: null,
      monthly_limit_usd: null,
      description: '',
      price: 0,
      original_price: 0,
      validity_days: 30,
      validity_unit: 'days',
      sort_order: 0,
      for_sale: true,
    })
    planFeaturesText.value = ''
  }
})

function normalizeQuota(value: QuotaInput): number | null {
  if (value === '' || value == null) return null
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : null
}

/** Build request payload with snake_case keys matching backend JSON tags */
function buildPlanPayload() {
  const features = planFeaturesText.value.split('\n').map(f => f.trim()).filter(Boolean).join('\n')
  const groupIds = [...planForm.group_ids]
  return {
    name: planForm.name,
    group_id: groupIds[0] ?? null,
    group_ids: groupIds,
    daily_limit_usd: normalizeQuota(planForm.daily_limit_usd),
    weekly_limit_usd: normalizeQuota(planForm.weekly_limit_usd),
    monthly_limit_usd: normalizeQuota(planForm.monthly_limit_usd),
    ...(props.plan ? { quota_limits_set: true } : {}),
    description: planForm.description,
    price: planForm.price,
    original_price: planForm.original_price || 0,
    validity_days: planForm.validity_days,
    validity_unit: planForm.validity_unit,
    sort_order: planForm.sort_order,
    for_sale: planForm.for_sale,
    features,
  }
}

async function handleSavePlan() {
  if (planForm.group_ids.length === 0) {
    appStore.showError(t('payment.admin.groupRequired'))
    return
  }
  if (!planForm.price || planForm.price <= 0) {
    appStore.showError(t('payment.admin.priceRequired'))
    return
  }
  if (!planForm.validity_days || planForm.validity_days < 1) {
    appStore.showError(t('payment.admin.validityDaysRequired'))
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

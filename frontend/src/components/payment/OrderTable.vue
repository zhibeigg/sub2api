<template>
  <DataTable :columns="columns" :data="orders" :loading="loading">
    <template #cell-id="{ value }">
      <span class="font-mono text-sm">#{{ value }}</span>
    </template>
    <template #cell-out_trade_no="{ value }">
      <span class="text-sm text-gray-900 dark:text-white">{{ value }}</span>
    </template>
    <template v-if="showUser" #cell-user_email="{ value, row }">
      <div class="text-sm">
        <span class="text-gray-900 dark:text-white">{{ value || row.user_name || '#' + row.user_id }}</span>
        <span v-if="row.user_notes" class="ml-1 text-xs text-gray-400">({{ row.user_notes }})</span>
      </div>
    </template>
    <template #cell-pay_amount="{ value, row }">
      <div class="text-sm">
        <span class="font-medium text-gray-900 dark:text-white">{{ paymentAmountSymbol(row) }}{{ value.toFixed(2) }}</span>
        <span v-if="row.fee_rate > 0" class="ml-1 text-xs text-gray-400" :title="t('payment.orders.fee') + ': ' + row.fee_rate + '%'">
          ({{ t('payment.orders.fee') }} {{ row.fee_rate }}%)
        </span>
        <div v-if="row.amount !== row.pay_amount" class="text-xs text-gray-500">
          {{ t('payment.orders.creditedAmount') }}: {{ creditedAmountSymbol }}{{ row.amount.toFixed(2) }}
        </div>
      </div>
    </template>
    <template #cell-signup_promo_attribution="{ row }">
      <div class="max-w-44 text-sm">
        <span class="font-mono font-medium text-gray-800 dark:text-gray-200">{{ promoAttributionLabel(row) }}</span>
        <span v-if="row.signup_promo_code_id" class="ml-1 text-xs text-gray-400">#{{ row.signup_promo_code_id }}</span>
      </div>
    </template>
    <template #cell-order_type="{ value }">
      <span class="text-sm text-gray-700 dark:text-gray-300">
        {{ value === 'subscription' ? t('payment.admin.subscriptionOrder') : t('payment.admin.balanceOrder') }}
      </span>
    </template>
    <template #cell-net_recharge_amount="{ row }">
      <div class="text-sm">
        <span class="font-medium text-gray-900 dark:text-white">{{ creditedAmountSymbol }}{{ formatAmount(row.net_recharge_amount) }}</span>
        <div v-if="row.first_recharge_bonus_applied" class="text-xs text-emerald-600 dark:text-emerald-400">
          {{ t('payment.admin.firstRechargeBonus') }} ×{{ formatMultiplier(row.recharge_bonus_multiplier) }}
        </div>
      </div>
    </template>
    <template #cell-payment_type="{ value }">
      <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.methods.' + value, value) }}</span>
    </template>
    <template #cell-status="{ value }">
      <OrderStatusBadge :status="value" />
    </template>
    <template #cell-created_at="{ value }">
      <span class="text-xs text-gray-500 dark:text-gray-400">{{ formatDate(value) }}</span>
    </template>
    <template #cell-actions="{ row }">
      <slot name="actions" :row="row" />
    </template>
  </DataTable>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PaymentOrder } from '@/types/payment'
import type { Column } from '@/components/common/types'
import DataTable from '@/components/common/DataTable.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'
import { currencySymbol } from '@/components/payment/currency'

const { t } = useI18n()

const props = defineProps<{
  orders: PaymentOrder[]
  loading: boolean
  showUser?: boolean
  showAttribution?: boolean
}>()

function formatDate(dateStr: string) { return new Date(dateStr).toLocaleString() }

function formatAmount(value: number | undefined): string {
  return Number(value || 0).toFixed(2)
}

function formatMultiplier(value: number | undefined): string {
  return Number(value || 1).toFixed(2).replace(/\.00$/, '')
}

function promoAttributionLabel(order: PaymentOrder): string {
  if (order.signup_promo_attribution === 'attributed') {
    return order.signup_promo_code || t('payment.admin.promoAttributed')
  }
  if (order.signup_promo_attribution === 'legacy_unknown') {
    return t('payment.admin.promoLegacyUnknown')
  }
  return t('payment.admin.promoNone')
}

const creditedAmountSymbol = currencySymbol('USD')

function paymentAmountSymbol(order: PaymentOrder): string {
  return currencySymbol(order.currency)
}

const columns = computed((): Column[] => {
  const cols: Column[] = [
    { key: 'id', label: t('payment.orders.orderId') },
    { key: 'out_trade_no', label: t('payment.orders.orderNo') },
  ]
  if (props.showUser) {
    cols.push({ key: 'user_email', label: t('payment.admin.colUser') })
  }
  if (props.showAttribution) {
    cols.push(
      { key: 'signup_promo_attribution', label: t('payment.admin.signupPromoCode') },
      { key: 'order_type', label: t('payment.admin.orderType') },
    )
  }
  cols.push(
    { key: 'pay_amount', label: t('payment.orders.payAmount') },
    { key: 'payment_type', label: t('payment.orders.paymentMethod') },
    { key: 'status', label: t('payment.orders.status') },
    ...(props.showAttribution ? [{ key: 'net_recharge_amount', label: t('payment.admin.netRecharge') }] : []),
    { key: 'created_at', label: t('payment.orders.createdAt') },
    { key: 'actions', label: t('common.actions') },
  )
  return cols
})
</script>

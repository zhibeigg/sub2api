<template>
  <AppLayout>
    <div class="space-y-4 sm:space-y-6">
      <section class="space-y-3">
        <div class="grid grid-cols-2 gap-3 lg:grid-cols-5">
          <div v-for="card in summaryCards" :key="card.key" class="card min-w-0 p-4">
            <p class="truncate text-xs font-medium text-gray-500 dark:text-gray-400">{{ card.label }}</p>
            <div class="mt-2 flex items-start justify-between gap-2">
              <div class="min-w-0">
                <p class="break-words text-xl font-semibold tracking-tight text-gray-900 dark:text-white sm:text-2xl">
                  <span v-if="summaryLoading" class="inline-block h-7 w-24 animate-pulse rounded bg-gray-200 dark:bg-dark-700"></span>
                  <template v-else>{{ card.value }}</template>
                </p>
                <p v-if="!summaryLoading && card.hint" class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ card.hint }}</p>
              </div>
              <Icon :name="card.icon" size="md" class="shrink-0 text-gray-400 dark:text-gray-500" />
            </div>
          </div>
        </div>
        <p class="px-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
          {{ t('payment.admin.summaryScopeHint') }}
          <span v-if="summary">{{ t('payment.admin.summaryCountHint', { orders: summary.totals.filtered_order_count, users: summary.totals.recharged_user_count }) }}</span>
        </p>
      </section>

      <section class="card overflow-visible">
        <div class="flex flex-wrap items-center border-b border-gray-200 px-2 dark:border-dark-700 sm:px-4">
          <button
            v-for="view in viewOptions"
            :key="view.value"
            type="button"
            class="-mb-px inline-flex items-center gap-1.5 border-b-2 px-3 py-3 text-sm font-medium transition-colors sm:px-4"
            :class="activeView === view.value
              ? 'border-primary-500 text-primary-600 dark:text-primary-400'
              : 'border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700 dark:text-gray-400 dark:hover:border-dark-500 dark:hover:text-gray-200'"
            @click="activeView = view.value"
          >
            <Icon :name="view.icon" size="sm" />
            {{ view.label }}
          </button>
        </div>

        <div class="space-y-3 border-b border-gray-100 p-3 dark:border-dark-700/60 sm:p-4">
          <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
            <input
              v-model="orderSearch"
              type="text"
              :placeholder="t('payment.admin.searchOrders')"
              class="input"
              @input="debounceFilterRefresh"
            />
            <DateRangePicker
              v-model:start-date="orderFilters.start_date"
              v-model:end-date="orderFilters.end_date"
              @change="handleFilterChange"
            />
            <Select v-model="orderFilters.time_field" :options="timeFieldOptions" @change="handleFilterChange" />
            <Select
              v-model="promoSelection"
              :options="promoFilterOptions"
              searchable
              clearable
              :search-placeholder="t('payment.admin.searchPromoCode')"
              :placeholder="t('payment.admin.allPromoCodes')"
              @change="handleFilterChange"
            >
              <template #option="{ option }">
                <div class="min-w-0 flex-1">
                  <div class="truncate">{{ option.label }}</div>
                  <div v-if="option.description" class="truncate text-xs text-gray-400">{{ option.description }}</div>
                </div>
              </template>
            </Select>
          </div>

          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Select v-model="orderFilters.status" :options="statusFilterOptions" @change="handleFilterChange" />
            <Select v-model="orderFilters.payment_type" :options="paymentTypeFilterOptions" @change="handleFilterChange" />
            <Select v-model="orderFilters.order_type" :options="orderTypeFilterOptions" @change="handleFilterChange" />
            <div class="flex flex-wrap items-center justify-end gap-2">
              <button type="button" class="btn btn-ghost flex-1 sm:flex-none" :disabled="isRefreshing" @click="resetFilters">
                {{ t('common.reset') }}
              </button>
              <button type="button" class="btn btn-secondary flex-1 sm:flex-none" :disabled="isRefreshing" @click="refreshFilteredData(false)">
                <Icon name="refresh" size="sm" :class="isRefreshing ? 'animate-spin' : ''" />
                <span>{{ t('common.refresh') }}</span>
              </button>
              <button type="button" class="btn btn-secondary flex-1 sm:flex-none" :disabled="exporting" @click="handleExport(activeView === 'orders' ? 'orders' : 'attribution')">
                <Icon name="download" size="sm" :class="exporting ? 'animate-pulse' : ''" />
                <span>{{ activeView === 'orders' ? t('payment.admin.exportOrders') : t('payment.admin.exportAttribution') }}</span>
              </button>
            </div>
          </div>
        </div>

        <div v-show="activeView === 'orders'" class="overflow-hidden rounded-b-2xl">
          <OrderTable :orders="orders" :loading="ordersLoading" show-user show-attribution>
            <template #actions="{ row }">
              <div class="flex flex-wrap items-center gap-1">
                <button @click="showOrderDetail(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-600">
                  <Icon name="eye" size="sm" />
                  {{ t('common.view') }}
                </button>
                <button v-if="row.status === 'PENDING'" @click="handleCancelOrder(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-yellow-600 hover:bg-yellow-50 dark:text-yellow-400 dark:hover:bg-yellow-900/20">
                  <Icon name="x" size="sm" />
                  {{ t('payment.orders.cancel') }}
                </button>
                <button v-if="row.status === 'FAILED'" @click="handleRetryOrder(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20">
                  <Icon name="refresh" size="sm" />
                  {{ t('payment.admin.retry') }}
                </button>
                <template v-if="row.status === 'REFUND_REQUESTED'">
                  <span v-if="row.refund_amount" class="rounded-full bg-purple-100 px-1.5 py-0.5 text-xs font-medium text-purple-700 dark:bg-purple-900/30 dark:text-purple-300">{{ creditedAmountSymbol }}{{ row.refund_amount.toFixed(2) }}</span>
                  <button @click="openRefundDialog(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-purple-600 hover:bg-purple-50 dark:text-purple-400 dark:hover:bg-purple-900/20">
                    <Icon name="check" size="sm" />
                    {{ t('payment.admin.approveRefund') }}
                  </button>
                </template>
                <button v-else-if="row.status === 'REFUND_FAILED'" @click="openRefundDialog(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-purple-600 hover:bg-purple-50 dark:text-purple-400 dark:hover:bg-purple-900/20">
                  <Icon name="refresh" size="sm" />
                  {{ t('payment.admin.retryRefund') }}
                </button>
                <button v-else-if="row.status === 'REFUND_PENDING'" :disabled="refundQueryingIds.has(row.id)" @click="handleQueryRefund(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-orange-600 hover:bg-orange-50 disabled:opacity-60 dark:text-orange-400 dark:hover:bg-orange-900/20">
                  <Icon name="refresh" size="sm" :class="refundQueryingIds.has(row.id) ? 'animate-spin' : ''" />
                  {{ t('payment.admin.queryRefundStatus') }}
                </button>
                <button v-else-if="row.status === 'COMPLETED' || row.status === 'PARTIALLY_REFUNDED'" @click="openRefundDialog(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20">
                  <Icon name="dollar" size="sm" />
                  {{ t('payment.admin.refund') }}
                </button>
              </div>
            </template>
          </OrderTable>
          <Pagination v-if="orderPagination.total > 0" :page="orderPagination.page" :total="orderPagination.total" :page-size="orderPagination.page_size" @update:page="handleOrderPageChange" @update:pageSize="handleOrderPageSizeChange" />
        </div>

        <div v-show="activeView === 'attribution'" class="overflow-hidden rounded-b-2xl">
          <DataTable
            :columns="attributionColumns"
            :data="attributionGroups"
            :loading="summaryLoading"
            :row-key="attributionRowKey"
            :sticky-actions-column="false"
            clickable-rows
            @row-click="filterByAttributionGroup"
          >
            <template #cell-promo_code="{ row }">
              <div class="max-w-56">
                <div class="font-medium text-gray-900 dark:text-white">{{ attributionLabel(row) }}</div>
                <div v-if="row.promo_code_id" class="font-mono text-xs text-gray-400">#{{ row.promo_code_id }}</div>
              </div>
            </template>
            <template #cell-gross_recharge_amount="{ value }"><span class="font-medium">{{ formatUsd(value) }}</span></template>
            <template #cell-refunded_amount="{ value }"><span class="text-red-600 dark:text-red-400">{{ formatUsd(value) }}</span></template>
            <template #cell-net_recharge_amount="{ value }"><span class="font-semibold text-emerald-600 dark:text-emerald-400">{{ formatUsd(value) }}</span></template>
          </DataTable>
          <Pagination v-if="groupPagination.total > 0" :page="groupPagination.page" :total="groupPagination.total" :page-size="groupPagination.page_size" @update:page="handleGroupPageChange" @update:pageSize="handleGroupPageSizeChange" />
        </div>
      </section>
    </div>

    <BaseDialog :show="showDetailDialog" :title="t('payment.admin.orderDetail')" width="wide" @close="showDetailDialog = false">
      <div v-if="selectedOrder" class="space-y-4">
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.orderId') }}</p><p class="font-mono text-sm font-medium text-gray-900 dark:text-white">#{{ selectedOrder.id }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.orderNo') }}</p><p class="break-all text-sm font-medium text-gray-900 dark:text-white">{{ selectedOrder.out_trade_no }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.status') }}</p><OrderStatusBadge :status="selectedOrder.status" /></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.amount') }}</p><p class="text-sm font-medium text-gray-900 dark:text-white">{{ creditedAmountSymbol }}{{ selectedOrder.amount.toFixed(2) }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.payAmount') }}</p><p class="text-sm font-medium text-gray-900 dark:text-white">{{ paymentAmountSymbol(selectedOrder) }}{{ selectedOrder.pay_amount.toFixed(2) }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.netRecharge') }}</p><p class="text-sm font-medium text-emerald-600 dark:text-emerald-400">{{ creditedAmountSymbol }}{{ Number(selectedOrder.net_recharge_amount || 0).toFixed(2) }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.signupPromoCode') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ orderPromoLabel(selectedOrder) }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.paymentMethod') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.methods.' + selectedOrder.payment_type, selectedOrder.payment_type) }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.feeRate') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ selectedOrder.fee_rate }}%</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.createdAt') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(selectedOrder.created_at) }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.expiresAt') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(selectedOrder.expires_at) }}</p></div>
          <div v-if="selectedOrder.paid_at"><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.paidAt') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(selectedOrder.paid_at) }}</p></div>
          <div v-if="selectedOrder.refund_amount"><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.refundAmount') }}</p><p class="text-sm font-medium text-red-600 dark:text-red-400">{{ creditedAmountSymbol }}{{ selectedOrder.refund_amount.toFixed(2) }}</p></div>
          <div v-if="selectedOrder.refund_reason" class="sm:col-span-2"><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.refundReason') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ selectedOrder.refund_reason }}</p></div>
          <div v-if="selectedOrder.refund_requested_at" class="border-t border-gray-200 pt-3 dark:border-dark-600 sm:col-span-2">
            <p class="mb-2 text-xs font-medium text-purple-600 dark:text-purple-400">{{ t('payment.admin.refundRequestInfo') }}</p>
            <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.refundRequestedAt') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(selectedOrder.refund_requested_at) }}</p></div>
              <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.refundRequestedBy') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">#{{ selectedOrder.refund_requested_by }}</p></div>
              <div class="sm:col-span-2"><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.refundRequestReason') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ selectedOrder.refund_request_reason }}</p></div>
            </div>
          </div>
        </div>
        <div v-if="orderAuditLogs.length > 0" class="border-t border-gray-200 pt-4 dark:border-dark-600">
          <p class="mb-2 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('payment.admin.auditLogs') }}</p>
          <div class="max-h-48 space-y-2 overflow-y-auto">
            <div v-for="log in orderAuditLogs" :key="log.id" class="rounded-lg border border-gray-100 bg-gray-50 p-2.5 dark:border-dark-600 dark:bg-dark-800">
              <div class="flex items-center justify-between gap-3"><span class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ log.action }}</span><span class="text-xs text-gray-400">{{ formatDateTime(log.created_at) }}</span></div>
              <div v-if="log.detail" class="mt-1 break-all text-xs text-gray-500 dark:text-gray-400">{{ log.detail }}</div>
              <div v-if="log.operator" class="mt-1 text-xs text-gray-400">{{ t('payment.admin.operator') }}: {{ log.operator }}</div>
            </div>
          </div>
        </div>
      </div>
    </BaseDialog>

    <AdminRefundDialog :show="showRefundDialog" :order="selectedOrder" :submitting="refundSubmitting" @confirm="handleRefund" @cancel="showRefundDialog = false" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatOrderDateTime } from '@/components/payment/orderUtils'
import type {
  AdminOrderAttributionGroup,
  AdminOrderFilters,
  AdminOrderPaidAmount,
  AdminOrderPromoCodeOption,
  AdminOrderSummary,
  PaymentOrder,
  PromoAttribution,
} from '@/types/payment'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import DataTable from '@/components/common/DataTable.vue'
import Icon from '@/components/icons/Icon.vue'
import AdminRefundDialog from '@/components/admin/payment/AdminRefundDialog.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'
import OrderTable from '@/components/payment/OrderTable.vue'
import { currencySymbol, formatPaymentAmount } from '@/components/payment/currency'

interface AuditLog {
  id: number
  action: string
  detail: string | null
  operator: string | null
  created_at: string
}

type OrdersView = 'orders' | 'attribution'

const { t } = useI18n()
const appStore = useAppStore()
const creditedAmountSymbol = currencySymbol('USD')
const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'

const activeView = ref<OrdersView>('orders')
const ordersLoading = ref(false)
const summaryLoading = ref(false)
const exporting = ref(false)
const orders = ref<PaymentOrder[]>([])
const summary = ref<AdminOrderSummary | null>(null)
const orderSearch = ref('')
const promoSelection = ref<string | number | null>('all')
const promoCodeOptions = ref<AdminOrderPromoCodeOption[]>([])
const orderFilters = reactive({
  status: '',
  payment_type: '',
  order_type: '',
  start_date: '',
  end_date: '',
  time_field: 'paid_at' as 'created_at' | 'paid_at',
})
const orderPagination = reactive({ page: 1, page_size: 20, total: 0 })
const groupPagination = reactive({ page: 1, page_size: 20, total: 0 })
const selectedOrder = ref<PaymentOrder | null>(null)
const showDetailDialog = ref(false)
const showRefundDialog = ref(false)
const refundSubmitting = ref(false)
const refundQueryingIds = ref(new Set<number>())
const orderAuditLogs = ref<AuditLog[]>([])

let ordersController: AbortController | null = null
let summaryController: AbortController | null = null
let promoController: AbortController | null = null
let debounceTimer: ReturnType<typeof setTimeout> | null = null
let ordersRequestSequence = 0
let summaryRequestSequence = 0

const isRefreshing = computed(() => ordersLoading.value || summaryLoading.value)
const attributionGroups = computed(() => summary.value?.groups || [])

const viewOptions = computed<Array<{ value: OrdersView; label: string; icon: 'menu' | 'chart' }>>(() => [
  { value: 'orders', label: t('payment.admin.orderDetailsView'), icon: 'menu' },
  { value: 'attribution', label: t('payment.admin.promoAttributionView'), icon: 'chart' },
])

const summaryCards = computed<Array<{ key: string; label: string; value: string; hint?: string; icon: 'dollar' | 'creditCard' | 'refresh' | 'check' }>>(() => {
  const totals = summary.value?.totals
  return [
    {
      key: 'paid',
      label: t('payment.admin.paidAmount'),
      value: formatPaidAmounts(totals?.paid_amounts),
      hint: t('payment.admin.paidOrderCount', { count: totals?.paid_order_count || 0 }),
      icon: 'creditCard',
    },
    { key: 'net', label: t('payment.admin.netRecharge'), value: formatUsd(totals?.net_recharge_amount), icon: 'dollar' },
    { key: 'gross', label: t('payment.admin.grossRecharge'), value: formatUsd(totals?.gross_recharge_amount), icon: 'creditCard' },
    { key: 'refund', label: t('payment.admin.refundedTotal'), value: formatUsd(totals?.refunded_amount), icon: 'refresh' },
    { key: 'success', label: t('payment.admin.successfulRechargeOrders'), value: String(totals?.successful_order_count || 0), icon: 'check' },
  ]
})

const attributionColumns = computed<Column[]>(() => [
  { key: 'promo_code', label: t('payment.admin.signupPromoCode') },
  { key: 'order_user_count', label: t('payment.admin.orderUsers') },
  { key: 'recharged_user_count', label: t('payment.admin.rechargedUsers') },
  { key: 'successful_order_count', label: t('payment.admin.successfulRechargeOrders') },
  { key: 'gross_recharge_amount', label: t('payment.admin.grossRecharge') },
  { key: 'refunded_amount', label: t('payment.admin.refundedTotal') },
  { key: 'net_recharge_amount', label: t('payment.admin.netRecharge') },
])

const statusFilterOptions = computed(() => [
  { value: '', label: t('payment.admin.allStatuses') },
  ...['PENDING', 'PAID', 'RECHARGING', 'COMPLETED', 'EXPIRED', 'CANCELLED', 'FAILED', 'REFUND_REQUESTED', 'REFUNDING', 'REFUND_PENDING', 'PARTIALLY_REFUNDED', 'REFUNDED', 'REFUND_FAILED']
    .map((value) => ({ value, label: t(`payment.status.${value.toLowerCase()}`) })),
])

const paymentTypeFilterOptions = computed(() => [
  { value: '', label: t('payment.admin.allPaymentTypes') },
  ...['alipay', 'wxpay', 'alipay_direct', 'wxpay_direct', 'stripe', 'easypay', 'airwallex']
    .map((value) => ({ value, label: t(`payment.methods.${value}`, value) })),
])

const orderTypeFilterOptions = computed(() => [
  { value: '', label: t('payment.admin.allOrderTypes') },
  { value: 'balance', label: t('payment.admin.balanceOrder') },
  { value: 'subscription', label: t('payment.admin.subscriptionOrder') },
])

const timeFieldOptions = computed(() => [
  { value: 'paid_at', label: t('payment.admin.filterByPaidAt') },
  { value: 'created_at', label: t('payment.admin.filterByCreatedAt') },
])

const promoFilterOptions = computed(() => {
  const options: Array<Record<string, unknown>> = [{ value: 'all', label: t('payment.admin.allPromoCodes') }]
  for (const option of promoCodeOptions.value) {
    if (option.promo_attribution === 'none') {
      options.push({ value: 'none', label: t('payment.admin.promoNone') })
    } else if (option.promo_attribution === 'legacy_unknown') {
      options.push({ value: 'legacy_unknown', label: t('payment.admin.promoLegacyUnknown') })
    } else if (option.promo_code_id) {
      options.push({
        value: `promo:${option.promo_code_id}`,
        label: option.promo_code || `#${option.promo_code_id}`,
        description: option.historical
          ? t('payment.admin.historicalPromoCode')
          : option.status ? t('payment.admin.promoCodeStatus', { status: option.status }) : '',
      })
    }
  }
  return options
})

function buildFilters(): AdminOrderFilters {
  const filters: AdminOrderFilters = {
    keyword: orderSearch.value.trim() || undefined,
    status: orderFilters.status || undefined,
    payment_type: orderFilters.payment_type || undefined,
    order_type: orderFilters.order_type || undefined,
    start_date: orderFilters.start_date || undefined,
    end_date: orderFilters.end_date || undefined,
    timezone,
    time_field: orderFilters.time_field,
  }
  if (promoSelection.value === 'none' || promoSelection.value === 'legacy_unknown') {
    filters.promo_attribution = promoSelection.value as PromoAttribution
  } else if (typeof promoSelection.value === 'string' && promoSelection.value.startsWith('promo:')) {
    const id = Number(promoSelection.value.slice(6))
    if (Number.isInteger(id) && id > 0) filters.promo_code_id = id
  }
  return filters
}

function isCancelled(error: unknown): boolean {
  const candidate = error as { code?: string; name?: string }
  return candidate?.code === 'ERR_CANCELED' || candidate?.name === 'AbortError'
}

async function loadOrders() {
  ordersController?.abort()
  const controller = new AbortController()
  const requestSequence = ++ordersRequestSequence
  ordersController = controller
  ordersLoading.value = true
  try {
    const response = await adminPaymentAPI.getOrders({
      ...buildFilters(),
      page: orderPagination.page,
      page_size: orderPagination.page_size,
    }, { signal: controller.signal })
    if (controller.signal.aborted || requestSequence !== ordersRequestSequence) return
    orders.value = response.data.items || []
    orderPagination.total = response.data.total || 0
  } catch (error: unknown) {
    if (!isCancelled(error) && requestSequence === ordersRequestSequence) {
      appStore.showError(extractI18nErrorMessage(error, t, 'payment.errors', t('common.error')))
    }
  } finally {
    if (ordersController === controller) {
      ordersController = null
      ordersLoading.value = false
    }
  }
}

async function loadSummary() {
  summaryController?.abort()
  const controller = new AbortController()
  const requestSequence = ++summaryRequestSequence
  summaryController = controller
  summaryLoading.value = true
  try {
    const response = await adminPaymentAPI.getOrderSummary({
      ...buildFilters(),
      group_page: groupPagination.page,
      group_page_size: groupPagination.page_size,
    }, { signal: controller.signal })
    if (controller.signal.aborted || requestSequence !== summaryRequestSequence) return
    summary.value = response.data
    groupPagination.page = response.data.group_page || groupPagination.page
    groupPagination.page_size = response.data.group_page_size || groupPagination.page_size
    groupPagination.total = response.data.group_total || 0
  } catch (error: unknown) {
    if (!isCancelled(error) && requestSequence === summaryRequestSequence) {
      appStore.showError(extractI18nErrorMessage(error, t, 'payment.errors', t('common.error')))
    }
  } finally {
    if (summaryController === controller) {
      summaryController = null
      summaryLoading.value = false
    }
  }
}

async function loadPromoCodeOptions(search?: string) {
  promoController?.abort()
  const controller = new AbortController()
  promoController = controller
  try {
    const response = await adminPaymentAPI.getOrderPromoCodeOptions({ search: search || undefined, limit: 100 }, { signal: controller.signal })
    if (!controller.signal.aborted && promoController === controller) promoCodeOptions.value = response.data || []
  } catch (error: unknown) {
    if (!isCancelled(error)) appStore.showError(extractI18nErrorMessage(error, t, 'payment.errors', t('common.error')))
  } finally {
    if (promoController === controller) promoController = null
  }
}

async function refreshFilteredData(resetPages = true) {
  if (resetPages) {
    orderPagination.page = 1
    groupPagination.page = 1
  }
  await Promise.allSettled([loadOrders(), loadSummary()])
}

function handleFilterChange() {
  void refreshFilteredData(true)
}

function resetFilters() {
  orderSearch.value = ''
  promoSelection.value = 'all'
  orderFilters.status = ''
  orderFilters.payment_type = ''
  orderFilters.order_type = ''
  orderFilters.start_date = ''
  orderFilters.end_date = ''
  orderFilters.time_field = 'paid_at'
  void refreshFilteredData(true)
}

function filterByAttributionGroup(group: AdminOrderAttributionGroup) {
  if (group.promo_attribution === 'attributed' && group.promo_code_id) {
    promoSelection.value = `promo:${group.promo_code_id}`
  } else {
    promoSelection.value = group.promo_attribution
  }
  activeView.value = 'orders'
  void refreshFilteredData(true)
}

function debounceFilterRefresh() {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => void refreshFilteredData(true), 300)
}

function handleOrderPageChange(page: number) {
  orderPagination.page = page
  void loadOrders()
}

function handleOrderPageSizeChange(size: number) {
  orderPagination.page_size = size
  orderPagination.page = 1
  void loadOrders()
}

function handleGroupPageChange(page: number) {
  groupPagination.page = page
  void loadSummary()
}

function handleGroupPageSizeChange(size: number) {
  groupPagination.page_size = size
  groupPagination.page = 1
  void loadSummary()
}

async function refreshAfterMutation() {
  await Promise.allSettled([loadOrders(), loadSummary()])
}

async function handleExport(mode: 'orders' | 'attribution') {
  exporting.value = true
  try {
    const blob = await adminPaymentAPI.exportOrders(mode, buildFilters())
    const date = new Date().toISOString().slice(0, 10)
    saveAs(blob, mode === 'orders' ? `payment-orders-${date}.csv` : `payment-order-attribution-${date}.csv`)
  } catch (error: unknown) {
    appStore.showError(extractI18nErrorMessage(error, t, 'payment.errors', t('common.error')))
  } finally {
    exporting.value = false
  }
}

function formatPaidAmounts(amounts: AdminOrderPaidAmount[] | undefined): string {
  if (!amounts?.length) return formatPaymentAmount(0, 'CNY')
  const showCurrencyCode = amounts.length > 1
  return amounts.map((item) => {
    const formatted = formatPaymentAmount(Number(item.amount || 0), item.currency)
    return showCurrencyCode ? `${formatted} ${item.currency}` : formatted
  }).join(' · ')
}

function formatUsd(value: string | number | undefined): string {
  const amount = Number(value || 0)
  return `${creditedAmountSymbol}${Number.isFinite(amount) ? amount.toFixed(2) : '0.00'}`
}

function paymentAmountSymbol(order: PaymentOrder | null | undefined): string {
  return currencySymbol(order?.currency)
}

function attributionLabel(group: AdminOrderAttributionGroup): string {
  if (group.promo_attribution === 'attributed') return group.promo_code || t('payment.admin.promoAttributed')
  if (group.promo_attribution === 'legacy_unknown') return t('payment.admin.promoLegacyUnknown')
  return t('payment.admin.promoNone')
}

function orderPromoLabel(order: PaymentOrder): string {
  const label = attributionLabel({ promo_attribution: order.signup_promo_attribution || 'none' } as AdminOrderAttributionGroup)
  if (order.signup_promo_attribution !== 'attributed') return label
  return `${order.signup_promo_code || label}${order.signup_promo_code_id ? ` (#${order.signup_promo_code_id})` : ''}`
}

function attributionRowKey(row: AdminOrderAttributionGroup): string {
  return `${row.promo_attribution}:${row.promo_code_id || 'none'}`
}

async function showOrderDetail(order: PaymentOrder) {
  selectedOrder.value = order
  orderAuditLogs.value = []
  showDetailDialog.value = true
  try {
    const response = await adminPaymentAPI.getOrder(order.id)
    const data = response.data as unknown as Record<string, unknown>
    if (data.order) selectedOrder.value = data.order as PaymentOrder
    orderAuditLogs.value = (data.auditLogs || data.audit_logs || []) as AuditLog[]
  } catch {
    // Keep the cached row visible when detail enrichment fails.
  }
}

async function handleCancelOrder(order: PaymentOrder) {
  try {
    await adminPaymentAPI.cancelOrder(order.id)
    appStore.showSuccess(t('payment.admin.orderCancelled'))
    await refreshAfterMutation()
  } catch (error: unknown) {
    appStore.showError(extractI18nErrorMessage(error, t, 'payment.errors', t('common.error')))
  }
}

async function handleRetryOrder(order: PaymentOrder) {
  try {
    await adminPaymentAPI.retryRecharge(order.id)
    appStore.showSuccess(t('payment.admin.retrySuccess'))
    await refreshAfterMutation()
  } catch (error: unknown) {
    appStore.showError(extractI18nErrorMessage(error, t, 'payment.errors', t('common.error')))
  }
}

function openRefundDialog(order: PaymentOrder) {
  selectedOrder.value = order
  showRefundDialog.value = true
}

function isRefundPendingWarning(warning: string | undefined): boolean {
  return /pending|处理中|待/.test(String(warning || '').toLowerCase())
}

async function handleRefund(data: { amount: number; reason: string; deduct_balance: boolean; force: boolean }) {
  if (!selectedOrder.value) return
  refundSubmitting.value = true
  try {
    const response = await adminPaymentAPI.refundOrder(selectedOrder.value.id, data)
    if (response.data.success || isRefundPendingWarning(response.data.warning)) {
      appStore.showSuccess(response.data.success ? t('payment.admin.refundSuccess') : t('payment.admin.refundPending'))
      showRefundDialog.value = false
      await refreshAfterMutation()
      return
    }
    appStore.showError(response.data.warning || t('common.error'))
  } catch (error: unknown) {
    appStore.showError(extractI18nErrorMessage(error, t, 'payment.errors', t('common.error')))
  } finally {
    refundSubmitting.value = false
  }
}

async function handleQueryRefund(order: PaymentOrder) {
  refundQueryingIds.value = new Set(refundQueryingIds.value).add(order.id)
  try {
    const response = await adminPaymentAPI.queryRefund(order.id)
    if (response.data.success) appStore.showSuccess(t('payment.admin.refundSuccess'))
    else if (isRefundPendingWarning(response.data.warning)) appStore.showSuccess(t('payment.admin.refundPending'))
    else appStore.showError(response.data.warning || t('common.error'))
    await refreshAfterMutation()
  } catch (error: unknown) {
    appStore.showError(extractI18nErrorMessage(error, t, 'payment.errors', t('common.error')))
  } finally {
    const next = new Set(refundQueryingIds.value)
    next.delete(order.id)
    refundQueryingIds.value = next
  }
}

function formatDateTime(dateStr: string): string {
  return formatOrderDateTime(dateStr)
}

onMounted(() => {
  void loadPromoCodeOptions()
  void refreshFilteredData(false)
})

onUnmounted(() => {
  if (debounceTimer) clearTimeout(debounceTimer)
  ordersController?.abort()
  summaryController?.abort()
  promoController?.abort()
})
</script>

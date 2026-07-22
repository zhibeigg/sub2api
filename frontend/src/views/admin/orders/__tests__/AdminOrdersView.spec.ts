import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import AdminOrdersView from '../AdminOrdersView.vue'
import { adminPaymentAPI } from '@/api/admin/payment'

const { showError, showSuccess, saveAs } = vi.hoisted(() => ({
  showError: vi.fn(),
  showSuccess: vi.fn(),
  saveAs: vi.fn(),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('file-saver', () => ({ saveAs }))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('@/api/admin/payment', () => {
  const api = {
    getOrders: vi.fn(),
    getOrderSummary: vi.fn(),
    getOrderPromoCodeOptions: vi.fn(),
    exportOrders: vi.fn(),
    getOrder: vi.fn(),
    cancelOrder: vi.fn(),
    retryRecharge: vi.fn(),
    refundOrder: vi.fn(),
    queryRefund: vi.fn(),
  }
  return { adminPaymentAPI: api, default: api }
})

const order = {
  id: 1,
  user_id: 2,
  amount: 10,
  pay_amount: 10,
  fee_rate: 0,
  payment_type: 'stripe',
  out_trade_no: 'order-1',
  status: 'PENDING',
  order_type: 'balance',
  created_at: '2026-01-01T00:00:00Z',
  expires_at: '2026-01-01T01:00:00Z',
  refund_amount: 0,
  signup_promo_attribution: 'none',
  net_recharge_amount: 0,
} as const

const summary = {
  totals: {
    filtered_order_count: 1,
    paid_order_count: 1,
    paid_amounts: [{ currency: 'CNY', order_count: 1, amount: '12.34' }],
    successful_order_count: 0,
    recharged_user_count: 0,
    gross_recharge_amount: '0',
    refunded_amount: '0',
    net_recharge_amount: '0',
  },
  groups: [{
    promo_attribution: 'none',
    order_user_count: 1,
    recharged_user_count: 0,
    successful_order_count: 0,
    gross_recharge_amount: '0',
    refunded_amount: '0',
    net_recharge_amount: '0',
  }],
  group_page: 1,
  group_page_size: 20,
  group_total: 1,
} as const

const DateRangePickerStub = defineComponent({
  name: 'DateRangePicker',
  emits: ['update:startDate', 'update:endDate', 'change'],
  template: '<button data-test="date-filter" @click="$emit(\'update:startDate\', \'2026-01-01\'); $emit(\'update:endDate\', \'2026-01-31\'); $emit(\'change\')">date</button>',
})

const PaginationStub = defineComponent({
  name: 'Pagination',
  props: ['page', 'total', 'pageSize'],
  emits: ['update:page', 'update:pageSize'],
  template: '<button class="pagination-stub" @click="$emit(\'update:page\', 2)">page</button>',
})

const OrderTableStub = defineComponent({
  name: 'OrderTable',
  props: ['orders', 'loading'],
  template: '<div><div v-for="row in orders" :key="row.id"><slot name="actions" :row="row" /></div></div>',
})

const DataTableStub = defineComponent({
  name: 'DataTable',
  props: ['data'],
  emits: ['rowClick'],
  template: '<button v-if="data?.length" data-test="attribution-row" @click="$emit(\'rowClick\', data[0])">row</button>',
})

function mountView() {
  return mount(AdminOrdersView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        DateRangePicker: DateRangePickerStub,
        Pagination: PaginationStub,
        OrderTable: OrderTableStub,
        Select: { template: '<div />' },
        DataTable: DataTableStub,
        BaseDialog: { template: '<div />' },
        AdminRefundDialog: { template: '<div />' },
        OrderStatusBadge: { template: '<span />' },
        Icon: { template: '<span />' },
      },
    },
  })
}

beforeEach(() => {
  showError.mockReset()
  showSuccess.mockReset()
  saveAs.mockReset()
  vi.mocked(adminPaymentAPI.getOrders).mockReset().mockResolvedValue({
    data: { items: [order], total: 1, page: 1, page_size: 20, pages: 1 },
  } as never)
  vi.mocked(adminPaymentAPI.getOrderSummary).mockReset().mockResolvedValue({ data: summary } as never)
  vi.mocked(adminPaymentAPI.getOrderPromoCodeOptions).mockReset().mockResolvedValue({
    data: [{ promo_attribution: 'none' }, { promo_attribution: 'legacy_unknown' }],
  } as never)
  vi.mocked(adminPaymentAPI.cancelOrder).mockReset().mockResolvedValue({ data: {} } as never)
})

describe('AdminOrdersView reporting interactions', () => {
  it('loads orders and reconciliation totals using payment time by default', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(adminPaymentAPI.getOrders).toHaveBeenCalledWith(
      expect.objectContaining({ time_field: 'paid_at' }),
      { signal: expect.any(AbortSignal) },
    )
    expect(adminPaymentAPI.getOrderSummary).toHaveBeenCalledWith(
      expect.objectContaining({ time_field: 'paid_at' }),
      { signal: expect.any(AbortSignal) },
    )
    expect(adminPaymentAPI.getOrderPromoCodeOptions).toHaveBeenCalledWith(
      { search: undefined, limit: 100 },
      { signal: expect.any(AbortSignal) },
    )
    expect(wrapper.text()).toContain('12.34')
  })

  it('refreshes only the order list when order pagination changes', async () => {
    const wrapper = mountView()
    await flushPromises()
    vi.mocked(adminPaymentAPI.getOrders).mockClear()
    vi.mocked(adminPaymentAPI.getOrderSummary).mockClear()

    await wrapper.findAll('.pagination-stub')[0].trigger('click')
    await flushPromises()

    expect(adminPaymentAPI.getOrders).toHaveBeenCalledTimes(1)
    expect(adminPaymentAPI.getOrderSummary).not.toHaveBeenCalled()
    expect(adminPaymentAPI.getOrders).toHaveBeenCalledWith(
      expect.objectContaining({ page: 2 }),
      { signal: expect.any(AbortSignal) },
    )
  })

  it('refreshes orders and summary together when a filter changes', async () => {
    const wrapper = mountView()
    await flushPromises()
    vi.mocked(adminPaymentAPI.getOrders).mockClear()
    vi.mocked(adminPaymentAPI.getOrderSummary).mockClear()

    await wrapper.get('[data-test="date-filter"]').trigger('click')
    await flushPromises()

    expect(adminPaymentAPI.getOrders).toHaveBeenCalledWith(
      expect.objectContaining({ page: 1, start_date: '2026-01-01', end_date: '2026-01-31', time_field: 'paid_at' }),
      { signal: expect.any(AbortSignal) },
    )
    expect(adminPaymentAPI.getOrderSummary).toHaveBeenCalledWith(
      expect.objectContaining({ group_page: 1, start_date: '2026-01-01', end_date: '2026-01-31', time_field: 'paid_at' }),
      { signal: expect.any(AbortSignal) },
    )
  })

  it('applies a promo attribution group to the order filters', async () => {
    const wrapper = mountView()
    await flushPromises()
    vi.mocked(adminPaymentAPI.getOrders).mockClear()
    vi.mocked(adminPaymentAPI.getOrderSummary).mockClear()

    await wrapper.get('[data-test="attribution-row"]').trigger('click')
    await flushPromises()

    expect(adminPaymentAPI.getOrders).toHaveBeenCalledWith(
      expect.objectContaining({ page: 1, promo_attribution: 'none' }),
      { signal: expect.any(AbortSignal) },
    )
    expect(adminPaymentAPI.getOrderSummary).toHaveBeenCalledWith(
      expect.objectContaining({ group_page: 1, promo_attribution: 'none' }),
      { signal: expect.any(AbortSignal) },
    )
  })

  it('refreshes summary after a successful order mutation', async () => {
    const wrapper = mountView()
    await flushPromises()
    vi.mocked(adminPaymentAPI.getOrders).mockClear()
    vi.mocked(adminPaymentAPI.getOrderSummary).mockClear()

    const cancelButton = wrapper.findAll('button').find((button) => button.text().includes('payment.orders.cancel'))
    expect(cancelButton).toBeTruthy()
    await cancelButton!.trigger('click')
    await flushPromises()

    expect(adminPaymentAPI.cancelOrder).toHaveBeenCalledWith(1)
    expect(adminPaymentAPI.getOrders).toHaveBeenCalledTimes(1)
    expect(adminPaymentAPI.getOrderSummary).toHaveBeenCalledTimes(1)
  })
})

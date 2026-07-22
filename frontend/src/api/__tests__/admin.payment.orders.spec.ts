import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { get },
}))

import { adminPaymentAPI } from '@/api/admin/payment'
import type { AdminOrderFilters, AdminOrderSummary, AdminOrderPromoCodeOption } from '@/types/payment'

describe('admin payment order reporting API', () => {
  beforeEach(() => get.mockReset())

  it('passes the complete shared filter contract to orders and summary requests', async () => {
    const filters: AdminOrderFilters = {
      user_id: 9,
      status: 'COMPLETED',
      order_type: 'balance',
      payment_type: 'stripe',
      keyword: 'alice@example.com',
      promo_code_id: 12,
      promo_attribution: 'attributed',
      start_date: '2026-01-01',
      end_date: '2026-01-31',
      timezone: 'America/Los_Angeles',
      time_field: 'paid_at',
    }
    const controller = new AbortController()
    get.mockResolvedValue({ data: {} })

    await adminPaymentAPI.getOrders({ ...filters, page: 2, page_size: 50 }, { signal: controller.signal })
    await adminPaymentAPI.getOrderSummary({ ...filters, group_page: 3, group_page_size: 25 }, { signal: controller.signal })

    expect(get).toHaveBeenNthCalledWith(1, '/admin/payment/orders', {
      params: { ...filters, page: 2, page_size: 50 },
      signal: controller.signal,
    })
    expect(get).toHaveBeenNthCalledWith(2, '/admin/payment/orders/summary', {
      params: { ...filters, group_page: 3, group_page_size: 25 },
      signal: controller.signal,
    })
  })

  it('loads promo options and exports both CSV modes as blobs', async () => {
    const options: AdminOrderPromoCodeOption[] = [
      { promo_attribution: 'none' },
      { promo_attribution: 'legacy_unknown' },
      { promo_attribution: 'attributed', promo_code_id: 7, promo_code: 'WELCOME' },
    ]
    const blob = new Blob(['csv'])
    get.mockResolvedValueOnce({ data: options }).mockResolvedValueOnce({ data: blob }).mockResolvedValueOnce({ data: blob })

    const optionResponse = await adminPaymentAPI.getOrderPromoCodeOptions({ search: 'wel', limit: 20 })
    const orderExport = await adminPaymentAPI.exportOrders('orders', { status: 'COMPLETED' })
    const attributionExport = await adminPaymentAPI.exportOrders('attribution', { status: 'COMPLETED' })

    expect(optionResponse.data).toEqual(options)
    expect(orderExport).toBe(blob)
    expect(attributionExport).toBe(blob)
    expect(get).toHaveBeenNthCalledWith(1, '/admin/payment/orders/promo-code-options', {
      params: { search: 'wel', limit: 20 },
      signal: undefined,
    })
    expect(get).toHaveBeenNthCalledWith(2, '/admin/payment/orders/export', {
      params: { status: 'COMPLETED', mode: 'orders' },
      responseType: 'blob',
    })
    expect(get).toHaveBeenNthCalledWith(3, '/admin/payment/orders/export', {
      params: { status: 'COMPLETED', mode: 'attribution' },
      responseType: 'blob',
    })
  })

  it('keeps summary money values as decimal strings', () => {
    const summary: AdminOrderSummary = {
      totals: {
        filtered_order_count: 2,
        paid_order_count: 2,
        paid_amounts: [{ currency: 'CNY', order_count: 2, amount: '20.40' }],
        successful_order_count: 1,
        recharged_user_count: 1,
        gross_recharge_amount: '10.20',
        refunded_amount: '1.10',
        net_recharge_amount: '9.10',
      },
      groups: [],
      group_page: 1,
      group_page_size: 20,
      group_total: 0,
    }

    expect(summary.totals.net_recharge_amount).toBe('9.10')
    expect(summary.totals.paid_amounts).toEqual([{ currency: 'CNY', order_count: 2, amount: '20.40' }])
  })
})

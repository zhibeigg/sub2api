/**
 * Admin Payment API endpoints
 * Handles payment management operations for administrators
 */

import { apiClient } from '../client'
import type {
  DashboardStats,
  PaymentOrder,
  AdminOrderFilters,
  AdminOrderSummary,
  AdminOrderPromoCodeOption,
  PaymentChannel,
  SubscriptionPlan,
  SubscriptionPlanType,
  ProviderInstance
} from '@/types/payment'
import type { BasePaginationResponse } from '@/types'

/** Admin-facing payment config returned by GET /admin/payment/config */
export interface AdminPaymentConfig {
  enabled: boolean
  min_amount: number
  max_amount: number
  daily_limit: number
  order_timeout_minutes: number
  max_pending_orders: number
  enabled_payment_types: string[]
  balance_disabled: boolean
  subscription_disabled: boolean
  balance_recharge_multiplier: number
  subscription_usd_to_cny_rate: number
  recharge_fee_rate: number
  load_balance_strategy: string
  product_name_prefix: string
  product_name_suffix: string
  help_image_url: string
  help_text: string
}

/** Fields accepted by PUT /admin/payment/config (all optional via pointer semantics) */
export interface UpdatePaymentConfigRequest {
  enabled?: boolean
  min_amount?: number
  max_amount?: number
  daily_limit?: number
  order_timeout_minutes?: number
  max_pending_orders?: number
  enabled_payment_types?: string[]
  balance_disabled?: boolean
  subscription_disabled?: boolean
  balance_recharge_multiplier?: number
  subscription_usd_to_cny_rate?: number
  recharge_fee_rate?: number
  load_balance_strategy?: string
  product_name_prefix?: string
  product_name_suffix?: string
  help_image_url?: string
  help_text?: string
}

export interface CreateSubscriptionPlanRequest {
  name: string
  plan_type: SubscriptionPlanType
  group_id: number | null
  group_ids: number[]
  daily_limit_usd: number | null
  weekly_limit_usd: number | null
  monthly_limit_usd: number | null
  concurrency_limit: number | null
  description: string
  price: number
  original_price?: number
  validity_days: number
  validity_unit: string
  features?: string | string[]
  for_sale?: boolean
  sort_order?: number
}

export interface UpdateSubscriptionPlanRequest extends Partial<CreateSubscriptionPlanRequest> {
  quota_limits_set?: boolean
  concurrency_limit_set?: boolean
}

export interface RefundResult {
  success: boolean
  warning?: string
  require_force?: boolean
  balance_deducted?: number
  subscription_days_deducted?: number
}

export const adminPaymentAPI = {
  // ==================== Config ====================

  /** Get payment configuration (admin view) */
  getConfig() {
    return apiClient.get<AdminPaymentConfig>('/admin/payment/config')
  },

  /** Update payment configuration */
  updateConfig(data: UpdatePaymentConfigRequest) {
    return apiClient.put('/admin/payment/config', data)
  },

  // ==================== Dashboard ====================

  /** Get payment dashboard statistics */
  getDashboard(days?: number) {
    return apiClient.get<DashboardStats>('/admin/payment/dashboard', {
      params: days ? { days } : undefined
    })
  },

  // ==================== Orders ====================

  /** Get all orders (paginated, with filters) */
  getOrders(
    params?: AdminOrderFilters & { page?: number; page_size?: number },
    options?: { signal?: AbortSignal },
  ) {
    return apiClient.get<BasePaginationResponse<PaymentOrder>>('/admin/payment/orders', {
      params,
      signal: options?.signal,
    })
  },

  /** Get filtered recharge totals and promo attribution groups. */
  getOrderSummary(
    params?: AdminOrderFilters & { group_page?: number; group_page_size?: number },
    options?: { signal?: AbortSignal },
  ) {
    return apiClient.get<AdminOrderSummary>('/admin/payment/orders/summary', {
      params,
      signal: options?.signal,
    })
  },

  /** Get current, historical, and unattributed promo code filter options. */
  getOrderPromoCodeOptions(params?: { search?: string; limit?: number }, options?: { signal?: AbortSignal }) {
    return apiClient.get<AdminOrderPromoCodeOption[]>('/admin/payment/orders/promo-code-options', {
      params,
      signal: options?.signal,
    })
  },

  /** Export filtered orders or attribution groups as CSV. */
  async exportOrders(mode: 'orders' | 'attribution', params?: AdminOrderFilters): Promise<Blob> {
    const response = await apiClient.get('/admin/payment/orders/export', {
      params: { ...params, mode },
      responseType: 'blob',
    })
    return response.data
  },

  /** Get a specific order by ID */
  getOrder(id: number) {
    return apiClient.get<PaymentOrder>(`/admin/payment/orders/${id}`)
  },

  /** Cancel an order (admin) */
  cancelOrder(id: number) {
    return apiClient.post(`/admin/payment/orders/${id}/cancel`)
  },

  /** Retry recharge for a failed order */
  retryRecharge(id: number) {
    return apiClient.post(`/admin/payment/orders/${id}/retry`)
  },

  /** Process a refund */
  refundOrder(id: number, data: { amount: number; reason: string; deduct_balance?: boolean; force?: boolean }) {
    return apiClient.post<RefundResult>(`/admin/payment/orders/${id}/refund`, data)
  },

  /** Query and finalize a pending refund */
  queryRefund(id: number) {
    return apiClient.post<RefundResult>(`/admin/payment/orders/${id}/refund/query`)
  },

  // ==================== Channels ====================

  /** Get all payment channels */
  getChannels() {
    return apiClient.get<PaymentChannel[]>('/admin/payment/channels')
  },

  /** Create a payment channel */
  createChannel(data: Partial<PaymentChannel>) {
    return apiClient.post<PaymentChannel>('/admin/payment/channels', data)
  },

  /** Update a payment channel */
  updateChannel(id: number, data: Partial<PaymentChannel>) {
    return apiClient.put<PaymentChannel>(`/admin/payment/channels/${id}`, data)
  },

  /** Delete a payment channel */
  deleteChannel(id: number) {
    return apiClient.delete(`/admin/payment/channels/${id}`)
  },

  // ==================== Subscription Plans ====================

  /** Get all subscription plans */
  getPlans() {
    return apiClient.get<SubscriptionPlan[]>('/admin/payment/plans')
  },

  /** Create a subscription plan */
  createPlan(data: CreateSubscriptionPlanRequest) {
    return apiClient.post<SubscriptionPlan>('/admin/payment/plans', data)
  },

  /** Update a subscription plan */
  updatePlan(id: number, data: UpdateSubscriptionPlanRequest) {
    return apiClient.put<SubscriptionPlan>(`/admin/payment/plans/${id}`, data)
  },

  /** Delete a subscription plan */
  deletePlan(id: number) {
    return apiClient.delete(`/admin/payment/plans/${id}`)
  },

  // ==================== Provider Instances ====================

  /** Get all provider instances */
  getProviders() {
    return apiClient.get<ProviderInstance[]>('/admin/payment/providers')
  },

  /** Create a provider instance */
  createProvider(data: Partial<ProviderInstance>) {
    return apiClient.post<ProviderInstance>('/admin/payment/providers', data)
  },

  /** Update a provider instance */
  updateProvider(id: number, data: Partial<ProviderInstance>) {
    return apiClient.put<ProviderInstance>(`/admin/payment/providers/${id}`, data)
  },

  /** Delete a provider instance */
  deleteProvider(id: number) {
    return apiClient.delete(`/admin/payment/providers/${id}`)
  }
}

export default adminPaymentAPI

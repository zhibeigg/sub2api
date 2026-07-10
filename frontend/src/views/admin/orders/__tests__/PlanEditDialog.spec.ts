import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import PlanEditDialog from '../PlanEditDialog.vue'
import { adminPaymentAPI } from '@/api/admin/payment'
import type { AdminGroup } from '@/types'
import type { SubscriptionPlan } from '@/types/payment'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => {
      if (key === 'payment.admin.subscriptionCnyPayPreview') return `preview ${params?.amount}`
      if (key === 'payment.admin.subscriptionCnyPayPreviewWithFee') return `fee ${params?.feeRate} ${params?.total}`
      return key
    },
  }),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('@/api/admin/payment', () => ({
  adminPaymentAPI: {
    createPlan: vi.fn(),
    updatePlan: vi.fn(),
  },
}))

function mountDialog(
  paymentConfig: Record<string, unknown> | null,
  options: { plan?: SubscriptionPlan | null; groups?: AdminGroup[] } = {},
) {
  return mount(PlanEditDialog, {
    props: {
      show: true,
      plan: options.plan ?? null,
      groups: options.groups ?? [],
      paymentConfig,
    },
    global: {
      stubs: {
        BaseDialog: {
          props: ['show'],
          template: '<div v-if="show"><slot /><slot name="footer" /></div>',
        },
        Select: true,
        Icon: true,
        GroupBadge: true,
      },
    },
  })
}

describe('PlanEditDialog subscription CNY payment preview', () => {
  it('shows CNY channel charge using the configured subscription rate and fee', async () => {
    const wrapper = mountDialog({
      subscription_usd_to_cny_rate: 7.15,
      recharge_fee_rate: 2.5,
    })

    await wrapper.find('[data-test="plan-price"]').setValue('9.99')

    expect(wrapper.text()).toContain('preview')
    expect(wrapper.text()).toContain('¥71.43')
    expect(wrapper.text()).toContain('fee 2.5')
    expect(wrapper.text()).toContain('¥73.22')
  })

  it('hides the preview when the subscription rate is not configured', async () => {
    const wrapper = mountDialog({
      subscription_usd_to_cny_rate: 0,
      recharge_fee_rate: 2.5,
    })

    await wrapper.find('[data-test="plan-price"]').setValue('9.99')

    expect(wrapper.text()).not.toContain('preview')
    expect(wrapper.text()).not.toContain('¥71.43')
  })
})

describe('PlanEditDialog shared quota payload', () => {
  it('updates group ids, keeps the first legacy group id, and explicitly snapshots empty quotas', async () => {
    const groups = [
      { id: 11, name: 'OpenAI A', platform: 'openai', rate_multiplier: 1, subscription_type: 'subscription', status: 'active' },
      { id: 12, name: 'Claude B', platform: 'anthropic', rate_multiplier: 1.2, subscription_type: 'subscription', status: 'active' },
    ] as unknown as AdminGroup[]
    const plan = {
      id: 7,
      group_id: 11,
      group_ids: [11, 12],
      groups: [
        { id: 11, name: 'OpenAI A', platform: 'openai', rate_multiplier: 1 },
        { id: 12, name: 'Claude B', platform: 'anthropic', rate_multiplier: 1.2 },
      ],
      name: 'Shared Plan',
      description: 'Shared quota',
      price: 20,
      original_price: 0,
      validity_days: 30,
      validity_unit: 'days',
      features: [],
      for_sale: true,
      sort_order: 0,
      daily_limit_usd: 5,
      weekly_limit_usd: null,
      monthly_limit_usd: 50,
    } satisfies SubscriptionPlan
    vi.mocked(adminPaymentAPI.updatePlan).mockReset().mockResolvedValue({} as never)

    const wrapper = mountDialog(null, { plan, groups })
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await wrapper.find('[data-test="daily-limit"]').setValue('')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adminPaymentAPI.updatePlan).toHaveBeenCalledWith(7, expect.objectContaining({
      group_id: 11,
      group_ids: [11, 12],
      daily_limit_usd: null,
      weekly_limit_usd: null,
      monthly_limit_usd: 50,
      quota_limits_set: true,
    }))
  })
})

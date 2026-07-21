import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import PlanEditDialog from '../PlanEditDialog.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'
import { adminPaymentAPI } from '@/api/admin/payment'
import type { AdminGroup } from '@/types'
import type { SubscriptionPlan } from '@/types/payment'

const showError = vi.fn()
const showSuccess = vi.fn()

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
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('@/api/admin/payment', () => ({
  adminPaymentAPI: {
    createPlan: vi.fn(),
    updatePlan: vi.fn(),
  },
}))

const groups = [
  { id: 1, name: 'Standard A', platform: 'openai', rate_multiplier: 1, subscription_type: 'standard', status: 'active' },
  { id: 2, name: 'Standard B', platform: 'anthropic', rate_multiplier: 1.2, subscription_type: 'standard', status: 'active' },
  { id: 11, name: 'Subscription A', platform: 'openai', rate_multiplier: 1, subscription_type: 'subscription', status: 'active' },
  { id: 12, name: 'Subscription B', platform: 'anthropic', rate_multiplier: 1.2, subscription_type: 'subscription', status: 'active' },
] as unknown as AdminGroup[]

function mountDialog(
  paymentConfig: Record<string, unknown> | null = null,
  options: { plan?: SubscriptionPlan | null; groups?: AdminGroup[] } = {},
) {
  return mount(PlanEditDialog, {
    props: {
      show: true,
      plan: options.plan ?? null,
      groups: options.groups ?? groups,
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

async function reopen(wrapper: VueWrapper) {
  await wrapper.setProps({ show: false })
  await wrapper.setProps({ show: true })
}

async function selectGroup(wrapper: VueWrapper, index = 0) {
  const selector = wrapper.findComponent(GroupSelector)
  const inputType = selector.props('multiple') ? 'checkbox' : 'radio'
  const inputs = selector.findAll(`input[type="${inputType}"]`)
  await inputs[index].setValue(true)
  return selector
}

beforeEach(() => {
  showError.mockReset()
  showSuccess.mockReset()
  vi.mocked(adminPaymentAPI.createPlan).mockReset().mockResolvedValue({} as never)
  vi.mocked(adminPaymentAPI.updatePlan).mockReset().mockResolvedValue({} as never)
})

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

describe('PlanEditDialog plan types', () => {
  it('filters subscription groups, uses single selection, and hides plan quotas', async () => {
    const wrapper = mountDialog()
    const selector = wrapper.findComponent(GroupSelector)

    expect(selector.props('multiple')).toBe(false)
    expect((selector.props('groups') as AdminGroup[]).map(group => group.id)).toEqual([11, 12])
    expect(selector.findAll('input[type="radio"]')).toHaveLength(2)
    expect(wrapper.find('[data-test="shared-quota-section"]').exists()).toBe(false)

    await selectGroup(wrapper, 0)
    await selectGroup(wrapper, 1)

    expect(wrapper.findComponent(GroupSelector).props('modelValue')).toEqual([12])
  })

  it('filters standard groups, keeps multiple selections, and shows shared quotas', async () => {
    const wrapper = mountDialog()
    await wrapper.find('[data-test="plan-type-standard_quota"]').setValue(true)

    const selector = wrapper.findComponent(GroupSelector)
    expect(selector.props('multiple')).toBe(true)
    expect((selector.props('groups') as AdminGroup[]).map(group => group.id)).toEqual([1, 2])
    expect(selector.findAll('input[type="checkbox"]')).toHaveLength(2)
    expect(wrapper.find('[data-test="shared-quota-section"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="concurrency-limit"]').exists()).toBe(true)

    await selectGroup(wrapper, 0)
    await selectGroup(wrapper, 1)

    expect(wrapper.findComponent(GroupSelector).props('modelValue')).toEqual([1, 2])
  })

  it('clears incompatible groups and quotas when switching types', async () => {
    const plan = makePlan({
      plan_type: 'standard_quota',
      group_id: 1,
      group_ids: [1, 2],
      daily_limit_usd: 5,
      monthly_limit_usd: 50,
      concurrency_limit: 4,
    })
    const wrapper = mountDialog(null, { plan })
    await reopen(wrapper)

    await wrapper.find('[data-test="plan-type-subscription"]').setValue(true)

    expect(wrapper.findComponent(GroupSelector).props('modelValue')).toEqual([])
    expect(wrapper.find('[data-test="shared-quota-section"]').exists()).toBe(false)

    await selectGroup(wrapper, 0)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adminPaymentAPI.updatePlan).toHaveBeenCalledWith(7, expect.objectContaining({
      plan_type: 'subscription',
      group_id: 11,
      group_ids: [11],
      daily_limit_usd: null,
      weekly_limit_usd: null,
      monthly_limit_usd: null,
      quota_limits_set: true,
      concurrency_limit: null,
      concurrency_limit_set: true,
    }))
  })

  it('refills an existing concurrency snapshot and explicitly clears it on update', async () => {
    const wrapper = mountDialog(null, {
      plan: makePlan({
        plan_type: 'standard_quota',
        group_id: 1,
        group_ids: [1],
        daily_limit_usd: 5,
        concurrency_limit: 8,
      }),
    })
    await reopen(wrapper)

    expect((wrapper.find('[data-test="concurrency-limit"]').element as HTMLInputElement).value).toBe('8')

    await wrapper.find('[data-test="concurrency-limit"]').setValue('')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adminPaymentAPI.updatePlan).toHaveBeenCalledWith(7, expect.objectContaining({
      concurrency_limit: null,
      concurrency_limit_set: true,
    }))
  })

  it('rejects zero, negative, and fractional concurrency limits', async () => {
    for (const invalidValue of ['0', '-1', '1.5', '2147483648']) {
      const wrapper = mountDialog()
      await wrapper.find('[data-test="plan-type-standard_quota"]').setValue(true)
      await selectGroup(wrapper)
      await wrapper.find('[data-test="daily-limit"]').setValue('5')
      await wrapper.find('[data-test="concurrency-limit"]').setValue(invalidValue)
      await wrapper.find('[data-test="plan-price"]').setValue('10')
      await wrapper.find('form').trigger('submit')

      expect(showError).toHaveBeenLastCalledWith('payment.admin.concurrencyLimitInvalid')
      wrapper.unmount()
    }
    expect(adminPaymentAPI.createPlan).not.toHaveBeenCalled()
  })

  it('requires a positive standard quota before saving', async () => {
    const wrapper = mountDialog()
    await wrapper.find('[data-test="plan-type-standard_quota"]').setValue(true)
    await selectGroup(wrapper)
    await wrapper.find('[data-test="plan-price"]').setValue('10')
    await wrapper.find('form').trigger('submit')

    expect(showError).toHaveBeenCalledWith('payment.admin.standardQuotaRequired')
    expect(adminPaymentAPI.createPlan).not.toHaveBeenCalled()
  })

  it('submits the standard quota type, multiple groups, and shared limits', async () => {
    const wrapper = mountDialog()
    await wrapper.find('[data-test="plan-type-standard_quota"]').setValue(true)
    await selectGroup(wrapper, 0)
    await selectGroup(wrapper, 1)
    await wrapper.find('[data-test="daily-limit"]').setValue('5')
    await wrapper.find('[data-test="concurrency-limit"]').setValue('3')
    await wrapper.find('[data-test="plan-price"]').setValue('10')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adminPaymentAPI.createPlan).toHaveBeenCalledWith(expect.objectContaining({
      plan_type: 'standard_quota',
      group_id: 1,
      group_ids: [1, 2],
      daily_limit_usd: 5,
      weekly_limit_usd: null,
      monthly_limit_usd: null,
      concurrency_limit: 3,
    }))
  })

  it('shows a legacy conversion warning and blocks saving until a formal type is selected', async () => {
    const wrapper = mountDialog(null, {
      plan: makePlan({ plan_type: 'legacy_shared_subscription', group_id: 11, group_ids: [11, 12] }),
    })
    await reopen(wrapper)

    expect(wrapper.find('[data-test="legacy-plan-warning"]').exists()).toBe(true)
    expect((wrapper.findComponent(GroupSelector).props('groups') as AdminGroup[])).toEqual([])

    await wrapper.find('form').trigger('submit')

    expect(showError).toHaveBeenCalledWith('payment.admin.formalPlanTypeRequired')
    expect(adminPaymentAPI.updatePlan).not.toHaveBeenCalled()
  })
})

function makePlan(overrides: Partial<SubscriptionPlan> = {}): SubscriptionPlan {
  return {
    id: 7,
    plan_type: 'subscription',
    group_id: 11,
    group_ids: [11],
    groups: [
      { id: 11, name: 'Subscription A', platform: 'openai', rate_multiplier: 1, subscription_type: 'subscription' },
    ],
    name: 'Plan',
    description: 'Description',
    price: 20,
    original_price: 0,
    validity_days: 30,
    validity_unit: 'days',
    features: [],
    for_sale: true,
    sort_order: 0,
    daily_limit_usd: null,
    weekly_limit_usd: null,
    monthly_limit_usd: null,
    concurrency_limit: null,
    ...overrides,
  }
}

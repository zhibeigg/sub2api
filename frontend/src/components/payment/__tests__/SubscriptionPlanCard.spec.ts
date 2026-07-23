import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import SubscriptionPlanCard from '../SubscriptionPlanCard.vue'
import type { UserSubscription } from '@/types'
import type { SubscriptionPlan } from '@/types/payment'

const messages = vi.hoisted<Record<string, string>>(() => ({
  'payment.days': 'days',
  'payment.weeks': 'weeks',
  'payment.months': 'months',
  'payment.perMonth': 'month',
  'payment.groupFallback': 'Group',
  'payment.planCard.dailyLimit': 'Daily',
  'payment.planCard.weeklyLimit': 'Weekly',
  'payment.planCard.monthlyLimit': 'Monthly',
  'payment.planCard.concurrency': 'Concurrency limit',
  'payment.planCard.concurrencyValue': 'Concurrency {limit}',
  'payment.planCard.noExtraConcurrencyLimit': 'No additional limit',
  'payment.planCard.quota': 'Quota',
  'payment.planCard.sharedQuota': 'Shared plan quota',
  'payment.planCard.nativeQuota': 'Native group quota',
  'payment.planCard.peakRate': 'Peak Rate',
  'payment.planCard.unlimited': 'Unlimited',
  'payment.planCard.models': 'Models',
  'payment.subscribeNow': 'Subscribe now',
  'payment.renewNow': 'Renew now',
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => {
      const message = messages[key] || key
      return Object.entries(params || {}).reduce(
        (result, [name, value]) => result.replace(`{${name}}`, String(value)),
        message,
      )
    },
  }),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ cachedPublicSettings: null }),
}))

function makePlan(overrides: Partial<SubscriptionPlan> = {}): SubscriptionPlan {
  return {
    id: 1,
    plan_type: 'subscription',
    group_id: 10,
    group_ids: [10],
    groups: [{
      id: 10,
      name: 'Primary Group',
      platform: 'openai',
      rate_multiplier: 1,
      subscription_type: 'subscription',
      daily_limit_usd: 3,
      weekly_limit_usd: 15,
      monthly_limit_usd: 40,
    }],
    group_platform: 'openai',
    group_name: 'Primary Group',
    name: 'Pro',
    description: 'Plan description',
    price: 10,
    original_price: 0,
    features: [],
    rate_multiplier: 1,
    validity_days: 30,
    validity_unit: 'day',
    supported_model_scopes: ['claude', 'gemini_text', 'gemini_image'],
    daily_limit_usd: 99,
    weekly_limit_usd: 199,
    monthly_limit_usd: 299,
    concurrency_limit: null,
    for_sale: true,
    sort_order: 0,
    ...overrides,
  }
}

function makeSubscription(overrides: Partial<UserSubscription> = {}): UserSubscription {
  return {
    id: 100,
    user_id: 1,
    group_id: 10,
    group_ids: [10],
    groups: [],
    source_plan_id: null,
    quota_snapshotted: true,
    status: 'active',
    starts_at: '2026-01-01T00:00:00Z',
    daily_usage_usd: 0,
    weekly_usage_usd: 0,
    monthly_usage_usd: 0,
    daily_window_start: null,
    weekly_window_start: null,
    monthly_window_start: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    expires_at: '2026-02-01T00:00:00Z',
    ...overrides,
  }
}

function mountPlanCard(plan: SubscriptionPlan, activeSubscriptions: UserSubscription[] = []) {
  return mount(SubscriptionPlanCard, {
    props: { plan, activeSubscriptions },
  })
}

describe('SubscriptionPlanCard quota display', () => {
  it('shows plan-level shared quotas for standard quota plans', () => {
    const plan = makePlan({
      plan_type: 'standard_quota',
      group_ids: [10, 20],
      groups: [
        { id: 10, name: 'OpenAI', platform: 'openai', rate_multiplier: 1, subscription_type: 'standard', daily_limit_usd: 3 },
        { id: 20, name: 'Claude', platform: 'anthropic', rate_multiplier: 1, subscription_type: 'standard', daily_limit_usd: 4 },
      ],
      daily_limit_usd: 10,
      weekly_limit_usd: 50,
      monthly_limit_usd: 120,
    })

    const text = mountPlanCard(plan).text()

    expect(text).toContain('Shared plan quota')
    expect(text).toContain('$10')
    expect(text).toContain('$50')
    expect(text).toContain('$120')
    expect(text).toContain('OpenAI')
    expect(text).toContain('Claude')
  })

  it('shows a positive standard-plan concurrency limit', () => {
    const wrapper = mountPlanCard(makePlan({
      plan_type: 'standard_quota',
      concurrency_limit: 5,
    }))

    expect(wrapper.find('[data-test="plan-concurrency"]').text()).toContain('Concurrency 5')
  })

  it('shows no additional concurrency limit when the standard plan value is null', () => {
    const wrapper = mountPlanCard(makePlan({
      plan_type: 'standard_quota',
      concurrency_limit: null,
    }))

    expect(wrapper.find('[data-test="plan-concurrency"]').text()).toContain('No additional limit')
  })

  it('does not show plan concurrency for native subscription plans', () => {
    expect(mountPlanCard(makePlan({ plan_type: 'subscription' })).find('[data-test="plan-concurrency"]').exists()).toBe(false)
  })

  it('shows the single subscription group native quotas instead of plan-level quotas', () => {
    const plan = makePlan({
      plan_type: 'subscription',
      group_ids: [10, 20],
      groups: [
        {
          id: 10,
          name: 'Native Subscription',
          platform: 'openai',
          rate_multiplier: 1,
          subscription_type: 'subscription',
          daily_limit_usd: 3,
          weekly_limit_usd: 15,
          monthly_limit_usd: 40,
        },
        {
          id: 20,
          name: 'Ignored Group',
          platform: 'anthropic',
          rate_multiplier: 1,
          subscription_type: 'subscription',
          daily_limit_usd: 999,
        },
      ],
      daily_limit_usd: 99,
      weekly_limit_usd: 199,
      monthly_limit_usd: 299,
    })

    const text = mountPlanCard(plan).text()

    expect(text).toContain('Native group quota')
    expect(text).toContain('$3')
    expect(text).toContain('$15')
    expect(text).toContain('$40')
    expect(text).not.toContain('$99')
    expect(text).not.toContain('$199')
    expect(text).not.toContain('$299')
    expect(text).not.toContain('Ignored Group')
  })

  it('only shows Antigravity model scopes for Antigravity plans', () => {
    expect(mountPlanCard(makePlan({ group_platform: 'openai' })).text()).not.toContain('Claude')

    const antigravityPlan = makePlan({
      group_platform: 'antigravity',
      groups: [{
        id: 10,
        name: 'Antigravity',
        platform: 'antigravity',
        rate_multiplier: 1,
        subscription_type: 'subscription',
        supported_model_scopes: ['claude', 'gemini_text', 'gemini_image'],
      }],
    })
    const text = mountPlanCard(antigravityPlan).text()

    expect(text).toContain('Claude')
    expect(text).toContain('Gemini')
    expect(text).toContain('Imagen')
  })
})

describe('SubscriptionPlanCard renewal detection', () => {
  it('prefers an exact source plan id match', () => {
    const plan = makePlan({ id: 7, group_ids: [10] })

    expect(mountPlanCard(plan, [
      makeSubscription({ source_plan_id: 7, group_id: 999, group_ids: [999] }),
    ]).text()).toContain('Renew now')

    expect(mountPlanCard(plan, [
      makeSubscription({ source_plan_id: 8, group_id: 10, group_ids: [10] }),
    ]).text()).toContain('Subscribe now')
  })

  it('uses exact group-set compatibility only for subscriptions without a source plan id', () => {
    const plan = makePlan({
      plan_type: 'standard_quota',
      group_id: 10,
      group_ids: [10, 20],
      groups: [
        { id: 10, name: 'A', platform: 'openai', rate_multiplier: 1, subscription_type: 'standard' },
        { id: 20, name: 'B', platform: 'anthropic', rate_multiplier: 1, subscription_type: 'standard' },
      ],
    })

    expect(mountPlanCard(plan, [
      makeSubscription({ source_plan_id: null, group_id: 20, group_ids: [20, 10] }),
    ]).text()).toContain('Renew now')

    expect(mountPlanCard(plan, [
      makeSubscription({ source_plan_id: null, group_id: 10, group_ids: [10] }),
    ]).text()).toContain('Subscribe now')
  })
})

describe('SubscriptionPlanCard validity display', () => {
  it('renders plural admin-form validity units instead of mislabeled days', () => {
    expect(mountPlanCard(makePlan({ validity_days: 1, validity_unit: 'months' })).text()).toContain('/ month')
    expect(mountPlanCard(makePlan({ validity_days: 3, validity_unit: 'months' })).text()).toContain('/ 3months')
    expect(mountPlanCard(makePlan({ validity_days: 2, validity_unit: 'weeks' })).text()).toContain('/ 2weeks')
    expect(mountPlanCard(makePlan({ validity_days: 30, validity_unit: 'day' })).text()).toContain('/ 30days')
  })
})

describe('SubscriptionPlanCard currency display', () => {
  it('uses the configured currency symbol while preserving USD for legacy plans', () => {
    const cnyPlan = mountPlanCard(makePlan({ currency: 'CNY', original_price: 20 })).text()

    expect(cnyPlan).toContain('¥10CNY')
    expect(cnyPlan).toContain('¥20CNY')
    expect(mountPlanCard(makePlan({ currency: 'USD' })).text()).toContain('$10USD')
    expect(mountPlanCard(makePlan({ currency: '' })).text()).toContain('$10')
  })
})

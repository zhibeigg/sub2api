import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import SubscriptionsView from '../SubscriptionsView.vue'
import type { UserSubscription } from '@/types'

const { getMySubscriptions } = vi.hoisted(() => ({
  getMySubscriptions: vi.fn(),
}))

vi.mock('@/api/subscriptions', () => ({
  default: {
    getMySubscriptions,
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    cachedPublicSettings: null,
  }),
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (key === 'userSubscriptions.concurrencyLimit') return 'Concurrency limit'
        if (key === 'userSubscriptions.concurrencyValue') return `Concurrency ${params?.limit}`
        if (key === 'userSubscriptions.noExtraConcurrencyLimit') return 'No additional limit'
        return key
      },
    }),
  }
})

function makeSubscription(id: number, concurrencyLimit: number | null, quotaSnapshotted = true): UserSubscription {
  return {
    id,
    user_id: 1,
    group_id: 10,
    group_ids: [10],
    groups: [{
      id: 10,
      name: 'Standard Group',
      platform: 'openai',
      rate_multiplier: 1,
      subscription_type: 'standard',
    }],
    source_plan_id: 7,
    quota_snapshotted: quotaSnapshotted,
    daily_limit_usd: 10,
    weekly_limit_usd: null,
    monthly_limit_usd: null,
    concurrency_limit: concurrencyLimit,
    status: 'active',
    starts_at: '2026-07-01T00:00:00Z',
    daily_usage_usd: 0,
    weekly_usage_usd: 0,
    monthly_usage_usd: 0,
    daily_window_start: null,
    weekly_window_start: null,
    monthly_window_start: null,
    created_at: '2026-07-01T00:00:00Z',
    updated_at: '2026-07-01T00:00:00Z',
    expires_at: '2026-08-01T00:00:00Z',
  } as UserSubscription
}

describe('User SubscriptionsView concurrency snapshot', () => {
  beforeEach(() => {
    getMySubscriptions.mockReset().mockResolvedValue([
      makeSubscription(1, 4),
      makeSubscription(2, null),
      makeSubscription(3, null, false),
    ])
  })

  it('renders the concurrency value from each subscription instance snapshot', async () => {
    const wrapper = mount(SubscriptionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
        },
      },
    })

    await flushPromises()

    const snapshots = wrapper.findAll('[data-test="subscription-concurrency"]')
    expect(snapshots).toHaveLength(3)
    expect(snapshots[0].text()).toContain('Concurrency 4')
    expect(snapshots[1].text()).toContain('No additional limit')
    expect(snapshots[2].text()).toContain('-')
    expect(snapshots[2].text()).not.toContain('No additional limit')
  })
})

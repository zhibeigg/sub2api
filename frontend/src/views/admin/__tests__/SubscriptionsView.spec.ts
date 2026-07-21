import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import SubscriptionsView from '../SubscriptionsView.vue'
import type { UserSubscription } from '@/types'

const { listSubscriptions, getGroups, getPlans } = vi.hoisted(() => ({
  listSubscriptions: vi.fn(),
  getGroups: vi.fn(),
  getPlans: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    subscriptions: {
      list: listSubscriptions,
    },
    groups: {
      getAll: getGroups,
    },
    usage: {
      searchUsers: vi.fn(),
    },
  },
}))

vi.mock('@/api/admin/payment', () => ({
  adminPaymentAPI: {
    getPlans,
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (key === 'admin.subscriptions.concurrencyLimit') return 'Concurrency limit'
        if (key === 'admin.subscriptions.concurrencyValue') return `Concurrency ${params?.limit}`
        if (key === 'admin.subscriptions.noExtraConcurrencyLimit') return 'No additional limit'
        return key
      },
    }),
  }
})

const TablePageLayoutStub = {
  template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>',
}

const DataTableStub = {
  props: ['data'],
  template: `
    <div>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-usage" :row="row" />
      </div>
    </div>
  `,
}

function makeSubscription(concurrencyLimit: number | null, quotaSnapshotted = true, id = 100): UserSubscription {
  return {
    id,
    user_id: 1,
    group_id: 10,
    group_ids: [10],
    groups: [],
    source_plan_id: 7,
    quota_snapshotted: quotaSnapshotted,
    daily_limit_usd: null,
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
    user: { id: 1, email: 'user@example.com', username: 'user' } as UserSubscription['user'],
  }
}

describe('Admin SubscriptionsView concurrency snapshot', () => {
  beforeEach(() => {
    listSubscriptions.mockReset().mockResolvedValue({
      items: [makeSubscription(3), makeSubscription(null, false, 101)],
      page: 1,
      page_size: 20,
      total: 1,
      pages: 1,
    })
    getGroups.mockReset().mockResolvedValue([])
    getPlans.mockReset().mockResolvedValue({ data: [] })
  })

  it('renders the subscription instance concurrency snapshot in the usage cell', async () => {
    const wrapper = mount(SubscriptionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: TablePageLayoutStub,
          DataTable: DataTableStub,
          Pagination: true,
          BaseDialog: true,
          ConfirmDialog: true,
          EmptyState: true,
          Select: true,
          GroupBadge: true,
          Icon: true,
          RouterLink: true,
        },
      },
    })

    await flushPromises()

    const snapshots = wrapper.findAll('[data-test="subscription-concurrency"]')
    expect(snapshots).toHaveLength(2)
    expect(snapshots[0].text()).toContain('Concurrency 3')
    expect(snapshots[1].text()).toContain('-')
    expect(snapshots[1].text()).not.toContain('No additional limit')
  })
})

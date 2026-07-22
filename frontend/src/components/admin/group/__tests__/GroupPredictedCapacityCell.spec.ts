import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import type { GroupPredictedCapacitySummary } from '@/types'
import GroupPredictedCapacityCell from '../GroupPredictedCapacityCell.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => ({
        'admin.groups.predictedCapacity.balance': 'Balance',
        'admin.groups.predictedCapacity.requests': 'Requests',
        'admin.groups.predictedCapacity.requestUnit': 'requests',
        'admin.groups.predictedCapacity.unlimited': 'Unlimited',
        'admin.groups.predictedCapacity.insufficient': 'Insufficient data',
      })[key] ?? key,
    }),
  }
})

const summary = (
  overrides: Partial<GroupPredictedCapacitySummary> = {},
): GroupPredictedCapacitySummary => ({
  group_id: 1,
  available: true,
  balance_complete: true,
  balance_unlimited: false,
  remaining_balance_usd: 123.456,
  known_remaining_balance_usd: 123.456,
  requests_complete: true,
  requests_unlimited: false,
  estimated_remaining_requests: 12346,
  known_request_account_count: 2,
  unknown_request_account_count: 0,
  unknown_account_count: 0,
  stale_account_count: 0,
  incompatible_unit_account_count: 0,
  evaluated_at: '2026-07-22T10:00:00Z',
  ...overrides,
})

describe('GroupPredictedCapacityCell', () => {
  it('shows complete balance and request estimates', () => {
    const wrapper = mount(GroupPredictedCapacityCell, {
      props: { summary: summary() },
    })

    expect(wrapper.text()).toContain('≈ $123.46')
    expect(wrapper.text()).toContain('≈ 12,346 requests')
  })

  it('shows known lower bounds and partial-estimate hints', () => {
    const wrapper = mount(GroupPredictedCapacityCell, {
      props: {
        summary: summary({
          balance_complete: false,
          remaining_balance_usd: null,
          known_remaining_balance_usd: 80,
          requests_complete: false,
          estimated_remaining_requests: 900,
          unknown_request_account_count: 1,
          unknown_account_count: 1,
          stale_account_count: 1,
        }),
      },
    })

    expect(wrapper.text()).toContain('≥ $80')
    expect(wrapper.text()).toContain('≥ 900 requests')
    expect(wrapper.get('[data-testid="predicted-capacity-cell"]').attributes('title'))
      .toContain('admin.groups.predictedCapacity.partialBalanceHint')
    expect(wrapper.get('[data-testid="predicted-capacity-cell"]').attributes('title'))
      .toContain('admin.groups.predictedCapacity.partialRequestsHint')
  })

  it('formats int64 request counts from decimal strings without losing precision', () => {
    const wrapper = mount(GroupPredictedCapacityCell, {
      props: {
        summary: summary({
          estimated_remaining_requests: '9223372036854775807',
        }),
      },
    })

    expect(wrapper.text()).toContain('≈ 9,223,372,036,854,775,807 requests')
  })

  it('shows unlimited for unlimited balance and requests', () => {
    const wrapper = mount(GroupPredictedCapacityCell, {
      props: {
        summary: summary({
          balance_unlimited: true,
          remaining_balance_usd: null,
          known_remaining_balance_usd: null,
          requests_unlimited: true,
          estimated_remaining_requests: null,
        }),
      },
    })

    expect(wrapper.text().match(/Unlimited/g)).toHaveLength(2)
  })

  it('shows insufficient data when the summary is unavailable', () => {
    const wrapper = mount(GroupPredictedCapacityCell, {
      props: { summary: summary({ available: false }) },
    })

    expect(wrapper.text().match(/Insufficient data/g)).toHaveLength(2)
  })

  it('keeps a stable two-line placeholder while loading', () => {
    const wrapper = mount(GroupPredictedCapacityCell, {
      props: { loading: true },
    })

    const placeholder = wrapper.get('[data-testid="predicted-capacity-loading"]')
    expect(placeholder.attributes('aria-busy')).toBe('true')
    expect(placeholder.text()).toContain('Balance')
    expect(placeholder.text()).toContain('Requests')
    expect(placeholder.findAll('.h-3')).toHaveLength(2)
  })
})

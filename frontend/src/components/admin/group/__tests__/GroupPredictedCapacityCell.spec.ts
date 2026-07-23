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
        'admin.groups.predictedCapacity.capacity': 'Capacity',
        'admin.groups.predictedCapacity.requests': 'Requests',
        'admin.groups.predictedCapacity.images': 'Images',
        'admin.groups.predictedCapacity.requestUnit': 'requests',
        'admin.groups.predictedCapacity.imageUnit': 'images',
        'admin.groups.predictedCapacity.unlimited': 'Unlimited',
        'admin.groups.predictedCapacity.insufficient': 'Insufficient data',
        'admin.groups.predictedCapacity.error': 'Load failed',
        'admin.groups.predictedCapacity.errorHint': 'Capacity prediction failed to load.',
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
  prediction_mode: 'historical_requests',
  prediction_unit: 'request',
  prediction_configured: true,
  prediction_complete: true,
  prediction_unlimited: false,
  predicted_quantity: '12346',
  prediction_unit_cost_usd: null,
  known_prediction_account_count: 2,
  unknown_prediction_account_count: 0,
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
  it('shows complete balance and generic request capacity', () => {
    const wrapper = mount(GroupPredictedCapacityCell, {
      props: { summary: summary() },
    })

    expect(wrapper.text()).toContain('≈ $123.46')
    expect(wrapper.text()).toContain('Requests')
    expect(wrapper.text()).toContain('≈ 12,346 requests')
  })

  it('prefers generic image fields and formats int64 strings without Number precision loss', () => {
    const wrapper = mount(GroupPredictedCapacityCell, {
      props: {
        summary: summary({
          prediction_mode: 'fixed_image_cost',
          prediction_unit: 'image',
          prediction_complete: true,
          predicted_quantity: '9223372036854775807',
          prediction_unit_cost_usd: 0.25,
          requests_complete: false,
          estimated_remaining_requests: 12,
        }),
      },
    })

    expect(wrapper.text()).toContain('Images')
    expect(wrapper.text()).toContain('≈ 9,223,372,036,854,775,807 images')
    expect(wrapper.text()).not.toContain('12 requests')
  })

  it('falls back to legacy request fields when generic fields are missing', () => {
    const legacy = summary({ estimated_remaining_requests: '9007199254740993' })
    delete legacy.prediction_mode
    delete legacy.prediction_unit
    delete legacy.prediction_configured
    delete legacy.prediction_complete
    delete legacy.prediction_unlimited
    delete legacy.predicted_quantity
    delete legacy.prediction_unit_cost_usd
    delete legacy.known_prediction_account_count
    delete legacy.unknown_prediction_account_count
    const wrapper = mount(GroupPredictedCapacityCell, {
      props: { summary: legacy },
    })

    expect(wrapper.text()).toContain('Requests')
    expect(wrapper.text()).toContain('≈ 9,007,199,254,740,993 requests')
  })

  it('shows known lower bounds and a dynamic partial image hint', () => {
    const wrapper = mount(GroupPredictedCapacityCell, {
      props: {
        summary: summary({
          balance_complete: false,
          remaining_balance_usd: null,
          known_remaining_balance_usd: 80,
          prediction_unit: 'image',
          prediction_complete: false,
          predicted_quantity: '900',
          unknown_prediction_account_count: 1,
          unknown_account_count: 1,
          stale_account_count: 1,
        }),
      },
    })

    expect(wrapper.text()).toContain('≥ $80')
    expect(wrapper.text()).toContain('≥ 900 images')
    expect(wrapper.get('[data-testid="predicted-capacity-cell"]').attributes('title'))
      .toContain('admin.groups.predictedCapacity.partialBalanceHint')
    expect(wrapper.get('[data-testid="predicted-capacity-cell"]').attributes('title'))
      .toContain('admin.groups.predictedCapacity.partialImagesHint')
  })

  it('shows unlimited from generic prediction fields', () => {
    const wrapper = mount(GroupPredictedCapacityCell, {
      props: {
        summary: summary({
          balance_unlimited: true,
          remaining_balance_usd: null,
          known_remaining_balance_usd: null,
          prediction_unlimited: true,
          predicted_quantity: null,
          requests_unlimited: false,
          estimated_remaining_requests: null,
        }),
      },
    })

    expect(wrapper.text().match(/Unlimited/g)).toHaveLength(2)
  })

  it('shows insufficient capacity when the selected prediction algorithm is not configured', () => {
    const wrapper = mount(GroupPredictedCapacityCell, {
      props: {
        summary: summary({
          prediction_configured: false,
          prediction_complete: false,
          predicted_quantity: null,
        }),
      },
    })

    expect(wrapper.text()).toContain('≈ $123.46')
    expect(wrapper.text()).toContain('Insufficient data')
  })

  it('shows insufficient data when the summary is unavailable', () => {
    const wrapper = mount(GroupPredictedCapacityCell, {
      props: { summary: summary({ available: false }) },
    })

    expect(wrapper.text().match(/Insufficient data/g)).toHaveLength(2)
  })

  it('shows a stable error state after a failed summary request', () => {
    const wrapper = mount(GroupPredictedCapacityCell, {
      props: { error: true },
    })

    expect(wrapper.text().match(/Load failed/g)).toHaveLength(2)
    expect(wrapper.get('[data-testid="predicted-capacity-cell"]').attributes('title'))
      .toBe('Capacity prediction failed to load.')
  })

  it('keeps a stable two-line placeholder while loading', () => {
    const wrapper = mount(GroupPredictedCapacityCell, {
      props: { loading: true },
    })

    const placeholder = wrapper.get('[data-testid="predicted-capacity-loading"]')
    expect(placeholder.attributes('aria-busy')).toBe('true')
    expect(placeholder.text()).toContain('Balance')
    expect(placeholder.text()).toContain('Capacity')
    expect(placeholder.findAll('.h-3')).toHaveLength(2)
  })
})

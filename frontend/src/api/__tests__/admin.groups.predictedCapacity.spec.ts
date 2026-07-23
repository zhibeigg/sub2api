import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({
  get: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get },
}))

import groupsAPI, {
  getPredictedCapacitySummary,
  list,
  normalizeGroupPredictedCapacitySummary,
} from '@/api/admin/groups'
import type { GroupPredictedCapacitySummary } from '@/types'

const legacySummary = (
  overrides: Partial<GroupPredictedCapacitySummary> = {},
): GroupPredictedCapacitySummary => ({
  group_id: 1,
  available: true,
  balance_complete: true,
  balance_unlimited: false,
  remaining_balance_usd: 25,
  known_remaining_balance_usd: 25,
  requests_complete: true,
  requests_unlimited: false,
  estimated_remaining_requests: '1000',
  known_request_account_count: 2,
  unknown_request_account_count: 0,
  unknown_account_count: 0,
  stale_account_count: 0,
  incompatible_unit_account_count: 0,
  evaluated_at: null,
  ...overrides,
})

describe('admin group predicted capacity API', () => {
  beforeEach(() => {
    get.mockReset()
    get.mockResolvedValue({ data: [legacySummary()] })
  })

  it('sends deduplicated positive integer group ids and normalizes legacy request fields', async () => {
    const controller = new AbortController()

    const result = await getPredictedCapacitySummary(
      [1, 2, 2, 0, -1, 1.5, Number.NaN, 3],
      { signal: controller.signal },
    )

    expect(get).toHaveBeenCalledWith('/admin/groups/predicted-capacity-summary', {
      params: { ids: '1,2,3' },
      signal: controller.signal,
    })
    expect(result).toEqual([
      expect.objectContaining({
        group_id: 1,
        prediction_mode: 'historical_requests',
        prediction_unit: 'request',
        prediction_configured: true,
        prediction_complete: true,
        prediction_unlimited: false,
        predicted_quantity: '1000',
        prediction_unit_cost_usd: null,
        known_prediction_account_count: 2,
        unknown_prediction_account_count: 0,
      }),
    ])
    expect(groupsAPI.getPredictedCapacitySummary).toBe(getPredictedCapacitySummary)
  })

  it('preserves generic fixed-image fields instead of falling back to legacy requests', () => {
    const result = normalizeGroupPredictedCapacitySummary(legacySummary({
      prediction_mode: 'fixed_image_cost',
      prediction_unit: 'image',
      prediction_configured: true,
      prediction_complete: false,
      prediction_unlimited: false,
      predicted_quantity: '9223372036854775807',
      prediction_unit_cost_usd: 0.125,
      known_prediction_account_count: 7,
      unknown_prediction_account_count: 3,
      requests_complete: true,
      estimated_remaining_requests: 12,
    }))

    expect(result).toEqual(expect.objectContaining({
      prediction_mode: 'fixed_image_cost',
      prediction_unit: 'image',
      prediction_complete: false,
      predicted_quantity: '9223372036854775807',
      prediction_unit_cost_usd: 0.125,
      known_prediction_account_count: 7,
      unknown_prediction_account_count: 3,
    }))
  })

  it('chunks more than 100 ids and merges normalized batch responses', async () => {
    const ids = Array.from({ length: 101 }, (_, index) => index + 1)
    get
      .mockResolvedValueOnce({ data: [legacySummary({ group_id: 1 })] })
      .mockResolvedValueOnce({ data: [legacySummary({ group_id: 101 })] })

    const result = await getPredictedCapacitySummary(ids)

    expect(result.map((item) => item.group_id)).toEqual([1, 101])
    expect(result.every((item) => item.prediction_unit === 'request')).toBe(true)
    expect(get).toHaveBeenCalledTimes(2)
    expect(get.mock.calls[0]?.[1]).toEqual({
      params: { ids: ids.slice(0, 100).join(',') },
      signal: undefined,
    })
    expect(get.mock.calls[1]?.[1]).toEqual({
      params: { ids: '101' },
      signal: undefined,
    })
  })

  it('normalizes missing admin group prediction config to legacy defaults', async () => {
    get.mockResolvedValueOnce({
      data: {
        items: [{ id: 9, name: 'Legacy group' }],
        total: 1,
        page: 1,
        page_size: 20,
        pages: 1,
      },
    })

    const result = await list()

    expect(result.items[0]).toEqual(expect.objectContaining({
      predicted_capacity_mode: 'historical_requests',
      predicted_image_unit_cost_usd: null,
    }))
  })

  it('returns an empty array without making a request when no valid ids remain', async () => {
    await expect(getPredictedCapacitySummary([0, -2, 1.2, Number.NaN])).resolves.toEqual([])
    expect(get).not.toHaveBeenCalled()
  })
})

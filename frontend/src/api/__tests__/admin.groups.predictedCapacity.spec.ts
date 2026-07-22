import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({
  get: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get },
}))

import groupsAPI, { getPredictedCapacitySummary } from '@/api/admin/groups'

describe('admin group predicted capacity API', () => {
  beforeEach(() => {
    get.mockReset()
    get.mockResolvedValue({ data: [{ group_id: 1, available: true }] })
  })

  it('sends deduplicated positive integer group ids and the AbortSignal', async () => {
    const controller = new AbortController()

    const result = await getPredictedCapacitySummary(
      [1, 2, 2, 0, -1, 1.5, Number.NaN, 3],
      { signal: controller.signal },
    )

    expect(get).toHaveBeenCalledWith('/admin/groups/predicted-capacity-summary', {
      params: { ids: '1,2,3' },
      signal: controller.signal,
    })
    expect(result).toEqual([{ group_id: 1, available: true }])
    expect(groupsAPI.getPredictedCapacitySummary).toBe(getPredictedCapacitySummary)
  })

  it('chunks more than 100 ids and merges batch responses', async () => {
    const ids = Array.from({ length: 101 }, (_, index) => index + 1)
    get
      .mockResolvedValueOnce({ data: [{ group_id: 1, available: true }] })
      .mockResolvedValueOnce({ data: [{ group_id: 101, available: true }] })

    await expect(getPredictedCapacitySummary(ids)).resolves.toEqual([
      { group_id: 1, available: true },
      { group_id: 101, available: true },
    ])
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

  it('returns an empty array without making a request when no valid ids remain', async () => {
    await expect(getPredictedCapacitySummary([0, -2, 1.2, Number.NaN])).resolves.toEqual([])
    expect(get).not.toHaveBeenCalled()
  })
})

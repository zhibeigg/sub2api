import { beforeEach, describe, expect, it, vi } from 'vitest'

const get = vi.hoisted(() => vi.fn())

vi.mock('@/api/client', () => ({
  apiClient: { get },
}))

import { getAvailable, getModelSquare } from '@/api/channels'

describe('user channels API', () => {
  beforeEach(() => {
    get.mockReset()
    get.mockResolvedValue({ data: [] })
  })

  it('keeps available channels on the existing endpoint', async () => {
    const controller = new AbortController()

    await expect(getAvailable({ signal: controller.signal })).resolves.toEqual([])
    expect(get).toHaveBeenCalledWith('/channels/available', {
      signal: controller.signal,
    })
  })

  it('loads model square data from the dedicated endpoint', async () => {
    const controller = new AbortController()

    await expect(getModelSquare({ signal: controller.signal })).resolves.toEqual([])
    expect(get).toHaveBeenCalledWith('/models/available', {
      signal: controller.signal,
    })
  })
})

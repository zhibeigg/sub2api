import { beforeEach, describe, expect, it, vi } from 'vitest'

const { put } = vi.hoisted(() => ({ put: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { put }
}))

import { updateApiKeyGroupBindings } from '@/api/admin/apiKeys'

describe('admin API key group bindings API', () => {
  beforeEach(() => {
    put.mockReset()
  })

  it('sends the complete reindexed group_bindings payload without group_id', async () => {
    put.mockResolvedValue({ data: { api_key: {}, auto_granted_group_access: false } })

    await updateApiKeyGroupBindings(12, [
      { group_id: 9, priority: 8 },
      { group_id: 4, priority: 3 }
    ])

    expect(put).toHaveBeenCalledWith('/admin/api-keys/12', {
      group_bindings: [
        { group_id: 9, priority: 0 },
        { group_id: 4, priority: 1 }
      ]
    })
    expect(put.mock.calls[0][1]).not.toHaveProperty('group_id')
  })
})

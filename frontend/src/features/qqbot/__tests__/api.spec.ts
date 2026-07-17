import { beforeEach, describe, expect, it, vi } from 'vitest'

const client = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn(), post: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: client }))

import qqbotAPI from '../api'

describe('QQBot API', () => {
  beforeEach(() => Object.values(client).forEach((mock) => mock.mockReset()))

  it('uses the unified admin QQBot namespace', async () => {
    client.get.mockResolvedValue({ data: {} })
    await qqbotAPI.getConfig()
    await qqbotAPI.getRuntime()
    await qqbotAPI.getStats()
    expect(client.get).toHaveBeenCalledWith('/admin/qqbot/config')
    expect(client.get).toHaveBeenCalledWith('/admin/qqbot/runtime')
    expect(client.get).toHaveBeenCalledWith('/admin/qqbot/stats')
  })

  it('uses the planned public binding endpoints', async () => {
    client.post.mockResolvedValue({ data: { status: 'pending', bonus_amount: 5 } })
    await qqbotAPI.inspectBinding('token-value')
    expect(client.post).toHaveBeenCalledWith('/public/bindings/inspect', { token: 'token-value' })

    await qqbotAPI.completeBinding('token-value', '123456')
    expect(client.post).toHaveBeenCalledWith('/public/bindings/complete', {
      token: 'token-value',
      qq_number: '123456',
    })
  })

  it('passes filters and unbind reasons without an admin subject', async () => {
    client.get.mockResolvedValue({ data: { items: [], total: 0, page: 1, page_size: 20, pages: 1 } })
    await qqbotAPI.listBindings({ status: 'completed', scene: '', search: 'fingerprint', from: '', to: '' }, 2, 20)
    expect(client.get).toHaveBeenCalledWith('/admin/qqbot/bindings', {
      params: { page: 2, page_size: 20, status: 'completed', search: 'fingerprint' },
    })

    client.post.mockResolvedValue({ data: { status: 'revoked' } })
    await qqbotAPI.unbind('42', 'duplicate identity')
    expect(client.post).toHaveBeenCalledWith('/admin/qqbot/bindings/42/unbind', { reason: 'duplicate identity' })
  })
})

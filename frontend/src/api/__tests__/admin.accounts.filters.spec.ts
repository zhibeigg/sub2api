import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({
  get: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get }
}))

import { exportData, list, listWithEtag } from '@/api/admin/accounts'

describe('admin account plan filters API', () => {
  beforeEach(() => {
    get.mockReset()
  })

  it('passes plan_type through paginated list requests', async () => {
    get.mockResolvedValueOnce({
      data: { items: [], total: 0, page: 1, page_size: 20, pages: 0 }
    })

    await list(2, 50, { platform: 'openai', plan_type: 'k12', status: 'active' })

    expect(get).toHaveBeenCalledWith('/admin/accounts', {
      params: {
        page: 2,
        page_size: 50,
        platform: 'openai',
        plan_type: 'k12',
        status: 'active'
      },
      signal: undefined
    })
  })

  it('passes plan_type through ETag refresh requests', async () => {
    get.mockResolvedValueOnce({
      status: 304,
      headers: { etag: '"same"' },
      data: null
    })

    await listWithEtag(1, 20, { plan_type: 'plus' }, { etag: '"same"' })

    expect(get).toHaveBeenCalledWith('/admin/accounts', expect.objectContaining({
      params: expect.objectContaining({ plan_type: 'plus' }),
      headers: { 'If-None-Match': '"same"' }
    }))
  })

  it('passes plan_type through filtered export requests', async () => {
    get.mockResolvedValueOnce({ data: { type: 'sub2api', accounts: [] } })

    await exportData({
      filters: {
        platform: 'openai',
        type: 'oauth',
        plan_type: '__unset__',
        sort_by: 'name',
        sort_order: 'asc'
      }
    })

    expect(get).toHaveBeenCalledWith('/admin/accounts/data', {
      params: {
        platform: 'openai',
        type: 'oauth',
        plan_type: '__unset__',
        sort_by: 'name',
        sort_order: 'asc'
      }
    })
  })
})

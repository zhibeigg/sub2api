import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { post }
}))

import { pollDashboardAuth, startDashboardAuth } from '@/api/admin/cursor'

describe('admin Cursor Dashboard authorization API', () => {
  beforeEach(() => {
    post.mockReset()
  })

  it('starts a server-side authorization session with only the account id', async () => {
    const result = {
      session_id: 'session-1',
      auth_url: 'https://cursor.example/authorize',
      expires_at: '2026-06-01T00:00:00Z'
    }
    post.mockResolvedValue({ data: result })

    await expect(startDashboardAuth(42)).resolves.toEqual(result)
    expect(post).toHaveBeenCalledWith('/admin/cursor/dashboard-auth/start', { account_id: 42 })
  })

  it('polls by session id without accepting or returning tokens', async () => {
    const result = { status: 'connected', account_id: 42 }
    post.mockResolvedValue({ data: result })

    await expect(pollDashboardAuth('session-1')).resolves.toEqual(result)
    expect(post).toHaveBeenCalledWith('/admin/cursor/dashboard-auth/poll', { session_id: 'session-1' })
    expect(result).not.toHaveProperty('access_token')
    expect(result).not.toHaveProperty('refresh_token')
  })
})

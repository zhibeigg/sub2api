import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put }
}))

import {
  create,
  getEmailCapability,
  getEmailStatus,
  retryEmailNotification,
  update
} from '@/api/admin/announcements'

const notification = {
  status: 'pending' as const,
  total_count: 12,
  sent_count: 0,
  failed_count: 0,
  ambiguous_count: 0,
  skipped_count: 0,
  can_retry: false
}

describe('admin announcements email API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
  })

  it('sends send_email and a stable Idempotency-Key for create and update', async () => {
    post.mockResolvedValue({ data: { id: 1 } })
    put.mockResolvedValue({ data: { id: 1 } })

    await create({
      title: 'Maintenance',
      content: 'Window',
      targeting: { any_of: [] },
      status: 'active',
      send_email: true
    }, 'session-key')
    await update(1, { content: 'Updated', send_email: true }, { idempotencyKey: 'session-key' })

    expect(post).toHaveBeenCalledWith(
      '/admin/announcements',
      expect.objectContaining({ send_email: true }),
      { headers: { 'Idempotency-Key': 'session-key' } }
    )
    expect(put).toHaveBeenCalledWith(
      '/admin/announcements/1',
      { content: 'Updated', send_email: true },
      { headers: { 'Idempotency-Key': 'session-key' } }
    )
  })

  it('keeps existing non-email create calls compatible without an idempotency header', async () => {
    post.mockResolvedValue({ data: { id: 2 } })
    const payload = { title: 'Draft', content: 'Body', targeting: { any_of: [] } }

    await create(payload)

    expect(post).toHaveBeenCalledWith('/admin/announcements', payload, undefined)
  })

  it('loads capability and status from the announcement email endpoints', async () => {
    const capability = { enabled: true, smtp_configured: true, eligible_count: 12 }
    get.mockResolvedValueOnce({ data: capability }).mockResolvedValueOnce({ data: notification })

    await expect(getEmailCapability()).resolves.toEqual(capability)
    await expect(getEmailStatus(7)).resolves.toEqual(notification)

    expect(get).toHaveBeenNthCalledWith(1, '/admin/announcements/email-capability', { signal: undefined })
    expect(get).toHaveBeenNthCalledWith(2, '/admin/announcements/7/email-notification', { signal: undefined })
  })

  it('passes include_ambiguous and Idempotency-Key when retrying', async () => {
    post.mockResolvedValue({ data: { email_notification: notification } })

    await retryEmailNotification(7, { include_ambiguous: true }, 'retry-key')

    expect(post).toHaveBeenCalledWith(
      '/admin/announcements/7/email-notification/retry',
      { include_ambiguous: true },
      { headers: { 'Idempotency-Key': 'retry-key' } }
    )
  })
})

import { describe, expect, it, vi } from 'vitest'
import type { Announcement, AnnouncementEmailJobStatus } from '@/types'
import {
  announcementEmailStatusLabelKey,
  createAnnouncementIdempotencyKey,
  isActiveAnnouncementEmailStatus,
  requiresAnnouncementEmailConfirmation,
  shouldPollAnnouncementEmails
} from '../announcementEmailNotification'

function announcement(status?: AnnouncementEmailJobStatus): Announcement {
  return {
    id: 1,
    title: 'Notice',
    content: 'Body',
    status: 'active',
    notify_mode: 'silent',
    targeting: { any_of: [] },
    created_at: '2026-05-01T00:00:00Z',
    updated_at: '2026-05-01T00:00:00Z',
    email_notification: status ? {
      status,
      total_count: 10,
      sent_count: 2,
      failed_count: 0,
      ambiguous_count: 0,
      skipped_count: 0,
      can_retry: false
    } : null
  }
}

describe('announcement email notification helpers', () => {
  it('polls only while a job is pending, preparing, or sending', () => {
    for (const status of ['pending', 'preparing', 'sending'] as const) {
      expect(isActiveAnnouncementEmailStatus(status)).toBe(true)
      expect(shouldPollAnnouncementEmails([announcement(status)])).toBe(true)
    }
    for (const status of ['completed', 'completed_with_failures', 'failed', 'cancelled'] as const) {
      expect(isActiveAnnouncementEmailStatus(status)).toBe(false)
      expect(shouldPollAnnouncementEmails([announcement(status)])).toBe(false)
    }
    expect(shouldPollAnnouncementEmails([announcement()])).toBe(false)
  })

  it('maps missing and terminal statuses to stable locale keys', () => {
    expect(announcementEmailStatusLabelKey()).toBe('admin.announcements.email.status.notSent')
    expect(announcementEmailStatusLabelKey('completed_with_failures')).toBe(
      'admin.announcements.email.status.completed_with_failures'
    )
  })

  it('requires the high-cost confirmation only when email is selected', () => {
    expect(requiresAnnouncementEmailConfirmation(true)).toBe(true)
    expect(requiresAnnouncementEmailConfirmation(false)).toBe(false)
  })

  it('uses crypto.randomUUID for a stable session idempotency key', () => {
    const randomUUID = vi.spyOn(crypto, 'randomUUID').mockReturnValue('9d94754f-4af0-44ae-bdc8-f675ab5cc240')
    expect(createAnnouncementIdempotencyKey()).toBe('9d94754f-4af0-44ae-bdc8-f675ab5cc240')
    randomUUID.mockRestore()
  })
})

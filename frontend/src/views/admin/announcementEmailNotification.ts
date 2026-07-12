import type { Announcement, AnnouncementEmailJobStatus } from '@/types'

export const ACTIVE_ANNOUNCEMENT_EMAIL_STATUSES: readonly AnnouncementEmailJobStatus[] = [
  'pending',
  'preparing',
  'sending'
]

export function isActiveAnnouncementEmailStatus(status?: AnnouncementEmailJobStatus | null): boolean {
  return Boolean(status && ACTIVE_ANNOUNCEMENT_EMAIL_STATUSES.includes(status))
}

export function shouldPollAnnouncementEmails(announcements: Announcement[]): boolean {
  return announcements.some((announcement) =>
    isActiveAnnouncementEmailStatus(announcement.email_notification?.status)
  )
}

export function announcementEmailStatusLabelKey(status?: AnnouncementEmailJobStatus | null): string {
  return `admin.announcements.email.status.${status || 'notSent'}`
}

export function requiresAnnouncementEmailConfirmation(sendEmail: boolean): boolean {
  return sendEmail
}

export function createAnnouncementIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `announcement-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

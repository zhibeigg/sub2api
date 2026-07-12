/**
 * Admin Announcements API endpoints
 */

import { apiClient } from '../client'
import type {
  Announcement,
  AnnouncementEmailCapability,
  AnnouncementEmailNotificationStatus,
  AnnouncementUserReadStatus,
  BasePaginationResponse,
  CreateAnnouncementRequest,
  RetryAnnouncementEmailNotificationRequest,
  RetryAnnouncementEmailNotificationResponse,
  UpdateAnnouncementRequest
} from '@/types'

export type IdempotencyKeyOptions = string | { idempotencyKey?: string }

function resolveIdempotencyKey(options?: IdempotencyKeyOptions): string | undefined {
  return typeof options === 'string' ? options : options?.idempotencyKey
}

function idempotencyConfig(options?: IdempotencyKeyOptions) {
  const idempotencyKey = resolveIdempotencyKey(options)
  return idempotencyKey ? { headers: { 'Idempotency-Key': idempotencyKey } } : undefined
}

export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    status?: string
    search?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  },
  options?: {
    signal?: AbortSignal
  }
): Promise<BasePaginationResponse<Announcement>> {
  const { data } = await apiClient.get<BasePaginationResponse<Announcement>>('/admin/announcements', {
    params: { page, page_size: pageSize, ...filters },
    signal: options?.signal
  })
  return data
}

export async function getById(id: number): Promise<Announcement> {
  const { data } = await apiClient.get<Announcement>(`/admin/announcements/${id}`)
  return data
}

export async function create(
  request: CreateAnnouncementRequest,
  options?: IdempotencyKeyOptions
): Promise<Announcement> {
  const config = request.send_email ? idempotencyConfig(options) : undefined
  const { data } = await apiClient.post<Announcement>('/admin/announcements', request, config)
  return data
}

export async function update(
  id: number,
  request: UpdateAnnouncementRequest,
  options?: IdempotencyKeyOptions
): Promise<Announcement> {
  const config = request.send_email ? idempotencyConfig(options) : undefined
  const { data } = await apiClient.put<Announcement>(`/admin/announcements/${id}`, request, config)
  return data
}

export async function deleteAnnouncement(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/announcements/${id}`)
  return data
}

export async function getReadStatus(
  id: number,
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    search?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  },
  options?: {
    signal?: AbortSignal
  }
): Promise<BasePaginationResponse<AnnouncementUserReadStatus>> {
  const { data } = await apiClient.get<BasePaginationResponse<AnnouncementUserReadStatus>>(
    `/admin/announcements/${id}/read-status`,
    {
      params: { page, page_size: pageSize, ...filters },
      signal: options?.signal
    }
  )
  return data
}

export async function getEmailCapability(options?: {
  signal?: AbortSignal
}): Promise<AnnouncementEmailCapability> {
  const { data } = await apiClient.get<AnnouncementEmailCapability>(
    '/admin/announcements/email-capability',
    { signal: options?.signal }
  )
  return data
}

export async function getEmailStatus(
  id: number,
  options?: { signal?: AbortSignal }
): Promise<AnnouncementEmailNotificationStatus> {
  const { data } = await apiClient.get<AnnouncementEmailNotificationStatus>(
    `/admin/announcements/${id}/email-notification`,
    { signal: options?.signal }
  )
  return data
}

export async function retryEmailNotification(
  id: number,
  request: RetryAnnouncementEmailNotificationRequest = {},
  options?: IdempotencyKeyOptions
): Promise<RetryAnnouncementEmailNotificationResponse> {
  const { data } = await apiClient.post<RetryAnnouncementEmailNotificationResponse>(
    `/admin/announcements/${id}/email-notification/retry`,
    request,
    idempotencyConfig(options)
  )
  return data
}

const announcementsAPI = {
  list,
  getById,
  create,
  update,
  delete: deleteAnnouncement,
  getReadStatus,
  getEmailCapability,
  getEmailStatus,
  retryEmailNotification
}

export default announcementsAPI

import { apiClient } from '../client'

export type CursorDashboardAuthStatus = 'pending' | 'connected' | 'expired' | 'error'

export interface CursorDashboardAuthStartResult {
  session_id: string
  auth_url: string
  expires_at: string
}

export interface CursorDashboardAuthPollResult {
  status: CursorDashboardAuthStatus
  account_id?: number
  expires_at?: string
  message?: string
}

export async function startDashboardAuth(accountId: number): Promise<CursorDashboardAuthStartResult> {
  const { data } = await apiClient.post<CursorDashboardAuthStartResult>(
    '/admin/cursor/dashboard-auth/start',
    { account_id: accountId }
  )
  return data
}

export async function pollDashboardAuth(sessionId: string): Promise<CursorDashboardAuthPollResult> {
  const { data } = await apiClient.post<CursorDashboardAuthPollResult>(
    '/admin/cursor/dashboard-auth/poll',
    { session_id: sessionId }
  )
  return data
}

export const cursorAPI = {
  startDashboardAuth,
  pollDashboardAuth
}

export default cursorAPI

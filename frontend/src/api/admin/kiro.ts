/**
 * Admin Kiro / AWS CodeWhisperer API endpoints.
 * Handles interactive login (Builder ID device code, IAM Identity Center PKCE,
 * SSO token import), account creation, token refresh, and usage/overage.
 */

import { apiClient } from '../client'

export interface KiroTokenInfo {
  AccessToken?: string
  RefreshToken?: string
  ExpiresAt?: number
  ProfileArn?: string
  ClientID?: string
  ClientSecret?: string
  AuthMethod?: string
  Region?: string
  MachineID?: string
  Provider?: string
  Email?: string
  [key: string]: unknown
}

export interface KiroCredentials {
  [key: string]: unknown
}

export interface KiroDeviceLoginResult {
  session_id: string
  user_code: string
  verification_uri: string
  interval: number
  expires_in: number
}

export interface KiroAuthUrlResult {
  session_id: string
  auth_url: string
  state: string
  expires_in: number
}

export interface KiroDevicePollResult {
  status: 'pending' | 'completed'
  token_info?: KiroTokenInfo
  credentials?: KiroCredentials
}

export interface KiroLoginTokenResult {
  token_info: KiroTokenInfo
  credentials: KiroCredentials
}

export interface KiroUsageSnapshot {
  subscription_type?: string
  subscription_raw?: string
  usage_current?: number
  usage_limit?: number
  usage_percent?: number
  trial_current?: number
  trial_limit?: number
  trial_status?: string
  next_reset_date?: string
  overage_status?: string
  overage_cap?: number
  overage_rate?: number
  current_overages?: number
  email?: string
  user_id?: string
  checked_at?: number
}

export interface KiroUsageProbeResult {
  snapshot: KiroUsageSnapshot
  fetched_at: number
}

// ---- Builder ID device code ----

export async function startBuilderID(payload: {
  region?: string
  proxy_id?: number
}): Promise<KiroDeviceLoginResult> {
  const { data } = await apiClient.post<KiroDeviceLoginResult>(
    '/admin/kiro/oauth/builderid/start',
    payload
  )
  return data
}

export async function pollBuilderID(sessionId: string): Promise<KiroDevicePollResult> {
  const { data } = await apiClient.post<KiroDevicePollResult>('/admin/kiro/oauth/builderid/poll', {
    session_id: sessionId,
  })
  return data
}

// ---- IAM Identity Center PKCE ----

export async function startIAMSSO(payload: {
  start_url?: string
  region?: string
  proxy_id?: number
}): Promise<KiroAuthUrlResult> {
  const { data } = await apiClient.post<KiroAuthUrlResult>('/admin/kiro/oauth/iam-sso/start', payload)
  return data
}

export async function completeIAMSSO(payload: {
  session_id: string
  callback_url: string
}): Promise<KiroLoginTokenResult> {
  const { data } = await apiClient.post<KiroLoginTokenResult>(
    '/admin/kiro/oauth/iam-sso/complete',
    payload
  )
  return data
}

// ---- SSO token import ----

export async function importSSOToken(payload: {
  bearer_token: string
  region?: string
  proxy_id?: number
}): Promise<KiroLoginTokenResult> {
  const { data } = await apiClient.post<KiroLoginTokenResult>('/admin/kiro/oauth/sso-token', payload)
  return data
}

// ---- Account creation from resolved credentials ----

export async function createAccount(payload: {
  credentials: KiroCredentials
  name?: string
  proxy_id?: number
  concurrency?: number
  priority?: number
  group_ids?: number[]
  model_mapping?: Record<string, string>
}): Promise<unknown> {
  const { data } = await apiClient.post('/admin/kiro/oauth/create-account', payload)
  return data
}

// ---- Refresh / usage / overage ----

export async function refreshAccountToken(id: number): Promise<unknown> {
  const { data } = await apiClient.post(`/admin/kiro/accounts/${id}/refresh`)
  return data
}

export async function queryUsage(id: number): Promise<KiroUsageProbeResult> {
  const { data } = await apiClient.get<KiroUsageProbeResult>(`/admin/kiro/accounts/${id}/usage`)
  return data
}

export async function setOverage(id: number, enabled: boolean): Promise<KiroUsageProbeResult> {
  const { data } = await apiClient.post<KiroUsageProbeResult>(`/admin/kiro/accounts/${id}/overage`, {
    enabled,
  })
  return data
}

export default {
  startBuilderID,
  pollBuilderID,
  startIAMSSO,
  completeIAMSSO,
  importSSOToken,
  createAccount,
  refreshAccountToken,
  queryUsage,
  setOverage,
}

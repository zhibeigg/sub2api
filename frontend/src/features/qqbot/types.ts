export type QQBotRuntimeStatus = 'disabled' | 'starting' | 'running' | 'reloading' | 'degraded' | string

export interface QQBotConfig {
  enabled: boolean
  app_id: string
  app_secret_configured: boolean
  webhook_secret_configured: boolean
  sandbox: boolean
  public_base_url: string
  worker_count: number
  queue_capacity: number
  api_timeout_ms: number
  binding_enabled: boolean
  first_bind_bonus: number
  link_ttl_minutes: number
  welcome_enabled: boolean
  first_interaction_enabled: boolean
  channel_check_enabled: boolean
  help_message: string
  allowed_group_ids: string[]
  allowed_guild_ids: string[]
  guild_welcome_channels: Record<string, string>
  config_version: number
  updated_at?: string
  updated_by?: number
  change_summary?: string
}

export interface QQBotDraft extends Omit<QQBotConfig, 'allowed_group_ids' | 'allowed_guild_ids' | 'guild_welcome_channels'> {
  app_secret: string
  webhook_secret: string
  allowed_group_ids_text: string
  allowed_guild_ids_text: string
  guild_welcome_channels_text: string
}

export interface QQBotUpdateRequest {
  expected_config_version: number
  enabled: boolean
  app_id: string
  app_secret?: string
  webhook_secret?: string
  sandbox: boolean
  public_base_url: string
  worker_count: number
  queue_capacity: number
  api_timeout_ms: number
  binding_enabled: boolean
  first_bind_bonus: number
  link_ttl_minutes: number
  welcome_enabled: boolean
  first_interaction_enabled: boolean
  channel_check_enabled: boolean
  help_message: string
  allowed_group_ids: string[]
  allowed_guild_ids: string[]
  guild_welcome_channels: Record<string, string>
}

export interface QQBotProbeRequest {
  app_id: string
  app_secret?: string
  webhook_secret?: string
  sandbox: boolean
  public_base_url: string
  api_timeout_ms: number
  channel_check_enabled: boolean
}

export interface QQBotProbeResult {
  ok: boolean
  status: string
  message: string
  error_code?: string
  latency_ms?: number
  checked_at: string
}

export interface QQBotRuntime {
  process_status: QQBotRuntimeStatus
  desired_config_version: number
  active_config_version: number
  worker_total: number
  worker_active: number
  stream_backlog: number
  stream_pending: number
  dead_letter_total?: number
  last_webhook_at?: string
  last_event_at?: string
  last_send_at?: string
  last_error_code?: string
  last_error_message?: string
  last_error_at?: string
}

export interface QQBotStats {
  today_requests: number
  total_requests: number
  completed: number
  pending: number
  expired: number
  failed: number
  revoked: number
  granted_total: number
  today_granted_total: number
  completion_rate: number
}

export interface QQBotBindingRecord {
  id: string
  status: string
  masked_email: string
  openid_fingerprint: string
  scene: string
  source_id?: string
  channel_id?: string
  declared_qq_number?: string
  bonus_amount: number
  balance_before?: number
  balance_after?: number
  failure_code?: string
  email_status?: string
  notification_status?: string
  created_at: string
  expires_at: string
  completed_at?: string
  revoked_at?: string
}

export interface QQBotBindingPage {
  items: QQBotBindingRecord[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface QQBotBindingFilters {
  status: string
  scene: string
  search: string
  from: string
  to: string
}

export interface BindingInspection {
  status: 'pending' | 'completed' | 'expired' | 'revoked' | 'failed' | 'service_disabled' | string
  masked_email?: string
  scene?: string
  bonus_amount: number
  expires_at?: string
  completed_at?: string
  balance_after?: number
  declared_qq_number?: string
}

export interface CompleteBindingResponse {
  status: string
  granted: boolean
  bonus_amount: number
  balance_after?: number
  masked_email?: string
  declared_qq_number?: string
}

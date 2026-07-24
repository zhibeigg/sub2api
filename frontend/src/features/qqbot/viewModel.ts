import type {
  QQBotBindingFilters,
  QQBotConfig,
  QQBotDraft,
  QQBotOneBotConfig,
  QQBotOneBotDraft,
  QQBotOneBotProbeRequest,
  QQBotOneBotUpdateRequest,
  QQBotProbeRequest,
  QQBotUpdateRequest,
} from './types'

export function cloneData<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

export function parseIDLines(value: string): string[] {
  return [...new Set(value.split(/\r?\n/).map((line) => line.trim()).filter(Boolean))].sort()
}

export function parseChannelMapping(value: string): Record<string, string> | null {
  const result: Record<string, string> = {}
  for (const line of value.split(/\r?\n/).map((item) => item.trim()).filter(Boolean)) {
    const parts = line.split('=').map((item) => item.trim())
    if (parts.length !== 2 || !parts[0] || !parts[1]) return null
    result[parts[0]] = parts[1]
  }
  return result
}

export function configToDraft(config: QQBotConfig): QQBotDraft {
  return {
    ...cloneData(config),
    app_secret: '',
    webhook_secret: '',
    allowed_group_ids_text: (config.allowed_group_ids ?? []).join('\n'),
    allowed_guild_ids_text: (config.allowed_guild_ids ?? []).join('\n'),
    guild_welcome_channels_text: Object.entries(config.guild_welcome_channels ?? {})
      .map(([guildID, channelID]) => `${guildID} = ${channelID}`)
      .join('\n'),
  }
}

export function buildUpdateRequest(draft: QQBotDraft): QQBotUpdateRequest {
  const mapping = parseChannelMapping(draft.guild_welcome_channels_text)
  if (mapping === null) throw new Error('invalid_channel_mapping')
  const payload: QQBotUpdateRequest = {
    expected_config_version: draft.config_version,
    enabled: draft.enabled,
    app_id: draft.app_id.trim(),
    sandbox: draft.sandbox,
    public_base_url: draft.public_base_url.trim().replace(/\/$/, ''),
    worker_count: Number(draft.worker_count),
    queue_capacity: Number(draft.queue_capacity),
    api_timeout_ms: Number(draft.api_timeout_ms),
    binding_enabled: draft.binding_enabled,
    first_bind_bonus: Number(draft.first_bind_bonus),
    link_ttl_minutes: Number(draft.link_ttl_minutes),
    command_cooldown_seconds: Number(draft.command_cooldown_seconds),
    welcome_enabled: draft.welcome_enabled,
    welcome_message: draft.welcome_message.trim(),
    first_interaction_enabled: draft.first_interaction_enabled,
    channel_check_enabled: draft.channel_check_enabled,
    help_message: draft.help_message.trim(),
    allowed_group_ids: parseIDLines(draft.allowed_group_ids_text),
    allowed_guild_ids: parseIDLines(draft.allowed_guild_ids_text),
    guild_welcome_channels: mapping,
  }
  if (draft.app_secret.trim()) payload.app_secret = draft.app_secret.trim()
  if (draft.webhook_secret.trim()) payload.webhook_secret = draft.webhook_secret.trim()
  return payload
}

export function buildProbeRequest(draft: QQBotDraft): QQBotProbeRequest {
  const payload: QQBotProbeRequest = {
    app_id: draft.app_id.trim(),
    sandbox: draft.sandbox,
    public_base_url: draft.public_base_url.trim().replace(/\/$/, ''),
    api_timeout_ms: Number(draft.api_timeout_ms),
    channel_check_enabled: draft.channel_check_enabled,
  }
  if (draft.app_secret.trim()) payload.app_secret = draft.app_secret.trim()
  if (draft.webhook_secret.trim()) payload.webhook_secret = draft.webhook_secret.trim()
  return payload
}

export function oneBotConfigToDraft(config: QQBotOneBotConfig): QQBotOneBotDraft {
  return { ...cloneData(config), access_token: '' }
}

export function buildOneBotUpdateRequest(draft: QQBotOneBotDraft): QQBotOneBotUpdateRequest {
  const payload: QQBotOneBotUpdateRequest = {
    expected_config_version: draft.config_version,
    enabled: draft.enabled,
    self_id: draft.self_id.trim(),
    worker_count: Number(draft.worker_count),
    queue_capacity: Number(draft.queue_capacity),
    action_timeout_ms: Number(draft.action_timeout_ms),
    auto_approve_friend_requests: draft.auto_approve_friend_requests,
    auto_approve_group_requests: draft.auto_approve_group_requests,
  }
  if (draft.access_token.trim()) payload.access_token = draft.access_token.trim()
  return payload
}

export function buildOneBotProbeRequest(draft: QQBotOneBotDraft): QQBotOneBotProbeRequest {
  const payload: QQBotOneBotProbeRequest = {
    self_id: draft.self_id.trim(),
    action_timeout_ms: Number(draft.action_timeout_ms),
  }
  if (draft.access_token.trim()) payload.access_token = draft.access_token.trim()
  return payload
}

export function oneBotCredentialsReady(draft: QQBotOneBotDraft): boolean {
  return Boolean(/^\d{5,20}$/.test(draft.self_id.trim()) && (draft.access_token_configured || draft.access_token.trim().length >= 32))
}

export function validateOneBotDraft(draft: QQBotOneBotDraft): string[] {
  const errors: string[] = []
  if (!/^\d{5,20}$/.test(draft.self_id.trim())) errors.push('oneBotSelfId')
  if (draft.access_token.trim() && draft.access_token.trim().length < 32) errors.push('oneBotToken')
  if (!Number.isInteger(Number(draft.worker_count)) || Number(draft.worker_count) < 1 || Number(draft.worker_count) > 64) errors.push('workers')
  if (!Number.isInteger(Number(draft.queue_capacity)) || Number(draft.queue_capacity) < 16 || Number(draft.queue_capacity) > 100_000) errors.push('oneBotQueue')
  if (!Number.isInteger(Number(draft.action_timeout_ms)) || Number(draft.action_timeout_ms) < 500 || Number(draft.action_timeout_ms) > 30_000) errors.push('timeout')
  return errors
}

export function oneBotCredentialFingerprint(draft: QQBotOneBotDraft): string {
  return JSON.stringify(buildOneBotProbeRequest(draft))
}

export function oneBotDraftFingerprint(draft: QQBotOneBotDraft | null): string {
  if (!draft) return ''
  return JSON.stringify(buildOneBotUpdateRequest(draft))
}

export function credentialsReady(draft: QQBotDraft): boolean {
  return Boolean(
    draft.app_id.trim() &&
      (draft.app_secret_configured || draft.app_secret.trim()) &&
      (draft.webhook_secret_configured || draft.webhook_secret.trim()),
  )
}

export function credentialFingerprint(draft: QQBotDraft): string {
  return JSON.stringify(buildProbeRequest(draft))
}

export function draftFingerprint(draft: QQBotDraft | null): string {
  if (!draft) return ''
  try {
    return JSON.stringify(buildUpdateRequest(draft))
  } catch {
    return JSON.stringify(draft)
  }
}

function isChannelCheckPublicURL(url: URL): boolean {
  const hostname = url.hostname.toLowerCase().replace(/^\[|\]$/g, '')
  if (url.protocol !== 'https:' || url.pathname !== '/' || url.search || url.hash || url.username || url.password) return false
  if (!hostname.includes('.') || hostname === 'localhost' || hostname.endsWith('.localhost') || hostname.endsWith('.local') || hostname.endsWith('.internal') || hostname === '::1') return false
  const octets = hostname.split('.').map((value) => Number(value))
  if (octets.length === 4 && octets.every((value) => Number.isInteger(value) && value >= 0 && value <= 255)) {
    const [first, second] = octets
    if (first === 0 || first === 10 || first === 127 || (first === 169 && second === 254) || (first === 172 && second >= 16 && second <= 31) || (first === 192 && second === 168)) return false
  }
  return true
}

export function validateDraft(draft: QQBotDraft): string[] {
  const errors: string[] = []
  if (!/^\d{1,64}$/.test(draft.app_id.trim())) errors.push('appId')
  try {
    const url = new URL(draft.public_base_url)
    if ((url.protocol !== 'https:' && url.protocol !== 'http:') || (draft.channel_check_enabled && !isChannelCheckPublicURL(url))) errors.push('publicUrl')
  } catch {
    errors.push('publicUrl')
  }
  if (!Number.isInteger(Number(draft.worker_count)) || Number(draft.worker_count) < 1 || Number(draft.worker_count) > 64) errors.push('workers')
  if (!Number.isInteger(Number(draft.queue_capacity)) || Number(draft.queue_capacity) < 100 || Number(draft.queue_capacity) > 100_000) errors.push('queue')
  if (!Number.isInteger(Number(draft.api_timeout_ms)) || Number(draft.api_timeout_ms) < 500 || Number(draft.api_timeout_ms) > 30_000) errors.push('timeout')
  if (!Number.isFinite(Number(draft.first_bind_bonus)) || Number(draft.first_bind_bonus) < 0) errors.push('bonus')
  if (!Number.isInteger(Number(draft.link_ttl_minutes)) || Number(draft.link_ttl_minutes) < 5 || Number(draft.link_ttl_minutes) > 1440) errors.push('ttl')
  if (!Number.isInteger(Number(draft.command_cooldown_seconds)) || Number(draft.command_cooldown_seconds) < 10 || Number(draft.command_cooldown_seconds) > 3600) errors.push('cooldown')
  if (draft.welcome_message.length > 4000) errors.push('welcome')
  if (draft.help_message.length > 4000) errors.push('help')
  if (parseChannelMapping(draft.guild_welcome_channels_text) === null) errors.push('mapping')
  return errors
}

export function bindingQueryParams(filters: QQBotBindingFilters, page: number, pageSize: number) {
  const params: Record<string, string | number> = { page, page_size: pageSize }
  for (const [key, value] of Object.entries(filters)) {
    const trimmed = value.trim()
    if (trimmed) params[key] = trimmed
  }
  return params
}

export function webhookURL(config: Pick<QQBotConfig, 'public_base_url'>): string {
  return `${config.public_base_url.replace(/\/$/, '')}/webhooks/qq`
}

export function validationURL(config: Pick<QQBotConfig, 'public_base_url' | 'app_id'>): string {
  return `${config.public_base_url.replace(/\/$/, '')}/${config.app_id}.json`
}

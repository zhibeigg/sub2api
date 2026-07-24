import { describe, expect, it } from 'vitest'
import type { QQBotConfig, QQBotOneBotConfig } from '../types'
import {
  buildProbeRequest,
  buildUpdateRequest,
  configToDraft,
  credentialFingerprint,
  credentialsReady,
  draftFingerprint,
  oneBotConfigToDraft,
  oneBotCredentialFingerprint,
  oneBotCredentialsReady,
  buildOneBotProbeRequest,
  buildOneBotUpdateRequest,
  parseChannelMapping,
  validateDraft,
  validateOneBotDraft,
} from '../viewModel'

const config = (): QQBotConfig => ({
  enabled: false,
  app_id: '102087449',
  app_secret_configured: true,
  webhook_secret_configured: true,
  sandbox: false,
  public_base_url: 'https://qqbot.example.com',
  worker_count: 4,
  queue_capacity: 1000,
  api_timeout_ms: 5000,
  binding_enabled: true,
  first_bind_bonus: 5,
  link_ttl_minutes: 15,
  command_cooldown_seconds: 60,
  welcome_enabled: true,
  welcome_message: 'welcome {user}',
  first_interaction_enabled: true,
  channel_check_enabled: false,
  help_message: 'help',
  allowed_group_ids: ['2', '1'],
  allowed_guild_ids: ['g1'],
  guild_welcome_channels: { g1: 'c1' },
  config_version: 7,
})

const oneBotConfig = (): QQBotOneBotConfig => ({
  enabled: false,
  self_id: '123456789',
  access_token_configured: true,
  worker_count: 2,
  queue_capacity: 1024,
  action_timeout_ms: 10000,
  auto_approve_friend_requests: false,
  auto_approve_group_requests: false,
  reverse_ws_url: 'ws://127.0.0.1:8080/webhooks/qq/onebot',
  config_version: 3,
})

describe('QQBot view model', () => {
  it('models configured secrets without copying plaintext into the draft', () => {
    const draft = configToDraft(config())
    expect(draft.app_secret).toBe('')
    expect(draft.webhook_secret).toBe('')
    expect(draft.app_secret_configured).toBe(true)
    expect(draft.webhook_secret_configured).toBe(true)
    expect(credentialsReady(draft)).toBe(true)
  })

  it('omits blank secrets to preserve stored values and sends explicit replacements only', () => {
    const draft = configToDraft(config())
    expect(buildUpdateRequest(draft)).not.toHaveProperty('app_secret')
    expect(buildUpdateRequest(draft)).not.toHaveProperty('webhook_secret')
    expect(buildProbeRequest(draft)).not.toHaveProperty('app_secret')

    draft.app_secret = 'new-app-secret'
    draft.webhook_secret = 'new-webhook-secret'
    expect(buildUpdateRequest(draft)).toMatchObject({
      app_secret: 'new-app-secret',
      webhook_secret: 'new-webhook-secret',
    })
  })

  it('round-trips the channel status image switch and includes it in dirty-state fingerprints', () => {
    const draft = configToDraft(config())
    expect(draft.channel_check_enabled).toBe(false)
    expect(buildUpdateRequest(draft).channel_check_enabled).toBe(false)
    expect(buildProbeRequest(draft).channel_check_enabled).toBe(false)

    const before = draftFingerprint(draft)
    draft.channel_check_enabled = true

    expect(buildUpdateRequest(draft).channel_check_enabled).toBe(true)
    expect(draftFingerprint(draft)).not.toBe(before)
  })

  it('round-trips and validates the member welcome message', () => {
    const draft = configToDraft(config())
    expect(draft.welcome_message).toBe('welcome {user}')

    draft.welcome_message = '  hello {site} {user}  '
    expect(buildUpdateRequest(draft).welcome_message).toBe('hello {site} {user}')

    draft.welcome_message = 'x'.repeat(4001)
    expect(validateDraft(draft)).toContain('welcome')
  })

  it('requires configured or newly entered credentials before enablement', () => {
    const draft = configToDraft({ ...config(), app_secret_configured: false })
    expect(credentialsReady(draft)).toBe(false)
    draft.app_secret = 'temporary-secret'
    expect(credentialsReady(draft)).toBe(true)
  })

  it('normalizes allowlists and rejects malformed identifiers and mappings', () => {
    const draft = configToDraft(config())
    draft.app_id = 'not-numeric'
    expect(validateDraft(draft)).toContain('appId')
    draft.app_id = '102087449'
    draft.allowed_group_ids_text = '2\n1\n2\n'
    draft.guild_welcome_channels_text = 'g1 = c1\ng2 = c2'
    expect(buildUpdateRequest(draft).allowed_group_ids).toEqual(['1', '2'])
    expect(parseChannelMapping('broken line')).toBeNull()
    draft.guild_welcome_channels_text = 'broken line'
    expect(validateDraft(draft)).toContain('mapping')
  })

  it('requires a public root HTTPS URL while channel status images are enabled', () => {
    const draft = configToDraft(config())
    draft.channel_check_enabled = true

    draft.public_base_url = 'http://qqbot.example.com'
    expect(validateDraft(draft)).toContain('publicUrl')
    draft.public_base_url = 'https://qqbot.example.com/proxy'
    expect(validateDraft(draft)).toContain('publicUrl')
    draft.public_base_url = 'https://127.0.0.1'
    expect(validateDraft(draft)).toContain('publicUrl')
    draft.public_base_url = 'https://qqbot.example.com'
    expect(validateDraft(draft)).not.toContain('publicUrl')
  })

  it('changes the credential fingerprint when a secret is rotated', () => {
    const draft = configToDraft(config())
    const before = credentialFingerprint(draft)
    draft.app_secret = 'rotated'
    expect(credentialFingerprint(draft)).not.toBe(before)
  })

  it('keeps the encrypted OneBot token out of drafts and update requests', () => {
    const draft = oneBotConfigToDraft(oneBotConfig())
    expect(draft.access_token).toBe('')
    expect(oneBotCredentialsReady(draft)).toBe(true)
    expect(buildOneBotUpdateRequest(draft)).not.toHaveProperty('access_token')
    expect(buildOneBotProbeRequest(draft)).not.toHaveProperty('access_token')
  })

  it('round-trips OneBot auto-approval switches into update requests and dirty-state fingerprints', () => {
    const draft = oneBotConfigToDraft(oneBotConfig())
    const before = JSON.stringify(buildOneBotUpdateRequest(draft))
    draft.auto_approve_friend_requests = true
    draft.auto_approve_group_requests = true

    expect(buildOneBotUpdateRequest(draft)).toMatchObject({
      auto_approve_friend_requests: true,
      auto_approve_group_requests: true,
    })
    expect(JSON.stringify(buildOneBotUpdateRequest(draft))).not.toBe(before)
  })

  it('validates OneBot credentials and fingerprints token rotations', () => {
    const draft = oneBotConfigToDraft({ ...oneBotConfig(), access_token_configured: false })
    expect(oneBotCredentialsReady(draft)).toBe(false)
    draft.self_id = 'invalid'
    draft.access_token = 'short'
    expect(validateOneBotDraft(draft)).toEqual(expect.arrayContaining(['oneBotSelfId', 'oneBotToken']))
    draft.self_id = '123456789'
    draft.access_token = 'x'.repeat(32)
    const before = oneBotCredentialFingerprint(draft)
    draft.access_token = 'y'.repeat(32)
    expect(oneBotCredentialsReady(draft)).toBe(true)
    expect(oneBotCredentialFingerprint(draft)).not.toBe(before)
  })
})

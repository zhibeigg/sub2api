import { describe, expect, it } from 'vitest'
import type { QQBotConfig } from '../types'
import {
  buildProbeRequest,
  buildUpdateRequest,
  configToDraft,
  credentialFingerprint,
  credentialsReady,
  parseChannelMapping,
  validateDraft,
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
  welcome_enabled: true,
  first_interaction_enabled: true,
  help_message: 'help',
  allowed_group_ids: ['2', '1'],
  allowed_guild_ids: ['g1'],
  guild_welcome_channels: { g1: 'c1' },
  config_version: 7,
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

  it('changes the credential fingerprint when a secret is rotated', () => {
    const draft = configToDraft(config())
    const before = credentialFingerprint(draft)
    draft.app_secret = 'rotated'
    expect(credentialFingerprint(draft)).not.toBe(before)
  })
})

import { nextTick } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'
import {
  migratePlaygroundSettings,
  PLAYGROUND_SETTINGS_DEFAULTS,
  PLAYGROUND_SETTINGS_STORAGE_KEY,
  resetPlaygroundSettingsForTest,
  usePlaygroundSettings
} from '@/composables/usePlaygroundSettings'

const option = {
  group_id: 8,
  group_name: 'group',
  group_priority: 1,
  model: 'model-a',
  platform: 'openai',
  capabilities: ['chat'] as const,
  features: { image_input: true, responses: true, web_search: false, code_execution: false, web_fetch: true }
}

describe('playground settings v3', () => {
  afterEach(() => {
    localStorage.clear()
    resetPlaygroundSettingsForTest()
  })

  it('migrates v1 and v2 without retaining API key plaintext', () => {
    const migratedV1 = migratePlaygroundSettings(null, null, JSON.stringify({
      keyId: 12, model: 'legacy-model', temperature: 0.2, maxTokens: 100, apiKey: 'must-not-survive'
    }))
    expect(migratedV1.selections.chat?.model).toBe('legacy-model')
    expect(JSON.stringify(migratedV1)).not.toContain('must-not-survive')

    const migratedV2 = migratePlaygroundSettings(null, JSON.stringify({
      version: 2,
      keyId: 7,
      selections: { chat: option },
      systemPrompt: 'system',
      temperature: 1.2,
      maxTokens: 200
    }))
    expect(migratedV2.version).toBe(3)
    expect(migratedV2.topP).toBe(1)
    expect(migratedV2.webSearch).toBe(false)
    expect(migratedV2.selections.chat?.features).toEqual(option.features)
  })

  it('cleans ranges and invalid advanced settings', () => {
    const migrated = migratePlaygroundSettings(JSON.stringify({
      version: 3,
      keyId: -3,
      selections: {},
      temperature: 99,
      maxTokens: -20,
      topP: -1,
      reasoningEffort: 'extreme',
      webSearch: 'yes',
      codeExecution: true,
      webFetch: false
    }))
    expect(migrated.keyId).toBeNull()
    expect(migrated.temperature).toBe(2)
    expect(migrated.maxTokens).toBe(0)
    expect(migrated.topP).toBe(0)
    expect(migrated.reasoningEffort).toBe('')
    expect(migrated.webSearch).toBe(false)
    expect(migrated.codeExecution).toBe(true)
  })

  it('persists v3 fields and can restore defaults', async () => {
    const settings = usePlaygroundSettings()
    settings.keyId.value = 4
    settings.chatOption.value = { ...option, capabilities: ['chat'] }
    settings.topP.value = 0.4
    settings.reasoningEffort.value = 'high'
    settings.webSearch.value = true
    settings.codeExecution.value = true
    settings.webFetch.value = true
    await nextTick()

    const persisted = JSON.parse(localStorage.getItem(PLAYGROUND_SETTINGS_STORAGE_KEY) || '{}')
    expect(persisted.version).toBe(3)
    expect(persisted.selections.chat.features.responses).toBe(true)
    expect(persisted.topP).toBe(0.4)
    expect(persisted.reasoningEffort).toBe('high')
    expect(JSON.stringify(persisted)).not.toContain('apiKey')

    settings.restoreDefaults()
    expect(settings.topP.value).toBe(PLAYGROUND_SETTINGS_DEFAULTS.topP)
    expect(settings.reasoningEffort.value).toBe(PLAYGROUND_SETTINGS_DEFAULTS.reasoningEffort)
    expect(settings.webSearch.value).toBe(false)
  })
})

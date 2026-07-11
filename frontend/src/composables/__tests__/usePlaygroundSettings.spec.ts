import { nextTick } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'
import {
  migratePlaygroundSettings,
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
  capabilities: ['chat'] as const
}

describe('playground settings v2', () => {
  afterEach(() => {
    localStorage.clear()
    resetPlaygroundSettingsForTest()
  })

  it('migrates v1 model settings into each mode without persisting an API key value', () => {
    const migrated = migratePlaygroundSettings(null, JSON.stringify({
      keyId: 12,
      model: 'legacy-model',
      systemPrompt: 'system',
      temperature: 0.2,
      maxTokens: 100,
      apiKey: 'must-not-survive'
    }))

    expect(migrated.keyId).toBe(12)
    expect(migrated.selections.chat?.model).toBe('legacy-model')
    expect(migrated.selections.image?.model).toBe('legacy-model')
    expect(migrated.selections.video?.model).toBe('legacy-model')
    expect(JSON.stringify(migrated)).not.toContain('must-not-survive')
  })

  it('persists complete, independent selections by mode', async () => {
    const settings = usePlaygroundSettings()
    settings.keyId.value = 4
    settings.chatOption.value = { ...option, capabilities: ['chat'] }
    settings.imageOption.value = { ...option, group_id: 9, model: 'image-a', capabilities: ['image'] }
    settings.videoOption.value = { ...option, group_id: 10, model: 'video-a', capabilities: ['video'] }
    await nextTick()

    const persisted = JSON.parse(localStorage.getItem(PLAYGROUND_SETTINGS_STORAGE_KEY) || '{}')
    expect(persisted.version).toBe(2)
    expect(persisted.selections.chat.group_id).toBe(8)
    expect(persisted.selections.image.model).toBe('image-a')
    expect(persisted.selections.video.group_id).toBe(10)
    expect(JSON.stringify(persisted)).not.toContain('apiKey')
  })
})

import { ref, watch } from 'vue'
import type { PlaygroundCapability, PlaygroundModelOption } from '@/types/playground'

export const PLAYGROUND_SETTINGS_STORAGE_KEY = 'playground_settings_v2'
const LEGACY_STORAGE_KEY = 'playground_settings_v1'

interface PersistedSettingsV2 {
  version: 2
  keyId: number | null
  selections: Record<PlaygroundCapability, PlaygroundModelOption | null>
  systemPrompt: string
  temperature: number
  maxTokens: number
}

const EMPTY_SELECTIONS: Record<PlaygroundCapability, PlaygroundModelOption | null> = {
  chat: null,
  image: null,
  video: null
}

const DEFAULTS: PersistedSettingsV2 = {
  version: 2,
  keyId: null,
  selections: { ...EMPTY_SELECTIONS },
  systemPrompt: '',
  temperature: 0.7,
  maxTokens: 0
}

function sanitizeOption(value: unknown): PlaygroundModelOption | null {
  if (!value || typeof value !== 'object') return null
  const option = value as Partial<PlaygroundModelOption>
  if (typeof option.model !== 'string' || !option.model.trim()) return null
  return {
    id: typeof option.id === 'string' && option.id ? option.id : undefined,
    group_id: typeof option.group_id === 'number' ? option.group_id : 0,
    group_name: typeof option.group_name === 'string' ? option.group_name : '',
    group_priority: typeof option.group_priority === 'number' ? option.group_priority : 0,
    model: option.model,
    platform: typeof option.platform === 'string' ? option.platform : '',
    capabilities: Array.isArray(option.capabilities)
      ? option.capabilities.filter((cap): cap is PlaygroundCapability =>
          cap === 'chat' || cap === 'image' || cap === 'video'
        )
      : []
  }
}

function legacyOption(model: string, capability: PlaygroundCapability): PlaygroundModelOption | null {
  if (!model.trim()) return null
  return {
    group_id: 0,
    group_name: '',
    group_priority: 0,
    model,
    platform: '',
    capabilities: [capability]
  }
}

export function migratePlaygroundSettings(rawV2: string | null, rawV1: string | null): PersistedSettingsV2 {
  try {
    if (rawV2) {
      const parsed = JSON.parse(rawV2) as Partial<PersistedSettingsV2>
      return {
        version: 2,
        keyId: typeof parsed.keyId === 'number' ? parsed.keyId : null,
        selections: {
          chat: sanitizeOption(parsed.selections?.chat),
          image: sanitizeOption(parsed.selections?.image),
          video: sanitizeOption(parsed.selections?.video)
        },
        systemPrompt: typeof parsed.systemPrompt === 'string' ? parsed.systemPrompt : '',
        temperature: typeof parsed.temperature === 'number' ? parsed.temperature : DEFAULTS.temperature,
        maxTokens: typeof parsed.maxTokens === 'number' ? parsed.maxTokens : DEFAULTS.maxTokens
      }
    }
  } catch {
    // Fall through to the v1 migration.
  }

  try {
    if (rawV1) {
      const legacy = JSON.parse(rawV1) as Record<string, unknown>
      const model = typeof legacy.model === 'string' ? legacy.model : ''
      return {
        version: 2,
        keyId: typeof legacy.keyId === 'number' ? legacy.keyId : null,
        selections: {
          chat: legacyOption(model, 'chat'),
          image: legacyOption(model, 'image'),
          video: legacyOption(model, 'video')
        },
        systemPrompt: typeof legacy.systemPrompt === 'string' ? legacy.systemPrompt : '',
        temperature: typeof legacy.temperature === 'number' ? legacy.temperature : DEFAULTS.temperature,
        maxTokens: typeof legacy.maxTokens === 'number' ? legacy.maxTokens : DEFAULTS.maxTokens
      }
    }
  } catch {
    // Use defaults.
  }
  return { ...DEFAULTS, selections: { ...EMPTY_SELECTIONS } }
}

let store: ReturnType<typeof build> | null = null

function build() {
  const initial = migratePlaygroundSettings(
    localStorage.getItem(PLAYGROUND_SETTINGS_STORAGE_KEY),
    localStorage.getItem(LEGACY_STORAGE_KEY)
  )

  const keyId = ref<number | null>(initial.keyId)
  const chatOption = ref<PlaygroundModelOption | null>(initial.selections.chat)
  const imageOption = ref<PlaygroundModelOption | null>(initial.selections.image)
  const videoOption = ref<PlaygroundModelOption | null>(initial.selections.video)
  const systemPrompt = ref(initial.systemPrompt)
  const temperature = ref(initial.temperature)
  const maxTokens = ref(initial.maxTokens)

  const persist = () => {
    try {
      localStorage.setItem(
        PLAYGROUND_SETTINGS_STORAGE_KEY,
        JSON.stringify({
          version: 2,
          keyId: keyId.value,
          selections: {
            chat: sanitizeOption(chatOption.value),
            image: sanitizeOption(imageOption.value),
            video: sanitizeOption(videoOption.value)
          },
          systemPrompt: systemPrompt.value,
          temperature: temperature.value,
          maxTokens: maxTokens.value
        } satisfies PersistedSettingsV2)
      )
    } catch {
      // Storage may be unavailable or full.
    }
  }

  watch(
    [keyId, chatOption, imageOption, videoOption, systemPrompt, temperature, maxTokens],
    persist,
    { deep: true }
  )
  persist()

  return { keyId, chatOption, imageOption, videoOption, systemPrompt, temperature, maxTokens }
}

export function usePlaygroundSettings() {
  if (!store) store = build()
  return store
}

export function resetPlaygroundSettingsForTest(): void {
  store = null
}

import { ref, watch } from 'vue'
import type { PlaygroundCapability, PlaygroundFeatures, PlaygroundModelOption } from '@/types/playground'

export const PLAYGROUND_SETTINGS_STORAGE_KEY = 'playground_settings_v3'
export const PLAYGROUND_SETTINGS_V2_STORAGE_KEY = 'playground_settings_v2'
export const PLAYGROUND_SETTINGS_V1_STORAGE_KEY = 'playground_settings_v1'

export type PlaygroundReasoningEffort = '' | 'none' | 'minimal' | 'low' | 'medium' | 'high' | 'xhigh'

export interface PersistedPlaygroundSettingsV3 {
  version: 3
  keyId: number | null
  selections: Record<PlaygroundCapability, PlaygroundModelOption | null>
  systemPrompt: string
  temperature: number
  maxTokens: number
  topP: number
  reasoningEffort: PlaygroundReasoningEffort
  webSearch: boolean
  codeExecution: boolean
  webFetch: boolean
}

const EMPTY_SELECTIONS: Record<PlaygroundCapability, PlaygroundModelOption | null> = {
  chat: null,
  image: null,
  video: null
}

export const PLAYGROUND_SETTINGS_DEFAULTS: Readonly<PersistedPlaygroundSettingsV3> = {
  version: 3,
  keyId: null,
  selections: { ...EMPTY_SELECTIONS },
  systemPrompt: '',
  temperature: 0.7,
  maxTokens: 0,
  topP: 1,
  reasoningEffort: '',
  webSearch: false,
  codeExecution: false,
  webFetch: false
}

function finiteNumber(value: unknown, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback
}

function clamp(value: unknown, min: number, max: number, fallback: number): number {
  return Math.min(max, Math.max(min, finiteNumber(value, fallback)))
}

function sanitizeMaxTokens(value: unknown): number {
  return Math.round(clamp(value, 0, 1_000_000, PLAYGROUND_SETTINGS_DEFAULTS.maxTokens))
}

function sanitizeReasoningEffort(value: unknown): PlaygroundReasoningEffort {
  return value === 'none' || value === 'minimal' || value === 'low' || value === 'medium' || value === 'high' || value === 'xhigh'
    ? value
    : ''
}

function sanitizeFeatures(value: unknown): PlaygroundFeatures | undefined {
  if (Array.isArray(value)) {
    const features = Array.from(new Set(value.filter((item): item is string => typeof item === 'string' && !!item.trim())))
    return features.length ? features : undefined
  }
  if (value && typeof value === 'object') {
    const features: Record<string, boolean> = {}
    for (const [key, enabled] of Object.entries(value as Record<string, unknown>)) {
      if (key.trim() && typeof enabled === 'boolean') features[key] = enabled
    }
    return Object.keys(features).length ? features : undefined
  }
  return undefined
}

export function sanitizePlaygroundOption(value: unknown): PlaygroundModelOption | null {
  if (!value || typeof value !== 'object') return null
  const option = value as Partial<PlaygroundModelOption>
  if (typeof option.model !== 'string' || !option.model.trim()) return null
  return {
    id: typeof option.id === 'string' && option.id ? option.id : undefined,
    group_id: typeof option.group_id === 'number' && Number.isFinite(option.group_id) ? option.group_id : 0,
    group_name: typeof option.group_name === 'string' ? option.group_name : '',
    group_priority: typeof option.group_priority === 'number' && Number.isFinite(option.group_priority) ? option.group_priority : 0,
    model: option.model.trim(),
    platform: typeof option.platform === 'string' ? option.platform : '',
    capabilities: Array.isArray(option.capabilities)
      ? Array.from(new Set(option.capabilities.filter((cap): cap is PlaygroundCapability => cap === 'chat' || cap === 'image' || cap === 'video')))
      : [],
    features: sanitizeFeatures(option.features)
  }
}

function legacyOption(model: string, capability: PlaygroundCapability): PlaygroundModelOption | null {
  if (!model.trim()) return null
  return {
    group_id: 0,
    group_name: '',
    group_priority: 0,
    model: model.trim(),
    platform: '',
    capabilities: [capability]
  }
}

function defaults(): PersistedPlaygroundSettingsV3 {
  return { ...PLAYGROUND_SETTINGS_DEFAULTS, selections: { ...EMPTY_SELECTIONS } }
}

function sanitizeCommon(parsed: Record<string, unknown>, selections: PersistedPlaygroundSettingsV3['selections']): PersistedPlaygroundSettingsV3 {
  return {
    version: 3,
    keyId: typeof parsed.keyId === 'number' && Number.isInteger(parsed.keyId) && parsed.keyId > 0 ? parsed.keyId : null,
    selections,
    systemPrompt: typeof parsed.systemPrompt === 'string' ? parsed.systemPrompt : '',
    temperature: clamp(parsed.temperature, 0, 2, PLAYGROUND_SETTINGS_DEFAULTS.temperature),
    maxTokens: sanitizeMaxTokens(parsed.maxTokens),
    topP: clamp(parsed.topP, 0, 1, PLAYGROUND_SETTINGS_DEFAULTS.topP),
    reasoningEffort: sanitizeReasoningEffort(parsed.reasoningEffort),
    webSearch: parsed.webSearch === true,
    codeExecution: parsed.codeExecution === true,
    webFetch: parsed.webFetch === true
  }
}

function parseSettings(raw: string | null, legacyV1 = false): PersistedPlaygroundSettingsV3 | null {
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return null
    const record = parsed as Record<string, unknown>
    if (legacyV1) {
      const model = typeof record.model === 'string' ? record.model : ''
      return sanitizeCommon(record, {
        chat: legacyOption(model, 'chat'),
        image: legacyOption(model, 'image'),
        video: legacyOption(model, 'video')
      })
    }
    const selections = record.selections && typeof record.selections === 'object'
      ? record.selections as Record<string, unknown>
      : {}
    return sanitizeCommon(record, {
      chat: sanitizePlaygroundOption(selections.chat),
      image: sanitizePlaygroundOption(selections.image),
      video: sanitizePlaygroundOption(selections.video)
    })
  } catch {
    return null
  }
}

export function migratePlaygroundSettings(
  rawV3: string | null,
  rawV2: string | null = null,
  rawV1: string | null = null
): PersistedPlaygroundSettingsV3 {
  return parseSettings(rawV3) ?? parseSettings(rawV2) ?? parseSettings(rawV1, true) ?? defaults()
}

function safeGetItem(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

let store: ReturnType<typeof build> | null = null

function build() {
  const initial = migratePlaygroundSettings(
    safeGetItem(PLAYGROUND_SETTINGS_STORAGE_KEY),
    safeGetItem(PLAYGROUND_SETTINGS_V2_STORAGE_KEY),
    safeGetItem(PLAYGROUND_SETTINGS_V1_STORAGE_KEY)
  )

  const keyId = ref<number | null>(initial.keyId)
  const chatOption = ref<PlaygroundModelOption | null>(initial.selections.chat)
  const imageOption = ref<PlaygroundModelOption | null>(initial.selections.image)
  const videoOption = ref<PlaygroundModelOption | null>(initial.selections.video)
  const systemPrompt = ref(initial.systemPrompt)
  const temperature = ref(initial.temperature)
  const maxTokens = ref(initial.maxTokens)
  const topP = ref(initial.topP)
  const reasoningEffort = ref<PlaygroundReasoningEffort>(initial.reasoningEffort)
  const webSearch = ref(initial.webSearch)
  const codeExecution = ref(initial.codeExecution)
  const webFetch = ref(initial.webFetch)

  const persist = () => {
    const sanitized = sanitizeCommon({
      keyId: keyId.value,
      systemPrompt: systemPrompt.value,
      temperature: temperature.value,
      maxTokens: maxTokens.value,
      topP: topP.value,
      reasoningEffort: reasoningEffort.value,
      webSearch: webSearch.value,
      codeExecution: codeExecution.value,
      webFetch: webFetch.value
    }, {
      chat: sanitizePlaygroundOption(chatOption.value),
      image: sanitizePlaygroundOption(imageOption.value),
      video: sanitizePlaygroundOption(videoOption.value)
    })
    try {
      localStorage.setItem(PLAYGROUND_SETTINGS_STORAGE_KEY, JSON.stringify(sanitized))
    } catch {
      // Storage may be unavailable or full.
    }
  }

  const restoreDefaults = () => {
    keyId.value = PLAYGROUND_SETTINGS_DEFAULTS.keyId
    chatOption.value = null
    imageOption.value = null
    videoOption.value = null
    systemPrompt.value = PLAYGROUND_SETTINGS_DEFAULTS.systemPrompt
    temperature.value = PLAYGROUND_SETTINGS_DEFAULTS.temperature
    maxTokens.value = PLAYGROUND_SETTINGS_DEFAULTS.maxTokens
    topP.value = PLAYGROUND_SETTINGS_DEFAULTS.topP
    reasoningEffort.value = PLAYGROUND_SETTINGS_DEFAULTS.reasoningEffort
    webSearch.value = PLAYGROUND_SETTINGS_DEFAULTS.webSearch
    codeExecution.value = PLAYGROUND_SETTINGS_DEFAULTS.codeExecution
    webFetch.value = PLAYGROUND_SETTINGS_DEFAULTS.webFetch
    persist()
  }

  watch(
    [keyId, chatOption, imageOption, videoOption, systemPrompt, temperature, maxTokens, topP, reasoningEffort, webSearch, codeExecution, webFetch],
    persist,
    { deep: true }
  )
  persist()

  return {
    keyId, chatOption, imageOption, videoOption, systemPrompt, temperature, maxTokens,
    topP, reasoningEffort, webSearch, codeExecution, webFetch, restoreDefaults
  }
}

export function usePlaygroundSettings() {
  if (!store) store = build()
  return store
}

export function resetPlaygroundSettingsForTest(): void {
  store = null
}

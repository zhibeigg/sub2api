/**
 * Shared, persisted playground toolbar settings (selected key id, last model,
 * system prompt, temperature, max tokens). API key *values* are never stored —
 * only the key id, so the value is re-resolved from the live key list.
 */

import { ref, watch } from 'vue'

const STORAGE_KEY = 'playground_settings_v1'

interface PersistedSettings {
  keyId: number | null
  model: string
  systemPrompt: string
  temperature: number
  maxTokens: number
}

const DEFAULTS: PersistedSettings = {
  keyId: null,
  model: '',
  systemPrompt: '',
  temperature: 0.7,
  maxTokens: 0
}

let store: ReturnType<typeof build> | null = null

function build() {
  let initial = { ...DEFAULTS }
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) initial = { ...DEFAULTS, ...JSON.parse(raw) }
  } catch {
    // ignore
  }

  const keyId = ref<number | null>(initial.keyId)
  const model = ref<string>(initial.model)
  const systemPrompt = ref<string>(initial.systemPrompt)
  const temperature = ref<number>(initial.temperature)
  const maxTokens = ref<number>(initial.maxTokens)

  watch([keyId, model, systemPrompt, temperature, maxTokens], () => {
    try {
      localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({
          keyId: keyId.value,
          model: model.value,
          systemPrompt: systemPrompt.value,
          temperature: temperature.value,
          maxTokens: maxTokens.value
        })
      )
    } catch {
      // ignore
    }
  })

  return { keyId, model, systemPrompt, temperature, maxTokens }
}

export function usePlaygroundSettings() {
  if (!store) store = build()
  return store
}

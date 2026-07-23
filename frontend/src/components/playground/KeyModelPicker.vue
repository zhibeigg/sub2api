<template>
  <div :class="containerClass">
    <div class="min-w-0 flex-1">
      <label class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
        {{ t('playground.apiKey') }}
      </label>
      <select
        :value="keyId ?? ''"
        class="input"
        @change="onKeyChange(($event.target as HTMLSelectElement).value)"
      >
        <option value="" disabled>{{ t('playground.selectKey') }}</option>
        <option v-for="key in keys" :key="key.id" :value="key.id">{{ key.name }}</option>
      </select>
    </div>

    <div class="min-w-0 flex-1">
      <label class="mb-1 flex items-center gap-1.5 text-xs font-medium text-gray-500 dark:text-gray-400">
        {{ t('playground.model') }}
        <Icon v-if="optionsLoading" name="refresh" size="xs" class="animate-spin" />
      </label>
      <select
        :value="option ? playgroundOptionKey(option) : ''"
        class="input"
        :disabled="optionsLoading || compatibleOptions.length === 0"
        @change="onOptionChange(($event.target as HTMLSelectElement).value)"
      >
        <option value="" disabled>{{ modelPlaceholder }}</option>
        <option v-for="item in compatibleOptions" :key="playgroundOptionKey(item)" :value="playgroundOptionKey(item)">
          {{ item.model }}
        </option>
      </select>
      <div v-if="optionsError" class="mt-1.5 flex items-center gap-2 text-xs text-red-600 dark:text-red-400">
        <span>{{ t('playground.modelListLoadFailed') }}</span>
        <button type="button" class="font-medium underline underline-offset-2" @click="refreshOptions">
          {{ t('playground.retry') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import keysAPI from '@/api/keys'
import playgroundAPI from '@/api/playground'
import type { ApiKey } from '@/types'
import {
  playgroundOptionKey,
  samePlaygroundOption,
  type PlaygroundCapability,
  type PlaygroundModelOption
} from '@/types/playground'

const props = withDefaults(defineProps<{
  keyId: number | null
  option: PlaygroundModelOption | null
  capability: PlaygroundCapability
  resolvedKey?: string
  layout?: 'responsive' | 'stacked'
}>(), {
  resolvedKey: '',
  layout: 'responsive'
})

const emit = defineEmits<{
  (event: 'update:keyId', value: number | null): void
  (event: 'update:option', value: PlaygroundModelOption | null): void
  (event: 'update:resolvedKey', value: string): void
  (event: 'resolved-key', value: string): void
}>()

const { t } = useI18n()
const keys = ref<ApiKey[]>([])
const options = ref<PlaygroundModelOption[]>([])
const optionsLoading = ref(false)
const optionsLoaded = ref(false)
const optionsError = ref(false)
const OPTION_CACHE_TTL_MS = 30_000
const optionCache = new Map<number, { options: PlaygroundModelOption[]; expiresAt: number }>()
let requestVersion = 0
let optionsController: AbortController | null = null

const containerClass = computed(() => props.layout === 'stacked'
  ? 'flex flex-col gap-3'
  : 'flex flex-col gap-3 sm:flex-row sm:items-end')
const compatibleOptions = computed(() => {
  const seen = new Set<string>()
  return options.value.filter((item) => {
    if (!item.capabilities.includes(props.capability)) return false
    const modelKey = item.model.trim().toLowerCase()
    if (!modelKey || seen.has(modelKey)) return false
    seen.add(modelKey)
    return true
  })
})
const modelPlaceholder = computed(() => {
  if (optionsLoading.value) return t('playground.loadingModels')
  if (optionsError.value) return t('playground.modelListLoadFailed')
  if (!optionsLoaded.value) return t('playground.selectModel')
  if (options.value.length === 0) return t('playground.noConfiguredModels')
  if (compatibleOptions.value.length === 0) return t('playground.noModels')
  return t('playground.selectModel')
})

function resolvedKeyValue(keyId = props.keyId): string {
  return keys.value.find((key) => key.id === keyId)?.key ?? ''
}

function syncResolvedKey(): void {
  const value = resolvedKeyValue()
  if (value !== props.resolvedKey) emit('update:resolvedKey', value)
  emit('resolved-key', value)
}

function onKeyChange(raw: string): void {
  emit('update:keyId', raw ? Number(raw) : null)
}

function onOptionChange(value: string): void {
  emit(
    'update:option',
    compatibleOptions.value.find((item) => playgroundOptionKey(item) === value) ?? null
  )
}

async function loadKeys(): Promise<void> {
  try {
    const pageSize = 100
    const loaded: ApiKey[] = []
    let page = 1
    let pages = 1
    do {
      const response = await keysAPI.list(page, pageSize, { status: 'active' })
      loaded.push(...(response.items ?? []))
      pages = Math.max(1, response.pages || 1)
      page += 1
    } while (page <= pages)
    keys.value = loaded
    if (props.keyId == null && keys.value.length > 0) emit('update:keyId', keys.value[0].id)
    syncResolvedKey()
  } catch {
    keys.value = []
    syncResolvedKey()
  }
}

function ensureOptionSelected(): void {
  const current = props.option
  const exact = compatibleOptions.value.find((item) => samePlaygroundOption(item, current))
  if (exact) return
  const migrated = current?.group_id === 0
    ? compatibleOptions.value.find((item) => item.model === current.model)
    : undefined
  emit('update:option', migrated ?? compatibleOptions.value[0] ?? null)
}

async function loadOptions(force = false): Promise<void> {
  const keyId = props.keyId
  syncResolvedKey()
  optionsController?.abort()
  const version = ++requestVersion

  if (!keyId) {
    options.value = []
    optionsLoaded.value = false
    optionsError.value = false
    emit('update:option', null)
    return
  }

  const cached = optionCache.get(keyId)
  if (!force && cached && cached.expiresAt > Date.now() && cached.options.length > 0) {
    options.value = cached.options
    optionsLoaded.value = true
    optionsError.value = false
    ensureOptionSelected()
    return
  }
  if (cached) optionCache.delete(keyId)

  // Prevent the previous key's group/model route from remaining sendable while
  // the newly selected key's capability catalog is still loading.
  options.value = []
  optionsLoaded.value = false
  optionsError.value = false
  emit('update:option', null)

  const controller = new AbortController()
  optionsController = controller
  optionsLoading.value = true
  try {
    const list = await playgroundAPI.listModelOptions(keyId, controller.signal)
    if (version !== requestVersion || keyId !== props.keyId) return
    if (list.length > 0) {
      optionCache.set(keyId, { options: list, expiresAt: Date.now() + OPTION_CACHE_TTL_MS })
    }
    options.value = list
    optionsLoaded.value = true
    optionsError.value = false
    ensureOptionSelected()
  } catch (error) {
    if ((error as Error).name !== 'CanceledError' && (error as Error).name !== 'AbortError' && version === requestVersion) {
      options.value = []
      optionsLoaded.value = false
      optionsError.value = true
      emit('update:option', null)
    }
  } finally {
    if (version === requestVersion) optionsLoading.value = false
  }
}

function refreshOptions(): void {
  if (props.keyId) optionCache.delete(props.keyId)
  void loadOptions(true)
}

function onWindowFocus(): void {
  if (!props.keyId || optionsLoading.value) return
  const cached = optionCache.get(props.keyId)
  if (optionsError.value || options.value.length === 0 || !cached || cached.expiresAt <= Date.now()) {
    void loadOptions(true)
  }
}

watch(() => props.keyId, () => {
  syncResolvedKey()
  void loadOptions()
})
watch(() => props.capability, () => {
  syncResolvedKey()
  ensureOptionSelected()
})

onMounted(async () => {
  window.addEventListener('focus', onWindowFocus)
  await loadKeys()
  await nextTick()
  if (requestVersion === 0) await loadOptions()
})

onBeforeUnmount(() => {
  window.removeEventListener('focus', onWindowFocus)
  optionsController?.abort()
})

defineExpose({ refreshOptions })
</script>

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
        <option value="" disabled>
          {{ compatibleOptions.length === 0 ? t('playground.noModels') : t('playground.selectModel') }}
        </option>
        <option v-for="item in compatibleOptions" :key="playgroundOptionKey(item)" :value="playgroundOptionKey(item)">
          {{ item.model }} · {{ item.group_name || t('playground.groupFallback', { id: item.group_id }) }} · {{ platformLabel(item.platform) }}
        </option>
      </select>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import keysAPI from '@/api/keys'
import playgroundAPI from '@/api/playground'
import { platformLabel } from '@/utils/platformColors'
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
const optionCache = new Map<number, PlaygroundModelOption[]>()
let requestVersion = 0
let optionsController: AbortController | null = null

const containerClass = computed(() => props.layout === 'stacked'
  ? 'flex flex-col gap-3'
  : 'flex flex-col gap-3 sm:flex-row sm:items-end')
const compatibleOptions = computed(() =>
  options.value.filter((item) => item.capabilities.includes(props.capability))
)

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

async function loadOptions(): Promise<void> {
  const keyId = props.keyId
  const apiKey = resolvedKeyValue(keyId)
  syncResolvedKey()
  optionsController?.abort()
  const version = ++requestVersion

  if (!keyId || !apiKey) {
    options.value = []
    emit('update:option', null)
    return
  }

  const cached = optionCache.get(keyId)
  if (cached) {
    options.value = cached
    ensureOptionSelected()
    return
  }

  // Prevent the previous key's group/model route from remaining sendable while
  // the newly selected key's capability catalog is still loading.
  options.value = []
  emit('update:option', null)

  const controller = new AbortController()
  optionsController = controller
  optionsLoading.value = true
  try {
    const list = await playgroundAPI.listModelOptions(keyId, controller.signal)
    if (version !== requestVersion || keyId !== props.keyId) return
    optionCache.set(keyId, list)
    options.value = list
    ensureOptionSelected()
  } catch (error) {
    if ((error as Error).name !== 'CanceledError' && (error as Error).name !== 'AbortError' && version === requestVersion) {
      options.value = []
      emit('update:option', null)
    }
  } finally {
    if (version === requestVersion) optionsLoading.value = false
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
  await loadKeys()
  await nextTick()
  if (requestVersion === 0) await loadOptions()
})

onBeforeUnmount(() => optionsController?.abort())
</script>

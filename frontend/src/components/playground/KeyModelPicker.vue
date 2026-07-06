<template>
  <div class="flex flex-col gap-3 sm:flex-row sm:items-end">
    <!-- API Key select -->
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
        <option v-for="k in keys" :key="k.id" :value="k.id">
          {{ k.name }} · {{ platformLabel(k.group?.platform || '') }}
        </option>
      </select>
    </div>

    <!-- Model select -->
    <div class="min-w-0 flex-1">
      <label class="mb-1 flex items-center gap-1.5 text-xs font-medium text-gray-500 dark:text-gray-400">
        {{ t('playground.model') }}
        <Icon v-if="modelsLoading" name="refresh" size="xs" class="animate-spin" />
      </label>
      <select
        :value="model"
        class="input"
        :disabled="modelsLoading || models.length === 0"
        @change="emit('update:model', ($event.target as HTMLSelectElement).value)"
      >
        <option value="" disabled>
          {{ models.length === 0 ? t('playground.noModels') : t('playground.selectModel') }}
        </option>
        <option v-for="m in models" :key="m.id" :value="m.id">{{ m.id }}</option>
      </select>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import keysAPI from '@/api/keys'
import playgroundAPI, { type PlaygroundModel } from '@/api/playground'
import { platformLabel } from '@/utils/platformColors'
import type { ApiKey } from '@/types'

const props = defineProps<{
  keyId: number | null
  model: string
}>()

const emit = defineEmits<{
  (e: 'update:keyId', v: number | null): void
  (e: 'update:model', v: string): void
  (e: 'resolved-key', v: string): void
}>()

const { t } = useI18n()

const keys = ref<ApiKey[]>([])
const models = ref<PlaygroundModel[]>([])
const modelsLoading = ref(false)

// Per-key model cache to avoid refetching when switching back and forth.
const modelCache = new Map<number, PlaygroundModel[]>()

function resolvedKeyValue(): string {
  const k = keys.value.find((x) => x.id === props.keyId)
  return k?.key ?? ''
}

function onKeyChange(raw: string): void {
  const id = raw ? Number(raw) : null
  emit('update:keyId', id)
}

async function loadKeys(): Promise<void> {
  try {
    const res = await keysAPI.list(1, 100, { status: 'active' })
    keys.value = res.items ?? []
    // If no key selected yet, auto-pick the first active one.
    if (props.keyId == null && keys.value.length > 0) {
      emit('update:keyId', keys.value[0].id)
    }
  } catch {
    keys.value = []
  }
}

async function loadModels(): Promise<void> {
  const apiKey = resolvedKeyValue()
  if (!apiKey || props.keyId == null) {
    models.value = []
    return
  }
  emit('resolved-key', apiKey)

  const cached = modelCache.get(props.keyId)
  if (cached) {
    models.value = cached
    ensureModelSelected()
    return
  }

  modelsLoading.value = true
  try {
    const list = await playgroundAPI.listModels(apiKey)
    models.value = list
    modelCache.set(props.keyId, list)
    ensureModelSelected()
  } catch {
    models.value = []
  } finally {
    modelsLoading.value = false
  }
}

function ensureModelSelected(): void {
  if (models.value.length === 0) return
  const exists = models.value.some((m) => m.id === props.model)
  if (!exists) emit('update:model', models.value[0].id)
}

watch(
  () => props.keyId,
  () => loadModels()
)

onMounted(async () => {
  await loadKeys()
  await loadModels()
})
</script>

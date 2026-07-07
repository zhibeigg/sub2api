<template>
  <div class="space-y-3">
    <!-- Provider filter chips -->
    <div class="flex flex-wrap gap-2">
      <button
        v-for="opt in providerOptions"
        :key="`prov-${opt.value}`"
        type="button"
        class="rounded-full border px-3 py-1 text-xs font-medium transition-colors"
        :class="
          activeProvider === opt.value
            ? 'border-primary-500 bg-primary-500/10 text-primary-600 dark:text-primary-400'
            : 'border-gray-200 text-gray-600 hover:border-gray-300 dark:border-dark-600 dark:text-gray-400'
        "
        @click="activeProvider = opt.value"
      >
        <span class="inline-flex items-center gap-1">
          <PlatformIcon v-if="opt.value !== 'all'" :platform="opt.value as GroupPlatform" size="xs" />
          {{ opt.label }}
          <span class="opacity-60">{{ opt.count }}</span>
        </span>
      </button>
    </div>

    <!-- Add-group dropdown -->
    <select class="input" :value="''" @change="onAddGroup(($event.target as HTMLSelectElement).value)">
      <option value="" disabled>{{ t('keys.selectGroup') }}</option>
      <option v-for="g in addableGroups" :key="g.id" :value="g.id">
        {{ g.name }} · {{ formatMultiplier(rateFor(g)) }}
      </option>
    </select>

    <!-- Selected groups (draggable, priority ordered) -->
    <VueDraggable
      v-if="selected.length"
      v-model="selected"
      :animation="180"
      handle=".pk-drag"
      class="space-y-2"
      @end="emitChange"
    >
      <div
        v-for="(sel, idx) in selected"
        :key="sel.group_id"
        class="flex items-center gap-2 rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 dark:border-dark-600 dark:bg-dark-800"
      >
        <span class="pk-drag flex cursor-grab items-center text-gray-300 hover:text-gray-500 active:cursor-grabbing dark:text-dark-500">
          <svg class="h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
            <path d="M7 2a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM13 2a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM7 8a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM13 8a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM7 14a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM13 14a2 2 0 1 0 0 4 2 2 0 0 0 0-4z" />
          </svg>
        </span>
        <span class="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-primary-500/10 text-xs font-semibold text-primary-600 dark:text-primary-400">
          {{ idx + 1 }}
        </span>
        <PlatformIcon :platform="platformFor(sel.group_id) as GroupPlatform" size="xs" />
        <span class="min-w-0 flex-1 truncate text-sm font-medium text-gray-800 dark:text-gray-100">
          {{ nameFor(sel.group_id) }}
        </span>
        <span class="flex-shrink-0 rounded bg-green-500/10 px-1.5 py-0.5 text-xs font-medium text-green-600 dark:text-green-400">
          {{ formatMultiplier(rateForId(sel.group_id)) }}
        </span>
        <button
          type="button"
          class="flex-shrink-0 text-gray-400 hover:text-red-500"
          :title="t('common.delete')"
          @click="removeGroup(sel.group_id)"
        >
          <Icon name="x" size="sm" />
        </button>
      </div>
    </VueDraggable>

    <!-- Per-group available models -->
    <div v-if="selected.length" class="space-y-3">
      <div
        v-for="sel in selected"
        :key="`models-${sel.group_id}`"
        class="rounded-lg border border-gray-100 bg-gray-50/50 p-3 dark:border-dark-700 dark:bg-dark-800/40"
      >
        <div class="mb-2 flex items-center gap-1.5 text-xs font-medium text-gray-600 dark:text-gray-300">
          <PlatformIcon :platform="platformFor(sel.group_id) as GroupPlatform" size="xs" />
          {{ nameFor(sel.group_id) }}
          <span class="text-gray-400">·</span>
          <span class="text-gray-400">
            {{ t('keys.groupModels', { count: modelsFor(sel.group_id).length }) }}
          </span>
        </div>
        <div v-if="modelsFor(sel.group_id).length" class="flex flex-wrap gap-1.5">
          <span
            v-for="m in modelsFor(sel.group_id)"
            :key="`${sel.group_id}-${m}`"
            class="inline-flex items-center gap-1 rounded-md bg-white px-2 py-0.5 text-[11px] text-gray-600 dark:bg-dark-700 dark:text-gray-300"
          >
            <ModelIcon :model="m" size="14px" />
            {{ m }}
          </span>
        </div>
        <p v-else class="text-[11px] text-gray-400">{{ t('keys.groupNoModels') }}</p>
      </div>
    </div>

    <p class="text-xs text-gray-400">{{ t('keys.multiGroupHint') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { VueDraggable } from 'vue-draggable-plus'
import Icon from '@/components/icons/Icon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import userChannelsAPI, { type UserAvailableChannel } from '@/api/channels'
import { platformLabel } from '@/utils/platformColors'
import type { Group, GroupPlatform, ApiKeyGroupBindingInput } from '@/types'

const props = defineProps<{
  groups: Group[]
  userGroupRates: Record<number, number>
  modelValue: ApiKeyGroupBindingInput[]
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: ApiKeyGroupBindingInput[]): void
}>()

const { t } = useI18n()

const activeProvider = ref<string>('all')
const selected = ref<ApiKeyGroupBindingInput[]>([])
// group_id -> [model names]
const modelsByGroup = ref<Record<number, string[]>>({})

// Sync incoming modelValue → local ordered list.
watch(
  () => props.modelValue,
  (v) => {
    const next = [...(v || [])].sort((a, b) => a.priority - b.priority)
    // avoid feedback loop when identical
    if (JSON.stringify(next.map((x) => x.group_id)) !== JSON.stringify(selected.value.map((x) => x.group_id))) {
      selected.value = next
    }
  },
  { immediate: true }
)

const groupById = computed<Record<number, Group>>(() => {
  const m: Record<number, Group> = {}
  for (const g of props.groups) m[g.id] = g
  return m
})

const providerOptions = computed(() => {
  const counts = new Map<string, number>()
  for (const g of props.groups) counts.set(g.platform, (counts.get(g.platform) ?? 0) + 1)
  const opts = [{ value: 'all', label: t('modelSquare.filters.all'), count: props.groups.length }]
  for (const [value, count] of Array.from(counts.entries()).sort((a, b) => b[1] - a[1])) {
    opts.push({ value, label: platformLabel(value), count })
  }
  return opts
})

const selectedIds = computed(() => new Set(selected.value.map((s) => s.group_id)))

const addableGroups = computed(() =>
  props.groups.filter(
    (g) => !selectedIds.value.has(g.id) && (activeProvider.value === 'all' || g.platform === activeProvider.value)
  )
)

function rateFor(g: Group): number {
  return props.userGroupRates[g.id] ?? g.rate_multiplier
}
function rateForId(id: number): number {
  const g = groupById.value[id]
  return g ? rateFor(g) : 1
}
function nameFor(id: number): string {
  return groupById.value[id]?.name ?? `#${id}`
}
function platformFor(id: number): string {
  return groupById.value[id]?.platform ?? ''
}
function modelsFor(id: number): string[] {
  return modelsByGroup.value[id] ?? []
}
function formatMultiplier(rate: number): string {
  return `${Number(rate.toFixed(3)).toString()}x`
}

function reindex(): void {
  selected.value = selected.value.map((s, i) => ({ group_id: s.group_id, priority: i }))
}

function emitChange(): void {
  reindex()
  emit('update:modelValue', selected.value.map((s) => ({ ...s })))
}

function onAddGroup(raw: string): void {
  const id = Number(raw)
  if (!id || selectedIds.value.has(id)) return
  selected.value.push({ group_id: id, priority: selected.value.length })
  emitChange()
}

function removeGroup(id: number): void {
  selected.value = selected.value.filter((s) => s.group_id !== id)
  emitChange()
}

// Load per-group available models from /channels/available (aggregated).
async function loadModels(): Promise<void> {
  try {
    const channels: UserAvailableChannel[] = await userChannelsAPI.getAvailable()
    const map: Record<number, Set<string>> = {}
    for (const ch of channels) {
      for (const section of ch.platforms) {
        for (const g of section.groups) {
          if (!map[g.id]) map[g.id] = new Set()
          for (const m of section.supported_models) map[g.id].add(m.name)
        }
      }
    }
    const out: Record<number, string[]> = {}
    for (const [gid, set] of Object.entries(map)) {
      out[Number(gid)] = Array.from(set).sort((a, b) => a.localeCompare(b))
    }
    modelsByGroup.value = out
  } catch {
    modelsByGroup.value = {}
  }
}

onMounted(loadModels)
</script>

<template>
  <div class="space-y-3" data-test="sortable-group-binding-picker">
    <div class="relative">
      <Icon
        name="search"
        size="sm"
        class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
      />
      <input
        ref="searchInputRef"
        v-model="searchQuery"
        type="search"
        class="input pl-9"
        :placeholder="t('keys.searchGroup')"
        data-test="group-binding-search"
      />
    </div>

    <div v-if="selected.length" class="space-y-2">
      <div class="flex items-center justify-between gap-3">
        <span class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
          {{ t('keys.selectedGroups', { count: selected.length }) }}
        </span>
        <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('keys.dragToPrioritize') }}</span>
      </div>
      <VueDraggable
        v-model="selected"
        :animation="180"
        handle=".group-binding-drag-handle"
        class="space-y-2"
        data-test="selected-group-bindings"
        @end="emitChange"
      >
        <div
          v-for="(binding, index) in selected"
          :key="binding.group_id"
          :class="[
            'flex min-w-0 items-center gap-2 rounded-lg border px-2.5 py-2',
            platformBadgeClass(groupFor(binding.group_id)?.platform ?? '')
          ]"
          :data-test="`selected-group-${binding.group_id}`"
        >
          <button
            type="button"
            class="group-binding-drag-handle flex h-7 w-7 flex-shrink-0 cursor-grab items-center justify-center rounded-md text-gray-400 hover:bg-white hover:text-gray-600 focus:outline-none focus:ring-2 focus:ring-primary-500 active:cursor-grabbing dark:hover:bg-dark-600 dark:hover:text-gray-200"
            :aria-label="t('keys.dragGroup', { name: groupFor(binding.group_id)?.name ?? binding.group_id })"
          >
            <Icon name="arrowsUpDown" size="sm" />
          </button>
          <span class="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-primary-100 text-xs font-semibold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">
            {{ index + 1 }}
          </span>
          <PlatformIcon
            v-if="groupFor(binding.group_id)"
            :platform="groupFor(binding.group_id)!.platform"
            size="xs"
          />
          <div class="min-w-0 flex-1">
            <div class="truncate text-sm font-semibold">
              {{ groupFor(binding.group_id)?.name ?? `#${binding.group_id}` }}
            </div>
            <div class="truncate text-[11px] opacity-70">
              {{ platformName(groupFor(binding.group_id)?.platform) }}
            </div>
          </div>
          <span class="flex-shrink-0 rounded-md bg-white/70 px-1.5 py-0.5 text-xs font-semibold tabular-nums shadow-sm dark:bg-black/20">
            {{ formatMultiplier(rateForId(binding.group_id)) }}
          </span>
          <div class="flex flex-shrink-0 items-center">
            <button
              type="button"
              class="flex h-7 w-6 items-center justify-center rounded text-gray-400 hover:bg-white hover:text-gray-700 focus:outline-none focus:ring-2 focus:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-30 dark:hover:bg-dark-600 dark:hover:text-gray-200"
              :disabled="index === 0"
              :aria-label="t('keys.moveGroupUp', { name: groupFor(binding.group_id)?.name ?? binding.group_id })"
              :data-test="`move-up-group-${binding.group_id}`"
              @click="moveGroup(binding.group_id, -1)"
            >
              <Icon name="chevronUp" size="xs" />
            </button>
            <button
              type="button"
              class="flex h-7 w-6 items-center justify-center rounded text-gray-400 hover:bg-white hover:text-gray-700 focus:outline-none focus:ring-2 focus:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-30 dark:hover:bg-dark-600 dark:hover:text-gray-200"
              :disabled="index === selected.length - 1"
              :aria-label="t('keys.moveGroupDown', { name: groupFor(binding.group_id)?.name ?? binding.group_id })"
              :data-test="`move-down-group-${binding.group_id}`"
              @click="moveGroup(binding.group_id, 1)"
            >
              <Icon name="chevronDown" size="xs" />
            </button>
          </div>
          <button
            type="button"
            class="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-md text-gray-400 hover:bg-red-50 hover:text-red-600 focus:outline-none focus:ring-2 focus:ring-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400"
            :aria-label="t('keys.removeGroup', { name: groupFor(binding.group_id)?.name ?? binding.group_id })"
            :data-test="`remove-group-${binding.group_id}`"
            @click="removeGroup(binding.group_id)"
          >
            <Icon name="x" size="sm" />
          </button>
        </div>
      </VueDraggable>
    </div>

    <div>
      <div class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
        {{ t('keys.availableGroups') }}
      </div>
      <div class="max-h-60 space-y-1 overflow-y-auto overscroll-contain pr-1" data-test="available-group-list">
        <button
          v-for="group in filteredAvailableGroups"
          :key="group.id"
          type="button"
          :class="[
            'flex w-full min-w-0 items-center gap-2 rounded-lg border px-2.5 py-2 text-left transition-all hover:brightness-95 focus:outline-none focus:ring-2 focus:ring-primary-500 dark:hover:brightness-110',
            platformBadgeClass(group.platform)
          ]"
          :data-test="`add-group-${group.id}`"
          @click="addGroup(group.id)"
        >
          <span class="flex h-5 w-5 flex-shrink-0 items-center justify-center rounded border border-gray-300 text-gray-400 dark:border-dark-500">
            <Icon name="plus" size="xs" />
          </span>
          <PlatformIcon :platform="group.platform" size="xs" />
          <div class="min-w-0 flex-1">
            <div class="truncate text-sm font-semibold">{{ group.name }}</div>
            <div class="truncate text-[11px] opacity-70">
              {{ platformName(group.platform) }}<template v-if="group.description"> · {{ group.description }}</template>
            </div>
          </div>
          <span class="flex-shrink-0 rounded-md bg-white/70 px-1.5 py-0.5 text-xs font-semibold tabular-nums shadow-sm dark:bg-black/20">
            {{ formatMultiplier(rateFor(group)) }}
          </span>
        </button>
        <div
          v-if="filteredAvailableGroups.length === 0"
          class="rounded-lg border border-dashed border-gray-200 px-3 py-6 text-center text-sm text-gray-400 dark:border-dark-600 dark:text-gray-500"
          data-test="no-available-groups"
        >
          {{ searchQuery.trim() ? t('keys.noGroupFound') : t('keys.allGroupsSelected') }}
        </div>
      </div>
    </div>

    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('keys.multiGroupHint') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { VueDraggable } from 'vue-draggable-plus'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import { platformBadgeClass, platformLabel } from '@/utils/platformColors'
import type { ApiKeyGroupBindingInput, Group } from '@/types'

const props = withDefaults(defineProps<{
  groups: Group[]
  modelValue: ApiKeyGroupBindingInput[]
  userGroupRates?: Record<number, number>
}>(), {
  userGroupRates: () => ({})
})

const emit = defineEmits<{
  (event: 'update:modelValue', value: ApiKeyGroupBindingInput[]): void
}>()

const { t } = useI18n()
const searchInputRef = ref<HTMLInputElement | null>(null)
const searchQuery = ref('')
const selected = ref<ApiKeyGroupBindingInput[]>([])

watch(
  () => props.modelValue,
  (value) => {
    const normalized = normalizeBindings(value)
    if (bindingSignature(normalized) !== bindingSignature(selected.value)) {
      selected.value = normalized
    }
  },
  { immediate: true, deep: true }
)

const groupMap = computed(() => new Map(props.groups.map((group) => [group.id, group])))
const selectedIds = computed(() => new Set(selected.value.map((binding) => binding.group_id)))
const filteredAvailableGroups = computed(() => {
  const query = searchQuery.value.trim().toLocaleLowerCase()
  return props.groups.filter((group) => {
    if (selectedIds.value.has(group.id)) return false
    if (!query) return true
    return [group.name, group.platform, group.description ?? '']
      .some((value) => value.toLocaleLowerCase().includes(query))
  })
})

function normalizeBindings(bindings: ApiKeyGroupBindingInput[] | undefined): ApiKeyGroupBindingInput[] {
  const seen = new Set<number>()
  return [...(bindings ?? [])]
    .sort((left, right) => left.priority - right.priority)
    .filter((binding) => {
      if (seen.has(binding.group_id)) return false
      seen.add(binding.group_id)
      return true
    })
    .map((binding, priority) => ({ group_id: binding.group_id, priority }))
}

function bindingSignature(bindings: ApiKeyGroupBindingInput[]): string {
  return bindings.map((binding) => binding.group_id).join(',')
}

function groupFor(groupId: number): Group | undefined {
  return groupMap.value.get(groupId)
}

function rateFor(group: Group): number {
  return props.userGroupRates[group.id] ?? group.rate_multiplier
}

function rateForId(groupId: number): number {
  const group = groupFor(groupId)
  return group ? rateFor(group) : 1
}

function formatMultiplier(rate: number): string {
  return `${Number(rate.toFixed(3)).toString()}x`
}

function platformName(platform?: string): string {
  return platform ? platformLabel(platform) : t('keys.unknownPlatform')
}

function emitChange(): void {
  const seen = new Set<number>()
  selected.value = selected.value
    .filter((binding) => {
      if (seen.has(binding.group_id)) return false
      seen.add(binding.group_id)
      return true
    })
    .map((binding, priority) => ({ group_id: binding.group_id, priority }))
  emit('update:modelValue', selected.value.map((binding) => ({ ...binding })))
}

function addGroup(groupId: number): void {
  if (selectedIds.value.has(groupId)) return
  selected.value.push({ group_id: groupId, priority: selected.value.length })
  emitChange()
}

function removeGroup(groupId: number): void {
  selected.value = selected.value.filter((binding) => binding.group_id !== groupId)
  emitChange()
}

function moveGroup(groupId: number, delta: -1 | 1): void {
  const currentIndex = selected.value.findIndex((binding) => binding.group_id === groupId)
  const targetIndex = currentIndex + delta
  if (currentIndex < 0 || targetIndex < 0 || targetIndex >= selected.value.length) return
  const next = [...selected.value]
  const [binding] = next.splice(currentIndex, 1)
  if (!binding) return
  next.splice(targetIndex, 0, binding)
  selected.value = next
  emitChange()
}

async function focusSearch(): Promise<void> {
  await nextTick()
  searchInputRef.value?.focus()
}

defineExpose({ focusSearch })
</script>

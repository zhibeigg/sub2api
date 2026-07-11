<template>
  <BaseDialog :show="show" :title="t('admin.users.userApiKeys')" width="wide" @close="handleClose">
    <div v-if="user" class="space-y-4">
      <div class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
        <div class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30">
          <span class="text-lg font-medium text-primary-700 dark:text-primary-300">{{ user.email.charAt(0).toUpperCase() }}</span>
        </div>
        <div class="min-w-0">
          <p class="truncate font-medium text-gray-900 dark:text-white">{{ user.email }}</p>
          <p class="truncate text-sm text-gray-500 dark:text-dark-400">{{ user.username }}</p>
        </div>
      </div>

      <div v-if="loading" class="flex justify-center py-8">
        <Icon name="refresh" size="xl" class="animate-spin text-primary-500" />
      </div>
      <div v-else-if="apiKeys.length === 0" class="py-8 text-center">
        <p class="text-sm text-gray-500">{{ t('admin.users.noApiKeys') }}</p>
      </div>
      <div v-else ref="scrollContainerRef" class="max-h-96 space-y-3 overflow-y-auto" @scroll="closeGroupSelector(false)">
        <article
          v-for="key in apiKeys"
          :key="key.id"
          class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0 flex-1">
              <div class="mb-1 flex flex-wrap items-center gap-2">
                <span class="font-medium text-gray-900 dark:text-white">{{ key.name }}</span>
                <span :class="['badge text-xs', key.status === 'active' ? 'badge-success' : 'badge-danger']">{{ key.status }}</span>
              </div>
              <p class="truncate font-mono text-sm text-gray-500">{{ key.key.substring(0, 20) }}...{{ key.key.substring(key.key.length - 8) }}</p>
            </div>
          </div>

          <div class="mt-3 flex flex-col gap-2 text-xs text-gray-500 sm:flex-row sm:flex-wrap sm:items-center sm:gap-4">
            <div class="flex min-w-0 items-start gap-1.5">
              <span class="flex-shrink-0 pt-1">{{ t('admin.users.group') }}:</span>
              <button
                :ref="(element) => setGroupButtonRef(key.id, element)"
                type="button"
                class="group-binding-trigger flex min-w-0 flex-1 flex-wrap items-center gap-1.5 rounded-lg px-1.5 py-1 text-left transition-colors hover:bg-gray-100 focus:outline-none focus:ring-2 focus:ring-primary-500 dark:hover:bg-dark-700 sm:flex-none"
                :disabled="updatingKeyIds.has(key.id)"
                :aria-expanded="groupSelectorKeyId === key.id"
                @click="openGroupSelector(key)"
              >
                <template v-if="orderedBindingsForKey(key).length">
                  <span
                    v-for="(binding, index) in visibleBindingsForKey(key)"
                    :key="binding.group_id"
                    :data-test="`admin-api-key-group-${binding.group_id}`"
                    :class="[
                      'inline-flex min-w-0 items-center gap-1 rounded-md border px-1.5 py-1',
                      platformBadgeClass(groupForBinding(binding)?.platform ?? '')
                    ]"
                  >
                    <span class="flex h-4 w-4 flex-shrink-0 items-center justify-center rounded-full bg-white/70 text-[10px] font-semibold shadow-sm dark:bg-black/20">{{ index + 1 }}</span>
                    <PlatformIcon v-if="groupForBinding(binding)" :platform="groupForBinding(binding)!.platform" size="xs" />
                    <span class="max-w-24 truncate font-semibold">{{ groupForBinding(binding)?.name || `#${binding.group_id}` }}</span>
                    <span class="rounded bg-white/70 px-1 py-0.5 font-semibold tabular-nums shadow-sm dark:bg-black/20">{{ formatMultiplier(rateForGroup(groupForBinding(binding))) }}</span>
                  </span>
                  <span
                    v-if="orderedBindingsForKey(key).length > GROUP_BINDING_PREVIEW_LIMIT"
                    class="rounded-md bg-gray-100 px-1.5 py-1 font-semibold text-gray-600 dark:bg-dark-700 dark:text-gray-300"
                  >+{{ orderedBindingsForKey(key).length - GROUP_BINDING_PREVIEW_LIMIT }}</span>
                </template>
                <span v-else class="italic text-gray-400">{{ t('admin.users.none') }}</span>
                <Icon v-if="updatingKeyIds.has(key.id)" name="refresh" size="xs" class="animate-spin text-primary-500" />
                <Icon v-else name="chevronDown" size="xs" class="text-gray-400" />
              </button>
            </div>
            <div>{{ t('admin.users.columns.created') }}: {{ formatDateTime(key.created_at) }}</div>
          </div>
        </article>
      </div>
    </div>
  </BaseDialog>

  <Teleport to="body">
    <section
      v-if="groupSelectorKeyId !== null && dropdownPosition && selectedKeyForGroup"
      ref="dropdownRef"
      role="dialog"
      :aria-label="t('admin.users.editGroupBindings')"
      class="fixed z-[100000020] flex max-h-[calc(100vh-24px)] flex-col overflow-hidden rounded-xl border border-gray-200 bg-white shadow-2xl dark:border-dark-600 dark:bg-dark-800"
      :style="{
        top: dropdownPosition.top !== undefined ? dropdownPosition.top + 'px' : undefined,
        bottom: dropdownPosition.bottom !== undefined ? dropdownPosition.bottom + 'px' : undefined,
        left: dropdownPosition.left + 'px',
        width: dropdownPosition.width + 'px'
      }"
      data-test="admin-group-binding-popover"
      @click.stop
    >
      <header class="flex items-start justify-between gap-3 border-b border-gray-100 px-4 py-3 dark:border-dark-700">
        <div class="min-w-0">
          <h3 class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.users.editGroupBindings') }}</h3>
          <p class="truncate text-xs text-gray-500 dark:text-gray-400">{{ selectedKeyForGroup.name }}</p>
        </div>
        <button
          type="button"
          class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg text-gray-400 hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus:ring-2 focus:ring-primary-500 dark:hover:bg-dark-700 dark:hover:text-gray-200"
          :aria-label="t('common.close')"
          @click="closeGroupSelector()"
        >
          <Icon name="x" size="sm" />
        </button>
      </header>
      <div class="min-h-0 flex-1 overflow-y-auto p-4">
        <SortableGroupBindingPicker
          ref="bindingPickerRef"
          v-model="draftGroupBindings"
          :groups="allGroups"
          :user-group-rates="userGroupRates"
        />
      </div>
      <footer class="flex flex-col-reverse gap-2 border-t border-gray-100 px-4 py-3 sm:flex-row sm:justify-end dark:border-dark-700">
        <button type="button" class="btn btn-secondary" :disabled="savingGroupBindings" @click="closeGroupSelector()">
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          class="btn btn-secondary"
          :disabled="savingGroupBindings"
          data-test="admin-save-group-bindings-continue"
          @click="saveGroupBindings(true)"
        >
          {{ t('keys.saveAndContinue') }}
        </button>
        <button
          type="button"
          class="btn btn-primary"
          :disabled="savingGroupBindings"
          data-test="admin-save-group-bindings"
          @click="saveGroupBindings(false)"
        >
          {{ savingGroupBindings ? t('keys.saving') : t('common.save') }}
        </button>
      </footer>
    </section>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch, type ComponentPublicInstance } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { formatDateTime } from '@/utils/format'
import { platformBadgeClass } from '@/utils/platformColors'
import type { AdminGroup, AdminUser, ApiKey, ApiKeyGroupBinding, ApiKeyGroupBindingInput, Group } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import SortableGroupBindingPicker from '@/components/keys/SortableGroupBindingPicker.vue'

const props = defineProps<{ show: boolean; user: AdminUser | null }>()
const emit = defineEmits<{ (event: 'close'): void }>()
const { t } = useI18n()
const appStore = useAppStore()

const apiKeys = ref<ApiKey[]>([])
const allGroups = ref<AdminGroup[]>([])
const loading = ref(false)
const updatingKeyIds = ref(new Set<number>())
const groupSelectorKeyId = ref<number | null>(null)
const dropdownPosition = ref<{ top?: number; bottom?: number; left: number; width: number } | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const bindingPickerRef = ref<InstanceType<typeof SortableGroupBindingPicker> | null>(null)
const scrollContainerRef = ref<HTMLElement | null>(null)
const groupButtonRefs = ref<Map<number, HTMLElement>>(new Map())
const draftGroupBindings = ref<ApiKeyGroupBindingInput[]>([])
const savingGroupBindings = ref(false)
const GROUP_BINDING_PREVIEW_LIMIT = 2

const selectedKeyForGroup = computed(() => {
  if (groupSelectorKeyId.value === null) return null
  return apiKeys.value.find((key) => key.id === groupSelectorKeyId.value) ?? null
})
const userGroupRates = computed<Record<number, number>>(() => props.user?.group_rates ?? {})

const orderedBindingsForKey = (key: ApiKey): ApiKeyGroupBinding[] => {
  if (key.group_bindings !== undefined) {
    return [...key.group_bindings].sort((left, right) => left.priority - right.priority)
  }
  if (key.group_id != null) {
    return [{ group_id: key.group_id, priority: 0, group: key.group }]
  }
  return []
}

const visibleBindingsForKey = (key: ApiKey): ApiKeyGroupBinding[] =>
  orderedBindingsForKey(key).slice(0, GROUP_BINDING_PREVIEW_LIMIT)

const groupForBinding = (binding: ApiKeyGroupBinding): Group | undefined =>
  binding.group ?? allGroups.value.find((group) => group.id === binding.group_id)

const rateForGroup = (group?: Group): number => group ? (userGroupRates.value[group.id] ?? group.rate_multiplier) : 1
const formatMultiplier = (rate: number): string => `${Number(rate.toFixed(3)).toString()}x`

const setGroupButtonRef = (keyId: number, element: Element | ComponentPublicInstance | null) => {
  if (element instanceof HTMLElement) groupButtonRefs.value.set(keyId, element)
  else groupButtonRefs.value.delete(keyId)
}

watch(() => props.show, (visible) => {
  if (visible && props.user) {
    void load()
    void loadGroups()
  } else {
    closeGroupSelector(false)
  }
})

const load = async () => {
  if (!props.user) return
  loading.value = true
  groupButtonRefs.value.clear()
  try {
    const response = await adminAPI.users.getUserApiKeys(props.user.id)
    apiKeys.value = response.items ?? []
  } catch (error) {
    console.error('Failed to load API keys:', error)
  } finally {
    loading.value = false
  }
}

const loadGroups = async () => {
  try {
    allGroups.value = await adminAPI.groups.getAll()
  } catch (error) {
    console.error('Failed to load groups:', error)
  }
}

const closeGroupSelector = (restoreFocus = true) => {
  const keyId = groupSelectorKeyId.value
  groupSelectorKeyId.value = null
  dropdownPosition.value = null
  draftGroupBindings.value = []
  if (restoreFocus && keyId !== null) {
    nextTick(() => groupButtonRefs.value.get(keyId)?.focus())
  }
}

const openGroupSelector = async (key: ApiKey) => {
  if (groupSelectorKeyId.value === key.id) {
    closeGroupSelector()
    return
  }
  const trigger = groupButtonRefs.value.get(key.id)
  if (!trigger) return

  const rect = trigger.getBoundingClientRect()
  const viewportPadding = 12
  const gap = 6
  const width = Math.min(460, Math.max(1, window.innerWidth - viewportPadding * 2))
  const left = Math.min(
    Math.max(viewportPadding, rect.left),
    Math.max(viewportPadding, window.innerWidth - width - viewportPadding)
  )
  const estimatedHeight = Math.min(620, window.innerHeight - viewportPadding * 2)
  const spaceBelow = window.innerHeight - rect.bottom
  const openUpward = spaceBelow < estimatedHeight && rect.top > spaceBelow

  dropdownPosition.value = openUpward
    ? { bottom: window.innerHeight - rect.top + gap, left, width }
    : { top: rect.bottom + gap, left, width }
  draftGroupBindings.value = orderedBindingsForKey(key).map((binding, priority) => ({
    group_id: binding.group_id,
    priority
  }))
  groupSelectorKeyId.value = key.id
  await nextTick()
  await bindingPickerRef.value?.focusSearch()
}

const saveGroupBindings = async (keepEditing: boolean) => {
  const key = selectedKeyForGroup.value
  if (!key) return

  const orderedBindings = draftGroupBindings.value.map((binding, priority) => ({
    group_id: binding.group_id,
    priority
  }))
  savingGroupBindings.value = true
  updatingKeyIds.value.add(key.id)
  try {
    const result = await adminAPI.apiKeys.updateApiKeyGroupBindings(key.id, orderedBindings)
    const hydratedBindings: ApiKeyGroupBinding[] = orderedBindings.map((binding) => ({
      ...binding,
      group: allGroups.value.find((group) => group.id === binding.group_id)
    }))
    const index = apiKeys.value.findIndex((item) => item.id === key.id)
    if (index !== -1) {
      apiKeys.value[index] = {
        ...key,
        ...result.api_key,
        group_bindings: result.api_key.group_bindings ?? hydratedBindings
      }
    }
    draftGroupBindings.value = orderedBindings
    if (result.auto_granted_group_access && result.granted_group_name) {
      appStore.showSuccess(t('admin.users.groupChangedWithGrant', { group: result.granted_group_name }))
    } else {
      appStore.showSuccess(t('admin.users.groupBindingsSaved'))
    }
    if (!keepEditing) closeGroupSelector()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.users.groupChangeFailed'))
  } finally {
    updatingKeyIds.value.delete(key.id)
    savingGroupBindings.value = false
  }
}

const handleKeyDown = (event: KeyboardEvent) => {
  if (event.key === 'Escape' && groupSelectorKeyId.value !== null) {
    event.preventDefault()
    event.stopPropagation()
    closeGroupSelector()
  }
}

const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (groupSelectorKeyId.value === null || dropdownRef.value?.contains(target)) return
  for (const element of groupButtonRefs.value.values()) {
    if (element.contains(target)) return
  }
  closeGroupSelector(false)
}

const handleClose = () => {
  closeGroupSelector(false)
  emit('close')
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  document.addEventListener('keydown', handleKeyDown, true)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  document.removeEventListener('keydown', handleKeyDown, true)
})
</script>

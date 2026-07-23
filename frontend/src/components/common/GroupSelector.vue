<template>
  <div>
    <label class="input-label">
      {{ t('admin.users.groups') }}
      <span class="font-normal text-gray-400">{{ t('common.selectedCount', { count: modelValue.length }) }}</span>
    </label>
    <div
      v-if="isSearchable"
      class="flex items-center gap-2 rounded-t-lg border border-b-0 border-gray-200 bg-gray-50 px-3 py-2 dark:border-dark-600 dark:bg-dark-800"
    >
      <Icon name="search" size="sm" class="shrink-0 text-gray-400" />
      <input
        v-model="searchText"
        type="text"
        :placeholder="t('common.searchPlaceholder')"
        class="flex-1 bg-transparent text-sm text-gray-900 placeholder:text-gray-400 focus:outline-none dark:text-gray-100 dark:placeholder:text-dark-400"
      />
    </div>
    <div
      :class="[
        'grid max-h-32 grid-cols-2 gap-1 overflow-y-auto p-2',
        isSearchable
          ? 'rounded-b-lg border border-t-0 border-gray-200 bg-gray-50 dark:border-dark-600 dark:bg-dark-800'
          : 'rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-600 dark:bg-dark-800'
      ]"
    >
      <label
        v-for="item in filteredGroups"
        :key="item.group.id"
        :class="[
          'flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 transition-colors hover:bg-white dark:hover:bg-dark-700',
          item.compatible ? '' : 'bg-amber-50/80 ring-1 ring-inset ring-amber-200 dark:bg-amber-900/10 dark:ring-amber-800'
        ]"
        :title="item.compatible
          ? t('admin.groups.rateAndAccounts', { rate: item.group.rate_multiplier, count: item.group.account_count || 0 })
          : t('admin.groups.endpointProtocols.selectedIncompatible')"
        :data-incompatible="item.compatible ? undefined : 'true'"
      >
        <input
          :type="multiple ? 'checkbox' : 'radio'"
          :name="multiple ? undefined : radioGroupName"
          :value="item.group.id"
          :checked="modelValue.includes(item.group.id)"
          @change="handleChange(item.group.id, ($event.target as HTMLInputElement).checked)"
          :class="[
            'h-3.5 w-3.5 shrink-0 border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500',
            multiple ? 'rounded' : 'rounded-full'
          ]"
        />
        <GroupBadge
          :name="item.group.name"
          :platform="item.group.platform"
          :endpoint-protocols="item.protocols"
          :subscription-type="item.group.subscription_type"
          :rate-multiplier="item.group.rate_multiplier"
          class="min-w-0 flex-1"
        />
        <Icon
          v-if="!item.compatible"
          name="exclamationTriangle"
          size="xs"
          class="shrink-0 text-amber-500"
        />
        <span class="shrink-0 text-xs text-gray-400">{{ item.group.account_count || 0 }}</span>
      </label>
      <div
        v-if="filteredGroups.length === 0"
        class="col-span-2 py-2 text-center text-sm text-gray-500 dark:text-gray-400"
      >
        {{ t('common.noGroupsAvailable') }}
      </div>
    </div>
    <p
      v-if="incompatibleSelectedCount > 0"
      class="mt-1.5 flex items-start gap-1.5 text-xs text-amber-600 dark:text-amber-400"
      role="alert"
    >
      <Icon name="exclamationTriangle" size="xs" class="mt-0.5 shrink-0" />
      <span>{{ t('admin.groups.endpointProtocols.incompatibleSelectedWarning', { count: incompatibleSelectedCount }) }}</span>
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import GroupBadge from './GroupBadge.vue'
import Icon from '@/components/icons/Icon.vue'
import type { AdminGroup, EndpointProtocol, GroupPlatform } from '@/types'
import {
  endpointProtocolsIntersect,
  getGroupEndpointProtocols
} from '@/constants/platforms'

const { t } = useI18n()

interface Props {
  modelValue: number[]
  groups: AdminGroup[]
  supportedEndpointProtocols?: EndpointProtocol[]
  platform?: GroupPlatform // 旧调用兼容：未提供协议能力时使用旧平台筛选
  mixedScheduling?: boolean // 旧调用兼容
  searchable?: boolean | 'auto'
  multiple?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  searchable: 'auto',
  multiple: true
})
const emit = defineEmits<{
  'update:modelValue': [value: number[]]
}>()

const searchText = ref('')
const radioGroupName = `group-selector-${Math.random().toString(36).slice(2)}`

const isSearchable = computed(() => {
  if (props.searchable === 'auto') return props.groups.length > 5
  return props.searchable
})

const hasProtocolFilter = computed(() => props.supportedEndpointProtocols !== undefined)

function legacyPlatformCompatible(group: AdminGroup): boolean {
  if (!props.platform) return true
  if (props.platform === 'antigravity' && props.mixedScheduling) {
    return group.platform === 'antigravity' || group.platform === 'anthropic' || group.platform === 'gemini'
  }
  if (props.platform === 'kiro' && props.mixedScheduling) {
    return group.platform === 'kiro' || group.platform === 'anthropic' || group.platform === 'openai'
  }
  if (props.platform === 'opencode' && props.mixedScheduling) {
    return group.platform === 'opencode' || group.platform === 'openai' || group.platform === 'anthropic'
  }
  if (props.platform === 'cursor' && props.mixedScheduling) {
    return ['cursor', 'anthropic', 'gemini', 'openai', 'grok'].includes(group.platform)
  }
  return group.platform === props.platform
}

const groupItems = computed(() => props.groups.map((group) => {
  const protocols = getGroupEndpointProtocols(group)
  const compatible = hasProtocolFilter.value
    ? endpointProtocolsIntersect(props.supportedEndpointProtocols ?? [], protocols)
    : legacyPlatformCompatible(group)
  return { group, protocols, compatible }
}))

const incompatibleSelectedCount = computed(() => groupItems.value.filter(
  (item) => props.modelValue.includes(item.group.id) && !item.compatible
).length)

const filteredGroups = computed(() => {
  const q = searchText.value.trim().toLowerCase()
  return groupItems.value.filter((item) => {
    const selected = props.modelValue.includes(item.group.id)
    if (!item.compatible && !selected) return false
    if (!q || selected) return true
    return item.group.name.toLowerCase().includes(q) || item.group.description?.toLowerCase().includes(q)
  })
})

const handleChange = (groupId: number, checked: boolean) => {
  if (!props.multiple) {
    emit('update:modelValue', checked ? [groupId] : [])
    return
  }

  const newValue = checked
    ? [...new Set([...props.modelValue, groupId])]
    : props.modelValue.filter((id) => id !== groupId)
  emit('update:modelValue', newValue)
}
</script>

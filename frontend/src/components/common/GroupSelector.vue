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
        v-for="group in filteredGroups"
        :key="group.id"
        class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 transition-colors hover:bg-white dark:hover:bg-dark-700"
        :title="t('admin.groups.rateAndAccounts', { rate: group.rate_multiplier, count: group.account_count || 0 })"
      >
        <input
          :type="multiple ? 'checkbox' : 'radio'"
          :name="multiple ? undefined : radioGroupName"
          :value="group.id"
          :checked="modelValue.includes(group.id)"
          @change="handleChange(group.id, ($event.target as HTMLInputElement).checked)"
          :class="[
            'h-3.5 w-3.5 shrink-0 border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500',
            multiple ? 'rounded' : 'rounded-full'
          ]"
        />
        <GroupBadge
          :name="group.name"
          :platform="group.platform"
          :subscription-type="group.subscription_type"
          :rate-multiplier="group.rate_multiplier"
          class="min-w-0 flex-1"
        />
        <span class="shrink-0 text-xs text-gray-400">{{ group.account_count || 0 }}</span>
      </label>
      <div
        v-if="filteredGroups.length === 0"
        class="col-span-2 py-2 text-center text-sm text-gray-500 dark:text-gray-400"
      >
        {{ t('common.noGroupsAvailable') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import GroupBadge from './GroupBadge.vue'
import Icon from '@/components/icons/Icon.vue'
import type { AdminGroup, GroupPlatform } from '@/types'

const { t } = useI18n()

interface Props {
  modelValue: number[]
  groups: AdminGroup[]
  platform?: GroupPlatform // Optional platform filter
  mixedScheduling?: boolean // Allow compatible cross-platform groups for mixed-scheduling accounts
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

// Filter groups by platform if specified
const filteredGroups = computed(() => {
  let result: AdminGroup[] = props.groups
  if (props.platform) {
    // antigravity 账户启用混合调度后，可选择 anthropic/gemini 分组
    if (props.platform === 'antigravity' && props.mixedScheduling) {
      result = result.filter(
        (g) => g.platform === 'antigravity' || g.platform === 'anthropic' || g.platform === 'gemini'
      )
    } else if (props.platform === 'kiro' && props.mixedScheduling) {
      // kiro 账户启用混合调度后，可选择 anthropic（Claude via /v1/messages）
      // 与 openai（国模等，模型 ID 透传给上游）分组
      result = result.filter(
        (g) => g.platform === 'kiro' || g.platform === 'anthropic' || g.platform === 'openai'
      )
    } else if (props.platform === 'opencode' && props.mixedScheduling) {
      result = result.filter(
        (g) => g.platform === 'opencode' || g.platform === 'openai' || g.platform === 'anthropic'
      )
    } else if (props.platform === 'cursor' && props.mixedScheduling) {
      // Cursor Cloud Agents 模型目录覆盖 Claude、Gemini、OpenAI、Grok 及 Cursor 原生模型。
      result = result.filter(
        (g) =>
          g.platform === 'cursor' ||
          g.platform === 'anthropic' ||
          g.platform === 'gemini' ||
          g.platform === 'openai' ||
          g.platform === 'grok'
      )
    } else {
      // 默认：只能选择同 platform 的分组
      result = result.filter((g) => g.platform === props.platform)
    }
  }
  if (isSearchable.value && searchText.value) {
    const q = searchText.value.toLowerCase()
    result = result.filter(
      (g) => g.name.toLowerCase().includes(q) || g.description?.toLowerCase().includes(q)
    )
  }
  return result
})

const handleChange = (groupId: number, checked: boolean) => {
  if (!props.multiple) {
    emit('update:modelValue', checked ? [groupId] : [])
    return
  }

  const newValue = checked
    ? [...props.modelValue, groupId]
    : props.modelValue.filter((id) => id !== groupId)
  emit('update:modelValue', newValue)
}
</script>

<template>
  <BaseDialog
    :show="show"
    :title="t('admin.users.groupConfig')"
    width="wide"
    @close="handleClose"
  >
    <div v-if="user" class="space-y-5">
      <div
        class="flex items-center gap-3 rounded-2xl bg-gradient-to-r from-primary-50 to-primary-100 p-4 dark:from-primary-900/30 dark:to-primary-800/20 sm:gap-4 sm:p-5"
      >
        <div
          class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-full bg-white shadow-sm dark:bg-dark-700 sm:h-14 sm:w-14"
          aria-hidden="true"
        >
          <span class="text-xl font-semibold text-primary-600 dark:text-primary-400 sm:text-2xl">
            {{ user.email.charAt(0).toUpperCase() }}
          </span>
        </div>
        <div class="min-w-0 flex-1">
          <p class="truncate text-base font-semibold text-gray-900 dark:text-white sm:text-lg">
            {{ user.email }}
          </p>
          <p class="mt-1 text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.users.groupConfigHint') }}
          </p>
        </div>
      </div>

      <div v-if="loading" class="flex min-h-56 items-center justify-center" role="status">
        <svg
          class="h-10 w-10 animate-spin text-primary-500"
          fill="none"
          viewBox="0 0 24 24"
          aria-hidden="true"
        >
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
          <path
            class="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
          />
        </svg>
        <span class="sr-only">{{ t('common.loading') }}</span>
      </div>

      <div
        v-else-if="loadError"
        class="flex min-h-56 flex-col items-center justify-center rounded-xl border border-red-200 bg-red-50 p-6 text-center dark:border-red-900/60 dark:bg-red-900/20"
        role="alert"
      >
        <p class="font-medium text-red-700 dark:text-red-300">
          {{ t('admin.users.groupConfigLoadFailed') }}
        </p>
        <p class="mt-2 max-w-lg text-sm text-red-600 dark:text-red-400">{{ loadError }}</p>
        <button
          type="button"
          class="btn btn-secondary mt-5 min-h-11 px-5 focus-visible:ring-2 focus-visible:ring-primary-500"
          data-test="retry-load"
          @click="load"
        >
          {{ t('common.retry') }}
        </button>
      </div>

      <div v-else class="space-y-5">
        <fieldset class="space-y-3">
          <legend class="text-sm font-semibold text-gray-800 dark:text-gray-200">
            {{ t('admin.users.accessMode') }}
          </legend>
          <div class="grid gap-3 md:grid-cols-2">
            <label
              class="flex min-h-20 cursor-pointer gap-3 rounded-xl border-2 p-4 transition-colors focus-within:ring-2 focus-within:ring-primary-500/40"
              :class="accessMode === 'inherit'
                ? 'border-primary-500 bg-primary-50/70 dark:bg-primary-900/20'
                : 'border-gray-200 bg-white dark:border-dark-600 dark:bg-dark-800'"
            >
              <input
                type="radio"
                name="group-access-mode"
                value="inherit"
                class="mt-1 h-5 w-5 flex-shrink-0 accent-primary-600"
                :checked="accessMode === 'inherit'"
                :aria-label="t('admin.users.accessModeInherit')"
                data-test="mode-inherit"
                @change="setAccessMode('inherit')"
              />
              <span>
                <span class="block font-semibold text-gray-900 dark:text-white">
                  {{ t('admin.users.accessModeInherit') }}
                </span>
                <span class="mt-1 block text-sm text-gray-600 dark:text-gray-400">
                  {{ t('admin.users.accessModeInheritHint') }}
                </span>
              </span>
            </label>

            <label
              class="flex min-h-20 cursor-pointer gap-3 rounded-xl border-2 p-4 transition-colors focus-within:ring-2 focus-within:ring-primary-500/40"
              :class="accessMode === 'restricted'
                ? 'border-amber-500 bg-amber-50/70 dark:bg-amber-900/20'
                : 'border-gray-200 bg-white dark:border-dark-600 dark:bg-dark-800'"
            >
              <input
                type="radio"
                name="group-access-mode"
                value="restricted"
                class="mt-1 h-5 w-5 flex-shrink-0 accent-amber-600"
                :checked="accessMode === 'restricted'"
                :aria-label="t('admin.users.accessModeRestricted')"
                data-test="mode-restricted"
                @change="setAccessMode('restricted')"
              />
              <span>
                <span class="block font-semibold text-gray-900 dark:text-white">
                  {{ t('admin.users.accessModeRestricted') }}
                </span>
                <span class="mt-1 block text-sm text-gray-600 dark:text-gray-400">
                  {{ t('admin.users.accessModeRestrictedHint') }}
                </span>
              </span>
            </label>
          </div>

          <div
            v-if="accessMode === 'restricted'"
            class="flex flex-col gap-2 rounded-xl border border-amber-200 bg-amber-50 p-3 dark:border-amber-900/60 dark:bg-amber-900/20 sm:flex-row sm:items-center sm:justify-between"
          >
            <p class="text-sm text-amber-800 dark:text-amber-300">
              {{ t('admin.users.restrictedModeActive') }}
            </p>
            <button
              type="button"
              class="min-h-11 rounded-lg border border-amber-300 bg-white px-4 text-sm font-medium text-amber-800 transition-colors hover:bg-amber-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-amber-500 dark:border-amber-700 dark:bg-dark-800 dark:text-amber-300 dark:hover:bg-amber-900/30"
              data-test="allow-all-inherit"
              @click="allowAllAndInherit"
            >
              {{ t('admin.users.allowAllAndInherit') }}
            </button>
          </div>
        </fieldset>

        <div class="space-y-3">
          <div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
            <label class="block min-w-0 flex-1">
              <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.users.searchGroups') }}
              </span>
              <input
                ref="searchInput"
                v-model="searchQuery"
                type="search"
                class="min-h-11 w-full rounded-lg border border-gray-300 bg-white px-3 text-sm focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-500 dark:bg-dark-700"
                :placeholder="t('admin.users.searchGroupsPlaceholder')"
                :aria-label="t('admin.users.searchGroups')"
                data-test="group-search"
              />
            </label>
            <div class="grid grid-cols-2 gap-2 sm:flex">
              <button
                type="button"
                class="btn btn-secondary min-h-11 px-3 text-sm focus-visible:ring-2 focus-visible:ring-primary-500"
                :disabled="accessMode === 'inherit' || filteredGroupConfigs.length === 0"
                data-test="select-results"
                @click="setVisibleSelection(true)"
              >
                {{ t('admin.users.selectCurrentResults') }}
              </button>
              <button
                type="button"
                class="btn btn-secondary min-h-11 px-3 text-sm focus-visible:ring-2 focus-visible:ring-primary-500"
                :disabled="accessMode === 'inherit' || filteredGroupConfigs.length === 0"
                data-test="clear-results"
                @click="setVisibleSelection(false)"
              >
                {{ t('admin.users.clearCurrentResults') }}
              </button>
            </div>
          </div>

          <p class="text-sm text-gray-600 dark:text-gray-400" aria-live="polite" data-test="selection-status">
            {{ selectionStatus }}
          </p>
        </div>

        <div
          v-if="accessMode === 'restricted' && selectedCount === 0"
          class="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-900/20 dark:text-red-300"
          role="alert"
          data-test="empty-whitelist-warning"
        >
          {{ t('admin.users.emptyWhitelistWarning') }}
        </div>

        <div v-if="groupConfigs.length === 0" class="py-10 text-center text-gray-500 dark:text-gray-400">
          {{ t('admin.users.noStandardGroups') }}
        </div>

        <div
          v-else-if="filteredGroupConfigs.length === 0"
          class="rounded-xl border border-dashed border-gray-300 py-10 text-center text-gray-500 dark:border-dark-600 dark:text-gray-400"
        >
          {{ t('admin.users.noMatchingGroups') }}
        </div>

        <div v-else class="space-y-5">
          <section v-if="filteredPublicConfigs.length > 0" aria-labelledby="public-groups-title">
            <div class="mb-3 flex items-center gap-2">
              <span class="h-2 w-2 rounded-full bg-emerald-500" aria-hidden="true" />
              <h4 id="public-groups-title" class="text-sm font-semibold text-gray-700 dark:text-gray-300">
                {{ t('admin.users.publicGroups') }}
              </h4>
              <span class="text-xs text-gray-400">
                ({{ selectedPublicCount }}/{{ publicGroupCount }})
              </span>
            </div>
            <div class="grid gap-3">
              <GroupConfigRow
                v-for="config in filteredPublicConfigs"
                :key="config.groupId"
                :config="config"
                :access-disabled="accessMode === 'inherit'"
                :rate-disabled="!canEditRate(config)"
                :custom-rate-label="t('admin.users.customRate')"
                :default-rate-label="t('admin.users.defaultRate')"
                :disabled-rate-hint="t('admin.users.disabledGroupRatePreserved')"
                :exclusive-grant-label="t('admin.users.exclusiveGrant')"
                :exclusive-grant-aria-label="t('admin.users.exclusiveGrantAria', { group: config.groupName })"
                :rate-placeholder="t('admin.users.customRatePlaceholder')"
                :rate-aria-label="t('admin.users.customRateAria', { group: config.groupName })"
                :select-aria-label="t('admin.users.groupPermissionAria', { group: config.groupName })"
                :type-label="t('admin.users.publicGroupLabel')"
                @toggle-access="toggleGroupAccess(config, $event)"
                @toggle-exclusive-grant="toggleExclusiveGrant(config, $event)"
                @rate-input="config.customRateInput = $event"
              />
            </div>
          </section>

          <section v-if="filteredExclusiveConfigs.length > 0" aria-labelledby="exclusive-groups-title">
            <div class="mb-3 flex items-center gap-2">
              <span class="h-2 w-2 rounded-full bg-purple-500" aria-hidden="true" />
              <h4 id="exclusive-groups-title" class="text-sm font-semibold text-gray-700 dark:text-gray-300">
                {{ t('admin.users.exclusiveGroups') }}
              </h4>
              <span class="text-xs text-gray-400">
                ({{ selectedExclusiveCount }}/{{ exclusiveGroupCount }})
              </span>
            </div>
            <div class="grid gap-3">
              <GroupConfigRow
                v-for="config in filteredExclusiveConfigs"
                :key="config.groupId"
                :config="config"
                :access-disabled="accessMode === 'inherit'"
                :rate-disabled="!canEditRate(config)"
                :custom-rate-label="t('admin.users.customRate')"
                :default-rate-label="t('admin.users.defaultRate')"
                :disabled-rate-hint="t('admin.users.disabledGroupRatePreserved')"
                :exclusive-grant-label="t('admin.users.exclusiveGrant')"
                :exclusive-grant-aria-label="t('admin.users.exclusiveGrantAria', { group: config.groupName })"
                :rate-placeholder="t('admin.users.customRatePlaceholder')"
                :rate-aria-label="t('admin.users.customRateAria', { group: config.groupName })"
                :select-aria-label="t('admin.users.groupPermissionAria', { group: config.groupName })"
                :type-label="t('admin.groups.exclusive')"
                @toggle-access="toggleGroupAccess(config, $event)"
                @toggle-exclusive-grant="toggleExclusiveGrant(config, $event)"
                @rate-input="config.customRateInput = $event"
              />
            </div>
          </section>
        </div>

        <p
          v-if="invalidRateCount > 0"
          class="text-sm text-red-600 dark:text-red-400"
          role="alert"
          data-test="invalid-rate-warning"
        >
          {{ t('admin.users.invalidCustomRate', { count: invalidRateCount }) }}
        </p>
      </div>
    </div>

    <template #footer>
      <div class="flex w-full flex-col-reverse gap-3 sm:flex-row sm:justify-end">
        <button
          type="button"
          class="btn btn-secondary min-h-11 px-5 focus-visible:ring-2 focus-visible:ring-primary-500"
          @click="handleClose"
        >
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          class="btn btn-primary min-h-11 px-6 focus-visible:ring-2 focus-visible:ring-primary-500"
          :disabled="loading || !!loadError || submitting || invalidRateCount > 0"
          data-test="save-group-config"
          @click="showSaveConfirm = true"
        >
          {{ t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>

  <ConfirmDialog
    :show="showSaveConfirm"
    :title="t('admin.users.confirmGroupConfigTitle')"
    :message="saveConfirmationMessage"
    :confirm-text="submitting ? t('common.saving') : t('common.confirm')"
    @confirm="handleSave"
    @cancel="showSaveConfirm = false"
  />
</template>

<script setup lang="ts">
import { computed, defineComponent, h, nextTick, ref, watch, type PropType } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type {
  AdminUser,
  Group,
  GroupPlatform,
  UpdateUserGroupConfigRequest,
  UserGroupAccessMode,
} from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'

interface GroupRateConfig {
  groupId: number
  groupName: string
  platform: GroupPlatform
  isExclusive: boolean
  defaultRate: number
  customRateInput: string
  isAccessAllowed: boolean
  isExclusiveGranted: boolean
}

const GroupConfigRow = defineComponent({
  name: 'GroupConfigRow',
  props: {
    config: { type: Object as PropType<GroupRateConfig>, required: true },
    accessDisabled: { type: Boolean, required: true },
    rateDisabled: { type: Boolean, required: true },
    customRateLabel: { type: String, required: true },
    defaultRateLabel: { type: String, required: true },
    disabledRateHint: { type: String, required: true },
    exclusiveGrantLabel: { type: String, required: true },
    exclusiveGrantAriaLabel: { type: String, required: true },
    ratePlaceholder: { type: String, required: true },
    rateAriaLabel: { type: String, required: true },
    selectAriaLabel: { type: String, required: true },
    typeLabel: { type: String, required: true },
  },
  emits: {
    toggleAccess: (_checked: boolean) => true,
    toggleExclusiveGrant: (_checked: boolean) => true,
    rateInput: (_value: string) => true,
  },
  setup(props, { emit }) {
    return () => h(
      'div',
      {
        class: [
          'rounded-xl border-2 p-3 transition-colors sm:p-4',
          props.config.isAccessAllowed
            ? 'border-primary-300 bg-primary-50/40 dark:border-primary-700 dark:bg-primary-900/10'
            : 'border-gray-200 bg-white dark:border-dark-600 dark:bg-dark-800',
        ],
        'data-test': `group-row-${props.config.groupId}`,
      },
      [
        h('div', { class: 'flex flex-col gap-3 lg:flex-row lg:items-center lg:gap-4' }, [
          h('label', {
            class: [
              'flex min-h-11 min-w-0 flex-1 items-center gap-3',
              props.accessDisabled ? 'cursor-not-allowed opacity-75' : 'cursor-pointer',
            ],
          }, [
            h('input', {
              type: 'checkbox',
              checked: props.config.isAccessAllowed,
              disabled: props.accessDisabled,
              class: 'h-5 w-5 flex-shrink-0 rounded accent-primary-600 disabled:cursor-not-allowed',
              'aria-label': props.selectAriaLabel,
              'data-test': `group-checkbox-${props.config.groupId}`,
              onChange: (event: Event) => emit('toggleAccess', (event.target as HTMLInputElement).checked),
            }),
            h('span', { class: 'min-w-0 flex-1' }, [
              h('span', { class: 'flex flex-wrap items-center gap-2' }, [
                h('span', { class: 'font-semibold text-gray-900 dark:text-white' }, props.config.groupName),
                h(
                  'span',
                  { class: 'rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300' },
                  props.typeLabel
                ),
              ]),
              h('span', { class: 'mt-1 flex flex-wrap items-center gap-2 text-sm text-gray-500 dark:text-gray-400' }, [
                h(PlatformIcon, { platform: props.config.platform, size: 'xs' }),
                h('span', props.config.platform),
                h('span', { 'aria-hidden': 'true' }, '•'),
                h('span', `${props.defaultRateLabel}: ${props.config.defaultRate}x`),
              ]),
            ]),
          ]),
          props.config.isExclusive
            ? h('label', { class: 'flex min-h-11 items-center gap-2 rounded-lg border border-purple-200 px-3 dark:border-purple-900/70' }, [
                h('input', {
                  type: 'checkbox',
                  checked: props.config.isExclusiveGranted,
                  class: 'h-5 w-5 rounded accent-purple-600',
                  'aria-label': props.exclusiveGrantAriaLabel,
                  'data-test': `exclusive-grant-${props.config.groupId}`,
                  onChange: (event: Event) => emit('toggleExclusiveGrant', (event.target as HTMLInputElement).checked),
                }),
                h('span', { class: 'text-sm font-medium text-purple-700 dark:text-purple-300' }, props.exclusiveGrantLabel),
              ])
            : null,
          h('label', { class: 'flex min-h-11 items-center gap-2 lg:flex-shrink-0' }, [
            h('span', { class: 'text-sm font-medium text-gray-600 dark:text-gray-400' }, props.customRateLabel),
            h('input', {
              type: 'number',
              step: '0.001',
              min: '0.001',
              value: props.config.customRateInput,
              disabled: props.rateDisabled,
              placeholder: props.ratePlaceholder,
              class: 'hide-spinner min-h-11 min-w-0 flex-1 rounded-lg border border-gray-300 bg-white px-3 text-sm font-medium focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 disabled:cursor-not-allowed disabled:bg-gray-100 disabled:text-gray-500 dark:border-dark-500 dark:bg-dark-700 dark:disabled:bg-dark-800 lg:w-28 lg:flex-none',
              'aria-label': props.rateAriaLabel,
              'data-test': `rate-${props.config.groupId}`,
              onInput: (event: Event) => emit('rateInput', (event.target as HTMLInputElement).value),
            }),
          ]),
        ]),
        props.rateDisabled && props.config.customRateInput.trim() !== ''
          ? h(
              'p',
              {
                class: 'mt-2 text-xs text-amber-700 dark:text-amber-300',
                'data-test': `preserved-rate-${props.config.groupId}`,
              },
              props.disabledRateHint
            )
          : null,
      ]
    )
  },
})

const props = defineProps<{ show: boolean; user: AdminUser | null }>()
const emit = defineEmits<{
  (event: 'close'): void
  (event: 'success'): void
}>()
const { t } = useI18n()
const appStore = useAppStore()

const groupConfigs = ref<GroupRateConfig[]>([])
const originalGroupRates = ref<Record<number, number>>({})
const accessMode = ref<UserGroupAccessMode>('inherit')
const searchQuery = ref('')
const loading = ref(false)
const submitting = ref(false)
const loadError = ref<string | null>(null)
const showSaveConfirm = ref(false)
const searchInput = ref<HTMLInputElement | null>(null)
let loadSequence = 0

const normalizedSearch = computed(() => searchQuery.value.trim().toLocaleLowerCase())
const filteredGroupConfigs = computed(() => {
  if (!normalizedSearch.value) return groupConfigs.value
  return groupConfigs.value.filter((config) =>
    `${config.groupName} ${config.platform}`.toLocaleLowerCase().includes(normalizedSearch.value)
  )
})
const filteredPublicConfigs = computed(() => filteredGroupConfigs.value.filter((config) => !config.isExclusive))
const filteredExclusiveConfigs = computed(() => filteredGroupConfigs.value.filter((config) => config.isExclusive))
const publicGroupCount = computed(() => groupConfigs.value.filter((config) => !config.isExclusive).length)
const exclusiveGroupCount = computed(() => groupConfigs.value.filter((config) => config.isExclusive).length)
const selectedPublicCount = computed(() => groupConfigs.value.filter((config) => !config.isExclusive && config.isAccessAllowed).length)
const selectedExclusiveCount = computed(() => groupConfigs.value.filter((config) => config.isExclusive && config.isAccessAllowed).length)
const selectedCount = computed(() => groupConfigs.value.filter((config) => config.isAccessAllowed).length)
const invalidRateCount = computed(() => groupConfigs.value.filter((config) => {
  const value = config.customRateInput.trim()
  if (!value) return false
  const parsed = Number(value)
  return !Number.isFinite(parsed) || parsed <= 0
}).length)
const selectionStatus = computed(() => t('admin.users.groupSelectionStatus', {
  selected: selectedCount.value,
  total: groupConfigs.value.length,
  visible: filteredGroupConfigs.value.length,
}))
const saveConfirmationMessage = computed(() => t('admin.users.confirmGroupConfigMessage', {
  mode: t(accessMode.value === 'inherit'
    ? 'admin.users.accessModeInherit'
    : 'admin.users.accessModeRestricted'),
  selected: selectedCount.value,
  total: groupConfigs.value.length,
}))

watch(
  () => [props.show, props.user?.id] as const,
  ([isOpen]) => {
    if (isOpen && props.user) {
      void load()
    } else {
      loadSequence += 1
      showSaveConfirm.value = false
    }
  },
  { immediate: true }
)

function errorMessage(error: unknown, fallback: string): string {
  const candidate = error as {
    response?: { data?: { detail?: string; message?: string } }
    message?: string
  }
  return candidate.response?.data?.detail
    || candidate.response?.data?.message
    || candidate.message
    || fallback
}

async function load() {
  if (!props.user) return
  const sequence = ++loadSequence
  loading.value = true
  loadError.value = null
  showSaveConfirm.value = false

  try {
    const [groupsResponse, config] = await Promise.all([
      adminAPI.groups.list(1, 1000, { status: 'active' }),
      adminAPI.users.getGroupConfig(props.user.id),
    ])
    if (sequence !== loadSequence || !props.show) return

    const groups = groupsResponse.items.filter(
      (group: Group) => group.status === 'active' && group.subscription_type === 'standard'
    )
    const restrictedIds = new Set(config.restricted_group_ids)
    const exclusiveIds = new Set(config.exclusive_group_ids)

    accessMode.value = config.access_mode
    originalGroupRates.value = { ...config.group_rates }
    groupConfigs.value = groups.map((group: Group) => ({
      groupId: group.id,
      groupName: group.name,
      platform: group.platform,
      isExclusive: group.is_exclusive,
      defaultRate: group.rate_multiplier,
      customRateInput: config.group_rates[group.id] === undefined
        ? ''
        : String(config.group_rates[group.id]),
      isAccessAllowed: config.access_mode === 'inherit' || restrictedIds.has(group.id),
      isExclusiveGranted: !group.is_exclusive || exclusiveIds.has(group.id),
    }))
    searchQuery.value = ''
    await nextTick()
    searchInput.value?.focus()
  } catch (error) {
    if (sequence !== loadSequence || !props.show) return
    const message = errorMessage(error, t('admin.users.groupConfigLoadFailed'))
    loadError.value = message
    appStore.showError(message)
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

function setAccessMode(mode: UserGroupAccessMode) {
  if (mode === accessMode.value) return
  if (mode === 'inherit' || accessMode.value === 'inherit') {
    for (const config of groupConfigs.value) {
      config.isAccessAllowed = true
    }
  }
  accessMode.value = mode
}

function toggleGroupAccess(config: GroupRateConfig, checked: boolean) {
  if (accessMode.value !== 'restricted') return
  config.isAccessAllowed = checked
}

function toggleExclusiveGrant(config: GroupRateConfig, checked: boolean) {
  if (!config.isExclusive) return
  config.isExclusiveGranted = checked
}

function setVisibleSelection(selected: boolean) {
  if (accessMode.value !== 'restricted') return
  for (const config of filteredGroupConfigs.value) {
    config.isAccessAllowed = selected
  }
}

function allowAllAndInherit() {
  for (const config of groupConfigs.value) {
    config.isAccessAllowed = true
  }
  accessMode.value = 'inherit'
}

function canEditRate(config: GroupRateConfig): boolean {
  return config.isAccessAllowed && (!config.isExclusive || config.isExclusiveGranted)
}

function buildPayload(): UpdateUserGroupConfigRequest {
  const groupRates: Record<number, number | null> = {}
  for (const config of groupConfigs.value) {
    const value = config.customRateInput.trim()
    if (value) {
      groupRates[config.groupId] = Number(value)
    } else if (Object.prototype.hasOwnProperty.call(originalGroupRates.value, config.groupId)) {
      groupRates[config.groupId] = null
    }
  }

  return {
    access_mode: accessMode.value,
    restricted_group_ids: accessMode.value === 'restricted'
      ? groupConfigs.value
          .filter((config) => config.isAccessAllowed)
          .map((config) => config.groupId)
      : [],
    exclusive_group_ids: groupConfigs.value
      .filter((config) => config.isExclusive && config.isExclusiveGranted)
      .map((config) => config.groupId),
    group_rates: groupRates,
  }
}

async function handleSave() {
  if (!props.user || submitting.value || invalidRateCount.value > 0) return
  submitting.value = true
  showSaveConfirm.value = false
  try {
    await adminAPI.users.updateGroupConfig(props.user.id, buildPayload())
    appStore.showSuccess(t('admin.users.groupConfigUpdated'))
    emit('success')
    emit('close')
  } catch (error) {
    const message = errorMessage(error, t('admin.users.groupConfigSaveFailed'))
    appStore.showError(message)
  } finally {
    submitting.value = false
  }
}

function handleClose() {
  showSaveConfirm.value = false
  emit('close')
}
</script>

<style scoped>
:deep(.hide-spinner::-webkit-outer-spin-button),
:deep(.hide-spinner::-webkit-inner-spin-button) {
  -webkit-appearance: none;
  margin: 0;
}

:deep(.hide-spinner) {
  -moz-appearance: textfield;
}
</style>

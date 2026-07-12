<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <!-- Left: Search + Filters -->
          <div class="flex-1 sm:max-w-64">
            <input
              v-model="searchQuery"
              type="text"
              :placeholder="t('admin.announcements.searchAnnouncements')"
              class="input"
              @input="handleSearch"
            />
          </div>
          <Select
            v-model="filters.status"
            :options="statusFilterOptions"
            class="w-40"
            @change="handleStatusChange"
          />

          <!-- Right: Action buttons -->
          <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
            <button
              @click="loadAnnouncements"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button @click="openCreateDialog" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-1" />
              {{ t('admin.announcements.createAnnouncement') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="announcements"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-title="{ value, row }">
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <span class="truncate font-medium text-gray-900 dark:text-white">{{ value }}</span>
              </div>
              <div class="mt-1 flex items-center gap-2 text-xs text-gray-500 dark:text-dark-400">
                <span>#{{ row.id }}</span>
                <span class="text-gray-300 dark:text-dark-700">·</span>
                <span>{{ formatDateTime(row.created_at) }}</span>
              </div>
            </div>
          </template>

          <template #cell-status="{ value }">
            <span
              :class="[
                'badge',
                value === 'active'
                  ? 'badge-success'
                  : value === 'draft'
                    ? 'badge-gray'
                    : 'badge-warning'
              ]"
            >
              {{ statusLabel(value) }}
            </span>
          </template>

          <template #cell-notify_mode="{ row }">
            <span
              :class="[
                'badge',
                row.notify_mode === 'popup'
                  ? 'badge-warning'
                  : 'badge-gray'
              ]"
            >
              {{ row.notify_mode === 'popup' ? t('admin.announcements.notifyModeLabels.popup') : t('admin.announcements.notifyModeLabels.silent') }}
            </span>
          </template>

          <template #cell-targeting="{ row }">
            <span class="text-sm text-gray-600 dark:text-gray-300">
              {{ targetingSummary(row.targeting) }}
            </span>
          </template>

          <template #cell-email_notification="{ row }">
            <button
              type="button"
              class="min-h-10 min-w-24 rounded-lg px-2 py-1.5 text-left transition-colors hover:bg-gray-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 dark:hover:bg-dark-700"
              :class="row.email_notification ? 'cursor-pointer' : 'cursor-default'"
              :disabled="!row.email_notification"
              :aria-label="t('admin.announcements.email.openDetailsFor', { title: row.title })"
              @click="row.email_notification && openEmailStatus(row)"
            >
              <span :class="['badge', emailStatusBadgeClass(row.email_notification?.status)]">
                {{ t(announcementEmailStatusLabelKey(row.email_notification?.status)) }}
              </span>
              <span
                v-if="row.email_notification && ['pending', 'preparing', 'sending'].includes(row.email_notification.status)"
                class="mt-1 block font-mono text-xs tabular-nums text-gray-500 dark:text-dark-400"
              >
                {{ row.email_notification.sent_count }}/{{ row.email_notification.total_count }}
              </span>
            </button>
          </template>

          <template #cell-timeRange="{ row }">
            <div class="text-sm text-gray-600 dark:text-gray-300">
              <div>
                <span class="font-medium">{{ t('admin.announcements.form.startsAt') }}:</span>
                <span class="ml-1">{{ row.starts_at ? formatDateTime(row.starts_at) : t('admin.announcements.timeImmediate') }}</span>
              </div>
              <div class="mt-0.5">
                <span class="font-medium">{{ t('admin.announcements.form.endsAt') }}:</span>
                <span class="ml-1">{{ row.ends_at ? formatDateTime(row.ends_at) : t('admin.announcements.timeNever') }}</span>
              </div>
            </div>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatDateTime(value) }}</span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center space-x-1">
              <button
                @click="openReadStatus(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
                :title="t('admin.announcements.readStatus')"
              >
                <Icon name="eye" size="sm" />
              </button>
              <button
                @click="openEditDialog(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-600 dark:hover:text-gray-300"
                :title="t('common.edit')"
              >
                <Icon name="edit" size="sm" />
              </button>
              <button
                @click="handleDelete(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                :title="t('common.delete')"
              >
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('empty.noData')"
              :description="t('admin.announcements.failedToLoad')"
              :action-text="t('admin.announcements.createAnnouncement')"
              @action="openCreateDialog"
            />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <!-- Create/Edit Dialog -->
    <BaseDialog
      :show="showEditDialog"
      :title="isEditing ? t('admin.announcements.editAnnouncement') : t('admin.announcements.createAnnouncement')"
      width="wide"
      @close="closeEdit"
    >
      <form id="announcement-form" @submit.prevent="handleSave" class="space-y-4">
        <div>
          <label class="input-label">{{ t('admin.announcements.form.title') }}</label>
          <input v-model="form.title" type="text" class="input" required />
        </div>

        <div>
          <label class="input-label">{{ t('admin.announcements.form.content') }}</label>
          <textarea v-model="form.content" rows="6" class="input" required></textarea>
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.announcements.form.status') }}</label>
            <Select v-model="form.status" :options="statusOptions" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.announcements.form.notifyMode') }}</label>
            <Select v-model="form.notify_mode" :options="notifyModeOptions" />
            <p class="input-hint">{{ t('admin.announcements.form.notifyModeHint') }}</p>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.announcements.form.startsAt') }}</label>
            <input v-model="form.starts_at_str" type="datetime-local" class="input" />
            <p class="input-hint">{{ t('admin.announcements.form.startsAtHint') }}</p>
          </div>
          <div>
            <label class="input-label">{{ t('admin.announcements.form.endsAt') }}</label>
            <input v-model="form.ends_at_str" type="datetime-local" class="input" />
            <p class="input-hint">{{ t('admin.announcements.form.endsAtHint') }}</p>
          </div>
        </div>

        <AnnouncementTargetingEditor
          v-model="form.targeting"
          :groups="subscriptionGroups"
        />

        <section
          class="rounded-lg border border-amber-200 bg-amber-50/80 p-4 dark:border-amber-900/60 dark:bg-amber-950/20"
          aria-labelledby="announcement-email-title"
        >
          <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div class="min-w-0">
              <div id="announcement-email-title" class="flex items-center gap-2 text-sm font-semibold text-amber-950 dark:text-amber-100">
                <Icon name="mail" size="sm" />
                {{ t('admin.announcements.email.sendTitle') }}
              </div>
              <p class="mt-1 text-sm leading-6 text-amber-900/80 dark:text-amber-200/80">
                {{ t('admin.announcements.email.sendDescription') }}
              </p>
            </div>
            <label class="inline-flex min-h-11 shrink-0 items-center gap-3 self-start rounded-lg px-1 focus-within:ring-2 focus-within:ring-amber-500/40">
              <span class="text-sm font-medium text-amber-950 dark:text-amber-100">{{ t('admin.announcements.email.sendToggle') }}</span>
              <input
                v-model="form.send_email"
                type="checkbox"
                class="h-5 w-5 rounded border-amber-400 text-amber-600 focus:ring-amber-500"
                :disabled="!canEnableEmail"
                :aria-describedby="emailCapabilityHelpId"
              />
            </label>
          </div>

          <div :id="emailCapabilityHelpId" class="mt-3 rounded-md border border-amber-200/80 bg-white/60 p-3 text-sm text-amber-950 dark:border-amber-900/60 dark:bg-dark-900/30 dark:text-amber-100">
            <div v-if="emailCapabilityLoading" class="text-amber-800 dark:text-amber-300">
              {{ t('admin.announcements.email.loadingCapability') }}
            </div>
            <div v-else-if="emailDisabledReason" class="font-medium text-amber-900 dark:text-amber-200">
              {{ emailDisabledReason }}
            </div>
            <div v-else class="font-medium">
              {{ t('admin.announcements.email.estimatedRecipients', { count: emailCapability?.eligible_count ?? 0 }) }}
            </div>
            <ul class="mt-2 list-disc space-y-1 pl-5 text-xs leading-5 text-amber-900/80 dark:text-amber-200/80">
              <li>{{ t('admin.announcements.email.primaryEmailOnly') }}</li>
              <li>{{ t('admin.announcements.email.ignoresTargeting') }}</li>
              <li>{{ form.starts_at_str ? t('admin.announcements.email.scheduledAtStarts') : t('admin.announcements.email.startsImmediately') }}</li>
              <li>{{ t('admin.announcements.email.onceOnly') }}</li>
            </ul>
          </div>
        </section>
      </form>

      <template #footer>
        <div class="flex w-full flex-col-reverse gap-3 sm:flex-row sm:justify-end">
          <button type="button" @click="closeEdit" class="btn btn-secondary min-h-11 w-full sm:w-auto">
            {{ t('common.cancel') }}
          </button>
          <button type="submit" form="announcement-form" :disabled="saving" class="btn btn-primary min-h-11 w-full sm:w-auto">
            {{ saving ? t('common.saving') : form.send_email ? t('admin.announcements.email.publishAndQueue') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Delete Confirmation -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.announcements.deleteAnnouncement')"
      :message="t('admin.announcements.deleteConfirm')"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />

    <!-- Read Status Dialog -->
    <AnnouncementReadStatusDialog
      :show="showReadStatusDialog"
      :announcement-id="readStatusAnnouncementId"
      @close="showReadStatusDialog = false"
    />

    <ConfirmDialog
      :show="showEmailSendConfirm"
      :title="t('admin.announcements.email.confirmTitle')"
      :message="t('admin.announcements.email.confirmMessage', { count: emailCapability?.eligible_count ?? 0 })"
      :confirm-text="t('admin.announcements.email.confirmSend')"
      :cancel-text="t('admin.announcements.email.continueEditing')"
      @confirm="confirmEmailSend"
      @cancel="showEmailSendConfirm = false"
    />

    <AnnouncementEmailStatusDialog
      :show="showEmailStatusDialog"
      :notification="selectedEmailNotification"
      :loading="emailStatusLoading"
      :retrying="emailRetrying"
      @close="closeEmailStatus"
      @refresh="refreshSelectedEmailStatus"
      @retry="requestEmailRetry"
    />

    <ConfirmDialog
      :show="showEmailRetryConfirm"
      :title="pendingRetryIncludeAmbiguous ? t('admin.announcements.email.retryAmbiguousConfirmTitle') : t('admin.announcements.email.retryConfirmTitle')"
      :message="pendingRetryIncludeAmbiguous ? t('admin.announcements.email.retryAmbiguousConfirmMessage') : t('admin.announcements.email.retryConfirmMessage')"
      :confirm-text="pendingRetryIncludeAmbiguous ? t('admin.announcements.email.retryIncludingAmbiguous') : t('admin.announcements.email.retryFailed')"
      :cancel-text="t('common.cancel')"
      :danger="pendingRetryIncludeAmbiguous"
      @confirm="confirmEmailRetry"
      @cancel="cancelEmailRetry"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { adminAPI } from '@/api/admin'
import { formatDateTime, formatDateTimeLocalInput, parseDateTimeLocalInput } from '@/utils/format'
import type {
  AdminGroup,
  Announcement,
  AnnouncementEmailCapability,
  AnnouncementEmailJobStatus,
  AnnouncementEmailNotificationStatus,
  AnnouncementTargeting
} from '@/types'
import type { Column } from '@/components/common/types'

import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select from '@/components/common/Select.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'

import AnnouncementTargetingEditor from '@/components/admin/announcements/AnnouncementTargetingEditor.vue'
import AnnouncementReadStatusDialog from '@/components/admin/announcements/AnnouncementReadStatusDialog.vue'
import AnnouncementEmailStatusDialog from '@/components/admin/announcements/AnnouncementEmailStatusDialog.vue'
import {
  announcementEmailStatusLabelKey,
  createAnnouncementIdempotencyKey,
  isActiveAnnouncementEmailStatus,
  requiresAnnouncementEmailConfirmation,
  shouldPollAnnouncementEmails
} from './announcementEmailNotification'

const { t } = useI18n()
const appStore = useAppStore()

const announcements = ref<Announcement[]>([])
const loading = ref(false)

const filters = reactive({
  status: '',
})
const searchQuery = ref('')

const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
})

const sortState = reactive({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc'
})

const statusFilterOptions = computed(() => [
  { value: '', label: t('admin.announcements.allStatus') },
  { value: 'draft', label: t('admin.announcements.statusLabels.draft') },
  { value: 'active', label: t('admin.announcements.statusLabels.active') },
  { value: 'archived', label: t('admin.announcements.statusLabels.archived') }
])

const statusOptions = computed(() => [
  { value: 'draft', label: t('admin.announcements.statusLabels.draft') },
  { value: 'active', label: t('admin.announcements.statusLabels.active') },
  { value: 'archived', label: t('admin.announcements.statusLabels.archived') }
])

const notifyModeOptions = computed(() => [
  { value: 'silent', label: t('admin.announcements.notifyModeLabels.silent') },
  { value: 'popup', label: t('admin.announcements.notifyModeLabels.popup') }
])

const columns = computed<Column[]>(() => [
  { key: 'title', label: t('admin.announcements.columns.title'), sortable: true },
  { key: 'status', label: t('admin.announcements.columns.status'), sortable: true },
  { key: 'notify_mode', label: t('admin.announcements.columns.notifyMode'), sortable: true },
  { key: 'targeting', label: t('admin.announcements.columns.targeting') },
  { key: 'email_notification', label: t('admin.announcements.columns.emailStatus') },
  { key: 'timeRange', label: t('admin.announcements.columns.timeRange') },
  { key: 'created_at', label: t('admin.announcements.columns.createdAt'), sortable: true },
  { key: 'actions', label: t('admin.announcements.columns.actions') }
])

const statusLabel = (status: string) => {
  if (status === 'draft') return t('admin.announcements.statusLabels.draft')
  if (status === 'active') return t('admin.announcements.statusLabels.active')
  if (status === 'archived') return t('admin.announcements.statusLabels.archived')
  return status
}

const emailStatusBadgeClass = (status?: AnnouncementEmailJobStatus | null) => {
  if (status === 'completed') return 'badge-success'
  if (status === 'failed' || status === 'completed_with_failures') return 'badge-danger'
  if (status === 'cancelled' || !status) return 'badge-gray'
  return 'badge-warning'
}

const targetingSummary = (targeting: AnnouncementTargeting) => {
  const anyOf = targeting?.any_of ?? []
  if (!anyOf || anyOf.length === 0) return t('admin.announcements.targetingSummaryAll')
  return t('admin.announcements.targetingSummaryCustom', { groups: anyOf.length })
}

// ===== CRUD / list =====
let currentController: AbortController | null = null

async function loadAnnouncements() {
  currentController?.abort()
  const requestController = new AbortController()
  currentController = requestController
  const { signal } = requestController

  try {
    loading.value = true
    const res = await adminAPI.announcements.list(pagination.page, pagination.page_size, {
      status: filters.status || undefined,
      search: searchQuery.value || undefined,
      sort_by: sortState.sort_by,
      sort_order: sortState.sort_order
    }, { signal })

    if (signal.aborted || currentController !== requestController) return

    announcements.value = res.items
    pagination.total = res.total
    pagination.pages = res.pages
    pagination.page = res.page
    pagination.page_size = res.page_size
    syncEmailPolling()
  } catch (error: any) {
    if (
      signal.aborted ||
      currentController !== requestController ||
      error?.name === 'AbortError' ||
      error?.code === 'ERR_CANCELED'
    ) {
      return
    }
    console.error('Error loading announcements:', error)
    appStore.showError(error.response?.data?.detail || t('admin.announcements.failedToLoad'))
  } finally {
    if (currentController === requestController) {
      loading.value = false
      currentController = null
    }
  }
}

function handlePageChange(page: number) {
  pagination.page = page
  loadAnnouncements()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  loadAnnouncements()
}

function handleStatusChange() {
  pagination.page = 1
  loadAnnouncements()
}

function handleSort(key: string, order: 'asc' | 'desc') {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  loadAnnouncements()
}

let searchDebounceTimer: number | null = null
function handleSearch() {
  if (searchDebounceTimer) window.clearTimeout(searchDebounceTimer)
  searchDebounceTimer = window.setTimeout(() => {
    pagination.page = 1
    loadAnnouncements()
  }, 300)
}

// ===== Create/Edit dialog =====
const showEditDialog = ref(false)
const saving = ref(false)
const editingAnnouncement = ref<Announcement | null>(null)
const showEmailSendConfirm = ref(false)
const emailCapability = ref<AnnouncementEmailCapability | null>(null)
const emailCapabilityLoading = ref(false)
const announcementIdempotencyKey = ref<string | null>(null)
const emailCapabilityHelpId = 'announcement-email-capability-help'

const isEditing = computed(() => !!editingAnnouncement.value)

const emailDisabledReason = computed(() => {
  if (emailCapabilityLoading.value) return ''
  if (!emailCapability.value?.enabled) return t('admin.announcements.email.featureDisabled')
  if (!emailCapability.value.smtp_configured) return t('admin.announcements.email.smtpNotConfigured')
  if (form.status !== 'active') return t('admin.announcements.email.activeOnly')
  if (editingAnnouncement.value?.email_notification) return t('admin.announcements.email.alreadyQueued')
  return ''
})

const canEnableEmail = computed(() => !emailCapabilityLoading.value && !emailDisabledReason.value)

const form = reactive({
  title: '',
  content: '',
  status: 'draft',
  notify_mode: 'silent',
  starts_at_str: '',
  ends_at_str: '',
  targeting: { any_of: [] } as AnnouncementTargeting,
  send_email: false
})

const subscriptionGroups = ref<AdminGroup[]>([])

async function loadSubscriptionGroups() {
  try {
    const all = await adminAPI.groups.getAll()
    subscriptionGroups.value = (all || []).filter((g) => g.subscription_type === 'subscription')
  } catch (error: any) {
    console.error('Error loading groups:', error)
    // not fatal
  }
}

async function loadEmailCapability() {
  emailCapabilityLoading.value = true
  try {
    emailCapability.value = await adminAPI.announcements.getEmailCapability()
  } catch (error: any) {
    console.error('Failed to load announcement email capability:', error)
    emailCapability.value = null
    appStore.showError(error.response?.data?.detail || t('admin.announcements.email.failedToLoadCapability'))
  } finally {
    emailCapabilityLoading.value = false
  }
}

function resetForm() {
  form.title = ''
  form.content = ''
  form.status = 'draft'
  form.notify_mode = 'silent'
  form.starts_at_str = ''
  form.ends_at_str = ''
  form.targeting = { any_of: [] }
  form.send_email = false
}

function fillFormFromAnnouncement(a: Announcement) {
  form.title = a.title
  form.content = a.content
  form.status = a.status
  form.notify_mode = a.notify_mode || 'silent'

  // Backend returns RFC3339 strings
  form.starts_at_str = a.starts_at ? formatDateTimeLocalInput(Math.floor(new Date(a.starts_at).getTime() / 1000)) : ''
  form.ends_at_str = a.ends_at ? formatDateTimeLocalInput(Math.floor(new Date(a.ends_at).getTime() / 1000)) : ''

  form.targeting = a.targeting ?? { any_of: [] }
  form.send_email = false
}

function openCreateDialog() {
  editingAnnouncement.value = null
  resetForm()
  announcementIdempotencyKey.value = createAnnouncementIdempotencyKey()
  showEditDialog.value = true
  if (!emailCapability.value) void loadEmailCapability()
}

function openEditDialog(row: Announcement) {
  editingAnnouncement.value = row
  fillFormFromAnnouncement(row)
  announcementIdempotencyKey.value = createAnnouncementIdempotencyKey()
  showEditDialog.value = true
  if (!emailCapability.value) void loadEmailCapability()
}

function closeEdit() {
  showEditDialog.value = false
  showEmailSendConfirm.value = false
  editingAnnouncement.value = null
  announcementIdempotencyKey.value = null
  form.send_email = false
}

function buildCreatePayload() {
  const startsAt = parseDateTimeLocalInput(form.starts_at_str)
  const endsAt = parseDateTimeLocalInput(form.ends_at_str)

  return {
    title: form.title,
    content: form.content,
    status: form.status as any,
    notify_mode: form.notify_mode as any,
    targeting: form.targeting,
    starts_at: startsAt ?? undefined,
    ends_at: endsAt ?? undefined,
    send_email: form.send_email
  }
}

function buildUpdatePayload(original: Announcement) {
  const payload: any = {}

  if (form.title !== original.title) payload.title = form.title
  if (form.content !== original.content) payload.content = form.content
  if (form.status !== original.status) payload.status = form.status
  if (form.notify_mode !== (original.notify_mode || 'silent')) payload.notify_mode = form.notify_mode

  // starts_at / ends_at: distinguish unchanged vs clear(0) vs set
  const originalStarts = original.starts_at ? Math.floor(new Date(original.starts_at).getTime() / 1000) : null
  const originalEnds = original.ends_at ? Math.floor(new Date(original.ends_at).getTime() / 1000) : null

  const newStarts = parseDateTimeLocalInput(form.starts_at_str)
  const newEnds = parseDateTimeLocalInput(form.ends_at_str)

  if (newStarts !== originalStarts) {
    payload.starts_at = newStarts === null ? 0 : newStarts
  }
  if (newEnds !== originalEnds) {
    payload.ends_at = newEnds === null ? 0 : newEnds
  }

  // targeting: do shallow compare by JSON
  if (JSON.stringify(form.targeting ?? {}) !== JSON.stringify(original.targeting ?? {})) {
    payload.targeting = form.targeting
  }
  if (form.send_email) payload.send_email = true

  return payload
}

function validateAnnouncementForm(): boolean {
  const anyOf = form.targeting?.any_of ?? []
  if (anyOf.length > 50 || anyOf.some((group) => (group?.all_of ?? []).length > 50)) {
    appStore.showError(t('admin.announcements.failedToCreate'))
    return false
  }
  if (form.send_email && !canEnableEmail.value) {
    form.send_email = false
    appStore.showError(emailDisabledReason.value || t('admin.announcements.email.unavailable'))
    return false
  }
  return true
}

async function handleSave() {
  if (!validateAnnouncementForm()) return
  if (requiresAnnouncementEmailConfirmation(form.send_email)) {
    showEmailSendConfirm.value = true
    return
  }
  await saveAnnouncement()
}

async function confirmEmailSend() {
  showEmailSendConfirm.value = false
  await saveAnnouncement()
}

async function saveAnnouncement() {
  const requestedEmail = form.send_email
  const idempotencyKey = announcementIdempotencyKey.value || createAnnouncementIdempotencyKey()
  announcementIdempotencyKey.value = idempotencyKey
  saving.value = true
  try {
    if (!editingAnnouncement.value) {
      const payload = buildCreatePayload()
      await adminAPI.announcements.create(payload, requestedEmail ? idempotencyKey : undefined)
    } else {
      const original = editingAnnouncement.value
      const payload = buildUpdatePayload(original)
      await adminAPI.announcements.update(original.id, payload, requestedEmail ? idempotencyKey : undefined)
    }

    appStore.showSuccess(requestedEmail ? t('admin.announcements.email.queuedSuccess') : t('common.success'))
    showEditDialog.value = false
    editingAnnouncement.value = null
    announcementIdempotencyKey.value = null
    form.send_email = false
    await loadAnnouncements()
  } catch (error: any) {
    console.error('Failed to save announcement:', error)
    appStore.showError(error.response?.data?.detail || (editingAnnouncement.value ? t('admin.announcements.failedToUpdate') : t('admin.announcements.failedToCreate')))
  } finally {
    saving.value = false
  }
}

// ===== Delete =====
const showDeleteDialog = ref(false)
const deletingAnnouncement = ref<Announcement | null>(null)

function handleDelete(row: Announcement) {
  deletingAnnouncement.value = row
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!deletingAnnouncement.value) return

  try {
    await adminAPI.announcements.delete(deletingAnnouncement.value.id)
    appStore.showSuccess(t('common.success'))
    showDeleteDialog.value = false
    deletingAnnouncement.value = null
    await loadAnnouncements()
  } catch (error: any) {
    console.error('Failed to delete announcement:', error)
    appStore.showError(error.response?.data?.detail || t('admin.announcements.failedToDelete'))
  }
}

// ===== Read status =====
const showReadStatusDialog = ref(false)
const readStatusAnnouncementId = ref<number | null>(null)

function openReadStatus(row: Announcement) {
  readStatusAnnouncementId.value = row.id
  showReadStatusDialog.value = true
}

// ===== Email notification status / retry / polling =====
const showEmailStatusDialog = ref(false)
const selectedEmailAnnouncementId = ref<number | null>(null)
const selectedEmailNotification = ref<AnnouncementEmailNotificationStatus | null>(null)
const emailStatusLoading = ref(false)
const emailRetrying = ref(false)
const showEmailRetryConfirm = ref(false)
const pendingRetryIncludeAmbiguous = ref(false)
const retryIdempotencyKey = ref<string | null>(null)
let emailPollTimer: number | null = null
let emailStatusController: AbortController | null = null
let emailPollingDisposed = false

function patchEmailStatus(id: number, notification: AnnouncementEmailNotificationStatus) {
  const item = announcements.value.find((announcement) => announcement.id === id)
  if (item) item.email_notification = notification
  if (selectedEmailAnnouncementId.value === id) selectedEmailNotification.value = notification
}

async function openEmailStatus(row: Announcement) {
  selectedEmailAnnouncementId.value = row.id
  selectedEmailNotification.value = row.email_notification || null
  showEmailStatusDialog.value = true
  await refreshSelectedEmailStatus()
}

function closeEmailStatus() {
  showEmailStatusDialog.value = false
  selectedEmailAnnouncementId.value = null
  selectedEmailNotification.value = null
  cancelEmailRetry()
}

async function refreshSelectedEmailStatus() {
  if (!selectedEmailAnnouncementId.value) return
  emailStatusLoading.value = true
  try {
    const status = await adminAPI.announcements.getEmailStatus(selectedEmailAnnouncementId.value)
    patchEmailStatus(selectedEmailAnnouncementId.value, status)
    syncEmailPolling()
  } catch (error: any) {
    if (error?.name !== 'AbortError' && error?.code !== 'ERR_CANCELED') {
      appStore.showError(error.response?.data?.detail || t('admin.announcements.email.failedToLoadStatus'))
    }
  } finally {
    emailStatusLoading.value = false
  }
}

function requestEmailRetry(includeAmbiguous: boolean) {
  pendingRetryIncludeAmbiguous.value = includeAmbiguous
  retryIdempotencyKey.value = retryIdempotencyKey.value || createAnnouncementIdempotencyKey()
  showEmailRetryConfirm.value = true
}

function cancelEmailRetry() {
  showEmailRetryConfirm.value = false
  pendingRetryIncludeAmbiguous.value = false
  retryIdempotencyKey.value = null
}

async function confirmEmailRetry() {
  if (!selectedEmailAnnouncementId.value) return
  const announcementId = selectedEmailAnnouncementId.value
  const includeAmbiguous = pendingRetryIncludeAmbiguous.value
  const idempotencyKey = retryIdempotencyKey.value || createAnnouncementIdempotencyKey()
  retryIdempotencyKey.value = idempotencyKey
  showEmailRetryConfirm.value = false
  emailRetrying.value = true
  try {
    const result = await adminAPI.announcements.retryEmailNotification(
      announcementId,
      { include_ambiguous: includeAmbiguous },
      idempotencyKey
    )
    patchEmailStatus(announcementId, result.email_notification)
    retryIdempotencyKey.value = null
    pendingRetryIncludeAmbiguous.value = false
    appStore.showSuccess(t('admin.announcements.email.retryQueuedSuccess'))
    syncEmailPolling()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.announcements.email.failedToRetry'))
  } finally {
    emailRetrying.value = false
  }
}

function stopEmailPolling() {
  if (emailPollTimer !== null) {
    window.clearTimeout(emailPollTimer)
    emailPollTimer = null
  }
}

function syncEmailPolling() {
  stopEmailPolling()
  if (emailPollingDisposed || !shouldPollAnnouncementEmails(announcements.value)) return
  emailPollTimer = window.setTimeout(() => void pollEmailStatuses(), 5000)
}

async function pollEmailStatuses() {
  stopEmailPolling()
  if (emailPollingDisposed) return
  emailStatusController?.abort()
  const controller = new AbortController()
  emailStatusController = controller
  const activeAnnouncements = announcements.value.filter((announcement) =>
    isActiveAnnouncementEmailStatus(announcement.email_notification?.status)
  )
  if (activeAnnouncements.length === 0) {
    emailStatusController = null
    return
  }

  try {
    const results = await Promise.allSettled(
      activeAnnouncements.map(async (announcement) => ({
        id: announcement.id,
        status: await adminAPI.announcements.getEmailStatus(announcement.id, { signal: controller.signal })
      }))
    )
    if (controller.signal.aborted) return
    for (const result of results) {
      if (result.status === 'fulfilled') patchEmailStatus(result.value.id, result.value.status)
    }
  } finally {
    if (emailStatusController === controller) emailStatusController = null
    syncEmailPolling()
  }
}

watch(
  () => form.status,
  (status) => {
    if (status !== 'active') form.send_email = false
  }
)

onMounted(async () => {
  emailPollingDisposed = false
  await Promise.all([loadSubscriptionGroups(), loadEmailCapability()])
  await loadAnnouncements()
})

onUnmounted(() => {
  emailPollingDisposed = true
  if (searchDebounceTimer) window.clearTimeout(searchDebounceTimer)
  stopEmailPolling()
  emailStatusController?.abort()
  currentController?.abort()
})
</script>

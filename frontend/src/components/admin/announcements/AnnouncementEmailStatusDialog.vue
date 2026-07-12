<template>
  <BaseDialog
    :show="show"
    :title="t('admin.announcements.email.detailsTitle')"
    width="wide"
    @close="$emit('close')"
  >
    <div v-if="loading && !notification" class="flex min-h-32 items-center justify-center text-gray-500">
      {{ t('common.loading') }}
    </div>
    <div v-else-if="notification" class="space-y-5">
      <div class="flex flex-col gap-3 rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/60 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-dark-400">
            {{ t('admin.announcements.email.deliveryStatus') }}
          </div>
          <div class="mt-1 flex items-center gap-2">
            <span :class="['badge', statusBadgeClass(notification.status)]">
              {{ t(announcementEmailStatusLabelKey(notification.status)) }}
            </span>
            <span v-if="isActiveAnnouncementEmailStatus(notification.status)" class="text-xs text-gray-500 dark:text-dark-400">
              {{ t('admin.announcements.email.autoRefreshing') }}
            </span>
          </div>
        </div>
        <button
          type="button"
          class="btn btn-secondary btn-sm min-h-10"
          :disabled="loading"
          :aria-label="t('admin.announcements.email.refreshStatus')"
          @click="$emit('refresh')"
        >
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
          <span class="ml-1">{{ t('admin.announcements.email.refreshStatus') }}</span>
        </button>
      </div>

      <dl class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        <div v-for="metric in metrics" :key="metric.key" class="rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800">
          <dt class="text-xs text-gray-500 dark:text-dark-400">{{ metric.label }}</dt>
          <dd class="mt-1 font-mono text-lg font-semibold tabular-nums text-gray-900 dark:text-white">{{ metric.value }}</dd>
        </div>
      </dl>

      <div class="grid grid-cols-1 gap-3 text-sm sm:grid-cols-3">
        <div v-for="time in times" :key="time.key">
          <div class="text-xs text-gray-500 dark:text-dark-400">{{ time.label }}</div>
          <div class="mt-1 text-gray-800 dark:text-gray-200">{{ time.value }}</div>
        </div>
      </div>

      <div
        v-if="notification.last_error_code || notification.last_error_message"
        class="rounded-lg border border-red-200 bg-red-50 p-4 dark:border-red-900/60 dark:bg-red-950/20"
      >
        <div class="text-sm font-medium text-red-800 dark:text-red-300">
          {{ t('admin.announcements.email.errorSummary') }}
        </div>
        <div v-if="notification.last_error_code" class="mt-2 font-mono text-xs text-red-700 dark:text-red-300">
          {{ notification.last_error_code }}
        </div>
        <p v-if="notification.last_error_message" class="mt-1 break-words text-sm text-red-700 dark:text-red-300">
          {{ notification.last_error_message }}
        </p>
      </div>

      <div v-if="notification.can_retry" class="rounded-lg border border-amber-200 bg-amber-50 p-4 dark:border-amber-900/60 dark:bg-amber-950/20">
        <div class="text-sm font-medium text-amber-900 dark:text-amber-200">
          {{ t('admin.announcements.email.retryTitle') }}
        </div>
        <p class="mt-1 text-sm leading-6 text-amber-800 dark:text-amber-300">
          {{ t('admin.announcements.email.retryHint') }}
        </p>
        <div class="mt-3 flex flex-col gap-2 sm:flex-row sm:flex-wrap">
          <button
            type="button"
            class="btn btn-secondary min-h-10"
            :disabled="retrying || notification.failed_count === 0"
            @click="$emit('retry', false)"
          >
            {{ t('admin.announcements.email.retryFailed') }}
          </button>
          <button
            v-if="notification.ambiguous_count > 0"
            type="button"
            class="btn min-h-10 border border-amber-500 bg-amber-600 text-white hover:bg-amber-700 focus:ring-amber-500"
            :disabled="retrying"
            @click="$emit('retry', true)"
          >
            {{ t('admin.announcements.email.retryIncludingAmbiguous') }}
          </button>
        </div>
      </div>
    </div>
    <div v-else class="rounded-lg border border-gray-200 bg-gray-50 p-4 text-sm text-gray-600 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300">
      {{ t('admin.announcements.email.noStatus') }}
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button type="button" class="btn btn-secondary min-h-10" @click="$emit('close')">
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AnnouncementEmailNotificationStatus } from '@/types'
import { formatDateTime } from '@/utils/format'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  announcementEmailStatusLabelKey,
  isActiveAnnouncementEmailStatus
} from '@/views/admin/announcementEmailNotification'

const props = defineProps<{
  show: boolean
  notification: AnnouncementEmailNotificationStatus | null
  loading?: boolean
  retrying?: boolean
}>()

defineEmits<{
  (event: 'close'): void
  (event: 'refresh'): void
  (event: 'retry', includeAmbiguous: boolean): void
}>()

const { t } = useI18n()

const metrics = computed(() => {
  const item = props.notification
  return [
    { key: 'total', label: t('admin.announcements.email.metrics.total'), value: item?.total_count ?? 0 },
    { key: 'sent', label: t('admin.announcements.email.metrics.sent'), value: item?.sent_count ?? 0 },
    { key: 'failed', label: t('admin.announcements.email.metrics.failed'), value: item?.failed_count ?? 0 },
    { key: 'ambiguous', label: t('admin.announcements.email.metrics.ambiguous'), value: item?.ambiguous_count ?? 0 },
    { key: 'skipped', label: t('admin.announcements.email.metrics.skipped'), value: item?.skipped_count ?? 0 },
    { key: 'remaining', label: t('admin.announcements.email.metrics.remaining'), value: Math.max(0, (item?.total_count ?? 0) - (item?.sent_count ?? 0) - (item?.failed_count ?? 0) - (item?.ambiguous_count ?? 0) - (item?.skipped_count ?? 0)) }
  ]
})

const times = computed(() => {
  const item = props.notification
  const display = (value?: string | null) => value ? formatDateTime(value) : t('common.notAvailable')
  return [
    { key: 'available', label: t('admin.announcements.email.availableAt'), value: display(item?.available_at) },
    { key: 'started', label: t('admin.announcements.email.startedAt'), value: display(item?.started_at) },
    { key: 'finished', label: t('admin.announcements.email.finishedAt'), value: display(item?.finished_at) }
  ]
})

function statusBadgeClass(status: AnnouncementEmailNotificationStatus['status']): string {
  if (status === 'completed') return 'badge-success'
  if (status === 'failed' || status === 'completed_with_failures') return 'badge-danger'
  if (status === 'cancelled') return 'badge-gray'
  return 'badge-warning'
}
</script>

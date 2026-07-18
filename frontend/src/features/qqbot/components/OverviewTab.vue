<template>
  <div class="space-y-6">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.qqbot.overview.title') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('admin.qqbot.overview.description') }}</p>
      </div>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="$emit('refresh')">
        {{ t('common.refresh') }}
      </button>
    </div>

    <div v-if="error" role="alert" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300">
      {{ error }}
    </div>

    <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <article v-for="item in statusCards" :key="item.label" class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
        <p class="text-xs font-medium uppercase tracking-[0.08em] text-gray-500 dark:text-dark-400">{{ item.label }}</p>
        <p class="mt-2 text-xl font-semibold text-gray-950 dark:text-white">{{ item.value }}</p>
        <p v-if="item.hint" class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ item.hint }}</p>
      </article>
    </div>

    <div class="grid gap-6 xl:grid-cols-[1.2fr_0.8fr]">
      <section class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
        <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.qqbot.overview.queueTitle') }}</h3>
        <dl class="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <div v-for="item in queueItems" :key="item.label">
            <dt class="text-xs text-gray-500 dark:text-dark-400">{{ item.label }}</dt>
            <dd class="mt-1 font-mono text-lg text-gray-950 dark:text-white">{{ item.value }}</dd>
          </div>
        </dl>
      </section>

      <section class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
        <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.qqbot.overview.activityTitle') }}</h3>
        <dl class="mt-4 space-y-3">
          <div v-for="item in activityItems" :key="item.label" class="flex items-baseline justify-between gap-4 border-b border-gray-100 pb-3 last:border-0 last:pb-0 dark:border-dark-700">
            <dt class="text-xs text-gray-500 dark:text-dark-400">{{ item.label }}</dt>
            <dd class="text-right text-sm text-gray-800 dark:text-dark-100">{{ item.value }}</dd>
          </div>
        </dl>
      </section>
    </div>

    <section class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
      <div class="flex flex-wrap items-baseline justify-between gap-3">
        <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.qqbot.overview.bindingTitle') }}</h3>
        <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.qqbot.overview.totalRequests', { count: stats?.total_requests ?? 0 }) }}</span>
      </div>
      <dl class="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-5">
        <div v-for="item in bindingItems" :key="item.label">
          <dt class="text-xs text-gray-500 dark:text-dark-400">{{ item.label }}</dt>
          <dd class="mt-1 text-lg font-semibold text-gray-950 dark:text-white">{{ item.value }}</dd>
        </div>
      </dl>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatDateTime } from '@/utils/format'
import type { QQBotConfig, QQBotRuntime, QQBotStats } from '../types'

const props = defineProps<{
  config: QQBotConfig | null
  runtime: QQBotRuntime | null
  stats: QQBotStats | null
  loading: boolean
  error: string
}>()

defineEmits<{ refresh: [] }>()
const { t, locale } = useI18n()

function date(value?: string): string {
  return value ? formatDateTime(value, undefined, locale.value) : t('common.time.never')
}

const statusCards = computed(() => [
  {
    label: t('admin.qqbot.overview.desiredState'),
    value: props.config?.enabled ? t('admin.qqbot.status.enabled') : t('admin.qqbot.status.disabled'),
    hint: t('admin.qqbot.overview.configVersion', { version: props.config?.config_version ?? 0 }),
  },
  {
    label: t('admin.qqbot.overview.runtimeState'),
    value: t(`admin.qqbot.runtime.${props.runtime?.process_status || 'unknown'}`),
    hint: t('admin.qqbot.overview.activeVersion', { version: props.runtime?.active_config_version ?? 0 }),
  },
  {
    label: t('admin.qqbot.overview.workers'),
    value: `${props.runtime?.worker_active ?? 0} / ${props.runtime?.worker_total ?? 0}`,
    hint: t('admin.qqbot.overview.pendingJobs', { count: props.runtime?.stream_pending ?? 0 }),
  },
  {
    label: t('admin.qqbot.overview.completionRate'),
    value: `${((props.stats?.completion_rate ?? 0) * 100).toFixed(1)}%`,
    hint: t('admin.qqbot.overview.todayRequests', { count: props.stats?.today_requests ?? 0 }),
  },
])

const queueItems = computed(() => [
  { label: t('admin.qqbot.overview.backlog'), value: props.runtime?.stream_backlog ?? 0 },
  { label: t('admin.qqbot.overview.pending'), value: props.runtime?.stream_pending ?? 0 },
  { label: t('admin.qqbot.overview.deadLetters'), value: props.runtime?.dead_letter_total ?? 0 },
  { label: t('admin.qqbot.overview.queueCapacity'), value: props.config?.queue_capacity ?? 0 },
])

const activityItems = computed(() => [
  { label: t('admin.qqbot.overview.lastWebhook'), value: date(props.runtime?.last_webhook_at) },
  { label: t('admin.qqbot.overview.lastEvent'), value: date(props.runtime?.last_event_at) },
  { label: t('admin.qqbot.overview.lastSend'), value: date(props.runtime?.last_send_at) },
  { label: t('admin.qqbot.overview.lastError'), value: props.runtime?.last_error_code || t('common.none') },
])

const bindingItems = computed(() => [
  { label: t('admin.qqbot.bindings.completed'), value: props.stats?.completed ?? 0 },
  { label: t('admin.qqbot.bindings.pending'), value: props.stats?.pending ?? 0 },
  { label: t('admin.qqbot.bindings.expired'), value: props.stats?.expired ?? 0 },
  { label: t('admin.qqbot.bindings.failed'), value: props.stats?.failed ?? 0 },
  { label: t('admin.qqbot.bindings.revoked'), value: props.stats?.revoked ?? 0 },
])
</script>

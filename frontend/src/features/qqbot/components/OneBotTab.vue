<template>
  <div class="space-y-6">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.qqbot.onebot.title') }}</h2>
        <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-dark-300">{{ t('admin.qqbot.onebot.description') }}</p>
      </div>
      <span class="rounded-full border px-3 py-1 text-xs font-medium" :class="runtime?.connected ? connectedClass : disconnectedClass">
        {{ runtime?.connected ? t('admin.qqbot.onebot.connected') : t('admin.qqbot.onebot.disconnected') }}
      </span>
    </div>

    <div v-if="error" role="alert" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300">
      {{ error }}
    </div>

    <section class="grid gap-5 rounded-xl border border-gray-200 bg-white p-5 sm:grid-cols-2 dark:border-dark-700 dark:bg-dark-800">
      <div>
        <label class="input-label" for="onebot-self-id">{{ t('admin.qqbot.onebot.selfId') }}</label>
        <input id="onebot-self-id" class="input" inputmode="numeric" autocomplete="off" :value="draft.self_id" @input="update('self_id', valueOf($event))" />
        <p class="input-hint">{{ t('admin.qqbot.onebot.selfIdHint') }}</p>
      </div>
      <div>
        <div class="mb-1.5 flex items-center justify-between gap-2">
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300" for="onebot-token">{{ t('admin.qqbot.onebot.accessToken') }}</label>
          <span class="rounded-full border px-2 py-0.5 text-xs" :class="draft.access_token_configured ? connectedClass : disconnectedClass">
            {{ draft.access_token_configured ? t('admin.qqbot.secrets.configured') : t('admin.qqbot.secrets.missing') }}
          </span>
        </div>
        <input id="onebot-token" type="password" class="input" autocomplete="new-password" :value="draft.access_token" :placeholder="t('admin.qqbot.secrets.keepPlaceholder')" @input="update('access_token', valueOf($event))" />
        <p class="input-hint">{{ t('admin.qqbot.onebot.tokenHint') }}</p>
      </div>
      <div class="sm:col-span-2">
        <label class="input-label">{{ t('admin.qqbot.onebot.reverseWsUrl') }}</label>
        <div class="flex items-start gap-2">
          <code class="min-w-0 flex-1 break-all rounded-lg bg-gray-100 px-3 py-2 text-xs text-gray-800 dark:bg-dark-900 dark:text-dark-100">{{ draft.reverse_ws_url }}</code>
          <button type="button" class="btn btn-secondary btn-sm" @click="copyURL">{{ copied ? t('common.copied') : t('common.copy') }}</button>
        </div>
        <p class="input-hint">{{ t('admin.qqbot.onebot.reverseWsHint') }}</p>
      </div>
    </section>

    <section class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
      <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.qqbot.config.runtimeTitle') }}</h3>
      <div class="mt-4 grid gap-5 sm:grid-cols-3">
        <div>
          <label class="input-label" for="onebot-workers">{{ t('admin.qqbot.config.workerCount') }}</label>
          <input id="onebot-workers" type="number" min="1" max="64" class="input" :value="draft.worker_count" @input="updateNumber('worker_count', $event)" />
        </div>
        <div>
          <label class="input-label" for="onebot-queue">{{ t('admin.qqbot.config.queueCapacity') }}</label>
          <input id="onebot-queue" type="number" min="16" max="100000" class="input" :value="draft.queue_capacity" @input="updateNumber('queue_capacity', $event)" />
        </div>
        <div>
          <label class="input-label" for="onebot-timeout">{{ t('admin.qqbot.onebot.actionTimeout') }}</label>
          <input id="onebot-timeout" type="number" min="500" max="30000" step="100" class="input" :value="draft.action_timeout_ms" @input="updateNumber('action_timeout_ms', $event)" />
        </div>
      </div>
    </section>

    <section class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <article v-for="item in statusItems" :key="item.label" class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
        <p class="text-xs text-gray-500 dark:text-dark-400">{{ item.label }}</p>
        <p class="mt-2 break-all font-mono text-sm text-gray-950 dark:text-white">{{ item.value }}</p>
      </article>
    </section>

    <div v-if="probeResult" class="rounded-xl border px-4 py-3" :class="probeResult.ok ? connectedClass : failureClass" role="status">
      <div class="flex flex-wrap items-baseline justify-between gap-2">
        <strong class="text-sm">{{ probeResult.ok ? t('admin.qqbot.diagnostics.probeOk') : t('admin.qqbot.diagnostics.probeFailed') }}</strong>
        <span class="font-mono text-xs">{{ probeResult.latency_ms ?? 0 }} ms</span>
      </div>
      <p class="mt-2 text-sm">{{ probeResult.error_code ? `${probeResult.error_code}: ` : '' }}{{ probeResult.message }}</p>
    </div>

    <div class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-gray-200 bg-gray-50 px-4 py-3 dark:border-dark-700 dark:bg-dark-900">
      <label class="flex cursor-pointer items-center gap-2 text-sm text-gray-800 dark:text-dark-100">
        <input type="checkbox" class="h-4 w-4 accent-primary-600" :checked="draft.enabled" data-test="onebot-enabled" @change="$emit('set-enabled', checkedOf($event))" />
        {{ t('admin.qqbot.onebot.enableRuntime') }}
      </label>
      <div class="flex flex-wrap gap-3">
        <button type="button" class="btn btn-secondary" :disabled="probing" data-test="onebot-probe" @click="$emit('probe')">
          {{ probing ? t('admin.qqbot.actions.probing') : t('admin.qqbot.actions.probe') }}
        </button>
        <button type="button" class="btn btn-secondary" :disabled="!dirty || saving" @click="$emit('reset')">{{ t('common.reset') }}</button>
        <button type="button" class="btn bg-primary-600 text-white hover:bg-primary-700" :disabled="!dirty || saving" data-test="onebot-save" @click="$emit('save')">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatDateTime } from '@/utils/format'
import type { QQBotOneBotDraft, QQBotOneBotRuntime, QQBotProbeResult } from '../types'

const props = defineProps<{
  draft: QQBotOneBotDraft
  runtime: QQBotOneBotRuntime | null
  probeResult: QQBotProbeResult | null
  dirty: boolean
  saving: boolean
  probing: boolean
  error: string
}>()
const emit = defineEmits<{
  'update:draft': [value: QQBotOneBotDraft]
  'set-enabled': [value: boolean]
  save: []
  reset: []
  probe: []
}>()
const { t, locale } = useI18n()
const copied = ref(false)
const connectedClass = 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900 dark:bg-emerald-950/30 dark:text-emerald-300'
const disconnectedClass = 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-300'
const failureClass = 'border-red-200 bg-red-50 text-red-800 dark:border-red-900 dark:bg-red-950/30 dark:text-red-200'

function update<K extends keyof QQBotOneBotDraft>(key: K, value: QQBotOneBotDraft[K]) {
  emit('update:draft', { ...props.draft, [key]: value })
}
function valueOf(event: Event) { return (event.target as HTMLInputElement).value }
function checkedOf(event: Event) { return (event.target as HTMLInputElement).checked }
function updateNumber(key: 'worker_count' | 'queue_capacity' | 'action_timeout_ms', event: Event) { update(key, Number(valueOf(event))) }
function date(value?: string) { return value ? formatDateTime(value, undefined, locale.value) : t('common.time.never') }
async function copyURL() {
  await navigator.clipboard.writeText(props.draft.reverse_ws_url)
  copied.value = true
  window.setTimeout(() => { copied.value = false }, 1500)
}

const statusItems = computed(() => [
  { label: t('admin.qqbot.onebot.runtimeState'), value: t(`admin.qqbot.runtime.${props.runtime?.process_status || 'unknown'}`) },
  { label: t('admin.qqbot.onebot.workers'), value: `${props.runtime?.worker_active ?? 0} / ${props.runtime?.worker_total ?? 0}` },
  { label: t('admin.qqbot.onebot.pendingActions'), value: props.runtime?.pending_actions ?? 0 },
  { label: t('admin.qqbot.onebot.lastConnection'), value: date(props.runtime?.connected_at || props.runtime?.last_disconnect_at) },
  { label: t('admin.qqbot.overview.backlog'), value: props.runtime?.stream_backlog ?? 0 },
  { label: t('admin.qqbot.overview.pending'), value: props.runtime?.stream_pending ?? 0 },
  { label: t('admin.qqbot.overview.lastEvent'), value: date(props.runtime?.last_event_at || props.runtime?.last_webhook_at) },
  { label: t('admin.qqbot.overview.lastSend'), value: date(props.runtime?.last_send_at) },
])
</script>

<template>
  <div class="space-y-6">
    <div>
      <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.qqbot.config.title') }}</h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('admin.qqbot.config.description') }}</p>
    </div>

    <section class="grid gap-5 rounded-xl border border-gray-200 bg-white p-5 sm:grid-cols-2 dark:border-dark-700 dark:bg-dark-800">
      <div>
        <label class="input-label" for="qqbot-app-id">{{ t('admin.qqbot.config.appId') }}</label>
        <input id="qqbot-app-id" class="input" :value="draft.app_id" autocomplete="off" @input="update('app_id', valueOf($event))" />
      </div>
      <label class="flex min-h-11 items-center gap-3 rounded-xl border border-gray-200 px-4 py-3 dark:border-dark-700">
        <input type="checkbox" :checked="draft.sandbox" class="h-4 w-4 accent-primary-600" @change="update('sandbox', checkedOf($event))" />
        <span>
          <span class="block text-sm font-medium text-gray-800 dark:text-dark-100">{{ t('admin.qqbot.config.sandbox') }}</span>
          <span class="block text-xs text-gray-500 dark:text-dark-400">{{ t('admin.qqbot.config.sandboxHint') }}</span>
        </span>
      </label>

      <div>
        <div class="mb-1.5 flex items-center justify-between gap-2">
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300" for="qqbot-app-secret">{{ t('admin.qqbot.config.appSecret') }}</label>
          <span class="rounded-full border px-2 py-0.5 text-xs" :class="draft.app_secret_configured ? configuredClass : missingClass">
            {{ draft.app_secret_configured ? t('admin.qqbot.secrets.configured') : t('admin.qqbot.secrets.missing') }}
          </span>
        </div>
        <input id="qqbot-app-secret" type="password" class="input" :value="draft.app_secret" autocomplete="new-password" :placeholder="t('admin.qqbot.secrets.keepPlaceholder')" @input="update('app_secret', valueOf($event))" />
        <p class="input-hint">{{ t('admin.qqbot.secrets.keepHint') }}</p>
      </div>

      <div>
        <div class="mb-1.5 flex items-center justify-between gap-2">
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300" for="qqbot-webhook-secret">{{ t('admin.qqbot.config.webhookSecret') }}</label>
          <span class="rounded-full border px-2 py-0.5 text-xs" :class="draft.webhook_secret_configured ? configuredClass : missingClass">
            {{ draft.webhook_secret_configured ? t('admin.qqbot.secrets.configured') : t('admin.qqbot.secrets.missing') }}
          </span>
        </div>
        <input id="qqbot-webhook-secret" type="password" class="input" :value="draft.webhook_secret" autocomplete="new-password" :placeholder="t('admin.qqbot.secrets.keepPlaceholder')" @input="update('webhook_secret', valueOf($event))" />
        <p class="input-hint">{{ t('admin.qqbot.secrets.keepHint') }}</p>
      </div>

      <div class="sm:col-span-2">
        <label class="input-label" for="qqbot-public-url">{{ t('admin.qqbot.config.publicBaseUrl') }}</label>
        <input id="qqbot-public-url" type="url" class="input" :value="draft.public_base_url" placeholder="https://qqbot.example.com" @input="update('public_base_url', valueOf($event))" />
        <p class="input-hint">{{ t('admin.qqbot.config.publicBaseUrlHint') }}</p>
      </div>
    </section>

    <section class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
      <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.qqbot.config.runtimeTitle') }}</h3>
      <div class="mt-4 grid gap-5 sm:grid-cols-3">
        <div>
          <label class="input-label" for="qqbot-workers">{{ t('admin.qqbot.config.workerCount') }}</label>
          <input id="qqbot-workers" type="number" min="1" max="64" class="input" :value="draft.worker_count" @input="updateNumber('worker_count', $event)" />
        </div>
        <div>
          <label class="input-label" for="qqbot-queue">{{ t('admin.qqbot.config.queueCapacity') }}</label>
          <input id="qqbot-queue" type="number" min="100" max="100000" class="input" :value="draft.queue_capacity" @input="updateNumber('queue_capacity', $event)" />
        </div>
        <div>
          <label class="input-label" for="qqbot-timeout">{{ t('admin.qqbot.config.apiTimeout') }}</label>
          <input id="qqbot-timeout" type="number" min="500" max="30000" step="100" class="input" :value="draft.api_timeout_ms" @input="updateNumber('api_timeout_ms', $event)" />
        </div>
      </div>
    </section>

    <div class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-gray-200 bg-gray-50 px-4 py-3 dark:border-dark-700 dark:bg-dark-900">
      <div>
        <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.qqbot.config.probeTitle') }}</p>
        <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.qqbot.config.probeHint') }}</p>
      </div>
      <button type="button" class="btn btn-secondary" :disabled="probing" data-test="probe-button" @click="$emit('probe')">
        {{ probing ? t('admin.qqbot.actions.probing') : t('admin.qqbot.actions.probe') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { QQBotDraft } from '../types'

const props = defineProps<{ draft: QQBotDraft; probing: boolean }>()
const emit = defineEmits<{ 'update:draft': [value: QQBotDraft]; probe: [] }>()
const { t } = useI18n()
const configuredClass = 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900 dark:bg-emerald-950/30 dark:text-emerald-300'
const missingClass = 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-300'

function update<K extends keyof QQBotDraft>(key: K, value: QQBotDraft[K]) {
  emit('update:draft', { ...props.draft, [key]: value })
}
function valueOf(event: Event) { return (event.target as HTMLInputElement).value }
function checkedOf(event: Event) { return (event.target as HTMLInputElement).checked }
function updateNumber(key: 'worker_count' | 'queue_capacity' | 'api_timeout_ms', event: Event) {
  update(key, Number(valueOf(event)))
}
</script>

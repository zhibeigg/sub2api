<template>
  <div class="space-y-6">
    <div class="flex flex-wrap items-end justify-between gap-3">
      <div>
        <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.qqbot.diagnostics.title') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('admin.qqbot.diagnostics.description') }}</p>
      </div>
      <button type="button" class="btn btn-secondary" :disabled="probing" @click="$emit('probe')">{{ probing ? t('admin.qqbot.actions.probing') : t('admin.qqbot.actions.probe') }}</button>
    </div>

    <section class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
      <dl class="grid gap-5 md:grid-cols-2">
        <div v-for="item in endpoints" :key="item.label" class="min-w-0">
          <dt class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-dark-400">{{ item.label }}</dt>
          <dd class="mt-2 flex items-start gap-2"><code class="min-w-0 flex-1 break-all rounded-lg bg-gray-100 px-3 py-2 text-xs text-gray-800 dark:bg-dark-900 dark:text-dark-100">{{ item.value }}</code><button type="button" class="btn btn-secondary btn-sm" :aria-label="t('common.copy')" @click="copy(item.value)">{{ copied === item.value ? t('common.copied') : t('common.copy') }}</button></dd>
        </div>
      </dl>
    </section>

    <section class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <article v-for="item in versions" :key="item.label" class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
        <p class="text-xs text-gray-500 dark:text-dark-400">{{ item.label }}</p>
        <p class="mt-2 font-mono text-lg text-gray-950 dark:text-white">{{ item.value }}</p>
      </article>
    </section>

    <section class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
      <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.qqbot.diagnostics.probeResult') }}</h3>
      <div v-if="probeResult" class="mt-4 rounded-xl border px-4 py-3" :class="probeResult.ok ? successClass : failureClass" role="status">
        <div class="flex flex-wrap items-baseline justify-between gap-2"><strong class="text-sm">{{ probeResult.ok ? t('admin.qqbot.diagnostics.probeOk') : t('admin.qqbot.diagnostics.probeFailed') }}</strong><span class="font-mono text-xs">{{ probeResult.latency_ms ?? 0 }} ms</span></div>
        <p class="mt-2 text-sm">{{ probeResult.error_code ? `${probeResult.error_code}: ` : '' }}{{ probeResult.message }}</p>
      </div>
      <p v-else class="mt-3 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.qqbot.diagnostics.noProbe') }}</p>
    </section>

    <section class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
      <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.qqbot.diagnostics.lastError') }}</h3>
      <dl class="mt-4 grid gap-4 sm:grid-cols-3">
        <div><dt class="text-xs text-gray-500">{{ t('admin.qqbot.diagnostics.errorCode') }}</dt><dd class="mt-1 font-mono text-sm">{{ runtime?.last_error_code || t('common.none') }}</dd></div>
        <div><dt class="text-xs text-gray-500">{{ t('admin.qqbot.diagnostics.errorTime') }}</dt><dd class="mt-1 text-sm">{{ date(runtime?.last_error_at) }}</dd></div>
        <div><dt class="text-xs text-gray-500">{{ t('admin.qqbot.diagnostics.errorMessage') }}</dt><dd class="mt-1 text-sm">{{ runtime?.last_error_message || t('common.none') }}</dd></div>
      </dl>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatDateTime } from '@/utils/format'
import type { QQBotConfig, QQBotProbeResult, QQBotRuntime } from '../types'
import { validationURL, webhookURL } from '../viewModel'

const props = defineProps<{ config: QQBotConfig; runtime: QQBotRuntime | null; probeResult: QQBotProbeResult | null; probing: boolean }>()
defineEmits<{ probe: [] }>()
const { t, locale } = useI18n()
const copied = ref('')
const successClass = 'border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950/30 dark:text-emerald-200'
const failureClass = 'border-red-200 bg-red-50 text-red-800 dark:border-red-900 dark:bg-red-950/30 dark:text-red-200'

const endpoints = computed(() => [
  { label: t('admin.qqbot.diagnostics.webhookUrl'), value: webhookURL(props.config) },
  { label: t('admin.qqbot.diagnostics.validationUrl'), value: validationURL(props.config) },
])
const versions = computed(() => [
  { label: t('admin.qqbot.diagnostics.configVersion'), value: props.config.config_version },
  { label: t('admin.qqbot.diagnostics.desiredVersion'), value: props.runtime?.desired_config_version ?? 0 },
  { label: t('admin.qqbot.diagnostics.activeVersion'), value: props.runtime?.active_config_version ?? 0 },
  { label: t('admin.qqbot.diagnostics.runtimeState'), value: t(`admin.qqbot.runtime.${props.runtime?.process_status || 'unknown'}`) },
])
function date(value?: string) { return value ? formatDateTime(value, undefined, locale.value) : t('common.time.never') }
async function copy(value: string) { await navigator.clipboard.writeText(value); copied.value = value; window.setTimeout(() => { copied.value = '' }, 1500) }
</script>

<template>
  <AppLayout>
    <div class="mx-auto max-w-[1600px] pb-24">
      <header class="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div>
          <p class="text-xs font-semibold uppercase tracking-[0.16em] text-primary-700 dark:text-primary-300">QQBOT</p>
          <h1 class="mt-1 text-2xl font-semibold tracking-tight text-gray-950 dark:text-white">{{ t('admin.qqbot.title') }}</h1>
          <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-dark-300">{{ t('admin.qqbot.description') }}</p>
        </div>
        <div v-if="draft" class="text-right text-xs text-gray-500 dark:text-dark-400">
          <p>{{ t('admin.qqbot.overview.configVersion', { version: draft.config_version }) }}</p>
          <p v-if="draft.updated_at" class="mt-1">{{ date(draft.updated_at) }}</p>
        </div>
      </header>

      <div v-if="loadErrors.config && !draft" role="alert" class="rounded-xl border border-red-200 bg-red-50 p-5 dark:border-red-900 dark:bg-red-950/30">
        <p class="text-sm text-red-700 dark:text-red-300">{{ loadErrors.config }}</p>
        <button type="button" class="btn btn-secondary btn-sm mt-3" @click="loadConfig">{{ t('common.retry') }}</button>
      </div>

      <template v-else>
        <div class="mb-4 overflow-x-auto" role="tablist" :aria-label="t('admin.qqbot.title')">
          <div class="tabs min-w-max">
            <button v-for="tab in tabs" :key="tab.id" type="button" role="tab" class="tab" :class="{ 'tab-active': activeTab === tab.id }" :aria-selected="activeTab === tab.id" :data-test="`tab-${tab.id}`" @click="activeTab = tab.id">{{ tab.label }}</button>
          </div>
        </div>

        <main class="card p-4 sm:p-6 lg:p-8">
          <OverviewTab v-show="activeTab === 'overview'" :config="serverConfig" :runtime="runtime" :stats="stats" :loading="loading.runtime || loading.stats" :error="loadErrors.runtime || loadErrors.stats" @refresh="refreshOverview" />
          <BotConfigTab v-if="draft" v-show="activeTab === 'config'" :draft="draft" :probing="loading.probing" @update:draft="replaceDraft" @probe="runProbe" />
          <MessagesTab v-if="draft" v-show="activeTab === 'messages'" :draft="draft" @update:draft="replaceDraft" />
          <BindingsTab ref="bindingsTabRef" v-show="activeTab === 'bindings'" :page="bindings" :filters="bindingFilters" :loading="loading.bindings" :error="loadErrors.bindings" :unbinding="loading.unbinding" @update:filters="bindingFilters = $event" @search="searchBindings" @reset="resetBindings" @refresh="loadBindings" @page="changeBindingPage" @unbind="unbindRecord" />
          <DiagnosticsTab v-if="serverConfig" v-show="activeTab === 'diagnostics'" :config="serverConfig" :runtime="runtime" :probe-result="probeResult" :probing="loading.probing" @probe="runProbe" />
        </main>
      </template>
    </div>

    <div v-if="draft && (activeTab === 'config' || activeTab === 'messages')" class="fixed inset-x-0 bottom-0 z-30 border-t border-gray-200 bg-white px-4 py-3 shadow-[0_-8px_24px_rgba(15,23,42,0.08)] dark:border-dark-700 dark:bg-dark-900 lg:left-64">
      <div class="mx-auto flex max-w-[1600px] flex-wrap items-center justify-between gap-3">
        <div class="flex flex-wrap items-center gap-4">
          <label class="flex cursor-pointer items-center gap-2 text-sm text-gray-800 dark:text-dark-100">
            <input type="checkbox" class="h-4 w-4 accent-primary-600" :checked="draft.enabled" data-test="enabled-toggle" @change="setEnabled(($event.target as HTMLInputElement).checked)" />
            {{ t('admin.qqbot.actions.enabled') }}
          </label>
          <span class="text-sm" :class="dirty ? 'text-amber-700 dark:text-amber-300' : 'text-gray-500 dark:text-dark-400'">{{ dirty ? t('admin.qqbot.actions.unsaved') : t('admin.qqbot.actions.synced') }}</span>
        </div>
        <div class="flex gap-3">
          <button type="button" class="btn btn-secondary" :disabled="!dirty || loading.saving" @click="resetDraft">{{ t('common.reset') }}</button>
          <button type="button" class="btn bg-primary-600 text-white hover:bg-primary-700" :disabled="!dirty || loading.saving" data-test="save-config" @click="saveConfig">{{ loading.saving ? t('common.saving') : t('common.save') }}</button>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorCode, extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import qqbotAPI from './api'
import BindingsTab from './components/BindingsTab.vue'
import BotConfigTab from './components/BotConfigTab.vue'
import DiagnosticsTab from './components/DiagnosticsTab.vue'
import MessagesTab from './components/MessagesTab.vue'
import OverviewTab from './components/OverviewTab.vue'
import type {
  QQBotBindingFilters,
  QQBotBindingPage,
  QQBotConfig,
  QQBotDraft,
  QQBotProbeResult,
  QQBotRuntime,
  QQBotStats,
} from './types'
import {
  buildProbeRequest,
  buildUpdateRequest,
  cloneData,
  configToDraft,
  credentialFingerprint,
  credentialsReady,
  draftFingerprint,
  validateDraft,
} from './viewModel'

type QQBotTab = 'overview' | 'config' | 'messages' | 'bindings' | 'diagnostics'
type BindingsTabExpose = { closeUnbind: () => void }

const { t, locale } = useI18n()
const appStore = useAppStore()
const activeTab = ref<QQBotTab>('overview')
const serverConfig = ref<QQBotConfig | null>(null)
const serverDraft = ref<QQBotDraft | null>(null)
const draft = ref<QQBotDraft | null>(null)
const runtime = ref<QQBotRuntime | null>(null)
const stats = ref<QQBotStats | null>(null)
const probeResult = ref<QQBotProbeResult | null>(null)
const probeFingerprint = ref('')
const bindings = reactive<QQBotBindingPage>({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
const bindingFilters = ref<QQBotBindingFilters>({ status: '', scene: '', search: '', from: '', to: '' })
const bindingsTabRef = ref<BindingsTabExpose | null>(null)
const loading = reactive({ config: false, runtime: false, stats: false, bindings: false, saving: false, probing: false, unbinding: false })
const loadErrors = reactive({ config: '', runtime: '', stats: '', bindings: '' })

const tabs = computed(() => [
  { id: 'overview' as const, label: t('admin.qqbot.tabs.overview') },
  { id: 'config' as const, label: t('admin.qqbot.tabs.config') },
  { id: 'messages' as const, label: t('admin.qqbot.tabs.messages') },
  { id: 'bindings' as const, label: t('admin.qqbot.tabs.bindings') },
  { id: 'diagnostics' as const, label: t('admin.qqbot.tabs.diagnostics') },
])
const dirty = computed(() => draftFingerprint(draft.value) !== draftFingerprint(serverDraft.value))

function errorMessage(error: unknown, fallbackKey: string): string {
  const code = extractApiErrorCode(error)
  if (code) {
    const key = `admin.qqbot.errors.${code}`
    const translated = t(key)
    if (translated !== key) return translated
  }
  return extractApiErrorMessage(error, t(fallbackKey))
}
function date(value: string) { return formatDateTime(value, undefined, locale.value) }
function replaceDraft(value: QQBotDraft) { draft.value = cloneData(value) }
function resetDraft() { if (serverDraft.value) draft.value = cloneData(serverDraft.value) }

async function loadConfig() {
  loading.config = true
  loadErrors.config = ''
  try {
    const config = await qqbotAPI.getConfig()
    serverConfig.value = config
    serverDraft.value = configToDraft(config)
    draft.value = configToDraft(config)
  } catch (error) {
    loadErrors.config = errorMessage(error, 'admin.qqbot.errors.loadConfig')
  } finally { loading.config = false }
}
async function loadRuntime() {
  loading.runtime = true; loadErrors.runtime = ''
  try { runtime.value = await qqbotAPI.getRuntime() }
  catch (error) { loadErrors.runtime = errorMessage(error, 'admin.qqbot.errors.loadRuntime') }
  finally { loading.runtime = false }
}
async function loadStats() {
  loading.stats = true; loadErrors.stats = ''
  try { stats.value = await qqbotAPI.getStats() }
  catch (error) { loadErrors.stats = errorMessage(error, 'admin.qqbot.errors.loadStats') }
  finally { loading.stats = false }
}
async function loadBindings() {
  loading.bindings = true; loadErrors.bindings = ''
  try { Object.assign(bindings, await qqbotAPI.listBindings(bindingFilters.value, bindings.page, bindings.page_size)) }
  catch (error) { loadErrors.bindings = errorMessage(error, 'admin.qqbot.errors.loadBindings') }
  finally { loading.bindings = false }
}
async function refreshOverview() { await Promise.allSettled([loadRuntime(), loadStats()]) }
function searchBindings() { bindings.page = 1; void loadBindings() }
function resetBindings() { bindingFilters.value = { status: '', scene: '', search: '', from: '', to: '' }; bindings.page = 1; void loadBindings() }
function changeBindingPage(page: number) { bindings.page = page; void loadBindings() }

function requiresFreshProbe(value: QQBotDraft): boolean {
  if (!serverDraft.value) return true
  if (!serverDraft.value.enabled && value.enabled) return true
  return credentialFingerprint(value) !== credentialFingerprint(serverDraft.value)
}
function hasCurrentSuccessfulProbe(value: QQBotDraft): boolean {
  return Boolean(probeResult.value?.ok && probeFingerprint.value === credentialFingerprint(value))
}
function setEnabled(value: boolean) {
  if (!draft.value) return
  if (!value) { replaceDraft({ ...draft.value, enabled: false }); return }
  if (!credentialsReady(draft.value)) { appStore.showError(t('admin.qqbot.errors.credentialsRequired')); return }
  if (requiresFreshProbe(draft.value) && !hasCurrentSuccessfulProbe(draft.value)) { appStore.showError(t('admin.qqbot.errors.probeRequired')); return }
  replaceDraft({ ...draft.value, enabled: true })
}

async function runProbe() {
  if (!draft.value || loading.probing) return
  if (!credentialsReady(draft.value)) { appStore.showError(t('admin.qqbot.errors.credentialsRequired')); return }
  loading.probing = true
  try {
    const fingerprint = credentialFingerprint(draft.value)
    probeResult.value = await qqbotAPI.probe(buildProbeRequest(draft.value))
    probeFingerprint.value = fingerprint
    if (probeResult.value.ok) appStore.showSuccess(t('admin.qqbot.notices.probeSucceeded'))
    else appStore.showError(`${probeResult.value.error_code || probeResult.value.status}: ${probeResult.value.message}`)
  } catch (error) {
    probeResult.value = null
    probeFingerprint.value = ''
    appStore.showError(errorMessage(error, 'admin.qqbot.errors.probe'))
  } finally { loading.probing = false }
}

async function saveConfig() {
  if (!draft.value || !dirty.value) return
  const invalid = validateDraft(draft.value)
  if (invalid.length) { appStore.showError(t(`admin.qqbot.validation.${invalid[0]}`)); return }
  if (draft.value.enabled && !credentialsReady(draft.value)) { appStore.showError(t('admin.qqbot.errors.credentialsRequired')); return }
  if (requiresFreshProbe(draft.value) && !hasCurrentSuccessfulProbe(draft.value)) { appStore.showError(t('admin.qqbot.errors.probeRequired')); return }
  loading.saving = true
  try {
    const saved = await qqbotAPI.updateConfig(buildUpdateRequest(draft.value))
    serverConfig.value = saved
    serverDraft.value = configToDraft(saved)
    draft.value = configToDraft(saved)
    probeFingerprint.value = ''
    appStore.showSuccess(t('admin.qqbot.notices.saved'))
    await Promise.allSettled([loadRuntime(), loadStats()])
  } catch (error) {
    appStore.showError(errorMessage(error, 'admin.qqbot.errors.saveConfig'))
  } finally { loading.saving = false }
}

async function unbindRecord(id: string, reason: string) {
  if (loading.unbinding) return
  loading.unbinding = true
  try {
    await qqbotAPI.unbind(id, reason)
    bindingsTabRef.value?.closeUnbind()
    appStore.showSuccess(t('admin.qqbot.notices.unbound'))
    await Promise.allSettled([loadBindings(), loadStats()])
  } catch (error) { appStore.showError(errorMessage(error, 'admin.qqbot.errors.unbind')) }
  finally { loading.unbinding = false }
}

async function loadInitial() {
  await Promise.allSettled([loadConfig(), loadRuntime(), loadStats(), loadBindings()])
}
onMounted(loadInitial)
</script>

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
          <OverviewTab v-show="activeTab === 'overview'" :config="serverConfig" :runtime="activeRuntime" :stats="stats" :loading="activeRuntimeLoading || loading.stats" :error="activeRuntimeError || loadErrors.stats" @refresh="refreshOverview" />
          <TransportModeTab v-if="serverConfig" v-show="activeTab === 'transport'" :mode="selectedTransportMode" :inherited="serverConfig.transport_mode_inherited" :loading="loading.transport" :error="loadErrors.transport" @select="selectTransportMode" />
          <BotConfigTab v-if="draft && selectedTransportMode === 'botgo'" v-show="activeTab === 'config'" :draft="draft" :probing="loading.probing" @update:draft="replaceDraft" @probe="runProbe" />
          <OneBotTab v-if="oneBotDraft && selectedTransportMode === 'onebot'" v-show="activeTab === 'config'" :draft="oneBotDraft" :runtime="oneBotRuntime" :probe-result="oneBotProbeResult" :dirty="oneBotDirty" :saving="loading.oneBotSaving" :probing="loading.oneBotProbing" :error="loadErrors.oneBotConfig || loadErrors.oneBotRuntime" @update:draft="replaceOneBotDraft" @set-enabled="setOneBotEnabled" @save="saveOneBotConfig" @reset="resetOneBotDraft" @probe="runOneBotProbe" />
          <MessagesTab v-if="draft" v-show="activeTab === 'messages'" :draft="draft" @update:draft="replaceDraft" />
          <BindingsTab ref="bindingsTabRef" v-show="activeTab === 'bindings'" :page="bindings" :filters="bindingFilters" :loading="loading.bindings" :error="loadErrors.bindings" :unbinding="loading.unbinding" @update:filters="bindingFilters = $event" @search="searchBindings" @reset="resetBindings" @refresh="loadBindings" @page="changeBindingPage" @unbind="unbindRecord" />
          <DiagnosticsTab v-if="serverConfig" v-show="activeTab === 'diagnostics'" :config="serverConfig" :mode="selectedTransportMode" :runtime="activeRuntime" :probe-result="activeProbeResult" :probing="activeProbeLoading" @probe="runActiveProbe" />
        </main>
      </template>
    </div>

    <div v-if="draft && activeTab === 'config' && selectedTransportMode === 'botgo'" class="fixed inset-x-0 bottom-0 z-30 border-t border-gray-200 bg-white px-4 py-3 shadow-[0_-8px_24px_rgba(15,23,42,0.08)] dark:border-dark-700 dark:bg-dark-900 lg:left-64">
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
import OneBotTab from './components/OneBotTab.vue'
import OverviewTab from './components/OverviewTab.vue'
import TransportModeTab from './components/TransportModeTab.vue'
import type {
  QQBotBindingFilters,
  QQBotBindingPage,
  QQBotConfig,
  QQBotDraft,
  QQBotOneBotConfig,
  QQBotOneBotDraft,
  QQBotOneBotRuntime,
  QQBotProbeResult,
  QQBotRuntime,
  QQBotStats,
  QQBotTransportMode,
} from './types'
import {
  buildOneBotProbeRequest,
  buildOneBotUpdateRequest,
  buildProbeRequest,
  buildUpdateRequest,
  cloneData,
  configToDraft,
  credentialFingerprint,
  credentialsReady,
  draftFingerprint,
  oneBotConfigToDraft,
  oneBotCredentialFingerprint,
  oneBotCredentialsReady,
  oneBotDraftFingerprint,
  validateDraft,
  validateOneBotDraft,
} from './viewModel'

type QQBotTab = 'overview' | 'transport' | 'config' | 'messages' | 'bindings' | 'diagnostics'
type BindingsTabExpose = { closeUnbind: () => void }

const { t, locale } = useI18n()
const appStore = useAppStore()
const activeTab = ref<QQBotTab>('overview')
const serverConfig = ref<QQBotConfig | null>(null)
const serverDraft = ref<QQBotDraft | null>(null)
const draft = ref<QQBotDraft | null>(null)
const oneBotConfig = ref<QQBotOneBotConfig | null>(null)
const oneBotServerDraft = ref<QQBotOneBotDraft | null>(null)
const oneBotDraft = ref<QQBotOneBotDraft | null>(null)
const runtime = ref<QQBotRuntime | null>(null)
const oneBotRuntime = ref<QQBotOneBotRuntime | null>(null)
const stats = ref<QQBotStats | null>(null)
const probeResult = ref<QQBotProbeResult | null>(null)
const probeFingerprint = ref('')
const oneBotProbeResult = ref<QQBotProbeResult | null>(null)
const oneBotProbeFingerprint = ref('')
const bindings = reactive<QQBotBindingPage>({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
const bindingFilters = ref<QQBotBindingFilters>({ status: '', scene: '', search: '', from: '', to: '' })
const bindingsTabRef = ref<BindingsTabExpose | null>(null)
const loading = reactive({ config: false, runtime: false, oneBotConfig: false, oneBotRuntime: false, stats: false, bindings: false, saving: false, probing: false, oneBotSaving: false, oneBotProbing: false, transport: false, unbinding: false })
const loadErrors = reactive({ config: '', runtime: '', oneBotConfig: '', oneBotRuntime: '', stats: '', bindings: '', transport: '' })

const tabs = computed(() => [
  { id: 'overview' as const, label: t('admin.qqbot.tabs.overview') },
  { id: 'transport' as const, label: t('admin.qqbot.tabs.transport') },
  { id: 'config' as const, label: t('admin.qqbot.tabs.config') },
  { id: 'messages' as const, label: t('admin.qqbot.tabs.messages') },
  { id: 'bindings' as const, label: t('admin.qqbot.tabs.bindings') },
  { id: 'diagnostics' as const, label: t('admin.qqbot.tabs.diagnostics') },
])
const selectedTransportMode = computed<QQBotTransportMode>(() => serverConfig.value?.transport_mode ?? 'botgo')
const activeRuntime = computed<QQBotRuntime | QQBotOneBotRuntime | null>(() => selectedTransportMode.value === 'botgo' ? runtime.value : oneBotRuntime.value)
const activeRuntimeLoading = computed(() => selectedTransportMode.value === 'botgo' ? loading.runtime : loading.oneBotRuntime)
const activeRuntimeError = computed(() => selectedTransportMode.value === 'botgo' ? loadErrors.runtime : loadErrors.oneBotRuntime)
const activeProbeResult = computed(() => selectedTransportMode.value === 'botgo' ? probeResult.value : oneBotProbeResult.value)
const activeProbeLoading = computed(() => selectedTransportMode.value === 'botgo' ? loading.probing : loading.oneBotProbing)
const dirty = computed(() => draftFingerprint(draft.value) !== draftFingerprint(serverDraft.value))
const oneBotDirty = computed(() => oneBotDraftFingerprint(oneBotDraft.value) !== oneBotDraftFingerprint(oneBotServerDraft.value))

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
function replaceOneBotDraft(value: QQBotOneBotDraft) { oneBotDraft.value = cloneData(value) }
function resetOneBotDraft() { if (oneBotServerDraft.value) oneBotDraft.value = cloneData(oneBotServerDraft.value) }
function resetTransportDrafts() {
  resetDraft()
  resetOneBotDraft()
  probeResult.value = null
  probeFingerprint.value = ''
  oneBotProbeResult.value = null
  oneBotProbeFingerprint.value = ''
}

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
async function loadOneBotConfig() {
  loading.oneBotConfig = true
  loadErrors.oneBotConfig = ''
  try {
    const config = await qqbotAPI.getOneBotConfig()
    oneBotConfig.value = config
    oneBotServerDraft.value = oneBotConfigToDraft(config)
    oneBotDraft.value = oneBotConfigToDraft(config)
  } catch (error) {
    loadErrors.oneBotConfig = errorMessage(error, 'admin.qqbot.errors.loadOneBotConfig')
  } finally { loading.oneBotConfig = false }
}
async function loadRuntime() {
  loading.runtime = true; loadErrors.runtime = ''
  try { runtime.value = await qqbotAPI.getRuntime() }
  catch (error) { loadErrors.runtime = errorMessage(error, 'admin.qqbot.errors.loadRuntime') }
  finally { loading.runtime = false }
}
async function loadOneBotRuntime() {
  loading.oneBotRuntime = true; loadErrors.oneBotRuntime = ''
  try { oneBotRuntime.value = await qqbotAPI.getOneBotRuntime() }
  catch (error) { loadErrors.oneBotRuntime = errorMessage(error, 'admin.qqbot.errors.loadOneBotRuntime') }
  finally { loading.oneBotRuntime = false }
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
async function refreshOverview() { await Promise.allSettled([loadRuntime(), loadOneBotRuntime(), loadStats()]) }
function searchBindings() { bindings.page = 1; void loadBindings() }
function resetBindings() { bindingFilters.value = { status: '', scene: '', search: '', from: '', to: '' }; bindings.page = 1; void loadBindings() }
function changeBindingPage(page: number) { bindings.page = page; void loadBindings() }

async function selectTransportMode(mode: QQBotTransportMode) {
  if (!serverConfig.value || loading.transport || (mode === selectedTransportMode.value && !serverConfig.value.transport_mode_inherited)) return
  loading.transport = true
  loadErrors.transport = ''
  try {
    const saved = await qqbotAPI.updateTransportMode({ mode, expected_config_version: serverConfig.value.config_version })
    serverConfig.value = saved
    serverDraft.value = configToDraft(saved)
    resetTransportDrafts()
    await Promise.allSettled([loadConfig(), loadOneBotConfig(), loadRuntime(), loadOneBotRuntime(), loadStats()])
    activeTab.value = 'config'
    appStore.showSuccess(t('admin.qqbot.notices.transportUpdated'))
  } catch (error) {
    loadErrors.transport = errorMessage(error, 'admin.qqbot.errors.updateTransport')
  } finally { loading.transport = false }
}

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

function hasCurrentSuccessfulOneBotProbe(value: QQBotOneBotDraft): boolean {
  return Boolean(oneBotProbeResult.value?.ok && oneBotProbeFingerprint.value === oneBotCredentialFingerprint(value))
}
function setOneBotEnabled(value: boolean) {
  if (!oneBotDraft.value) return
  if (!value) { replaceOneBotDraft({ ...oneBotDraft.value, enabled: false }); return }
  if (!oneBotCredentialsReady(oneBotDraft.value)) { appStore.showError(t('admin.qqbot.errors.oneBotCredentialsRequired')); return }
  if (!hasCurrentSuccessfulOneBotProbe(oneBotDraft.value)) { appStore.showError(t('admin.qqbot.errors.oneBotProbeRequired')); return }
  replaceOneBotDraft({ ...oneBotDraft.value, enabled: true })
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

async function runOneBotProbe() {
  if (!oneBotDraft.value || loading.oneBotProbing) return
  if (oneBotDirty.value) { appStore.showError(t('admin.qqbot.errors.oneBotSaveBeforeProbe')); return }
  if (!oneBotCredentialsReady(oneBotDraft.value)) { appStore.showError(t('admin.qqbot.errors.oneBotCredentialsRequired')); return }
  loading.oneBotProbing = true
  try {
    const fingerprint = oneBotCredentialFingerprint(oneBotDraft.value)
    oneBotProbeResult.value = await qqbotAPI.probeOneBot(buildOneBotProbeRequest(oneBotDraft.value))
    oneBotProbeFingerprint.value = fingerprint
    if (oneBotProbeResult.value.ok) appStore.showSuccess(t('admin.qqbot.notices.oneBotProbeSucceeded'))
    else appStore.showError(`${oneBotProbeResult.value.error_code || oneBotProbeResult.value.status}: ${oneBotProbeResult.value.message}`)
    await loadOneBotRuntime()
  } catch (error) {
    oneBotProbeResult.value = null
    oneBotProbeFingerprint.value = ''
    appStore.showError(errorMessage(error, 'admin.qqbot.errors.oneBotProbe'))
  } finally { loading.oneBotProbing = false }
}

function runActiveProbe() {
  if (selectedTransportMode.value === 'botgo') {
    void runProbe()
    return
  }
  void runOneBotProbe()
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

async function saveOneBotConfig() {
  if (!oneBotDraft.value || !oneBotDirty.value) return
  const invalid = validateOneBotDraft(oneBotDraft.value)
  if (invalid.length) { appStore.showError(t(`admin.qqbot.validation.${invalid[0]}`)); return }
  if (oneBotDraft.value.enabled && !oneBotCredentialsReady(oneBotDraft.value)) { appStore.showError(t('admin.qqbot.errors.oneBotCredentialsRequired')); return }
  if (oneBotDraft.value.enabled && !hasCurrentSuccessfulOneBotProbe(oneBotDraft.value)) { appStore.showError(t('admin.qqbot.errors.oneBotProbeRequired')); return }
  loading.oneBotSaving = true
  try {
    const saved = await qqbotAPI.updateOneBotConfig(buildOneBotUpdateRequest(oneBotDraft.value))
    oneBotConfig.value = saved
    oneBotServerDraft.value = oneBotConfigToDraft(saved)
    oneBotDraft.value = oneBotConfigToDraft(saved)
    oneBotProbeResult.value = null
    oneBotProbeFingerprint.value = ''
    appStore.showSuccess(t('admin.qqbot.notices.oneBotSaved'))
    await loadOneBotRuntime()
  } catch (error) {
    appStore.showError(errorMessage(error, 'admin.qqbot.errors.saveOneBotConfig'))
  } finally { loading.oneBotSaving = false }
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
  await Promise.allSettled([loadConfig(), loadOneBotConfig(), loadRuntime(), loadOneBotRuntime(), loadStats(), loadBindings()])
}
onMounted(loadInitial)
</script>

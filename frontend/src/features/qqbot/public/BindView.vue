<template>
  <div class="min-h-screen bg-[#f5f3ed] text-[#171717] dark:bg-[#111111] dark:text-[#f4f1e8]">
    <a href="#qqbot-bind-main" class="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-50 focus:bg-white focus:px-4 focus:py-2 focus:text-black">{{ t('qqbotBind.skip') }}</a>
    <header class="border-b border-black/15 dark:border-white/15">
      <div class="mx-auto flex min-h-16 max-w-5xl items-center justify-between gap-4 px-5">
        <div class="flex items-baseline gap-3"><strong class="text-lg tracking-tight">Sub2API</strong><span class="font-mono text-[11px] uppercase tracking-[0.16em] text-black/55 dark:text-white/55">QQBOT / BIND</span></div>
        <div class="flex gap-1" :aria-label="t('qqbotBind.language')">
          <button type="button" class="rounded-md px-2 py-1 text-xs" :class="locale === 'zh' ? activeLanguageClass : inactiveLanguageClass" @click="changeLocale('zh')">中文</button>
          <button type="button" class="rounded-md px-2 py-1 text-xs" :class="locale === 'en' ? activeLanguageClass : inactiveLanguageClass" @click="changeLocale('en')">EN</button>
        </div>
      </div>
    </header>

    <main id="qqbot-bind-main" class="mx-auto grid max-w-5xl gap-8 px-5 py-10 md:grid-cols-[12rem_minmax(0,1fr)] md:py-16">
      <aside class="hidden md:block" aria-hidden="true">
        <ol class="space-y-5 text-sm">
          <li v-for="(step, index) in steps" :key="step" class="flex items-baseline gap-3" :class="index === stepIndex ? 'font-medium text-black dark:text-white' : index < stepIndex ? 'text-black/55 dark:text-white/55' : 'text-black/35 dark:text-white/35'">
            <span class="font-mono text-[11px]">0{{ index + 1 }}</span><span>{{ step }}</span>
          </li>
        </ol>
      </aside>

      <section class="border border-black/20 bg-[#fffdf7] p-6 shadow-[6px_6px_0_rgba(0,0,0,0.08)] sm:p-9 dark:border-white/20 dark:bg-[#191919] dark:shadow-[6px_6px_0_rgba(255,255,255,0.05)]" aria-live="polite">
        <div v-if="phase === 'loading'" class="py-10 text-center">
          <span class="mx-auto block h-7 w-7 animate-spin rounded-full border-2 border-black/20 border-t-black dark:border-white/20 dark:border-t-white" role="status" :aria-label="t('qqbotBind.loadingTitle')"></span>
          <h1 class="mt-5 text-2xl font-semibold tracking-tight">{{ t('qqbotBind.loadingTitle') }}</h1>
          <p class="mt-2 text-sm text-black/60 dark:text-white/60">{{ t('qqbotBind.loadingDescription') }}</p>
        </div>

        <div v-else-if="phase === 'pending'">
          <p class="font-mono text-[11px] uppercase tracking-[0.16em] text-black/50 dark:text-white/50">{{ t('qqbotBind.pendingKicker') }}</p>
          <h1 class="mt-2 text-3xl font-semibold tracking-tight">{{ t('qqbotBind.pendingTitle') }}</h1>
          <p class="mt-3 max-w-2xl text-sm leading-6 text-black/65 dark:text-white/65">{{ t('qqbotBind.pendingDescription') }}</p>

          <dl class="my-7 divide-y divide-black/10 border-y border-black/15 dark:divide-white/10 dark:border-white/15">
            <div v-for="item in facts" :key="item.label" class="flex flex-col gap-1 py-3 sm:flex-row sm:items-baseline sm:justify-between sm:gap-4"><dt class="font-mono text-[11px] uppercase tracking-wide text-black/50 dark:text-white/50">{{ item.label }}</dt><dd class="text-sm sm:text-right">{{ item.value }}</dd></div>
          </dl>

          <form novalidate class="space-y-5" @submit.prevent="submitBinding">
            <div><label for="qqbot-number" class="mb-2 block text-sm font-medium">{{ t('qqbotBind.qqLabel') }}</label><input id="qqbot-number" v-model="qqNumber" type="text" inputmode="numeric" autocomplete="off" maxlength="12" class="w-full border border-black/25 bg-transparent px-4 py-3 text-base outline-none transition-colors placeholder:text-black/35 focus:border-black focus:ring-2 focus:ring-black/15 dark:border-white/25 dark:placeholder:text-white/35 dark:focus:border-white dark:focus:ring-white/15" :placeholder="t('qqbotBind.qqPlaceholder')" :aria-invalid="Boolean(fieldError)" :aria-describedby="fieldError ? 'qqbot-number-error' : 'qqbot-number-hint'" @blur="touched = true" /><p v-if="fieldError" id="qqbot-number-error" class="mt-2 text-sm text-red-700 dark:text-red-300">{{ fieldError }}</p><p v-else id="qqbot-number-hint" class="mt-2 text-xs leading-5 text-black/55 dark:text-white/55">{{ t('qqbotBind.qqHint') }}</p></div>
            <p v-if="submitError" role="alert" class="border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-800 dark:border-red-900 dark:bg-red-950/30 dark:text-red-200">{{ submitError }}</p>
            <button type="submit" class="w-full bg-[#171717] px-5 py-3 text-sm font-medium text-white transition-colors hover:bg-black focus:outline-none focus:ring-2 focus:ring-black focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-[#f4f1e8] dark:text-black dark:hover:bg-white dark:focus:ring-white dark:focus:ring-offset-[#191919]" :disabled="submitting">{{ submitting ? t('qqbotBind.submitting') : t('qqbotBind.submit') }}</button>
          </form>
        </div>

        <div v-else class="py-7 text-center">
          <p class="font-mono text-[11px] uppercase tracking-[0.16em] text-black/45 dark:text-white/45">QQBOT / BINDING</p>
          <h1 class="mt-3 text-3xl font-semibold tracking-tight">{{ stateCopy.title }}</h1>
          <p class="mx-auto mt-3 max-w-xl text-sm leading-6 text-black/65 dark:text-white/65">{{ stateCopy.description }}</p>
          <dl v-if="phase === 'completed' && completedBalance !== undefined" class="mx-auto mt-6 max-w-sm border-y border-black/15 py-4 dark:border-white/15"><div class="flex items-baseline justify-between gap-4"><dt class="text-xs text-black/50 dark:text-white/50">{{ t('qqbotBind.balanceAfter') }}</dt><dd class="font-mono">{{ formatAmount(completedBalance) }}</dd></div></dl>
          <button v-if="phase === 'network-error'" type="button" class="mt-6 border border-black px-5 py-2.5 text-sm font-medium hover:bg-black hover:text-white dark:border-white dark:hover:bg-white dark:hover:text-black" @click="loadInspection">{{ t('common.retry') }}</button>
        </div>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { setLocale } from '@/i18n'
import qqbotAPI from '../api'
import type { BindingInspection } from '../types'

type Phase = 'loading' | 'invalid-token' | 'network-error' | 'pending' | 'completed' | 'already-completed' | 'expired' | 'revoked' | 'failed' | 'service-disabled'
const route = useRoute()
const { t, locale } = useI18n()
const activeLanguageClass = 'bg-black text-white dark:bg-white dark:text-black'
const inactiveLanguageClass = 'text-black/55 hover:bg-black/5 dark:text-white/55 dark:hover:bg-white/10'
const phase = ref<Phase>('loading')
const inspection = ref<BindingInspection | null>(null)
const qqNumber = ref('')
const touched = ref(false)
const submitting = ref(false)
const submitError = ref('')
const completedBalance = ref<number | undefined>()
const remainingMinutes = ref(0)
let timer: ReturnType<typeof setInterval> | undefined

const token = computed(() => typeof route.query.token === 'string' ? route.query.token.trim() : '')
const steps = computed(() => [t('qqbotBind.steps.verify'), t('qqbotBind.steps.confirm'), t('qqbotBind.steps.complete')])
const stepIndex = computed(() => phase.value === 'pending' ? 1 : ['completed', 'already-completed', 'expired', 'revoked', 'failed', 'service-disabled'].includes(phase.value) ? 2 : 0)
const fieldError = computed(() => touched.value ? validateQQ(qqNumber.value) : '')
const facts = computed(() => [
  { label: t('qqbotBind.email'), value: inspection.value?.masked_email || t('common.unknown') },
  { label: t('qqbotBind.scene'), value: inspection.value?.scene || '—' },
  { label: t('qqbotBind.bonus'), value: formatAmount(inspection.value?.bonus_amount) },
  { label: t('qqbotBind.expires'), value: t('qqbotBind.remainingMinutes', { count: remainingMinutes.value }) },
])
const stateCopy = computed(() => {
  const key = phase.value === 'already-completed' ? 'alreadyCompleted' : phase.value.replace(/-([a-z])/g, (_, char: string) => char.toUpperCase())
  return { title: t(`qqbotBind.states.${key}.title`), description: t(`qqbotBind.states.${key}.description`) }
})

function validateQQ(value: string): string {
  const normalized = value.trim()
  if (!normalized) return t('qqbotBind.validation.required')
  if (!/^\d+$/.test(normalized)) return t('qqbotBind.validation.digits')
  if (normalized.startsWith('0')) return t('qqbotBind.validation.leadingZero')
  if (normalized.length < 5 || normalized.length > 12) return t('qqbotBind.validation.length')
  return ''
}
function formatAmount(value?: number) { return value === undefined ? '—' : new Intl.NumberFormat(locale.value, { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value) }
function errorShape(error: unknown): { status?: number; code?: string; reason?: string; message?: string } { return (error && typeof error === 'object' ? error : {}) as { status?: number; code?: string; reason?: string; message?: string } }
function stopTimer() { if (timer) { clearInterval(timer); timer = undefined } }
function startTimer(expiresAt?: string) {
  stopTimer()
  if (!expiresAt) return
  const update = () => {
    remainingMinutes.value = Math.max(0, Math.ceil((new Date(expiresAt).getTime() - Date.now()) / 60000))
    if (remainingMinutes.value <= 0 && phase.value === 'pending') { phase.value = 'expired'; stopTimer() }
  }
  update(); timer = setInterval(update, 30_000)
}
function applyInspection(result: BindingInspection) {
  if (result.status === 'pending') { phase.value = 'pending'; startTimer(result.expires_at); return }
  if (result.status === 'completed') phase.value = 'already-completed'
  else if (result.status === 'service_disabled') phase.value = 'service-disabled'
  else if (['expired', 'revoked', 'failed'].includes(result.status)) phase.value = result.status as Phase
  else phase.value = 'failed'
}
async function loadInspection() {
  if (token.value.length < 20 || token.value.length > 256) { phase.value = 'invalid-token'; return }
  phase.value = 'loading'
  try { inspection.value = await qqbotAPI.inspectBinding(token.value); applyInspection(inspection.value) }
  catch (error) {
    const err = errorShape(error)
    if (err.reason === 'INVALID_BINDING_TOKEN' || err.code === 'INVALID_BINDING_TOKEN' || err.status === 404) phase.value = 'invalid-token'
    else if (!err.status || err.status >= 500) phase.value = 'network-error'
    else phase.value = 'failed'
  }
}
async function submitBinding() {
  touched.value = true; submitError.value = ''
  if (validateQQ(qqNumber.value) || submitting.value) return
  submitting.value = true
  try {
    const result = await qqbotAPI.completeBinding(token.value, qqNumber.value.trim())
    completedBalance.value = result.balance_after
    phase.value = 'completed'; stopTimer()
  } catch (error) {
    const err = errorShape(error)
    const reason = String(err.reason || err.code || '')
    if (reason === 'INVALID_QQ_NUMBER') submitError.value = t('qqbotBind.validation.format')
    else if (reason === 'BINDING_ALREADY_COMPLETED' || err.status === 409) phase.value = 'already-completed'
    else if (reason === 'BINDING_EXPIRED') phase.value = 'expired'
    else if (reason === 'BINDING_REVOKED') phase.value = 'revoked'
    else if (reason === 'BINDING_DISABLED') phase.value = 'service-disabled'
    else if (!err.status || err.status >= 500) submitError.value = t('qqbotBind.states.networkError.description')
    else submitError.value = t('qqbotBind.states.failed.description')
  } finally { submitting.value = false }
}
async function changeLocale(value: 'zh' | 'en') { await setLocale(value) }
onMounted(loadInspection)
onUnmounted(stopTimer)
</script>

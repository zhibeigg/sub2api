<template>
  <div class="space-y-5">
    <div class="flex flex-wrap items-end justify-between gap-3">
      <div>
        <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.qqbot.bindings.title') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('admin.qqbot.bindings.description', { count: page.total }) }}</p>
      </div>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="$emit('refresh')">{{ t('common.refresh') }}</button>
    </div>

    <form class="grid gap-3 rounded-xl border border-gray-200 bg-white p-4 sm:grid-cols-2 xl:grid-cols-6 dark:border-dark-700 dark:bg-dark-800" @submit.prevent="$emit('search')">
      <div>
        <label class="input-label" for="qqbot-filter-status">{{ t('admin.qqbot.bindings.filterStatus') }}</label>
        <select id="qqbot-filter-status" class="input" :value="filters.status" @change="updateFilter('status', valueOf($event))">
          <option value="">{{ t('common.all') }}</option>
          <option v-for="status in statuses" :key="status" :value="status">{{ t(`admin.qqbot.bindingStatus.${status}`) }}</option>
        </select>
      </div>
      <div>
        <label class="input-label" for="qqbot-filter-scene">{{ t('admin.qqbot.bindings.filterScene') }}</label>
        <select id="qqbot-filter-scene" class="input" :value="filters.scene" @change="updateFilter('scene', valueOf($event))">
          <option value="">{{ t('common.all') }}</option>
          <option value="group">Group</option>
          <option value="c2c">C2C</option>
          <option value="guild">Guild</option>
        </select>
      </div>
      <div class="sm:col-span-2 xl:col-span-2">
        <label class="input-label" for="qqbot-filter-search">{{ t('admin.qqbot.bindings.filterSearch') }}</label>
        <input id="qqbot-filter-search" class="input" :value="filters.search" @input="updateFilter('search', valueOf($event))" />
      </div>
      <div>
        <label class="input-label" for="qqbot-filter-from">{{ t('admin.qqbot.bindings.filterFrom') }}</label>
        <input id="qqbot-filter-from" type="date" class="input" :value="filters.from" @input="updateFilter('from', valueOf($event))" />
      </div>
      <div>
        <label class="input-label" for="qqbot-filter-to">{{ t('admin.qqbot.bindings.filterTo') }}</label>
        <input id="qqbot-filter-to" type="date" class="input" :value="filters.to" @input="updateFilter('to', valueOf($event))" />
      </div>
      <div class="flex flex-wrap gap-2 sm:col-span-2 xl:col-span-6">
        <button type="submit" class="btn bg-primary-600 text-white hover:bg-primary-700">{{ t('common.search') }}</button>
        <button type="button" class="btn btn-secondary" @click="$emit('reset')">{{ t('common.reset') }}</button>
      </div>
    </form>

    <div v-if="error" role="alert" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300">{{ error }}</div>

    <section class="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
      <div v-if="loading" class="space-y-3 p-5" aria-label="loading">
        <div v-for="index in 5" :key="index" class="skeleton h-10"></div>
      </div>
      <div v-else-if="page.items.length === 0" class="p-10 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('admin.qqbot.bindings.empty') }}</div>
      <template v-else>
        <div class="hidden overflow-x-auto lg:block">
          <table class="w-full min-w-[920px] text-left text-sm">
            <thead class="border-b border-gray-200 bg-gray-50 text-xs uppercase tracking-wide text-gray-500 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-400">
              <tr>
                <th class="px-4 py-3">{{ t('admin.qqbot.bindings.status') }}</th>
                <th class="px-4 py-3">{{ t('admin.qqbot.bindings.account') }}</th>
                <th class="px-4 py-3">{{ t('admin.qqbot.bindings.scene') }}</th>
                <th class="px-4 py-3">{{ t('admin.qqbot.bindings.qq') }}</th>
                <th class="px-4 py-3">{{ t('admin.qqbot.bindings.bonus') }}</th>
                <th class="px-4 py-3">{{ t('admin.qqbot.bindings.createdAt') }}</th>
                <th class="px-4 py-3">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="record in page.items" :key="record.id">
                <td class="px-4 py-3"><StatusBadge :status="record.status" /></td>
                <td class="px-4 py-3"><p>{{ record.masked_email || '—' }}</p><p class="font-mono text-xs text-gray-400">{{ record.openid_fingerprint }}</p></td>
                <td class="px-4 py-3">{{ record.scene || '—' }}</td>
                <td class="px-4 py-3 font-mono">{{ record.declared_qq_number || '—' }}</td>
                <td class="px-4 py-3">{{ formatAmount(record.bonus_amount) }}</td>
                <td class="px-4 py-3 whitespace-nowrap">{{ date(record.created_at) }}</td>
                <td class="px-4 py-3">
                  <div class="flex gap-2">
                    <button type="button" class="text-sm text-primary-700 underline underline-offset-2 dark:text-primary-300" @click="selected = record">{{ t('common.view') }}</button>
                    <button v-if="record.status === 'completed'" type="button" class="text-sm text-red-600 underline underline-offset-2 dark:text-red-400" @click="openUnbind(record)">{{ t('admin.qqbot.bindings.unbind') }}</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <ul class="divide-y divide-gray-100 lg:hidden dark:divide-dark-700">
          <li v-for="record in page.items" :key="record.id" class="space-y-3 p-4">
            <div class="flex items-center justify-between gap-3"><StatusBadge :status="record.status" /><span class="text-xs text-gray-500">{{ date(record.created_at) }}</span></div>
            <p class="text-sm font-medium">{{ record.masked_email || '—' }}</p>
            <dl class="grid grid-cols-2 gap-3 text-xs"><div><dt class="text-gray-500">{{ t('admin.qqbot.bindings.scene') }}</dt><dd class="mt-1">{{ record.scene || '—' }}</dd></div><div><dt class="text-gray-500">{{ t('admin.qqbot.bindings.qq') }}</dt><dd class="mt-1">{{ record.declared_qq_number || '—' }}</dd></div></dl>
            <div class="flex gap-3"><button type="button" class="text-sm text-primary-700 underline" @click="selected = record">{{ t('common.view') }}</button><button v-if="record.status === 'completed'" type="button" class="text-sm text-red-600 underline" @click="openUnbind(record)">{{ t('admin.qqbot.bindings.unbind') }}</button></div>
          </li>
        </ul>
      </template>
    </section>

    <nav class="flex items-center justify-center gap-4" :aria-label="t('common.pagination')">
      <button type="button" class="btn btn-secondary btn-sm" :disabled="page.page <= 1" @click="$emit('page', page.page - 1)">{{ t('common.previous') }}</button>
      <span class="text-sm text-gray-500">{{ t('admin.qqbot.bindings.pageInfo', { page: page.page, pages: Math.max(page.pages, 1) }) }}</span>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="page.page >= page.pages" @click="$emit('page', page.page + 1)">{{ t('common.next') }}</button>
    </nav>

    <BaseDialog :show="Boolean(selected)" :title="t('admin.qqbot.bindings.detailTitle')" width="wide" @close="selected = null">
      <dl v-if="selected" class="grid gap-4 sm:grid-cols-2">
        <div v-for="item in detailItems" :key="item.label"><dt class="text-xs text-gray-500 dark:text-dark-400">{{ item.label }}</dt><dd class="mt-1 break-all text-sm text-gray-900 dark:text-white">{{ item.value }}</dd></div>
      </dl>
    </BaseDialog>

    <BaseDialog :show="Boolean(unbindTarget)" :title="t('admin.qqbot.bindings.unbindTitle')" width="narrow" @close="closeUnbind">
      <p class="text-sm text-gray-600 dark:text-dark-300">{{ t('admin.qqbot.bindings.unbindWarning') }}</p>
      <div class="mt-4"><label class="input-label" for="qqbot-unbind-reason">{{ t('admin.qqbot.bindings.unbindReason') }}</label><textarea id="qqbot-unbind-reason" v-model="unbindReason" rows="4" class="input resize-y" :aria-invalid="reasonInvalid"></textarea><p v-if="reasonInvalid" class="input-error-text">{{ t('admin.qqbot.bindings.unbindReasonError') }}</p></div>
      <template #footer><div class="flex justify-end gap-3"><button type="button" class="btn btn-secondary" @click="closeUnbind">{{ t('common.cancel') }}</button><button type="button" class="btn bg-red-600 text-white hover:bg-red-700" :disabled="unbinding" @click="confirmUnbind">{{ unbinding ? t('admin.qqbot.actions.unbinding') : t('admin.qqbot.bindings.unbindConfirm') }}</button></div></template>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { formatDateTime } from '@/utils/format'
import type { QQBotBindingFilters, QQBotBindingPage, QQBotBindingRecord } from '../types'

const props = defineProps<{ page: QQBotBindingPage; filters: QQBotBindingFilters; loading: boolean; error: string; unbinding: boolean }>()
const emit = defineEmits<{ 'update:filters': [value: QQBotBindingFilters]; search: []; reset: []; refresh: []; page: [value: number]; unbind: [id: string, reason: string] }>()
const { t, locale } = useI18n()
const statuses = ['pending', 'completed', 'expired', 'revoked', 'failed']
const selected = ref<QQBotBindingRecord | null>(null)
const unbindTarget = ref<QQBotBindingRecord | null>(null)
const unbindReason = ref('')
const reasonInvalid = ref(false)

const StatusBadge = defineComponent({
  props: { status: { type: String, required: true } },
  setup(statusProps) {
    return () => h('span', { class: ['inline-flex rounded-full border px-2 py-0.5 text-xs', statusProps.status === 'completed' ? 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900 dark:bg-emerald-950/30 dark:text-emerald-300' : statusProps.status === 'failed' ? 'border-red-200 bg-red-50 text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300' : 'border-gray-200 bg-gray-50 text-gray-700 dark:border-dark-600 dark:bg-dark-900 dark:text-dark-200'] }, t(`admin.qqbot.bindingStatus.${statusProps.status}`))
  },
})

const detailItems = computed(() => {
  const record = selected.value
  if (!record) return []
  return [
    { label: t('admin.qqbot.bindings.status'), value: t(`admin.qqbot.bindingStatus.${record.status}`) },
    { label: t('admin.qqbot.bindings.account'), value: record.masked_email || '—' },
    { label: 'OpenID', value: record.openid_fingerprint || '—' },
    { label: t('admin.qqbot.bindings.scene'), value: record.scene || '—' },
    { label: t('admin.qqbot.bindings.source'), value: record.source_id || '—' },
    { label: t('admin.qqbot.bindings.channel'), value: record.channel_id || '—' },
    { label: t('admin.qqbot.bindings.qq'), value: record.declared_qq_number || '—' },
    { label: t('admin.qqbot.bindings.bonus'), value: formatAmount(record.bonus_amount) },
    { label: t('admin.qqbot.bindings.createdAt'), value: date(record.created_at) },
    { label: t('admin.qqbot.bindings.completedAt'), value: date(record.completed_at) },
    { label: t('admin.qqbot.bindings.failureCode'), value: record.failure_code || '—' },
    { label: t('admin.qqbot.bindings.delivery'), value: `${record.email_status || '—'} / ${record.notification_status || '—'}` },
  ]
})

function valueOf(event: Event) { return (event.target as HTMLInputElement | HTMLSelectElement).value }
function updateFilter(key: keyof QQBotBindingFilters, value: string) { emit('update:filters', { ...props.filters, [key]: value }) }
function date(value?: string) { return value ? formatDateTime(value, undefined, locale.value) : '—' }
function formatAmount(value: number) { return new Intl.NumberFormat(locale.value, { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value) }
function openUnbind(record: QQBotBindingRecord) { unbindTarget.value = record; unbindReason.value = ''; reasonInvalid.value = false }
function closeUnbind() { unbindTarget.value = null; unbindReason.value = ''; reasonInvalid.value = false }
function confirmUnbind() {
  const reason = unbindReason.value.trim()
  reasonInvalid.value = reason.length < 3 || reason.length > 300
  if (!unbindTarget.value || reasonInvalid.value) return
  emit('unbind', unbindTarget.value.id, reason)
}
defineExpose({ closeUnbind })
</script>

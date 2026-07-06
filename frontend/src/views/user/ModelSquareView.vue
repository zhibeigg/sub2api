<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Top bar: search + refresh -->
      <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div class="flex items-center gap-3">
          <span
            class="inline-flex items-center gap-1.5 rounded-full bg-primary-500/10 px-3 py-1 text-sm font-medium text-primary-600 dark:text-primary-400"
          >
            <Icon name="grid" size="sm" />
            {{ t('modelSquare.count', { count: totalModels }) }}
          </span>
          <span v-if="filteredModels.length !== totalModels" class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('modelSquare.filtered', { count: filteredModels.length }) }}
          </span>
        </div>

        <div class="flex items-center gap-3">
          <div class="relative w-full sm:w-72">
            <Icon
              name="search"
              size="md"
              class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
            />
            <input
              v-model="searchQuery"
              type="text"
              :placeholder="t('modelSquare.searchPlaceholder')"
              class="input pl-10"
            />
          </div>
          <button
            @click="loadData"
            :disabled="loading"
            class="btn btn-secondary flex-shrink-0"
            :title="t('common.refresh')"
          >
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </div>

      <!-- Mobile filter chips -->
      <div class="flex flex-wrap gap-2 lg:hidden">
        <button
          v-for="opt in platformOptions"
          :key="`m-plat-${opt.value}`"
          class="rounded-full border px-3 py-1 text-xs font-medium transition-colors"
          :class="
            activePlatform === opt.value
              ? 'border-primary-500 bg-primary-500/10 text-primary-600 dark:text-primary-400'
              : 'border-gray-200 text-gray-600 dark:border-dark-700 dark:text-gray-400'
          "
          @click="activePlatform = opt.value"
        >
          {{ opt.label }} · {{ opt.count }}
        </button>
      </div>

      <div class="flex flex-col gap-6 lg:flex-row">
        <!-- Left filter rail -->
        <aside class="hidden w-56 flex-shrink-0 lg:block">
          <div class="sticky top-6 space-y-6">
            <div>
              <h3 class="mb-2 px-1 text-xs font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">
                {{ t('modelSquare.filters.platform') }}
              </h3>
              <div class="space-y-0.5">
                <button
                  v-for="opt in platformOptions"
                  :key="`plat-${opt.value}`"
                  class="flex w-full items-center justify-between rounded-lg px-3 py-2 text-sm transition-colors"
                  :class="
                    activePlatform === opt.value
                      ? 'bg-primary-500/10 font-medium text-primary-600 dark:text-primary-400'
                      : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-800'
                  "
                  @click="activePlatform = opt.value"
                >
                  <span class="flex items-center gap-2">
                    <PlatformIcon
                      v-if="opt.value !== 'all'"
                      :platform="opt.value as GroupPlatform"
                      size="xs"
                    />
                    <Icon v-else name="grid" size="sm" />
                    {{ opt.label }}
                  </span>
                  <span class="text-xs text-gray-400 dark:text-gray-500">{{ opt.count }}</span>
                </button>
              </div>
            </div>

            <div>
              <h3 class="mb-2 px-1 text-xs font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">
                {{ t('modelSquare.filters.billing') }}
              </h3>
              <div class="space-y-0.5">
                <button
                  v-for="opt in billingOptions"
                  :key="`bill-${opt.value}`"
                  class="flex w-full items-center justify-between rounded-lg px-3 py-2 text-sm transition-colors"
                  :class="
                    activeBilling === opt.value
                      ? 'bg-primary-500/10 font-medium text-primary-600 dark:text-primary-400'
                      : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-800'
                  "
                  @click="activeBilling = opt.value"
                >
                  <span>{{ opt.label }}</span>
                  <span class="text-xs text-gray-400 dark:text-gray-500">{{ opt.count }}</span>
                </button>
              </div>
            </div>
          </div>
        </aside>

        <!-- Card grid -->
        <div class="min-w-0 flex-1">
          <!-- Loading -->
          <div v-if="loading" class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
            <div
              v-for="n in 6"
              :key="`sk-${n}`"
              class="h-52 animate-pulse rounded-xl bg-gray-100 dark:bg-dark-800"
            ></div>
          </div>

          <!-- Empty -->
          <div
            v-else-if="filteredModels.length === 0"
            class="flex flex-col items-center justify-center rounded-xl border border-dashed border-gray-200 py-20 dark:border-dark-700"
          >
            <Icon name="inbox" size="xl" class="mb-3 h-12 w-12 text-gray-300 dark:text-gray-600" />
            <p class="text-sm text-gray-500 dark:text-gray-400">
              {{ searchQuery || activePlatform !== 'all' || activeBilling !== 'all' ? t('modelSquare.noMatch') : t('modelSquare.empty') }}
            </p>
          </div>

          <!-- Grid -->
          <div v-else class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
            <article
              v-for="model in filteredModels"
              :key="model.key"
              class="group flex flex-col rounded-xl border border-gray-200 bg-white p-5 transition-all duration-200 hover:-translate-y-0.5 hover:border-gray-300 hover:shadow-lg dark:border-dark-700 dark:bg-dark-800/60 dark:hover:border-dark-600"
            >
              <!-- Header -->
              <div class="mb-3">
                <div class="mb-2.5 flex items-center justify-between gap-2">
                  <div
                    class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg bg-gray-100 dark:bg-dark-700"
                  >
                    <ModelIcon :model="model.name" size="20px" />
                  </div>
                  <span
                    class="flex-shrink-0 rounded-md border border-gray-200 bg-gray-50 px-2 py-0.5 text-[10px] font-medium uppercase text-gray-500 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-400"
                  >
                    {{ billingLabel(model.billingMode) }}
                  </span>
                </div>
                <button
                  class="group/name flex w-full items-start gap-1.5 text-left"
                  :title="model.name + ' · ' + t('modelSquare.clickToCopy')"
                  @click="copyModel(model.name)"
                >
                  <span class="min-w-0 flex-1 break-words font-semibold leading-snug text-gray-900 dark:text-white">
                    {{ model.name }}
                  </span>
                  <Icon
                    :name="copiedName === model.name ? 'check' : 'copy'"
                    size="xs"
                    class="mt-1 flex-shrink-0"
                    :class="copiedName === model.name ? 'text-green-500' : 'text-gray-300 group-hover/name:text-primary-500'"
                  />
                </button>
              </div>

              <!-- Platforms -->
              <div class="mb-4 flex flex-wrap gap-1.5">
                <span
                  v-for="p in model.platforms"
                  :key="`${model.key}-${p}`"
                  class="inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-[11px] font-medium uppercase"
                  :class="platformBadgeClass(p)"
                >
                  <PlatformIcon :platform="p as GroupPlatform" size="xs" />
                  {{ platformLabel(p) }}
                </span>
              </div>

              <!-- Pricing -->
              <div class="mt-auto space-y-1.5 border-t border-gray-100 pt-4 text-sm dark:border-dark-700">
                <template v-if="!model.pricing">
                  <p class="text-xs text-gray-400 dark:text-gray-500">{{ t('modelSquare.noPricing') }}</p>
                </template>

                <template v-else-if="model.billingMode === 'token'">
                  <div v-if="model.pricing.input_price != null" class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('availableChannels.pricing.inputPrice') }}</span>
                    <span class="font-medium text-gray-900 dark:text-gray-100">
                      {{ formatScaled(model.pricing.input_price, PER_M) }}
                      <span class="text-xs font-normal text-gray-400">{{ t('availableChannels.pricing.unitPerMillion') }}</span>
                    </span>
                  </div>
                  <div v-if="model.pricing.output_price != null" class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('availableChannels.pricing.outputPrice') }}</span>
                    <span class="font-medium text-gray-900 dark:text-gray-100">
                      {{ formatScaled(model.pricing.output_price, PER_M) }}
                      <span class="text-xs font-normal text-gray-400">{{ t('availableChannels.pricing.unitPerMillion') }}</span>
                    </span>
                  </div>
                  <div
                    v-if="model.pricing.cache_read_price != null && model.pricing.cache_read_price > 0"
                    class="flex justify-between"
                  >
                    <span class="text-gray-500 dark:text-gray-400">{{ t('availableChannels.pricing.cacheReadPrice') }}</span>
                    <span class="text-gray-700 dark:text-gray-300">
                      {{ formatScaled(model.pricing.cache_read_price, PER_M) }}
                      <span class="text-xs text-gray-400">{{ t('availableChannels.pricing.unitPerMillion') }}</span>
                    </span>
                  </div>
                </template>

                <template v-else-if="model.billingMode === 'per_request'">
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('availableChannels.pricing.perRequestPrice') }}</span>
                    <span class="font-medium text-gray-900 dark:text-gray-100">
                      {{ formatScaled(model.pricing.per_request_price, 1) }}
                      <span class="text-xs font-normal text-gray-400">{{ t('availableChannels.pricing.unitPerRequest') }}</span>
                    </span>
                  </div>
                </template>

                <template v-else-if="model.billingMode === 'image'">
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('availableChannels.pricing.imageOutputPrice') }}</span>
                    <span class="font-medium text-gray-900 dark:text-gray-100">
                      {{ formatScaled(model.pricing.image_output_price, 1) }}
                      <span class="text-xs font-normal text-gray-400">{{ t('availableChannels.pricing.unitPerRequest') }}</span>
                    </span>
                  </div>
                </template>

                <!-- Group multipliers -->
                <div v-if="model.groups.length > 0" class="flex flex-wrap gap-1 pt-1">
                  <span
                    v-for="g in model.groups"
                    :key="`${model.key}-g-${g.id}`"
                    class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px]"
                    :class="
                      g.isExclusive
                        ? 'bg-purple-500/10 text-purple-600 dark:text-purple-300'
                        : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
                    "
                    :title="g.name"
                  >
                    <Icon :name="g.isExclusive ? 'shield' : 'globe'" size="xs" />
                    {{ formatMultiplier(g.rate) }}
                  </span>
                </div>
              </div>
            </article>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import userChannelsAPI, {
  type UserAvailableChannel,
  type UserSupportedModelPricing
} from '@/api/channels'
import userGroupsAPI from '@/api/groups'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatScaled } from '@/utils/pricing'
import { platformBadgeClass, platformLabel } from '@/utils/platformColors'
import {
  BILLING_MODE_TOKEN,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_IMAGE,
  type BillingMode
} from '@/constants/channel'
import type { GroupPlatform } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

const PER_M = 1_000_000

interface SquareGroup {
  id: number
  name: string
  rate: number
  isExclusive: boolean
}

interface SquareModel {
  key: string
  name: string
  platforms: string[]
  billingMode: BillingMode
  pricing: UserSupportedModelPricing | null
  groups: SquareGroup[]
}

const channels = ref<UserAvailableChannel[]>([])
const userGroupRates = ref<Record<number, number>>({})
const loading = ref(false)
const searchQuery = ref('')
const activePlatform = ref<string>('all')
const activeBilling = ref<string>('all')
const copiedName = ref('')
let copyTimer: ReturnType<typeof setTimeout> | null = null

/**
 * Flatten channels → platforms → supported_models into deduped model cards.
 * Same model name is merged: platforms unioned, groups unioned, pricing takes
 * the first non-null representative (same-name pricing is consistent upstream).
 */
const allModels = computed<SquareModel[]>(() => {
  const map = new Map<string, SquareModel>()
  for (const ch of channels.value) {
    for (const section of ch.platforms) {
      const sectionGroups: SquareGroup[] = section.groups.map((g) => ({
        id: g.id,
        name: g.name,
        rate: userGroupRates.value[g.id] ?? g.rate_multiplier,
        isExclusive: g.is_exclusive
      }))
      for (const m of section.supported_models) {
        const key = m.name.toLowerCase()
        let entry = map.get(key)
        if (!entry) {
          entry = {
            key,
            name: m.name,
            platforms: [],
            billingMode: (m.pricing?.billing_mode as BillingMode) || BILLING_MODE_TOKEN,
            pricing: m.pricing,
            groups: []
          }
          map.set(key, entry)
        }
        const plat = m.platform || section.platform
        if (plat && !entry.platforms.includes(plat)) entry.platforms.push(plat)
        if (!entry.pricing && m.pricing) {
          entry.pricing = m.pricing
          entry.billingMode = (m.pricing.billing_mode as BillingMode) || BILLING_MODE_TOKEN
        }
        for (const g of sectionGroups) {
          if (!entry.groups.some((x) => x.id === g.id)) entry.groups.push(g)
        }
      }
    }
  }
  return Array.from(map.values()).sort((a, b) => a.name.localeCompare(b.name))
})

const totalModels = computed(() => allModels.value.length)

const platformOptions = computed(() => {
  const counts = new Map<string, number>()
  for (const m of allModels.value) {
    for (const p of m.platforms) counts.set(p, (counts.get(p) ?? 0) + 1)
  }
  const opts = [{ value: 'all', label: t('modelSquare.filters.all'), count: allModels.value.length }]
  for (const [value, count] of Array.from(counts.entries()).sort((a, b) => b[1] - a[1])) {
    opts.push({ value, label: platformLabel(value), count })
  }
  return opts
})

const billingOptions = computed(() => {
  const counts = new Map<string, number>()
  for (const m of allModels.value) counts.set(m.billingMode, (counts.get(m.billingMode) ?? 0) + 1)
  return [
    { value: 'all', label: t('modelSquare.filters.all'), count: allModels.value.length },
    { value: BILLING_MODE_TOKEN, label: t('availableChannels.pricing.billingModeToken'), count: counts.get(BILLING_MODE_TOKEN) ?? 0 },
    { value: BILLING_MODE_PER_REQUEST, label: t('availableChannels.pricing.billingModePerRequest'), count: counts.get(BILLING_MODE_PER_REQUEST) ?? 0 },
    { value: BILLING_MODE_IMAGE, label: t('availableChannels.pricing.billingModeImage'), count: counts.get(BILLING_MODE_IMAGE) ?? 0 }
  ].filter((o) => o.value === 'all' || o.count > 0)
})

const filteredModels = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return allModels.value.filter((m) => {
    if (activePlatform.value !== 'all' && !m.platforms.includes(activePlatform.value)) return false
    if (activeBilling.value !== 'all' && m.billingMode !== activeBilling.value) return false
    if (q && !m.name.toLowerCase().includes(q) && !m.platforms.some((p) => p.toLowerCase().includes(q))) return false
    return true
  })
})

function billingLabel(mode: BillingMode): string {
  switch (mode) {
    case BILLING_MODE_PER_REQUEST:
      return t('availableChannels.pricing.billingModePerRequest')
    case BILLING_MODE_IMAGE:
      return t('availableChannels.pricing.billingModeImage')
    default:
      return t('availableChannels.pricing.billingModeToken')
  }
}

function formatMultiplier(rate: number): string {
  const s = Number(rate.toFixed(3)).toString()
  return `${s}x`
}

async function copyModel(name: string) {
  try {
    await navigator.clipboard.writeText(name)
  } catch {
    const ta = document.createElement('textarea')
    ta.value = name
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    document.body.removeChild(ta)
  }
  copiedName.value = name
  if (copyTimer) clearTimeout(copyTimer)
  copyTimer = setTimeout(() => {
    copiedName.value = ''
  }, 1500)
}

async function loadData() {
  loading.value = true
  try {
    const [list, rates] = await Promise.all([
      userChannelsAPI.getAvailable(),
      userGroupsAPI.getUserGroupRates().catch((err: unknown) => {
        console.error('Failed to load user group rates:', err)
        return {} as Record<number, number>
      })
    ])
    channels.value = list
    userGroupRates.value = rates
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

onMounted(loadData)
</script>

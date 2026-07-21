<template>
  <AppLayout full-height>
    <div class="space-y-6 lg:flex lg:h-full lg:min-h-0 lg:flex-col lg:gap-6 lg:space-y-0">
      <!-- Top bar: count + search + refresh -->
      <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between lg:flex-none">
        <div class="flex flex-wrap items-center gap-3">
          <span
            class="inline-flex items-center gap-1.5 rounded-full bg-primary-500/10 px-3 py-1 text-sm font-medium text-primary-600 dark:text-primary-400"
          >
            <Icon name="grid" size="sm" />
            {{ t('modelSquare.count', { count: totalModels }) }}
          </span>
          <span v-if="filteredModels.length !== totalModels" class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('modelSquare.filtered', { count: filteredModels.length }) }}
          </span>
          <button
            v-if="hasActiveFilter"
            class="inline-flex items-center gap-1 rounded-full border border-gray-200 px-2.5 py-1 text-xs text-gray-500 transition-colors hover:border-primary-400 hover:text-primary-600 dark:border-dark-600 dark:text-gray-400"
            @click="clearFilters"
          >
            <Icon name="x" size="xs" />
            {{ t('modelSquare.clearFilters') }}
          </button>
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
            @click="refreshNow"
            :disabled="loading"
            class="btn btn-secondary flex-shrink-0"
            :title="t('common.refresh')"
          >
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </div>

      <!-- Mobile filter chips (provider + group) -->
      <div class="space-y-2 lg:hidden">
        <div class="flex flex-wrap gap-2">
          <button
            v-for="opt in providerOptions"
            :key="`m-prov-${opt.value}`"
            class="rounded-full border px-3 py-1 text-xs font-medium transition-colors"
            :class="chipClass(activeProvider === opt.value, opt.count === 0)"
            :disabled="opt.count === 0"
            @click="activeProvider = opt.value"
          >
            {{ opt.label }} · {{ opt.count }}
          </button>
        </div>
        <div v-if="groupOptions.length > 1" class="flex flex-wrap gap-2">
          <button
            v-for="opt in groupOptions"
            :key="`m-grp-${opt.value}`"
            class="rounded-full border px-3 py-1 text-xs font-medium transition-colors"
            :class="chipClass(activeGroup === opt.value, false)"
            @click="activeGroup = opt.value"
          >
            {{ opt.label }} · {{ opt.count }}
          </button>
        </div>
      </div>

      <div class="flex flex-col gap-6 lg:min-h-0 lg:flex-1 lg:flex-row lg:overflow-hidden">
        <!-- Left filter rail -->
        <aside
          class="model-square-scroll-pane hidden flex-shrink-0 overflow-y-auto overscroll-contain lg:block lg:h-full lg:min-h-0 lg:w-80 xl:w-[22rem]"
          tabindex="0"
          data-testid="model-filter-scroll-region"
          :aria-label="t('modelSquare.filterRegion')"
        >
          <div class="space-y-5 pr-2">
            <!-- Provider -->
            <FilterSection :title="t('modelSquare.filters.provider')" :columns="2">
              <button
                v-for="opt in providerOptions"
                :key="`prov-${opt.value}`"
                class="flex min-w-0 w-full items-center justify-between gap-2 rounded-lg border px-2.5 py-1.5 text-xs transition-colors"
                :class="railClass(activeProvider === opt.value, opt.count === 0)"
                :disabled="opt.count === 0"
                @click="activeProvider = opt.value"
              >
                <span class="flex min-w-0 items-center gap-2">
                  <Icon v-if="opt.value === 'all'" name="grid" size="sm" />
                  <ModelIcon v-else :model="opt.keyword" size="16px" class="flex-shrink-0" />
                  <span class="truncate" :class="opt.colorClass">
                    {{ opt.label }}
                  </span>
                </span>
                <span class="flex-shrink-0 rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium tabular-nums text-gray-500 dark:bg-dark-700 dark:text-gray-400">{{ opt.count }}</span>
              </button>
            </FilterSection>

            <!-- Group -->
            <FilterSection v-if="groupOptions.length > 1" :title="t('modelSquare.filters.group')" :columns="2">
              <button
                v-for="opt in groupOptions"
                :key="`grp-${opt.value}`"
                class="flex min-w-0 w-full items-center justify-between gap-2 rounded-lg border px-2.5 py-1.5 text-xs transition-colors"
                :class="railClass(activeGroup === opt.value, false)"
                @click="activeGroup = opt.value"
              >
                <span class="flex min-w-0 items-center gap-2">
                  <Icon v-if="opt.value === 'all'" name="grid" size="sm" />
                  <ModelIcon
                    v-else
                    :model="groupBrand(opt.platform, opt.label).keyword"
                    size="16px"
                    class="flex-shrink-0"
                  />
                  <span
                    class="truncate"
                    :class="opt.value === 'all' ? '' : groupBrand(opt.platform, opt.label).colorClass"
                  >
                    {{ opt.label }}
                  </span>
                  <Icon
                    v-if="opt.value !== 'all' && opt.exclusive"
                    name="shield"
                    size="xs"
                    class="flex-shrink-0 text-purple-500"
                  />
                </span>
                <span class="flex-shrink-0 rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium tabular-nums text-gray-500 dark:bg-dark-700 dark:text-gray-400">{{ opt.count }}</span>
              </button>
            </FilterSection>

            <!-- Endpoint -->
            <FilterSection v-if="endpointOptions.length > 2" :title="t('modelSquare.filters.endpoint')" :columns="2">
              <button
                v-for="opt in endpointOptions"
                :key="`ep-${opt.value}`"
                class="flex min-w-0 w-full items-center justify-between gap-2 rounded-lg border px-2.5 py-1.5 text-xs transition-colors"
                :class="railClass(activeEndpoint === opt.value, false)"
                @click="activeEndpoint = opt.value"
              >
                <span>{{ opt.label }}</span>
                <span class="flex-shrink-0 rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium tabular-nums text-gray-500 dark:bg-dark-700 dark:text-gray-400">{{ opt.count }}</span>
              </button>
            </FilterSection>

            <!-- Billing -->
            <FilterSection :title="t('modelSquare.filters.billing')" :columns="2">
              <button
                v-for="opt in billingOptions"
                :key="`bill-${opt.value}`"
                class="flex min-w-0 w-full items-center justify-between gap-2 rounded-lg border px-2.5 py-1.5 text-xs transition-colors"
                :class="railClass(activeBilling === opt.value, false)"
                @click="activeBilling = opt.value"
              >
                <span>{{ opt.label }}</span>
                <span class="flex-shrink-0 rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium tabular-nums text-gray-500 dark:bg-dark-700 dark:text-gray-400">{{ opt.count }}</span>
              </button>
            </FilterSection>
          </div>
        </aside>

        <!-- Card grid -->
        <div
          class="model-square-scroll-pane min-w-0 flex-1 overflow-y-auto overscroll-contain lg:h-full lg:min-h-0 lg:pr-1"
          tabindex="0"
          data-testid="model-results-scroll-region"
          :aria-label="t('modelSquare.resultsRegion')"
        >
          <div v-if="loading" class="grid grid-cols-1 gap-4 sm:grid-cols-2 2xl:grid-cols-3">
            <div v-for="n in 6" :key="`sk-${n}`" class="h-56 animate-pulse rounded-xl bg-gray-100 dark:bg-dark-800"></div>
          </div>

          <div
            v-else-if="filteredModels.length === 0"
            class="flex flex-col items-center justify-center rounded-xl border border-dashed border-gray-200 py-20 dark:border-dark-700"
          >
            <Icon name="inbox" size="xl" class="mb-3 h-12 w-12 text-gray-300 dark:text-gray-600" />
            <p class="mb-3 text-sm text-gray-500 dark:text-gray-400">
              {{ hasActiveFilter ? t('modelSquare.noMatch') : t('modelSquare.empty') }}
            </p>
            <button v-if="hasActiveFilter" class="btn btn-secondary btn-sm" @click="clearFilters">
              {{ t('modelSquare.clearFilters') }}
            </button>
          </div>

          <div v-else class="grid grid-cols-1 gap-4 sm:grid-cols-2 2xl:grid-cols-3">
            <article
              v-for="model in filteredModels"
              :key="model.key"
              class="group flex cursor-pointer flex-col rounded-xl border bg-white p-5 transition-all duration-200 hover:-translate-y-0.5 hover:shadow-lg focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 dark:bg-dark-800/60"
              tabindex="0"
              :aria-label="t('modelSquare.openDetails', { model: model.name })"
              :class="
                selectedModel?.key === model.key
                  ? 'border-primary-400 ring-2 ring-primary-500/15 dark:border-primary-600'
                  : 'border-gray-200 hover:border-gray-300 dark:border-dark-700 dark:hover:border-dark-600'
              "
              @click="openModelDetails(model)"
              @keydown.enter.self="openModelDetails(model)"
              @keydown.space.self.prevent="openModelDetails(model)"
            >
              <!-- Header -->
              <div class="mb-3">
                <div class="mb-2.5 flex items-center justify-between gap-2">
                  <div class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg bg-gray-100 dark:bg-dark-700">
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
                  @click.stop="copyModel(model.name)"
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

              <!-- Model brand (not the internal account platform) -->
              <div class="mb-4 flex flex-wrap gap-1.5">
                <button
                  type="button"
                  class="inline-flex min-h-8 items-center gap-1.5 rounded-md border border-gray-200 bg-gray-50 px-2.5 py-1 text-xs font-medium transition-colors hover:border-primary-300 hover:bg-primary-50/50 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 dark:border-dark-700 dark:bg-dark-800 dark:hover:border-primary-700 dark:hover:bg-dark-700"
                  :class="modelBrand(model).colorClass"
                  :aria-label="t('modelSquare.openDetails', { model: model.name })"
                  @click.stop="openModelDetails(model)"
                >
                  <ModelIcon :model="modelBrand(model).keyword" size="14px" />
                  {{ modelBrand(model).label }}
                  <Icon name="chevronRight" size="xs" class="text-gray-400 dark:text-gray-500" />
                </button>
              </div>

              <!-- Pricing -->
              <div class="space-y-1.5 border-t border-gray-100 pt-4 text-sm dark:border-dark-700">
                <template v-if="!model.pricing && model.imageTiers.length === 0">
                  <p class="text-xs text-gray-400 dark:text-gray-500">{{ t('modelSquare.noPricing') }}</p>
                </template>

                <template v-else-if="model.billingMode === 'image'">
                  <div
                    v-if="model.imageTiers.length > 0"
                    class="flex flex-wrap items-baseline gap-x-3 gap-y-1"
                  >
                    <span
                      v-for="tier in model.imageTiers"
                      :key="`${model.key}-${tier.tier}`"
                      class="inline-flex items-baseline gap-1"
                    >
                      <span class="text-[11px] font-medium uppercase text-gray-400 dark:text-gray-500">
                        {{ tier.tier }}
                      </span>
                      <span class="font-semibold tabular-nums text-rose-600 dark:text-rose-400">
                        {{ formatImagePriceRange(tier) }}
                      </span>
                    </span>
                    <span class="text-xs text-gray-400 dark:text-gray-500">
                      {{ t('availableChannels.pricing.unitPerImage') }}
                    </span>
                  </div>
                  <div v-else-if="model.pricing && mediaPrice(model.pricing) != null" class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('availableChannels.pricing.imageOutputPrice') }}</span>
                    <span class="font-medium text-gray-900 dark:text-gray-100">
                      {{ formatScaled(mediaPrice(model.pricing), 1) }}
                      <span class="text-xs font-normal text-gray-400">{{ t('availableChannels.pricing.unitPerImage') }}</span>
                    </span>
                  </div>
                </template>

                <template v-else-if="model.billingMode === 'token' && model.pricing">
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
                  <div v-if="model.pricing.cache_write_price != null && model.pricing.cache_write_price > 0" class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('availableChannels.pricing.cacheWritePrice') }}</span>
                    <span class="text-gray-700 dark:text-gray-300">
                      {{ formatScaled(model.pricing.cache_write_price, PER_M) }}
                      <span class="text-xs text-gray-400">{{ t('availableChannels.pricing.unitPerMillion') }}</span>
                    </span>
                  </div>
                  <div v-if="model.pricing.cache_read_price != null && model.pricing.cache_read_price > 0" class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('availableChannels.pricing.cacheReadPrice') }}</span>
                    <span class="text-gray-700 dark:text-gray-300">
                      {{ formatScaled(model.pricing.cache_read_price, PER_M) }}
                      <span class="text-xs text-gray-400">{{ t('availableChannels.pricing.unitPerMillion') }}</span>
                    </span>
                  </div>
                </template>

                <template v-else-if="model.billingMode === 'per_request' && model.pricing">
                  <div class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('availableChannels.pricing.perRequestPrice') }}</span>
                    <span class="font-medium text-gray-900 dark:text-gray-100">
                      {{ formatScaled(model.pricing.per_request_price, 1) }}
                      <span class="text-xs font-normal text-gray-400">{{ t('availableChannels.pricing.unitPerRequest') }}</span>
                    </span>
                  </div>
                </template>

                <template v-else-if="model.billingMode === 'video' && model.pricing">
                  <div v-if="mediaPrice(model.pricing) != null" class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('availableChannels.pricing.perSecondPrice') }}</span>
                    <span class="font-medium text-gray-900 dark:text-gray-100">
                      {{ formatScaled(mediaPrice(model.pricing), 1) }}
                      <span class="text-xs font-normal text-gray-400">/second</span>
                    </span>
                  </div>
                </template>
              </div>

              <!-- Available groups (name + badge + multiplier) -->
              <div v-if="model.groups.length > 0" class="mt-auto border-t border-gray-100 pt-3 dark:border-dark-700">
                <div class="mb-2 text-[10px] font-medium uppercase tracking-wide text-gray-400 dark:text-gray-500">
                  {{ t('modelSquare.availableGroups') }}
                </div>

                <div v-if="model.billingMode === 'image'" class="flex flex-wrap gap-2">
                  <div
                    v-for="g in model.groups"
                    :key="`${model.key}-g-${g.id}`"
                    class="inline-flex max-w-full items-center gap-1.5 rounded-md border px-2 py-1 text-xs"
                    :class="
                      g.isExclusive
                        ? 'border-purple-200 bg-purple-50 text-purple-700 dark:border-purple-800 dark:bg-purple-950/30 dark:text-purple-200'
                        : 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900 dark:bg-emerald-950/30 dark:text-emerald-300'
                    "
                  >
                    <ModelIcon
                      :model="groupBrand(g.platform, g.name).keyword"
                      size="14px"
                      class="flex-shrink-0"
                    />
                    <span class="max-w-36 truncate font-medium" :title="g.name">{{ g.name }}</span>
                    <span
                      class="flex-shrink-0 rounded px-1.5 py-0.5 font-semibold tabular-nums"
                      :class="multiplierBadgeClass(g.rate)"
                    >
                      {{ formatMultiplier(g.rate) }}
                    </span>
                  </div>
                </div>

                <div v-else class="space-y-1">
                  <div
                    v-for="g in model.groups"
                    :key="`${model.key}-g-${g.id}`"
                    class="flex items-center justify-between gap-2 rounded-md px-2 py-1 text-xs"
                    :class="
                      g.isExclusive
                        ? 'bg-purple-500/10'
                        : 'bg-gray-50 dark:bg-dark-700/60'
                    "
                  >
                    <span class="flex min-w-0 items-center gap-1.5">
                      <ModelIcon
                        :model="groupBrand(g.platform, g.name).keyword"
                        size="15px"
                        class="flex-shrink-0"
                      />
                      <span
                        class="truncate font-medium"
                        :class="groupBrand(g.platform, g.name).colorClass"
                        :title="g.name"
                      >
                        {{ g.name }}
                      </span>
                      <span
                        v-if="g.isExclusive"
                        class="flex-shrink-0 rounded bg-purple-500/20 px-1 text-[9px] font-medium uppercase text-purple-700 dark:text-purple-200"
                      >
                        {{ t('availableChannels.exclusive') }}
                      </span>
                    </span>
                    <span
                      class="flex-shrink-0 rounded px-1.5 py-0.5 font-semibold tabular-nums"
                      :class="multiplierBadgeClass(g.rate)"
                    >
                      {{ formatMultiplier(g.rate) }}
                    </span>
                  </div>
                </div>
              </div>
            </article>
          </div>
        </div>
      </div>
    </div>

    <ModelDetailsDrawer
      :show="selectedModel != null"
      :model="selectedModel"
      :copied="selectedModel != null && copiedName === selectedModel.name"
      @close="closeModelDetails"
      @copy="copyModel"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref, watch, type VNode } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import ModelDetailsDrawer from '@/components/channels/ModelDetailsDrawer.vue'
import userChannelsAPI, {
  type UserAvailableChannel,
  type UserSupportedModelPricing
} from '@/api/channels'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatScaled } from '@/utils/pricing'
import { resolveGroupBrand, BRAND_LABEL } from '@/utils/groupBrand'
import { useVisibleAutoRefresh } from '@/composables/useVisibleAutoRefresh'
import {
  effectiveModelBillingMode,
  formatImagePriceRange,
  resolveImageTierPrices
} from './modelSquarePricing'
import {
  BILLING_MODE_TOKEN,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_IMAGE,
  BILLING_MODE_VIDEO,
  type BillingMode
} from '@/constants/channel'
import {
  endpointFilterKeys,
  resolveModelEndpoints,
  type ModelSquareGroup,
  type ModelSquareModel,
  type ModelSquareRoute
} from './modelSquareDetails'

const { t } = useI18n()
const appStore = useAppStore()

const PER_M = 1_000_000
const mediaPrice = (pricing: UserSupportedModelPricing): number | null =>
  pricing.per_request_price ?? pricing.image_output_price ?? null
const AUTO_REFRESH_INTERVAL_MS = 60_000

// Lightweight filter section wrapper (title + slotted list).
const FilterSection = (props: { title: string; columns?: 1 | 2 }, ctx: { slots: { default?: () => VNode[] } }) =>
  h('div', {}, [
    h(
      'h3',
      { class: 'mb-2 px-1 text-xs font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500' },
      props.title
    ),
    h(
      'div',
      { class: props.columns === 2 ? 'grid grid-cols-2 gap-2' : 'space-y-0.5' },
      ctx.slots.default?.() ?? []
    )
  ])

const channels = ref<UserAvailableChannel[]>([])
const loading = ref(false)
const searchQuery = ref('')
const activeProvider = ref<string>('all')
const activeGroup = ref<string>('all')
const activeEndpoint = ref<string>('all')
const activeBilling = ref<string>('all')
const copiedName = ref('')
const selectedModel = ref<ModelSquareModel | null>(null)
let copyTimer: ReturnType<typeof setTimeout> | null = null
let abortController: AbortController | null = null
let isFetching = false

interface LoadDataOptions {
  showLoading?: boolean
  silent?: boolean
}

function isAbortError(error: unknown): boolean {
  const maybeError = error as { name?: string; code?: string }
  return maybeError.name === 'AbortError' || maybeError.name === 'CanceledError' || maybeError.code === 'ERR_CANCELED'
}

function shouldPreferPricing(
  current: UserSupportedModelPricing | null,
  candidate: UserSupportedModelPricing | null,
  billingMode: BillingMode
): boolean {
  if (!candidate) return false
  if (!current) return true

  const matches = (pricing: UserSupportedModelPricing): boolean => {
    if (billingMode === BILLING_MODE_IMAGE) {
      return pricing.billing_mode === BILLING_MODE_IMAGE || pricing.billing_mode === BILLING_MODE_PER_REQUEST
    }
    return pricing.billing_mode === billingMode
  }
  return !matches(current) && matches(candidate)
}

/** Flatten channels → platforms → supported_models into cards split by effective billing mode. */
const allModels = computed<ModelSquareModel[]>(() => {
  const map = new Map<string, ModelSquareModel>()
  for (const ch of channels.value) {
    for (const section of ch.platforms) {
      const sectionGroups: ModelSquareGroup[] = section.groups.map((g) => ({
        id: g.id,
        name: g.name,
        rate: g.rate_multiplier,
        isExclusive: g.is_exclusive,
        platform: g.platform || section.platform,
        allowMessagesDispatch: g.allow_messages_dispatch ?? false,
        allowImageGeneration: g.allow_image_generation ?? false,
        allowVideoGeneration: g.allow_video_generation ?? false,
        imageBillingEnabled: g.image_billing_enabled ?? false,
        imageRateIndependent: g.image_rate_independent ?? false,
        imageRateMultiplier: g.image_rate_multiplier ?? 1,
        imagePrice1K: g.image_price_1k ?? null,
        imagePrice2K: g.image_price_2k ?? null,
        imagePrice4K: g.image_price_4k ?? null,
        videoBillingEnabled: g.video_billing_enabled ?? false,
        videoRateIndependent: g.video_rate_independent ?? false,
        videoRateMultiplier: g.video_rate_multiplier ?? 1,
        videoPrice480P: g.video_price_480p ?? null,
        videoPrice720P: g.video_price_720p ?? null,
        videoPrice1080P: g.video_price_1080p ?? null
      }))

      for (const m of section.supported_models) {
        const mediaType = m.media_type ?? ''
        const eligibleGroups =
          mediaType === 'image'
            ? sectionGroups.filter((group) => group.allowImageGeneration)
            : mediaType === 'video'
              ? sectionGroups.filter((group) => group.allowVideoGeneration)
              : sectionGroups
        if (eligibleGroups.length === 0) continue

        const rateByGroupID = new Map((m.group_rates ?? []).map((rate) => [rate.group_id, rate]))
        const groupsByMode = new Map<BillingMode, ModelSquareGroup[]>()
        for (const group of eligibleGroups) {
          const mode = effectiveModelBillingMode(mediaType, m.pricing, group)
          const rateSnapshot = rateByGroupID.get(group.id)
          if (!rateSnapshot) continue
          let effectiveRate = rateSnapshot.token_rate_multiplier
          if (mode === BILLING_MODE_IMAGE) {
            effectiveRate = rateSnapshot.image_rate_multiplier
          } else if (mode === BILLING_MODE_VIDEO) {
            effectiveRate = rateSnapshot.video_rate_multiplier
          }
          const displayGroup = { ...group, rate: effectiveRate }
          const bucket = groupsByMode.get(mode) ?? []
          bucket.push(displayGroup)
          groupsByMode.set(mode, bucket)
        }

        for (const [billingMode, modeGroups] of groupsByMode) {
          const baseKey = m.name.toLowerCase()
          const key = mediaType ? `${baseKey}::${billingMode}` : baseKey
          let entry = map.get(key)
          if (!entry) {
            entry = {
              key,
              name: m.name,
              platforms: [],
              brand: resolveGroupBrand(m.platform || section.platform, m.name).brand,
              mediaType,
              billingMode,
              pricing: m.pricing,
              imageTiers: [],
              groups: [],
              groupIds: [],
              routes: [],
              endpoints: [],
              endpointDetails: []
            }
            map.set(key, entry)
          } else if (shouldPreferPricing(entry.pricing, m.pricing, billingMode)) {
            entry.pricing = m.pricing
          }

          const platform = m.platform || section.platform
          if (platform && !entry.platforms.includes(platform)) entry.platforms.push(platform)
          for (const group of modeGroups) {
            if (!entry.groups.some((item) => item.id === group.id)) {
              entry.groups.push(group)
              entry.groupIds.push(group.id)
            }

            const routeKey = `${ch.name}|${platform}|${group.id}|${billingMode}`
            if (!entry.routes.some((route) => route.key === routeKey)) {
              const route: ModelSquareRoute = {
                key: routeKey,
                channelName: ch.name,
                platform,
                group,
                billingMode,
                pricing: m.pricing,
                defaultVideoPrice480P: m.default_video_price_480p ?? null,
                defaultVideoPrice720P: m.default_video_price_720p ?? null,
                defaultVideoPrice1080P: m.default_video_price_1080p ?? null
              }
              entry.routes.push(route)
            }
          }
        }
      }
    }
  }

  const list = Array.from(map.values())
  for (const model of list) {
    model.groups.sort((a, b) => Number(b.isExclusive) - Number(a.isExclusive))
    model.endpointDetails = resolveModelEndpoints(model)
    model.endpoints = endpointFilterKeys(model.endpointDetails)
    if (model.billingMode === BILLING_MODE_IMAGE) {
      model.imageTiers = resolveImageTierPrices(model.pricing, model.groups)
    }
  }
  return list.sort((a, b) => a.name.localeCompare(b.name) || a.billingMode.localeCompare(b.billingMode))
})

watch(allModels, (models) => {
  if (!selectedModel.value) return
  selectedModel.value = models.find((model) => model.key === selectedModel.value?.key) ?? null
})

const totalModels = computed(() => allModels.value.length)

/** Count models passing every *other* active filter (so counts reflect AND context). */
function countWith(pred: (m: ModelSquareModel) => boolean, exclude: 'provider' | 'group' | 'endpoint' | 'billing'): number {
  return allModels.value.filter((m) => {
    if (exclude !== 'provider' && activeProvider.value !== 'all' && m.brand !== activeProvider.value) return false
    if (exclude !== 'group' && activeGroup.value !== 'all' && !m.groupIds.includes(Number(activeGroup.value))) return false
    if (exclude !== 'endpoint' && activeEndpoint.value !== 'all' && !m.endpoints.includes(activeEndpoint.value)) return false
    if (exclude !== 'billing' && activeBilling.value !== 'all' && m.billingMode !== activeBilling.value) return false
    return pred(m)
  }).length
}

const providerOptions = computed(() => {
  // 按模型品牌/厂商动态生成（根据模型名归类），仅展示实际有模型的品牌
  const brands = new Map<string, { label: string; keyword: string; colorClass: string }>()
  for (const m of allModels.value) {
    if (!m.brand || brands.has(m.brand)) continue
    const info = resolveGroupBrand('', m.name)
    brands.set(m.brand, {
      label: BRAND_LABEL[m.brand] || info.label,
      keyword: info.keyword,
      colorClass: info.colorClass
    })
  }
  const opts = [
    { value: 'all', label: t('modelSquare.allProviders'), keyword: '', colorClass: '', count: countWith(() => true, 'provider') }
  ]
  for (const [brand, info] of brands) {
    opts.push({
      value: brand,
      label: info.label,
      keyword: info.keyword,
      colorClass: info.colorClass,
      count: countWith((m) => m.brand === brand, 'provider')
    })
  }
  // "全部" 置顶，其余按数量降序
  opts.sort((a, b) => {
    if (a.value === 'all') return -1
    if (b.value === 'all') return 1
    return b.count - a.count
  })
  return opts
})

const groupOptions = computed(() => {
  const seen = new Map<number, { name: string; exclusive: boolean; platform: string }>()
  for (const m of allModels.value) {
    for (const g of m.groups) {
      if (!seen.has(g.id)) seen.set(g.id, { name: g.name, exclusive: g.isExclusive, platform: g.platform })
    }
  }
  const opts: { value: string; label: string; count: number; exclusive: boolean; platform: string }[] = [
    { value: 'all', label: t('modelSquare.allGroups'), count: countWith(() => true, 'group'), exclusive: false, platform: '' }
  ]
  for (const [id, info] of seen) {
    opts.push({
      value: String(id),
      label: info.name,
      exclusive: info.exclusive,
      platform: info.platform,
      count: countWith((m) => m.groupIds.includes(id), 'group')
    })
  }
  // Exclusive groups first, then by count desc.
  opts.sort((a, b) => {
    if (a.value === 'all') return -1
    if (b.value === 'all') return 1
    if (a.exclusive !== b.exclusive) return Number(b.exclusive) - Number(a.exclusive)
    return b.count - a.count
  })
  return opts
})

const endpointOptions = computed(() => {
  const kinds = ['anthropic', 'openai', 'gemini']
  const opts = [{ value: 'all', label: t('modelSquare.filters.all'), count: countWith(() => true, 'endpoint') }]
  for (const k of kinds) {
    const count = countWith((m) => m.endpoints.includes(k), 'endpoint')
    if (count > 0) opts.push({ value: k, label: t(`modelSquare.endpoints.${k}`), count })
  }
  return opts
})

const billingOptions = computed(() => {
  const opts = [{ value: 'all', label: t('modelSquare.filters.all'), count: countWith(() => true, 'billing') }]
  const modes: { v: BillingMode; k: string }[] = [
    { v: BILLING_MODE_TOKEN, k: 'billingModeToken' },
    { v: BILLING_MODE_PER_REQUEST, k: 'billingModePerRequest' },
    { v: BILLING_MODE_IMAGE, k: 'billingModeImage' },
    { v: BILLING_MODE_VIDEO, k: 'billingModeVideo' }
  ]
  for (const { v, k } of modes) {
    const count = countWith((m) => m.billingMode === v, 'billing')
    if (count > 0) opts.push({ value: v, label: t(`availableChannels.pricing.${k}`), count })
  }
  return opts
})

const filteredModels = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return allModels.value.filter((m) => {
    if (activeProvider.value !== 'all' && m.brand !== activeProvider.value) return false
    if (activeGroup.value !== 'all' && !m.groupIds.includes(Number(activeGroup.value))) return false
    if (activeEndpoint.value !== 'all' && !m.endpoints.includes(activeEndpoint.value)) return false
    if (activeBilling.value !== 'all' && m.billingMode !== activeBilling.value) return false
    if (
      q &&
      !m.name.toLowerCase().includes(q) &&
      !m.platforms.some((p) => p.toLowerCase().includes(q)) &&
      !m.groups.some((g) => g.name.toLowerCase().includes(q))
    ) {
      return false
    }
    return true
  })
})

const hasActiveFilter = computed(
  () =>
    activeProvider.value !== 'all' ||
    activeGroup.value !== 'all' ||
    activeEndpoint.value !== 'all' ||
    activeBilling.value !== 'all' ||
    searchQuery.value.trim() !== ''
)

function clearFilters(): void {
  activeProvider.value = 'all'
  activeGroup.value = 'all'
  activeEndpoint.value = 'all'
  activeBilling.value = 'all'
  searchQuery.value = ''
}

function railClass(active: boolean, disabled: boolean): string {
  if (disabled) {
    return 'cursor-not-allowed border-gray-100 bg-gray-50/60 text-gray-300 dark:border-dark-700 dark:bg-dark-800/40 dark:text-gray-600'
  }
  return active
    ? 'border-primary-400 bg-primary-500/10 font-medium text-primary-600 dark:border-primary-600 dark:text-primary-400'
    : 'border-gray-200 bg-white text-gray-600 hover:border-primary-300 hover:bg-primary-50/50 dark:border-dark-700 dark:bg-dark-900/30 dark:text-gray-300 dark:hover:border-primary-700 dark:hover:bg-dark-800'
}

function chipClass(active: boolean, disabled: boolean): string {
  if (disabled) return 'cursor-not-allowed border-gray-100 text-gray-300 dark:border-dark-700 dark:text-gray-600'
  return active
    ? 'border-primary-500 bg-primary-500/10 text-primary-600 dark:text-primary-400'
    : 'border-gray-200 text-gray-600 dark:border-dark-700 dark:text-gray-400'
}

function billingLabel(mode: BillingMode): string {
  switch (mode) {
    case BILLING_MODE_PER_REQUEST:
      return t('availableChannels.pricing.billingModePerRequest')
    case BILLING_MODE_IMAGE:
      return t('availableChannels.pricing.billingModeImage')
    case BILLING_MODE_VIDEO:
      return t('availableChannels.pricing.billingModeVideo')
    default:
      return t('availableChannels.pricing.billingModeToken')
  }
}

function formatMultiplier(rate: number): string {
  return `${Number(rate.toFixed(3)).toString()}x`
}

// Cached brand resolution keyed by "platform|name" to avoid recomputing in the
// template on every render.
const brandCache = new Map<string, ReturnType<typeof resolveGroupBrand>>()
function groupBrand(platform: string, name: string) {
  const key = `${platform}|${name}`
  let cached = brandCache.get(key)
  if (!cached) {
    cached = resolveGroupBrand(platform, name)
    brandCache.set(key, cached)
  }
  return cached
}

function modelBrand(model: ModelSquareModel) {
  return groupBrand('', model.name)
}

function openModelDetails(model: ModelSquareModel): void {
  selectedModel.value = model
}

function closeModelDetails(): void {
  selectedModel.value = null
}

// Discount-aware multiplier badge: <1 reads as a discount (green), =1 neutral,
// >1 as a premium (amber). Mirrors the colorful rate chips in the design.
function multiplierBadgeClass(rate: number): string {
  if (rate < 1) return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
  if (rate > 1) return 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
  return 'bg-gray-200/70 text-gray-600 dark:bg-dark-600 dark:text-gray-300'
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

async function loadData(options: LoadDataOptions = {}) {
  const autoRefresh = options.silent === true
  if (autoRefresh && isFetching) return

  abortController?.abort()
  const currentController = new AbortController()
  const signal = currentController.signal
  abortController = currentController
  isFetching = true

  const showLoading = options.showLoading ?? channels.value.length === 0
  const controlsLoading = showLoading || loading.value
  if (controlsLoading) loading.value = true

  try {
    const list = await userChannelsAPI.getModelSquare({ signal })

    if (signal.aborted || abortController !== currentController) return
    channels.value = list
  } catch (err: unknown) {
    if (signal.aborted || isAbortError(err)) return
    if (options.silent) {
      console.warn('Failed to auto refresh model square data:', err)
      return
    }
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    if (abortController === currentController) {
      abortController = null
      isFetching = false
      if (controlsLoading) loading.value = false
    }
  }
}

function refreshNow() {
  void loadData({ showLoading: true, silent: false })
}

useVisibleAutoRefresh({
  intervalMs: AUTO_REFRESH_INTERVAL_MS,
  onRefresh: () => loadData({ showLoading: false, silent: true }),
  shouldRefresh: () => !isFetching,
})

onMounted(refreshNow)

onBeforeUnmount(() => {
  abortController?.abort()
  if (copyTimer) clearTimeout(copyTimer)
})
</script>

<style scoped>
@media (min-width: 1024px) {
  .model-square-scroll-pane {
    scrollbar-gutter: stable;
  }
}
</style>

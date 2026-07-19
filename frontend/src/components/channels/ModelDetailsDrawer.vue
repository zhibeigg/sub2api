<template>
  <Teleport to="body">
    <Transition name="model-drawer">
      <div
        v-if="show && model"
        class="fixed inset-0 z-50 bg-gray-950/45"
        @mousedown.self="emit('close')"
      >
        <aside
          ref="panelRef"
          class="model-details-panel absolute inset-x-2 bottom-2 top-10 flex flex-col overflow-hidden rounded-2xl border border-gray-200 bg-gray-50 text-gray-900 shadow-2xl dark:border-dark-700 dark:bg-dark-900 dark:text-gray-100 sm:inset-y-0 sm:left-auto sm:right-0 sm:w-full sm:max-w-[38rem] sm:rounded-none sm:border-y-0 sm:border-r-0"
          role="dialog"
          aria-modal="true"
          aria-labelledby="model-details-title"
          tabindex="-1"
          data-testid="model-details-drawer"
          @click.stop
        >
          <header class="flex flex-none items-start gap-3 border-b border-gray-200 bg-white px-5 py-4 dark:border-dark-700 dark:bg-dark-900 sm:px-6">
            <div class="flex h-10 w-10 flex-none items-center justify-center rounded-xl bg-gray-100 dark:bg-dark-800">
              <ModelIcon :model="model.name" size="22px" />
            </div>
            <div class="min-w-0 flex-1">
              <div class="flex items-start gap-2">
                <h2 id="model-details-title" class="min-w-0 flex-1 break-words text-base font-semibold leading-6">
                  {{ model.name }}
                </h2>
                <button
                  type="button"
                  class="inline-flex h-9 w-9 flex-none items-center justify-center rounded-lg text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/50 dark:text-gray-500 dark:hover:bg-dark-800 dark:hover:text-gray-200"
                  :aria-label="t('modelSquare.details.copyModel')"
                  @click="emit('copy', model.name)"
                >
                  <Icon :name="copied ? 'check' : 'copy'" size="sm" :class="copied ? 'text-emerald-500' : ''" />
                </button>
              </div>
              <div class="mt-2 flex flex-wrap items-center gap-2">
                <span
                  class="inline-flex items-center gap-1.5 rounded-md border border-gray-200 bg-gray-50 px-2 py-1 text-xs font-medium dark:border-dark-700 dark:bg-dark-800"
                  :class="brand.colorClass"
                >
                  <ModelIcon :model="brand.keyword" size="14px" />
                  {{ brand.label }}
                </span>
                <span class="rounded-md border border-gray-200 px-2 py-1 text-[11px] font-medium text-gray-500 dark:border-dark-700 dark:text-gray-400">
                  {{ billingLabel(model.billingMode) }}
                </span>
              </div>
            </div>
            <button
              type="button"
              class="inline-flex h-10 w-10 flex-none items-center justify-center rounded-lg text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/50 dark:text-gray-500 dark:hover:bg-dark-800 dark:hover:text-gray-200"
              :aria-label="t('modelSquare.details.close')"
              data-testid="model-details-close"
              @click="emit('close')"
            >
              <Icon name="x" size="md" />
            </button>
          </header>

          <div class="min-h-0 flex-1 overflow-y-auto overscroll-contain px-5 py-5 sm:px-6">
            <section aria-labelledby="model-details-overview" class="space-y-3">
              <h3 id="model-details-overview" class="text-sm font-semibold">
                {{ t('modelSquare.details.overview') }}
              </h3>
              <dl class="grid grid-cols-[7rem_minmax(0,1fr)] gap-x-3 gap-y-2 border-y border-gray-200 py-3 text-sm dark:border-dark-700">
                <dt class="text-gray-500 dark:text-gray-400">{{ t('modelSquare.details.provider') }}</dt>
                <dd class="font-medium" :class="brand.colorClass">{{ brand.label }}</dd>
                <dt class="text-gray-500 dark:text-gray-400">{{ t('modelSquare.details.billing') }}</dt>
                <dd class="font-medium">{{ billingLabel(model.billingMode) }}</dd>
                <dt class="text-gray-500 dark:text-gray-400">{{ t('modelSquare.details.availableGroups') }}</dt>
                <dd class="font-medium tabular-nums">{{ model.groups.length }}</dd>
              </dl>
            </section>

            <section aria-labelledby="model-details-endpoints" class="mt-7 space-y-3">
              <div>
                <h3 id="model-details-endpoints" class="text-sm font-semibold">
                  {{ t('modelSquare.details.apiEndpoints') }}
                </h3>
                <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
                  {{ t('modelSquare.details.apiEndpointsHint') }}
                </p>
              </div>

              <div v-if="model.endpointDetails.length > 0" class="divide-y divide-gray-200 border-y border-gray-200 dark:divide-dark-700 dark:border-dark-700">
                <div
                  v-for="endpoint in model.endpointDetails"
                  :key="endpoint.key"
                  class="grid grid-cols-[minmax(0,1fr)_auto] gap-3 py-3"
                  :data-testid="`model-endpoint-${endpoint.kind}`"
                >
                  <div class="min-w-0">
                    <div class="text-xs font-medium text-gray-700 dark:text-gray-200">
                      {{ t(endpoint.labelKey) }}
                    </div>
                    <code class="mt-1 block break-all text-xs text-primary-600 dark:text-primary-400">
                      {{ endpoint.path }}
                    </code>
                    <p class="mt-1 text-[11px] text-gray-400 dark:text-gray-500">
                      {{ t('modelSquare.details.endpointGroups', { count: endpoint.groupIds.length }) }}
                    </p>
                  </div>
                  <span class="mt-0.5 font-mono text-[10px] font-semibold tracking-wide text-gray-500 dark:text-gray-400">
                    {{ endpoint.method }}
                  </span>
                </div>
              </div>
              <p v-else class="border-y border-gray-200 py-4 text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">
                {{ t('modelSquare.details.noEndpoints') }}
              </p>
            </section>

            <section aria-labelledby="model-details-pricing" class="mt-7 space-y-3">
              <div>
                <h3 id="model-details-pricing" class="text-sm font-semibold">
                  {{ t('modelSquare.details.groupPricing') }}
                </h3>
                <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
                  {{ t('modelSquare.details.groupPricingHint') }}
                </p>
              </div>

              <div v-if="groupRoutes.length > 0" class="divide-y divide-gray-200 border-y border-gray-200 dark:divide-dark-700 dark:border-dark-700">
                <article
                  v-for="route in groupRoutes"
                  :key="route.key"
                  class="py-4"
                  :data-testid="`model-price-group-${route.group.id}`"
                >
                  <div class="flex items-start justify-between gap-3">
                    <div class="flex min-w-0 items-center gap-2">
                      <ModelIcon :model="groupBrand(route).keyword" size="16px" class="flex-none" />
                      <div class="min-w-0">
                        <div class="flex flex-wrap items-center gap-1.5">
                          <span class="truncate text-sm font-semibold" :class="groupBrand(route).colorClass" :title="route.group.name">
                            {{ route.group.name }}
                          </span>
                          <span
                            v-if="route.group.isExclusive"
                            class="rounded bg-purple-500/15 px-1.5 py-0.5 text-[9px] font-semibold uppercase tracking-wide text-purple-700 dark:text-purple-300"
                          >
                            {{ t('availableChannels.exclusive') }}
                          </span>
                        </div>
                      </div>
                    </div>
                    <div class="flex-none text-right">
                      <div class="text-[10px] uppercase tracking-wide text-gray-400 dark:text-gray-500">
                        {{ t('modelSquare.details.table.rate') }}
                      </div>
                      <span class="mt-1 inline-flex rounded px-1.5 py-0.5 text-xs font-semibold tabular-nums" :class="multiplierClass(route.group.rate)">
                        {{ formatMultiplier(route.group.rate) }}
                      </span>
                    </div>
                  </div>

                  <dl class="mt-3 grid grid-cols-1 gap-2 text-xs sm:grid-cols-2">
                    <div class="min-w-0 rounded-lg border border-gray-200 bg-white px-3 py-2 dark:border-dark-700 dark:bg-dark-800/60">
                      <dt class="text-[10px] uppercase tracking-wide text-gray-400 dark:text-gray-500">
                        {{ t('modelSquare.details.table.channel') }}
                      </dt>
                      <dd class="mt-1 truncate font-medium text-gray-700 dark:text-gray-200" :title="route.channelName">
                        {{ route.channelName }}
                      </dd>
                    </div>
                    <div class="rounded-lg border border-gray-200 bg-white px-3 py-2 dark:border-dark-700 dark:bg-dark-800/60">
                      <dt class="text-[10px] uppercase tracking-wide text-gray-400 dark:text-gray-500">
                        {{ t('modelSquare.details.table.billing') }}
                      </dt>
                      <dd class="mt-1 font-medium text-gray-700 dark:text-gray-200">
                        {{ billingLabel(route.billingMode) }}
                      </dd>
                    </div>
                  </dl>

                  <div v-if="priceItems(route).length > 0" class="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-3">
                    <div
                      v-for="item in priceItems(route)"
                      :key="item.key"
                      class="rounded-lg bg-gray-100/80 px-3 py-2.5 dark:bg-dark-800"
                    >
                      <div class="text-[10px] uppercase tracking-wide text-gray-400 dark:text-gray-500">
                        {{ item.label }}
                      </div>
                      <div class="mt-1 text-sm font-semibold tabular-nums text-gray-900 dark:text-gray-100">
                        {{ item.value }}
                      </div>
                      <div v-if="item.unit" class="mt-0.5 text-[10px] text-gray-400 dark:text-gray-500">
                        {{ item.unit }}
                      </div>
                    </div>
                  </div>
                  <p v-else class="mt-3 text-xs text-gray-400 dark:text-gray-500">
                    {{ t('modelSquare.details.noConfiguredPrice') }}
                  </p>
                </article>
              </div>
              <p v-else class="border-y border-gray-200 py-4 text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">
                {{ t('modelSquare.details.noGroupPricing') }}
              </p>
              <p class="text-[11px] leading-5 text-gray-400 dark:text-gray-500">
                {{ t('modelSquare.details.priceUnit') }}
              </p>
            </section>
          </div>
        </aside>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useScrollLock } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_TOKEN,
  BILLING_MODE_VIDEO,
  type BillingMode
} from '@/constants/channel'
import { resolveGroupBrand } from '@/utils/groupBrand'
import { formatScaled } from '@/utils/pricing'
import {
  effectiveImageTierPrice,
  effectiveRoutePriceRange,
  effectiveVideoTierPrice,
  type EffectivePriceRange,
  type ModelSquareModel,
  type ModelSquareRoute,
  type RoutePricingField
} from '@/views/user/modelSquareDetails'

const props = defineProps<{
  show: boolean
  model: ModelSquareModel | null
  copied?: boolean
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'copy', name: string): void
}>()

interface PriceItem {
  key: string
  label: string
  value: string
  unit?: string
}

const { t } = useI18n()
const panelRef = ref<HTMLElement | null>(null)
const bodyScrollLocked = useScrollLock(typeof document === 'undefined' ? null : document.body)
const PER_M = 1_000_000
let previousActiveElement: HTMLElement | null = null
let backgroundRoot: (HTMLElement & { inert: boolean }) | null = null
let previousBackgroundAriaHidden: string | null = null

const brand = computed(() => resolveGroupBrand('', props.model?.name ?? ''))

const groupRoutes = computed<ModelSquareRoute[]>(() => {
  if (!props.model) return []
  const byGroup = new Map<number, ModelSquareRoute>()
  for (const route of props.model.routes) {
    const existing = byGroup.get(route.group.id)
    if (!existing || (!existing.pricing && route.pricing)) byGroup.set(route.group.id, route)
  }
  return Array.from(byGroup.values()).sort((a, b) => {
    if (a.group.isExclusive !== b.group.isExclusive) return Number(b.group.isExclusive) - Number(a.group.isExclusive)
    return a.group.name.localeCompare(b.group.name)
  })
})

function billingLabel(mode: BillingMode): string {
  if (mode === BILLING_MODE_PER_REQUEST) return t('availableChannels.pricing.billingModePerRequest')
  if (mode === BILLING_MODE_IMAGE) return t('availableChannels.pricing.billingModeImage')
  if (mode === BILLING_MODE_VIDEO) return t('availableChannels.pricing.billingModeVideo')
  return t('availableChannels.pricing.billingModeToken')
}

function groupBrand(route: ModelSquareRoute) {
  return resolveGroupBrand(route.platform, route.group.name)
}

function formatPriceRange(range: EffectivePriceRange, scale: number): string {
  const min = formatScaled(range.min, scale)
  if (Math.abs(range.max - range.min) < 1e-12) return min
  return `${min}–${formatScaled(range.max, scale)}`
}

function rangedPriceItem(
  route: ModelSquareRoute,
  field: RoutePricingField,
  key: string,
  label: string,
  unit: string,
  scale: number
): PriceItem | null {
  const range = effectiveRoutePriceRange(route, field)
  if (!range) return null
  return { key, label, value: formatPriceRange(range, scale), unit }
}

function priceItems(route: ModelSquareRoute): PriceItem[] {
  if (route.billingMode === BILLING_MODE_IMAGE) {
    return (['1K', '2K', '4K'] as const).flatMap((tier) => {
      const price = effectiveImageTierPrice(route, tier)
      return price == null
        ? []
        : [{
            key: `image-${tier}`,
            label: tier,
            value: formatScaled(price, 1),
            unit: t('availableChannels.pricing.unitPerImage')
          }]
    })
  }

  if (route.billingMode === BILLING_MODE_VIDEO) {
    const defaultVideoPrices = [
      route.defaultVideoPrice480P,
      route.defaultVideoPrice720P,
      route.defaultVideoPrice1080P
    ].filter((price): price is number => price != null)
    const hasTieredVideoPrice = route.group.videoBillingEnabled || new Set(defaultVideoPrices).size > 1
    if (!hasTieredVideoPrice) {
      const price = effectiveVideoTierPrice(route, '480P')
      return price == null
        ? []
        : [{
            key: 'video-per-second',
            label: t('availableChannels.pricing.perSecondPrice'),
            value: formatScaled(price, 1),
            unit: t('modelSquare.details.units.perSecond')
          }]
    }
    return (['480P', '720P', '1080P'] as const).flatMap((tier) => {
      const price = effectiveVideoTierPrice(route, tier)
      return price == null
        ? []
        : [{
            key: `video-${tier}`,
            label: tier,
            value: formatScaled(price, 1),
            unit: t('modelSquare.details.units.perSecond')
          }]
    })
  }

  if (route.billingMode === BILLING_MODE_PER_REQUEST) {
    const item = rangedPriceItem(
      route,
      'per_request_price',
      'per-request',
      t('availableChannels.pricing.perRequestPrice'),
      t('availableChannels.pricing.unitPerRequest'),
      1
    )
    return item ? [item] : []
  }

  if (route.billingMode !== BILLING_MODE_TOKEN) return []
  return [
    rangedPriceItem(route, 'input_price', 'input', t('modelSquare.details.table.input'), t('availableChannels.pricing.unitPerMillion'), PER_M),
    rangedPriceItem(route, 'output_price', 'output', t('modelSquare.details.table.output'), t('availableChannels.pricing.unitPerMillion'), PER_M),
    rangedPriceItem(route, 'cache_write_price', 'cache-write', t('modelSquare.details.table.cacheWrite'), t('availableChannels.pricing.unitPerMillion'), PER_M),
    rangedPriceItem(route, 'cache_read_price', 'cache-read', t('modelSquare.details.table.cacheRead'), t('availableChannels.pricing.unitPerMillion'), PER_M)
  ].filter((item): item is PriceItem => item != null)
}

function formatMultiplier(rate: number): string {
  return `${Number(rate.toFixed(3)).toString()}x`
}

function multiplierClass(rate: number): string {
  if (rate < 1) return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
  if (rate > 1) return 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
  return 'bg-gray-200/70 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
}

function focusableElements(): HTMLElement[] {
  if (!panelRef.value) return []
  return Array.from(
    panelRef.value.querySelectorAll<HTMLElement>(
      'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
    )
  )
}

function setBackgroundInert(inert: boolean): void {
  if (typeof document === 'undefined') return
  if (inert) {
    const root = document.getElementById('app') as (HTMLElement & { inert: boolean }) | null
    if (!root) return
    backgroundRoot = root
    previousBackgroundAriaHidden = root.getAttribute('aria-hidden')
    root.inert = true
    root.setAttribute('aria-hidden', 'true')
    return
  }
  if (!backgroundRoot) return
  backgroundRoot.inert = false
  if (previousBackgroundAriaHidden == null) backgroundRoot.removeAttribute('aria-hidden')
  else backgroundRoot.setAttribute('aria-hidden', previousBackgroundAriaHidden)
  backgroundRoot = null
  previousBackgroundAriaHidden = null
}

function handleKeydown(event: KeyboardEvent): void {
  if (!props.show) return
  if (event.key === 'Escape') {
    event.preventDefault()
    emit('close')
    return
  }
  if (event.key !== 'Tab') return

  const panel = panelRef.value
  const focusable = focusableElements()
  if (!panel || focusable.length === 0) {
    event.preventDefault()
    panel?.focus()
    return
  }
  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  const active = document.activeElement
  if (!active || !panel.contains(active) || active === panel) {
    event.preventDefault()
    ;(event.shiftKey ? last : first).focus()
  } else if (event.shiftKey && active === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && active === last) {
    event.preventDefault()
    first.focus()
  }
}

watch(
  () => props.show,
  async (show) => {
    bodyScrollLocked.value = show
    if (show) {
      previousActiveElement = document.activeElement as HTMLElement | null
      await nextTick()
      if (!props.show) return
      setBackgroundInert(true)
      focusableElements()[0]?.focus()
      if (!panelRef.value?.contains(document.activeElement)) panelRef.value?.focus()
      return
    }
    setBackgroundInert(false)
    await nextTick()
    if (previousActiveElement?.isConnected) previousActiveElement.focus()
    previousActiveElement = null
  },
  { immediate: true }
)

onMounted(() => document.addEventListener('keydown', handleKeydown))
onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleKeydown)
  bodyScrollLocked.value = false
  setBackgroundInert(false)
})
</script>

<style scoped>
.model-drawer-enter-active,
.model-drawer-leave-active {
  transition: opacity 180ms cubic-bezier(0.25, 1, 0.5, 1);
}

.model-drawer-enter-active .model-details-panel,
.model-drawer-leave-active .model-details-panel {
  transition: transform 300ms cubic-bezier(0.16, 1, 0.3, 1);
}

.model-drawer-enter-from,
.model-drawer-leave-to {
  opacity: 0;
}

.model-drawer-enter-from .model-details-panel,
.model-drawer-leave-to .model-details-panel {
  transform: translateX(100%);
}

.model-details-panel {
  padding-bottom: env(safe-area-inset-bottom, 0px);
}

@media (prefers-reduced-motion: reduce) {
  .model-drawer-enter-active,
  .model-drawer-leave-active,
  .model-drawer-enter-active .model-details-panel,
  .model-drawer-leave-active .model-details-panel {
    transition-duration: 0.01ms;
  }
}
</style>

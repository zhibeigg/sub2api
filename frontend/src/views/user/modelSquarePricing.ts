import type { UserSupportedModelPricing } from '@/api/channels'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_TOKEN,
  BILLING_MODE_VIDEO,
  type BillingMode
} from '@/constants/channel'

export const IMAGE_PRICE_TIERS = ['1K', '2K', '4K'] as const

export type ImagePriceTier = (typeof IMAGE_PRICE_TIERS)[number]

export interface ModelSquareImageGroup {
  rate: number
  imageBillingEnabled: boolean
  imageRateIndependent: boolean
  imageRateMultiplier: number
  imagePrice1K: number | null
  imagePrice2K: number | null
  imagePrice4K: number | null
  videoBillingEnabled: boolean
}

export interface ModelSquareImageTierPrice {
  tier: ImagePriceTier
  minPrice: number
  maxPrice: number
}

export function effectiveModelBillingMode(
  mediaType: string | undefined,
  pricing: UserSupportedModelPricing | null,
  group: Pick<ModelSquareImageGroup, 'imageBillingEnabled'> & Partial<Pick<ModelSquareImageGroup, 'videoBillingEnabled'>>
): BillingMode {
  const channelMode = pricing?.billing_mode ?? BILLING_MODE_TOKEN
  if (mediaType === 'video') {
    if (group.videoBillingEnabled) return BILLING_MODE_VIDEO
    if (channelMode === BILLING_MODE_IMAGE || channelMode === BILLING_MODE_PER_REQUEST) {
      return BILLING_MODE_PER_REQUEST
    }
    return BILLING_MODE_VIDEO
  }
  if (mediaType !== 'image') return channelMode

  if (group.imageBillingEnabled) return BILLING_MODE_IMAGE
  if (!pricing || channelMode === BILLING_MODE_IMAGE || channelMode === BILLING_MODE_PER_REQUEST) {
    return BILLING_MODE_IMAGE
  }
  return channelMode
}

export function resolveImageTierPrices(
  pricing: UserSupportedModelPricing | null,
  groups: ModelSquareImageGroup[]
): ModelSquareImageTierPrice[] {
  const prices = IMAGE_PRICE_TIERS.flatMap((tier) => {
    const channelPrice = channelTierPrice(pricing, tier)
    const resolved = groups.length > 0
      ? groups.map((group) => {
          const basePrice = group.imageBillingEnabled ? groupTierPrice(group, tier) : channelPrice
          return basePrice == null ? null : basePrice * group.rate
        })
      : [channelPrice]

    // 分组只覆盖部分档位时，后端会使用模型默认价而不是渠道价。默认价不在当前
    // 用户接口中暴露，因此这里宁可省略无法完整解析的档位，也不展示错误价格。
    if (resolved.length === 0 || resolved.some((price) => price == null)) return []
    const completePrices = resolved as number[]
    return [{
      tier,
      minPrice: Math.min(...completePrices),
      maxPrice: Math.max(...completePrices)
    }]
  })

  return prices
}

export function formatImagePrice(value: number): string {
  if (!Number.isFinite(value)) return '-'
  const digits = Math.abs(value) < 0.001 && value !== 0 ? 6 : 3
  return `$${value.toFixed(digits)}`
}

export function formatImagePriceRange(price: ModelSquareImageTierPrice): string {
  const min = formatImagePrice(price.minPrice)
  if (Math.abs(price.maxPrice - price.minPrice) < 1e-12) return min
  return `${min}–${formatImagePrice(price.maxPrice)}`
}

function channelTierPrice(
  pricing: UserSupportedModelPricing | null,
  tier: ImagePriceTier
): number | null {
  if (!pricing) return null
  if (pricing.billing_mode !== BILLING_MODE_IMAGE && pricing.billing_mode !== BILLING_MODE_PER_REQUEST) {
    return null
  }

  const interval = pricing.intervals?.find(
    (item) => item.tier_label?.trim().toUpperCase() === tier
  )
  if (interval?.per_request_price != null) return interval.per_request_price
  return pricing.per_request_price ?? pricing.image_output_price ?? null
}

function groupTierPrice(group: ModelSquareImageGroup, tier: ImagePriceTier): number | null {
  switch (tier) {
    case '1K':
      return group.imagePrice1K
    case '2K':
      return group.imagePrice2K
    case '4K':
      return group.imagePrice4K
  }
}

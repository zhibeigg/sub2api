import type { UserSupportedModelPricing } from '@/api/channels'
import type { BillingMode } from '@/constants/channel'
import type { ModelSquareImageTierPrice } from './modelSquarePricing'

export interface ModelSquareGroup {
  id: number
  name: string
  platform: string
  rate: number
  isExclusive: boolean
  allowImageGeneration: boolean
  allowVideoGeneration: boolean
  allowMessagesDispatch: boolean

  imageBillingEnabled: boolean
  imageRateIndependent: boolean
  imageRateMultiplier: number
  imagePrice1K: number | null
  imagePrice2K: number | null
  imagePrice4K: number | null
  videoBillingEnabled: boolean
  videoRateIndependent: boolean
  videoRateMultiplier: number
  videoPrice480P: number | null
  videoPrice720P: number | null
  videoPrice1080P: number | null
}

export interface ModelSquareRoute {
  key: string
  channelName: string
  platform: string
  group: ModelSquareGroup
  billingMode: BillingMode
  pricing: UserSupportedModelPricing | null
  defaultVideoPrice480P: number | null
  defaultVideoPrice720P: number | null
  defaultVideoPrice1080P: number | null
}

export type ModelSquareEndpointKind =
  | 'chat_completions'
  | 'messages'
  | 'responses'
  | 'gemini'
  | 'images'
  | 'image_edits'
  | 'videos'
  | 'video_edits'
  | 'video_extensions'
  | 'video_status'
  | 'video_content'

export interface ModelSquareEndpoint {
  key: string
  kind: ModelSquareEndpointKind
  path: string
  method: 'GET' | 'POST'
  labelKey: string
  groupIds: number[]
}

export interface ModelSquareModel {
  key: string
  name: string
  platforms: string[]
  brand: string
  mediaType: string
  billingMode: BillingMode
  pricing: UserSupportedModelPricing | null
  imageTiers: ModelSquareImageTierPrice[]
  groups: ModelSquareGroup[]
  groupIds: number[]
  routes: ModelSquareRoute[]
  endpoints: string[]
  endpointDetails: ModelSquareEndpoint[]
}

interface EndpointDefinition {
  kind: ModelSquareEndpointKind
  path: string
  method: 'GET' | 'POST'
  labelKey: string
}

const CHAT_COMPLETIONS: EndpointDefinition = {
  kind: 'chat_completions',
  path: '/v1/chat/completions',
  method: 'POST',
  labelKey: 'modelSquare.details.endpoints.chatCompletions'
}

const MESSAGES: EndpointDefinition = {
  kind: 'messages',
  path: '/v1/messages',
  method: 'POST',
  labelKey: 'modelSquare.details.endpoints.messages'
}

const RESPONSES: EndpointDefinition = {
  kind: 'responses',
  path: '/v1/responses',
  method: 'POST',
  labelKey: 'modelSquare.details.endpoints.responses'
}

const GEMINI: EndpointDefinition = {
  kind: 'gemini',
  path: '/v1beta/models/{model}:generateContent',
  method: 'POST',
  labelKey: 'modelSquare.details.endpoints.gemini'
}

const ANTIGRAVITY_GEMINI: EndpointDefinition = {
  kind: 'gemini',
  path: '/antigravity/v1beta/models/{model}:generateContent',
  method: 'POST',
  labelKey: 'modelSquare.details.endpoints.gemini'
}

const IMAGES: EndpointDefinition = {
  kind: 'images',
  path: '/v1/images/generations',
  method: 'POST',
  labelKey: 'modelSquare.details.endpoints.images'
}

const IMAGE_EDITS: EndpointDefinition = {
  kind: 'image_edits',
  path: '/v1/images/edits',
  method: 'POST',
  labelKey: 'modelSquare.details.endpoints.imageEdits'
}

const VIDEOS: EndpointDefinition = {
  kind: 'videos',
  path: '/v1/videos/generations',
  method: 'POST',
  labelKey: 'modelSquare.details.endpoints.videos'
}

const VIDEO_EDITS: EndpointDefinition = {
  kind: 'video_edits',
  path: '/v1/videos/edits',
  method: 'POST',
  labelKey: 'modelSquare.details.endpoints.videoEdits'
}

const VIDEO_EXTENSIONS: EndpointDefinition = {
  kind: 'video_extensions',
  path: '/v1/videos/extensions',
  method: 'POST',
  labelKey: 'modelSquare.details.endpoints.videoExtensions'
}

const VIDEO_STATUS: EndpointDefinition = {
  kind: 'video_status',
  path: '/v1/videos/{request_id}',
  method: 'GET',
  labelKey: 'modelSquare.details.endpoints.videoStatus'
}

const VIDEO_CONTENT: EndpointDefinition = {
  kind: 'video_content',
  path: '/v1/videos/{request_id}/content',
  method: 'GET',
  labelKey: 'modelSquare.details.endpoints.videoContent'
}

function routeSupportsMessages(route: ModelSquareRoute): boolean {
  if (route.platform === 'adobe') return false
  if (route.platform === 'openai') return route.group.allowMessagesDispatch
  return true
}

export function resolveRouteEndpoints(route: ModelSquareRoute, mediaType: string): EndpointDefinition[] {
  if (mediaType === 'image') {
    if (route.platform === 'gemini') return [GEMINI]
    if (route.platform === 'antigravity') return [ANTIGRAVITY_GEMINI]
    if (route.platform === 'openai' || route.platform === 'grok' || route.platform === 'adobe') {
      return [IMAGES, IMAGE_EDITS]
    }
    return []
  }
  if (mediaType === 'video') {
    if (route.platform === 'grok') {
      return [VIDEOS, VIDEO_EDITS, VIDEO_EXTENSIONS, VIDEO_STATUS, VIDEO_CONTENT]
    }
    if (route.platform === 'openai' || route.platform === 'adobe') {
      return [VIDEOS, VIDEO_STATUS]
    }
    return []
  }
  if (route.platform === 'adobe') return []

  const endpoints: EndpointDefinition[] = [CHAT_COMPLETIONS, RESPONSES]
  if (routeSupportsMessages(route)) endpoints.splice(1, 0, MESSAGES)
  if (route.platform === 'gemini') endpoints.push(GEMINI)
  if (route.platform === 'antigravity') endpoints.push(ANTIGRAVITY_GEMINI)
  return endpoints
}

export function resolveModelEndpoints(model: Pick<ModelSquareModel, 'routes' | 'mediaType'>): ModelSquareEndpoint[] {
  const endpointMap = new Map<string, ModelSquareEndpoint>()

  for (const route of model.routes) {
    for (const definition of resolveRouteEndpoints(route, model.mediaType)) {
      const key = `${definition.kind}:${definition.method}:${definition.path}`
      const existing = endpointMap.get(key)
      if (existing) {
        if (!existing.groupIds.includes(route.group.id)) existing.groupIds.push(route.group.id)
        continue
      }
      endpointMap.set(key, {
        key,
        ...definition,
        groupIds: [route.group.id]
      })
    }
  }

  return Array.from(endpointMap.values())
}

export function endpointFilterKeys(endpoints: ModelSquareEndpoint[]): string[] {
  const keys = new Set<string>()
  for (const endpoint of endpoints) {
    switch (endpoint.kind) {
      case 'messages':
        keys.add('anthropic')
        break
      case 'gemini':
        keys.add('gemini')
        break
      default:
        keys.add('openai')
        break
    }
  }
  return Array.from(keys)
}

export interface EffectivePriceRange {
  min: number
  max: number
}

export type RoutePricingField =
  | 'input_price'
  | 'output_price'
  | 'cache_write_price'
  | 'cache_read_price'
  | 'per_request_price'

export function effectiveRoutePrice(value: number | null | undefined, rate: number): number | null {
  if (value == null || !Number.isFinite(value)) return null
  return value * rate
}

export function effectiveRoutePriceRange(
  route: ModelSquareRoute,
  field: RoutePricingField
): EffectivePriceRange | null {
  const values = [
    route.pricing?.[field],
    ...(route.pricing?.intervals ?? []).map((interval) => interval[field])
  ]
    .map((value) => effectiveRoutePrice(value, route.group.rate))
    .filter((value): value is number => value != null)
  if (values.length === 0) return null
  return { min: Math.min(...values), max: Math.max(...values) }
}

export function effectiveImageTierPrice(route: ModelSquareRoute, tier: '1K' | '2K' | '4K'): number | null {
  let base: number | null | undefined
  if (route.group.imageBillingEnabled) {
    base = tier === '1K'
      ? route.group.imagePrice1K
      : tier === '2K'
        ? route.group.imagePrice2K
        : route.group.imagePrice4K
  } else {
    const interval = route.pricing?.intervals?.find(
      (item) => item.tier_label?.trim().toUpperCase() === tier
    )
    base = interval?.per_request_price ?? route.pricing?.per_request_price ?? route.pricing?.image_output_price
  }
  return effectiveRoutePrice(base, route.group.rate)
}

export function effectiveVideoTierPrice(route: ModelSquareRoute, tier: '480P' | '720P' | '1080P'): number | null {
  let base = route.group.videoBillingEnabled
    ? tier === '480P'
      ? route.group.videoPrice480P
      : tier === '720P'
        ? route.group.videoPrice720P
        : route.group.videoPrice1080P
    : null

  if (base == null) {
    base = tier === '480P'
      ? route.defaultVideoPrice480P
      : tier === '720P'
        ? route.defaultVideoPrice720P
        : route.defaultVideoPrice1080P
  }
  return effectiveRoutePrice(base, route.group.rate)
}

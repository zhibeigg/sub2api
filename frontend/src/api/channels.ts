/**
 * User Channels API endpoints (non-admin)
 * 用户侧「可用渠道」聚合查询：渠道 + 用户可访问的分组 + 支持模型（含定价）。
 */

import { apiClient } from './client'
import type { BillingMode } from '@/constants/channel'

export interface UserAvailableGroup {
  id: number
  name: string
  platform: string
  /** 'standard' | 'subscription' — 订阅分组视觉加深，和 API 密钥页保持一致。 */
  subscription_type: string
  /** 分组基础倍率；详情价格优先使用 supported_models[].group_rates 的后端快照。 */
  rate_multiplier: number
  peak_rate_enabled: boolean
  peak_start: string
  peak_end: string
  peak_rate_multiplier: number
  /** true = 专属分组（小范围授权）；false = 公开分组。 */
  is_exclusive: boolean
  /** 是否允许调用图片生成能力。 */
  allow_image_generation: boolean
  /** 是否允许调用视频生成能力。 */
  allow_video_generation: boolean
  allow_messages_dispatch: boolean

  /** 是否因分组图片价格覆盖而采用按图片计费。 */
  image_billing_enabled: boolean
  /** 图片生成是否使用独立倍率。 */
  image_rate_independent: boolean
  image_rate_multiplier: number
  image_price_1k: number | null
  image_price_2k: number | null
  image_price_4k: number | null
  /** 是否存在分组级视频分辨率价格。 */
  video_billing_enabled?: boolean
  /** 视频生成是否使用独立倍率。 */
  video_rate_independent?: boolean
  video_rate_multiplier?: number
  video_price_480p?: number | null
  video_price_720p?: number | null
  video_price_1080p?: number | null
}

export interface UserPricingInterval {
  min_tokens: number
  max_tokens: number | null
  tier_label?: string
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  per_request_price: number | null
}

export interface UserSupportedModelPricing {
  billing_mode: BillingMode
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  image_input_price: number | null
  image_output_price: number | null
  per_request_price: number | null
  intervals: UserPricingInterval[]
}

export interface UserSupportedModelGroupRate {
  group_id: number
  /** 已应用用户专属、模型级和当前高峰规则的 Token 倍率。 */
  token_rate_multiplier: number
  /** 已应用用户专属/模型级规则与图片独立倍率配置的图片倍率。 */
  image_rate_multiplier: number
  /** 已应用用户专属/模型级规则与视频独立倍率配置的视频倍率。 */
  video_rate_multiplier: number
}

export interface UserSupportedModel {
  name: string
  platform: string
  /** 后端统一识别的媒体类型；空字符串表示普通文本模型。 */
  media_type?: '' | 'image' | 'video'
  pricing: UserSupportedModelPricing | null
  /** 与真实结算回退一致的默认视频每秒价；Adobe 无默认价时为 null。 */
  default_video_price_480p: number | null
  default_video_price_720p: number | null
  default_video_price_1080p: number | null
  /** 后端按真实计费优先级计算出的分组倍率快照。 */
  group_rates?: UserSupportedModelGroupRate[]
}

/**
 * 渠道下单个平台的子视图：用户可访问的分组 + 该平台支持的模型。
 * 后端把一个渠道按平台聚合成 sections，前端可以把渠道名作为 row-group
 * 一次渲染，后面按 sections 顺序用 rowspan 铺开。
 */
export interface UserChannelPlatformSection {
  platform: string
  groups: UserAvailableGroup[]
  supported_models: UserSupportedModel[]
}

export interface UserAvailableChannel {
  name: string
  description: string
  platforms: UserChannelPlatformSection[]
}

/** 列出当前用户可见的「可用渠道」（与 /groups/available 保持一致，返回平数组）。 */
export async function getAvailable(options?: { signal?: AbortSignal }): Promise<UserAvailableChannel[]> {
  const { data } = await apiClient.get<UserAvailableChannel[]>('/channels/available', {
    signal: options?.signal
  })
  return data
}

export const userChannelsAPI = { getAvailable }

export default userChannelsAPI

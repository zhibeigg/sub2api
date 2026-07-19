import { describe, expect, it } from 'vitest'
import { BILLING_MODE_IMAGE, BILLING_MODE_TOKEN, BILLING_MODE_VIDEO } from '@/constants/channel'
import {
  effectiveImageTierPrice,
  effectiveRoutePrice,
  effectiveRoutePriceRange,
  effectiveVideoTierPrice,
  endpointFilterKeys,
  resolveModelEndpoints,
  resolveRouteEndpoints,
  type ModelSquareRoute
} from '../modelSquareDetails'

function route(
  platform: string,
  allowMessagesDispatch = false,
  billingMode: ModelSquareRoute['billingMode'] = BILLING_MODE_TOKEN,
  pricing: ModelSquareRoute['pricing'] = null
): ModelSquareRoute {
  return {
    key: `${platform}-1`,
    channelName: 'channel',
    platform,
    billingMode,
    pricing,
    defaultVideoPrice480P: null,
    defaultVideoPrice720P: null,
    defaultVideoPrice1080P: null,
    group: {
      id: 1,
      name: 'group',
      rate: 0.5,
      isExclusive: false,
      platform,
      allowMessagesDispatch,
      allowImageGeneration: false,
      allowVideoGeneration: false,
      imageBillingEnabled: false,
      imageRateIndependent: false,
      imageRateMultiplier: 1,
      imagePrice1K: null,
      imagePrice2K: null,
      imagePrice4K: null,
      videoBillingEnabled: false,
      videoRateIndependent: false,
      videoRateMultiplier: 1,
      videoPrice480P: null,
      videoPrice720P: null,
      videoPrice1080P: null
    }
  }
}

describe('model square endpoint details', () => {
  it('shows Messages for OpenCode routes and filters them as Anthropic compatible', () => {
    const endpoints = resolveModelEndpoints({ routes: [route('opencode')], mediaType: '' })

    expect(endpoints.map((endpoint) => endpoint.kind)).toEqual([
      'chat_completions',
      'messages',
      'responses'
    ])
    expect(endpointFilterKeys(endpoints)).toEqual(['openai', 'anthropic'])
  })

  it('only shows Messages for OpenAI groups that explicitly allow dispatch', () => {
    expect(resolveRouteEndpoints(route('openai', false), '').map((endpoint) => endpoint.kind)).toEqual([
      'chat_completions',
      'responses'
    ])
    expect(resolveRouteEndpoints(route('openai', true), '').map((endpoint) => endpoint.kind)).toEqual([
      'chat_completions',
      'messages',
      'responses'
    ])
  })

  it('uses the real Gemini and Antigravity native paths and hides Adobe text endpoints', () => {
    expect(resolveRouteEndpoints(route('gemini'), '').map((endpoint) => endpoint.path)).toContain(
      '/v1beta/models/{model}:generateContent'
    )
    expect(resolveRouteEndpoints(route('antigravity'), '').map((endpoint) => endpoint.path)).toContain(
      '/antigravity/v1beta/models/{model}:generateContent'
    )
    expect(resolveRouteEndpoints(route('adobe'), '')).toEqual([])
  })

  it.each([
    ['openai', ['/v1/images/generations', '/v1/images/edits']],
    ['grok', ['/v1/images/generations', '/v1/images/edits']],
    ['adobe', ['/v1/images/generations', '/v1/images/edits']],
    ['gemini', ['/v1beta/models/{model}:generateContent']],
    ['antigravity', ['/antigravity/v1beta/models/{model}:generateContent']]
  ])('maps %s image models to the real public endpoints', (platform, paths) => {
    expect(resolveRouteEndpoints(route(platform), 'image').map((endpoint) => endpoint.path)).toEqual(paths)
  })

  it('shows platform-specific video submission and status routes', () => {
    for (const platform of ['openai', 'adobe']) {
      expect(resolveRouteEndpoints(route(platform), 'video').map((endpoint) => [endpoint.method, endpoint.path])).toEqual([
        ['POST', '/v1/videos/generations'],
        ['GET', '/v1/videos/{request_id}']
      ])
    }
    expect(resolveRouteEndpoints(route('grok'), 'video').map((endpoint) => [endpoint.method, endpoint.path])).toEqual([
      ['POST', '/v1/videos/generations'],
      ['POST', '/v1/videos/edits'],
      ['POST', '/v1/videos/extensions'],
      ['GET', '/v1/videos/{request_id}'],
      ['GET', '/v1/videos/{request_id}/content']
    ])
    expect(resolveRouteEndpoints(route('gemini'), 'video')).toEqual([])
  })

  it('applies effective multipliers to flat and tiered token prices', () => {
    expect(effectiveRoutePrice(0.000002, 0.5)).toBe(0.000001)
    expect(effectiveRoutePrice(null, 0.5)).toBeNull()

    const tokenRoute = route('opencode', false, BILLING_MODE_TOKEN, {
      billing_mode: BILLING_MODE_TOKEN,
      input_price: 0.000002,
      output_price: 0.00001,
      cache_write_price: null,
      cache_read_price: null,
      image_input_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: [{
        min_tokens: 1000,
        max_tokens: null,
        input_price: 0.000004,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        per_request_price: null
      }]
    })
    expect(effectiveRoutePriceRange(tokenRoute, 'input_price')).toEqual({ min: 0.000001, max: 0.000002 })
  })

  it('resolves image and video tier prices after their media multipliers', () => {
    const imageRoute = route('openai', false, BILLING_MODE_IMAGE)
    imageRoute.group.imageBillingEnabled = true
    imageRoute.group.imagePrice1K = 0.2
    expect(effectiveImageTierPrice(imageRoute, '1K')).toBe(0.1)
    expect(effectiveImageTierPrice(imageRoute, '2K')).toBeNull()

    const videoRoute = route('grok', false, BILLING_MODE_VIDEO)
    videoRoute.group.videoBillingEnabled = true
    videoRoute.group.videoPrice720P = 0.08
    videoRoute.group.rate = 0.25
    expect(effectiveVideoTierPrice(videoRoute, '720P')).toBe(0.02)
    videoRoute.defaultVideoPrice1080P = 0.25
    expect(effectiveVideoTierPrice(videoRoute, '1080P')).toBe(0.0625)
  })
})

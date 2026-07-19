import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ModelSquareView from '../ModelSquareView.vue'
import {
  effectiveImageGroupRate,
  effectiveModelBillingMode,
  formatImagePriceRange,
  resolveImageTierPrices
} from '../modelSquarePricing'
import { BILLING_MODE_IMAGE, BILLING_MODE_TOKEN } from '@/constants/channel'
import type { UserSupportedModelPricing } from '@/api/channels'

const { getAvailableChannels, getUserGroupRates, showError } = vi.hoisted(() => ({
  getAvailableChannels: vi.fn(),
  getUserGroupRates: vi.fn(),
  showError: vi.fn()
}))

const messages: Record<string, string> = {
  'modelSquare.count': 'Models',
  'modelSquare.filtered': 'Filtered',
  'modelSquare.clearFilters': 'Clear',
  'modelSquare.searchPlaceholder': 'Search',
  'modelSquare.empty': 'Empty',
  'modelSquare.noMatch': 'No match',
  'modelSquare.noPricing': 'No pricing',
  'modelSquare.clickToCopy': 'Copy',
  'modelSquare.availableGroups': 'Available groups',
  'modelSquare.allProviders': 'All providers',
  'modelSquare.allGroups': 'All groups',
  'modelSquare.filterRegion': 'Model filters',
  'modelSquare.resultsRegion': 'Model results',
  'modelSquare.filters.all': 'All',
  'modelSquare.filters.provider': 'Provider',
  'modelSquare.filters.group': 'Group',
  'modelSquare.filters.endpoint': 'Endpoint',
  'modelSquare.filters.billing': 'Billing',
  'modelSquare.endpoints.openai': 'OpenAI compatible',
  'availableChannels.pricing.billingModeToken': 'Per Token',
  'availableChannels.pricing.billingModePerRequest': 'Per Request',
  'availableChannels.pricing.billingModeImage': 'Per Image',
  'availableChannels.pricing.inputPrice': 'Input',
  'availableChannels.pricing.outputPrice': 'Output',
  'availableChannels.pricing.cacheWritePrice': 'Cache write',
  'availableChannels.pricing.cacheReadPrice': 'Cache read',
  'availableChannels.pricing.imageOutputPrice': 'Image output',
  'availableChannels.pricing.perRequestPrice': 'Per request',
  'availableChannels.pricing.perSecondPrice': 'Per second',
  'availableChannels.pricing.unitPerMillion': '/ 1M tokens',
  'availableChannels.pricing.unitPerRequest': '/ request',
  'availableChannels.pricing.unitPerImage': '/ image',
  'availableChannels.exclusive': 'Exclusive',
  'common.refresh': 'Refresh',
  'common.error': 'Error'
}

vi.mock('@/api/channels', async () => {
  const actual = await vi.importActual<typeof import('@/api/channels')>('@/api/channels')
  return {
    ...actual,
    default: { getAvailable: getAvailableChannels },
    getAvailable: getAvailableChannels
  }
})

vi.mock('@/api/groups', () => ({
  default: { getUserGroupRates },
  getUserGroupRates
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError })
}))

vi.mock('@/composables/useVisibleAutoRefresh', () => ({
  useVisibleAutoRefresh: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key
    })
  }
})

const tokenPricing: UserSupportedModelPricing = {
  billing_mode: BILLING_MODE_TOKEN,
  input_price: 0.000005,
  output_price: 0.00001,
  cache_write_price: null,
  cache_read_price: null,
  image_output_price: 0.00003,
  per_request_price: null,
  intervals: []
}

const imageGroup = {
  imageBillingEnabled: true,
  imageRateIndependent: false,
  imageRateMultiplier: 1,
  imagePrice1K: 0.02,
  imagePrice2K: 0.04,
  imagePrice4K: 0.04
}

describe('model square image pricing helpers', () => {
  it('lets explicit group image pricing override a token channel mode', () => {
    expect(effectiveModelBillingMode('image', tokenPricing, imageGroup)).toBe(BILLING_MODE_IMAGE)
    expect(effectiveModelBillingMode('image', tokenPricing, { imageBillingEnabled: false })).toBe(BILLING_MODE_TOKEN)
  })

  it('uses independent image rate and formats exact screenshot-style tiers', () => {
    expect(effectiveImageGroupRate({ imageRateIndependent: true, imageRateMultiplier: 0.5 }, 0.1)).toBe(0.5)
    expect(effectiveImageGroupRate({ imageRateIndependent: false, imageRateMultiplier: 0.5 }, 0.1)).toBe(0.1)

    const tiers = resolveImageTierPrices(tokenPricing, [imageGroup])
    expect(tiers.map((tier) => [tier.tier, formatImagePriceRange(tier)])).toEqual([
      ['1K', '$0.020'],
      ['2K', '$0.040'],
      ['4K', '$0.040']
    ])
  })

  it('shows a range when accessible image groups use different base prices', () => {
    const tiers = resolveImageTierPrices(tokenPricing, [
      imageGroup,
      { ...imageGroup, imagePrice1K: 0.03 }
    ])
    expect(formatImagePriceRange(tiers[0])).toBe('$0.020–$0.030')
  })

  it('does not substitute channel prices for missing group override tiers', () => {
    const nativeImagePricing: UserSupportedModelPricing = {
      ...tokenPricing,
      billing_mode: BILLING_MODE_IMAGE,
      per_request_price: 0.05
    }
    const tiers = resolveImageTierPrices(nativeImagePricing, [{
      ...imageGroup,
      imagePrice2K: null,
      imagePrice4K: null
    }])

    expect(tiers.map((tier) => [tier.tier, formatImagePriceRange(tier)])).toEqual([
      ['1K', '$0.020']
    ])
  })
})

describe('ModelSquareView image model cards', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getUserGroupRates.mockResolvedValue({ 1: 0.8, 2: 0.2, 4: 0.6 })
    getAvailableChannels.mockResolvedValue([
      {
        name: 'OpenAI channel',
        description: '',
        platforms: [
          {
            platform: 'openai',
            groups: [
              {
                id: 1,
                name: 'Image group',
                platform: 'openai',
                subscription_type: 'standard',
                rate_multiplier: 1,
                peak_rate_enabled: false,
                peak_start: '',
                peak_end: '',
                peak_rate_multiplier: 1,
                is_exclusive: false,
                allow_image_generation: true,
                image_billing_enabled: true,
                image_rate_independent: false,
                image_rate_multiplier: 1,
                image_price_1k: 0.02,
                image_price_2k: 0.04,
                image_price_4k: 0.04
              },
              {
                id: 2,
                name: 'High concurrency image group',
                platform: 'openai',
                subscription_type: 'standard',
                rate_multiplier: 1,
                peak_rate_enabled: false,
                peak_start: '',
                peak_end: '',
                peak_rate_multiplier: 1,
                is_exclusive: false,
                allow_image_generation: true,
                image_billing_enabled: true,
                image_rate_independent: true,
                image_rate_multiplier: 0.5,
                image_price_1k: 0.02,
                image_price_2k: 0.04,
                image_price_4k: 0.04
              },
              {
                id: 3,
                name: 'No image access',
                platform: 'openai',
                subscription_type: 'standard',
                rate_multiplier: 1,
                peak_rate_enabled: false,
                peak_start: '',
                peak_end: '',
                peak_rate_multiplier: 1,
                is_exclusive: false,
                allow_image_generation: false,
                image_billing_enabled: true,
                image_rate_independent: false,
                image_rate_multiplier: 1,
                image_price_1k: 0.01,
                image_price_2k: 0.02,
                image_price_4k: 0.03
              },
              {
                id: 4,
                name: 'Token image group',
                platform: 'openai',
                subscription_type: 'standard',
                rate_multiplier: 1,
                peak_rate_enabled: false,
                peak_start: '',
                peak_end: '',
                peak_rate_multiplier: 1,
                is_exclusive: false,
                allow_image_generation: true,
                image_billing_enabled: false,
                image_rate_independent: false,
                image_rate_multiplier: 1,
                image_price_1k: null,
                image_price_2k: null,
                image_price_4k: null
              }
            ],
            supported_models: [
              {
                name: 'gpt-image-2',
                platform: 'openai',
                media_type: 'image',
                pricing: tokenPricing
              }
            ]
          }
        ]
      }
    ])
  })

  it('renders separate per-image and token cards with compatible groups only', async () => {
    const wrapper = mount(ModelSquareView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
          ModelIcon: true,
          PlatformIcon: true
        }
      }
    })

    await flushPromises()

    const cards = wrapper.findAll('article')
    expect(cards).toHaveLength(2)

    const imageCard = cards.find((card) => card.text().includes('Per Image'))
    const tokenCard = cards.find((card) => card.text().includes('Per Token'))
    expect(imageCard).toBeDefined()
    expect(tokenCard).toBeDefined()

    const imageText = imageCard!.text()
    expect(imageText).toContain('1K$0.020')
    expect(imageText).toContain('2K$0.040')
    expect(imageText).toContain('4K$0.040')
    expect(imageText).toContain('/ image')
    expect(imageText).toContain('Image group0.8x')
    expect(imageText).toContain('High concurrency image group0.5x')
    expect(imageText).not.toContain('No image access')

    expect(tokenCard!.text()).toContain('Token image group0.6x')
  })

  it('keeps desktop filters and results in separate scroll regions', async () => {
    const wrapper = mount(ModelSquareView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
          ModelIcon: true,
          PlatformIcon: true
        }
      }
    })

    await flushPromises()

    const filterRegion = wrapper.get('[data-testid="model-filter-scroll-region"]')
    const resultsRegion = wrapper.get('[data-testid="model-results-scroll-region"]')

    expect(filterRegion.element).not.toBe(resultsRegion.element)
    expect(filterRegion.classes()).toEqual(expect.arrayContaining(['overflow-y-auto', 'overscroll-contain']))
    expect(resultsRegion.classes()).toEqual(expect.arrayContaining(['overflow-y-auto', 'overscroll-contain']))
    expect(filterRegion.attributes('tabindex')).toBe('0')
    expect(resultsRegion.attributes('tabindex')).toBe('0')
    expect(filterRegion.attributes('aria-label')).toBe('Model filters')
    expect(resultsRegion.attributes('aria-label')).toBe('Model results')
    expect(filterRegion.find('.grid-cols-2').exists()).toBe(true)
  })
})

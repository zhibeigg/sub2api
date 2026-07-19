import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ModelSquareView from '../ModelSquareView.vue'
import ModelDetailsDrawer from '@/components/channels/ModelDetailsDrawer.vue'
import {
  effectiveModelBillingMode,
  formatImagePriceRange,
  resolveImageTierPrices
} from '../modelSquarePricing'
import { BILLING_MODE_IMAGE, BILLING_MODE_TOKEN } from '@/constants/channel'
import type { UserSupportedModelPricing } from '@/api/channels'

const { getAvailableChannels, showError } = vi.hoisted(() => ({
  getAvailableChannels: vi.fn(),
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
  'modelSquare.openDetails': 'Open details',
  'modelSquare.details.copyModel': 'Copy model',
  'modelSquare.details.close': 'Close details',
  'modelSquare.details.overview': 'Overview',
  'modelSquare.details.provider': 'Provider',
  'modelSquare.details.billing': 'Billing',
  'modelSquare.details.availableGroups': 'Available groups',
  'modelSquare.details.apiEndpoints': 'API endpoints',
  'modelSquare.details.apiEndpointsHint': 'Endpoint hint',
  'modelSquare.details.endpointGroups': 'Available groups count',
  'modelSquare.details.noEndpoints': 'No endpoints',
  'modelSquare.details.groupPricing': 'Group pricing',
  'modelSquare.details.groupPricingHint': 'Pricing hint',
  'modelSquare.details.noGroupPricing': 'No group pricing',
  'modelSquare.details.noConfiguredPrice': 'No configured price',
  'modelSquare.details.priceUnit': 'Current configured prices',
  'modelSquare.details.units.perSecond': 'USD / second',
  'modelSquare.details.endpoints.chatCompletions': 'OpenAI Chat Completions',
  'modelSquare.details.endpoints.messages': 'Anthropic Messages',
  'modelSquare.details.endpoints.responses': 'OpenAI Responses',
  'modelSquare.details.endpoints.gemini': 'Gemini Generate Content',
  'modelSquare.details.endpoints.images': 'OpenAI Images generation',
  'modelSquare.details.endpoints.imageEdits': 'OpenAI Images edits',
  'modelSquare.details.endpoints.videos': 'OpenAI Videos',
  'modelSquare.details.endpoints.videoEdits': 'OpenAI Videos edits',
  'modelSquare.details.endpoints.videoExtensions': 'OpenAI Videos extensions',
  'modelSquare.details.endpoints.videoStatus': 'OpenAI Videos status',
  'modelSquare.details.endpoints.videoContent': 'OpenAI Videos content',
  'modelSquare.details.table.group': 'Group',
  'modelSquare.details.table.channel': 'Channel',
  'modelSquare.details.table.billing': 'Billing',
  'modelSquare.details.table.input': 'Input',
  'modelSquare.details.table.output': 'Output',
  'modelSquare.details.table.cacheWrite': 'Cache write',
  'modelSquare.details.table.cacheRead': 'Cache read',
  'modelSquare.details.table.rate': 'Rate',
  'modelSquare.filters.all': 'All',
  'modelSquare.filters.provider': 'Provider',
  'modelSquare.filters.group': 'Group',
  'modelSquare.filters.endpoint': 'Endpoint',
  'modelSquare.filters.billing': 'Billing',
  'modelSquare.endpoints.openai': 'OpenAI compatible',
  'availableChannels.pricing.billingModeToken': 'Per Token',
  'availableChannels.pricing.billingModePerRequest': 'Per Request',
  'availableChannels.pricing.billingModeImage': 'Per Image',
  'availableChannels.pricing.billingModeVideo': 'Per Video',
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
  rate: 0.8,
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

  it('applies the backend effective image rate to screenshot-style tiers', () => {
    const tiers = resolveImageTierPrices(tokenPricing, [imageGroup])
    expect(tiers.map((tier) => [tier.tier, formatImagePriceRange(tier)])).toEqual([
      ['1K', '$0.016'],
      ['2K', '$0.032'],
      ['4K', '$0.032']
    ])
  })

  it('shows a range when accessible image groups use different base prices', () => {
    const tiers = resolveImageTierPrices(tokenPricing, [
      imageGroup,
      { ...imageGroup, imagePrice1K: 0.03 }
    ])
    expect(formatImagePriceRange(tiers[0])).toBe('$0.016–$0.024')
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
      ['1K', '$0.016']
    ])
  })
})

describe('ModelSquareView image model cards', () => {
  beforeEach(() => {
    vi.clearAllMocks()
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
                pricing: tokenPricing,
                group_rates: [
                  {
                    group_id: 1,
                    token_rate_multiplier: 0.8,
                    image_rate_multiplier: 0.8,
                    video_rate_multiplier: 0.8
                  },
                  {
                    group_id: 2,
                    token_rate_multiplier: 0.2,
                    image_rate_multiplier: 0.5,
                    video_rate_multiplier: 0.2
                  },
                  {
                    group_id: 4,
                    token_rate_multiplier: 0.6,
                    image_rate_multiplier: 0.6,
                    video_rate_multiplier: 0.6
                  }
                ]
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
    expect(imageText).toContain('1K$0.010–$0.016')
    expect(imageText).toContain('2K$0.020–$0.032')
    expect(imageText).toContain('4K$0.020–$0.032')
    expect(imageText).toContain('/ image')
    expect(imageText).toContain('Image group0.8x')
    expect(imageText).toContain('High concurrency image group0.5x')
    expect(imageText).not.toContain('No image access')

    expect(tokenCard!.text()).toContain('Token image group0.6x')
  })

  it('does not guess effective multipliers when the backend snapshot is missing', async () => {
    getAvailableChannels.mockResolvedValue([
      {
        name: 'Missing snapshot channel',
        description: '',
        platforms: [
          {
            platform: 'openai',
            groups: [
              {
                id: 9,
                name: 'Missing snapshot group',
                platform: 'openai',
                subscription_type: 'standard',
                rate_multiplier: 0.1,
                peak_rate_enabled: true,
                peak_start: '00:00',
                peak_end: '23:59',
                peak_rate_multiplier: 9,
                is_exclusive: false,
                allow_messages_dispatch: false,
                allow_image_generation: false,
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
                name: 'gpt-4.1',
                platform: 'openai',
                media_type: '',
                pricing: tokenPricing,
                group_rates: []
              }
            ]
          }
        ]
      }
    ])

    const wrapper = mount(ModelSquareView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
          ModelIcon: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.findAll('article')).toHaveLength(0)
    expect(wrapper.text()).toContain('Empty')
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

  it('shows the Kimi brand and opens endpoint details with effective group pricing', async () => {
    getAvailableChannels.mockResolvedValue([
      {
        name: 'Kimi low-price channel',
        description: '',
        platforms: [
          {
            platform: 'opencode',
            groups: [
              {
                id: 8,
                name: 'Kimi low-price group',
                platform: 'opencode',
                subscription_type: 'standard',
                rate_multiplier: 1.2,
                peak_rate_enabled: false,
                peak_start: '',
                peak_end: '',
                peak_rate_multiplier: 1,
                is_exclusive: false,
                allow_messages_dispatch: false,
                allow_image_generation: false,
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
                name: 'kimi-k3',
                platform: 'opencode',
                media_type: '',
                pricing: {
                  billing_mode: BILLING_MODE_TOKEN,
                  input_price: 0.000002,
                  output_price: 0.00001,
                  cache_write_price: null,
                  cache_read_price: 0.0000002,
                  image_input_price: null,
                  image_output_price: null,
                  per_request_price: null,
                  intervals: []
                },
                group_rates: [
                  {
                    group_id: 8,
                    token_rate_multiplier: 0.5,
                    image_rate_multiplier: 0.5,
                    video_rate_multiplier: 0.5
                  }
                ]
              }
            ]
          }
        ]
      }
    ])

    const appRoot = document.createElement('div')
    appRoot.id = 'app'
    document.body.appendChild(appRoot)
    const wrapper = mount(ModelSquareView, {
      attachTo: appRoot,
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
          ModelIcon: true
        }
      }
    })

    await flushPromises()

    const card = wrapper.get('article')
    expect(card.text()).toContain('Kimi')
    expect(card.text()).not.toContain('OpenCode Go')
    expect(card.text()).not.toContain('OpenAI')

    ;(card.element as HTMLElement).focus()
    expect(document.activeElement).toBe(card.element)
    await card.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    const drawer = document.body.querySelector('[data-testid="model-details-drawer"]')
    expect(drawer).not.toBeNull()
    expect((appRoot as HTMLElement & { inert: boolean }).inert).toBe(true)
    expect(appRoot.getAttribute('aria-hidden')).toBe('true')
    const drawerText = drawer?.textContent ?? ''
    expect(drawerText).toContain('kimi-k3')
    expect(drawerText).toContain('OpenAI Chat Completions')
    expect(drawerText).toContain('/v1/chat/completions')
    expect(drawerText).toContain('Anthropic Messages')
    expect(drawerText).toContain('/v1/messages')
    expect(drawerText).toContain('OpenAI Responses')
    expect(drawerText).toContain('Kimi low-price group')
    expect(drawerText).toContain('Kimi low-price channel')

    const priceRow = document.body.querySelector('[data-testid="model-price-group-8"]')
    expect(priceRow?.textContent).toContain('$1')
    expect(priceRow?.textContent).toContain('$5')
    expect(priceRow?.textContent).toContain('$0.1')
    expect(priceRow?.textContent).toContain('0.5x')

    ;(document.body.querySelector('[data-testid="model-details-close"]') as HTMLButtonElement).click()
    await flushPromises()
    expect(wrapper.getComponent(ModelDetailsDrawer).props('show')).toBe(false)
    expect((appRoot as HTMLElement & { inert: boolean }).inert).toBe(false)
    expect(appRoot.hasAttribute('aria-hidden')).toBe(false)
    expect(document.activeElement).toBe(card.element)

    await card.trigger('click')
    await flushPromises()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()
    expect(wrapper.getComponent(ModelDetailsDrawer).props('show')).toBe(false)

    wrapper.unmount()
    appRoot.remove()
  })
})

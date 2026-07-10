import { describe, expect, it } from 'vitest'
import { ADOBE_PUBLIC_MODELS, PLATFORM_ORDER, PLATFORM_REGISTRY, QUOTA_PLATFORMS } from '@/constants/platforms'
import { getModelsByPlatform } from '@/composables/useModelWhitelist'
import { platformBadgeClass, platformLabel } from '@/utils/platformColors'
import { resolveGroupBrand } from '@/utils/groupBrand'

describe('Adobe platform registry', () => {
  it('registers Adobe after Grok with media, quota, models and usage capabilities', () => {
    const adobe = PLATFORM_REGISTRY.adobe
    expect(PLATFORM_ORDER.indexOf('adobe')).toBeGreaterThan(PLATFORM_ORDER.indexOf('grok'))
    expect(adobe.capabilities).toMatchObject({ quota: true, image: true, video: true, models: true, usage: true, modelSync: false })
    expect(QUOTA_PLATFORMS).toContain('adobe')
  })

  it('keeps the exact public Firefly model catalog', () => {
    expect(getModelsByPlatform('adobe')).toEqual([...ADOBE_PUBLIC_MODELS])
  })

  it('resolves Adobe label, restrained red badge and group brand', () => {
    expect(platformLabel('adobe')).toBe('Adobe')
    expect(platformBadgeClass('adobe')).toContain('red')
    expect(resolveGroupBrand('adobe', 'Firefly Media').brand).toBe('adobe')
  })
})

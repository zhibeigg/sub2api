import { describe, expect, it } from 'vitest'
import { ADOBE_PUBLIC_MODELS, PLATFORM_ORDER, PLATFORM_REGISTRY, QUOTA_PLATFORMS } from '@/constants/platforms'
import { getModelsByPlatform } from '@/composables/useModelWhitelist'
import { platformBadgeClass, platformLabel } from '@/utils/platformColors'
import { resolveGroupBrand } from '@/utils/groupBrand'

describe('OpenCode Go platform registry', () => {
  it('registers OpenCode Go with quota, models and usage capabilities', () => {
    const opencode = PLATFORM_REGISTRY.opencode
    expect(PLATFORM_ORDER.indexOf('opencode')).toBeGreaterThan(PLATFORM_ORDER.indexOf('cursor'))
    expect(PLATFORM_ORDER.indexOf('opencode')).toBeLessThan(PLATFORM_ORDER.indexOf('kiro'))
    expect(opencode.label).toBe('OpenCode Go')
    expect(opencode.capabilities).toMatchObject({ quota: true, models: true, usage: true, modelSync: false })
    expect(QUOTA_PLATFORMS).toContain('opencode')
    expect(platformLabel('opencode')).toBe('OpenCode Go')
    expect(platformBadgeClass('opencode')).toContain('teal')
    expect(getModelsByPlatform('opencode')).toEqual([
      'grok-4.5', 'glm-5.2', 'glm-5.1', 'kimi-k3', 'kimi-k2.7-code', 'kimi-k2.6',
      'deepseek-v4-pro', 'deepseek-v4-flash', 'mimo-v2.5', 'mimo-v2.5-pro',
      'minimax-m3', 'minimax-m2.7', 'minimax-m2.5', 'qwen3.7-max', 'qwen3.7-plus', 'qwen3.6-plus'
    ])
    expect(resolveGroupBrand('opencode', 'OpenCode Go').brand).toBe('opencode')
  })
})

describe('Cursor platform registry', () => {
  it('registers Cursor between Adobe and Kiro with the requested capabilities', () => {
    const cursor = PLATFORM_REGISTRY.cursor
    expect(PLATFORM_ORDER.indexOf('cursor')).toBeGreaterThan(PLATFORM_ORDER.indexOf('adobe'))
    expect(PLATFORM_ORDER.indexOf('cursor')).toBeLessThan(PLATFORM_ORDER.indexOf('kiro'))
    expect(cursor.capabilities).toEqual({
      quota: true,
      image: false,
      video: false,
      models: true,
      usage: true,
      modelSync: true
    })
    expect(QUOTA_PLATFORMS).toContain('cursor')
    expect(platformLabel('cursor')).toBe('Cursor')
    expect(platformBadgeClass('cursor')).toContain('cyan')
    expect(getModelsByPlatform('cursor').length).toBeGreaterThan(0)
  })
})

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

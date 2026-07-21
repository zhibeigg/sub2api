import { beforeEach, describe, expect, it, vi } from 'vitest'

const appStore = vi.hoisted(() => ({
  cachedPublicSettings: null as null | {
    available_channels_enabled?: boolean
    model_square_enabled?: boolean
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))

import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'

describe('model square and available channels feature flags', () => {
  beforeEach(() => {
    appStore.cachedPublicSettings = null
  })

  it.each([
    [false, false],
    [false, true],
    [true, false],
    [true, true],
  ])(
    'resolves available channels=%s and model square=%s independently',
    (availableChannelsEnabled, modelSquareEnabled) => {
      appStore.cachedPublicSettings = {
        available_channels_enabled: availableChannelsEnabled,
        model_square_enabled: modelSquareEnabled,
      }

      expect(isFeatureFlagEnabled(FeatureFlags.availableChannels)).toBe(availableChannelsEnabled)
      expect(isFeatureFlagEnabled(FeatureFlags.modelSquare)).toBe(modelSquareEnabled)
    },
  )

  it('defaults both opt-in flags to disabled when settings are missing', () => {
    expect(isFeatureFlagEnabled(FeatureFlags.availableChannels)).toBe(false)
    expect(isFeatureFlagEnabled(FeatureFlags.modelSquare)).toBe(false)
  })
})

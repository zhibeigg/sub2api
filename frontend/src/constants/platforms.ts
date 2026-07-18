import type { AccountPlatform, GroupPlatform } from '@/types'

export interface PlatformDefinition {
  value: AccountPlatform & GroupPlatform
  label: string
  order: number
  capabilities: {
    quota: boolean
    image: boolean
    video: boolean
    models: boolean
    usage: boolean
    modelSync: boolean
  }
}

export const PLATFORM_REGISTRY: Record<AccountPlatform & GroupPlatform, PlatformDefinition> = {
  anthropic: { value: 'anthropic', label: 'Anthropic', order: 10, capabilities: { quota: true, image: false, video: false, models: true, usage: true, modelSync: true } },
  openai: { value: 'openai', label: 'OpenAI', order: 20, capabilities: { quota: true, image: true, video: false, models: true, usage: true, modelSync: true } },
  gemini: { value: 'gemini', label: 'Gemini', order: 30, capabilities: { quota: true, image: true, video: false, models: true, usage: true, modelSync: true } },
  antigravity: { value: 'antigravity', label: 'Antigravity', order: 40, capabilities: { quota: true, image: true, video: false, models: true, usage: true, modelSync: true } },
  grok: { value: 'grok', label: 'Grok', order: 50, capabilities: { quota: true, image: true, video: true, models: true, usage: true, modelSync: true } },
  adobe: { value: 'adobe', label: 'Adobe', order: 60, capabilities: { quota: true, image: true, video: true, models: true, usage: true, modelSync: false } },
  cursor: { value: 'cursor', label: 'Cursor', order: 65, capabilities: { quota: true, image: false, video: false, models: true, usage: true, modelSync: true } },
  opencode: { value: 'opencode', label: 'OpenCode Go', order: 68, capabilities: { quota: true, image: false, video: false, models: true, usage: true, modelSync: true } },
  kiro: { value: 'kiro', label: 'Kiro', order: 70, capabilities: { quota: false, image: false, video: false, models: true, usage: true, modelSync: false } },
}

export const PLATFORM_ORDER = Object.values(PLATFORM_REGISTRY)
  .sort((a, b) => a.order - b.order)
  .map((item) => item.value)

export const QUOTA_PLATFORMS = PLATFORM_ORDER.filter((platform) => PLATFORM_REGISTRY[platform].capabilities.quota)
export const ADOBE_PUBLIC_MODELS = [
  'nano-banana-pro',
  'nano-banana-v2',
  'nano-banana',
  'veo3',
  'veo3.1',
  'sora',
  'sora-2-pro',
] as const

export function getPlatformDefinition(platform: string): PlatformDefinition | undefined {
  return PLATFORM_REGISTRY[platform as keyof typeof PLATFORM_REGISTRY]
}

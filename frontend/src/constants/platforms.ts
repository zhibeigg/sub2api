import type {
  AccountPlatform,
  EndpointProtocol,
  LegacyGroupPlatform
} from '@/types'

export interface PlatformDefinition {
  value: AccountPlatform & LegacyGroupPlatform
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

export const PLATFORM_REGISTRY: Record<AccountPlatform & LegacyGroupPlatform, PlatformDefinition> = {
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

export type EndpointProtocolIcon = 'chat' | 'terminal' | 'sparkles' | 'search' | 'grid' | 'play'

export interface EndpointProtocolDefinition {
  value: EndpointProtocol
  label: string
  shortLabel: string
  icon: EndpointProtocolIcon
  colorClass: string
  selectedClass: string
  order: number
  capability: 'text' | 'embeddings' | 'search' | 'image' | 'video' | 'batch_image'
}

export const ENDPOINT_PROTOCOL_REGISTRY: Record<EndpointProtocol, EndpointProtocolDefinition> = {
  anthropic_messages: {
    value: 'anthropic_messages',
    label: 'Anthropic Messages',
    shortLabel: 'Messages',
    icon: 'chat',
    colorClass: 'bg-orange-50 text-orange-700 dark:bg-orange-900/20 dark:text-orange-300',
    selectedClass: 'border-orange-400 bg-orange-50 text-orange-700 dark:border-orange-600 dark:bg-orange-900/20 dark:text-orange-300',
    order: 10,
    capability: 'text'
  },
  openai_chat_completions: {
    value: 'openai_chat_completions',
    label: 'OpenAI Chat Completions',
    shortLabel: 'Chat Completions',
    icon: 'chat',
    colorClass: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300',
    selectedClass: 'border-emerald-400 bg-emerald-50 text-emerald-700 dark:border-emerald-600 dark:bg-emerald-900/20 dark:text-emerald-300',
    order: 20,
    capability: 'text'
  },
  openai_responses: {
    value: 'openai_responses',
    label: 'OpenAI Responses',
    shortLabel: 'Responses',
    icon: 'terminal',
    colorClass: 'bg-teal-50 text-teal-700 dark:bg-teal-900/20 dark:text-teal-300',
    selectedClass: 'border-teal-400 bg-teal-50 text-teal-700 dark:border-teal-600 dark:bg-teal-900/20 dark:text-teal-300',
    order: 30,
    capability: 'text'
  },
  gemini_generate_content: {
    value: 'gemini_generate_content',
    label: 'Gemini GenerateContent',
    shortLabel: 'GenerateContent',
    icon: 'sparkles',
    colorClass: 'bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-300',
    selectedClass: 'border-blue-400 bg-blue-50 text-blue-700 dark:border-blue-600 dark:bg-blue-900/20 dark:text-blue-300',
    order: 40,
    capability: 'text'
  },
  openai_embeddings: {
    value: 'openai_embeddings',
    label: 'OpenAI Embeddings',
    shortLabel: 'Embeddings',
    icon: 'grid',
    colorClass: 'bg-cyan-50 text-cyan-700 dark:bg-cyan-900/20 dark:text-cyan-300',
    selectedClass: 'border-cyan-400 bg-cyan-50 text-cyan-700 dark:border-cyan-600 dark:bg-cyan-900/20 dark:text-cyan-300',
    order: 50,
    capability: 'embeddings'
  },
  openai_alpha_search: {
    value: 'openai_alpha_search',
    label: 'OpenAI Alpha Search',
    shortLabel: 'Alpha Search',
    icon: 'search',
    colorClass: 'bg-indigo-50 text-indigo-700 dark:bg-indigo-900/20 dark:text-indigo-300',
    selectedClass: 'border-indigo-400 bg-indigo-50 text-indigo-700 dark:border-indigo-600 dark:bg-indigo-900/20 dark:text-indigo-300',
    order: 60,
    capability: 'search'
  },
  openai_images: {
    value: 'openai_images',
    label: 'OpenAI Images',
    shortLabel: 'Images',
    icon: 'sparkles',
    colorClass: 'bg-fuchsia-50 text-fuchsia-700 dark:bg-fuchsia-900/20 dark:text-fuchsia-300',
    selectedClass: 'border-fuchsia-400 bg-fuchsia-50 text-fuchsia-700 dark:border-fuchsia-600 dark:bg-fuchsia-900/20 dark:text-fuchsia-300',
    order: 70,
    capability: 'image'
  },
  openai_videos: {
    value: 'openai_videos',
    label: 'OpenAI Videos',
    shortLabel: 'Videos',
    icon: 'play',
    colorClass: 'bg-violet-50 text-violet-700 dark:bg-violet-900/20 dark:text-violet-300',
    selectedClass: 'border-violet-400 bg-violet-50 text-violet-700 dark:border-violet-600 dark:bg-violet-900/20 dark:text-violet-300',
    order: 80,
    capability: 'video'
  },
  batch_images: {
    value: 'batch_images',
    label: 'Batch Images',
    shortLabel: 'Batch Images',
    icon: 'grid',
    colorClass: 'bg-pink-50 text-pink-700 dark:bg-pink-900/20 dark:text-pink-300',
    selectedClass: 'border-pink-400 bg-pink-50 text-pink-700 dark:border-pink-600 dark:bg-pink-900/20 dark:text-pink-300',
    order: 90,
    capability: 'batch_image'
  }
}

export const ENDPOINT_PROTOCOL_ORDER = Object.values(ENDPOINT_PROTOCOL_REGISTRY)
  .sort((a, b) => a.order - b.order)
  .map((item) => item.value)

const LEGACY_GROUP_PROTOCOLS: Record<LegacyGroupPlatform, EndpointProtocol[]> = {
  anthropic: ['anthropic_messages', 'openai_chat_completions', 'openai_responses'],
  openai: ['openai_chat_completions', 'openai_responses', 'openai_embeddings', 'openai_alpha_search'],
  gemini: ['gemini_generate_content', 'openai_chat_completions', 'openai_responses'],
  antigravity: ['anthropic_messages', 'openai_chat_completions', 'openai_responses', 'gemini_generate_content'],
  grok: ['anthropic_messages', 'openai_chat_completions', 'openai_responses', 'openai_videos'],
  adobe: ['openai_images', 'openai_videos'],
  cursor: ['anthropic_messages', 'openai_chat_completions', 'openai_responses'],
  opencode: ['anthropic_messages', 'openai_chat_completions', 'openai_responses'],
  kiro: ['anthropic_messages', 'openai_chat_completions', 'openai_responses']
}

export interface LegacyEndpointProtocolSource {
  platform?: LegacyGroupPlatform | string | null
  endpoint_protocols?: EndpointProtocol[] | null
  allow_messages_dispatch?: boolean
  allow_image_generation?: boolean
  allow_batch_image_generation?: boolean
  video_rate_independent?: boolean
  video_price_480p?: number | null
  video_price_720p?: number | null
  video_price_1080p?: number | null
}

function sortEndpointProtocols(protocols: Iterable<EndpointProtocol>): EndpointProtocol[] {
  const values = new Set(protocols)
  return ENDPOINT_PROTOCOL_ORDER.filter((protocol) => values.has(protocol))
}

export function getLegacyGroupEndpointProtocols(group: LegacyEndpointProtocolSource): EndpointProtocol[] {
  const platform = group.platform as LegacyGroupPlatform | undefined
  const protocols = new Set<EndpointProtocol>(platform ? LEGACY_GROUP_PROTOCOLS[platform] ?? [] : [])

  if (group.allow_messages_dispatch) protocols.add('anthropic_messages')
  if (group.allow_image_generation) protocols.add('openai_images')
  if (group.allow_batch_image_generation) protocols.add('batch_images')
  if (
    platform === 'adobe' ||
    platform === 'grok' ||
    group.video_rate_independent ||
    group.video_price_480p != null ||
    group.video_price_720p != null ||
    group.video_price_1080p != null
  ) {
    protocols.add('openai_videos')
  }

  return sortEndpointProtocols(protocols)
}

export function getGroupEndpointProtocols(group: LegacyEndpointProtocolSource): EndpointProtocol[] {
  if (Array.isArray(group.endpoint_protocols) && group.endpoint_protocols.length > 0) {
    return sortEndpointProtocols(group.endpoint_protocols.filter(
      (protocol): protocol is EndpointProtocol => protocol in ENDPOINT_PROTOCOL_REGISTRY
    ))
  }
  return getLegacyGroupEndpointProtocols(group)
}

export function getLegacyAccountEndpointProtocols(
  platform: AccountPlatform | undefined,
  mixedScheduling = false
): EndpointProtocol[] {
  if (!platform) return []

  const protocols = new Set<EndpointProtocol>()
  switch (platform) {
    case 'anthropic':
      protocols.add('anthropic_messages')
      break
    case 'openai':
      protocols.add('openai_chat_completions')
      protocols.add('openai_responses')
      protocols.add('openai_embeddings')
      protocols.add('openai_alpha_search')
      protocols.add('openai_images')
      protocols.add('openai_videos')
      if (mixedScheduling) protocols.add('anthropic_messages')
      break
    case 'gemini':
      protocols.add('gemini_generate_content')
      break
    case 'antigravity':
      protocols.add('anthropic_messages')
      protocols.add('openai_chat_completions')
      protocols.add('openai_responses')
      protocols.add('gemini_generate_content')
      break
    case 'grok':
      protocols.add('anthropic_messages')
      protocols.add('openai_chat_completions')
      protocols.add('openai_responses')
      protocols.add('openai_images')
      protocols.add('openai_videos')
      break
    case 'adobe':
      protocols.add('openai_images')
      protocols.add('openai_videos')
      break
    case 'cursor':
      protocols.add('openai_chat_completions')
      protocols.add('openai_responses')
      if (mixedScheduling) {
        protocols.add('anthropic_messages')
        protocols.add('gemini_generate_content')
      }
      break
    case 'opencode':
    case 'kiro':
      protocols.add('openai_chat_completions')
      protocols.add('openai_responses')
      if (mixedScheduling) protocols.add('anthropic_messages')
      break
  }
  return sortEndpointProtocols(protocols)
}

export function endpointProtocolsIntersect(
  left: readonly EndpointProtocol[],
  right: readonly EndpointProtocol[]
): boolean {
  const rightSet = new Set(right)
  return left.some((protocol) => rightSet.has(protocol))
}

export function groupSupportsEndpointCapability(
  group: LegacyEndpointProtocolSource,
  capability: EndpointProtocolDefinition['capability']
): boolean {
  return getGroupEndpointProtocols(group).some(
    (protocol) => ENDPOINT_PROTOCOL_REGISTRY[protocol].capability === capability
  )
}

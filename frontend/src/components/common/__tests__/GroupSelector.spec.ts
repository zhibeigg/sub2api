import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import GroupSelector from '../GroupSelector.vue'
import type { AdminGroup, GroupPlatform } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const platforms: GroupPlatform[] = [
  'cursor',
  'opencode',
  'anthropic',
  'gemini',
  'openai',
  'grok',
  'adobe',
  'antigravity',
  'kiro'
]

const groups = platforms.map((platform, index) => ({
  id: index + 1,
  name: `${platform}-group`,
  description: null,
  platform,
  rate_multiplier: 1,
  account_count: 0
})) as AdminGroup[]

function visiblePlatforms(mixedScheduling: boolean, platform: GroupPlatform = 'cursor'): string[] {
  const wrapper = mount(GroupSelector, {
    props: {
      modelValue: [],
      groups,
      platform,
      mixedScheduling
    },
    global: {
      stubs: {
        Icon: true,
        GroupBadge: {
          props: ['name', 'platform'],
          template: '<span data-group-badge :data-platform="platform">{{ name }}</span>'
        }
      }
    }
  })

  return wrapper.findAll('[data-group-badge]').map((node) => node.attributes('data-platform'))
}

describe('GroupSelector Cursor mixed scheduling', () => {
  it('shows all model-family group platforms supported by Cursor', () => {
    expect(visiblePlatforms(true)).toEqual(['cursor', 'anthropic', 'gemini', 'openai', 'grok'])
  })

  it('keeps Cursor-only filtering when mixed scheduling is disabled', () => {
    expect(visiblePlatforms(false)).toEqual(['cursor'])
  })

  it('supports OpenCode Go mixed scheduling with OpenAI and Anthropic groups', () => {
    expect(visiblePlatforms(true, 'opencode')).toEqual(['opencode', 'anthropic', 'openai'])
  })
})

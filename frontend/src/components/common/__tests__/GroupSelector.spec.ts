import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import GroupSelector from '../GroupSelector.vue'
import type { AdminGroup, EndpointProtocol, GroupPlatform } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

function group(
  id: number,
  name: string,
  platform: GroupPlatform,
  endpointProtocols?: EndpointProtocol[]
): AdminGroup {
  return {
    id,
    name,
    description: null,
    platform,
    endpoint_protocols: endpointProtocols as EndpointProtocol[],
    quota_platform: platform,
    rate_multiplier: 1,
    account_count: 0
  } as AdminGroup
}

const groups = [
  group(1, 'messages-group', 'opencode', ['anthropic_messages']),
  group(2, 'chat-group', 'openai', ['openai_chat_completions']),
  group(3, 'responses-group', 'cursor', ['openai_responses']),
  group(4, 'gemini-group', 'gemini', ['gemini_generate_content']),
  group(5, 'legacy-anthropic', 'anthropic', undefined),
  group(6, 'legacy-cursor', 'cursor', undefined),
  group(7, 'legacy-openai', 'openai', undefined),
  group(8, 'legacy-grok', 'grok', undefined),
]

function mountSelector(props: Record<string, unknown>) {
  return mount(GroupSelector, {
    props: {
      modelValue: [],
      groups,
      ...props
    },
    global: {
      stubs: {
        Icon: true,
        GroupBadge: {
          props: ['name', 'platform', 'endpointProtocols'],
          template: '<span data-group-badge :data-platform="platform">{{ name }}</span>'
        }
      }
    }
  })
}

function visibleNames(wrapper: ReturnType<typeof mountSelector>): string[] {
  return wrapper.findAll('[data-group-badge]').map((node) => node.text())
}

describe('GroupSelector endpoint protocol compatibility', () => {
  it('filters by the intersection of account and group endpoint protocols', () => {
    const wrapper = mountSelector({
      supportedEndpointProtocols: ['anthropic_messages'] satisfies EndpointProtocol[]
    })

    expect(visibleNames(wrapper)).toEqual(['messages-group', 'legacy-anthropic', 'legacy-cursor', 'legacy-grok'])
  })

  it('uses the legacy group mapper when a group response has no endpoint_protocols', () => {
    const wrapper = mountSelector({
      supportedEndpointProtocols: ['openai_chat_completions'] satisfies EndpointProtocol[]
    })

    expect(visibleNames(wrapper)).toEqual([
      'chat-group',
      'legacy-anthropic',
      'legacy-cursor',
      'legacy-openai',
      'legacy-grok'
    ])
  })

  it('keeps a selected incompatible group visible and warns instead of silently dropping it', () => {
    const wrapper = mountSelector({
      modelValue: [4],
      supportedEndpointProtocols: ['openai_chat_completions'] satisfies EndpointProtocol[]
    })

    expect(visibleNames(wrapper)).toContain('gemini-group')
    expect(wrapper.get('[data-incompatible="true"]').text()).toContain('gemini-group')
    expect(wrapper.text()).toContain('admin.groups.endpointProtocols.incompatibleSelectedWarning')
  })

  it('keeps old platform and mixedScheduling props working when protocols are not provided', () => {
    const wrapper = mountSelector({ platform: 'cursor', mixedScheduling: true })

    expect(visibleNames(wrapper)).toEqual([
      'chat-group',
      'responses-group',
      'gemini-group',
      'legacy-anthropic',
      'legacy-cursor',
      'legacy-openai',
      'legacy-grok'
    ])
  })

  it('preserves strict legacy filtering when mixed scheduling is disabled', () => {
    const wrapper = mountSelector({ platform: 'cursor', mixedScheduling: false })
    expect(visibleNames(wrapper)).toEqual(['responses-group', 'legacy-cursor'])
  })
})

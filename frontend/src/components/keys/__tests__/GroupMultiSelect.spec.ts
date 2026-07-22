import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import GroupMultiSelect from '../GroupMultiSelect.vue'
import type { Group } from '@/types'

const { getAvailable, getModelSquare } = vi.hoisted(() => ({
  getAvailable: vi.fn(),
  getModelSquare: vi.fn()
}))

vi.mock('@/api/channels', () => ({
  default: { getAvailable, getModelSquare }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      key === 'keys.groupModels' ? `${String(params?.count ?? 0)} models` : key
  })
}))

const groups = [
  { id: 1, name: 'Claude Team', platform: 'anthropic', rate_multiplier: 1 },
  { id: 2, name: 'Codex Pro', platform: 'openai', rate_multiplier: 0.8 }
] as Group[]

const SortableGroupBindingPickerStub = {
  name: 'SortableGroupBindingPicker',
  props: ['modelValue'],
  emits: ['update:modelValue'],
  template: '<div data-test="picker">{{ JSON.stringify(modelValue) }}</div>'
}

describe('GroupMultiSelect', () => {
  it('restores per-group model display and platform color distinction', async () => {
    getModelSquare.mockResolvedValueOnce([
      {
        name: 'Primary channel',
        description: '',
        platforms: [
          {
            platform: 'openai',
            groups: [{ id: 2, name: 'Codex Pro' }],
            supported_models: [
              { name: 'gpt-5.4', platform: 'openai', pricing: null },
              { name: 'codex-auto-review', platform: 'openai', pricing: null }
            ]
          }
        ]
      }
    ])

    const wrapper = mount(GroupMultiSelect, {
      props: {
        groups,
        userGroupRates: {},
        modelValue: [{ group_id: 2, priority: 0 }]
      },
      global: {
        stubs: {
          SortableGroupBindingPicker: SortableGroupBindingPickerStub,
          PlatformIcon: { props: ['platform'], template: '<span>{{ platform }}</span>' },
          ModelIcon: { props: ['model'], template: '<span>{{ model }}</span>' }
        }
      }
    })

    await flushPromises()

    const modelCard = wrapper.get('[data-test="group-models-2"]')
    expect(modelCard.classes()).toContain('text-green-600')
    expect(modelCard.text()).toContain('Codex Pro')
    expect(modelCard.text()).toContain('2 models')
    expect(modelCard.text()).toContain('gpt-5.4')
    expect(modelCard.text()).toContain('codex-auto-review')
    expect(getModelSquare).toHaveBeenCalledWith({ signal: expect.any(AbortSignal) })
    expect(getAvailable).not.toHaveBeenCalled()
  })
})

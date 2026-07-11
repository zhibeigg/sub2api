import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import SortableGroupBindingPicker from '../SortableGroupBindingPicker.vue'
import type { Group } from '@/types'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => params?.name ? `${key}:${params.name}` : key
  })
}))

const groups = [
  { id: 1, name: 'Claude Team', platform: 'anthropic', rate_multiplier: 1.25, description: 'Primary' },
  { id: 2, name: 'Codex Pro', platform: 'openai', rate_multiplier: 0.8, description: 'Fallback' },
  { id: 3, name: 'Gemini Flash', platform: 'gemini', rate_multiplier: 1, description: 'Fast' }
] as Group[]

const VueDraggableStub = {
  name: 'VueDraggable',
  props: ['modelValue'],
  emits: ['update:modelValue', 'end'],
  template: '<div><slot /></div>'
}

const mountPicker = (modelValue = [{ group_id: 2, priority: 0 }]) => mount(SortableGroupBindingPicker, {
  props: {
    groups,
    modelValue,
    userGroupRates: { 2: 0.65 }
  },
  global: {
    stubs: {
      VueDraggable: VueDraggableStub,
      Icon: { props: ['name'], template: '<span>{{ name }}</span>' },
      PlatformIcon: { props: ['platform'], template: '<span>{{ platform }}</span>' }
    }
  }
})

describe('SortableGroupBindingPicker', () => {
  it('renders ordered selections with effective rates and adds/removes groups', async () => {
    const wrapper = mountPicker()

    expect(wrapper.get('[data-test="selected-group-2"]').text()).toContain('Codex Pro')
    expect(wrapper.get('[data-test="selected-group-2"]').text()).toContain('0.65x')

    await wrapper.get('[data-test="add-group-1"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual([
      { group_id: 2, priority: 0 },
      { group_id: 1, priority: 1 }
    ])

    await wrapper.get('[data-test="remove-group-2"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual([
      { group_id: 1, priority: 0 }
    ])
  })

  it('filters available groups by name, platform and description', async () => {
    const wrapper = mountPicker([])
    const search = wrapper.get('[data-test="group-binding-search"]')

    await search.setValue('fallback')
    expect(wrapper.find('[data-test="add-group-2"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="add-group-1"]').exists()).toBe(false)

    await search.setValue('gemini')
    expect(wrapper.find('[data-test="add-group-3"]').exists()).toBe(true)
  })

  it('reindexes the complete binding list through keyboard-accessible sorting', async () => {
    const wrapper = mountPicker([
      { group_id: 1, priority: 0 },
      { group_id: 2, priority: 1 }
    ])

    await wrapper.get('[data-test="move-up-group-2"]').trigger('click')

    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual([
      { group_id: 2, priority: 0 },
      { group_id: 1, priority: 1 }
    ])
  })

  it('focuses the search field through the exposed keyboard-focus API', async () => {
    const wrapper = mount(SortableGroupBindingPicker, {
      attachTo: document.body,
      props: { groups, modelValue: [], userGroupRates: {} },
      global: {
        stubs: {
          VueDraggable: VueDraggableStub,
          Icon: { props: ['name'], template: '<span>{{ name }}</span>' },
          PlatformIcon: { props: ['platform'], template: '<span>{{ platform }}</span>' }
        }
      }
    })
    await (wrapper.vm as unknown as { focusSearch: () => Promise<void> }).focusSearch()
    expect(document.activeElement).toBe(wrapper.get('[data-test="group-binding-search"]').element)
    wrapper.unmount()
  })
})

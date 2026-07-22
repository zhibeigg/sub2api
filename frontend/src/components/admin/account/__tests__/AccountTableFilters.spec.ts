import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'

import AccountTableFilters from '../AccountTableFilters.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

const SelectStub = defineComponent({
  inheritAttrs: false,
  props: {
    modelValue: { type: [String, Number, Boolean], default: '' },
    options: { type: Array, default: () => [] },
    disabled: { type: Boolean, default: false }
  },
  emits: ['update:modelValue', 'change'],
  template: `
    <button v-bind="$attrs" type="button" :disabled="disabled">
      {{ options.map((option) => option.label).join('|') }}
    </button>
  `
})

function mountFilters(filters: Record<string, unknown>) {
  return mount(AccountTableFilters, {
    props: {
      searchQuery: '',
      filters,
      groups: []
    },
    global: {
      stubs: {
        Select: SelectStub,
        SearchInput: true
      }
    }
  })
}

describe('AccountTableFilters OpenAI plan filter', () => {
  it('offers K12, Free, Plus, Pro and unset options', () => {
    const wrapper = mountFilters({ platform: 'openai', plan_type: '' })
    const planFilter = wrapper.get('[data-test="openai-plan-filter"]')

    expect(planFilter.attributes('disabled')).toBeUndefined()
    expect(planFilter.text()).toContain('admin.accounts.allOpenAIPlans')
    expect(planFilter.text()).toContain('K12')
    expect(planFilter.text()).toContain('Free')
    expect(planFilter.text()).toContain('Plus')
    expect(planFilter.text()).toContain('Pro')
    expect(planFilter.text()).toContain('admin.accounts.planTypeUnset')
  })

  it('clears and disables the plan filter for a non-OpenAI platform', async () => {
    const wrapper = mountFilters({ platform: 'openai', plan_type: 'plus', status: 'active' })
    const selects = wrapper.findAllComponents(SelectStub)

    selects[0].vm.$emit('update:modelValue', 'anthropic')
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('update:filters')?.[0]?.[0]).toEqual({
      platform: 'anthropic',
      plan_type: '',
      status: 'active'
    })

    await wrapper.setProps({ filters: { platform: 'anthropic', plan_type: '' } })
    expect(wrapper.get('[data-test="openai-plan-filter"]').attributes('disabled')).toBeDefined()
  })

  it('emits the selected canonical plan value', () => {
    const wrapper = mountFilters({ platform: '', plan_type: '' })
    const selects = wrapper.findAllComponents(SelectStub)

    selects[1].vm.$emit('update:modelValue', 'k12')

    expect(wrapper.emitted('update:filters')?.[0]?.[0]).toEqual({
      platform: '',
      plan_type: 'k12'
    })
  })
})

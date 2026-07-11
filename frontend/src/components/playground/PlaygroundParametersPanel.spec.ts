import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import PlaygroundParametersPanel from './PlaygroundParametersPanel.vue'
import type { PlaygroundParameterValues } from './playgroundUiTypes'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key
  })
}))

const values: PlaygroundParameterValues = {
  systemPrompt: 'Be concise',
  temperature: 1.2,
  topP: 0.8,
  maxTokens: 2048,
  reasoningEffort: 'high',
  webSearch: true,
  codeExecution: true,
  webFetch: true
}

const option = {
  group_id: 1,
  group_name: 'Default',
  group_priority: 0,
  model: 'test-model',
  platform: 'openai',
  capabilities: ['chat'] as const,
  features: { responses: true, web_search: true, code_execution: false, web_fetch: false, image_input: false }
}

describe('PlaygroundParametersPanel', () => {
  it('disables tools that the selected model does not support', () => {
    const wrapper = mount(PlaygroundParametersPanel, {
      props: { mode: 'chat', option, modelValue: values }
    })

    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    expect(checkboxes).toHaveLength(3)
    expect(checkboxes[0].attributes('disabled')).toBeUndefined()
    expect(checkboxes[1].attributes('disabled')).toBeDefined()
    expect(checkboxes[2].attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('playground.unsupportedByModel')
  })

  it('keeps tools disabled when the backend does not declare capabilities', () => {
    const wrapper = mount(PlaygroundParametersPanel, {
      props: { mode: 'chat', option: { ...option, features: undefined }, modelValue: values }
    })

    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    expect(checkboxes).toHaveLength(3)
    expect(checkboxes.every((checkbox) => checkbox.attributes('disabled') !== undefined)).toBe(true)
  })

  it('shows shared sampling controls without chat-only tool toggles in compare mode', () => {
    const wrapper = mount(PlaygroundParametersPanel, {
      props: { mode: 'compare', option: null, modelValue: values }
    })

    expect(wrapper.findAll('input[type="checkbox"]')).toHaveLength(0)
    expect(wrapper.text()).toContain('playground.compareParametersHint')
    expect(wrapper.text()).toContain('playground.temperature')
    expect(wrapper.text()).toContain('playground.topP')
  })

  it('restores safe defaults without changing key or model selection', async () => {
    const wrapper = mount(PlaygroundParametersPanel, {
      props: { mode: 'compare', option: null, modelValue: values }
    })

    await wrapper.findAll('button').find((button) => button.text() === 'playground.restoreDefaults')!.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toEqual({
      systemPrompt: '',
      temperature: 0.7,
      topP: 1,
      maxTokens: 0,
      reasoningEffort: '',
      webSearch: false,
      codeExecution: false,
      webFetch: false
    })
  })
})

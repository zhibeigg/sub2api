import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, onMounted } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import PlaygroundView from '../PlaygroundView.vue'
import {
  PLAYGROUND_SETTINGS_STORAGE_KEY,
  resetPlaygroundSettingsForTest
} from '@/composables/usePlaygroundSettings'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const chatOption = {
  group_id: 1,
  group_name: 'group',
  group_priority: 0,
  model: 'chat-model',
  platform: 'openai',
  capabilities: ['chat']
}
const imageOption = {
  group_id: 1,
  group_name: 'group',
  group_priority: 0,
  model: 'gpt-image-1',
  platform: 'openai',
  capabilities: ['image']
}

const KeyModelPickerStub = defineComponent({
  props: {
    resolvedKey: { type: String, default: '' },
    capability: { type: String, default: '' }
  },
  emits: ['update:resolvedKey'],
  setup(props, { emit }) {
    onMounted(() => emit('update:resolvedKey', 'secret-key'))
    return () => h('div', { 'data-testid': 'key-picker', 'data-capability': props.capability })
  }
})

const ImagePanelStub = defineComponent({
  props: {
    resolvedKey: { type: String, default: '' }
  },
  template: '<div data-testid="image-panel" :data-key="resolvedKey" />'
})

describe('PlaygroundView mode switching', () => {
  beforeEach(() => {
    localStorage.clear()
    resetPlaygroundSettingsForTest()
    localStorage.setItem(PLAYGROUND_SETTINGS_STORAGE_KEY, JSON.stringify({
      version: 3,
      keyId: 1,
      selections: { chat: chatOption, image: imageOption, video: null },
      systemPrompt: '',
      temperature: 0.7,
      maxTokens: 0,
      topP: 1,
      reasoningEffort: '',
      webSearch: false,
      codeExecution: false,
      webFetch: false
    }))
  })

  it('keeps the resolved key effective when switching from chat to image', async () => {
    const wrapper = mount(PlaygroundView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
          KeyModelPicker: KeyModelPickerStub,
          ChatPanel: true,
          ImagePanel: ImagePanelStub,
          VideoPanel: true,
          ComparePanel: true,
          PlaygroundParametersPanel: true
        }
      }
    })
    await flushPromises()
    expect(wrapper.get('[data-testid="key-picker"]').attributes('data-capability')).toBe('chat')

    await wrapper.findAll('[role="tab"]')[1].trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="image-panel"]').attributes('data-key')).toBe('secret-key')
  })
})

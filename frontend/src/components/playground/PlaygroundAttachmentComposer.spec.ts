import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import PlaygroundAttachmentComposer from './PlaygroundAttachmentComposer.vue'
import type { PlaygroundAttachment } from '@/types/playground'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key
  })
}))

const readyAttachment: PlaygroundAttachment = {
  id: 'attachment-1',
  name: 'notes.txt',
  type: 'text/plain',
  size: 12,
  status: 'ready',
  text: 'hello'
}

describe('PlaygroundAttachmentComposer', () => {
  it('exposes an accessible 44px attachment trigger and remove control', async () => {
    const wrapper = mount(PlaygroundAttachmentComposer, {
      props: { modelValue: [readyAttachment] },
      global: { stubs: { Icon: true } }
    })

    const trigger = wrapper.find('button[aria-label="playground.addAttachments"]')
    expect(trigger.classes()).toContain('h-11')
    const remove = wrapper.find('button[aria-label*="playground.removeAttachment"]')
    expect(remove.classes()).toContain('h-11')
    await remove.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toEqual([])
  })

  it('keeps text attachments available while rejecting images for non-vision models', async () => {
    const wrapper = mount(PlaygroundAttachmentComposer, {
      props: { modelValue: [], allowImages: false },
      global: { stubs: { Icon: true } }
    })
    const input = wrapper.find('input[type="file"]')
    expect(input.attributes('accept')).not.toContain('image/*')
    const file = new File(['image'], 'screen.png', { type: 'image/png' })
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.text()).toContain('playground.imageAttachmentUnsupported')
  })

  it('marks oversized files as errors instead of reading them', async () => {
    const wrapper = mount(PlaygroundAttachmentComposer, {
      props: { modelValue: [], maxBytes: 4 },
      global: { stubs: { Icon: true } }
    })
    const input = wrapper.find('input[type="file"]')
    const file = new File(['too large'], 'large.txt', { type: 'text/plain' })
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')

    const emitted = wrapper.emitted('update:modelValue')?.[0]?.[0] as PlaygroundAttachment[]
    expect(emitted).toHaveLength(1)
    expect(emitted[0].status).toBe('error')
    expect(emitted[0].error).toContain('playground.attachmentTooLarge')
  })
})

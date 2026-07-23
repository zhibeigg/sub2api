import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ImagePanel from './ImagePanel.vue'
import type { PlaygroundModelOption } from '@/types/playground'

const generateImage = vi.hoisted(() => vi.fn())

vi.mock('@/api/playground', () => ({
  default: { generateImage: (...args: unknown[]) => generateImage(...args) },
  imageQualityOptions: (model: string) => {
    if (model.includes('gpt-image')) return ['auto', 'low', 'medium', 'high']
    if (model.includes('dall-e')) return ['standard', 'hd']
    return []
  }
}))
vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key
    })
  }
})

const imageOption: PlaygroundModelOption = {
  group_id: 7,
  group_name: 'images',
  group_priority: 0,
  model: 'gpt-image-1',
  platform: 'openai',
  capabilities: ['image']
}

let objectUrlId = 0
let createObjectURL: ReturnType<typeof vi.fn>
let revokeObjectURL: ReturnType<typeof vi.fn>

function mountPanel(props: Partial<{
  keyId: number | null
  resolvedKey: string
  option: PlaygroundModelOption | null
}> = {}): VueWrapper {
  return mount(ImagePanel, {
    props: {
      keyId: 1,
      resolvedKey: 'secret-key',
      option: imageOption,
      ...props
    },
    global: {
      stubs: {
        Icon: true,
        KeyModelPicker: {
          props: ['keyId', 'resolvedKey', 'option', 'capability', 'layout'],
          template: '<div data-testid="key-model-picker" />'
        },
        Teleport: true
      }
    }
  })
}

async function setPrompt(wrapper: VueWrapper, value = 'A sharp editorial image'): Promise<void> {
  await wrapper.get('#image-prompt').setValue(value)
}

function referenceFile(name: string, type = 'image/png'): File {
  return new File([name], name, { type })
}

async function dropFiles(wrapper: VueWrapper, files: File[]): Promise<void> {
  await wrapper.get('[data-testid="reference-dropzone"]').trigger('drop', {
    dataTransfer: { files }
  })
}

describe('ImagePanel workbench', () => {
  beforeEach(() => {
    generateImage.mockReset()
    objectUrlId = 0
    createObjectURL = vi.fn(() => `blob:image-${++objectUrlId}`)
    revokeObjectURL = vi.fn()
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL })
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeObjectURL })
  })

  it('reports the exact disabled reason as key, model, and prompt state changes', async () => {
    const wrapper = mountPanel({ keyId: null, resolvedKey: '', option: null })
    const button = wrapper.get('[data-testid="image-generate"]')
    const reason = () => wrapper.get('[data-testid="image-disabled-reason"]').text()

    expect(button.attributes()).toHaveProperty('disabled')
    expect(reason()).toBe('playground.imageDisabledNoKey')

    await wrapper.setProps({ keyId: 1 })
    expect(reason()).toBe('playground.imageDisabledKeyUnavailable')

    await wrapper.setProps({ resolvedKey: 'secret-key' })
    expect(reason()).toBe('playground.imageDisabledNoModel')

    await wrapper.setProps({ option: imageOption })
    expect(reason()).toBe('playground.imageDisabledNoPrompt')

    await setPrompt(wrapper)
    expect(button.attributes()).not.toHaveProperty('disabled')
  })

  it('accepts click, drop, and paste while enforcing limits and revoking replaced resources', async () => {
    const wrapper = mountPanel()
    const input = wrapper.get('[data-testid="reference-input"]')
    const inputClick = vi.spyOn(input.element as HTMLInputElement, 'click')
    await wrapper.get('[data-testid="reference-dropzone"]').trigger('click')
    expect(inputClick).toHaveBeenCalledOnce()

    const first = referenceFile('first.png')
    const pasted = referenceFile('pasted.webp', 'image/webp')
    await dropFiles(wrapper, [first])
    await wrapper.get('[data-testid="image-workbench"]').trigger('paste', {
      clipboardData: {
        items: [{ kind: 'file', getAsFile: () => pasted }]
      }
    })
    expect(wrapper.findAll('[data-testid="reference-list"] > div')).toHaveLength(2)

    const unsupported = referenceFile('bad.gif', 'image/gif')
    await dropFiles(wrapper, [unsupported])
    expect(wrapper.text()).toContain('playground.imageReferenceType')

    const tooLarge = referenceFile('large.png')
    Object.defineProperty(tooLarge, 'size', { value: 20 * 1024 * 1024 + 1 })
    await dropFiles(wrapper, [tooLarge])
    expect(wrapper.text()).toContain('playground.imageReferenceTooLarge')

    await dropFiles(wrapper, [referenceFile('third.jpg', 'image/jpeg'), referenceFile('fourth.png')])
    await dropFiles(wrapper, [referenceFile('fifth.png')])
    expect(wrapper.findAll('[data-testid="reference-list"] > div')).toHaveLength(4)
    expect(wrapper.text()).toContain('playground.imageReferenceLimit')

    const replacement = referenceFile('replacement.png')
    await wrapper.findAll('[data-testid="reference-list"] > div')[0].get('button').trigger('click')
    Object.defineProperty(input.element, 'files', { configurable: true, value: [replacement] })
    await input.trigger('change')
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:image-1')

    const secondPreview = wrapper.findAll('[data-testid="reference-list"] img')[1]
    await secondPreview.trigger('error')
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:image-2')
    expect(wrapper.text()).toContain('playground.imageReferenceLoadFailed')

    const replacementUrl = createObjectURL.mock.results.at(-1)?.value
    await wrapper.findAll('[data-testid="reference-list"] > div')[0].findAll('button')[1].trigger('click')
    expect(revokeObjectURL).toHaveBeenCalledWith(replacementUrl)

    wrapper.unmount()
    expect(revokeObjectURL.mock.calls.length).toBeGreaterThanOrEqual(5)
  })

  it('shows real generation stages, switches multiple results, and keeps only 12 session batches', async () => {
    let resolveFirst!: (value: Array<{ b64_json: string }>) => void
    generateImage
      .mockReturnValueOnce(new Promise((resolve) => { resolveFirst = resolve }))
      .mockResolvedValue([{ b64_json: 'aW1hZ2U=' }])
    const wrapper = mountPanel()
    await setPrompt(wrapper, 'first batch')

    await wrapper.get('[data-testid="image-generate"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-testid="image-generating-state"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('playground.imageStageRequesting')
    resolveFirst([{ b64_json: 'b25l' }, { b64_json: 'dHdv' }])
    await flushPromises()

    expect(wrapper.find('[data-testid="image-complete-state"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="result-switcher"] button')).toHaveLength(2)
    const firstCanvasUrl = wrapper.get('[data-testid="canvas-image"]').attributes('src')
    await wrapper.findAll('[data-testid="result-switcher"] button')[1].trigger('click')
    expect(wrapper.get('[data-testid="canvas-image"]').attributes('src')).not.toBe(firstCanvasUrl)

    for (let index = 2; index <= 13; index += 1) {
      await setPrompt(wrapper, `batch ${index}`)
      await wrapper.get('[data-testid="image-generate"]').trigger('click')
      await flushPromises()
    }

    expect(wrapper.findAll('[data-testid="image-history"] > div')).toHaveLength(12)
    expect(revokeObjectURL).toHaveBeenCalled()

    await wrapper.get('[data-testid="image-history"]').findAll('button')[0].trigger('click')
    expect(wrapper.find('[data-testid="image-complete-state"]').exists()).toBe(true)

    const callsBeforeClear = revokeObjectURL.mock.calls.length
    const clearButton = wrapper.findAll('button').find((button) => button.text() === 'playground.imageHistoryClear')
    expect(clearButton).toBeTruthy()
    await clearButton?.trigger('click')
    expect(wrapper.find('[data-testid="image-history"]').exists()).toBe(false)
    expect(revokeObjectURL.mock.calls.length).toBeGreaterThan(callsBeforeClear)
  })

  it('keeps reference files after an error and retries them in a new history batch', async () => {
    generateImage
      .mockRejectedValueOnce(new Error('provider failed'))
      .mockResolvedValueOnce([{ b64_json: 'cmV0cnk=' }])
    const wrapper = mountPanel()
    const reference = referenceFile('reference.png')
    await dropFiles(wrapper, [reference])
    await setPrompt(wrapper, 'retry this edit')

    await wrapper.get('[data-testid="image-generate"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="image-error-state"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('provider failed')
    expect(wrapper.findAll('[data-testid="reference-list"] > div')).toHaveLength(1)

    const retryButton = wrapper.get('[data-testid="image-retry"]')
    expect(retryButton.attributes()).not.toHaveProperty('disabled')
    await retryButton.trigger('click')
    await vi.waitFor(() => expect(generateImage).toHaveBeenCalledTimes(2))
    await flushPromises()

    expect(generateImage.mock.calls[0][0]).toEqual(expect.objectContaining({
      apiKey: 'secret-key',
      model: 'gpt-image-1',
      images: [reference]
    }))
    expect(generateImage.mock.calls[1][0]).toEqual(expect.objectContaining({ images: [reference] }))
    expect(wrapper.find('[data-testid="image-complete-state"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="image-history"] > div')).toHaveLength(2)
    expect(wrapper.findAll('[data-testid="reference-list"] > div')).toHaveLength(1)
  })
})

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import KeyModelPicker from './KeyModelPicker.vue'

const listKeys = vi.fn()
const listModelOptions = vi.fn()

vi.mock('@/api/keys', () => ({
  default: { list: (...args: unknown[]) => listKeys(...args) }
}))
vi.mock('@/api/playground', () => ({
  default: { listModelOptions: (...args: unknown[]) => listModelOptions(...args) }
}))
vi.mock('@/utils/platformColors', () => ({ platformLabel: (platform: string) => platform }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}

const firstOption = {
  group_id: 1, group_name: 'first', group_priority: 0, model: 'first-model', platform: 'openai', capabilities: ['chat'] as const
}
const secondOption = {
  group_id: 2, group_name: 'second', group_priority: 0, model: 'second-model', platform: 'anthropic', capabilities: ['chat'] as const
}
const imageOption = {
  group_id: 1, group_name: 'first', group_priority: 0, model: 'gpt-image-1', platform: 'openai', capabilities: ['image'] as const
}

describe('KeyModelPicker model loading race', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listKeys.mockResolvedValue({ items: [
      { id: 1, name: 'one', key: 'secret-one' },
      { id: 2, name: 'two', key: 'secret-two' }
    ] })
  })

  it('re-synchronizes the resolved key when capability changes', async () => {
    listModelOptions.mockResolvedValue([firstOption, imageOption])
    const wrapper = mount(KeyModelPicker, {
      props: { keyId: 1, resolvedKey: '', option: firstOption as any, capability: 'chat' },
      global: {
        stubs: { Icon: true },
        mocks: { $t: (key: string) => key }
      }
    })
    await flushPromises()

    await wrapper.setProps({ capability: 'image' })
    await flushPromises()

    expect(wrapper.emitted('update:resolvedKey')?.at(-1)?.[0]).toBe('secret-one')
    expect(wrapper.emitted('resolved-key')?.at(-1)?.[0]).toBe('secret-one')
    expect(wrapper.emitted('update:option')?.at(-1)?.[0]).toEqual(imageOption)
  })

  it('ignores a stale response after the selected key changes', async () => {
    const first = deferred<any[]>()
    const second = deferred<any[]>()
    listModelOptions.mockImplementation((keyId: number) => keyId === 1 ? first.promise : second.promise)

    const wrapper = mount(KeyModelPicker, {
      props: { keyId: 1, option: null, capability: 'chat' },
      global: {
        stubs: { Icon: true },
        mocks: { $t: (key: string) => key }
      }
    })
    await flushPromises()

    await wrapper.setProps({ keyId: 2 })
    await flushPromises()
    expect(wrapper.emitted('update:option')?.at(-1)?.[0]).toBeNull()

    second.resolve([secondOption])
    await flushPromises()
    first.resolve([firstOption])
    await flushPromises()

    const updates = wrapper.emitted('update:option') ?? []
    expect(updates.at(-1)?.[0]).toEqual(secondOption)
    expect(updates.some((event) => (event[0] as any)?.model === 'first-model')).toBe(false)
    expect(listModelOptions.mock.calls.map((call) => call[0])).toEqual([1, 2])
  })
})

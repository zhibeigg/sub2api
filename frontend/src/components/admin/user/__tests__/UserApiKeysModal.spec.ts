import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import UserApiKeysModal from '../UserApiKeysModal.vue'

const { getUserApiKeys, getAllGroups, updateApiKeyGroupBindings, showSuccess, showError } = vi.hoisted(() => ({
  getUserApiKeys: vi.fn(),
  getAllGroups: vi.fn(),
  updateApiKeyGroupBindings: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: { getUserApiKeys },
    groups: { getAll: getAllGroups },
    apiKeys: { updateApiKeyGroupBindings }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const groups = [
  { id: 1, name: 'Claude', platform: 'anthropic', rate_multiplier: 1 },
  { id: 2, name: 'Codex', platform: 'openai', rate_multiplier: 0.8 },
  { id: 3, name: 'Gemini', platform: 'gemini', rate_multiplier: 1.2 }
] as any[]

const apiKey = {
  id: 7,
  user_id: 9,
  key: 'sk-admin-test-key-with-enough-length',
  name: 'Admin managed key',
  group_id: 1,
  group: groups[0],
  group_bindings: groups.map((group, priority) => ({ group_id: group.id, priority, group })),
  status: 'active',
  created_at: '2026-06-27T00:00:00Z'
} as any

const BaseDialogStub = {
  props: ['show'],
  emits: ['close'],
  template: '<div v-if="show"><slot /></div>'
}

const SortableGroupBindingPickerStub = {
  name: 'SortableGroupBindingPicker',
  props: ['modelValue', 'groups'],
  emits: ['update:modelValue'],
  methods: { focusSearch: vi.fn() },
  template: '<div data-test="admin-picker">{{ JSON.stringify(modelValue) }}</div>'
}

const mountModal = () => mount(UserApiKeysModal, {
  attachTo: document.body,
  props: {
    show: false,
    user: { id: 9, email: 'user@example.com', username: 'user' } as any
  },
  global: {
    stubs: {
      BaseDialog: BaseDialogStub,
      Teleport: true,
      SortableGroupBindingPicker: SortableGroupBindingPickerStub,
      PlatformIcon: { props: ['platform'], template: '<span>{{ platform }}</span>' },
      Icon: { props: ['name'], template: '<span>{{ name }}</span>' }
    }
  }
})

describe('UserApiKeysModal group bindings', () => {
  beforeEach(() => {
    getUserApiKeys.mockReset()
    getAllGroups.mockReset()
    updateApiKeyGroupBindings.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    getUserApiKeys.mockResolvedValue({ items: [apiKey] })
    getAllGroups.mockResolvedValue(groups)
    updateApiKeyGroupBindings.mockResolvedValue({
      api_key: apiKey,
      auto_granted_group_access: false
    })
  })

  it('renders ordered bindings with overflow and calls the extended bindings request', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    const trigger = wrapper.get('.group-binding-trigger')
    expect(trigger.text()).toContain('Claude')
    expect(trigger.text()).toContain('Codex')
    expect(trigger.text()).toContain('+1')
    expect(wrapper.get('[data-test="admin-api-key-group-1"]').classes()).toContain('text-orange-600')
    expect(wrapper.get('[data-test="admin-api-key-group-2"]').classes()).toContain('text-green-600')

    await trigger.trigger('click')
    await nextTick()
    wrapper.findComponent({ name: 'SortableGroupBindingPicker' }).vm.$emit('update:modelValue', [
      { group_id: 3, priority: 0 },
      { group_id: 2, priority: 1 }
    ])
    await nextTick()
    await wrapper.get('[data-test="admin-save-group-bindings"]').trigger('click')
    await flushPromises()

    expect(updateApiKeyGroupBindings).toHaveBeenCalledWith(7, [
      { group_id: 3, priority: 0 },
      { group_id: 2, priority: 1 }
    ])
    expect(updateApiKeyGroupBindings.mock.calls[0][1]).not.toHaveProperty('group_id')
    wrapper.unmount()
  })

  it('fits the fixed popover on narrow screens and closes it with Escape', async () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 320 })
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    const trigger = wrapper.get('.group-binding-trigger')
    await trigger.trigger('click')
    await nextTick()

    expect(wrapper.get('[data-test="admin-group-binding-popover"]').attributes('style')).toContain('width: 296px')

    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(wrapper.find('[data-test="admin-group-binding-popover"]').exists()).toBe(false)

    await trigger.trigger('click')
    await nextTick()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()

    expect(wrapper.find('[data-test="admin-group-binding-popover"]').exists()).toBe(false)
    expect(document.activeElement).toBe(trigger.element)
    wrapper.unmount()
  })
})

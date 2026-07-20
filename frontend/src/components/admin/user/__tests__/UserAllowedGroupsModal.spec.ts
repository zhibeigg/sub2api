import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'

import type { AdminUser, Group, UserGroupConfig } from '@/types'
import UserAllowedGroupsModal from '../UserAllowedGroupsModal.vue'

const {
  listGroups,
  getGroupConfig,
  updateGroupConfig,
  showError,
  showSuccess,
} = vi.hoisted(() => ({
  listGroups: vi.fn(),
  getGroupConfig: vi.fn(),
  updateGroupConfig: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: { list: listGroups },
    users: { getGroupConfig, updateGroupConfig },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => params
        ? `${key}:${JSON.stringify(params)}`
        : key,
    }),
  }
})

vi.mock('@/components/common/BaseDialog.vue', () => ({
  default: {
    name: 'BaseDialog',
    props: ['show', 'title', 'width'],
    emits: ['close'],
    template: '<div v-if="show"><slot /><slot name="footer" /></div>',
  },
}))

vi.mock('@/components/common/ConfirmDialog.vue', () => ({
  default: {
    name: 'ConfirmDialog',
    props: ['show', 'title', 'message', 'confirmText'],
    emits: ['confirm', 'cancel'],
    template: `
      <div v-if="show" data-test="save-confirm">
        <button data-test="confirm-save" @click="$emit('confirm')">confirm</button>
        <button data-test="cancel-save" @click="$emit('cancel')">cancel</button>
      </div>
    `,
  },
}))

vi.mock('@/components/common/PlatformIcon.vue', () => ({
  default: {
    name: 'PlatformIcon',
    props: ['platform'],
    template: '<span>{{ platform }}</span>',
  },
}))

const user = {
  id: 19,
  email: 'groups@example.com',
  username: 'groups',
} as AdminUser

const groups = [
  {
    id: 1,
    name: 'Alpha Public',
    platform: 'anthropic',
    rate_multiplier: 1,
    is_exclusive: false,
    status: 'active',
    subscription_type: 'standard',
  },
  {
    id: 2,
    name: 'Beta Public',
    platform: 'openai',
    rate_multiplier: 1.2,
    is_exclusive: false,
    status: 'active',
    subscription_type: 'standard',
  },
  {
    id: 3,
    name: 'VIP Exclusive',
    platform: 'gemini',
    rate_multiplier: 2,
    is_exclusive: true,
    status: 'active',
    subscription_type: 'standard',
  },
  {
    id: 4,
    name: 'Inactive Public',
    platform: 'openai',
    rate_multiplier: 1,
    is_exclusive: false,
    status: 'inactive',
    subscription_type: 'standard',
  },
  {
    id: 5,
    name: 'Subscription Group',
    platform: 'openai',
    rate_multiplier: 1,
    is_exclusive: false,
    status: 'active',
    subscription_type: 'subscription',
  },
] as Group[]

const inheritConfig: UserGroupConfig = {
  access_mode: 'inherit',
  restricted_group_ids: [],
  exclusive_group_ids: [3],
  group_rates: { 1: 1.25, 3: 2.5 },
}

async function mountAndOpen(config: UserGroupConfig = inheritConfig): Promise<VueWrapper> {
  getGroupConfig.mockResolvedValue(config)
  const wrapper = mount(UserAllowedGroupsModal, {
    props: { show: false, user },
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

async function save(wrapper: VueWrapper) {
  await wrapper.get('[data-test="save-group-config"]').trigger('click')
  expect(wrapper.find('[data-test="save-confirm"]').exists()).toBe(true)
  await wrapper.get('[data-test="confirm-save"]').trigger('click')
  await flushPromises()
}

beforeEach(() => {
  listGroups.mockReset()
  getGroupConfig.mockReset()
  updateGroupConfig.mockReset()
  showError.mockReset()
  showSuccess.mockReset()

  listGroups.mockResolvedValue({ items: groups })
  updateGroupConfig.mockResolvedValue(inheritConfig)
})

describe('UserAllowedGroupsModal', () => {
  it('loads inherit mode from the dedicated endpoint on every open and only renders active standard groups', async () => {
    const wrapper = await mountAndOpen()

    expect(listGroups).toHaveBeenCalledWith(1, 1000, { status: 'active' })
    expect(getGroupConfig).toHaveBeenCalledWith(19)
    expect((wrapper.get('[data-test="mode-inherit"]').element as HTMLInputElement).checked).toBe(true)
    expect((wrapper.get('[data-test="group-checkbox-1"]').element as HTMLInputElement).checked).toBe(true)
    expect((wrapper.get('[data-test="group-checkbox-2"]').element as HTMLInputElement).checked).toBe(true)
    expect((wrapper.get('[data-test="group-checkbox-3"]').element as HTMLInputElement).checked).toBe(true)
    expect(wrapper.find('[data-test="group-checkbox-4"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="group-checkbox-5"]').exists()).toBe(false)

    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(getGroupConfig).toHaveBeenCalledTimes(2)
  })

  it('keeps the inherited allowlist read-only and materializes all standard groups when restricted mode is selected', async () => {
    const wrapper = await mountAndOpen()

    expect((wrapper.get('[data-test="group-checkbox-1"]').element as HTMLInputElement).disabled).toBe(true)
    await wrapper.get('[data-test="mode-restricted"]').setValue(true)
    await wrapper.get('[data-test="group-checkbox-1"]').setValue(false)

    expect((wrapper.get('[data-test="mode-restricted"]').element as HTMLInputElement).checked).toBe(true)
    expect((wrapper.get('[data-test="group-checkbox-2"]').element as HTMLInputElement).checked).toBe(true)
    expect((wrapper.get('[data-test="group-checkbox-3"]').element as HTMLInputElement).checked).toBe(true)

    await save(wrapper)
    expect(updateGroupConfig).toHaveBeenCalledWith(19, expect.objectContaining({
      access_mode: 'restricted',
      restricted_group_ids: [2, 3],
      exclusive_group_ids: [3],
    }))
  })

  it('warns for an empty restricted whitelist and submits empty permission arrays', async () => {
    const wrapper = await mountAndOpen({
      access_mode: 'restricted',
      restricted_group_ids: [1],
      exclusive_group_ids: [],
      group_rates: {},
    })

    await wrapper.get('[data-test="group-checkbox-1"]').setValue(false)
    expect(wrapper.find('[data-test="empty-whitelist-warning"]').exists()).toBe(true)

    await save(wrapper)
    expect(updateGroupConfig).toHaveBeenCalledWith(19, {
      access_mode: 'restricted',
      restricted_group_ids: [],
      exclusive_group_ids: [],
      group_rates: {},
    })
  })

  it('allows all public groups and removes the restriction without changing exclusive choices', async () => {
    const wrapper = await mountAndOpen({
      access_mode: 'restricted',
      restricted_group_ids: [1],
      exclusive_group_ids: [3],
      group_rates: {},
    })

    await wrapper.get('[data-test="allow-all-inherit"]').trigger('click')

    expect((wrapper.get('[data-test="mode-inherit"]').element as HTMLInputElement).checked).toBe(true)
    expect((wrapper.get('[data-test="group-checkbox-1"]').element as HTMLInputElement).checked).toBe(true)
    expect((wrapper.get('[data-test="group-checkbox-2"]').element as HTMLInputElement).checked).toBe(true)
    expect((wrapper.get('[data-test="group-checkbox-3"]').element as HTMLInputElement).checked).toBe(true)

    await save(wrapper)
    expect(updateGroupConfig).toHaveBeenCalledWith(19, expect.objectContaining({
      access_mode: 'inherit',
      restricted_group_ids: [],
      exclusive_group_ids: [3],
    }))
  })

  it('selects only current search results and keeps restricted mode after every current group is selected', async () => {
    const wrapper = await mountAndOpen({
      access_mode: 'restricted',
      restricted_group_ids: [],
      exclusive_group_ids: [],
      group_rates: {},
    })

    await wrapper.get('[data-test="group-search"]').setValue('Beta')
    await wrapper.get('[data-test="select-results"]').trigger('click')
    expect((wrapper.get('[data-test="group-checkbox-2"]').element as HTMLInputElement).checked).toBe(true)

    await wrapper.get('[data-test="group-search"]').setValue('')
    expect((wrapper.get('[data-test="group-checkbox-1"]').element as HTMLInputElement).checked).toBe(false)
    expect((wrapper.get('[data-test="group-checkbox-3"]').element as HTMLInputElement).checked).toBe(false)

    await wrapper.get('[data-test="select-results"]').trigger('click')
    expect((wrapper.get('[data-test="mode-restricted"]').element as HTMLInputElement).checked).toBe(true)
  })

  it('preserves a custom rate when access is disabled and submits the retained rate independently', async () => {
    const wrapper = await mountAndOpen()

    await wrapper.get('[data-test="mode-restricted"]').setValue(true)
    await wrapper.get('[data-test="group-checkbox-1"]').setValue(false)

    expect((wrapper.get('[data-test="rate-1"]').element as HTMLInputElement).value).toBe('1.25')
    expect((wrapper.get('[data-test="rate-1"]').element as HTMLInputElement).disabled).toBe(true)
    expect(wrapper.find('[data-test="preserved-rate-1"]').exists()).toBe(true)

    await save(wrapper)
    expect(updateGroupConfig).toHaveBeenCalledWith(19, expect.objectContaining({
      restricted_group_ids: [2, 3],
      group_rates: { 1: 1.25, 3: 2.5 },
    }))
  })

  it('submits null when an existing custom rate is cleared', async () => {
    const wrapper = await mountAndOpen()

    await wrapper.get('[data-test="rate-1"]').setValue('')
    await save(wrapper)

    expect(updateGroupConfig).toHaveBeenCalledWith(19, expect.objectContaining({
      group_rates: { 1: null, 3: 2.5 },
    }))
  })

  it('shows a load error toast and retries both requests successfully', async () => {
    getGroupConfig.mockRejectedValueOnce(new Error('network unavailable'))
    getGroupConfig.mockResolvedValueOnce(inheritConfig)

    const wrapper = mount(UserAllowedGroupsModal, {
      props: { show: false, user },
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('network unavailable')
    expect(wrapper.find('[data-test="retry-load"]').exists()).toBe(true)

    await wrapper.get('[data-test="retry-load"]').trigger('click')
    await flushPromises()

    expect(getGroupConfig).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-test="group-checkbox-1"]').exists()).toBe(true)
  })

  it('keeps the modal open and shows a toast when saving fails', async () => {
    updateGroupConfig.mockRejectedValueOnce({ response: { data: { detail: 'save rejected' } } })
    const wrapper = await mountAndOpen()

    await save(wrapper)

    expect(showError).toHaveBeenCalledWith('save rejected')
    expect(wrapper.emitted('success')).toBeUndefined()
    expect(wrapper.find('[data-test="save-group-config"]').exists()).toBe(true)
  })
})

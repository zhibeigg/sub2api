import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AnnouncementsView from '../AnnouncementsView.vue'

const { list, create, getEmailCapability, getEmailStatus, retryEmailNotification, getAll } = vi.hoisted(() => ({
  list: vi.fn(),
  create: vi.fn(),
  getEmailCapability: vi.fn(),
  getEmailStatus: vi.fn(),
  retryEmailNotification: vi.fn(),
  getAll: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    announcements: {
      list,
      create,
      update: vi.fn(),
      delete: vi.fn(),
      getEmailCapability,
      getEmailStatus,
      retryEmailNotification
    },
    groups: { getAll }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const pendingNotification = {
  status: 'pending' as const,
  total_count: 5,
  sent_count: 0,
  failed_count: 0,
  ambiguous_count: 0,
  skipped_count: 0,
  can_retry: false
}

const activeAnnouncement = {
  id: 7,
  title: 'Maintenance',
  content: 'Window',
  status: 'active' as const,
  notify_mode: 'silent' as const,
  targeting: { any_of: [] },
  email_notification: pendingNotification,
  created_at: '2026-05-01T00:00:00Z',
  updated_at: '2026-05-01T00:00:00Z'
}

const stubs = {
  AppLayout: { template: '<div><slot /></div>' },
  TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
  DataTable: { props: ['data'], template: '<div><div v-for="row in data" :key="row.id"><slot name="cell-email_notification" :row="row" /></div></div>' },
  BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
  ConfirmDialog: { props: ['show'], emits: ['confirm', 'cancel'], template: '<div v-if="show" data-test="confirm-dialog"><button data-test="confirm" @click="$emit(\'confirm\')">confirm</button><button data-test="cancel" @click="$emit(\'cancel\')">cancel</button></div>' },
  Select: { props: ['modelValue', 'options'], emits: ['update:modelValue', 'change'], template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value); $emit(\'change\')"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>' },
  Pagination: true,
  EmptyState: true,
  Icon: true,
  AnnouncementTargetingEditor: true,
  AnnouncementReadStatusDialog: true,
  AnnouncementEmailStatusDialog: true,
  Teleport: true
}

describe('AnnouncementsView email delivery flow', () => {
  beforeEach(() => {
    vi.useRealTimers()
    list.mockReset().mockResolvedValue({ items: [activeAnnouncement], total: 1, page: 1, page_size: 20, pages: 1 })
    create.mockReset().mockResolvedValue(activeAnnouncement)
    getEmailCapability.mockReset().mockResolvedValue({ enabled: true, smtp_configured: true, eligible_count: 5 })
    getEmailStatus.mockReset().mockResolvedValue({ ...pendingNotification, status: 'completed' })
    retryEmailNotification.mockReset()
    getAll.mockReset().mockResolvedValue([])
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('requires explicit confirmation before creating an active announcement with email', async () => {
    const wrapper = mount(AnnouncementsView, { global: { stubs } })
    await flushPromises()

    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    const form = wrapper.get('#announcement-form')
    const statusSelect = form.findAll('select')[0]
    await statusSelect.setValue('active')
    await form.get('input[type="checkbox"]').setValue(true)
    await form.trigger('submit')
    await flushPromises()

    expect(create).not.toHaveBeenCalled()
    expect(wrapper.find('[data-test="confirm-dialog"]').exists()).toBe(true)

    await wrapper.get('[data-test="confirm"]').trigger('click')
    await flushPromises()

    expect(create).toHaveBeenCalledWith(
      expect.objectContaining({ status: 'active', send_email: true }),
      expect.any(String)
    )
  })

  it('polls running jobs after five seconds and stops after unmount', async () => {
    vi.useFakeTimers()
    const wrapper = mount(AnnouncementsView, { global: { stubs } })
    await flushPromises()

    expect(getEmailStatus).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(5000)
    await flushPromises()
    expect(getEmailStatus).toHaveBeenCalledTimes(1)

    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(10000)
    expect(getEmailStatus).toHaveBeenCalledTimes(1)
  })
})

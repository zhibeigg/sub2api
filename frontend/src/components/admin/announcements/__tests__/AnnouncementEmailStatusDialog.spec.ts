import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AnnouncementEmailStatusDialog from '../AnnouncementEmailStatusDialog.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const notification = {
  status: 'completed_with_failures' as const,
  total_count: 20,
  sent_count: 14,
  failed_count: 3,
  ambiguous_count: 2,
  skipped_count: 1,
  available_at: '2026-05-01T00:00:00Z',
  started_at: '2026-05-01T00:01:00Z',
  finished_at: '2026-05-01T00:02:00Z',
  last_error_code: 'SMTP_TIMEOUT',
  last_error_message: 'Temporary delivery timeout',
  can_retry: true
}

const BaseDialogStub = {
  props: ['show', 'title'],
  emits: ['close'],
  template: '<div v-if="show"><h2>{{ title }}</h2><slot /><slot name="footer" /></div>'
}

describe('AnnouncementEmailStatusDialog', () => {
  it('shows terminal status and tabular delivery counts without recipient addresses', () => {
    const wrapper = mount(AnnouncementEmailStatusDialog, {
      props: { show: true, notification },
      global: { stubs: { BaseDialog: BaseDialogStub, Icon: true } }
    })

    expect(wrapper.text()).toContain('admin.announcements.email.status.completed_with_failures')
    expect(wrapper.text()).toContain('20')
    expect(wrapper.text()).toContain('14')
    expect(wrapper.text()).toContain('SMTP_TIMEOUT')
    expect(wrapper.text()).not.toContain('@')
    expect(wrapper.find('.tabular-nums').exists()).toBe(true)
  })

  it('emits separate retry intents for failed-only and ambiguous-inclusive actions', async () => {
    const wrapper = mount(AnnouncementEmailStatusDialog, {
      props: { show: true, notification },
      global: { stubs: { BaseDialog: BaseDialogStub, Icon: true } }
    })
    const buttons = wrapper.findAll('button')
    const failedOnly = buttons.find((button) => button.text().includes('retryFailed'))
    const includingAmbiguous = buttons.find((button) => button.text().includes('retryIncludingAmbiguous'))

    await failedOnly?.trigger('click')
    await includingAmbiguous?.trigger('click')

    expect(wrapper.emitted('retry')).toEqual([[false], [true]])
  })
})

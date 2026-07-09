import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsRequestDetailsModal from '../OpsRequestDetailsModal.vue'

const mockListRequestDetails = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    listRequestDetails: (...args: any[]) => mockListRequestDetails(...args)
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showWarning: vi.fn()
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: vi.fn().mockResolvedValue(true)
  })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: { type: Boolean, default: false },
    title: { type: String, default: '' }
  },
  template: '<div v-if="show" class="dialog-stub"><slot /></div>'
})

const PaginationStub = defineComponent({
  name: 'Pagination',
  template: '<div class="pagination-stub" />'
})

describe('OpsRequestDetailsModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockListRequestDetails.mockResolvedValue({
      items: [
        {
          kind: 'success',
          created_at: '2026-07-10T03:35:39Z',
          request_id: 'client:req-1',
          platform: 'kiro',
          model: 'claude-opus-4-8',
          first_token_ms: 32000,
          duration_ms: 45000,
          stream: true
        }
      ],
      total: 1,
      page: 1,
      page_size: 10
    })
  })

  it('按TTFT排序请求成功流式明细并同时展示首Token与总耗时', async () => {
    const wrapper = mount(OpsRequestDetailsModal, {
      props: {
        modelValue: false,
        timeRange: '1h',
        preset: {
          title: 'TTFT',
          kind: 'success',
          sort: 'ttft_desc'
        },
        platform: 'kiro',
        groupId: 7
      },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Pagination: PaginationStub
        }
      }
    })

    await wrapper.setProps({ modelValue: true })
    await flushPromises()

    expect(mockListRequestDetails).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: 'success',
        sort: 'ttft_desc',
        platform: 'kiro',
        group_id: 7,
        page: 1,
        page_size: 10
      })
    )
    expect(wrapper.text()).toContain('32000 ms')
    expect(wrapper.text()).toContain('45000 ms')
    expect(wrapper.text()).toContain('client:req-1')
  })
})

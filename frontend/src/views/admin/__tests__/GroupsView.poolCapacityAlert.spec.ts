import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { AdminGroup } from '@/types'
import GroupsView from '@/views/admin/GroupsView.vue'

const {
  createGroup,
  updateGroup,
  listGroups,
  getAllGroups,
  getModelsListCandidates,
  getUsageSummary,
  getCapacitySummary,
  showError,
  showSuccess,
} = vi.hoisted(() => ({
  createGroup: vi.fn(),
  updateGroup: vi.fn(),
  listGroups: vi.fn(),
  getAllGroups: vi.fn(),
  getModelsListCandidates: vi.fn(),
  getUsageSummary: vi.fn(),
  getCapacitySummary: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      list: listGroups,
      getAll: getAllGroups,
      getModelsListCandidates,
      getUsageSummary,
      getCapacitySummary,
      create: createGroup,
      update: updateGroup,
      delete: vi.fn(),
      updateSortOrder: vi.fn(),
      toggleStatus: vi.fn(),
      duplicate: vi.fn(),
    },
    accounts: {
      list: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 }),
      getById: vi.fn(),
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError }),
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({ isCurrentStep: vi.fn(() => false), nextStep: vi.fn() }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const group: AdminGroup = {
  id: 7,
  name: 'Pool group',
  description: null,
  platform: 'anthropic',
  rate_multiplier: 1,
  rpm_limit: 0,
  pool_capacity_alert_enabled: true,
  pool_capacity_alert_metric: 'remaining_balance_usd',
  pool_capacity_alert_threshold_requests: 75,
  pool_capacity_alert_threshold_usd: 12.5,
  is_exclusive: false,
  status: 'active',
  subscription_type: 'standard',
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  allow_image_generation: false,
  allow_batch_image_generation: false,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  batch_image_discount_multiplier: 0.5,
  batch_image_hold_multiplier: 0.6,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  video_rate_independent: false,
  video_rate_multiplier: 1,
  video_price_480p: null,
  video_price_720p: null,
  video_price_1080p: null,
  web_search_price_per_call: null,
  peak_rate_enabled: false,
  peak_start: '',
  peak_end: '',
  peak_rate_multiplier: 1,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  allow_messages_dispatch: false,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '2026-07-21T00:00:00Z',
  updated_at: '2026-07-21T00:00:00Z',
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: true,
  supported_model_scopes: [],
  models_list_config: undefined,
  sort_order: 1,
}

const AppLayoutStub = defineComponent({ template: '<main><slot /></main>' })
const TablePageLayoutStub = defineComponent({
  template: '<section><slot name="filters" /><slot name="table" /><slot name="pagination" /></section>',
})
const DataTableStub = defineComponent({
  props: { data: { type: Array, default: () => [] } },
  template: '<div><div v-for="row in data" :key="row.id"><slot name="cell-actions" :row="row" /></div></div>',
})
const BaseDialogStub = defineComponent({
  props: { show: Boolean },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})
const SelectStub = defineComponent({
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'change'],
  template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value); $emit(\'change\')"><option v-for="option in options" :key="String(option.value)" :value="option.value">{{ option.label }}</option></select>',
})

function mountView() {
  return mount(GroupsView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        Pagination: true,
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        EmptyState: true,
        Select: SelectStub,
        PlatformIcon: true,
        Icon: true,
        GroupCapacityBadge: true,
        GroupRateMultipliersModal: true,
        GroupRPMOverridesModal: true,
        VueDraggable: { template: '<div><slot /></div>' },
      },
    },
  })
}

type MountedView = ReturnType<typeof mountView>

type ExposedCreateForm = {
  createForm: {
    pool_capacity_alert_metric: 'predicted_requests' | 'remaining_balance_usd'
    pool_capacity_alert_threshold_requests: number | string
    pool_capacity_alert_threshold_usd: number | string | null
  }
}

async function openCreate(wrapper: MountedView) {
  await wrapper.get('[data-tour="groups-create-btn"]').trigger('click')
}

async function openEdit(wrapper: MountedView, index = 0) {
  await wrapper.findAll('[data-testid="group-edit"]')[index].trigger('click')
  await flushPromises()
}

async function closeVisibleDialog(wrapper: MountedView) {
  const cancelButton = wrapper.findAll('button').find((button) => button.text() === 'common.cancel')
  if (!cancelButton) throw new Error('visible dialog cancel button not found')
  await cancelButton.trigger('click')
}

async function submitCreate(wrapper: MountedView) {
  await wrapper.get('#create-group-form').trigger('submit')
  await flushPromises()
}

function exposedCreateForm(wrapper: MountedView) {
  return (wrapper.vm as unknown as ExposedCreateForm).createForm
}

describe('GroupsView pool capacity alert form', () => {
  beforeEach(() => {
    localStorage.clear()
    createGroup.mockReset().mockResolvedValue(group)
    updateGroup.mockReset().mockResolvedValue(group)
    listGroups.mockReset().mockResolvedValue({
      items: [group],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    getAllGroups.mockReset().mockResolvedValue([])
    getModelsListCandidates.mockReset().mockResolvedValue([])
    getUsageSummary.mockReset().mockResolvedValue([])
    getCapacitySummary.mockReset().mockResolvedValue([])
    showError.mockReset()
    showSuccess.mockReset()
  })

  it('defaults create to off, then reveals predicted requests with threshold 50 and ARIA wiring', async () => {
    const wrapper = mountView()
    await flushPromises()
    await openCreate(wrapper)

    const alertSwitch = wrapper.get('[data-testid="create-pool-capacity-alert-switch"]')
    expect(alertSwitch.attributes('aria-checked')).toBe('false')
    expect(alertSwitch.attributes('aria-expanded')).toBe('false')
    expect(alertSwitch.attributes('aria-controls')).toBe('create-pool-capacity-alert-config')
    expect(wrapper.find('#create-pool-capacity-alert-config').exists()).toBe(false)

    await alertSwitch.trigger('click')

    expect(alertSwitch.attributes('aria-checked')).toBe('true')
    expect(alertSwitch.attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('#create-pool-capacity-alert-config').find('fieldset').exists()).toBe(true)
    expect(wrapper.get('[data-testid="create-pool-capacity-alert-metric-requests"]').attributes('name'))
      .toBe('create-pool-capacity-alert-metric')
    expect((wrapper.get('[data-testid="create-pool-capacity-alert-metric-requests"]').element as HTMLInputElement).checked)
      .toBe(true)
    expect((wrapper.get('[data-testid="create-pool-capacity-alert-threshold-requests"]').element as HTMLInputElement).value)
      .toBe('50')
    expect(wrapper.find('[data-testid="create-pool-capacity-alert-threshold-usd"]').exists()).toBe(false)
  })

  it('stores request and USD thresholds independently, preserves them across metric and switch changes, and submits all policy fields', async () => {
    const wrapper = mountView()
    await flushPromises()
    await openCreate(wrapper)
    await wrapper.get('[data-tour="group-form-name"]').setValue('New pool group')
    const alertSwitch = wrapper.get('[data-testid="create-pool-capacity-alert-switch"]')
    await alertSwitch.trigger('click')

    await wrapper.get('[data-testid="create-pool-capacity-alert-threshold-requests"]').setValue('123')
    await wrapper.get('[data-testid="create-pool-capacity-alert-metric-usd"]').setValue()
    await wrapper.get('[data-testid="create-pool-capacity-alert-threshold-usd"]').setValue('9.75')
    await wrapper.get('[data-testid="create-pool-capacity-alert-metric-requests"]').setValue()
    expect((wrapper.get('[data-testid="create-pool-capacity-alert-threshold-requests"]').element as HTMLInputElement).value)
      .toBe('123')

    await wrapper.get('[data-testid="create-pool-capacity-alert-metric-usd"]').setValue()
    expect((wrapper.get('[data-testid="create-pool-capacity-alert-threshold-usd"]').element as HTMLInputElement).value)
      .toBe('9.75')

    await alertSwitch.trigger('click')
    expect(wrapper.find('#create-pool-capacity-alert-config').exists()).toBe(false)
    await alertSwitch.trigger('click')
    expect((wrapper.get('[data-testid="create-pool-capacity-alert-threshold-usd"]').element as HTMLInputElement).value)
      .toBe('9.75')

    await submitCreate(wrapper)

    expect(createGroup).toHaveBeenCalledWith(
      expect.objectContaining({
        pool_capacity_alert_enabled: true,
        pool_capacity_alert_metric: 'remaining_balance_usd',
        pool_capacity_alert_threshold_requests: 123,
        pool_capacity_alert_threshold_usd: 9.75,
      }),
    )
  })

  it('resets create policy defaults after closing and reopening the dialog', async () => {
    const wrapper = mountView()
    await flushPromises()
    await openCreate(wrapper)
    await wrapper.get('[data-testid="create-pool-capacity-alert-switch"]').trigger('click')
    await wrapper.get('[data-testid="create-pool-capacity-alert-threshold-requests"]').setValue('321')
    await wrapper.get('[data-testid="create-pool-capacity-alert-metric-usd"]').setValue()
    await wrapper.get('[data-testid="create-pool-capacity-alert-threshold-usd"]').setValue('4.5')

    await closeVisibleDialog(wrapper)
    await openCreate(wrapper)

    const alertSwitch = wrapper.get('[data-testid="create-pool-capacity-alert-switch"]')
    expect(alertSwitch.attributes('aria-checked')).toBe('false')
    await alertSwitch.trigger('click')
    expect((wrapper.get('[data-testid="create-pool-capacity-alert-metric-requests"]').element as HTMLInputElement).checked)
      .toBe(true)
    expect((wrapper.get('[data-testid="create-pool-capacity-alert-threshold-requests"]').element as HTMLInputElement).value)
      .toBe('50')
    await wrapper.get('[data-testid="create-pool-capacity-alert-metric-usd"]').setValue()
    expect((wrapper.get('[data-testid="create-pool-capacity-alert-threshold-usd"]').element as HTMLInputElement).value)
      .toBe('')
  })

  it('loads saved edit values, keeps both thresholds while switching, and submits changes', async () => {
    const wrapper = mountView()
    await flushPromises()
    await openEdit(wrapper)

    const alertSwitch = wrapper.get('[data-testid="edit-pool-capacity-alert-switch"]')
    expect(alertSwitch.attributes('aria-checked')).toBe('true')
    expect((wrapper.get('[data-testid="edit-pool-capacity-alert-metric-usd"]').element as HTMLInputElement).checked)
      .toBe(true)
    expect((wrapper.get('[data-testid="edit-pool-capacity-alert-threshold-usd"]').element as HTMLInputElement).value)
      .toBe('12.5')

    await wrapper.get('[data-testid="edit-pool-capacity-alert-metric-requests"]').setValue()
    expect((wrapper.get('[data-testid="edit-pool-capacity-alert-threshold-requests"]').element as HTMLInputElement).value)
      .toBe('75')
    await wrapper.get('[data-testid="edit-pool-capacity-alert-threshold-requests"]').setValue('80')
    await wrapper.get('[data-testid="edit-pool-capacity-alert-metric-usd"]').setValue()
    expect((wrapper.get('[data-testid="edit-pool-capacity-alert-threshold-usd"]').element as HTMLInputElement).value)
      .toBe('12.5')

    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()

    expect(updateGroup).toHaveBeenCalledWith(
      7,
      expect.objectContaining({
        pool_capacity_alert_enabled: true,
        pool_capacity_alert_metric: 'remaining_balance_usd',
        pool_capacity_alert_threshold_requests: 80,
        pool_capacity_alert_threshold_usd: 12.5,
      }),
    )
  })

  it('blocks edit API calls and shows an inline error for an invalid USD threshold', async () => {
    const wrapper = mountView()
    await flushPromises()
    await openEdit(wrapper)
    await wrapper.get('[data-testid="edit-pool-capacity-alert-threshold-usd"]').setValue('0')

    await wrapper.get('#edit-group-form').trigger('submit')
    await flushPromises()

    expect(updateGroup).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="edit-pool-capacity-alert-threshold-usd"]').attributes('aria-invalid'))
      .toBe('true')
    expect(wrapper.get('[role="alert"]').exists()).toBe(true)
    expect(showError).toHaveBeenCalledWith('admin.groups.poolCapacityAlert.validation.usdRange')
  })

  it('falls back to predicted requests and 50 for legacy admin responses', async () => {
    const legacyGroup: AdminGroup = {
      ...group,
      pool_capacity_alert_metric: undefined,
      pool_capacity_alert_threshold_requests: undefined,
      pool_capacity_alert_threshold_usd: undefined,
    }
    listGroups.mockResolvedValueOnce({
      items: [legacyGroup],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = mountView()
    await flushPromises()
    await openEdit(wrapper)

    expect((wrapper.get('[data-testid="edit-pool-capacity-alert-metric-requests"]').element as HTMLInputElement).checked)
      .toBe(true)
    expect((wrapper.get('[data-testid="edit-pool-capacity-alert-threshold-requests"]').element as HTMLInputElement).value)
      .toBe('50')
  })

  it('reloads independent policy values when editing different groups', async () => {
    const requestsGroup: AdminGroup = {
      ...group,
      id: 8,
      name: 'Requests pool',
      pool_capacity_alert_metric: 'predicted_requests',
      pool_capacity_alert_threshold_requests: 11,
      pool_capacity_alert_threshold_usd: 3.25,
    }
    const usdGroup: AdminGroup = {
      ...group,
      id: 9,
      name: 'USD pool',
      pool_capacity_alert_metric: 'remaining_balance_usd',
      pool_capacity_alert_threshold_requests: 99,
      pool_capacity_alert_threshold_usd: 2.5,
    }
    listGroups.mockResolvedValueOnce({
      items: [requestsGroup, usdGroup],
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1,
    })

    const wrapper = mountView()
    await flushPromises()
    await openEdit(wrapper, 0)
    expect((wrapper.get('[data-testid="edit-pool-capacity-alert-threshold-requests"]').element as HTMLInputElement).value)
      .toBe('11')
    await wrapper.get('[data-testid="edit-pool-capacity-alert-threshold-requests"]').setValue('222')
    await closeVisibleDialog(wrapper)

    await openEdit(wrapper, 1)
    expect((wrapper.get('[data-testid="edit-pool-capacity-alert-metric-usd"]').element as HTMLInputElement).checked)
      .toBe(true)
    expect((wrapper.get('[data-testid="edit-pool-capacity-alert-threshold-usd"]').element as HTMLInputElement).value)
      .toBe('2.5')
    await wrapper.get('[data-testid="edit-pool-capacity-alert-metric-requests"]').setValue()
    expect((wrapper.get('[data-testid="edit-pool-capacity-alert-threshold-requests"]').element as HTMLInputElement).value)
      .toBe('99')
  })

  it.each([
    ['empty', ''],
    ['zero', 0],
    ['negative', -1],
    ['fractional', 1.5],
    ['over maximum', 1_000_000_001],
    ['NaN', Number.NaN],
    ['Infinity', Number.POSITIVE_INFINITY],
  ])('blocks create for invalid request threshold: %s', async (_label, invalidValue) => {
    const wrapper = mountView()
    await flushPromises()
    await openCreate(wrapper)
    await wrapper.get('[data-tour="group-form-name"]').setValue('Invalid requests')
    await wrapper.get('[data-testid="create-pool-capacity-alert-switch"]').trigger('click')
    exposedCreateForm(wrapper).pool_capacity_alert_threshold_requests = invalidValue

    await submitCreate(wrapper)

    expect(createGroup).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="create-pool-capacity-alert-threshold-requests"]').attributes('aria-invalid'))
      .toBe('true')
    expect(wrapper.get('[role="alert"]').exists()).toBe(true)
    expect(showError).toHaveBeenCalled()
  })

  it.each([
    ['empty', ''],
    ['zero', 0],
    ['negative', -1],
    ['below minimum', 0.001],
    ['over maximum', 1_000_000_000_000_001],
    ['NaN', Number.NaN],
    ['Infinity', Number.POSITIVE_INFINITY],
  ])('blocks create for invalid USD threshold: %s', async (_label, invalidValue) => {
    const wrapper = mountView()
    await flushPromises()
    await openCreate(wrapper)
    await wrapper.get('[data-tour="group-form-name"]').setValue('Invalid USD')
    await wrapper.get('[data-testid="create-pool-capacity-alert-switch"]').trigger('click')
    await wrapper.get('[data-testid="create-pool-capacity-alert-metric-usd"]').setValue()
    exposedCreateForm(wrapper).pool_capacity_alert_threshold_usd = invalidValue

    await submitCreate(wrapper)

    expect(createGroup).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="create-pool-capacity-alert-threshold-usd"]').attributes('aria-invalid'))
      .toBe('true')
    expect(wrapper.get('[role="alert"]').exists()).toBe(true)
    expect(showError).toHaveBeenCalled()
  })

  it.each([1, 1_000_000_000])('accepts request threshold boundary %s', async (threshold) => {
    const wrapper = mountView()
    await flushPromises()
    await openCreate(wrapper)
    await wrapper.get('[data-tour="group-form-name"]').setValue('Valid requests')
    await wrapper.get('[data-testid="create-pool-capacity-alert-switch"]').trigger('click')
    await wrapper.get('[data-testid="create-pool-capacity-alert-threshold-requests"]').setValue(String(threshold))

    await submitCreate(wrapper)

    expect(createGroup).toHaveBeenCalledWith(
      expect.objectContaining({
        pool_capacity_alert_metric: 'predicted_requests',
        pool_capacity_alert_threshold_requests: threshold,
        pool_capacity_alert_threshold_usd: null,
      }),
    )
  })

  it.each([0.01, 1_000_000_000_000_000])('accepts USD threshold boundary %s', async (threshold) => {
    const wrapper = mountView()
    await flushPromises()
    await openCreate(wrapper)
    await wrapper.get('[data-tour="group-form-name"]').setValue('Valid USD')
    await wrapper.get('[data-testid="create-pool-capacity-alert-switch"]').trigger('click')
    await wrapper.get('[data-testid="create-pool-capacity-alert-metric-usd"]').setValue()
    await wrapper.get('[data-testid="create-pool-capacity-alert-threshold-usd"]').setValue(String(threshold))

    await submitCreate(wrapper)

    expect(createGroup).toHaveBeenCalledWith(
      expect.objectContaining({
        pool_capacity_alert_metric: 'remaining_balance_usd',
        pool_capacity_alert_threshold_requests: 50,
        pool_capacity_alert_threshold_usd: threshold,
      }),
    )
  })

  it('validates retained policy values even while the alert switch is off', async () => {
    const wrapper = mountView()
    await flushPromises()
    await openCreate(wrapper)
    await wrapper.get('[data-tour="group-form-name"]').setValue('Disabled invalid policy')
    exposedCreateForm(wrapper).pool_capacity_alert_threshold_requests = 0

    await submitCreate(wrapper)

    expect(createGroup).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="create-pool-capacity-alert-switch"]').attributes('aria-checked'))
      .toBe('true')
    expect(wrapper.get('[data-testid="create-pool-capacity-alert-threshold-requests"]').attributes('aria-invalid'))
      .toBe('true')
    expect(wrapper.get('[role="alert"]').exists()).toBe(true)
    expect(showError).toHaveBeenCalledWith('admin.groups.poolCapacityAlert.validation.requestsRange')
  })
})

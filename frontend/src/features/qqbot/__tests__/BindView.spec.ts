import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import BindView from '../public/BindView.vue'

const mocks = vi.hoisted(() => ({ inspectBinding: vi.fn(), completeBinding: vi.fn(), setLocale: vi.fn() }))
vi.mock('../api', () => ({ default: { inspectBinding: mocks.inspectBinding, completeBinding: mocks.completeBinding } }))
vi.mock('@/i18n', () => ({ setLocale: mocks.setLocale }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ locale: { value: 'zh' }, t: (key: string, params?: Record<string, unknown>) => params?.count !== undefined ? `${key}:${params.count}` : key }) }
})

async function mountWithToken(token: string) {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/bind', component: BindView }] })
  await router.push({ path: '/bind', query: token ? { token } : {} })
  await router.isReady()
  const wrapper = mount(BindView, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}

describe('QQBot public BindView', () => {
  beforeEach(() => { mocks.inspectBinding.mockReset(); mocks.completeBinding.mockReset(); mocks.setLocale.mockReset() })
  afterEach(() => vi.useRealTimers())

  it('rejects malformed tokens without calling the API', async () => {
    const wrapper = await mountWithToken('short')
    expect(mocks.inspectBinding).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('qqbotBind.states.invalidToken.title')
  })

  it('renders pending details and validates QQ number format', async () => {
    mocks.inspectBinding.mockResolvedValue({ status: 'pending', masked_email: 'a***@example.com', scene: 'group', bonus_amount: 5, expires_at: new Date(Date.now() + 600000).toISOString() })
    const wrapper = await mountWithToken('a'.repeat(32))
    expect(wrapper.text()).toContain('a***@example.com')
    await wrapper.get('#qqbot-number').setValue('01234')
    await wrapper.get('form').trigger('submit')
    expect(mocks.completeBinding).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('qqbotBind.validation.leadingZero')
  })

  it('prevents duplicate submission and renders the completed state', async () => {
    mocks.inspectBinding.mockResolvedValue({ status: 'pending', masked_email: 'a***@example.com', bonus_amount: 5, expires_at: new Date(Date.now() + 600000).toISOString() })
    let resolveComplete: (value: unknown) => void = () => {}
    mocks.completeBinding.mockImplementation(() => new Promise((resolve) => { resolveComplete = resolve }))
    const wrapper = await mountWithToken('a'.repeat(32))
    await wrapper.get('#qqbot-number').setValue('123456')
    await wrapper.get('form').trigger('submit')
    await wrapper.get('form').trigger('submit')
    expect(mocks.completeBinding).toHaveBeenCalledTimes(1)
    resolveComplete({ status: 'completed', granted: true, bonus_amount: 5, balance_after: 105 })
    await flushPromises()
    expect(wrapper.text()).toContain('qqbotBind.states.completed.title')
    expect(wrapper.text()).toContain('105.00')
  })

  it('shows a retry action for network failures', async () => {
    mocks.inspectBinding.mockRejectedValue({ status: 0, message: 'offline' })
    const wrapper = await mountWithToken('a'.repeat(32))
    expect(wrapper.text()).toContain('qqbotBind.states.networkError.title')
    expect(wrapper.find('button.mt-6').exists()).toBe(true)
  })
})

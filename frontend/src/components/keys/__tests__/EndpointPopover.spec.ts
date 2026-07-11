import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { measureEndpointLatency } = vi.hoisted(() => ({
  measureEndpointLatency: vi.fn(),
}))
const copyToClipboard = vi.fn().mockResolvedValue(true)

const messages: Record<string, string> = {
  'keys.endpoints.availableTitle': '可用端点',
  'keys.endpoints.primaryName': '主线路',
  'keys.endpoints.builtInName': 'PokeAPI 备用入口',
  'keys.endpoints.builtInDescription': '备用访问域名',
  'keys.endpoints.defaultDescription': '管理员提供的线路',
  'keys.endpoints.default': '默认',
  'keys.endpoints.backup': '备用',
  'keys.endpoints.copied': '已复制',
  'keys.endpoints.copiedHint': '已复制到剪贴板',
  'keys.endpoints.clickToCopy': '点击复制此端点',
  'keys.endpoints.retest': '重新测速',
  'keys.endpoints.testingAll': '测速中',
  'keys.endpoints.testing': '检测中',
  'keys.endpoints.timeout': '超时',
  'keys.endpoints.unreachable': '不可达',
  'keys.endpoints.notTested': '尚未测速',
  'keys.endpoints.testedSummary': '已完成 {tested}/{total}',
  'keys.endpoints.nodeAddress': '节点地址',
  'keys.endpoints.measuredLatency': '当前延迟',
  'keys.endpoints.nodeStatus': '连接状态',
  'keys.endpoints.lastTestedAt': '测速时间',
  'keys.endpoints.statusTesting': '正在检测',
  'keys.endpoints.statusTimeout': '连接超时',
  'keys.endpoints.statusUnreachable': '当前网络无法访问',
  'keys.endpoints.status.fast': '响应迅速',
  'keys.endpoints.status.normal': '连接正常',
  'keys.endpoints.status.slow': '连接偏慢',
  'keys.endpoints.status.poor': '延迟较高',
  'keys.endpoints.status.unavailable': '暂不可用',
  'keys.endpoints.clientProbeHint': '由当前浏览器直连测得',
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    locale: { value: 'zh-CN' },
    t: (key: string, params?: Record<string, unknown>) => {
      const message = messages[key] ?? key
      return Object.entries(params ?? {}).reduce(
        (result, [name, value]) => result.replace(`{${name}}`, String(value)),
        message,
      )
    },
  }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard }),
}))

vi.mock('@/utils/endpointLatency', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/utils/endpointLatency')>()
  return { ...actual, measureEndpointLatency }
})

import EndpointPopover from '../EndpointPopover.vue'

describe('EndpointPopover', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    measureEndpointLatency.mockResolvedValue({
      status: 'success',
      latencyMs: 151,
      testedAt: new Date('2026-04-02T12:00:00Z').getTime(),
    })
  })

  it('展示主端点、自定义端点和内置 www.pokeapi.top，并显示浏览器实测延迟', async () => {
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: 'https://default.example.com/v1',
        customEndpoints: [
          { name: '香港节点', endpoint: 'https://hk.example.com', description: '国内低延迟入口' },
        ],
      },
    })

    await flushPromises()

    expect(wrapper.text()).toContain('default.example.com')
    expect(wrapper.text()).toContain('hk.example.com')
    expect(wrapper.text()).toContain('www.pokeapi.top')
    expect(wrapper.text()).toContain('151ms')
    expect(wrapper.text()).toContain('国内低延迟入口')
    expect(measureEndpointLatency).toHaveBeenCalledTimes(3)
  })

  it('规范化去重，管理员已配置备用域名时不会重复展示', async () => {
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: 'https://default.example.com',
        customEndpoints: [
          { name: '管理员备用入口', endpoint: 'https://www.pokeapi.top/', description: '自定义说明' },
        ],
      },
    })

    await flushPromises()

    expect(wrapper.findAll('article')).toHaveLength(2)
    expect(wrapper.text().match(/www\.pokeapi\.top/g)).toHaveLength(2)
    expect(wrapper.text()).toContain('管理员备用入口')
  })

  it('点击端点后复制并切换为已复制提示', async () => {
    const wrapper = mount(EndpointPopover, {
      props: { apiBaseUrl: 'https://default.example.com/v1', customEndpoints: [] },
    })
    await flushPromises()

    await wrapper.find('article button').trigger('click')
    await flushPromises()

    expect(copyToClipboard).toHaveBeenCalledWith('https://default.example.com/v1', '已复制')
    expect(wrapper.text()).toContain('已复制到剪贴板')
    expect(wrapper.find('button[aria-label="已复制到剪贴板"]').exists()).toBe(true)
  })

  it('重新测速会再次探测全部端点，并展示不可达状态', async () => {
    measureEndpointLatency.mockResolvedValue({ status: 'error', latencyMs: null, testedAt: Date.now() })
    const wrapper = mount(EndpointPopover, {
      props: { apiBaseUrl: 'https://default.example.com', customEndpoints: [] },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('不可达')
    expect(wrapper.text()).toContain('当前网络无法访问')

    await wrapper.find('button[aria-label="重新测速"]').trigger('click')
    await flushPromises()

    expect(measureEndpointLatency).toHaveBeenCalledTimes(4)
  })
})

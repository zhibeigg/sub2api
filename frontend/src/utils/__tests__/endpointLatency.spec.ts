import { describe, expect, it, vi } from 'vitest'
import {
  BUILT_IN_ENDPOINT_URL,
  buildEndpointDisplayItems,
  buildEndpointProbeUrl,
  classifyEndpointLatency,
  measureEndpointLatency,
  normalizeEndpoint,
  runWithConcurrency,
} from '../endpointLatency'

describe('endpointLatency', () => {
  it('规范化端点并过滤无效协议', () => {
    expect(normalizeEndpoint(' https://API.Example.com/v1/ ')).toBe('https://api.example.com/v1')
    expect(normalizeEndpoint('ftp://example.com')).toBeNull()
    expect(normalizeEndpoint('not-a-url')).toBeNull()
  })

  it('合并主端点、自定义端点和内置备用端点并去重', () => {
    const items = buildEndpointDisplayItems(
      'https://api.example.com/v1/',
      [
        { name: '重复线路', endpoint: 'https://api.example.com/v1', description: '' },
        { name: '香港节点', endpoint: 'https://hk.example.com', description: '低延迟' },
      ],
      [{ name: '备用入口', endpoint: BUILT_IN_ENDPOINT_URL, description: '备用域名' }],
    )

    expect(items).toHaveLength(3)
    expect(items[0]).toMatchObject({ host: 'api.example.com', isDefault: true })
    expect(items[1]).toMatchObject({ host: 'hk.example.com', isBuiltIn: false })
    expect(items[2]).toMatchObject({ host: 'www.pokeapi.top', isBuiltIn: true })
  })

  it('始终使用端点 origin 下的 health 路径进行浏览器探测', () => {
    expect(buildEndpointProbeUrl('https://api.example.com/v1', 123)).toBe(
      'https://api.example.com/health?endpoint_probe=123',
    )
  })

  it('按客户端延迟划分健康状态', () => {
    expect(classifyEndpointLatency(80, 'success')).toBe('fast')
    expect(classifyEndpointLatency(350, 'success')).toBe('normal')
    expect(classifyEndpointLatency(800, 'success')).toBe('slow')
    expect(classifyEndpointLatency(1_500, 'success')).toBe('poor')
    expect(classifyEndpointLatency(null, 'timeout')).toBe('unavailable')
  })

  it('测量当前浏览器到端点的耗时', async () => {
    const fetchImpl = vi.fn().mockResolvedValue(new Response(null, { status: 204 }))
    const timestamps = [100, 248]

    const result = await measureEndpointLatency('https://api.example.com/v1', {
      fetchImpl,
      now: () => timestamps.shift() ?? 248,
    })

    expect(result).toMatchObject({ status: 'success', latencyMs: 148 })
    expect(fetchImpl).toHaveBeenCalledWith(
      expect.stringMatching(/^https:\/\/api\.example\.com\/health\?endpoint_probe=/),
      expect.objectContaining({ mode: 'no-cors', cache: 'no-store', credentials: 'omit' }),
    )
  })

  it('将超时与普通网络错误区分开', async () => {
    vi.useFakeTimers()
    const fetchImpl = vi.fn((_url: RequestInfo | URL, init?: RequestInit) => new Promise((_resolve, reject) => {
      init?.signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))
    })) as typeof fetch

    const promise = measureEndpointLatency('https://slow.example.com', {
      fetchImpl,
      timeoutMs: 50,
      now: () => 0,
    })
    await vi.advanceTimersByTimeAsync(50)

    await expect(promise).resolves.toMatchObject({ status: 'timeout', latencyMs: null })
    vi.useRealTimers()
  })

  it('限制批量测速并发数', async () => {
    let active = 0
    let maxActive = 0

    await runWithConcurrency([1, 2, 3, 4, 5], 2, async () => {
      active += 1
      maxActive = Math.max(maxActive, active)
      await Promise.resolve()
      active -= 1
    })

    expect(maxActive).toBe(2)
  })
})

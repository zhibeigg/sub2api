import type { CustomEndpoint } from '@/types'

export const BUILT_IN_ENDPOINT_URL = 'https://www.pokeapi.top'
export const DEFAULT_ENDPOINT_PROBE_TIMEOUT_MS = 6_000

export type EndpointProbeStatus = 'idle' | 'testing' | 'success' | 'timeout' | 'error'
export type EndpointLatencyLevel = 'fast' | 'normal' | 'slow' | 'poor' | 'unavailable'

export interface EndpointDisplayItem extends CustomEndpoint {
  key: string
  host: string
  protocol: string
  isDefault: boolean
  isBuiltIn: boolean
}

export interface EndpointProbeResult {
  status: EndpointProbeStatus
  latencyMs: number | null
  testedAt: number | null
}

interface ProbeOptions {
  timeoutMs?: number
  signal?: AbortSignal
  fetchImpl?: typeof fetch
  now?: () => number
}

export function normalizeEndpoint(endpoint: string): string | null {
  const trimmed = endpoint.trim()
  if (!trimmed) return null

  try {
    const url = new URL(trimmed)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return null
    url.hash = ''
    url.pathname = url.pathname.replace(/\/+$/, '') || '/'
    return url.toString().replace(/\/$/, '')
  } catch {
    return null
  }
}

export function endpointKey(endpoint: string): string {
  return normalizeEndpoint(endpoint)?.toLowerCase() ?? endpoint.trim().toLowerCase()
}

export function buildEndpointDisplayItems(
  apiBaseUrl: string,
  customEndpoints: CustomEndpoint[],
  builtInEndpoints: CustomEndpoint[] = [],
): EndpointDisplayItem[] {
  const result: EndpointDisplayItem[] = []
  const seen = new Set<string>()

  const append = (item: CustomEndpoint, flags: { isDefault: boolean; isBuiltIn: boolean }) => {
    const normalized = normalizeEndpoint(item.endpoint)
    if (!normalized) return

    const key = endpointKey(normalized)
    if (seen.has(key)) return
    seen.add(key)

    const url = new URL(normalized)
    result.push({
      ...item,
      endpoint: normalized,
      key,
      host: url.host,
      protocol: url.protocol.replace(':', '').toUpperCase(),
      ...flags,
    })
  }

  if (apiBaseUrl.trim()) {
    append(
      { name: '', endpoint: apiBaseUrl, description: '' },
      { isDefault: true, isBuiltIn: false },
    )
  }

  customEndpoints.forEach((item) => append(item, { isDefault: false, isBuiltIn: false }))
  builtInEndpoints.forEach((item) => append(item, { isDefault: false, isBuiltIn: true }))

  return result
}

export function buildEndpointProbeUrl(endpoint: string, cacheBuster = Date.now()): string {
  const normalized = normalizeEndpoint(endpoint)
  if (!normalized) throw new Error('Invalid endpoint URL')

  const url = new URL(normalized)
  url.pathname = '/health'
  url.search = `endpoint_probe=${cacheBuster}`
  url.hash = ''
  return url.toString()
}

export function classifyEndpointLatency(latencyMs: number | null, status: EndpointProbeStatus): EndpointLatencyLevel {
  if (status !== 'success' || latencyMs == null) return 'unavailable'
  if (latencyMs < 200) return 'fast'
  if (latencyMs < 600) return 'normal'
  if (latencyMs < 1_200) return 'slow'
  return 'poor'
}

export async function measureEndpointLatency(
  endpoint: string,
  options: ProbeOptions = {},
): Promise<EndpointProbeResult> {
  const timeoutMs = options.timeoutMs ?? DEFAULT_ENDPOINT_PROBE_TIMEOUT_MS
  const fetchImpl = options.fetchImpl ?? fetch
  const now = options.now ?? (() => performance.now())
  const controller = new AbortController()
  let timedOut = false

  const abortFromParent = () => controller.abort(options.signal?.reason)
  if (options.signal?.aborted) {
    abortFromParent()
  } else {
    options.signal?.addEventListener('abort', abortFromParent, { once: true })
  }

  const timeoutId = window.setTimeout(() => {
    timedOut = true
    controller.abort()
  }, timeoutMs)

  const startedAt = now()

  try {
    await fetchImpl(buildEndpointProbeUrl(endpoint), {
      method: 'GET',
      mode: 'no-cors',
      cache: 'no-store',
      redirect: 'follow',
      credentials: 'omit',
      signal: controller.signal,
    })

    return {
      status: 'success',
      latencyMs: Math.max(1, Math.round(now() - startedAt)),
      testedAt: Date.now(),
    }
  } catch {
    return {
      status: timedOut ? 'timeout' : 'error',
      latencyMs: null,
      testedAt: Date.now(),
    }
  } finally {
    window.clearTimeout(timeoutId)
    options.signal?.removeEventListener('abort', abortFromParent)
  }
}

export async function runWithConcurrency<T>(
  items: T[],
  limit: number,
  worker: (item: T) => Promise<void>,
): Promise<void> {
  const queue = [...items]
  const workerCount = Math.max(1, Math.min(limit, queue.length))

  await Promise.all(
    Array.from({ length: workerCount }, async () => {
      while (queue.length > 0) {
        const item = queue.shift()
        if (item === undefined) return
        await worker(item)
      }
    }),
  )
}

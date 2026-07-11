/**
 * Playground gateway API — thin wrappers around sub2api's OpenAI-compatible
 * gateway endpoints. These use the raw `fetch` API (NOT the axios apiClient)
 * because they authenticate with the user's *API key* (Bearer) instead of the
 * JWT session token, and need streaming (ReadableStream) support.
 */

import { apiClient } from './client'
import { buildGatewayUrl } from './url'
import type { PlaygroundModelOption } from '@/types/playground'

const GROUP_HEADER = 'X-Sub2API-Group-ID'

function gatewayHeaders(apiKey: string, groupId: number, contentType = false): Record<string, string> {
  return {
    Authorization: `Bearer ${apiKey}`,
    [GROUP_HEADER]: String(groupId),
    ...(contentType ? { 'Content-Type': 'application/json' } : {})
  }
}

export interface PlaygroundModel {
  id: string
  owned_by?: string
}

export interface ChatUsage {
  prompt_tokens?: number
  completion_tokens?: number
  total_tokens?: number
}

export interface ChatMessagePayload {
  role: 'system' | 'user' | 'assistant'
  content: string
}

export interface ChatStreamOptions {
  apiKey: string
  groupId: number
  model: string
  messages: ChatMessagePayload[]
  temperature?: number
  maxTokens?: number
  signal?: AbortSignal
  onDelta: (chunk: string) => void
  onUsage?: (usage: ChatUsage) => void
}

export interface ImageGenerationOptions {
  apiKey: string
  groupId: number
  model: string
  prompt: string
  size?: string
  quality?: string
  n?: number
  signal?: AbortSignal
}

export interface GeneratedImage {
  url?: string
  b64_json?: string
  revised_prompt?: string
}

/** List group-aware model options using the JWT session; no API key value is sent or returned. */
export async function listModelOptions(apiKeyId: number, signal?: AbortSignal): Promise<PlaygroundModelOption[]> {
  const { data } = await apiClient.get<PlaygroundModelOption[]>(
    `/playground/api-keys/${apiKeyId}/model-options`,
    { signal }
  )
  return Array.isArray(data) ? data : []
}

/** Legacy gateway model listing helper. */
export async function listModels(apiKey: string, groupId: number, signal?: AbortSignal): Promise<PlaygroundModel[]> {
  const res = await fetch(buildGatewayUrl('/v1/models'), {
    method: 'GET',
    headers: gatewayHeaders(apiKey, groupId),
    signal
  })
  if (!res.ok) {
    throw new Error(await extractError(res))
  }
  const json = await res.json()
  const data = Array.isArray(json?.data) ? json.data : []
  return data
    .map((m: Record<string, unknown>) => ({
      id: String(m.id ?? ''),
      owned_by: typeof m.owned_by === 'string' ? m.owned_by : undefined
    }))
    .filter((m: PlaygroundModel) => m.id)
}

/**
 * Stream a chat completion. Resolves when the stream ends; rejects on error
 * (including aborts, which surface as AbortError — caller may ignore those).
 */
export async function streamChat(opts: ChatStreamOptions): Promise<void> {
  const res = await fetch(buildGatewayUrl('/v1/chat/completions'), {
    method: 'POST',
    headers: gatewayHeaders(opts.apiKey, opts.groupId, true),
    body: JSON.stringify({
      model: opts.model,
      messages: opts.messages,
      stream: true,
      stream_options: { include_usage: true },
      ...(opts.temperature != null ? { temperature: opts.temperature } : {}),
      ...(opts.maxTokens != null && opts.maxTokens > 0 ? { max_tokens: opts.maxTokens } : {})
    }),
    signal: opts.signal
  })

  if (!res.ok || !res.body) {
    throw new Error(await extractError(res))
  }

  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })

    // SSE events are separated by a blank line.
    let sepIndex: number
    while ((sepIndex = buffer.indexOf('\n\n')) !== -1) {
      const rawEvent = buffer.slice(0, sepIndex)
      buffer = buffer.slice(sepIndex + 2)
      handleSseEvent(rawEvent, opts)
    }
  }
  // Flush any trailing event without a blank-line terminator.
  if (buffer.trim()) handleSseEvent(buffer, opts)
}

function handleSseEvent(rawEvent: string, opts: ChatStreamOptions): void {
  for (const line of rawEvent.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed.startsWith('data:')) continue
    const data = trimmed.slice(5).trim()
    if (!data || data === '[DONE]') continue
    try {
      const json = JSON.parse(data)
      const delta = json?.choices?.[0]?.delta?.content
      if (typeof delta === 'string' && delta) opts.onDelta(delta)
      if (json?.usage && opts.onUsage) opts.onUsage(json.usage as ChatUsage)
    } catch {
      // Ignore keep-alive comments / partial fragments.
    }
  }
}

/** Generate images via the OpenAI-compatible images endpoint. */
export async function generateImage(opts: ImageGenerationOptions): Promise<GeneratedImage[]> {
  const res = await fetch(buildGatewayUrl('/v1/images/generations'), {
    method: 'POST',
    headers: gatewayHeaders(opts.apiKey, opts.groupId, true),
    body: JSON.stringify({
      model: opts.model,
      prompt: opts.prompt,
      ...(opts.size ? { size: opts.size } : {}),
      ...(opts.quality ? { quality: opts.quality } : {}),
      n: opts.n && opts.n > 0 ? opts.n : 1
    }),
    signal: opts.signal
  })
  if (!res.ok) {
    throw new Error(await extractError(res))
  }
  const json = await res.json()
  const data = Array.isArray(json?.data) ? json.data : []
  return data as GeneratedImage[]
}

export interface VideoGenerationOptions {
  apiKey: string
  groupId: number
  model: string
  prompt: string
  seconds?: number
  resolution?: string
  ratio?: string
  signal?: AbortSignal
}

export interface VideoSubmitResult {
  request_id: string
  status?: string
  model?: string
}

export interface VideoTaskStatus {
  request_id: string
  status: 'pending' | 'processing' | 'completed' | 'failed' | string
  url?: string
  video_url?: string
  error?: string
  model?: string
  usage?: { total_tokens?: number }
}

/** Submit an asynchronous text-to-video generation task. Returns a request_id. */
export async function generateVideo(opts: VideoGenerationOptions): Promise<VideoSubmitResult> {
  const res = await fetch(buildGatewayUrl('/v1/videos/generations'), {
    method: 'POST',
    headers: gatewayHeaders(opts.apiKey, opts.groupId, true),
    body: JSON.stringify({
      model: opts.model,
      prompt: opts.prompt,
      ...(opts.seconds && opts.seconds > 0 ? { seconds: opts.seconds } : {}),
      ...(opts.resolution ? { resolution: opts.resolution } : {}),
      ...(opts.ratio ? { ratio: opts.ratio } : {})
    }),
    signal: opts.signal
  })
  if (!res.ok) {
    throw new Error(await extractError(res))
  }
  const json = await res.json()
  const requestId = String(json?.request_id ?? json?.id ?? '')
  if (!requestId) {
    throw new Error('Video task id missing in response')
  }
  return { request_id: requestId, status: json?.status, model: json?.model }
}

/** Poll a video generation task's status by request_id. */
export async function getVideoStatus(
  apiKey: string,
  groupId: number,
  requestId: string,
  signal?: AbortSignal
): Promise<VideoTaskStatus> {
  const res = await fetch(buildGatewayUrl(`/v1/videos/${encodeURIComponent(requestId)}`), {
    method: 'GET',
    headers: gatewayHeaders(apiKey, groupId),
    signal
  })
  if (!res.ok) {
    throw new Error(await extractError(res))
  }
  const json = await res.json()
  return {
    request_id: String(json?.request_id ?? requestId),
    status: String(json?.status ?? 'processing'),
    url: json?.url || json?.video_url || undefined,
    video_url: json?.video_url || undefined,
    error: json?.error || undefined,
    model: json?.model || undefined,
    usage: json?.usage || undefined
  }
}

/** Extract a human-readable error message from a failed gateway response. */
async function extractError(res: Response): Promise<string> {
  try {
    const json = await res.json()
    const msg =
      json?.error?.message ||
      json?.error?.type ||
      json?.message ||
      (typeof json?.error === 'string' ? json.error : '')
    if (msg) return `${res.status} · ${msg}`
  } catch {
    // fall through
  }
  return `${res.status} ${res.statusText || 'Request failed'}`
}

export const playgroundAPI = {
  listModelOptions,
  listModels,
  streamChat,
  generateImage,
  generateVideo,
  getVideoStatus
}

export default playgroundAPI

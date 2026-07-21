/** Credential-aware streaming gateway helpers and JWT-authenticated playground APIs. */
import { apiClient } from './client'
import { buildGatewayUrl } from './url'
import type {
  PlaygroundContentPart,
  PlaygroundImageQuality,
  PlaygroundModelOption,
  PlaygroundTokenUsage,
  PlaygroundToolActivity
} from '@/types/playground'

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

export interface ChatUsage extends PlaygroundTokenUsage {}
export type ChatMessageContent = string | PlaygroundContentPart[]

export interface ChatMessagePayload<TContent = ChatMessageContent> {
  role: 'system' | 'user' | 'assistant'
  content: TContent
}

export interface ChatToolCallDelta {
  index: number
  id?: string
  type?: string
  function?: {
    name?: string
    arguments?: string
  }
}

export interface ChatToolCall extends ChatToolCallDelta {
  id: string
}

export interface ChatStreamOptions {
  apiKey: string
  groupId: number
  model: string
  messages: ChatMessagePayload[]
  temperature?: number
  topP?: number
  maxTokens?: number
  reasoningEffort?: string
  signal?: AbortSignal
  onDelta: (chunk: string) => void
  onUsage?: (usage: ChatUsage) => void
  onToolCallDelta?: (delta: ChatToolCallDelta) => void
  onToolCallComplete?: (toolCall: ChatToolCall) => void
  onEvent?: (event: PlaygroundSSEEvent) => void
}

export interface ImageGenerationOptions {
  apiKey: string
  groupId: number
  model: string
  prompt: string
  size?: string
  quality?: PlaygroundImageQuality
  n?: number
  images?: File[]
  signal?: AbortSignal
}

export interface GeneratedImage {
  url?: string
  b64_json?: string
  revised_prompt?: string
}

export interface PlaygroundSSEEvent {
  event?: string
  data: string
  id?: string
}

export interface ResponsesInputTextPart { type: 'input_text'; text: string }
export interface ResponsesInputImagePart { type: 'input_image'; image_url: string; detail?: 'auto' | 'low' | 'high' }
export interface ResponsesInputFilePart { type: 'input_file'; file_data?: string; file_id?: string; filename?: string }
export type ResponsesInputContentPart = ResponsesInputTextPart | ResponsesInputImagePart | ResponsesInputFilePart

export interface ResponsesInputMessage {
  type?: 'message'
  role: 'system' | 'developer' | 'user' | 'assistant'
  content: string | ResponsesInputContentPart[]
}

export type ResponsesInputItem = ResponsesInputMessage | Record<string, unknown>

export function chatMessagesToResponsesInput(messages: ChatMessagePayload[]): ResponsesInputItem[] {
  return messages.map((message) => ({
    type: 'message',
    role: message.role,
    content: typeof message.content === 'string'
      ? message.content
      : message.content.map((part): ResponsesInputContentPart => {
          if (part.type === 'text') return { type: 'input_text', text: part.text }
          if (part.type === 'image_url') {
            const image = typeof part.image_url === 'string' ? { url: part.image_url } : part.image_url
            return { type: 'input_image', image_url: image.url, ...(image.detail ? { detail: image.detail } : {}) }
          }
          return {
            type: 'input_file',
            ...(part.file.file_data ? { file_data: part.file.file_data } : {}),
            ...(part.file.file_id ? { file_id: part.file.file_id } : {}),
            ...(part.file.filename ? { filename: part.file.filename } : {})
          }
        })
  }))
}

export type ResponsesTool =
  | { type: 'web_search_preview' | 'web_search'; [key: string]: unknown }
  | { type: 'code_interpreter'; container?: string | Record<string, unknown>; [key: string]: unknown }
  | { type: 'function'; name: string; description?: string; parameters?: Record<string, unknown>; strict?: boolean }
  | Record<string, unknown>

export interface ResponsesStreamCompleted {
  id?: string
  status?: string
  model?: string
  usage?: ChatUsage
  response: Record<string, unknown>
}

export interface ResponsesStreamOptions {
  apiKey: string
  groupId: number
  model: string
  input?: string | ResponsesInputItem[]
  messages?: Array<Omit<ChatMessagePayload, 'content'> & { content: unknown }>
  instructions?: string
  temperature?: number
  topP?: number
  maxOutputTokens?: number
  reasoningEffort?: string
  tools?: ResponsesTool[]
  signal?: AbortSignal
  onDelta: (chunk: string) => void
  onUsage?: (usage: ChatUsage) => void
  onToolActivity?: (activity: PlaygroundToolActivity) => void
  onCompleted?: (result: ResponsesStreamCompleted) => void
  onEvent?: (event: PlaygroundSSEEvent) => void
}

export interface PlaygroundFetchURLRequest {
  urls: string[]
}

export interface PlaygroundFetchedURL {
  url: string
  final_url?: string
  status_code: number
  content_type: string
  content: string
  truncated?: boolean
}

export interface PlaygroundFetchURLResult {
  results: PlaygroundFetchedURL[]
}

export interface FetchWebContentOptions {
  urls: string[]
  signal?: AbortSignal
  /** Accepted for UI call-site compatibility; JWT auth is used and these values are never sent. */
  apiKey?: string
  groupId?: number
}

export interface PlaygroundWebContent {
  url: string
  title?: string
  content: string
  statusCode?: number
  contentType?: string
}

/** List the deduplicated model union for one key; hidden route metadata is retained and no key value is returned. */
export async function listModelOptions(apiKeyId: number, signal?: AbortSignal): Promise<PlaygroundModelOption[]> {
  const { data } = await apiClient.get<PlaygroundModelOption[]>(`/playground/api-keys/${apiKeyId}/model-options`, { signal })
  return Array.isArray(data) ? data : []
}

/** Fetch public text URLs through the JWT-authenticated backend proxy. */
export async function fetchPlaygroundURL(
  request: PlaygroundFetchURLRequest,
  signal?: AbortSignal
): Promise<PlaygroundFetchURLResult> {
  const { data } = await apiClient.post<PlaygroundFetchURLResult>('/playground/fetch-url', { urls: request.urls }, { signal })
  return { results: Array.isArray(data?.results) ? data.results : [] }
}

/** UI-oriented projection of fetchPlaygroundURL; API-key routing values are intentionally ignored. */
export async function fetchWebContent(options: FetchWebContentOptions): Promise<PlaygroundWebContent[]> {
  const response = await fetchPlaygroundURL({ urls: options.urls }, options.signal)
  return response.results.map((item) => ({
    url: item.final_url || item.url,
    content: item.content,
    statusCode: item.status_code,
    contentType: item.content_type
  }))
}

/** Legacy gateway model listing helper. */
export async function listModels(apiKey: string, groupId: number, signal?: AbortSignal): Promise<PlaygroundModel[]> {
  const res = await fetch(buildGatewayUrl('/v1/models'), {
    method: 'GET', headers: gatewayHeaders(apiKey, groupId), signal
  })
  if (!res.ok) throw new Error(await extractError(res))
  const json = await res.json()
  const data = Array.isArray(json?.data) ? json.data : []
  return data.map((model: Record<string, unknown>) => ({
    id: String(model.id ?? ''),
    owned_by: typeof model.owned_by === 'string' ? model.owned_by : undefined
  })).filter((model: PlaygroundModel) => model.id)
}

function findEventSeparator(buffer: string): { index: number; length: number } | null {
  const match = /\r?\n\r?\n/.exec(buffer)
  return match ? { index: match.index, length: match[0].length } : null
}

function parseSSEEvent(rawEvent: string): PlaygroundSSEEvent | null {
  const data: string[] = []
  let event: string | undefined
  let id: string | undefined
  for (const rawLine of rawEvent.split(/\r?\n/)) {
    if (!rawLine || rawLine.startsWith(':')) continue
    const colon = rawLine.indexOf(':')
    const field = colon < 0 ? rawLine : rawLine.slice(0, colon)
    let value = colon < 0 ? '' : rawLine.slice(colon + 1)
    if (value.startsWith(' ')) value = value.slice(1)
    if (field === 'data') data.push(value)
    else if (field === 'event') event = value
    else if (field === 'id') id = value
  }
  return data.length ? { event, id, data: data.join('\n') } : null
}

async function consumeSSE(
  body: ReadableStream<Uint8Array>,
  onEvent: (event: PlaygroundSSEEvent) => void
): Promise<void> {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    let separator = findEventSeparator(buffer)
    while (separator) {
      const rawEvent = buffer.slice(0, separator.index)
      buffer = buffer.slice(separator.index + separator.length)
      const event = parseSSEEvent(rawEvent)
      if (event) onEvent(event)
      separator = findEventSeparator(buffer)
    }
  }
  buffer += decoder.decode()
  if (buffer.trim()) {
    const event = parseSSEEvent(buffer)
    if (event) onEvent(event)
  }
}

function parseEventJSON(event: PlaygroundSSEEvent): Record<string, any> | null {
  if (!event.data || event.data === '[DONE]') return null
  try {
    const parsed = JSON.parse(event.data)
    return parsed && typeof parsed === 'object' ? parsed : null
  } catch {
    return null
  }
}

function normalizeUsage(value: unknown): ChatUsage | undefined {
  if (!value || typeof value !== 'object') return undefined
  const source = value as Record<string, unknown>
  const usage: ChatUsage = {}
  for (const key of ['prompt_tokens', 'completion_tokens', 'total_tokens', 'input_tokens', 'output_tokens'] as const) {
    if (typeof source[key] === 'number') usage[key] = source[key]
  }
  if (source.input_tokens_details && typeof source.input_tokens_details === 'object') {
    usage.input_tokens_details = source.input_tokens_details as Record<string, number>
  }
  if (source.output_tokens_details && typeof source.output_tokens_details === 'object') {
    usage.output_tokens_details = source.output_tokens_details as Record<string, number>
  }
  return Object.keys(usage).length ? usage : undefined
}

/** Stream an OpenAI-compatible chat completion, including fragmented tool-call deltas. */
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
      ...(opts.topP != null ? { top_p: opts.topP } : {}),
      ...(opts.maxTokens != null && opts.maxTokens > 0 ? { max_tokens: opts.maxTokens } : {}),
      ...(opts.reasoningEffort ? { reasoning_effort: opts.reasoningEffort } : {})
    }),
    signal: opts.signal
  })
  if (!res.ok || !res.body) throw new Error(await extractError(res))

  const toolCalls = new Map<number, ChatToolCall>()
  const completed = new Set<number>()
  await consumeSSE(res.body, (event) => {
    opts.onEvent?.(event)
    const json = parseEventJSON(event)
    if (!json) return
    const choice = json?.choices?.[0]
    const delta = choice?.delta?.content
    if (typeof delta === 'string' && delta) opts.onDelta(delta)
    const deltas = Array.isArray(choice?.delta?.tool_calls) ? choice.delta.tool_calls : []
    for (const raw of deltas) {
      const index = typeof raw?.index === 'number' ? raw.index : 0
      const current: ChatToolCall = toolCalls.get(index) ?? { index, id: '' }
      if (typeof raw?.id === 'string') current.id = raw.id
      if (typeof raw?.type === 'string') current.type = raw.type
      const name = typeof raw?.function?.name === 'string' ? raw.function.name : ''
      const args = typeof raw?.function?.arguments === 'string' ? raw.function.arguments : ''
      if (name || args) current.function = {
        name: `${current.function?.name ?? ''}${name}` || undefined,
        arguments: `${current.function?.arguments ?? ''}${args}` || undefined
      }
      toolCalls.set(index, current)
      opts.onToolCallDelta?.({ index, id: raw?.id, type: raw?.type, function: raw?.function })
    }
    const usage = normalizeUsage(json?.usage)
    if (usage) opts.onUsage?.(usage)
    if (choice?.finish_reason === 'tool_calls') {
      for (const [index, toolCall] of toolCalls) {
        if (!completed.has(index) && toolCall.id) {
          completed.add(index)
          opts.onToolCallComplete?.(toolCall)
        }
      }
    }
  })
  for (const [index, toolCall] of toolCalls) {
    if (!completed.has(index) && toolCall.id) opts.onToolCallComplete?.(toolCall)
  }
}

function activityFromItem(item: Record<string, any>, status: 'running' | 'completed' | 'failed'): PlaygroundToolActivity | null {
  const type = typeof item.type === 'string' ? item.type : ''
  if (!type || (!type.includes('call') && !type.includes('search') && !type.includes('code'))) return null
  const id = String(item.id ?? item.call_id ?? '')
  if (!id) return null
  return {
    id,
    type,
    name: typeof item.name === 'string' ? item.name : undefined,
    status,
    input: item.arguments ?? item.input ?? item.code,
    output: item.output ?? item.results,
    error: typeof item.error === 'string' ? item.error : item.error?.message,
    startedAt: status === 'running' ? Date.now() : undefined,
    completedAt: status === 'completed' || status === 'failed' ? Date.now() : undefined
  }
}

/** Stream the OpenAI Responses API and normalize text, usage, and tool lifecycle callbacks. */
export async function streamResponses(opts: ResponsesStreamOptions): Promise<void> {
  const res = await fetch(buildGatewayUrl('/v1/responses'), {
    method: 'POST',
    headers: gatewayHeaders(opts.apiKey, opts.groupId, true),
    body: JSON.stringify({
      model: opts.model,
      input: opts.input ?? (opts.messages ? chatMessagesToResponsesInput(opts.messages as ChatMessagePayload[]) : ''),
      stream: true,
      ...(opts.instructions ? { instructions: opts.instructions } : {}),
      ...(opts.temperature != null ? { temperature: opts.temperature } : {}),
      ...(opts.topP != null ? { top_p: opts.topP } : {}),
      ...(opts.maxOutputTokens != null && opts.maxOutputTokens > 0 ? { max_output_tokens: opts.maxOutputTokens } : {}),
      ...(opts.reasoningEffort ? { reasoning: { effort: opts.reasoningEffort } } : {}),
      ...(opts.tools?.length ? { tools: opts.tools } : {})
    }),
    signal: opts.signal
  })
  if (!res.ok || !res.body) throw new Error(await extractError(res))

  const activities = new Map<string, PlaygroundToolActivity>()
  await consumeSSE(res.body, (event) => {
    opts.onEvent?.(event)
    const json = parseEventJSON(event)
    if (!json) return
    const eventType = String(json.type ?? event.event ?? '')
    if (eventType === 'response.output_text.delta' && typeof json.delta === 'string') opts.onDelta(json.delta)

    if (eventType === 'response.output_item.added' && json.item) {
      const activity = activityFromItem(json.item, 'running')
      if (activity) {
        activities.set(activity.id, activity)
        opts.onToolActivity?.({ ...activity })
      }
    }
    if (eventType.includes('arguments.delta') || eventType.includes('code.delta')) {
      const id = String(json.item_id ?? json.call_id ?? '')
      const activity = activities.get(id)
      if (activity && typeof json.delta === 'string') {
        activity.input = `${typeof activity.input === 'string' ? activity.input : ''}${json.delta}`
        opts.onToolActivity?.({ ...activity })
      }
    }
    if ((eventType === 'response.output_item.done' || eventType.endsWith('.completed') || eventType.endsWith('.failed')) && (json.item || json.item_id || json.call_id)) {
      const item = (json.item && typeof json.item === 'object') ? json.item as Record<string, any> : {}
      const id = String(item.id ?? json.item_id ?? json.call_id ?? '')
      const previous = activities.get(id)
      const failed = eventType.endsWith('.failed') || !!item.error || !!json.error
      const activity = activityFromItem({ ...previous, ...item, id }, failed ? 'failed' : 'completed')
      if (activity) {
        activities.set(id, activity)
        opts.onToolActivity?.({ ...activity })
      }
    }

    const response = json.response && typeof json.response === 'object' ? json.response as Record<string, unknown> : null
    const usage = normalizeUsage(response?.usage ?? json.usage)
    if (usage) opts.onUsage?.(usage)
    if ((eventType === 'response.completed' || eventType === 'response.failed' || eventType === 'response.incomplete') && response) {
      opts.onCompleted?.({
        id: typeof response.id === 'string' ? response.id : undefined,
        status: typeof response.status === 'string' ? response.status : undefined,
        model: typeof response.model === 'string' ? response.model : undefined,
        usage,
        response
      })
    }
  })
}

export function imageQualityOptions(model: string): PlaygroundImageQuality[] {
  const normalized = model.trim().toLowerCase()
  if (normalized.includes('gpt-image')) return ['auto', 'low', 'medium', 'high']
  if (/dall[-_.\s]?e/.test(normalized)) return ['standard', 'hd']
  return []
}

export function normalizeImageQuality(
  model: string,
  quality: PlaygroundImageQuality | undefined
): PlaygroundImageQuality | undefined {
  if (!quality) return undefined
  return imageQualityOptions(model).includes(quality) ? quality : undefined
}

/** Generate or edit images via the OpenAI-compatible images endpoints. */
export async function generateImage(opts: ImageGenerationOptions): Promise<GeneratedImage[]> {
  const images = (opts.images ?? []).filter((image) => image instanceof File)
  const quality = normalizeImageQuality(opts.model, opts.quality)
  const count = opts.n && opts.n > 0 ? opts.n : 1
  const headers = gatewayHeaders(opts.apiKey, opts.groupId, images.length === 0)
  let body: BodyInit
  let endpoint = '/v1/images/generations'

  if (images.length > 0) {
    endpoint = '/v1/images/edits'
    const form = new FormData()
    form.append('model', opts.model)
    form.append('prompt', opts.prompt)
    form.append('n', String(count))
    form.append('response_format', 'b64_json')
    if (opts.size) form.append('size', opts.size)
    if (quality) form.append('quality', quality)
    for (const image of images) form.append('image[]', image, image.name)
    body = form
  } else {
    body = JSON.stringify({
      model: opts.model,
      prompt: opts.prompt,
      ...(opts.size ? { size: opts.size } : {}),
      ...(quality ? { quality } : {}),
      n: count,
      response_format: 'b64_json'
    })
  }

  const res = await fetch(buildGatewayUrl(endpoint), {
    method: 'POST',
    headers,
    body,
    signal: opts.signal
  })
  if (!res.ok) throw new Error(await extractError(res))
  const json = await res.json()
  return (Array.isArray(json?.data) ? json.data : []) as GeneratedImage[]
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

export interface VideoSubmitResult { request_id: string; status?: string; model?: string }
export interface VideoTaskStatus {
  request_id: string
  status: 'pending' | 'processing' | 'completed' | 'failed' | string
  url?: string
  video_url?: string
  error?: string
  model?: string
  usage?: { total_tokens?: number }
}

export async function generateVideo(opts: VideoGenerationOptions): Promise<VideoSubmitResult> {
  const res = await fetch(buildGatewayUrl('/v1/videos/generations'), {
    method: 'POST', headers: gatewayHeaders(opts.apiKey, opts.groupId, true),
    body: JSON.stringify({
      model: opts.model, prompt: opts.prompt,
      ...(opts.seconds && opts.seconds > 0 ? { seconds: opts.seconds } : {}),
      ...(opts.resolution ? { resolution: opts.resolution } : {}),
      ...(opts.ratio ? { ratio: opts.ratio } : {})
    }), signal: opts.signal
  })
  if (!res.ok) throw new Error(await extractError(res))
  const json = await res.json()
  const requestId = String(json?.request_id ?? json?.id ?? '')
  if (!requestId) throw new Error('Video task id missing in response')
  return { request_id: requestId, status: json?.status, model: json?.model }
}

export async function getVideoStatus(apiKey: string, groupId: number, requestId: string, signal?: AbortSignal): Promise<VideoTaskStatus> {
  const res = await fetch(buildGatewayUrl(`/v1/videos/${encodeURIComponent(requestId)}`), {
    method: 'GET', headers: gatewayHeaders(apiKey, groupId), signal
  })
  if (!res.ok) throw new Error(await extractError(res))
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

async function extractError(res: Response): Promise<string> {
  try {
    const json = await res.json()
    const message = json?.error?.message || json?.error?.type || json?.message || (typeof json?.error === 'string' ? json.error : '')
    if (message) return `${res.status} · ${message}`
  } catch {
    // fall through
  }
  return `${res.status} ${res.statusText || 'Request failed'}`
}

export const playgroundAPI = {
  listModelOptions,
  fetchPlaygroundURL,
  fetchWebContent,
  listModels,
  streamChat,
  streamResponses,
  generateImage,
  generateVideo,
  getVideoStatus
}

export default playgroundAPI

import { beforeEach, describe, expect, it, vi } from 'vitest'

const getMock = vi.fn()
const postMock = vi.fn()
vi.mock('@/api/client', () => ({
  apiClient: {
    get: (...args: unknown[]) => getMock(...args),
    post: (...args: unknown[]) => postMock(...args)
  }
}))

import playgroundAPI, { chatMessagesToResponsesInput } from '@/api/playground'

function streamResponse(chunks: string[]): Response {
  const encoder = new TextEncoder()
  return new Response(new ReadableStream({
    start(controller) {
      chunks.forEach((chunk) => controller.enqueue(encoder.encode(chunk)))
      controller.close()
    }
  }), { status: 200, headers: { 'Content-Type': 'text/event-stream' } })
}

describe('playground API group routing', () => {
  beforeEach(() => {
    getMock.mockReset()
    postMock.mockReset()
    vi.restoreAllMocks()
  })

  it('loads credential-free model options with backend feature objects', async () => {
    const options = [{
      group_id: 3, group_name: 'g', group_priority: 0, model: 'm', platform: 'openai', capabilities: ['chat'],
      features: { image_input: true, responses: true, web_search: false, code_execution: false, web_fetch: true }
    }]
    getMock.mockResolvedValue({ data: options })
    await expect(playgroundAPI.listModelOptions(9)).resolves.toEqual(options)
    expect(getMock).toHaveBeenCalledWith('/playground/api-keys/9/model-options', { signal: undefined })
  })

  it('calls the JWT fetch-url API without forwarding API key material', async () => {
    postMock.mockResolvedValue({ data: { results: [{ url: 'https://example.com', status_code: 200, content_type: 'text/plain', content: 'page' }] } })
    const result = await playgroundAPI.fetchWebContent({ urls: ['https://example.com'], apiKey: 'secret', groupId: 4 })
    expect(result).toEqual([{ url: 'https://example.com', content: 'page', statusCode: 200, contentType: 'text/plain' }])
    expect(postMock).toHaveBeenCalledWith('/playground/fetch-url', { urls: ['https://example.com'] }, { signal: undefined })
    expect(JSON.stringify(postMock.mock.calls)).not.toContain('secret')
  })

  it('converts chat image parts to Responses input parts', () => {
    expect(chatMessagesToResponsesInput([{
      role: 'user',
      content: [
        { type: 'text', text: 'inspect' },
        { type: 'image_url', image_url: { url: 'data:image/png;base64,abc', detail: 'low' } }
      ]
    }])).toEqual([{
      type: 'message',
      role: 'user',
      content: [
        { type: 'input_text', text: 'inspect' },
        { type: 'input_image', image_url: 'data:image/png;base64,abc', detail: 'low' }
      ]
    }])
  })

  it('attaches the selected group to gateway requests', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')
    fetchMock
      .mockResolvedValueOnce(streamResponse(['data: [DONE]\n\n']))
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: [{ url: 'image' }] }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ request_id: 'task-1' }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ request_id: 'task-1', status: 'completed', url: 'video' }), { status: 200 }))

    await playgroundAPI.streamChat({ apiKey: 'secret-key', groupId: 42, model: 'chat-model', messages: [], onDelta: () => undefined })
    await playgroundAPI.generateImage({ apiKey: 'secret-key', groupId: 42, model: 'image-model', prompt: 'draw' })
    await playgroundAPI.generateVideo({ apiKey: 'secret-key', groupId: 42, model: 'video-model', prompt: 'move' })
    await playgroundAPI.getVideoStatus('secret-key', 42, 'task-1')

    for (const call of fetchMock.mock.calls) {
      const headers = new Headers((call[1] as RequestInit).headers)
      expect(headers.get('X-Sub2API-Group-ID')).toBe('42')
      expect(headers.get('Authorization')).toBe('Bearer secret-key')
    }
  })
})

describe('playground image requests', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('uses JSON generations with b64_json and the selected group', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ data: [{ b64_json: 'aW1hZ2U=' }] }), { status: 200 })
    )

    await playgroundAPI.generateImage({
      apiKey: 'image-key',
      groupId: 7,
      model: 'gpt-image-1',
      prompt: 'sharp editorial poster',
      size: '1024x1536',
      quality: 'high',
      n: 2
    })

    const [url, init] = fetchMock.mock.calls[0]
    expect(String(url)).toContain('/v1/images/generations')
    const headers = new Headers((init as RequestInit).headers)
    expect(headers.get('Authorization')).toBe('Bearer image-key')
    expect(headers.get('X-Sub2API-Group-ID')).toBe('7')
    expect(headers.get('Content-Type')).toBe('application/json')
    expect(JSON.parse(String((init as RequestInit).body))).toEqual({
      model: 'gpt-image-1',
      prompt: 'sharp editorial poster',
      size: '1024x1536',
      quality: 'high',
      n: 2,
      response_format: 'b64_json'
    })
  })

  it('uses multipart edits for reference images without overriding the boundary header', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ data: [{ b64_json: 'aW1hZ2U=' }] }), { status: 200 })
    )
    const first = new File(['one'], 'one.png', { type: 'image/png' })
    const second = new File(['two'], 'two.webp', { type: 'image/webp' })

    await playgroundAPI.generateImage({
      apiKey: 'edit-key',
      groupId: 9,
      model: 'dall-e-3',
      prompt: 'edit this',
      size: '1024x1024',
      quality: 'hd',
      n: 1,
      images: [first, second]
    })

    const [url, init] = fetchMock.mock.calls[0]
    expect(String(url)).toContain('/v1/images/edits')
    const headers = new Headers((init as RequestInit).headers)
    expect(headers.get('Authorization')).toBe('Bearer edit-key')
    expect(headers.get('X-Sub2API-Group-ID')).toBe('9')
    expect(headers.get('Content-Type')).toBeNull()
    const body = (init as RequestInit).body as FormData
    expect(body).toBeInstanceOf(FormData)
    expect(body.get('model')).toBe('dall-e-3')
    expect(body.get('prompt')).toBe('edit this')
    expect(body.get('size')).toBe('1024x1024')
    expect(body.get('quality')).toBe('hd')
    expect(body.get('n')).toBe('1')
    expect(body.get('response_format')).toBe('b64_json')
    expect(body.getAll('image[]')).toEqual([first, second])
  })

  it('omits unsupported quality values from the payload', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ data: [] }), { status: 200 })
    )

    await playgroundAPI.generateImage({
      apiKey: 'key',
      groupId: 1,
      model: 'custom-image-model',
      prompt: 'draw',
      quality: 'high'
    })

    const body = JSON.parse(String((fetchMock.mock.calls[0][1] as RequestInit).body))
    expect(body).not.toHaveProperty('quality')
    expect(body.response_format).toBe('b64_json')
  })
})

describe('playground SSE parsing', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('handles CRLF, cross-chunk events, usage, and fragmented chat tool calls', async () => {
    const deltas: string[] = []
    const completed: unknown[] = []
    let usage: unknown
    const payload = [
      'data: {"choices":[{"delta":{"content":"hel"}}]}\r',
      '\n\r\ndata: {"choices":[{"delta":{"content":"lo","tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"look","arguments":"{\\"q\\":"}}]}}]}\r\n\r\n',
      'data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"up","arguments":"\\"x\\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"total_tokens":7}}\r\n\r\n',
      'data: [DONE]\r\n\r\n'
    ]
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(streamResponse(payload))

    await playgroundAPI.streamChat({
      apiKey: 'k', groupId: 1, model: 'm', messages: [],
      onDelta: (delta) => deltas.push(delta),
      onUsage: (value) => { usage = value },
      onToolCallComplete: (call) => completed.push(call)
    })
    expect(deltas.join('')).toBe('hello')
    expect(usage).toEqual({ total_tokens: 7 })
    expect(completed).toEqual([expect.objectContaining({
      id: 'call-1', function: { name: 'lookup', arguments: '{"q":"x"}' }
    })])
  })

  it('normalizes Responses output text, usage, and tool completion events', async () => {
    const deltas: string[] = []
    const activities: Array<{ id: string; status: string }> = []
    let usage: unknown
    let completed: unknown
    const events = [
      'event: response.output_item.added\ndata: {"type":"response.output_item.added","item":{"id":"ws-1","type":"web_search_call"}}\n\n',
      'event: response.output_text.delta\ndata: {"type":"response.output_text.delta","delta":"answer"}\n\n',
      'event: response.output_item.done\ndata: {"type":"response.output_item.done","item":{"id":"ws-1","type":"web_search_call","results":["ok"]}}\n\n',
      'event: response.completed\ndata: {"type":"response.completed","response":{"id":"r-1","status":"completed","model":"m","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}\n\n'
    ]
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(streamResponse(events))

    await playgroundAPI.streamResponses({
      apiKey: 'k', groupId: 2, model: 'm', input: 'question', onDelta: (delta) => deltas.push(delta),
      onUsage: (value) => { usage = value },
      onToolActivity: (activity) => activities.push({ id: activity.id, status: activity.status }),
      onCompleted: (value) => { completed = value }
    })
    expect(deltas).toEqual(['answer'])
    expect(activities).toEqual([{ id: 'ws-1', status: 'running' }, { id: 'ws-1', status: 'completed' }])
    expect(usage).toEqual({ input_tokens: 2, output_tokens: 3, total_tokens: 5 })
    expect(completed).toEqual(expect.objectContaining({ id: 'r-1', status: 'completed' }))
  })
})

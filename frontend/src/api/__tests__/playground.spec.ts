import { beforeEach, describe, expect, it, vi } from 'vitest'

const getMock = vi.fn()
vi.mock('@/api/client', () => ({
  apiClient: { get: (...args: unknown[]) => getMock(...args) }
}))

import playgroundAPI from '@/api/playground'

describe('playground API group routing', () => {
  beforeEach(() => {
    getMock.mockReset()
    vi.restoreAllMocks()
  })

  it('loads credential-free model options through the JWT API', async () => {
    const options = [{ group_id: 3, group_name: 'g', group_priority: 0, model: 'm', platform: 'openai', capabilities: ['chat'] }]
    getMock.mockResolvedValue({ data: options })
    const controller = new AbortController()

    await expect(playgroundAPI.listModelOptions(9, controller.signal)).resolves.toEqual(options)
    expect(getMock).toHaveBeenCalledWith('/playground/api-keys/9/model-options', { signal: controller.signal })
    expect(JSON.stringify(getMock.mock.calls)).not.toContain('sk-')
  })

  it('attaches the selected group to chat, image, video submit and video polling', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')
    fetchMock
      .mockResolvedValueOnce(new Response('data: [DONE]\n\n', { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: [{ url: 'image' }] }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ request_id: 'task-1' }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ request_id: 'task-1', status: 'completed', url: 'video' }), { status: 200 }))

    await playgroundAPI.streamChat({
      apiKey: 'secret-key', groupId: 42, model: 'chat-model', messages: [], onDelta: () => undefined
    })
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

import { nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  exportPlaygroundConversationJSON,
  exportPlaygroundConversationMarkdown,
  migratePlaygroundConversations,
  PLAYGROUND_CONVERSATIONS_STORAGE_KEY,
  resetPlaygroundConversationsForTest,
  setPlaygroundAttachmentCleanupHook,
  usePlaygroundConversations
} from '@/composables/usePlaygroundConversations'
import type { PlaygroundConversation } from '@/types/playground'

function conversation(id: string, updatedAt: number): PlaygroundConversation {
  return {
    id,
    title: id,
    messages: [{ id: `${id}-m`, role: 'user', content: 'hello' }],
    createdAt: updatedAt - 1,
    updatedAt
  }
}

describe('playground conversations persistence', () => {
  afterEach(() => {
    vi.useRealTimers()
    localStorage.clear()
    resetPlaygroundConversationsForTest()
  })

  it('migrates legacy arrays, sanitizes records, and sorts by update time', () => {
    const migrated = migratePlaygroundConversations(JSON.stringify([
      conversation('old', 10),
      { ...conversation('new', 20), apiKey: 'secret' },
      { nope: true }
    ]))
    expect(migrated.schemaVersion).toBe(2)
    expect(migrated.conversations.map((item) => item.id)).toEqual(['new', 'old'])
    expect(migrated.activeId).toBe('new')
    expect(JSON.stringify(migrated)).not.toContain('secret')
  })

  it('persists activeId and metadata without inline attachment payloads', async () => {
    const store = usePlaygroundConversations()
    const first = store.create()
    first.messages.push({
      id: 'message', role: 'user', content: 'with file',
      attachments: [{
        id: 'attachment', name: 'secret.txt', type: 'text/plain', size: 6, status: 'ready', text: 'secret'
      }]
    })
    const second = store.create()
    store.activeId.value = first.id
    await nextTick()

    const persisted = localStorage.getItem(PLAYGROUND_CONVERSATIONS_STORAGE_KEY) || ''
    const parsed = JSON.parse(persisted)
    expect(parsed.activeId).toBe(first.id)
    expect(parsed.schemaVersion).toBe(2)
    expect(persisted).not.toContain('"text":"secret"')
    expect(parsed.conversations.some((item: PlaygroundConversation) => item.id === second.id)).toBe(true)
  })

  it('supports delayed deletion, undo, and orphan cleanup on finalization', async () => {
    vi.useFakeTimers()
    const cleanup = vi.fn()
    setPlaygroundAttachmentCleanupHook(cleanup)
    const store = usePlaygroundConversations()
    const item = store.create()
    item.messages.push({
      id: 'm', role: 'user', content: '',
      attachments: [{ id: 'a', name: 'a.txt', size: 1, kind: 'text', mimeType: 'text/plain', createdAt: 1 }]
    })

    expect(store.scheduleRemove(item.id, 1000)?.id).toBe(item.id)
    expect(store.activeConversation()).toBeNull()
    expect(store.undoRemove(item.id)).toBe(true)
    expect(cleanup).not.toHaveBeenCalled()

    store.scheduleRemove(item.id, 1000)
    await vi.advanceTimersByTimeAsync(1000)
    expect(cleanup).toHaveBeenCalledWith(['a'])
  })
})

describe('playground conversation export', () => {
  const source: PlaygroundConversation = {
    id: 'c', title: 'Export', apiKeyId: 9, createdAt: 1, updatedAt: 2,
    messages: [{
      id: 'm', role: 'user', content: 'hello',
      attachments: [{ id: 'a', name: 'a.txt', size: 5, kind: 'text', mimeType: 'text/plain', createdAt: 1 }]
    }]
  }

  it('exports safe JSON and optionally embeds attachment payloads', async () => {
    const json = await exportPlaygroundConversationJSON(source, {
      inlineAttachments: true,
      resolveAttachment: async () => ({ id: 'a', encoding: 'text', data: 'hello' })
    })
    expect(JSON.parse(json).attachmentPayloads).toBeUndefined()
    expect(JSON.parse(json).conversation.attachmentPayloads.a.data).toBe('hello')
    expect(json).not.toContain('apiKey"')
  })

  it('exports readable Markdown with inline text attachments', async () => {
    const markdown = await exportPlaygroundConversationMarkdown(source, {
      inlineAttachments: true,
      resolveAttachment: () => ({ id: 'a', encoding: 'text', data: 'hello' })
    })
    expect(markdown).toContain('# Export')
    expect(markdown).toContain('## User')
    expect(markdown).toContain('### a.txt')
    expect(markdown).toContain('hello')
  })
})

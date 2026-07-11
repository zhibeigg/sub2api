/**
 * Local persistence for playground chat conversations.
 *
 * Stores only conversation text + settings in localStorage — never API keys.
 * A single shared reactive store is created lazily so all playground panels
 * observe the same conversation list.
 */

import { ref, watch, type Ref } from 'vue'
import type { ChatUsage } from '@/api/playground'
import type { PlaygroundModelOption } from '@/types/playground'

const STORAGE_KEY = 'playground_conversations_v1'
const MAX_CONVERSATIONS = 50

export interface PlaygroundMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  error?: boolean
  usage?: ChatUsage
  latencyMs?: number
  model?: string
  option?: PlaygroundModelOption
}

export interface PlaygroundConversation {
  id: string
  title: string
  messages: PlaygroundMessage[]
  apiKeyId?: number
  model?: string
  option?: PlaygroundModelOption
  createdAt: number
  updatedAt: number
}

export function uid(): string {
  return Date.now().toString(36) + Math.random().toString(36).slice(2, 8)
}

interface ConversationsStore {
  conversations: Ref<PlaygroundConversation[]>
  activeId: Ref<string | null>
  activeConversation: () => PlaygroundConversation | null
  create: (option?: PlaygroundModelOption, apiKeyId?: number) => PlaygroundConversation
  remove: (id: string) => void
  rename: (id: string, title: string) => void
  touch: (id: string) => void
  clearAll: () => void
}

let store: ConversationsStore | null = null

function load(): PlaygroundConversation[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed
  } catch {
    return []
  }
}

export function usePlaygroundConversations(): ConversationsStore {
  if (store) return store

  const conversations = ref<PlaygroundConversation[]>(load())
  const activeId = ref<string | null>(conversations.value[0]?.id ?? null)

  watch(
    conversations,
    (list) => {
      try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(list.slice(0, MAX_CONVERSATIONS)))
      } catch {
        // storage full / disabled — ignore
      }
    },
    { deep: true }
  )

  function activeConversation(): PlaygroundConversation | null {
    return conversations.value.find((c) => c.id === activeId.value) ?? null
  }

  function create(option?: PlaygroundModelOption, apiKeyId?: number): PlaygroundConversation {
    const conv: PlaygroundConversation = {
      id: uid(),
      title: '',
      messages: [],
      apiKeyId,
      model: option?.model,
      option,
      createdAt: Date.now(),
      updatedAt: Date.now()
    }
    conversations.value.unshift(conv)
    if (conversations.value.length > MAX_CONVERSATIONS) {
      conversations.value = conversations.value.slice(0, MAX_CONVERSATIONS)
    }
    activeId.value = conv.id
    return conv
  }

  function remove(id: string): void {
    const idx = conversations.value.findIndex((c) => c.id === id)
    if (idx === -1) return
    conversations.value.splice(idx, 1)
    if (activeId.value === id) {
      activeId.value = conversations.value[0]?.id ?? null
    }
  }

  function rename(id: string, title: string): void {
    const conv = conversations.value.find((c) => c.id === id)
    if (conv) {
      conv.title = title
      conv.updatedAt = Date.now()
    }
  }

  function touch(id: string): void {
    const conv = conversations.value.find((c) => c.id === id)
    if (conv) conv.updatedAt = Date.now()
  }

  function clearAll(): void {
    conversations.value = []
    activeId.value = null
  }

  store = {
    conversations,
    activeId,
    activeConversation,
    create,
    remove,
    rename,
    touch,
    clearAll
  }
  return store
}

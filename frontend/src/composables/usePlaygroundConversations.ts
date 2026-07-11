/** Local, credential-free persistence for playground conversations. */
import { ref, watch, type Ref } from 'vue'
import { deletePlaygroundAttachments } from '@/utils/playgroundAttachments'
import { sanitizePlaygroundOption } from '@/composables/usePlaygroundSettings'
import type {
  PlaygroundAttachment,
  PlaygroundAttachmentPayload,
  PlaygroundContentPart,
  PlaygroundConversation,
  PlaygroundMessage,
  PlaygroundModelOption,
  PlaygroundToolActivity,
  PlaygroundTokenUsage
} from '@/types/playground'

export type { PlaygroundConversation, PlaygroundMessage } from '@/types/playground'

export const PLAYGROUND_CONVERSATIONS_STORAGE_KEY = 'playground_conversations_v2'
export const PLAYGROUND_CONVERSATIONS_LEGACY_STORAGE_KEY = 'playground_conversations_v1'
export const PLAYGROUND_CONVERSATIONS_SCHEMA_VERSION = 2
const MAX_CONVERSATIONS = 50

export interface PersistedConversationsV2 {
  schemaVersion: 2
  activeId: string | null
  conversations: PlaygroundConversation[]
}

export interface PendingConversationDeletion {
  id: string
  expiresAt: number
}

export type PlaygroundAttachmentCleanupHook = (orphanedAttachmentIds: string[]) => void | Promise<void>

export interface ConversationExportOptions {
  inlineAttachments?: boolean
  resolveAttachment?: (
    attachment: PlaygroundAttachment
  ) => PlaygroundAttachmentPayload | null | Promise<PlaygroundAttachmentPayload | null>
}

export interface PlaygroundConversationsStore {
  conversations: Ref<PlaygroundConversation[]>
  activeId: Ref<string | null>
  activeConversation: () => PlaygroundConversation | null
  create: (option?: PlaygroundModelOption, apiKeyId?: number) => PlaygroundConversation
  remove: (id: string) => PlaygroundConversation | null
  scheduleRemove: (id: string, delayMs?: number) => PendingConversationDeletion | null
  undoRemove: (id: string) => boolean
  finalizeRemove: (id: string) => boolean
  rename: (id: string, title: string) => void
  touch: (id: string) => void
  clearAll: () => void
  exportMarkdown: (id: string, options?: ConversationExportOptions) => Promise<string | null>
  exportJSON: (id: string, options?: ConversationExportOptions) => Promise<string | null>
}

interface PendingRecord {
  conversation: PlaygroundConversation
  index: number
  timer: ReturnType<typeof setTimeout>
  expiresAt: number
}

let store: PlaygroundConversationsStore | null = null
let attachmentCleanupHook: PlaygroundAttachmentCleanupHook = deletePlaygroundAttachments
let pendingRecords = new Map<string, PendingRecord>()

export function setPlaygroundAttachmentCleanupHook(hook?: PlaygroundAttachmentCleanupHook): void {
  attachmentCleanupHook = hook ?? deletePlaygroundAttachments
}

export function uid(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') return crypto.randomUUID()
  return Date.now().toString(36) + Math.random().toString(36).slice(2, 8)
}

function stringValue(value: unknown, fallback = ''): string {
  return typeof value === 'string' ? value : fallback
}

function positiveTimestamp(value: unknown, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : fallback
}

function sanitizeUsage(value: unknown): PlaygroundTokenUsage | undefined {
  if (!value || typeof value !== 'object') return undefined
  const result: PlaygroundTokenUsage = {}
  for (const key of ['prompt_tokens', 'completion_tokens', 'total_tokens', 'input_tokens', 'output_tokens'] as const) {
    const candidate = (value as Record<string, unknown>)[key]
    if (typeof candidate === 'number' && Number.isFinite(candidate) && candidate >= 0) result[key] = candidate
  }
  return Object.keys(result).length ? result : undefined
}

function sanitizeAttachment(value: unknown): PlaygroundAttachment | null {
  if (!value || typeof value !== 'object') return null
  const item = value as Partial<PlaygroundAttachment>
  if (typeof item.id !== 'string' || !item.id || typeof item.name !== 'string') return null
  const mimeType = stringValue(item.mimeType || item.type)
  const kind = item.kind === 'image' || item.kind === 'text'
    ? item.kind
    : mimeType.startsWith('image/') ? 'image' : 'text'
  return {
    id: item.id,
    name: item.name,
    mimeType,
    type: typeof item.type === 'string' ? item.type : undefined,
    size: typeof item.size === 'number' && item.size >= 0 ? item.size : 0,
    kind,
    createdAt: positiveTimestamp(item.createdAt, Date.now()),
    status: item.status === 'reading' || item.status === 'ready' || item.status === 'error' || item.status === 'missing' ? item.status : undefined,
    error: typeof item.error === 'string' ? item.error : undefined
  }
}

function sanitizeContentParts(value: unknown): PlaygroundContentPart[] | undefined {
  if (!Array.isArray(value)) return undefined
  const parts = value.filter((part): part is PlaygroundContentPart => {
    if (!part || typeof part !== 'object') return false
    const type = (part as { type?: unknown }).type
    if (type === 'text') return typeof (part as { text?: unknown }).text === 'string'
    if (type === 'image_url') return typeof (part as { image_url?: unknown }).image_url === 'string' || !!(part as { image_url?: unknown }).image_url
    if (type === 'file') return !!(part as { file?: unknown }).file
    return false
  })
  return parts.length ? parts : undefined
}

function sanitizeToolActivities(value: unknown): PlaygroundToolActivity[] | undefined {
  if (!Array.isArray(value)) return undefined
  const activities = value.flatMap((raw): PlaygroundToolActivity[] => {
    if (!raw || typeof raw !== 'object') return []
    const item = raw as Partial<PlaygroundToolActivity>
    if (typeof item.id !== 'string' || (typeof item.type !== 'string' && typeof item.kind !== 'string')) return []
    const status = item.status === 'pending' || item.status === 'running' || item.status === 'completed' || item.status === 'failed' || item.status === 'done' || item.status === 'error'
      ? item.status
      : 'pending'
    return [{
      id: item.id,
      type: typeof item.type === 'string' ? item.type : undefined,
      kind: item.kind,
      name: typeof item.name === 'string' ? item.name : undefined,
      label: typeof item.label === 'string' ? item.label : undefined,
      status,
      input: item.input,
      output: item.output,
      error: typeof item.error === 'string' ? item.error : undefined,
      startedAt: typeof item.startedAt === 'number' ? item.startedAt : undefined,
      completedAt: typeof item.completedAt === 'number' ? item.completedAt : undefined
    }]
  })
  return activities.length ? activities : undefined
}

function sanitizeMessage(value: unknown): PlaygroundMessage | null {
  if (!value || typeof value !== 'object') return null
  const item = value as Partial<PlaygroundMessage>
  if (typeof item.id !== 'string' || (item.role !== 'user' && item.role !== 'assistant')) return null
  const attachments = Array.isArray(item.attachments)
    ? item.attachments.map(sanitizeAttachment).filter((attachment): attachment is PlaygroundAttachment => !!attachment)
    : undefined
  return {
    id: item.id,
    role: item.role,
    content: stringValue(item.content),
    contentParts: sanitizeContentParts(item.contentParts),
    attachments: attachments?.length ? attachments : undefined,
    toolActivities: sanitizeToolActivities(item.toolActivities),
    error: item.error === true || undefined,
    usage: sanitizeUsage(item.usage),
    latencyMs: typeof item.latencyMs === 'number' && item.latencyMs >= 0 ? item.latencyMs : undefined,
    model: typeof item.model === 'string' ? item.model : undefined,
    option: sanitizePlaygroundOption(item.option) ?? undefined
  }
}

function sanitizeConversation(value: unknown, now = Date.now()): PlaygroundConversation | null {
  if (!value || typeof value !== 'object') return null
  const item = value as Partial<PlaygroundConversation>
  if (typeof item.id !== 'string' || !item.id) return null
  const createdAt = positiveTimestamp(item.createdAt, now)
  const messages = Array.isArray(item.messages)
    ? item.messages.map(sanitizeMessage).filter((message): message is PlaygroundMessage => !!message)
    : []
  return {
    id: item.id,
    title: stringValue(item.title),
    messages,
    apiKeyId: typeof item.apiKeyId === 'number' && Number.isInteger(item.apiKeyId) && item.apiKeyId > 0 ? item.apiKeyId : undefined,
    model: typeof item.model === 'string' ? item.model : undefined,
    option: sanitizePlaygroundOption(item.option) ?? undefined,
    createdAt,
    updatedAt: positiveTimestamp(item.updatedAt, createdAt)
  }
}

export function sortPlaygroundConversations(conversations: readonly PlaygroundConversation[]): PlaygroundConversation[] {
  return [...conversations].sort((left, right) => right.updatedAt - left.updatedAt || right.createdAt - left.createdAt || left.id.localeCompare(right.id))
}

export function migratePlaygroundConversations(raw: string | null): PersistedConversationsV2 {
  if (!raw) return { schemaVersion: 2, activeId: null, conversations: [] }
  try {
    const parsed = JSON.parse(raw)
    const source = Array.isArray(parsed) ? parsed : parsed?.conversations
    const conversations = sortPlaygroundConversations(
      (Array.isArray(source) ? source : [])
        .map((item) => sanitizeConversation(item))
        .filter((item): item is PlaygroundConversation => !!item)
        .slice(0, MAX_CONVERSATIONS)
    )
    const requestedActiveId = !Array.isArray(parsed) && typeof parsed?.activeId === 'string' ? parsed.activeId : null
    return {
      schemaVersion: 2,
      activeId: requestedActiveId && conversations.some((item) => item.id === requestedActiveId)
        ? requestedActiveId
        : conversations[0]?.id ?? null,
      conversations
    }
  } catch {
    return { schemaVersion: 2, activeId: null, conversations: [] }
  }
}

function attachmentIds(conversations: readonly PlaygroundConversation[]): Set<string> {
  return new Set(conversations.flatMap((conversation) => conversation.messages.flatMap((message) => message.attachments?.map((item) => item.id) ?? [])))
}

function runCleanup(removed: readonly PlaygroundConversation[], remaining: readonly PlaygroundConversation[]): void {
  const stillReferenced = attachmentIds(remaining)
  const orphaned = Array.from(attachmentIds(removed)).filter((id) => !stillReferenced.has(id))
  if (orphaned.length) void Promise.resolve(attachmentCleanupHook(orphaned)).catch(() => undefined)
}

function load(): PersistedConversationsV2 {
  try {
    const current = localStorage.getItem(PLAYGROUND_CONVERSATIONS_STORAGE_KEY)
    if (current) return migratePlaygroundConversations(current)
    return migratePlaygroundConversations(localStorage.getItem(PLAYGROUND_CONVERSATIONS_LEGACY_STORAGE_KEY))
  } catch {
    return migratePlaygroundConversations(null)
  }
}

function exportBase(conversation: PlaygroundConversation): Record<string, unknown> {
  return {
    id: conversation.id,
    title: conversation.title,
    messages: conversation.messages.map((message) => ({
      id: message.id,
      role: message.role,
      content: message.content,
      contentParts: message.contentParts,
      attachments: message.attachments?.map((attachment) => sanitizeAttachment(attachment)).filter(Boolean),
      toolActivities: message.toolActivities,
      error: message.error,
      usage: message.usage,
      latencyMs: message.latencyMs,
      model: message.model,
      option: message.option
    })),
    apiKeyId: conversation.apiKeyId,
    model: conversation.model,
    option: conversation.option,
    createdAt: conversation.createdAt,
    updatedAt: conversation.updatedAt
  }
}

async function inlineExportAttachments(
  conversation: PlaygroundConversation,
  options: ConversationExportOptions
): Promise<Map<string, PlaygroundAttachmentPayload>> {
  const payloads = new Map<string, PlaygroundAttachmentPayload>()
  if (!options.inlineAttachments || !options.resolveAttachment) return payloads
  for (const attachment of conversation.messages.flatMap((message) => message.attachments ?? [])) {
    if (payloads.has(attachment.id)) continue
    const payload = await options.resolveAttachment(attachment)
    if (payload) payloads.set(attachment.id, payload)
  }
  return payloads
}

/** Pure serializer: output is determined only by the conversation and explicit resolver option. */
export async function exportPlaygroundConversationJSON(
  conversation: PlaygroundConversation,
  options: ConversationExportOptions = {}
): Promise<string> {
  const data = exportBase(conversation)
  const payloads = await inlineExportAttachments(conversation, options)
  if (payloads.size) {
    data.attachmentPayloads = Object.fromEntries(Array.from(payloads, ([id, payload]) => [id, {
      encoding: payload.encoding,
      data: payload.data
    }]))
  }
  return JSON.stringify({ schemaVersion: PLAYGROUND_CONVERSATIONS_SCHEMA_VERSION, conversation: data }, null, 2)
}

function markdownFence(text: string): string {
  let fence = '```'
  while (text.includes(fence)) fence += '`'
  return fence
}

/** Pure serializer: API credentials are not part of its accepted conversation shape or output. */
export async function exportPlaygroundConversationMarkdown(
  conversation: PlaygroundConversation,
  options: ConversationExportOptions = {}
): Promise<string> {
  const payloads = await inlineExportAttachments(conversation, options)
  const lines = [`# ${conversation.title.trim() || 'Playground Conversation'}`, '']
  for (const message of conversation.messages) {
    lines.push(`## ${message.role === 'user' ? 'User' : 'Assistant'}`, '', message.content, '')
    for (const attachment of message.attachments ?? []) {
      const payload = payloads.get(attachment.id)
      if (payload?.encoding === 'data-url') lines.push(`![${attachment.name.replace(/]/g, '\\]')}](${payload.data})`, '')
      else if (payload?.encoding === 'text') {
        const fence = markdownFence(payload.data)
        lines.push(`### ${attachment.name}`, '', fence, payload.data, fence, '')
      } else lines.push(`- Attachment: ${attachment.name} (${attachment.mimeType || attachment.kind}, ${attachment.size} bytes)`, '')
    }
    for (const activity of message.toolActivities ?? []) {
      lines.push(`- Tool: ${activity.name || activity.type} — ${activity.status}`)
    }
    if (message.toolActivities?.length) lines.push('')
  }
  return `${lines.join('\n').trimEnd()}\n`
}

export function usePlaygroundConversations(): PlaygroundConversationsStore {
  if (store) return store

  const initial = load()
  const conversations = ref<PlaygroundConversation[]>(initial.conversations)
  const activeId = ref<string | null>(initial.activeId)

  const persist = () => {
    try {
      localStorage.setItem(PLAYGROUND_CONVERSATIONS_STORAGE_KEY, JSON.stringify({
        schemaVersion: PLAYGROUND_CONVERSATIONS_SCHEMA_VERSION,
        activeId: activeId.value,
        conversations: sortPlaygroundConversations(conversations.value)
          .map((conversation) => sanitizeConversation(conversation))
          .filter((conversation): conversation is PlaygroundConversation => !!conversation)
          .slice(0, MAX_CONVERSATIONS)
      } satisfies PersistedConversationsV2))
    } catch {
      // storage full / disabled — ignore
    }
  }

  watch([conversations, activeId], persist, { deep: true })
  persist()

  function activeConversation(): PlaygroundConversation | null {
    return conversations.value.find((conversation) => conversation.id === activeId.value) ?? null
  }

  function create(option?: PlaygroundModelOption, apiKeyId?: number): PlaygroundConversation {
    const now = Date.now()
    const conversation: PlaygroundConversation = {
      id: uid(), title: '', messages: [], apiKeyId, model: option?.model, option, createdAt: now, updatedAt: now
    }
    conversations.value.unshift(conversation)
    if (conversations.value.length > MAX_CONVERSATIONS) {
      const removed = conversations.value.splice(MAX_CONVERSATIONS)
      runCleanup(removed, conversations.value)
    }
    activeId.value = conversation.id
    return conversation
  }

  function detach(id: string): { conversation: PlaygroundConversation; index: number } | null {
    const index = conversations.value.findIndex((conversation) => conversation.id === id)
    if (index < 0) return null
    const [conversation] = conversations.value.splice(index, 1)
    if (activeId.value === id) activeId.value = sortPlaygroundConversations(conversations.value)[0]?.id ?? null
    return { conversation, index }
  }

  function remove(id: string): PlaygroundConversation | null {
    const pending = pendingRecords.get(id)
    if (pending) {
      clearTimeout(pending.timer)
      pendingRecords.delete(id)
      runCleanup([pending.conversation], conversations.value)
      return pending.conversation
    }
    const removed = detach(id)
    if (!removed) return null
    runCleanup([removed.conversation], conversations.value)
    return removed.conversation
  }

  function scheduleRemove(id: string, delayMs = 5000): PendingConversationDeletion | null {
    if (pendingRecords.has(id)) {
      const pending = pendingRecords.get(id)!
      return { id, expiresAt: pending.expiresAt }
    }
    const removed = detach(id)
    if (!removed) return null
    const expiresAt = Date.now() + Math.max(0, delayMs)
    const timer = setTimeout(() => finalizeRemove(id), Math.max(0, delayMs))
    pendingRecords.set(id, { ...removed, timer, expiresAt })
    return { id, expiresAt }
  }

  function undoRemove(id: string): boolean {
    const pending = pendingRecords.get(id)
    if (!pending) return false
    clearTimeout(pending.timer)
    pendingRecords.delete(id)
    conversations.value.splice(Math.min(pending.index, conversations.value.length), 0, pending.conversation)
    activeId.value = pending.conversation.id
    return true
  }

  function finalizeRemove(id: string): boolean {
    const pending = pendingRecords.get(id)
    if (!pending) return false
    clearTimeout(pending.timer)
    pendingRecords.delete(id)
    runCleanup([pending.conversation], conversations.value)
    return true
  }

  function rename(id: string, title: string): void {
    const conversation = conversations.value.find((item) => item.id === id)
    if (!conversation) return
    conversation.title = title
    conversation.updatedAt = Date.now()
    conversations.value = sortPlaygroundConversations(conversations.value)
  }

  function touch(id: string): void {
    const conversation = conversations.value.find((item) => item.id === id)
    if (!conversation) return
    conversation.updatedAt = Date.now()
    conversations.value = sortPlaygroundConversations(conversations.value)
  }

  function clearAll(): void {
    const removed = [...conversations.value, ...Array.from(pendingRecords.values(), (item) => item.conversation)]
    for (const pending of pendingRecords.values()) clearTimeout(pending.timer)
    pendingRecords.clear()
    conversations.value = []
    activeId.value = null
    runCleanup(removed, [])
  }

  async function exportMarkdown(id: string, options?: ConversationExportOptions): Promise<string | null> {
    const conversation = conversations.value.find((item) => item.id === id)
    return conversation ? exportPlaygroundConversationMarkdown(conversation, options) : null
  }

  async function exportJSON(id: string, options?: ConversationExportOptions): Promise<string | null> {
    const conversation = conversations.value.find((item) => item.id === id)
    return conversation ? exportPlaygroundConversationJSON(conversation, options) : null
  }

  store = {
    conversations, activeId, activeConversation, create, remove, scheduleRemove, undoRemove, finalizeRemove,
    rename, touch, clearAll, exportMarkdown, exportJSON
  }
  return store
}

export function resetPlaygroundConversationsForTest(): void {
  for (const pending of pendingRecords.values()) clearTimeout(pending.timer)
  pendingRecords = new Map()
  store = null
  attachmentCleanupHook = deletePlaygroundAttachments
}

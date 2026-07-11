import type {
  PlaygroundAttachment,
  PlaygroundAttachmentKind,
  PlaygroundAttachmentPayload
} from '@/types/playground'

export const PLAYGROUND_ATTACHMENT_MAX_COUNT = 4
export const PLAYGROUND_ATTACHMENT_MAX_IMAGE_BYTES = 5 * 1024 * 1024
export const PLAYGROUND_ATTACHMENT_MAX_TEXT_BYTES = 512 * 1024
export const PLAYGROUND_ATTACHMENT_MAX_TOTAL_BYTES = 12 * 1024 * 1024
export const PLAYGROUND_ATTACHMENT_METADATA_KEY = 'playground_attachment_metadata_v1'

const DB_NAME = 'sub2api-playground'
const DB_VERSION = 1
const STORE_NAME = 'attachment-payloads'
const IMAGE_MIME_TYPES = new Set(['image/png', 'image/jpeg', 'image/webp', 'image/gif'])
const TEXT_MIME_TYPES = new Set([
  'text/plain', 'text/markdown', 'text/csv', 'text/html', 'text/css', 'text/xml',
  'application/json', 'application/xml', 'application/javascript', 'application/typescript',
  'application/x-javascript', 'application/x-typescript', 'application/x-yaml', 'text/yaml',
  'application/toml', 'application/sql', 'application/x-sh', 'text/x-shellscript'
])
const TEXT_EXTENSIONS = new Set([
  'txt', 'md', 'markdown', 'csv', 'json', 'jsonl', 'xml', 'yaml', 'yml', 'toml', 'ini', 'cfg', 'conf',
  'log', 'html', 'htm', 'css', 'scss', 'sass', 'less', 'js', 'jsx', 'mjs', 'cjs', 'ts', 'tsx', 'vue',
  'py', 'pyi', 'java', 'kt', 'kts', 'go', 'rs', 'rb', 'php', 'swift', 'c', 'h', 'cc', 'cpp', 'hpp',
  'cs', 'sh', 'bash', 'zsh', 'fish', 'ps1', 'bat', 'sql', 'graphql', 'gql', 'proto', 'dockerfile'
])

export interface PlaygroundAttachmentPayloadStorage {
  put(payload: PlaygroundAttachmentPayload): Promise<void>
  get(id: string): Promise<PlaygroundAttachmentPayload | null>
  delete(id: string): Promise<void>
  deleteMany(ids: string[]): Promise<void>
  clear(): Promise<void>
}

export interface AddPlaygroundAttachmentsResult {
  attachments: PlaygroundAttachment[]
  allAttachments: PlaygroundAttachment[]
}

export class PlaygroundAttachmentValidationError extends Error {
  constructor(
    public readonly code: 'unsupported_type' | 'too_many' | 'file_too_large' | 'total_too_large',
    message: string,
    public readonly fileName?: string
  ) {
    super(message)
    this.name = 'PlaygroundAttachmentValidationError'
  }
}

export class PlaygroundAttachmentStorageUnavailableError extends Error {
  constructor(message = 'IndexedDB is unavailable') {
    super(message)
    this.name = 'PlaygroundAttachmentStorageUnavailableError'
  }
}

function requestResult<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error || new Error('IndexedDB request failed'))
  })
}

function transactionDone(transaction: IDBTransaction): Promise<void> {
  return new Promise((resolve, reject) => {
    transaction.oncomplete = () => resolve()
    transaction.onerror = () => reject(transaction.error || new Error('IndexedDB transaction failed'))
    transaction.onabort = () => reject(transaction.error || new Error('IndexedDB transaction aborted'))
  })
}

class IndexedDBAttachmentStorage implements PlaygroundAttachmentPayloadStorage {
  private dbPromise: Promise<IDBDatabase> | null = null

  private open(): Promise<IDBDatabase> {
    if (this.dbPromise) return this.dbPromise
    this.dbPromise = new Promise((resolve, reject) => {
      if (typeof indexedDB === 'undefined') {
        reject(new PlaygroundAttachmentStorageUnavailableError())
        return
      }
      let request: IDBOpenDBRequest
      try {
        request = indexedDB.open(DB_NAME, DB_VERSION)
      } catch {
        reject(new PlaygroundAttachmentStorageUnavailableError())
        return
      }
      request.onupgradeneeded = () => {
        const db = request.result
        if (!db.objectStoreNames.contains(STORE_NAME)) db.createObjectStore(STORE_NAME, { keyPath: 'id' })
      }
      request.onsuccess = () => resolve(request.result)
      request.onerror = () => reject(request.error || new PlaygroundAttachmentStorageUnavailableError())
      request.onblocked = () => reject(new PlaygroundAttachmentStorageUnavailableError('IndexedDB upgrade is blocked'))
    })
    return this.dbPromise
  }

  async put(payload: PlaygroundAttachmentPayload): Promise<void> {
    const db = await this.open()
    const tx = db.transaction(STORE_NAME, 'readwrite')
    tx.objectStore(STORE_NAME).put(payload)
    await transactionDone(tx)
  }

  async get(id: string): Promise<PlaygroundAttachmentPayload | null> {
    const db = await this.open()
    const tx = db.transaction(STORE_NAME, 'readonly')
    const value = await requestResult(tx.objectStore(STORE_NAME).get(id))
    return (value as PlaygroundAttachmentPayload | undefined) ?? null
  }

  async delete(id: string): Promise<void> {
    await this.deleteMany([id])
  }

  async deleteMany(ids: string[]): Promise<void> {
    if (ids.length === 0) return
    const db = await this.open()
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const objectStore = tx.objectStore(STORE_NAME)
    for (const id of new Set(ids)) objectStore.delete(id)
    await transactionDone(tx)
  }

  async clear(): Promise<void> {
    const db = await this.open()
    const tx = db.transaction(STORE_NAME, 'readwrite')
    tx.objectStore(STORE_NAME).clear()
    await transactionDone(tx)
  }
}

export function createMemoryPlaygroundAttachmentStorage(): PlaygroundAttachmentPayloadStorage {
  const values = new Map<string, PlaygroundAttachmentPayload>()
  return {
    async put(payload) { values.set(payload.id, { ...payload }) },
    async get(id) { return values.has(id) ? { ...values.get(id)! } : null },
    async delete(id) { values.delete(id) },
    async deleteMany(ids) { ids.forEach((id) => values.delete(id)) },
    async clear() { values.clear() }
  }
}

let payloadStorage: PlaygroundAttachmentPayloadStorage = new IndexedDBAttachmentStorage()

export function setPlaygroundAttachmentStorageForTest(storage: PlaygroundAttachmentPayloadStorage | null): void {
  payloadStorage = storage ?? new IndexedDBAttachmentStorage()
}

export function playgroundAttachmentId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') return crypto.randomUUID()
  return `att_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 10)}`
}

export function getPlaygroundAttachmentKind(file: Pick<File, 'name' | 'type'>): PlaygroundAttachmentKind | null {
  const mime = file.type.toLowerCase().split(';', 1)[0]
  if (IMAGE_MIME_TYPES.has(mime)) return 'image'
  const extension = file.name.toLowerCase().split('.').pop() || ''
  if (mime.startsWith('text/') || TEXT_MIME_TYPES.has(mime) || TEXT_EXTENSIONS.has(extension)) return 'text'
  return null
}

export function validatePlaygroundAttachments(
  files: readonly Pick<File, 'name' | 'type' | 'size'>[],
  existing: readonly PlaygroundAttachment[] = []
): PlaygroundAttachmentKind[] {
  if (existing.length + files.length > PLAYGROUND_ATTACHMENT_MAX_COUNT) {
    throw new PlaygroundAttachmentValidationError('too_many', `At most ${PLAYGROUND_ATTACHMENT_MAX_COUNT} attachments are allowed`)
  }
  const kinds = files.map((file) => {
    const kind = getPlaygroundAttachmentKind(file)
    if (!kind) throw new PlaygroundAttachmentValidationError('unsupported_type', `Unsupported attachment type: ${file.name}`, file.name)
    const limit = kind === 'image' ? PLAYGROUND_ATTACHMENT_MAX_IMAGE_BYTES : PLAYGROUND_ATTACHMENT_MAX_TEXT_BYTES
    if (file.size > limit) {
      throw new PlaygroundAttachmentValidationError('file_too_large', `Attachment is too large: ${file.name}`, file.name)
    }
    return kind
  })
  const total = existing.reduce((sum, item) => sum + item.size, 0) + files.reduce((sum, file) => sum + file.size, 0)
  if (total > PLAYGROUND_ATTACHMENT_MAX_TOTAL_BYTES) {
    throw new PlaygroundAttachmentValidationError('total_too_large', 'Total attachment payload exceeds 12 MB')
  }
  return kinds
}

function readFile(file: File, kind: PlaygroundAttachmentKind): Promise<PlaygroundAttachmentPayload> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(reader.error || new Error(`Failed to read ${file.name}`))
    reader.onload = () => resolve({
      id: '',
      encoding: kind === 'image' ? 'data-url' : 'text',
      data: String(reader.result ?? '')
    })
    if (kind === 'image') reader.readAsDataURL(file)
    else reader.readAsText(file)
  })
}

function sanitizeMetadata(value: unknown): PlaygroundAttachment | null {
  if (!value || typeof value !== 'object') return null
  const item = value as Partial<PlaygroundAttachment>
  if (typeof item.id !== 'string' || !item.id || typeof item.name !== 'string') return null
  if (item.kind !== 'image' && item.kind !== 'text') return null
  return {
    id: item.id,
    name: item.name,
    mimeType: typeof item.mimeType === 'string' ? item.mimeType : '',
    size: typeof item.size === 'number' && item.size >= 0 ? item.size : 0,
    kind: item.kind,
    createdAt: typeof item.createdAt === 'number' ? item.createdAt : Date.now()
  }
}

export function readPlaygroundAttachmentMetadata(): PlaygroundAttachment[] {
  try {
    const parsed = JSON.parse(localStorage.getItem(PLAYGROUND_ATTACHMENT_METADATA_KEY) || '[]')
    return Array.isArray(parsed) ? parsed.map(sanitizeMetadata).filter((item): item is PlaygroundAttachment => !!item) : []
  } catch {
    return []
  }
}

function persistMetadata(items: readonly PlaygroundAttachment[]): void {
  try {
    localStorage.setItem(PLAYGROUND_ATTACHMENT_METADATA_KEY, JSON.stringify(items))
  } catch {
    // Payload remains in IndexedDB; callers can still retain metadata in conversation state.
  }
}

export async function addPlaygroundAttachments(
  filesInput: Iterable<File>,
  existing: readonly PlaygroundAttachment[] = []
): Promise<AddPlaygroundAttachmentsResult> {
  const files = Array.from(filesInput)
  const kinds = validatePlaygroundAttachments(files, existing)
  const created: PlaygroundAttachment[] = []
  try {
    for (let index = 0; index < files.length; index += 1) {
      const file = files[index]
      const kind = kinds[index]
      const id = playgroundAttachmentId()
      const payload = await readFile(file, kind)
      payload.id = id
      await payloadStorage.put(payload)
      created.push({ id, name: file.name, mimeType: file.type || (kind === 'text' ? 'text/plain' : ''), size: file.size, kind, createdAt: Date.now() })
    }
  } catch (error) {
    await payloadStorage.deleteMany(created.map((item) => item.id)).catch(() => undefined)
    throw error
  }
  const allAttachments = [...existing, ...created]
  const catalog = readPlaygroundAttachmentMetadata().filter((item) => !created.some((newItem) => newItem.id === item.id))
  persistMetadata([...catalog, ...created])
  return { attachments: created, allAttachments }
}

export function readPlaygroundAttachment(id: string): Promise<PlaygroundAttachmentPayload | null> {
  return payloadStorage.get(id)
}

export async function deletePlaygroundAttachment(id: string): Promise<void> {
  await payloadStorage.delete(id)
  persistMetadata(readPlaygroundAttachmentMetadata().filter((item) => item.id !== id))
}

export async function deletePlaygroundAttachments(ids: Iterable<string>): Promise<void> {
  const uniqueIds = Array.from(new Set(ids))
  await payloadStorage.deleteMany(uniqueIds)
  const removed = new Set(uniqueIds)
  persistMetadata(readPlaygroundAttachmentMetadata().filter((item) => !removed.has(item.id)))
}

/** Delete payloads and metadata not present in the supplied live reference set. */
export async function cleanupPlaygroundAttachments(referencedIds: Iterable<string>): Promise<string[]> {
  const referenced = new Set(referencedIds)
  const metadata = readPlaygroundAttachmentMetadata()
  const staleIds = metadata.filter((item) => !referenced.has(item.id)).map((item) => item.id)
  await payloadStorage.deleteMany(staleIds)
  persistMetadata(metadata.filter((item) => referenced.has(item.id)))
  return staleIds
}

export async function clearPlaygroundAttachments(): Promise<void> {
  await payloadStorage.clear()
  persistMetadata([])
}

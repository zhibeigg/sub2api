export type PlaygroundCapability = 'chat' | 'image' | 'video'
export type PlaygroundMode = PlaygroundCapability | 'compare'
export type PlaygroundFeature =
  | 'attachments'
  | 'vision'
  | 'responses'
  | 'web_search'
  | 'code_execution'
  | 'web_fetch'
  | (string & {})
export interface PlaygroundModelFeatures {
  image_input?: boolean
  responses?: boolean
  web_search?: boolean
  code_execution?: boolean
  web_fetch?: boolean
  [key: string]: boolean | undefined
}
export type PlaygroundFeatures = PlaygroundModelFeatures | PlaygroundFeature[]

export interface PlaygroundModelOption {
  id?: string
  group_id: number
  group_name: string
  group_priority: number
  model: string
  platform: string
  capabilities: PlaygroundCapability[]
  /** Optional backend-advertised feature flags. Array and object forms are both accepted. */
  features?: PlaygroundFeatures
}

export type PlaygroundImageQuality = '' | 'auto' | 'low' | 'medium' | 'high' | 'standard' | 'hd'
export type PlaygroundImageBatchStatus = 'generating' | 'completed' | 'error'
export type PlaygroundImageStage = 'preparing' | 'requesting' | 'decoding'

export interface PlaygroundImageReference {
  id: string
  name: string
  size: number
  mimeType: string
  file: File
  previewUrl: string
  status: 'ready' | 'error'
  error?: string
}

export interface PlaygroundImageResult {
  id: string
  url: string
  mimeType: string
  revisedPrompt?: string
  revokeOnRelease: boolean
}

export interface PlaygroundImageBatch {
  id: string
  status: PlaygroundImageBatchStatus
  stage?: PlaygroundImageStage
  prompt: string
  option: PlaygroundModelOption
  model: string
  size: string
  quality: PlaygroundImageQuality
  count: number
  referenceCount: number
  createdAt: number
  completedAt?: number
  elapsedMs?: number
  results: PlaygroundImageResult[]
  error?: string
}

export type PlaygroundAttachmentKind = 'image' | 'text'
export type PlaygroundAttachmentEncoding = 'data-url' | 'text'

export interface PlaygroundAttachment {
  id: string
  name: string
  size: number
  mimeType?: string
  kind?: PlaygroundAttachmentKind
  createdAt?: number
  /** Transitional UI fields; payload persistence must still keep raw data outside localStorage. */
  type?: string
  status?: 'reading' | 'ready' | 'error' | 'missing'
  dataUrl?: string
  text?: string
  error?: string
}

export interface PlaygroundAttachmentPayload {
  id: string
  encoding: PlaygroundAttachmentEncoding
  data: string
}

export interface PlaygroundTextContentPart {
  type: 'text'
  text: string
}

export interface PlaygroundImageContentPart {
  type: 'image_url'
  image_url: string | {
    url: string
    detail?: 'auto' | 'low' | 'high'
  }
}

export interface PlaygroundFileContentPart {
  type: 'file'
  file: {
    file_data?: string
    file_id?: string
    filename?: string
  }
}

export type PlaygroundContentPart =
  | PlaygroundTextContentPart
  | PlaygroundImageContentPart
  | PlaygroundFileContentPart

export type PlaygroundToolActivityStatus = 'pending' | 'running' | 'completed' | 'failed' | 'done' | 'error'

export interface PlaygroundToolActivity {
  id: string
  type?: string
  kind?: 'webSearch' | 'codeExecution' | 'webFetch' | (string & {})
  name?: string
  label?: string
  status: PlaygroundToolActivityStatus
  input?: unknown
  output?: unknown
  error?: string
  startedAt?: number
  completedAt?: number
}

export interface PlaygroundTokenUsage {
  prompt_tokens?: number
  completion_tokens?: number
  total_tokens?: number
  input_tokens?: number
  output_tokens?: number
  input_tokens_details?: Record<string, number>
  output_tokens_details?: Record<string, number>
}

export interface PlaygroundMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  contentParts?: PlaygroundContentPart[]
  attachments?: PlaygroundAttachment[]
  toolActivities?: PlaygroundToolActivity[]
  error?: boolean
  usage?: PlaygroundTokenUsage
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

export function playgroundOptionKey(option: PlaygroundModelOption): string {
  return option.id || `${option.group_id}:${option.platform}:${option.model}`
}

export function samePlaygroundOption(
  left: PlaygroundModelOption | null | undefined,
  right: PlaygroundModelOption | null | undefined
): boolean {
  return !!left && !!right && playgroundOptionKey(left) === playgroundOptionKey(right)
}

import { computed, onBeforeUnmount, ref } from 'vue'

export const CURSOR_BROWSER_LOGIN_CHANNEL = 'sub2api.cursor-login'
export const CURSOR_BROWSER_LOGIN_PROTOCOL_VERSION = 1
export const CURSOR_EXTENSION_DOWNLOAD_URL = '/downloads/cursor-cookie-importer.zip'

const DETECTION_TIMEOUT_MS = 1500
const LOGIN_TIMEOUT_MS = 10 * 60 * 1000
const MAX_COOKIE_VALUE_LENGTH = 16 * 1024

type CursorBrowserLoginState =
  | 'detecting'
  | 'unavailable'
  | 'ready'
  | 'starting'
  | 'waiting_for_login'
  | 'reading_cookie'
  | 'received'
  | 'error'

export type CursorBrowserLoginErrorCode =
  | 'NOT_CONFIGURED'
  | 'BUSY'
  | 'CURSOR_TAB_CLOSED'
  | 'SOURCE_TAB_GONE'
  | 'SOURCE_DOCUMENT_CHANGED'
  | 'COOKIE_NOT_FOUND'
  | 'TIMEOUT'
  | 'CANCELLED'
  | 'ORIGIN_NOT_ALLOWED'
  | 'UNSUPPORTED_PROTOCOL'
  | 'INTERNAL_ERROR'
  | 'INVALID_CREDENTIAL'

export interface CursorBrowserCredential {
  value: string
  expirationDate?: number
}

interface ActiveFlow {
  flowId: string
  nonce: string
  bridgeInstanceId: string
  resolve: (credential: CursorBrowserCredential) => void
  reject: (error: CursorBrowserLoginError) => void
}

interface ExtensionMessage {
  channel?: unknown
  v?: unknown
  direction?: unknown
  type?: unknown
  flowId?: unknown
  nonce?: unknown
  bridgeInstanceId?: unknown
  extensionVersion?: unknown
  status?: unknown
  credential?: unknown
  code?: unknown
}

export class CursorBrowserLoginError extends Error {
  constructor(public readonly code: CursorBrowserLoginErrorCode) {
    super(code)
    this.name = 'CursorBrowserLoginError'
  }
}

function randomToken(bytes = 16): string {
  const values = new Uint8Array(bytes)
  crypto.getRandomValues(values)
  return Array.from(values, (value) => value.toString(16).padStart(2, '0')).join('')
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isSafeCookieValue(value: unknown): value is string {
  if (
    typeof value !== 'string' ||
    value.length === 0 ||
    value.length > MAX_COOKIE_VALUE_LENGTH ||
    value !== value.trim()
  ) return false
  for (const character of value) {
    const code = character.charCodeAt(0)
    if (code <= 0x20 || code === 0x7f || character === ';') return false
  }
  return true
}

function parseCredential(value: unknown): CursorBrowserCredential | null {
  if (!isRecord(value) || !isSafeCookieValue(value.value)) return null
  const credential: CursorBrowserCredential = { value: value.value }
  if (value.expirationDate !== undefined) {
    if (typeof value.expirationDate !== 'number' || !Number.isFinite(value.expirationDate) || value.expirationDate <= 0) {
      return null
    }
    credential.expirationDate = value.expirationDate
  }
  return credential
}

function isErrorCode(value: unknown): value is CursorBrowserLoginErrorCode {
  return typeof value === 'string' && [
    'NOT_CONFIGURED',
    'BUSY',
    'CURSOR_TAB_CLOSED',
    'SOURCE_TAB_GONE',
    'SOURCE_DOCUMENT_CHANGED',
    'COOKIE_NOT_FOUND',
    'TIMEOUT',
    'CANCELLED',
    'ORIGIN_NOT_ALLOWED',
    'UNSUPPORTED_PROTOCOL',
    'INTERNAL_ERROR',
    'INVALID_CREDENTIAL'
  ].includes(value)
}

export function useCursorBrowserLogin(options: { detectionTimeoutMs?: number; loginTimeoutMs?: number } = {}) {
  const state = ref<CursorBrowserLoginState>('detecting')
  const extensionVersion = ref('')
  const errorCode = ref<CursorBrowserLoginErrorCode | null>(null)
  const bridgeInstanceId = ref('')
  let activeFlow: ActiveFlow | null = null
  let detectionTimer: ReturnType<typeof setTimeout> | null = null
  let loginTimer: ReturnType<typeof setTimeout> | null = null
  let listening = false

  const detectionTimeoutMs = options.detectionTimeoutMs ?? DETECTION_TIMEOUT_MS
  const loginTimeoutMs = options.loginTimeoutMs ?? LOGIN_TIMEOUT_MS

  const available = computed(() => Boolean(bridgeInstanceId.value) && state.value !== 'unavailable')
  const busy = computed(() => ['starting', 'waiting_for_login', 'reading_cookie'].includes(state.value))

  const postPageMessage = (message: Record<string, unknown>) => {
    window.postMessage({
      channel: CURSOR_BROWSER_LOGIN_CHANNEL,
      v: CURSOR_BROWSER_LOGIN_PROTOCOL_VERSION,
      direction: 'page-to-extension',
      ...message
    }, window.location.origin)
  }

  const clearLoginTimer = () => {
    if (loginTimer) clearTimeout(loginTimer)
    loginTimer = null
  }

  const finishWithError = (code: CursorBrowserLoginErrorCode) => {
    const flow = activeFlow
    activeFlow = null
    clearLoginTimer()
    errorCode.value = code
    state.value = 'error'
    flow?.reject(new CursorBrowserLoginError(code))
  }

  const handleMessage = (event: MessageEvent) => {
    if (event.source !== window || event.origin !== window.location.origin) return
    const message = event.data as ExtensionMessage
    if (!isRecord(message)) return
    if (
      message.channel !== CURSOR_BROWSER_LOGIN_CHANNEL ||
      message.v !== CURSOR_BROWSER_LOGIN_PROTOCOL_VERSION ||
      message.direction !== 'extension-to-page'
    ) return

    if (message.type === 'READY') {
      if (typeof message.bridgeInstanceId !== 'string' || !message.bridgeInstanceId) return
      bridgeInstanceId.value = message.bridgeInstanceId
      extensionVersion.value = typeof message.extensionVersion === 'string' ? message.extensionVersion : ''
      errorCode.value = null
      if (!activeFlow) state.value = 'ready'
      if (detectionTimer) clearTimeout(detectionTimer)
      detectionTimer = null
      return
    }

    const flow = activeFlow
    if (!flow) return
    if (
      message.flowId !== flow.flowId ||
      message.nonce !== flow.nonce ||
      message.bridgeInstanceId !== flow.bridgeInstanceId
    ) return

    if (message.type === 'ACCEPTED') {
      state.value = 'waiting_for_login'
      return
    }
    if (message.type === 'STATUS') {
      if (message.status === 'waiting_for_login') state.value = 'waiting_for_login'
      if (message.status === 'reading_cookie' || message.status === 'returning') state.value = 'reading_cookie'
      return
    }
    if (message.type === 'RESULT') {
      const credential = parseCredential(message.credential)
      if (!credential) {
        finishWithError('INVALID_CREDENTIAL')
        return
      }
      activeFlow = null
      clearLoginTimer()
      errorCode.value = null
      state.value = 'received'
      flow.resolve(credential)
      return
    }
    if (message.type === 'ERROR') {
      finishWithError(isErrorCode(message.code) ? message.code : 'INTERNAL_ERROR')
    }
  }

  const ping = () => postPageMessage({ type: 'PING' })

  const initialize = () => {
    if (!listening) {
      window.addEventListener('message', handleMessage)
      listening = true
    }
    state.value = bridgeInstanceId.value ? 'ready' : 'detecting'
    ping()
    if (detectionTimer) clearTimeout(detectionTimer)
    detectionTimer = setTimeout(() => {
      if (!bridgeInstanceId.value && !activeFlow) state.value = 'unavailable'
    }, detectionTimeoutMs)
  }

  const start = (): Promise<CursorBrowserCredential> => {
    if (!bridgeInstanceId.value) return Promise.reject(new CursorBrowserLoginError('NOT_CONFIGURED'))
    if (activeFlow) return Promise.reject(new CursorBrowserLoginError('BUSY'))

    const flowId = crypto.randomUUID?.() || randomToken()
    const nonce = randomToken()
    const activeBridgeInstanceId = bridgeInstanceId.value
    state.value = 'starting'
    errorCode.value = null

    return new Promise<CursorBrowserCredential>((resolve, reject) => {
      activeFlow = { flowId, nonce, bridgeInstanceId: activeBridgeInstanceId, resolve, reject }
      loginTimer = setTimeout(() => {
        if (!activeFlow || activeFlow.flowId !== flowId) return
        postPageMessage({ type: 'CANCEL', flowId, nonce, bridgeInstanceId: activeBridgeInstanceId })
        finishWithError('TIMEOUT')
      }, loginTimeoutMs)
      postPageMessage({ type: 'START', flowId, nonce, bridgeInstanceId: activeBridgeInstanceId })
    })
  }

  const cancel = (silent = false) => {
    const flow = activeFlow
    if (flow) {
      postPageMessage({
        type: 'CANCEL',
        flowId: flow.flowId,
        nonce: flow.nonce,
        bridgeInstanceId: flow.bridgeInstanceId
      })
      activeFlow = null
      clearLoginTimer()
      if (!silent) flow.reject(new CursorBrowserLoginError('CANCELLED'))
    }
    errorCode.value = null
    state.value = bridgeInstanceId.value ? 'ready' : 'unavailable'
  }

  const dispose = () => {
    cancel(true)
    if (detectionTimer) clearTimeout(detectionTimer)
    detectionTimer = null
    if (listening) window.removeEventListener('message', handleMessage)
    listening = false
  }

  initialize()
  onBeforeUnmount(dispose)

  return {
    state,
    available,
    busy,
    extensionVersion,
    errorCode,
    initialize,
    ping,
    start,
    cancel,
    dispose
  }
}

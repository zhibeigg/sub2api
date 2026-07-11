import type { PublicSettings } from '@/types'

interface ChatwootUserAttributes {
  email?: string
  name?: string
  avatar_url?: string
  identifier_hash?: string
}

interface ChatwootClient {
  reset(): void
  setLocale(locale: string): void
  setUser(identifier: string, attributes: ChatwootUserAttributes): void
}

interface ChatwootSDK {
  run(config: { websiteToken: string; baseUrl: string }): void
}

declare global {
  interface Window {
    __APP_CONFIG__?: PublicSettings
    $chatwoot?: ChatwootClient
    chatwootSDK?: ChatwootSDK
    chatwootSettings?: Record<string, unknown>
  }
}

export {}

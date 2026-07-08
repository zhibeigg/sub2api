import { ref, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { KiroCredentials, KiroDeviceLoginResult, KiroAuthUrlResult } from '@/api/admin/kiro'

/**
 * useKiroOAuth drives the three interactive Kiro login flows:
 *  - Builder ID device code (start → poll on interval)
 *  - IAM Identity Center PKCE (start authorize URL → complete with callback)
 *  - SSO token import (single call)
 *
 * Device polling uses recursive setTimeout (not setInterval) and is cleaned up
 * on unmount.
 */
export function useKiroOAuth() {
  const appStore = useAppStore()
  const { t } = useI18n()

  const loading = ref(false)
  const error = ref('')

  // Builder ID device-code state
  const deviceSessionId = ref('')
  const userCode = ref('')
  const verificationUri = ref('')
  const polling = ref(false)

  // IAM SSO state
  const authUrl = ref('')
  const ssoSessionId = ref('')
  const state = ref('')

  let pollTimer: ReturnType<typeof setTimeout> | null = null

  const stopPolling = () => {
    if (pollTimer) {
      clearTimeout(pollTimer)
      pollTimer = null
    }
    polling.value = false
  }

  const resetState = () => {
    stopPolling()
    loading.value = false
    error.value = ''
    deviceSessionId.value = ''
    userCode.value = ''
    verificationUri.value = ''
    authUrl.value = ''
    ssoSessionId.value = ''
    state.value = ''
  }

  const fail = (msg: string) => {
    error.value = msg
    appStore.showError(msg)
  }

  // ---- Builder ID ----

  const startBuilderID = async (
    region: string | undefined,
    proxyId: number | null | undefined
  ): Promise<KiroDeviceLoginResult | null> => {
    loading.value = true
    error.value = ''
    try {
      const payload: Record<string, unknown> = {}
      if (region) payload.region = region
      if (proxyId) payload.proxy_id = proxyId
      const res = await adminAPI.kiro.startBuilderID(payload)
      deviceSessionId.value = res.session_id
      userCode.value = res.user_code
      verificationUri.value = res.verification_uri
      return res
    } catch (err: any) {
      fail(err.response?.data?.detail || t('admin.accounts.oauth.kiro.failedToStart'))
      return null
    } finally {
      loading.value = false
    }
  }

  /**
   * Poll the Builder ID device grant until completed. Resolves with the
   * credentials map on success or null on failure/timeout.
   */
  const pollBuilderID = (intervalSeconds: number): Promise<KiroCredentials | null> => {
    const intervalMs = Math.max(1, intervalSeconds || 5) * 1000
    polling.value = true
    return new Promise((resolve) => {
      const tick = async () => {
        if (!deviceSessionId.value) {
          stopPolling()
          resolve(null)
          return
        }
        try {
          const res = await adminAPI.kiro.pollBuilderID(deviceSessionId.value)
          if (res.status === 'completed' && res.credentials) {
            stopPolling()
            resolve(res.credentials)
            return
          }
          // still pending → schedule next poll
          pollTimer = setTimeout(tick, intervalMs)
        } catch (err: any) {
          stopPolling()
          fail(err.response?.data?.detail || t('admin.accounts.oauth.kiro.pollFailed'))
          resolve(null)
        }
      }
      pollTimer = setTimeout(tick, intervalMs)
    })
  }

  // ---- IAM Identity Center PKCE ----

  const startIAMSSO = async (
    startUrl: string | undefined,
    region: string | undefined,
    proxyId: number | null | undefined
  ): Promise<KiroAuthUrlResult | null> => {
    loading.value = true
    error.value = ''
    try {
      const payload: Record<string, unknown> = {}
      if (startUrl) payload.start_url = startUrl
      if (region) payload.region = region
      if (proxyId) payload.proxy_id = proxyId
      const res = await adminAPI.kiro.startIAMSSO(payload)
      authUrl.value = res.auth_url
      ssoSessionId.value = res.session_id
      state.value = res.state
      return res
    } catch (err: any) {
      fail(err.response?.data?.detail || t('admin.accounts.oauth.kiro.failedToStart'))
      return null
    } finally {
      loading.value = false
    }
  }

  const completeIAMSSO = async (callbackUrl: string): Promise<KiroCredentials | null> => {
    if (!ssoSessionId.value || !callbackUrl.trim()) {
      fail(t('admin.accounts.oauth.kiro.missingCallback'))
      return null
    }
    loading.value = true
    error.value = ''
    try {
      const res = await adminAPI.kiro.completeIAMSSO({
        session_id: ssoSessionId.value,
        callback_url: callbackUrl.trim(),
      })
      return res.credentials
    } catch (err: any) {
      fail(err.response?.data?.detail || t('admin.accounts.oauth.kiro.failedToComplete'))
      return null
    } finally {
      loading.value = false
    }
  }

  // ---- SSO token import ----

  const importSSOToken = async (
    bearerToken: string,
    region: string | undefined,
    proxyId: number | null | undefined
  ): Promise<KiroCredentials | null> => {
    if (!bearerToken.trim()) {
      fail(t('admin.accounts.oauth.kiro.missingBearer'))
      return null
    }
    loading.value = true
    error.value = ''
    try {
      const payload: Record<string, unknown> = { bearer_token: bearerToken.trim() }
      if (region) payload.region = region
      if (proxyId) payload.proxy_id = proxyId
      const res = await adminAPI.kiro.importSSOToken(payload as any)
      return res.credentials
    } catch (err: any) {
      fail(err.response?.data?.detail || t('admin.accounts.oauth.kiro.failedToImport'))
      return null
    } finally {
      loading.value = false
    }
  }

  onUnmounted(stopPolling)

  return {
    loading,
    error,
    // builder id
    deviceSessionId,
    userCode,
    verificationUri,
    polling,
    startBuilderID,
    pollBuilderID,
    stopPolling,
    // iam sso
    authUrl,
    ssoSessionId,
    state,
    startIAMSSO,
    completeIAMSSO,
    // sso token
    importSSOToken,
    resetState,
  }
}

import { onBeforeUnmount, onMounted } from 'vue'

export interface UseVisibleAutoRefreshOptions {
  /** Refresh interval while the page is visible. Set to 0 to disable polling. */
  intervalMs?: number
  /** Minimum gap between focus/visibility-triggered refreshes to avoid duplicate requests. */
  minTriggerGapMs?: number
  /** Called when a refresh should be performed. Errors are swallowed to avoid breaking timers. */
  onRefresh: () => Promise<void> | void
  /** Return false to skip a refresh, for example while another request is running. */
  shouldRefresh?: () => boolean
}

export function useVisibleAutoRefresh(options: UseVisibleAutoRefreshOptions) {
  const {
    intervalMs = 60_000,
    minTriggerGapMs = 3_000,
    onRefresh,
    shouldRefresh,
  } = options

  let timer: ReturnType<typeof setInterval> | null = null
  let lastTriggerAt = 0

  const isPageVisible = () => typeof document === 'undefined' || document.visibilityState === 'visible'

  function stopTimer() {
    if (timer !== null) {
      clearInterval(timer)
      timer = null
    }
  }

  function startTimer() {
    if (intervalMs <= 0 || timer !== null || !isPageVisible()) return
    timer = setInterval(() => {
      triggerRefresh('interval')
    }, intervalMs)
  }

  function triggerRefresh(reason: 'focus' | 'visibility' | 'interval') {
    if (!isPageVisible()) return
    if (shouldRefresh && !shouldRefresh()) return

    const now = Date.now()
    if (reason !== 'interval' && now - lastTriggerAt < minTriggerGapMs) return
    lastTriggerAt = now

    void Promise.resolve(onRefresh()).catch((err: unknown) => {
      console.warn('Visible auto refresh failed:', err)
    })
  }

  function handleVisibilityChange() {
    if (isPageVisible()) {
      startTimer()
      triggerRefresh('visibility')
    } else {
      stopTimer()
    }
  }

  function handleFocus() {
    triggerRefresh('focus')
  }

  onMounted(() => {
    startTimer()
    document.addEventListener('visibilitychange', handleVisibilityChange)
    window.addEventListener('focus', handleFocus)
  })

  onBeforeUnmount(() => {
    stopTimer()
    document.removeEventListener('visibilitychange', handleVisibilityChange)
    window.removeEventListener('focus', handleFocus)
  })

  return {
    triggerRefresh,
    startTimer,
    stopTimer,
  }
}

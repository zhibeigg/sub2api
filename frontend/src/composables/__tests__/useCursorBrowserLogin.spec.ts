import { defineComponent, nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  CURSOR_BROWSER_LOGIN_CHANNEL,
  CursorBrowserLoginError,
  useCursorBrowserLogin
} from '../useCursorBrowserLogin'

const Harness = defineComponent({
  setup() {
    return useCursorBrowserLogin({ detectionTimeoutMs: 25, loginTimeoutMs: 50 })
  },
  template: '<div />'
})

function extensionMessage(data: Record<string, unknown>, origin = window.location.origin) {
  window.dispatchEvent(new MessageEvent('message', {
    data: {
      channel: CURSOR_BROWSER_LOGIN_CHANNEL,
      v: 1,
      direction: 'extension-to-page',
      ...data
    },
    origin,
    source: window
  }))
}

function ready() {
  extensionMessage({ type: 'READY', bridgeInstanceId: 'bridge-1', extensionVersion: '0.34.5' })
}

describe('useCursorBrowserLogin', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.spyOn(window, 'postMessage').mockImplementation(() => undefined)
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('detects the extension through a strict READY message', async () => {
    const wrapper = mount(Harness)
    expect(window.postMessage).toHaveBeenCalledWith(expect.objectContaining({ type: 'PING' }), window.location.origin)

    ready()
    await nextTick()

    expect(wrapper.vm.state).toBe('ready')
    expect(wrapper.vm.available).toBe(true)
    expect(wrapper.vm.extensionVersion).toBe('0.34.5')
    wrapper.unmount()
  })

  it('ignores READY messages from another origin', async () => {
    const wrapper = mount(Harness)
    extensionMessage({ type: 'READY', bridgeInstanceId: 'forged' }, 'https://evil.example')
    vi.advanceTimersByTime(30)
    await nextTick()

    expect(wrapper.vm.state).toBe('unavailable')
    expect(wrapper.vm.available).toBe(false)
    wrapper.unmount()
  })

  it('returns only a valid credential bound to the active flow', async () => {
    const wrapper = mount(Harness)
    ready()
    await nextTick()

    const promise = wrapper.vm.start()
    const startMessage = vi.mocked(window.postMessage).mock.calls.at(-1)?.[0] as Record<string, unknown>
    expect(startMessage).toMatchObject({ type: 'START', bridgeInstanceId: 'bridge-1' })
    expect(startMessage).not.toHaveProperty('cookie')

    extensionMessage({
      type: 'RESULT',
      flowId: startMessage.flowId,
      nonce: 'wrong',
      bridgeInstanceId: 'bridge-1',
      credential: { value: 'forged' }
    })
    expect(wrapper.vm.state).toBe('starting')

    extensionMessage({
      type: 'ACCEPTED',
      flowId: startMessage.flowId,
      nonce: startMessage.nonce,
      bridgeInstanceId: 'bridge-1'
    })
    expect(wrapper.vm.state).toBe('waiting_for_login')

    extensionMessage({
      type: 'RESULT',
      flowId: startMessage.flowId,
      nonce: startMessage.nonce,
      bridgeInstanceId: 'bridge-1',
      credential: { value: 'safe-token', expirationDate: 1_800_000_000 }
    })

    await expect(promise).resolves.toEqual({ value: 'safe-token', expirationDate: 1_800_000_000 })
    expect(wrapper.vm.state).toBe('received')
    wrapper.unmount()
  })

  it('rejects unsafe Cookie values without exposing them', async () => {
    const wrapper = mount(Harness)
    ready()
    const promise = wrapper.vm.start()
    const startMessage = vi.mocked(window.postMessage).mock.calls.at(-1)?.[0] as Record<string, unknown>

    extensionMessage({
      type: 'RESULT',
      flowId: startMessage.flowId,
      nonce: startMessage.nonce,
      bridgeInstanceId: 'bridge-1',
      credential: { value: 'secret; injected=1' }
    })

    await expect(promise).rejects.toMatchObject({ code: 'INVALID_CREDENTIAL' })
    expect(wrapper.vm.errorCode).toBe('INVALID_CREDENTIAL')
    wrapper.unmount()
  })

  it('cancels the extension flow when login times out', async () => {
    const wrapper = mount(Harness)
    ready()
    const promise = wrapper.vm.start()
    const startMessage = vi.mocked(window.postMessage).mock.calls.at(-1)?.[0] as Record<string, unknown>

    vi.advanceTimersByTime(55)

    await expect(promise).rejects.toEqual(expect.objectContaining<Partial<CursorBrowserLoginError>>({ code: 'TIMEOUT' }))
    expect(window.postMessage).toHaveBeenCalledWith(expect.objectContaining({
      type: 'CANCEL',
      flowId: startMessage.flowId,
      nonce: startMessage.nonce
    }), window.location.origin)
    wrapper.unmount()
  })
})

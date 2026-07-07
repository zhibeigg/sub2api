import { afterEach, describe, expect, it, vi } from 'vitest'
// @ts-expect-error: this test intentionally imports the compiler-included build to validate message syntax.
import { createI18n } from '../../../node_modules/vue-i18n/dist/vue-i18n.mjs'

import en from '../locales/en'
import zh from '../locales/zh'

const renderKiroPlaceholder = (locale: 'zh' | 'en') => {
  const messages = { zh, en }
  const i18n = createI18n({
    legacy: false,
    locale,
    messages
  })

  return i18n.global.t('admin.accounts.kiro.credentialsPlaceholder')
}

describe('Kiro account locale messages', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders the zh credentials JSON placeholder without vue-i18n compilation errors', () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})

    const placeholder = renderKiroPlaceholder('zh')

    expect(consoleError).not.toHaveBeenCalled()
    expect(placeholder).toContain('粘贴从 Kiro-Go 导出的凭证 JSON')
    expect(placeholder).toContain('{\n  "accessToken": "..."')
    expect(placeholder).toContain('"profileArn": "arn:aws:codewhisperer:..."\n}')
  })

  it('renders the en credentials JSON placeholder without vue-i18n compilation errors', () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})

    const placeholder = renderKiroPlaceholder('en')

    expect(consoleError).not.toHaveBeenCalled()
    expect(placeholder).toContain('Paste the credentials JSON exported from Kiro-Go')
    expect(placeholder).toContain('{\n  "accessToken": "..."')
    expect(placeholder).toContain('"profileArn": "arn:aws:codewhisperer:..."\n}')
  })
})

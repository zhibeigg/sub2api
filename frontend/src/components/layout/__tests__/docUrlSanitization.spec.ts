import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const headerSource = readFileSync(resolve(dir, '../AppHeader.vue'), 'utf8')
const authLayoutSource = readFileSync(resolve(dir, '../AuthLayout.vue'), 'utf8')
const homeViewSource = readFileSync(resolve(dir, '../../../views/HomeView.vue'), 'utf8')
const keyUsageViewSource = readFileSync(resolve(dir, '../../../views/KeyUsageView.vue'), 'utf8')
const urlSource = readFileSync(resolve(dir, '../../../utils/url.ts'), 'utf8')

const documentationSurfaces = [headerSource, authLayoutSource, homeViewSource, keyUsageViewSource]

describe('documentation URL policy', () => {
  it('pins the public documentation host to docs.poke2api.com', () => {
    expect(urlSource).toContain("export const PUBLIC_DOCS_URL = 'https://docs.poke2api.com'")
  })

  it('uses the shared public documentation URL on every user-facing surface', () => {
    for (const source of documentationSurfaces) {
      expect(source).toContain('PUBLIC_DOCS_URL')
      expect(source).toContain('const docUrl = PUBLIC_DOCS_URL')
    }
  })

  it('does not allow public settings to override documentation links', () => {
    for (const source of documentationSurfaces) {
      expect(source).not.toContain('cachedPublicSettings?.doc_url')
      expect(source).not.toContain('appStore.docUrl')
    }
  })
})

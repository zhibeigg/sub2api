import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const homeViewSource = readFileSync(resolve(dir, '../../../views/HomeView.vue'), 'utf8')
const loginViewSource = readFileSync(resolve(dir, '../../../views/auth/LoginView.vue'), 'utf8')
const registerViewSource = readFileSync(resolve(dir, '../../../views/auth/RegisterView.vue'), 'utf8')
const authLayoutSource = readFileSync(resolve(dir, '../AuthLayout.vue'), 'utf8')
const monologStylesSource = readFileSync(resolve(dir, '../../../styles/monolog.css'), 'utf8')

describe('public page structure', () => {
  it('keeps documentation one click away from the home page', () => {
    expect(homeViewSource).toContain('data-testid="home-docs-link"')
    expect(homeViewSource).toContain('data-testid="home-docs-section"')
    expect(homeViewSource).toContain(
      'sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl'
    )
  })

  it('uses the PokeAPI public brand and exposes the alternate website node', () => {
    expect(homeViewSource).toContain("const siteName = 'PokeAPI'")
    expect(homeViewSource).toContain("labelKey: 'home.hero.websiteNode'")
    expect(homeViewSource).toContain("url: 'https://www.poke2api.com'")
  })

  it('states the privacy boundary on home and auth pages', () => {
    expect(homeViewSource).toContain("t('home.privacy.title')")
    expect(homeViewSource).toContain("t('home.privacy.minimum')")
    expect(authLayoutSource).toContain("t('auth.brand.privacyTitle')")
    expect(authLayoutSource).toContain("t('auth.brand.privacyNote')")
  })

  it('keeps light and dark theme controls on both public surfaces', () => {
    expect(homeViewSource).toContain('@click="toggleTheme"')
    expect(authLayoutSource).toContain('@click="toggleTheme"')
    expect(monologStylesSource).toContain('html.dark .monolog-scope')
    expect(monologStylesSource).toContain('oklch(')
  })

  it('keeps keyboard users close to the primary content', () => {
    expect(homeViewSource).toContain('href="#home-main"')
    expect(authLayoutSource).toContain('href="#auth-form-content"')
    expect(monologStylesSource).toContain(':focus-visible')
  })

  it('removes decorative icons from the primary email and password fields', () => {
    for (const source of [loginViewSource, registerViewSource]) {
      expect(source).not.toContain('<Icon name="mail"')
      expect(source).not.toContain('<Icon name="lock"')
      expect(source).toContain("t('auth.showPassword')")
      expect(source).toContain("t('auth.hidePassword')")
    }
  })

  it('does not bring back decorative public-page artwork', () => {
    for (const source of [homeViewSource, authLayoutSource]) {
      expect(source).not.toContain('grain.svg')
      expect(source).not.toContain('gateway-plate.svg')
      expect(source).not.toContain('auth-plate.svg')
      expect(source).not.toContain('<canvas')
    }

    expect(homeViewSource).not.toContain('mono-cursor')
  })
})

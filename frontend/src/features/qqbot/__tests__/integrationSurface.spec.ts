import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'

const here = dirname(fileURLToPath(import.meta.url))
const read = (path: string) => readFileSync(resolve(here, path), 'utf8')

function keys(value: unknown, prefix = ''): string[] {
  if (!value || typeof value !== 'object') return [prefix]
  return Object.entries(value as Record<string, unknown>).flatMap(([key, child]) => keys(child, prefix ? `${prefix}.${key}` : key))
}

describe('QQBot integration surface', () => {
  it('registers public binding and admin-only management routes and backend-mode allowlist', () => {
    const router = read('../../../router/index.ts')
    expect(router).toContain("path: '/bind'")
    expect(router).toContain("component: () => import('@/features/qqbot/public/BindView.vue')")
    expect(router).toContain("path: '/admin/qqbot'")
    const adminRoute = router.slice(router.indexOf("path: '/admin/qqbot'"), router.indexOf("path: '/admin/usage'"))
    expect(adminRoute).toContain('requiresAuth: true')
    expect(adminRoute).toContain('requiresAdmin: true')
    expect(router).toContain("'/bind']")
  })

  it('adds a standalone sidebar item and exposes all six accessible tabs', () => {
    const sidebar = read('../../../components/layout/AppSidebar.vue')
    expect(sidebar).toContain("path: '/admin/qqbot'")
    expect(sidebar).toContain("label: t('nav.qqbot')")
    const view = read('../QQBotView.vue')
    for (const tab of ['overview', 'config', 'onebot', 'messages', 'bindings', 'diagnostics']) {
      expect(view).toContain(`id: '${tab}' as const`)
    }
    expect(view).toContain('role="tablist"')
    expect(view).toContain(':aria-selected')
  })

  it('keeps Chinese and English locale trees symmetric', () => {
    expect(keys(zh.admin.qqbot).sort()).toEqual(keys(en.admin.qqbot).sort())
    expect(keys(zh.qqbotBind).sort()).toEqual(keys(en.qqbotBind).sort())
    expect(zh.nav.qqbot).toBeTruthy()
    expect(en.nav.qqbot).toBeTruthy()
  })

  it('avoids gradients, glass blur, and decorative side rails in new QQBot views', () => {
    const sources = [read('../QQBotView.vue'), read('../public/BindView.vue'), read('../components/OverviewTab.vue')].join('\n')
    expect(sources).not.toContain('bg-gradient')
    expect(sources).not.toContain('backdrop-blur')
    expect(sources).not.toContain('border-l-4')
  })
})

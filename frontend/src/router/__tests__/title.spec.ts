import { beforeEach, describe, expect, it } from 'vitest'
import {
  applyRouteSEO,
  CANONICAL_SITE_ORIGIN,
  INDEXABLE_ROBOTS,
  NOINDEX_ROBOTS,
  resolveDocumentTitle,
  resolveRouteDocumentTitle,
  resolveRouteSEO,
} from '@/router/title'

describe('resolveDocumentTitle', () => {
  it('路由存在标题时，使用“路由标题 - 站点名”格式', () => {
    expect(resolveDocumentTitle('Usage Records', 'My Site')).toBe('Usage Records - My Site')
  })

  it('路由无标题时，回退到站点名', () => {
    expect(resolveDocumentTitle(undefined, 'My Site')).toBe('My Site')
  })

  it('站点名为空时，回退默认站点名', () => {
    expect(resolveDocumentTitle('Dashboard', '')).toBe('Dashboard - Sub2API')
    expect(resolveDocumentTitle(undefined, '   ')).toBe('Sub2API')
  })

  it('站点名变更时仅影响后续路由标题计算', () => {
    const before = resolveDocumentTitle('Admin Dashboard', 'Alpha')
    const after = resolveDocumentTitle('Admin Dashboard', 'Beta')

    expect(before).toBe('Admin Dashboard - Alpha')
    expect(after).toBe('Admin Dashboard - Beta')
  })
})

describe('resolveRouteDocumentTitle', () => {
  it('自定义页面菜单加载后，使用菜单名称作为标题', () => {
    const route = {
      name: 'CustomPage',
      params: { id: 'scheduler' },
      meta: {
        title: 'Custom Page'
      }
    }

    expect(resolveRouteDocumentTitle(route, 'EzouAPI')).toBe('Custom Page - EzouAPI')
    expect(resolveRouteDocumentTitle(route, 'EzouAPI', [
      {
        id: 'scheduler',
        label: '账号调度器',
        icon_svg: '',
        url: 'https://example.com',
        visibility: 'admin',
        sort_order: 0
      }
    ])).toBe('账号调度器 - EzouAPI')
  })
})

describe('route SEO', () => {
  beforeEach(() => {
    document.head.innerHTML = ''
    document.documentElement.lang = 'zh-CN'
  })

  it('首页使用可收录规则和规范地址', () => {
    const seo = resolveRouteSEO({
      name: 'Home',
      path: '/home',
      params: {},
      meta: {
        title: 'Home',
        seoTitleKey: 'seo.home.title',
        seoDescriptionKey: 'seo.home.description',
        indexable: true,
        canonicalPath: '/home',
      },
    }, 'Poke API', [], (key) => ({
      'seo.home.title': 'Claude、OpenAI、Gemini AI API 网关',
      'seo.home.description': '一个 API Key 接入 Claude、OpenAI、Gemini。',
    })[String(key)] ?? '')

    expect(seo.title).toBe('Poke API｜Claude、OpenAI、Gemini AI API 网关')
    expect(seo.description).toContain('一个 API Key 接入 Claude、OpenAI、Gemini')
    expect(seo.robots).toBe(INDEXABLE_ROBOTS)
    expect(seo.canonicalUrl).toBe(`${CANONICAL_SITE_ORIGIN}/home`)
  })

  it('未显式开放收录的页面默认 noindex', () => {
    const seo = resolveRouteSEO({
      name: 'Login',
      path: '/login',
      params: {},
      meta: { title: 'Login' },
    }, 'Poke API')

    expect(seo.indexable).toBe(false)
    expect(seo.robots).toBe(NOINDEX_ROBOTS)
    expect(seo.canonicalUrl).toBe('')
  })

  it('把首页 SEO 写入 document head', () => {
    const seo = {
      title: 'Poke API｜AI API 网关',
      description: '稳定接入主流 AI 模型。',
      robots: INDEXABLE_ROBOTS,
      canonicalUrl: `${CANONICAL_SITE_ORIGIN}/home`,
      indexable: true,
    }

    applyRouteSEO(seo, 'zh-CN', 'Poke API')

    expect(document.title).toBe(seo.title)
    expect(document.querySelector('meta[name="description"]')?.getAttribute('content')).toBe(seo.description)
    expect(document.querySelector('meta[name="robots"]')?.getAttribute('content')).toBe(INDEXABLE_ROBOTS)
    expect(document.querySelector('link[rel="canonical"]')?.getAttribute('href')).toBe(seo.canonicalUrl)
    expect(document.querySelector('meta[property="og:url"]')?.getAttribute('content')).toBe(seo.canonicalUrl)
    expect(JSON.parse(document.querySelector('#site-structured-data')?.textContent ?? '{}')).toMatchObject({
      '@type': 'WebSite',
      name: 'Poke API',
      url: seo.canonicalUrl,
    })
  })

  it('切换到私有页时移除 canonical 并写入 noindex', () => {
    document.head.innerHTML = `<link rel="canonical" href="${CANONICAL_SITE_ORIGIN}/home"><script id="site-structured-data" type="application/ld+json">{"@type":"WebSite"}</script>`
    applyRouteSEO({
      title: 'Dashboard - Poke API',
      description: '',
      robots: NOINDEX_ROBOTS,
      canonicalUrl: '',
      indexable: false,
    }, 'en', 'Poke API')

    expect(document.documentElement.lang).toBe('en')
    expect(document.querySelector('meta[name="robots"]')?.getAttribute('content')).toBe(NOINDEX_ROBOTS)
    expect(document.querySelector('link[rel="canonical"]')).toBeNull()
    expect(document.querySelector('meta[property="og:url"]')).toBeNull()
    expect(document.querySelector('#site-structured-data')?.textContent).toBe('')
  })
})

import { i18n } from '@/i18n'
import type { RouteLocationNormalizedLoaded } from 'vue-router'
import type { CustomMenuItem } from '@/types'

export const CANONICAL_SITE_ORIGIN = 'https://www.poke2api.com'
export const INDEXABLE_ROBOTS = 'index, follow, max-image-preview:large, max-snippet:-1, max-video-preview:-1'
export const NOINDEX_ROBOTS = 'noindex, nofollow'

type RouteSEOInput = Pick<RouteLocationNormalizedLoaded, 'name' | 'path' | 'params' | 'meta'>

export type ResolvedRouteSEO = {
  title: string
  description: string
  robots: string
  canonicalUrl: string
  indexable: boolean
}

function normalizeSiteName(siteName?: string): string {
  return typeof siteName === 'string' && siteName.trim() ? siteName.trim() : 'Sub2API'
}

function translateMessage(key: unknown): string {
  if (typeof key !== 'string' || !key.trim()) {
    return ''
  }
  const translated = i18n.global.t(key)
  return translated && translated !== key ? translated : ''
}

/**
 * 统一生成页面标题，避免多处写入 document.title 产生覆盖冲突。
 * 优先使用 titleKey 通过 i18n 翻译，fallback 到静态 routeTitle。
 */
export function resolveDocumentTitle(routeTitle: unknown, siteName?: string, titleKey?: string): string {
  const normalizedSiteName = normalizeSiteName(siteName)
  const translated = translateMessage(titleKey)

  if (translated) {
    return `${translated} - ${normalizedSiteName}`
  }

  if (typeof routeTitle === 'string' && routeTitle.trim()) {
    return `${routeTitle.trim()} - ${normalizedSiteName}`
  }

  return normalizedSiteName
}

export function resolveRouteDocumentTitle(
  route: Pick<RouteLocationNormalizedLoaded, 'name' | 'params' | 'meta'>,
  siteName: string | undefined,
  customMenuItems: CustomMenuItem[] = [],
): string {
  const id = typeof route.params.id === 'string' ? route.params.id : ''
  const menuItem = route.name === 'CustomPage' && id
    ? customMenuItems.find((item) => item.id === id)
    : undefined
  const menuTitle = menuItem?.label.trim()

  return resolveDocumentTitle(menuTitle || route.meta.title, siteName, menuTitle ? undefined : route.meta.titleKey as string)
}

export function resolveRouteSEO(
  route: RouteSEOInput,
  siteName: string | undefined,
  customMenuItems: CustomMenuItem[] = [],
  translator: (key: unknown) => string = translateMessage,
): ResolvedRouteSEO {
  const normalizedSiteName = normalizeSiteName(siteName)
  const seoTitle = translator(route.meta.seoTitleKey)
  const indexable = route.meta.indexable === true
  const canonicalPath = typeof route.meta.canonicalPath === 'string' && route.meta.canonicalPath.startsWith('/')
    ? route.meta.canonicalPath
    : route.path

  return {
    title: seoTitle
      ? `${normalizedSiteName}｜${seoTitle}`
      : resolveRouteDocumentTitle(route, normalizedSiteName, customMenuItems),
    description: translator(indexable ? route.meta.seoDescriptionKey : route.meta.descriptionKey),
    robots: indexable ? INDEXABLE_ROBOTS : NOINDEX_ROBOTS,
    canonicalUrl: indexable ? new URL(canonicalPath, CANONICAL_SITE_ORIGIN).toString() : '',
    indexable,
  }
}

function setMetaContent(attribute: 'name' | 'property', key: string, content: string): void {
  let meta = document.head.querySelector<HTMLMetaElement>(`meta[${attribute}="${key}"]`)
  if (!content) {
    meta?.remove()
    return
  }
  if (!meta) {
    meta = document.createElement('meta')
    meta.setAttribute(attribute, key)
    document.head.appendChild(meta)
  }
  meta.content = content
}

function setCanonicalLink(url: string): void {
  let link = document.head.querySelector<HTMLLinkElement>('link[rel="canonical"]')
  if (!url) {
    link?.remove()
    return
  }
  if (!link) {
    link = document.createElement('link')
    link.rel = 'canonical'
    document.head.appendChild(link)
  }
  link.href = url
}

function setStructuredData(seo: ResolvedRouteSEO, siteName: string): void {
  let script = document.head.querySelector<HTMLScriptElement>('#site-structured-data')
  if (!seo.indexable) {
    if (script) {
      script.textContent = ''
    }
    return
  }

  if (!script) {
    script = document.createElement('script')
    script.id = 'site-structured-data'
    script.type = 'application/ld+json'
    const nonce = document.head.querySelector<HTMLScriptElement>('script[nonce]')?.nonce
    if (nonce) {
      script.nonce = nonce
    }
    document.head.appendChild(script)
  }

  script.textContent = JSON.stringify({
    '@context': 'https://schema.org',
    '@type': 'WebSite',
    name: siteName,
    alternateName: ['PokeAPI', 'Poke API Gateway'],
    url: seo.canonicalUrl,
  })
}

export function applyRouteSEO(seo: ResolvedRouteSEO, locale: string, siteName?: string): void {
  const normalizedSiteName = normalizeSiteName(siteName)
  const normalizedLocale = locale.toLowerCase().startsWith('en') ? 'en' : 'zh-CN'
  const openGraphLocale = normalizedLocale === 'en' ? 'en_US' : 'zh_CN'

  document.title = seo.title
  document.documentElement.lang = normalizedLocale
  setMetaContent('name', 'description', seo.description)
  setMetaContent('name', 'robots', seo.robots)
  setMetaContent('property', 'og:type', 'website')
  setMetaContent('property', 'og:site_name', normalizedSiteName)
  setMetaContent('property', 'og:locale', openGraphLocale)
  setMetaContent('property', 'og:title', seo.title)
  setMetaContent('property', 'og:description', seo.description)
  setMetaContent('property', 'og:url', seo.canonicalUrl)
  setMetaContent('name', 'twitter:card', 'summary')
  setMetaContent('name', 'twitter:title', seo.title)
  setMetaContent('name', 'twitter:description', seo.description)
  setCanonicalLink(seo.canonicalUrl)
  setStructuredData(seo, normalizedSiteName)
}

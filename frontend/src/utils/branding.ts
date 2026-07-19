import { sanitizeUrl } from '@/utils/url'

export function updateFavicon(logoUrl: string): void {
  const sanitizedLogoUrl = sanitizeUrl(logoUrl, {
    allowRelative: true,
    allowDataUrl: true,
  })
  if (!sanitizedLogoUrl) {
    return
  }

  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }

  link.type =
    sanitizedLogoUrl.startsWith('data:image/png') || /\.png(?:$|[?#])/i.test(sanitizedLogoUrl)
      ? 'image/png'
      : /\.svg(?:$|[?#])/i.test(sanitizedLogoUrl)
        ? 'image/svg+xml'
        : 'image/x-icon'
  link.href = sanitizedLogoUrl
}

import { onMounted, onBeforeUnmount } from 'vue'

/**
 * useSmoothScroll — bymonolog 风格的 Lenis 平滑滚动。
 *
 * 在公开页挂载时启用 Lenis（惯性平滑滚动），用 requestAnimationFrame 驱动，
 * 并与 GSAP ScrollTrigger 同步。返回 scrollTo 供锚点/回顶使用。
 * 尊重 prefers-reduced-motion（此时不启用，scrollTo 退回原生行为）。
 */
export function useSmoothScroll() {
  let lenis: { raf: (t: number) => void; scrollTo: (target: unknown, opts?: unknown) => void; on: (e: string, cb: () => void) => void; destroy: () => void } | null = null
  let rafId = 0
  let cancelled = false

  const scrollTo = (target: string | number | HTMLElement, opts?: Record<string, unknown>) => {
    if (lenis) {
      lenis.scrollTo(target, opts)
      return
    }
    // fallback：无 Lenis 时用原生平滑滚动
    if (typeof target === 'number') {
      window.scrollTo({ top: target, behavior: 'smooth' })
    } else if (typeof target === 'string') {
      document.querySelector(target)?.scrollIntoView({ behavior: 'smooth' })
    } else if (target instanceof HTMLElement) {
      target.scrollIntoView({ behavior: 'smooth' })
    }
  }

  onMounted(async () => {
    const prefersReduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    if (prefersReduced) return

    const [{ default: Lenis }, { ScrollTrigger }] = await Promise.all([
      import('lenis'),
      import('gsap/ScrollTrigger'),
    ])
    if (cancelled) return

    lenis = new Lenis({
      duration: 1.2,
      easing: (t: number) => Math.min(1, 1.001 - Math.pow(2, -10 * t)),
      smoothWheel: true,
    }) as unknown as typeof lenis

    lenis!.on('scroll', () => ScrollTrigger.update())

    const raf = (time: number) => {
      lenis?.raf(time)
      rafId = requestAnimationFrame(raf)
    }
    rafId = requestAnimationFrame(raf)
  })

  onBeforeUnmount(() => {
    cancelled = true
    if (rafId) cancelAnimationFrame(rafId)
    lenis?.destroy()
    lenis = null
  })

  return { scrollTo }
}

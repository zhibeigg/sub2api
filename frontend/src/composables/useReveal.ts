import { onMounted, onBeforeUnmount, type Ref } from 'vue'

/**
 * useReveal — bymonolog 风格的滚动揭示动画。
 *
 * 对容器内所有 [data-reveal] 元素做 fade-up (opacity/translateY) 入场，
 * 触发点为元素滚动到视口 85% 处，带 stagger。使用 GSAP + ScrollTrigger
 * （框架无关，动态 import 仅在公开页加载）。尊重 prefers-reduced-motion。
 *
 * @param scope 容器 ref（其内 [data-reveal] 元素会被动画）
 */
export function useReveal(scope: Ref<HTMLElement | null>) {
  let ctx: { revert: () => void } | null = null
  let cancelled = false

  onMounted(async () => {
    const root = scope.value
    if (!root) return

    const prefersReduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    const targets = Array.from(root.querySelectorAll<HTMLElement>('[data-reveal]'))
    if (targets.length === 0) return

    if (prefersReduced) {
      targets.forEach((el) => {
        el.style.opacity = '1'
        el.style.transform = 'none'
      })
      return
    }

    const [{ gsap }, { ScrollTrigger }] = await Promise.all([
      import('gsap'),
      import('gsap/ScrollTrigger'),
    ])
    if (cancelled) return
    gsap.registerPlugin(ScrollTrigger)

    ctx = gsap.context(() => {
      targets.forEach((el) => {
        gsap.fromTo(
          el,
          { autoAlpha: 0, y: 24 },
          {
            autoAlpha: 1,
            y: 0,
            duration: 0.9,
            ease: 'expo.out',
            scrollTrigger: {
              trigger: el,
              start: 'top 85%',
              once: true,
            },
          }
        )
      })
    }, root)

    // 布局稳定后刷新触发点
    requestAnimationFrame(() => ScrollTrigger.refresh())
  })

  onBeforeUnmount(() => {
    cancelled = true
    if (ctx) {
      ctx.revert()
      ctx = null
    }
  })
}

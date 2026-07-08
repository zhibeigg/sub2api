<template>
  <component :is="tag" ref="rootEl" class="mono-split" :class="{ 'mono-split--block': by === 'line' }">
    <span
      v-for="(token, i) in tokens"
      :key="i"
      class="mono-split-token"
    >
      <span class="mono-split-unit" data-unit>{{ token }}</span>
      <span v-if="by === 'word' && i < tokens.length - 1" class="mono-split-space"> </span>
    </span>
  </component>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'

/**
 * SplitReveal — bymonolog 风格的逐词/逐行滑入揭示。
 * 每个单元包在 overflow-hidden 容器里，内层从下方 110% 滑入。
 * onScroll=false 时加载即播（用于首屏）；否则滚动到 85% 触发。
 */
const props = withDefaults(
  defineProps<{
    text: string
    by?: 'word' | 'line' | 'char'
    tag?: string
    onScroll?: boolean
  }>(),
  {
    by: 'word',
    tag: 'span',
    onScroll: true,
  }
)

const rootEl = ref<HTMLElement | null>(null)
let ctx: { revert: () => void } | null = null
let cancelled = false

const tokens = computed(() => {
  if (props.by === 'char') return Array.from(props.text)
  if (props.by === 'line') return props.text.split('\n')
  return props.text.split(/\s+/).filter(Boolean)
})

onMounted(async () => {
  const root = rootEl.value
  if (!root) return

  const prefersReduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  const units = Array.from(root.querySelectorAll<HTMLElement>('[data-unit]'))
  if (units.length === 0 || prefersReduced) return

  const [{ gsap }, { ScrollTrigger }] = await Promise.all([
    import('gsap'),
    import('gsap/ScrollTrigger'),
  ])
  if (cancelled) return
  gsap.registerPlugin(ScrollTrigger)

  ctx = gsap.context(() => {
    const anim = {
      yPercent: 0,
      duration: 1,
      ease: 'expo.out',
      stagger: 0.06,
    }
    if (props.onScroll) {
      gsap.fromTo(units, { yPercent: 110 }, {
        ...anim,
        scrollTrigger: { trigger: root, start: 'top 85%', once: true },
      })
    } else {
      gsap.fromTo(units, { yPercent: 110 }, anim)
    }
  }, root)
})

onBeforeUnmount(() => {
  cancelled = true
  ctx?.revert()
  ctx = null
})
</script>

<style scoped>
.mono-split {
  display: inline;
}
.mono-split--block {
  display: block;
}
.mono-split-token {
  display: inline-flex;
  overflow: hidden;
  vertical-align: top;
}
.mono-split--block .mono-split-token {
  display: block;
}
.mono-split-unit {
  display: inline-block;
  will-change: transform;
}
.mono-split-space {
  display: inline-block;
  white-space: pre;
}
</style>

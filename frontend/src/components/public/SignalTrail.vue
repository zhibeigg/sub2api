<template>
  <div ref="root" class="signal-trail" aria-hidden="true">
    <span
      v-for="index in poolSize"
      :key="index"
      :ref="setNode"
      class="signal-trail__node"
    >
      <i></i>
      <b></b>
    </span>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

import { shouldReducePublicMotion } from './publicMotion'

const props = withDefaults(
  defineProps<{
    labels?: string[]
    bounded?: boolean
  }>(),
  {
    labels: () => ['API', '/v1', 'CLAUDE', 'OPENAI', 'GEMINI', '200 OK'],
    bounded: false
  }
)

const poolSize = 8
const root = ref<HTMLElement | null>(null)
const nodes: HTMLElement[] = []
let nodeIndex = 0
let lastX = -1000
let lastY = -1000
let lastAt = 0
let disabled = true

function setNode(element: unknown): void {
  if (element instanceof HTMLElement && !nodes.includes(element)) {
    nodes.push(element)
  }
}

function insideBounds(x: number, y: number): boolean {
  if (!props.bounded || !root.value?.parentElement) return true
  const rect = root.value.parentElement.getBoundingClientRect()
  return x >= rect.left && x <= rect.right && y >= rect.top && y <= rect.bottom
}

function handlePointerMove(event: PointerEvent): void {
  if (disabled || event.pointerType !== 'mouse' || !insideBounds(event.clientX, event.clientY)) return

  const now = performance.now()
  const distance = Math.hypot(event.clientX - lastX, event.clientY - lastY)
  if (distance < 42 && now - lastAt < 70) return

  const node = nodes[nodeIndex % nodes.length]
  if (!node) return

  const label = props.labels[nodeIndex % props.labels.length] || 'API'
  const marker = node.querySelector<HTMLElement>('i')
  const text = node.querySelector<HTMLElement>('b')
  if (text) text.textContent = label
  if (marker) marker.textContent = String((nodeIndex % 3) + 1).padStart(2, '0')

  node.style.transform = `translate3d(${event.clientX}px, ${event.clientY}px, 0)`
  node.getAnimations({ subtree: true }).forEach((animation) => animation.cancel())
  node.animate(
    [
      { opacity: 0 },
      { opacity: 0.9, offset: 0.12 },
      { opacity: 0, offset: 1 }
    ],
    { duration: 720, easing: 'cubic-bezier(0.16, 1, 0.3, 1)', fill: 'both' }
  )
  text?.animate(
    [
      { transform: 'translate3d(0, 12px, 0)' },
      { transform: 'translate3d(0, 0, 0)', offset: 0.16 },
      { transform: 'translate3d(0, -16px, 0)', offset: 1 }
    ],
    { duration: 720, easing: 'cubic-bezier(0.16, 1, 0.3, 1)', fill: 'both' }
  )

  nodeIndex += 1
  lastX = event.clientX
  lastY = event.clientY
  lastAt = now
}

onMounted(() => {
  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  const coarsePointer = window.matchMedia('(pointer: coarse)').matches
  const connection = navigator as Navigator & { connection?: { saveData?: boolean } }
  disabled = shouldReducePublicMotion({
    reducedMotion,
    coarsePointer,
    saveData: connection.connection?.saveData
  })

  if (!disabled) window.addEventListener('pointermove', handlePointerMove, { passive: true })
})

onBeforeUnmount(() => {
  window.removeEventListener('pointermove', handlePointerMove)
})
</script>

<style scoped>
.signal-trail {
  position: fixed;
  inset: 0;
  z-index: 6;
  overflow: hidden;
  pointer-events: none;
}

.signal-trail__node {
  position: absolute;
  top: -0.75rem;
  left: -0.75rem;
  display: grid;
  grid-template-columns: 1.5rem auto;
  opacity: 0;
  transform: translate3d(-100px, -100px, 0);
}

.signal-trail__node i,
.signal-trail__node b {
  display: grid;
  place-items: center;
  min-height: 1.5rem;
  border: 1px solid var(--public-line-strong);
  font-family: var(--public-font-mono);
  font-size: 0.5rem;
  font-style: normal;
  font-weight: 400;
  letter-spacing: 0.08em;
}

.signal-trail__node i {
  color: var(--public-inverse-bg);
  background: var(--public-accent);
}

.signal-trail__node b {
  min-width: 3.5rem;
  padding-inline: 0.5rem;
  background: var(--public-bg);
  color: var(--public-ink);
  white-space: nowrap;
}

@media (pointer: coarse), (prefers-reduced-motion: reduce) {
  .signal-trail {
    display: none;
  }
}
</style>

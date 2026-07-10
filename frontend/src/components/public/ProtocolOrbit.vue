<template>
  <div
    class="protocol-orbit"
    :class="[`protocol-orbit--scene-${scene + 1}`, { 'protocol-orbit--compact': compact }]"
    role="img"
    :aria-label="label"
  >
    <svg viewBox="0 0 320 320" aria-hidden="true">
      <circle class="protocol-orbit__ring protocol-orbit__ring--outer" cx="160" cy="160" r="144" />
      <circle class="protocol-orbit__ring protocol-orbit__ring--middle" cx="160" cy="160" r="103" />
      <circle class="protocol-orbit__ring protocol-orbit__ring--inner" cx="160" cy="160" r="61" />
      <path class="protocol-orbit__route" d="M55 177C92 67 227 61 269 164C299 237 215 286 148 256C87 229 97 138 160 124C217 112 249 169 220 206" />
      <g class="protocol-orbit__satellites protocol-orbit__satellites--outer">
        <circle cx="160" cy="16" r="7" />
        <circle cx="282" cy="237" r="6" />
        <circle cx="43" cy="244" r="5" />
      </g>
      <g class="protocol-orbit__satellites protocol-orbit__satellites--inner">
        <circle cx="160" cy="57" r="5" />
        <circle cx="248" cy="190" r="5" />
        <circle cx="90" cy="229" r="5" />
      </g>
      <g class="protocol-orbit__crosshair">
        <path d="M160 137V183M137 160H183" />
        <circle cx="160" cy="160" r="17" />
      </g>
    </svg>

    <div class="protocol-orbit__core">
      <span>{{ coreLabel }}</span>
      <strong>{{ sceneCode }}</strong>
    </div>

    <span class="protocol-orbit__tag protocol-orbit__tag--claude">CLAUDE</span>
    <span class="protocol-orbit__tag protocol-orbit__tag--openai">OPENAI</span>
    <span class="protocol-orbit__tag protocol-orbit__tag--gemini">GEMINI</span>
    <span class="protocol-orbit__status">{{ status }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    scene?: number
    label: string
    status: string
    coreLabel?: string
    compact?: boolean
  }>(),
  {
    scene: 0,
    coreLabel: 'POKE',
    compact: false
  }
)

const sceneCode = computed(() => String(props.scene + 1).padStart(2, '0'))
</script>

<style scoped>
.protocol-orbit {
  position: relative;
  width: min(34vw, 30rem);
  aspect-ratio: 1;
  color: var(--public-ink);
  isolation: isolate;
  transition: transform 500ms var(--public-ease), color 300ms var(--public-ease);
}

.protocol-orbit svg {
  width: 100%;
  height: 100%;
  overflow: visible;
}

.protocol-orbit__ring,
.protocol-orbit__route,
.protocol-orbit__crosshair path,
.protocol-orbit__crosshair circle {
  fill: none;
  stroke: currentColor;
  stroke-width: 1;
  vector-effect: non-scaling-stroke;
}

.protocol-orbit__ring {
  opacity: 0.28;
}

.protocol-orbit__ring--middle {
  stroke-dasharray: 5 7;
  opacity: 0.45;
}

.protocol-orbit__ring--inner {
  opacity: 0.75;
}

.protocol-orbit__route {
  stroke: var(--public-accent);
  stroke-dasharray: 2 9;
  opacity: 0.9;
}

.protocol-orbit__satellites {
  fill: var(--public-ink);
  transform-box: fill-box;
  transform-origin: center;
}

.protocol-orbit__satellites--outer {
  animation: public-orbit-clockwise 24s linear infinite;
}

.protocol-orbit__satellites--inner {
  fill: var(--public-accent);
  animation: public-orbit-counter 16s linear infinite;
}

.protocol-orbit__crosshair {
  opacity: 0.82;
}

.protocol-orbit__core {
  position: absolute;
  inset: 50% auto auto 50%;
  display: grid;
  place-items: center;
  width: 5rem;
  aspect-ratio: 1;
  border: 1px solid var(--public-line-strong);
  border-radius: 50%;
  background: var(--public-bg);
  transform: translate(-50%, -50%);
}

.protocol-orbit__core span,
.protocol-orbit__core strong,
.protocol-orbit__tag,
.protocol-orbit__status {
  font-family: var(--public-font-mono);
  text-transform: uppercase;
}

.protocol-orbit__core span {
  color: var(--public-soft);
  font-size: 0.5625rem;
}

.protocol-orbit__core strong {
  color: var(--public-ink);
  font-size: 1.25rem;
  font-weight: 400;
  font-variant-numeric: tabular-nums;
}

.protocol-orbit__tag,
.protocol-orbit__status {
  position: absolute;
  padding: 0.35rem 0.45rem;
  border: 1px solid var(--public-line);
  background: var(--public-bg);
  color: var(--public-muted);
  font-size: 0.5625rem;
  letter-spacing: 0.08em;
  white-space: nowrap;
  transition: transform 500ms var(--public-ease), color 300ms var(--public-ease), background-color 300ms var(--public-ease);
}

.protocol-orbit__tag--claude {
  top: 9%;
  left: 8%;
}

.protocol-orbit__tag--openai {
  right: -1%;
  bottom: 22%;
}

.protocol-orbit__tag--gemini {
  bottom: 3%;
  left: 19%;
}

.protocol-orbit__status {
  top: 34%;
  right: 2%;
  color: var(--public-inverse-bg);
  background: var(--public-accent);
}

.protocol-orbit--scene-2 .protocol-orbit__tag--claude {
  transform: translate(145%, 48%);
}

.protocol-orbit--scene-2 .protocol-orbit__tag--openai {
  transform: translate(-145%, -218%);
}

.protocol-orbit--scene-2 .protocol-orbit__tag--gemini {
  transform: translate(142%, -20%);
}

.protocol-orbit--scene-3 .protocol-orbit__tag--claude {
  transform: translate(42%, 330%);
}

.protocol-orbit--scene-3 .protocol-orbit__tag--openai {
  transform: translate(-185%, -48%);
}

.protocol-orbit--scene-3 .protocol-orbit__tag--gemini {
  transform: translate(175%, -315%);
}

.protocol-orbit--compact {
  width: min(18rem, 72vw);
}

@keyframes public-orbit-clockwise {
  to { transform: rotate(360deg); }
}

@keyframes public-orbit-counter {
  to { transform: rotate(-360deg); }
}

@media (prefers-reduced-motion: reduce) {
  .protocol-orbit__satellites {
    animation: none;
  }
}
</style>

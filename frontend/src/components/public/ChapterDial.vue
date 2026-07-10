<template>
  <div class="chapter-dial" :style="{ '--chapter-progress': `${progress}turn` }">
    <div class="chapter-dial__ring" aria-hidden="true">
      <span>{{ current }}</span>
      <i>/</i>
      <span>{{ count }}</span>
    </div>
    <div class="chapter-dial__controls">
      <button type="button" :aria-label="previousLabel" @click="$emit('previous')">
        <span aria-hidden="true">←</span>
      </button>
      <span>{{ activeLabel }}</span>
      <button type="button" :aria-label="nextLabel" @click="$emit('next')">
        <span aria-hidden="true">→</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  active: number
  total: number
  activeLabel: string
  previousLabel: string
  nextLabel: string
}>()

defineEmits<{
  previous: []
  next: []
}>()

const current = computed(() => String(props.active + 1).padStart(2, '0'))
const count = computed(() => String(props.total).padStart(2, '0'))
const progress = computed(() => (props.active + 1) / Math.max(props.total, 1))
</script>

<style scoped>
.chapter-dial {
  display: flex;
  gap: 0.875rem;
  align-items: center;
}

.chapter-dial__ring {
  position: relative;
  display: flex;
  gap: 0.2rem;
  align-items: center;
  justify-content: center;
  width: 4.5rem;
  aspect-ratio: 1;
  border: 1px solid var(--public-line);
  border-radius: 50%;
  color: var(--public-ink);
  font-family: var(--public-font-mono);
  font-size: 0.625rem;
  font-variant-numeric: tabular-nums;
}

.chapter-dial__ring::after {
  content: '';
  position: absolute;
  inset: -0.25rem;
  border: 1px solid var(--public-accent);
  border-radius: 50%;
  clip-path: polygon(50% 0, 100% 0, 100% 50%, 50% 50%);
  transform: rotate(var(--chapter-progress));
  transition: transform 500ms var(--public-ease);
}

.chapter-dial__ring i {
  color: var(--public-soft);
  font-style: normal;
}

.chapter-dial__controls {
  display: grid;
  grid-template-columns: 2.75rem minmax(6rem, auto) 2.75rem;
  align-items: center;
  border-top: 1px solid var(--public-line);
  border-bottom: 1px solid var(--public-line);
}

.chapter-dial__controls button {
  min-width: 2.75rem;
  min-height: 2.75rem;
  border: 0;
  background: transparent;
  color: var(--public-ink);
  cursor: pointer;
  transition: color 140ms var(--public-ease), transform 140ms var(--public-ease);
}

.chapter-dial__controls button:hover {
  color: var(--public-accent);
  transform: translateX(-0.125rem);
}

.chapter-dial__controls button:last-child:hover {
  transform: translateX(0.125rem);
}

.chapter-dial__controls > span {
  overflow: hidden;
  color: var(--public-muted);
  font-family: var(--public-font-mono);
  font-size: 0.625rem;
  text-align: center;
  text-overflow: ellipsis;
  text-transform: uppercase;
  white-space: nowrap;
}
</style>

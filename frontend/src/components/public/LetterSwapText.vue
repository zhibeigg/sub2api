<template>
  <component :is="as" class="public-letter-swap" :aria-label="text">
    <span class="public-letter-swap__row" aria-hidden="true">
      <span
        v-for="(character, index) in characters"
        :key="`${character}-${index}`"
        class="public-letter-swap__cell"
        :style="{ '--letter-index': index }"
      >
        <span class="public-letter-swap__glyph public-letter-swap__glyph--primary">{{ printable(character) }}</span>
        <span class="public-letter-swap__glyph public-letter-swap__glyph--alternate">{{ printable(character) }}</span>
      </span>
    </span>
  </component>
</template>

<script setup lang="ts">
import { computed } from 'vue'

import { splitCharacters } from './publicMotion'

const props = withDefaults(
  defineProps<{
    text: string
    as?: string
  }>(),
  {
    as: 'span'
  }
)

const characters = computed(() => splitCharacters(props.text))

function printable(character: string): string {
  return character === ' ' ? '\u00a0' : character
}
</script>

<style scoped>
.public-letter-swap {
  display: inline-flex;
  overflow: hidden;
  line-height: 1;
}

.public-letter-swap__row {
  display: inline-flex;
}

.public-letter-swap__cell {
  position: relative;
  display: inline-grid;
  overflow: hidden;
  height: 1em;
  line-height: 1;
}

.public-letter-swap__glyph {
  grid-area: 1 / 1;
  transition: transform 280ms var(--public-ease);
  transition-delay: calc(min(var(--letter-index), 8) * 18ms);
}

.public-letter-swap__glyph--alternate {
  transform: translateY(108%) rotateX(-72deg);
}

.public-letter-swap:hover .public-letter-swap__glyph--primary,
:where(a, button):focus-visible .public-letter-swap .public-letter-swap__glyph--primary {
  transform: translateY(-108%) rotateX(72deg);
}

.public-letter-swap:hover .public-letter-swap__glyph--alternate,
:where(a, button):focus-visible .public-letter-swap .public-letter-swap__glyph--alternate {
  transform: translateY(0) rotateX(0);
}

@media (prefers-reduced-motion: reduce) {
  .public-letter-swap__glyph--alternate {
    display: none;
  }
}
</style>

<template>
  <span class="mono-clock" style="font-variant-numeric: tabular-nums">{{ time }}</span>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'

/**
 * LiveClock — 实时时钟（用于页脚）。可指定时区，默认使用浏览器本地时区。
 */
const props = withDefaults(
  defineProps<{
    timeZone?: string
    label?: string
  }>(),
  {
    timeZone: undefined,
  }
)

const time = ref('')
let timer: ReturnType<typeof setInterval> | null = null

const format = () => {
  try {
    time.value = new Intl.DateTimeFormat('en-GB', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
      timeZone: props.timeZone,
    }).format(new Date())
  } catch {
    time.value = new Date().toLocaleTimeString('en-GB', { hour12: false })
  }
}

onMounted(() => {
  format()
  timer = setInterval(format, 1000)
})

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

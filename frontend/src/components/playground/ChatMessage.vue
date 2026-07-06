<template>
  <div class="group/msg flex gap-3" :class="isUser ? 'flex-row-reverse' : 'flex-row'">
    <!-- Avatar -->
    <div
      class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg text-xs font-semibold"
      :class="
        isUser
          ? 'bg-primary-500/10 text-primary-600 dark:text-primary-400'
          : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-300'
      "
    >
      <Icon :name="isUser ? 'user' : 'sparkles'" size="sm" />
    </div>

    <!-- Bubble -->
    <div class="min-w-0 flex-1" :class="isUser ? 'flex flex-col items-end' : ''">
      <div
        class="inline-block max-w-full rounded-2xl px-4 py-2.5 text-sm leading-relaxed"
        :class="bubbleClass"
      >
        <template v-if="message.error">
          <div class="flex items-start gap-2 text-red-600 dark:text-red-400">
            <Icon name="exclamationTriangle" size="sm" class="mt-0.5 flex-shrink-0" />
            <span class="break-words">{{ message.content }}</span>
          </div>
        </template>
        <template v-else-if="isUser">
          <p class="whitespace-pre-wrap break-words">{{ message.content }}</p>
        </template>
        <template v-else-if="!message.content && streaming">
          <span class="inline-flex gap-1">
            <span class="pk-typing-dot"></span>
            <span class="pk-typing-dot" style="animation-delay: 0.15s"></span>
            <span class="pk-typing-dot" style="animation-delay: 0.3s"></span>
          </span>
        </template>
        <div v-else class="pk-markdown break-words" v-html="rendered"></div>
      </div>

      <!-- Meta + actions -->
      <div
        class="mt-1 flex items-center gap-2 px-1 text-[11px] text-gray-400 opacity-0 transition-opacity group-hover/msg:opacity-100"
        :class="isUser ? 'flex-row-reverse' : ''"
      >
        <button class="hover:text-gray-600 dark:hover:text-gray-200" :title="t('playground.copy')" @click="copy">
          <Icon :name="copied ? 'check' : 'copy'" size="xs" />
        </button>
        <button
          v-if="!isUser && !streaming && !message.error"
          class="hover:text-gray-600 dark:hover:text-gray-200"
          :title="t('playground.regenerate')"
          @click="emit('regenerate')"
        >
          <Icon name="refresh" size="xs" />
        </button>
        <span v-if="metaText" class="tabular-nums">{{ metaText }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import Icon from '@/components/icons/Icon.vue'
import type { PlaygroundMessage } from '@/composables/usePlaygroundConversations'

const props = defineProps<{
  message: PlaygroundMessage
  streaming?: boolean
}>()

const emit = defineEmits<{
  (e: 'regenerate'): void
}>()

const { t } = useI18n()

const isUser = computed(() => props.message.role === 'user')

const bubbleClass = computed(() => {
  if (props.message.error) return 'bg-red-500/5 border border-red-500/20'
  if (isUser.value) return 'bg-primary-500 text-white'
  return 'bg-gray-100 text-gray-800 dark:bg-dark-700/70 dark:text-gray-100'
})

const rendered = computed(() => {
  const html = marked.parse(props.message.content || '', { async: false }) as string
  return DOMPurify.sanitize(html)
})

const metaText = computed(() => {
  const parts: string[] = []
  if (props.message.latencyMs != null) parts.push(`${(props.message.latencyMs / 1000).toFixed(1)}s`)
  if (props.message.usage?.total_tokens != null) parts.push(`${props.message.usage.total_tokens} tok`)
  return parts.join(' · ')
})

const copied = ref(false)
async function copy(): Promise<void> {
  try {
    await navigator.clipboard.writeText(props.message.content)
  } catch {
    const ta = document.createElement('textarea')
    ta.value = props.message.content
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    document.body.removeChild(ta)
  }
  copied.value = true
  setTimeout(() => (copied.value = false), 1400)
}
</script>

<style scoped>
.pk-typing-dot {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
  opacity: 0.4;
  animation: pk-typing 1s ease-in-out infinite;
}
@keyframes pk-typing {
  0%,
  60%,
  100% {
    opacity: 0.25;
    transform: translateY(0);
  }
  30% {
    opacity: 0.9;
    transform: translateY(-2px);
  }
}
@media (prefers-reduced-motion: reduce) {
  .pk-typing-dot {
    animation: none;
  }
}

/* Markdown content styling (scoped, deep) */
.pk-markdown :deep(p) {
  margin: 0 0 0.6em;
}
.pk-markdown :deep(p:last-child) {
  margin-bottom: 0;
}
.pk-markdown :deep(pre) {
  margin: 0.5em 0;
  padding: 0.75em 0.9em;
  border-radius: 0.5rem;
  background: rgb(0 0 0 / 0.06);
  overflow-x: auto;
  font-size: 0.85em;
}
:global(.dark) .pk-markdown :deep(pre) {
  background: rgb(0 0 0 / 0.35);
}
.pk-markdown :deep(code) {
  font-family: ui-monospace, 'Cascadia Code', Menlo, Consolas, monospace;
}
.pk-markdown :deep(:not(pre) > code) {
  padding: 0.1em 0.35em;
  border-radius: 0.3rem;
  background: rgb(0 0 0 / 0.06);
  font-size: 0.88em;
}
:global(.dark) .pk-markdown :deep(:not(pre) > code) {
  background: rgb(255 255 255 / 0.1);
}
.pk-markdown :deep(ul),
.pk-markdown :deep(ol) {
  margin: 0.4em 0;
  padding-left: 1.3em;
}
.pk-markdown :deep(a) {
  color: var(--tw-prose-links, #0d9488);
  text-decoration: underline;
}
.pk-markdown :deep(table) {
  border-collapse: collapse;
  margin: 0.5em 0;
}
.pk-markdown :deep(th),
.pk-markdown :deep(td) {
  border: 1px solid rgb(0 0 0 / 0.15);
  padding: 0.3em 0.6em;
}
</style>

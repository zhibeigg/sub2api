<template>
  <div class="flex h-full min-h-0 flex-col gap-3">
    <!-- Columns -->
    <div class="grid min-h-0 flex-1 gap-3" :style="gridStyle">
      <div
        v-for="(col, idx) in columns"
        :key="col.id"
        class="flex min-h-0 flex-col overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800/50"
      >
        <!-- Column header -->
        <div class="flex items-center gap-2 border-b border-gray-100 p-2 dark:border-dark-700">
          <KeyModelPicker
            v-model:key-id="col.keyId"
            v-model:option="col.option"
            v-model:resolved-key="col.resolvedKey"
            capability="chat"
            class="min-w-0 flex-1"
          />
          <button
            v-if="columns.length > 1"
            class="flex-shrink-0 text-gray-400 hover:text-red-500"
            :title="t('common.delete')"
            @click="removeColumn(idx)"
          >
            <Icon name="x" size="sm" />
          </button>
        </div>

        <!-- Column output -->
        <div :ref="(el) => setScrollRef(el as Element | null, idx)" class="min-h-0 flex-1 space-y-2 overflow-y-auto p-3 text-sm">
          <div v-if="col.error" class="flex items-start gap-1.5 text-red-500">
            <Icon name="exclamationTriangle" size="xs" class="mt-0.5 flex-shrink-0" />
            <span class="break-words">{{ col.content }}</span>
          </div>
          <div v-else-if="col.content" class="pk-markdown break-words" v-html="renderMd(col.content)"></div>
          <span v-else-if="col.streaming" class="text-gray-400">{{ t('playground.thinking') }}</span>
          <span v-else class="text-gray-300 dark:text-gray-600">{{ t('playground.compareWaiting') }}</span>
        </div>

        <!-- Column meta -->
        <div
          v-if="col.latencyMs != null || col.usage"
          class="flex items-center gap-2 border-t border-gray-100 px-3 py-1.5 text-[11px] text-gray-400 dark:border-dark-700"
        >
          <span v-if="col.latencyMs != null">{{ (col.latencyMs / 1000).toFixed(1) }}s</span>
          <span v-if="col.usage?.total_tokens != null">· {{ col.usage.total_tokens }} tok</span>
        </div>
      </div>

      <!-- Add column -->
      <button
        v-if="columns.length < 4"
        class="flex min-h-[120px] flex-col items-center justify-center rounded-xl border-2 border-dashed border-gray-200 text-gray-400 transition-colors hover:border-primary-400 hover:text-primary-500 dark:border-dark-600"
        @click="addColumn"
      >
        <Icon name="plus" size="lg" />
        <span class="mt-1 text-xs">{{ t('playground.addColumn') }}</span>
      </button>
    </div>

    <!-- Shared prompt input -->
    <div class="flex-shrink-0">
      <div
        class="flex items-end gap-2 rounded-2xl border border-gray-200 bg-white p-2 shadow-sm focus-within:border-primary-400 dark:border-dark-600 dark:bg-dark-800"
      >
        <textarea
          v-model="prompt"
          rows="1"
          :placeholder="t('playground.comparePlaceholder')"
          class="max-h-32 min-h-[40px] flex-1 resize-none border-0 bg-transparent px-2 py-2 text-sm outline-none placeholder:text-gray-400 dark:text-gray-100"
          @keydown="onKeydown"
        ></textarea>
        <button
          v-if="!anyStreaming"
          class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl bg-primary-500 text-white hover:bg-primary-600 disabled:opacity-40"
          :disabled="!canSend"
          :title="t('playground.send')"
          @click="runAll"
        >
          <Icon name="arrowUp" size="sm" :stroke-width="2" />
        </button>
        <button
          v-else
          class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl bg-gray-800 text-white dark:bg-gray-600"
          :title="t('playground.stop')"
          @click="stopAll"
        >
          <Icon name="x" size="sm" :stroke-width="2" />
        </button>
      </div>
      <p class="mt-1.5 px-2 text-[11px] text-gray-400">{{ t('playground.compareHint') }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import Icon from '@/components/icons/Icon.vue'
import KeyModelPicker from './KeyModelPicker.vue'
import playgroundAPI, { type ChatUsage } from '@/api/playground'
import { uid } from '@/composables/usePlaygroundConversations'
import type { PlaygroundModelOption } from '@/types/playground'
import type { PlaygroundParameterValues } from './playgroundUiTypes'

const props = defineProps<{
  parameters: PlaygroundParameterValues
}>()

const { t } = useI18n()

interface CompareColumn {
  id: string
  keyId: number | null
  option: PlaygroundModelOption | null
  resolvedKey: string
  content: string
  streaming: boolean
  error: boolean
  latencyMs: number | null
  usage: ChatUsage | null
  controller: AbortController | null
}

function makeColumn(): CompareColumn {
  return reactive({
    id: uid(),
    keyId: null as number | null,
    option: null as PlaygroundModelOption | null,
    resolvedKey: '',
    content: '',
    streaming: false,
    error: false,
    latencyMs: null as number | null,
    usage: null as ChatUsage | null,
    controller: null as AbortController | null
  }) as CompareColumn
}

function renderMd(content: string): string {
  return DOMPurify.sanitize(marked.parse(content || '', { async: false }) as string)
}

const columns = ref<CompareColumn[]>([makeColumn(), makeColumn()])
const prompt = ref('')
const scrollRefs: (HTMLElement | null)[] = []

const gridStyle = computed(() => ({
  gridTemplateColumns: `repeat(${Math.min(columns.value.length + (columns.value.length < 4 ? 1 : 0), 4)}, minmax(0, 1fr))`
}))

const anyStreaming = computed(() => columns.value.some((c) => c.streaming))
const canSend = computed(
  () => !!prompt.value.trim() && columns.value.some((column) => column.resolvedKey && column.option)
)

function setScrollRef(el: Element | null, idx: number): void {
  scrollRefs[idx] = el as HTMLElement | null
}

function addColumn(): void {
  if (columns.value.length < 4) columns.value.push(makeColumn())
}

function removeColumn(idx: number): void {
  columns.value[idx]?.controller?.abort()
  columns.value.splice(idx, 1)
}

function onKeydown(e: KeyboardEvent): void {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    if (canSend.value) runAll()
  } else if (e.key === 'Escape' && anyStreaming.value) {
    stopAll()
  }
}

async function runColumn(col: CompareColumn, text: string): Promise<void> {
  if (!col.resolvedKey || !col.option) return
  col.content = ''
  col.error = false
  col.usage = null
  col.latencyMs = null
  col.streaming = true
  col.controller = new AbortController()
  const started = performance.now()
  const idx = columns.value.indexOf(col)
  try {
    const option = col.option
    await playgroundAPI.streamChat({
      apiKey: col.resolvedKey,
      groupId: option.group_id,
      model: option.model,
      messages: [
        ...(props.parameters.systemPrompt.trim() ? [{ role: 'system' as const, content: props.parameters.systemPrompt.trim() }] : []),
        { role: 'user' as const, content: text }
      ],
      temperature: props.parameters.temperature,
      topP: props.parameters.topP,
      maxTokens: props.parameters.maxTokens,
      reasoningEffort: !props.parameters.reasoningEffort || props.parameters.reasoningEffort === 'none' ? undefined : props.parameters.reasoningEffort,
      signal: col.controller.signal,
      onDelta: (chunk) => {
        col.content += chunk
        nextTick(() => {
          const el = scrollRefs[idx]
          if (el) el.scrollTop = el.scrollHeight
        })
      },
      onUsage: (usage) => {
        col.usage = usage
      }
    } as Parameters<typeof playgroundAPI.streamChat>[0] & {
      topP?: number
      reasoningEffort?: string
    })
    col.latencyMs = performance.now() - started
  } catch (err) {
    const e = err as Error
    if (e.name === 'AbortError') {
      if (!col.content) col.content = t('playground.stopped')
    } else {
      col.error = true
      col.content = e.message || t('playground.requestFailed')
    }
  } finally {
    col.streaming = false
    col.controller = null
  }
}

function runAll(): void {
  const text = prompt.value.trim()
  if (!text || !canSend.value) return
  columns.value.forEach((col) => {
    if (col.resolvedKey && col.option) runColumn(col, text)
  })
}

function stopAll(): void {
  columns.value.forEach((col) => col.controller?.abort())
}
</script>

<style scoped>
.pk-markdown :deep(p) {
  margin: 0 0 0.5em;
}
.pk-markdown :deep(pre) {
  margin: 0.4em 0;
  padding: 0.6em;
  border-radius: 0.4rem;
  background: rgb(0 0 0 / 0.06);
  overflow-x: auto;
  font-size: 0.85em;
}
:global(.dark) .pk-markdown :deep(pre) {
  background: rgb(0 0 0 / 0.35);
}
.pk-markdown :deep(:not(pre) > code) {
  padding: 0.1em 0.3em;
  border-radius: 0.25rem;
  background: rgb(0 0 0 / 0.06);
}
:global(.dark) .pk-markdown :deep(:not(pre) > code) {
  background: rgb(255 255 255 / 0.1);
}
</style>

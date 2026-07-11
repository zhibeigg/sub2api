<template>
  <div class="flex h-full min-h-0 gap-4">
    <!-- Conversation list -->
    <aside class="hidden w-56 flex-shrink-0 flex-col md:flex">
      <button class="btn btn-primary mb-3 w-full" @click="newConversation">
        <Icon name="plus" size="sm" />
        {{ t('playground.newChat') }}
      </button>
      <div class="min-h-0 flex-1 space-y-1 overflow-y-auto pr-1">
        <div
          v-for="c in conversations"
          :key="c.id"
          class="group/conv flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-sm transition-colors"
          :class="
            c.id === activeId
              ? 'bg-primary-500/10 text-primary-600 dark:text-primary-400'
              : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-800'
          "
          @click="activeId = c.id"
        >
          <Icon name="chat" size="sm" class="flex-shrink-0 opacity-70" />
          <span class="min-w-0 flex-1 truncate">{{ c.title || t('playground.untitled') }}</span>
          <button
            class="opacity-0 transition-opacity hover:text-red-500 group-hover/conv:opacity-100"
            :title="t('common.delete')"
            @click.stop="removeConversation(c.id)"
          >
            <Icon name="trash" size="xs" />
          </button>
        </div>
        <p v-if="conversations.length === 0" class="px-3 py-4 text-center text-xs text-gray-400">
          {{ t('playground.noConversations') }}
        </p>
      </div>
    </aside>

    <!-- Chat area -->
    <div class="flex min-h-0 min-w-0 flex-1 flex-col">
      <!-- Messages -->
      <div ref="scrollEl" class="min-h-0 flex-1 space-y-5 overflow-y-auto px-1 py-2">
        <div
          v-if="messages.length === 0"
          class="flex h-full flex-col items-center justify-center text-center"
        >
          <div class="mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-primary-500/10">
            <Icon name="sparkles" size="lg" class="text-primary-500" />
          </div>
          <p class="mb-1 text-lg font-medium text-gray-700 dark:text-gray-200">
            {{ t('playground.emptyTitle') }}
          </p>
          <p class="mb-5 text-sm text-gray-400">{{ t('playground.emptyHint') }}</p>
          <div class="flex flex-wrap justify-center gap-2">
            <button
              v-for="(ex, i) in examples"
              :key="i"
              class="rounded-full border border-gray-200 px-3 py-1.5 text-xs text-gray-600 transition-colors hover:border-primary-400 hover:text-primary-600 dark:border-dark-600 dark:text-gray-300"
              @click="useExample(ex)"
            >
              {{ ex }}
            </button>
          </div>
        </div>

        <ChatMessage
          v-for="(m, idx) in messages"
          :key="m.id"
          :message="m"
          :streaming="streaming && idx === messages.length - 1 && m.role === 'assistant'"
          @regenerate="regenerate"
        />
      </div>

      <!-- Input -->
      <div class="mt-3 flex-shrink-0">
        <div
          class="flex items-end gap-2 rounded-2xl border border-gray-200 bg-white p-2 shadow-sm transition-colors focus-within:border-primary-400 dark:border-dark-600 dark:bg-dark-800"
        >
          <textarea
            ref="inputEl"
            v-model="input"
            :placeholder="t('playground.inputPlaceholder')"
            rows="1"
            class="max-h-40 min-h-[40px] flex-1 resize-none border-0 bg-transparent px-2 py-2 text-sm text-gray-800 outline-none placeholder:text-gray-400 dark:text-gray-100"
            @input="autoGrow"
            @keydown="onKeydown"
          ></textarea>
          <button
            v-if="!streaming"
            class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl bg-primary-500 text-white transition-colors hover:bg-primary-600 disabled:cursor-not-allowed disabled:opacity-40"
            :disabled="!canSend"
            :title="t('playground.send')"
            @click="send"
          >
            <Icon name="arrowUp" size="sm" :stroke-width="2" />
          </button>
          <button
            v-else
            class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl bg-gray-800 text-white transition-colors hover:bg-gray-900 dark:bg-gray-600"
            :title="t('playground.stop')"
            @click="stop"
          >
            <Icon name="x" size="sm" :stroke-width="2" />
          </button>
        </div>
        <p class="mt-1.5 px-2 text-[11px] text-gray-400">{{ t('playground.consumeHint') }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ChatMessage from './ChatMessage.vue'
import playgroundAPI, { type ChatMessagePayload } from '@/api/playground'
import { keysAPI } from '@/api/keys'
import {
  usePlaygroundConversations,
  uid,
  type PlaygroundMessage
} from '@/composables/usePlaygroundConversations'
import { usePlaygroundSettings } from '@/composables/usePlaygroundSettings'
import type { PlaygroundModelOption } from '@/types/playground'

const props = defineProps<{
  keyId: number | null
  resolvedKey: string
  option: PlaygroundModelOption | null
}>()

const { t } = useI18n()
const convStore = usePlaygroundConversations()
const { conversations, activeId } = convStore
const settings = usePlaygroundSettings()

const input = ref('')
const streaming = ref(false)
const scrollEl = ref<HTMLElement | null>(null)
const inputEl = ref<HTMLTextAreaElement | null>(null)
let controller: AbortController | null = null

const examples = computed<string[]>(() => [
  t('playground.example1'),
  t('playground.example2'),
  t('playground.example3')
])

const messages = computed<PlaygroundMessage[]>(() => convStore.activeConversation()?.messages ?? [])

const canSend = computed(() => !!input.value.trim() && !!props.resolvedKey && !!props.option)

function newConversation(): void {
  convStore.create(props.option ?? undefined, props.keyId ?? undefined)
}

function removeConversation(id: string): void {
  convStore.remove(id)
}

function ensureConversation() {
  let conv = convStore.activeConversation()
  if (!conv) conv = convStore.create(props.option ?? undefined, props.keyId ?? undefined)
  return conv
}

function scrollToBottom(): void {
  nextTick(() => {
    if (scrollEl.value) scrollEl.value.scrollTop = scrollEl.value.scrollHeight
  })
}

function autoGrow(): void {
  const el = inputEl.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 160) + 'px'
}

function onKeydown(e: KeyboardEvent): void {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    if (canSend.value) send()
  } else if (e.key === 'Escape' && streaming.value) {
    stop()
  }
}

function useExample(text: string): void {
  input.value = text
  nextTick(() => {
    autoGrow()
    inputEl.value?.focus()
  })
}

function buildPayload(conv: ReturnType<typeof ensureConversation>): ChatMessagePayload[] {
  const payload: ChatMessagePayload[] = []
  const sys = settings.systemPrompt.value.trim()
  if (sys) payload.push({ role: 'system', content: sys })
  for (const m of conv.messages) {
    if (m.error) continue
    payload.push({ role: m.role, content: m.content })
  }
  return payload
}

async function runCompletion(conv: ReturnType<typeof ensureConversation>): Promise<void> {
  const option = conv.option ?? props.option
  if (!option) return
  const routeKeyId = conv.apiKeyId ?? props.keyId
  let routeKey = props.resolvedKey

  // Snapshot the request payload and selected route before appending the assistant placeholder.
  const payload = buildPayload(conv)

  const assistant: PlaygroundMessage = {
    id: uid(),
    role: 'assistant',
    content: '',
    model: option.model,
    option
  }
  conv.messages.push(assistant)
  streaming.value = true
  scrollToBottom()

  controller = new AbortController()
  const started = performance.now()
  try {
    if (routeKeyId != null && routeKeyId !== props.keyId) {
      routeKey = (await keysAPI.getById(routeKeyId)).key
    }
    if (!routeKey) throw new Error(t('playground.selectKey'))
    await playgroundAPI.streamChat({
      apiKey: routeKey,
      groupId: option.group_id,
      model: option.model,
      messages: payload,
      temperature: settings.temperature.value,
      maxTokens: settings.maxTokens.value,
      signal: controller.signal,
      onDelta: (chunk) => {
        assistant.content += chunk
        scrollToBottom()
      },
      onUsage: (usage) => {
        assistant.usage = usage
      }
    })
    assistant.latencyMs = performance.now() - started
  } catch (err) {
    const e = err as Error
    if (e.name === 'AbortError') {
      if (!assistant.content) assistant.content = t('playground.stopped')
    } else {
      assistant.error = true
      assistant.content = e.message || t('playground.requestFailed')
    }
  } finally {
    streaming.value = false
    controller = null
    convStore.touch(conv.id)
    scrollToBottom()
  }
}

async function send(): Promise<void> {
  const text = input.value.trim()
  if (!text || !canSend.value) return
  const conv = ensureConversation()

  conv.messages.push({ id: uid(), role: 'user', content: text })
  if (!conv.title) {
    conv.title = text.slice(0, 24)
  }
  conv.apiKeyId = props.keyId ?? undefined
  conv.model = props.option?.model
  conv.option = props.option ?? undefined
  input.value = ''
  nextTick(autoGrow)

  await runCompletion(conv)
}

async function regenerate(): Promise<void> {
  const conv = convStore.activeConversation()
  if (!conv || streaming.value) return
  // Drop the trailing assistant message, then re-run.
  if (conv.messages[conv.messages.length - 1]?.role === 'assistant') {
    conv.messages.pop()
  }
  await runCompletion(conv)
}

function stop(): void {
  controller?.abort()
}

watch(activeId, () => scrollToBottom())
</script>

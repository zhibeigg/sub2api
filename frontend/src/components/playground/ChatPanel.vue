<template>
  <div class="flex h-full min-h-0 gap-4">
    <ConversationSidebar class="hidden md:flex" />

    <Teleport to="body">
      <div v-if="mobileSidebarOpen" class="fixed inset-0 z-50 md:hidden" role="dialog" aria-modal="true" :aria-label="t('playground.conversations')">
        <button class="absolute inset-0 bg-black/40" :aria-label="t('common.close')" @click="mobileSidebarOpen = false"></button>
        <ConversationSidebar class="relative h-full w-[min(88vw,320px)] bg-white p-3 shadow-xl dark:bg-dark-900" />
      </div>
    </Teleport>

    <div class="flex min-h-0 min-w-0 flex-1 flex-col">
      <header class="mb-2 flex min-h-11 items-center justify-between gap-2 md:hidden">
        <button class="flex min-h-11 items-center gap-2 rounded-lg px-2 text-sm text-gray-600 focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-gray-300" @click="mobileSidebarOpen = true">
          <Icon name="menu" size="sm" />
          <span class="max-w-44 truncate">{{ activeConversationTitle }}</span>
        </button>
        <ExportMenu />
      </header>

      <div ref="scrollEl" class="min-h-0 flex-1 space-y-5 overflow-y-auto px-1 py-2">
        <div v-if="messages.length === 0" class="flex h-full flex-col items-center justify-center text-center">
          <div class="mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-primary-500/10">
            <Icon name="sparkles" size="lg" class="text-primary-500" />
          </div>
          <p class="mb-1 text-lg font-medium text-gray-700 dark:text-gray-200">{{ t('playground.emptyTitle') }}</p>
          <p class="mb-5 text-sm text-gray-400">{{ t('playground.emptyHint') }}</p>
          <div class="flex flex-wrap justify-center gap-2">
            <button v-for="(example, index) in examples" :key="index" class="min-h-11 rounded-full border border-gray-200 px-3 text-xs text-gray-600 outline-none transition-colors hover:border-primary-400 hover:text-primary-600 focus-visible:ring-2 focus-visible:ring-primary-500 dark:border-dark-600 dark:text-gray-300" @click="useExample(example)">
              {{ example }}
            </button>
          </div>
        </div>

        <ChatMessage
          v-for="(message, index) in messages"
          :key="message.id"
          :message="message"
          :streaming="streaming && index === messages.length - 1 && message.role === 'assistant'"
          @regenerate="regenerate"
        />
      </div>

      <div class="mt-3 flex-shrink-0">
        <div class="rounded-2xl border border-gray-200 bg-white p-2 shadow-sm transition-colors focus-within:border-primary-400 dark:border-dark-600 dark:bg-dark-800">
          <div class="flex flex-wrap items-end gap-1">
            <textarea
              ref="inputEl"
              v-model="input"
              :placeholder="t('playground.inputPlaceholder')"
              rows="1"
              class="max-h-40 min-h-11 flex-1 resize-none border-0 bg-transparent px-2 py-2.5 text-sm text-gray-800 outline-none placeholder:text-gray-400 dark:text-gray-100"
              @input="autoGrow"
              @keydown="onKeydown"
              @paste="onComposerPaste"
            ></textarea>
            <PlaygroundAttachmentComposer
              ref="attachmentComposer"
              v-model="attachments"
              :allow-images="supportsImageAttachments"
              class="flex-shrink-0"
            />
            <button
              v-if="!streaming"
              class="flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-xl bg-primary-500 text-white outline-none transition-colors hover:bg-primary-600 focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-40"
              :disabled="!canSend"
              :title="t('playground.send')"
              @click="send"
            >
              <Icon name="arrowUp" size="sm" :stroke-width="2" />
            </button>
            <button v-else class="flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-xl bg-gray-800 text-white outline-none hover:bg-gray-900 focus-visible:ring-2 focus-visible:ring-primary-500 dark:bg-gray-600" :title="t('playground.stop')" @click="stop">
              <Icon name="x" size="sm" :stroke-width="2" />
            </button>
          </div>
        </div>
        <div class="mt-1.5 flex items-center justify-between gap-2 px-2 text-[11px] text-gray-400">
          <span>{{ t('playground.consumeHint') }}</span>
          <span v-if="activeTools.length" role="status" aria-live="polite">{{ activeTools.join(' · ') }}</span>
        </div>
      </div>
    </div>

    <div v-if="undoState" class="fixed bottom-4 left-1/2 z-50 flex min-h-11 -translate-x-1/2 items-center gap-3 rounded-xl bg-gray-900 px-4 py-2 text-sm text-white shadow-xl" role="status" aria-live="polite">
      <span>{{ undoState.label }}</span>
      <button class="min-h-11 font-semibold text-primary-300 outline-none focus-visible:ring-2 focus-visible:ring-primary-400" @click="undoDelete">{{ t('playground.undo') }}</button>
    </div>

    <ConfirmDialog
      :show="showClearDialog"
      :title="t('playground.clearConversations')"
      :message="t('playground.clearConversationsConfirm')"
      :confirm-text="t('playground.clearConversations')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmClearConversations"
      @cancel="showClearDialog = false"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'
import Icon from '@/components/icons/Icon.vue'
import ChatMessage from './ChatMessage.vue'
import PlaygroundAttachmentComposer from './PlaygroundAttachmentComposer.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import playgroundAPI, {
  type ChatMessageContent,
  type ChatMessagePayload,
  type ResponsesInputItem,
  type ResponsesInputContentPart
} from '@/api/playground'
import { keysAPI } from '@/api/keys'
import { usePlaygroundConversations, uid, type PlaygroundMessage, type PlaygroundConversation } from '@/composables/usePlaygroundConversations'
import type { PlaygroundAttachment, PlaygroundContentPart, PlaygroundModelOption, PlaygroundToolActivity } from '@/types/playground'
import { deletePlaygroundAttachments, readPlaygroundAttachment } from '@/utils/playgroundAttachments'
import type { PlaygroundParameterValues } from './playgroundUiTypes'

interface ExtendedMessage extends PlaygroundMessage {
  attachments?: PlaygroundAttachment[]
  toolActivities?: PlaygroundToolActivity[]
}


const props = defineProps<{
  keyId: number | null
  resolvedKey: string
  option: PlaygroundModelOption | null
  parameters: PlaygroundParameterValues
}>()

const { t } = useI18n()
const convStore = usePlaygroundConversations()
const { conversations, activeId } = convStore
const input = ref('')
const attachments = ref<PlaygroundAttachment[]>([])
const streaming = ref(false)
const mobileSidebarOpen = ref(false)
const scrollEl = ref<HTMLElement | null>(null)
const inputEl = ref<HTMLTextAreaElement | null>(null)
const attachmentComposer = ref<{ handlePaste: (event: ClipboardEvent) => void } | null>(null)
const editingId = ref<string | null>(null)
const editingTitle = ref('')
const exportOpen = ref(false)
const showClearDialog = ref(false)
const undoState = ref<{ id: string; label: string; timer: ReturnType<typeof setTimeout> } | null>(null)
let controller: AbortController | null = null

const examples = computed(() => [t('playground.example1'), t('playground.example2'), t('playground.example3')])
const messages = computed<ExtendedMessage[]>(() => (convStore.activeConversation()?.messages ?? []) as ExtendedMessage[])
const activeConversationTitle = computed(() => convStore.activeConversation()?.title || t('playground.untitled'))
const canSend = computed(() => (!!input.value.trim() || attachments.value.some((item) => item.status === 'ready')) && !attachments.value.some((item) => item.status === 'reading') && !!props.resolvedKey && !!props.option)
const supportsImageAttachments = computed(() => {
  if (!props.option?.features) return false
  if (Array.isArray(props.option.features)) {
    return props.option.features.includes('attachments') || props.option.features.includes('vision') || props.option.features.includes('image_input')
  }
  return props.option.features.image_input === true
})
const activeTools = computed(() => {
  const labels: string[] = []
  if (props.parameters.webSearch && props.option && optionSupports(props.option, 'web_search')) labels.push(t('playground.webSearch'))
  if (props.parameters.codeExecution && props.option && optionSupports(props.option, 'code_execution')) labels.push(t('playground.codeExecution'))
  if (props.parameters.webFetch) labels.push(t('playground.webFetch'))
  return labels
})

function newConversation(): void {
  convStore.create(props.option ?? undefined, props.keyId ?? undefined)
  mobileSidebarOpen.value = false
}

function selectConversation(id: string): void {
  activeId.value = id
  mobileSidebarOpen.value = false
}

function beginRename(conversation: PlaygroundConversation): void {
  editingId.value = conversation.id
  editingTitle.value = conversation.title || t('playground.untitled')
  nextTick(() => document.querySelector<HTMLInputElement>(`[data-rename-id="${conversation.id}"]`)?.select())
}

function commitRename(): void {
  if (!editingId.value) return
  convStore.rename(editingId.value, editingTitle.value.trim().slice(0, 80))
  editingId.value = null
}

function removeConversation(id: string): void {
  if (!convStore.scheduleRemove(id, 5000)) return
  clearUndo()
  undoState.value = {
    id,
    label: t('playground.conversationDeleted'),
    timer: setTimeout(() => { undoState.value = null }, 5000)
  }
}

function undoDelete(): void {
  if (!undoState.value) return
  const { id, timer } = undoState.value
  clearTimeout(timer)
  convStore.undoRemove(id)
  undoState.value = null
}

function clearUndo(): void {
  if (undoState.value) clearTimeout(undoState.value.timer)
  undoState.value = null
}

function clearConversations(): void {
  if (conversations.value.length) showClearDialog.value = true
}

function confirmClearConversations(): void {
  convStore.clearAll()
  showClearDialog.value = false
}

function ensureConversation(): PlaygroundConversation {
  return convStore.activeConversation() ?? convStore.create(props.option ?? undefined, props.keyId ?? undefined)
}

function scrollToBottom(): void {
  nextTick(() => { if (scrollEl.value) scrollEl.value.scrollTop = scrollEl.value.scrollHeight })
}

function autoGrow(): void {
  const element = inputEl.value
  if (!element) return
  element.style.height = 'auto'
  element.style.height = `${Math.min(element.scrollHeight, 160)}px`
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    if (canSend.value) void send()
  } else if (event.key === 'Escape' && streaming.value) stop()
}

function onComposerPaste(event: ClipboardEvent): void {
  if (Array.from(event.clipboardData?.items ?? []).some((item) => item.type.startsWith('image/'))) {
    attachmentComposer.value?.handlePaste(event)
  }
}

function useExample(text: string): void {
  input.value = text
  nextTick(() => { autoGrow(); inputEl.value?.focus() })
}

async function messageContent(message: ExtendedMessage): Promise<ChatMessageContent> {
  if (!message.attachments?.length) return message.content
  const parts: PlaygroundContentPart[] = []
  if (message.content) parts.push({ type: 'text', text: message.content })
  for (const attachment of message.attachments) {
    if (attachment.status === 'error' || attachment.status === 'reading') continue
    const payload = await readPlaygroundAttachment(attachment.id).catch(() => null)
    if (!payload) {
      attachment.status = 'missing'
      continue
    }
    attachment.status = 'ready'
    if (payload.encoding === 'data-url') parts.push({ type: 'image_url', image_url: { url: payload.data } })
    else parts.push({ type: 'text', text: `[${attachment.name}]\n${payload.data}` })
  }
  return parts.length ? parts : message.content
}

async function buildPayload(conversation: PlaygroundConversation): Promise<ChatMessagePayload[]> {
  const payload: ChatMessagePayload[] = []
  if (props.parameters.systemPrompt.trim()) payload.push({ role: 'system', content: props.parameters.systemPrompt.trim() })
  for (const rawMessage of conversation.messages as ExtendedMessage[]) {
    if (!rawMessage.error) payload.push({ role: rawMessage.role, content: await messageContent(rawMessage) })
  }
  return payload
}

function optionSupports(option: PlaygroundModelOption, feature: string): boolean {
  if (Array.isArray(option.features)) return option.features.includes(feature)
  return option.features?.[feature] === true
}

function selectedTools(option: PlaygroundModelOption): Array<Record<string, unknown>> {
  const tools: Array<Record<string, unknown>> = []
  if (props.parameters.webSearch && optionSupports(option, 'web_search')) tools.push({ type: 'web_search' })
  if (props.parameters.codeExecution && optionSupports(option, 'code_execution')) tools.push({ type: 'code_interpreter', container: { type: 'auto' } })
  return tools
}

function responsesInput(messages: ChatMessagePayload[]): ResponsesInputItem[] {
  return messages.map((message) => {
    if (typeof message.content === 'string') return { role: message.role, content: message.content }
    const content: ResponsesInputContentPart[] = message.content.flatMap((part): ResponsesInputContentPart[] => {
      if (part.type === 'text') return [{ type: 'input_text', text: part.text }]
      if (part.type === 'image_url') {
        const source = typeof part.image_url === 'string' ? part.image_url : part.image_url.url
        const detail = typeof part.image_url === 'string' ? undefined : part.image_url.detail
        return [{ type: 'input_image', image_url: source, detail }]
      }
      return [{
        type: 'input_file',
        file_data: part.file.file_data,
        file_id: part.file.file_id,
        filename: part.file.filename
      }]
    })
    return { role: message.role, content }
  })
}

function extractUrls(text: string): string[] {
  const matches = text.match(/https?:\/\/[^\s<>)\]}"']+/gi) ?? []
  return [...new Set(matches)].slice(0, 3)
}

async function appendFetchedContext(payload: ChatMessagePayload[], text: string, routeKey: string, option: PlaygroundModelOption): Promise<void> {
  if (!props.parameters.webFetch) return
  const urls = extractUrls(text)
  if (!urls.length) return
  const pages = await playgroundAPI.fetchWebContent({ apiKey: routeKey, groupId: option.group_id, urls, signal: controller?.signal })
  if (pages.length) payload.splice(Math.max(0, payload.length - 1), 0, {
    role: 'system',
    content: `${t('playground.webFetchContext')}\n\n${pages.map((page) => `URL: ${page.url}\n${page.title ? `${page.title}\n` : ''}${page.content}`).join('\n\n---\n\n')}`
  })
}

async function runCompletion(conversation: PlaygroundConversation, sourceText = ''): Promise<void> {
  const option = conversation.option ?? props.option
  if (!option) return
  const routeKeyId = conversation.apiKeyId ?? props.keyId
  let routeKey = props.resolvedKey
  const payload = await buildPayload(conversation)
  const assistant: ExtendedMessage = { id: uid(), role: 'assistant', content: '', model: option.model, option, toolActivities: [] }
  conversation.messages.push(assistant)
  streaming.value = true
  scrollToBottom()
  controller = new AbortController()
  const started = performance.now()

  try {
    if (routeKeyId != null && routeKeyId !== props.keyId) routeKey = (await keysAPI.getById(routeKeyId)).key
    if (!routeKey) throw new Error(t('playground.selectKey'))
    await appendFetchedContext(payload, sourceText, routeKey, option)
    const reasoningEffort = !props.parameters.reasoningEffort || props.parameters.reasoningEffort === 'none'
      ? undefined
      : props.parameters.reasoningEffort
    const onDelta = (chunk: string) => { assistant.content += chunk; scrollToBottom() }
    const onUsage = (usage: PlaygroundMessage['usage']) => { assistant.usage = usage }
    const tools = selectedTools(option)
    const useResponses = optionSupports(option, 'responses') && (tools.length > 0 || !!reasoningEffort)
    if (useResponses) {
      await playgroundAPI.streamResponses({
        apiKey: routeKey,
        groupId: option.group_id,
        model: option.model,
        input: responsesInput(payload),
        temperature: props.parameters.temperature,
        topP: props.parameters.topP,
        maxOutputTokens: props.parameters.maxTokens,
        reasoningEffort,
        tools,
        signal: controller.signal,
        onDelta,
        onUsage,
        onToolActivity: (activity) => {
          const index = assistant.toolActivities?.findIndex((item) => item.id === activity.id) ?? -1
          if (index >= 0) assistant.toolActivities?.splice(index, 1, activity)
          else assistant.toolActivities?.push(activity)
          scrollToBottom()
        }
      })
    } else {
      await playgroundAPI.streamChat({
        apiKey: routeKey,
        groupId: option.group_id,
        model: option.model,
        messages: payload,
        temperature: props.parameters.temperature,
        topP: props.parameters.topP,
        maxTokens: props.parameters.maxTokens,
        reasoningEffort,
        signal: controller.signal,
        onDelta,
        onUsage
      })
    }
    assistant.latencyMs = performance.now() - started
  } catch (error) {
    const requestError = error as Error
    if (requestError.name === 'AbortError') {
      if (!assistant.content) assistant.content = t('playground.stopped')
    } else {
      assistant.error = true
      assistant.content = requestError.message || t('playground.requestFailed')
    }
  } finally {
    streaming.value = false
    controller = null
    convStore.touch(conversation.id)
    scrollToBottom()
  }
}

async function send(): Promise<void> {
  if (!canSend.value) return
  const text = input.value.trim()
  const conversation = ensureConversation()
  const outgoingAttachments = attachments.value.map((attachment) => ({ ...attachment }))
  const userMessage: ExtendedMessage = { id: uid(), role: 'user', content: text, attachments: outgoingAttachments }
  conversation.messages.push(userMessage)
  if (!conversation.title) conversation.title = (text || outgoingAttachments[0]?.name || t('playground.untitled')).slice(0, 24)
  conversation.apiKeyId = props.keyId ?? undefined
  conversation.model = props.option?.model
  conversation.option = props.option ?? undefined
  input.value = ''
  attachments.value = []
  nextTick(autoGrow)
  await runCompletion(conversation, text)
}

async function regenerate(): Promise<void> {
  const conversation = convStore.activeConversation()
  if (!conversation || streaming.value) return
  if (conversation.messages[conversation.messages.length - 1]?.role === 'assistant') conversation.messages.pop()
  const lastUser = [...conversation.messages].reverse().find((message) => message.role === 'user')
  await runCompletion(conversation, lastUser?.content ?? '')
}

function stop(): void { controller?.abort() }

async function exportConversation(format: 'markdown' | 'json'): Promise<void> {
  const conversation = convStore.activeConversation()
  if (!conversation) return
  const safeTitle = (conversation.title || 'conversation').replace(/[\\/:*?"<>|]+/g, '-').slice(0, 60)
  const options = { inlineAttachments: true, resolveAttachment: (attachment: PlaygroundAttachment) => readPlaygroundAttachment(attachment.id) }
  const content = format === 'json'
    ? await convStore.exportJSON(conversation.id, options)
    : await convStore.exportMarkdown(conversation.id, options)
  if (!content) return
  const mime = format === 'json' ? 'application/json;charset=utf-8' : 'text/markdown;charset=utf-8'
  saveAs(new Blob([content], { type: mime }), `${safeTitle}.${format === 'json' ? 'json' : 'md'}`)
  exportOpen.value = false
}

const ExportMenu = defineComponent({
  setup() {
    return () => h('div', { class: 'relative' }, [
      h('button', {
        type: 'button',
        class: 'flex min-h-11 items-center gap-1.5 rounded-lg px-2 text-xs text-gray-500 outline-none hover:bg-gray-100 focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-gray-400 dark:hover:bg-dark-700',
        'aria-haspopup': 'menu',
        'aria-expanded': exportOpen.value,
        onClick: () => { exportOpen.value = !exportOpen.value }
      }, [h(Icon, { name: 'download', size: 'sm' }), t('playground.export')]),
      exportOpen.value ? h('div', { class: 'absolute right-0 top-full z-20 mt-1 min-w-40 rounded-lg border border-gray-200 bg-white p-1 shadow-lg dark:border-dark-600 dark:bg-dark-800', role: 'menu' }, [
        h('button', { class: 'flex min-h-11 w-full items-center rounded-md px-3 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700', role: 'menuitem', onClick: () => exportConversation('markdown') }, t('playground.exportMarkdown')),
        h('button', { class: 'flex min-h-11 w-full items-center rounded-md px-3 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700', role: 'menuitem', onClick: () => exportConversation('json') }, t('playground.exportJson'))
      ]) : null
    ])
  }
})

const ConversationSidebar = defineComponent({
  setup() {
    return () => h('aside', { class: 'w-60 flex-shrink-0 flex-col' }, [
      h('div', { class: 'mb-3 flex gap-2' }, [
        h('button', { class: 'btn btn-primary min-h-11 min-w-0 flex-1', onClick: newConversation }, [h(Icon, { name: 'plus', size: 'sm' }), t('playground.newChat')]),
        h(ExportMenu)
      ]),
      h('div', { class: 'min-h-0 flex-1 space-y-1 overflow-y-auto pr-1' }, conversations.value.map((conversation) =>
        h('div', {
          key: conversation.id,
          class: ['group/conv flex min-h-11 cursor-pointer items-center gap-1 rounded-lg px-2 text-sm', conversation.id === activeId.value ? 'bg-primary-500/10 text-primary-600 dark:text-primary-400' : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-800'],
          onClick: () => selectConversation(conversation.id)
        }, [
          h(Icon, { name: 'chat', size: 'sm', class: 'flex-shrink-0 opacity-70' }),
          editingId.value === conversation.id
            ? h('input', {
                'data-rename-id': conversation.id,
                value: editingTitle.value,
                class: 'min-w-0 flex-1 rounded border border-primary-400 bg-white px-1 py-1 text-sm text-gray-800 outline-none dark:bg-dark-900 dark:text-gray-100',
                onInput: (event: Event) => { editingTitle.value = (event.target as HTMLInputElement).value },
                onBlur: commitRename,
                onKeydown: (event: KeyboardEvent) => { if (event.key === 'Enter') commitRename(); if (event.key === 'Escape') editingId.value = null }
              })
            : h('span', { class: 'min-w-0 flex-1 truncate', onDblclick: () => beginRename(conversation) }, conversation.title || t('playground.untitled')),
          h('button', { class: 'flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-md opacity-60 outline-none hover:text-gray-900 focus-visible:ring-2 focus-visible:ring-primary-500 group-hover/conv:opacity-100 dark:hover:text-white', title: t('playground.renameConversation'), onClick: (event: Event) => { event.stopPropagation(); beginRename(conversation) } }, [h(Icon, { name: 'edit', size: 'xs' })]),
          h('button', { class: 'flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-md opacity-60 outline-none hover:text-red-500 focus-visible:ring-2 focus-visible:ring-primary-500 group-hover/conv:opacity-100', title: t('common.delete'), onClick: (event: Event) => { event.stopPropagation(); removeConversation(conversation.id) } }, [h(Icon, { name: 'trash', size: 'xs' })])
        ])
      )),
      !conversations.value.length ? h('p', { class: 'px-3 py-4 text-center text-xs text-gray-400' }, t('playground.noConversations')) : null,
      conversations.value.length ? h('button', { class: 'mt-2 min-h-11 w-full rounded-lg px-3 text-xs text-gray-500 outline-none hover:bg-red-50 hover:text-red-600 focus-visible:ring-2 focus-visible:ring-primary-500 dark:hover:bg-red-950/20', onClick: clearConversations }, t('playground.clearConversations')) : null
    ])
  }
})

watch(activeId, scrollToBottom)
watch(supportsImageAttachments, (supported) => {
  if (supported) return
  const removedIds = attachments.value.filter((attachment) => attachment.kind === 'image').map((attachment) => attachment.id)
  if (!removedIds.length) return
  attachments.value = attachments.value.filter((attachment) => attachment.kind !== 'image')
  void deletePlaygroundAttachments(removedIds).catch(() => undefined)
})
onBeforeUnmount(() => { controller?.abort(); clearUndo() })
</script>

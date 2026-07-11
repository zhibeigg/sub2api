<template>
  <div class="contents">
  <div
    class="relative"
    @dragenter.prevent="dragging = true"
    @dragover.prevent="dragging = true"
    @dragleave="onDragLeave"
    @drop.prevent="onDrop"
  >
    <input
      ref="fileInput"
      type="file"
      class="sr-only"
      multiple
      :accept="effectiveAccept"
      :aria-label="t('playground.addAttachments')"
      @change="onFileInput"
    />

    <button
      type="button"
      class="flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-xl text-gray-500 outline-none transition-colors hover:bg-gray-100 hover:text-gray-800 focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-gray-100"
      :title="disabled ? t('playground.attachmentUnsupported') : t('playground.addAttachments')"
      :aria-label="disabled ? t('playground.attachmentUnsupported') : t('playground.addAttachments')"
      :disabled="disabled"
      @click="fileInput?.click()"
    >
      <Icon name="upload" size="sm" />
    </button>

    <div
      v-if="dragging"
      class="pointer-events-none fixed inset-3 z-50 flex items-center justify-center rounded-2xl border-2 border-dashed border-primary-500 bg-white/95 text-center shadow-xl dark:bg-dark-900/95"
      role="status"
      aria-live="polite"
    >
      <div>
        <Icon name="upload" size="xl" class="mx-auto text-primary-500" />
        <p class="mt-3 text-sm font-semibold text-gray-800 dark:text-gray-100">{{ t('playground.dropAttachments') }}</p>
      </div>
    </div>
  </div>

  <div v-if="modelValue.length" class="order-first flex min-w-0 basis-full flex-wrap gap-2 px-1 pb-1" aria-live="polite">
    <article
      v-for="attachment in modelValue"
      :key="attachment.id"
      class="group/attachment relative flex min-h-11 max-w-full items-center gap-2 rounded-lg border border-gray-200 bg-gray-50 px-2.5 py-1.5 dark:border-dark-600 dark:bg-dark-700/70"
      :class="attachment.status === 'error' || attachment.status === 'missing' ? 'border-red-300 dark:border-red-800' : ''"
    >
      <div class="flex h-8 w-8 flex-shrink-0 items-center justify-center overflow-hidden rounded-md bg-white text-gray-500 dark:bg-dark-800 dark:text-gray-300">
        <img
          v-if="isImage(attachment) && previewData[attachment.id]"
          :src="previewData[attachment.id]"
          :alt="attachment.name"
          class="h-full w-full object-cover"
        />
        <span v-else-if="attachment.status === 'reading'" class="attachment-spinner" aria-hidden="true"></span>
        <Icon v-else :name="attachment.status === 'error' || attachment.status === 'missing' ? 'exclamationTriangle' : 'document'" size="sm" />
      </div>
      <div class="min-w-0 flex-1">
        <p class="max-w-44 truncate text-xs font-medium text-gray-700 dark:text-gray-200">{{ attachment.name }}</p>
        <p class="truncate text-[11px]" :class="attachment.status === 'error' || attachment.status === 'missing' ? 'text-red-500' : 'text-gray-400'">
          {{ attachmentStatus(attachment) }}
        </p>
      </div>
      <button
        type="button"
        class="flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-lg text-gray-400 outline-none transition-colors hover:bg-gray-200 hover:text-red-500 focus-visible:ring-2 focus-visible:ring-primary-500 dark:hover:bg-dark-600"
        :aria-label="t('playground.removeAttachment', { name: attachment.name })"
        @click="remove(attachment.id)"
      >
        <Icon name="x" size="xs" />
      </button>
    </article>
  </div>

  <p v-if="lastError" class="order-first basis-full px-2 pb-1 text-xs text-red-600 dark:text-red-400" role="alert">
    {{ lastError }}
  </p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { PlaygroundAttachment } from '@/types/playground'
import {
  addPlaygroundAttachments,
  deletePlaygroundAttachment,
  readPlaygroundAttachment
} from '@/utils/playgroundAttachments'

const props = withDefaults(defineProps<{
  modelValue: PlaygroundAttachment[]
  accept?: string
  maxFiles?: number
  maxBytes?: number
  disabled?: boolean
  allowImages?: boolean
}>(), {
  accept: 'image/*,text/*,.json,.csv,.md,.txt,.yaml,.yml,.xml,.html,.css,.js,.ts,.vue,.py,.java,.go,.rs,.kt,.sql',
  maxFiles: 4,
  maxBytes: 5 * 1024 * 1024,
  disabled: false,
  allowImages: true
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: PlaygroundAttachment[]): void
}>()

const { t } = useI18n()
const fileInput = ref<HTMLInputElement | null>(null)
const dragging = ref(false)
const lastError = ref('')
const previewData = ref<Record<string, string>>({})
const effectiveAccept = computed(() => props.allowImages
  ? props.accept
  : props.accept.split(',').filter((value) => !value.trim().startsWith('image/')).join(','))

function temporaryId(): string {
  return `reading_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`
}

function mimeType(attachment: PlaygroundAttachment): string {
  return attachment.mimeType || attachment.type || ''
}

function isImage(attachment: PlaygroundAttachment): boolean {
  return attachment.kind === 'image' || mimeType(attachment).startsWith('image/')
}

function attachmentStatus(attachment: PlaygroundAttachment): string {
  if (attachment.status === 'reading') return t('playground.attachmentReading')
  if (attachment.status === 'missing') return t('playground.attachmentMissing')
  if (attachment.status === 'error') return attachment.error || t('playground.attachmentFailed')
  return formatBytes(attachment.size)
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

async function refreshPreviews(): Promise<void> {
  const previews: Record<string, string> = {}
  await Promise.all(props.modelValue.filter(isImage).map(async (attachment) => {
    const payload = await readPlaygroundAttachment(attachment.id).catch(() => null)
    if (payload?.encoding === 'data-url') previews[attachment.id] = payload.data
  }))
  previewData.value = previews
}

async function addFiles(fileList: FileList | File[]): Promise<void> {
  if (props.disabled) return
  lastError.value = ''
  const existing = [...props.modelValue]
  const available = Math.max(0, props.maxFiles - existing.length)
  let files = Array.from(fileList).slice(0, available)
  if (files.length < fileList.length) lastError.value = t('playground.attachmentLimit', { count: props.maxFiles })
  if (!props.allowImages) {
    const withoutImages = files.filter((file) => !file.type.startsWith('image/'))
    if (withoutImages.length !== files.length) lastError.value = t('playground.imageAttachmentUnsupported')
    files = withoutImages
  }
  if (!files.length) return

  const oversized = files.filter((file) => file.size > props.maxBytes)
  if (oversized.length) {
    const errors: PlaygroundAttachment[] = oversized.map((file) => ({
      id: temporaryId(),
      name: file.name,
      mimeType: file.type,
      type: file.type,
      size: file.size,
      status: 'error',
      error: t('playground.attachmentTooLarge', { size: formatBytes(props.maxBytes) })
    }))
    emit('update:modelValue', [...existing, ...errors])
    return
  }

  const placeholders: PlaygroundAttachment[] = files.map((file) => ({
    id: temporaryId(),
    name: file.name || t('playground.pastedImage'),
    mimeType: file.type,
    type: file.type,
    size: file.size,
    status: 'reading'
  }))
  emit('update:modelValue', [...existing, ...placeholders])
  try {
    const result = await addPlaygroundAttachments(files, existing.filter((item) => item.status !== 'error' && item.status !== 'reading'))
    emit('update:modelValue', result.allAttachments.map((item) => ({ ...item, status: 'ready' })))
    await refreshPreviews()
  } catch (error) {
    const message = error instanceof Error ? error.message : t('playground.attachmentFailed')
    lastError.value = message
    emit('update:modelValue', [
      ...existing,
      ...placeholders.map((item) => ({ ...item, status: 'error' as const, error: message }))
    ])
  }
}

function remove(id: string): void {
  emit('update:modelValue', props.modelValue.filter((attachment) => attachment.id !== id))
  delete previewData.value[id]
  if (!id.startsWith('reading_')) void deletePlaygroundAttachment(id).catch(() => undefined)
}

function onFileInput(event: Event): void {
  const input = event.target as HTMLInputElement
  if (input.files) addFiles(input.files)
  input.value = ''
}

function onDrop(event: DragEvent): void {
  dragging.value = false
  if (event.dataTransfer?.files.length) addFiles(event.dataTransfer.files)
}

function onDragLeave(event: DragEvent): void {
  const current = event.currentTarget as HTMLElement
  if (!current.contains(event.relatedTarget as Node | null)) dragging.value = false
}

function onPaste(event: ClipboardEvent): void {
  const images = Array.from(event.clipboardData?.items ?? [])
    .filter((item) => item.kind === 'file' && item.type.startsWith('image/'))
    .map((item) => item.getAsFile())
    .filter((file): file is File => !!file)
  if (!images.length) return
  event.preventDefault()
  void addFiles(images.map((file, index) => new File([file], file.name || `screenshot-${Date.now()}-${index + 1}.png`, { type: file.type })))
}

defineExpose({ addFiles, handlePaste: onPaste })
watch(() => props.modelValue.map((item) => item.id).join('|'), () => { void refreshPreviews() }, { immediate: true })
</script>

<style scoped>
.attachment-spinner {
  width: 1rem;
  height: 1rem;
  border: 2px solid currentColor;
  border-right-color: transparent;
  border-radius: 9999px;
  animation: attachment-spin 0.8s linear infinite;
}
@keyframes attachment-spin {
  to { transform: rotate(360deg); }
}
@media (prefers-reduced-motion: reduce) {
  .attachment-spinner { animation: none; border-right-color: currentColor; opacity: 0.55; }
}
</style>

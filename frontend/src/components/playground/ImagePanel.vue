<template>
  <div
    class="image-workbench grid h-full min-h-0 overflow-y-auto border border-gray-200 bg-gray-200 dark:border-dark-700 dark:bg-dark-700 lg:grid-cols-[320px_minmax(0,1fr)] xl:grid-cols-[320px_minmax(0,1fr)_180px] xl:overflow-hidden"
    data-testid="image-workbench"
    @paste="handlePaste"
  >
    <aside class="min-w-0 bg-white p-4 dark:bg-dark-900 xl:overflow-y-auto">
      <div class="mb-5 flex items-center justify-between border-b border-gray-200 pb-3 dark:border-dark-700">
        <div>
          <p class="font-mono text-[10px] uppercase tracking-[0.22em] text-primary-700 dark:text-primary-300">
            {{ t('playground.imageWorkbenchSignal') }}
          </p>
          <h2 class="mt-1 text-base font-semibold tracking-tight text-gray-950 dark:text-white">
            {{ t('playground.imageWorkbench') }}
          </h2>
        </div>
        <span class="font-mono text-[10px] text-gray-400">IMG / 01</span>
      </div>

      <div class="space-y-5">
        <KeyModelPicker
          :key-id="keyId"
          :resolved-key="resolvedKey"
          :option="option"
          capability="image"
          layout="stacked"
          @update:key-id="emit('update:keyId', $event)"
          @update:resolved-key="emit('update:resolvedKey', $event)"
          @update:option="emit('update:option', $event)"
        />

        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="image-label" for="image-size">{{ t('playground.size') }}</label>
            <select id="image-size" v-model="size" class="input min-h-11">
              <option value="auto">{{ t('playground.auto') }}</option>
              <option value="1024x1024">1024×1024</option>
              <option value="1536x1024">1536×1024</option>
              <option value="1024x1536">1024×1536</option>
              <option value="1792x1024">1792×1024</option>
              <option value="1024x1792">1024×1792</option>
              <option value="512x512">512×512</option>
              <option value="256x256">256×256</option>
            </select>
          </div>
          <div>
            <label class="image-label" for="image-count">{{ t('playground.count') }}</label>
            <select id="image-count" v-model.number="count" class="input min-h-11">
              <option :value="1">1</option>
              <option :value="2">2</option>
              <option :value="3">3</option>
              <option :value="4">4</option>
            </select>
          </div>
        </div>

        <div>
          <label class="image-label" for="image-quality">{{ t('playground.imageQuality') }}</label>
          <select id="image-quality" v-model="quality" class="input min-h-11" :disabled="!option">
            <option value="">{{ t('playground.imageQualityDefault') }}</option>
            <option v-for="item in qualityOptions" :key="item" :value="item">
              {{ qualityLabel(item) }}
            </option>
          </select>
          <p v-if="option && qualityOptions.length === 0" class="mt-1.5 text-[11px] leading-4 text-gray-400 dark:text-gray-500">
            {{ t('playground.imageQualityOmitted') }}
          </p>
        </div>

        <div>
          <label class="image-label" for="image-prompt">{{ t('playground.prompt') }}</label>
          <textarea
            id="image-prompt"
            v-model="prompt"
            rows="6"
            :placeholder="t('playground.promptPlaceholder')"
            class="input min-h-32 resize-y leading-6"
          ></textarea>
        </div>

        <section aria-labelledby="reference-title">
          <div class="mb-2 flex items-end justify-between gap-3">
            <div>
              <p id="reference-title" class="image-label mb-0">{{ t('playground.imageReferences') }}</p>
              <p class="mt-1 text-[11px] text-gray-400 dark:text-gray-500">
                {{ t('playground.imageReferenceRules') }}
              </p>
            </div>
            <span class="font-mono text-[10px] tabular-nums text-gray-400">{{ references.length }}/4</span>
          </div>

          <input
            ref="fileInput"
            class="sr-only"
            type="file"
            :accept="referenceAccept"
            :multiple="replaceTarget === null"
            data-testid="reference-input"
            @change="handleFileInput"
          />

          <div
            class="group flex min-h-24 cursor-pointer flex-col items-center justify-center border border-dashed px-4 py-3 text-center outline-none transition-colors focus-visible:ring-2 focus-visible:ring-primary-500"
            :class="isDragging
              ? 'border-primary-600 bg-primary-50 text-primary-800 dark:border-primary-400 dark:bg-primary-950/30 dark:text-primary-200'
              : 'border-gray-300 bg-gray-50/70 text-gray-500 hover:border-gray-500 dark:border-dark-600 dark:bg-dark-800/40 dark:text-gray-400 dark:hover:border-dark-400'"
            role="button"
            tabindex="0"
            data-testid="reference-dropzone"
            @click="openFilePicker()"
            @keydown.enter.prevent="openFilePicker()"
            @keydown.space.prevent="openFilePicker()"
            @dragenter.prevent="isDragging = true"
            @dragover.prevent="isDragging = true"
            @dragleave.prevent="isDragging = false"
            @drop.prevent="handleDrop"
          >
            <Icon name="upload" size="md" class="mb-2" />
            <span class="text-xs font-medium">{{ t('playground.imageReferenceAdd') }}</span>
            <span class="mt-1 text-[11px] opacity-75">{{ t('playground.imageReferenceInputHint') }}</span>
          </div>

          <div v-if="references.length" class="mt-2 grid grid-cols-4 gap-2" data-testid="reference-list">
            <div
              v-for="(reference, index) in references"
              :key="reference.id"
              class="group/reference relative aspect-square min-w-0 overflow-hidden border border-gray-200 bg-gray-100 dark:border-dark-600 dark:bg-dark-800"
            >
              <button
                type="button"
                class="h-full w-full outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500"
                :title="t('playground.imageReferenceReplace', { name: reference.name })"
                @click.stop="openFilePicker(index)"
              >
                <img
                  v-if="reference.previewUrl"
                  :src="reference.previewUrl"
                  :alt="reference.name"
                  class="h-full w-full object-cover"
                  @error="markReferenceLoadFailed(index)"
                />
                <span v-else class="flex h-full items-center justify-center text-red-500">
                  <Icon name="exclamationTriangle" size="sm" />
                </span>
              </button>
              <button
                type="button"
                class="absolute right-1 top-1 flex h-7 w-7 items-center justify-center bg-black/70 text-white opacity-100 outline-none transition-opacity focus-visible:ring-2 focus-visible:ring-white sm:opacity-0 sm:group-hover/reference:opacity-100"
                :aria-label="t('playground.removeAttachment', { name: reference.name })"
                @click.stop="removeReference(index)"
              >
                <Icon name="x" size="xs" />
              </button>
              <span class="pointer-events-none absolute inset-x-0 bottom-0 truncate bg-black/70 px-1.5 py-1 text-[9px] text-white">
                {{ reference.name }}
              </span>
            </div>
          </div>

          <p v-if="referenceError" class="mt-2 flex items-start gap-1.5 text-xs leading-5 text-red-600 dark:text-red-400" role="alert">
            <Icon name="exclamationTriangle" size="xs" class="mt-1 flex-shrink-0" />
            {{ referenceError }}
          </p>
        </section>

        <div class="border-t border-gray-200 pt-4 dark:border-dark-700">
          <button
            type="button"
            class="btn btn-primary min-h-12 w-full justify-center"
            :disabled="!!disabledReason"
            :title="disabledReason || t('playground.generate')"
            data-testid="image-generate"
            @click="generate()"
          >
            <Icon v-if="isGenerating" name="refresh" size="sm" class="animate-spin" />
            <Icon v-else name="sparkles" size="sm" />
            {{ isGenerating ? t('playground.generating') : t('playground.generate') }}
          </button>
          <p
            v-if="disabledReason"
            class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400"
            data-testid="image-disabled-reason"
          >
            {{ disabledReason }}
          </p>
          <p v-else class="mt-2 text-[11px] leading-4 text-gray-400 dark:text-gray-500">
            {{ t('playground.consumeHint') }}
          </p>
        </div>
      </div>
    </aside>

    <main class="relative flex min-h-[420px] min-w-0 flex-col bg-[#f4f3ef] dark:bg-[#111318] xl:min-h-0">
      <div class="flex min-h-12 items-center justify-between border-b border-gray-300 px-4 dark:border-dark-700">
        <div class="flex min-w-0 items-center gap-2">
          <span class="h-2 w-2 rounded-full" :class="canvasSignalClass"></span>
          <span class="truncate font-mono text-[10px] uppercase tracking-[0.16em] text-gray-600 dark:text-gray-300">
            {{ canvasStatusLabel }}
          </span>
        </div>
        <span v-if="currentBatch" class="font-mono text-[10px] tabular-nums text-gray-500 dark:text-gray-400">
          {{ formatElapsed(currentElapsedMs) }}
        </span>
      </div>

      <div class="image-canvas-grid relative min-h-0 flex-1 overflow-hidden p-4 sm:p-6">
        <div v-if="!currentBatch" class="flex h-full min-h-[340px] flex-col items-center justify-center text-center">
          <span class="mb-5 flex h-16 w-16 items-center justify-center border border-gray-400 text-gray-700 dark:border-dark-500 dark:text-gray-300">
            <Icon name="sparkles" size="lg" />
          </span>
          <p class="text-lg font-semibold tracking-tight text-gray-900 dark:text-white">{{ t('playground.imageCanvasEmptyTitle') }}</p>
          <p class="mt-2 max-w-sm text-sm leading-6 text-gray-500 dark:text-gray-400">{{ t('playground.imageCanvasEmptyHint') }}</p>
        </div>

        <div v-else-if="currentBatch.status === 'generating'" class="flex h-full min-h-[340px] flex-col items-center justify-center text-center" data-testid="image-generating-state">
          <span class="image-progress-mark mb-5" aria-hidden="true"></span>
          <p class="font-mono text-[11px] uppercase tracking-[0.18em] text-primary-800 dark:text-primary-300">
            {{ stageLabel(currentBatch.stage) }}
          </p>
          <p class="mt-3 text-3xl font-semibold tabular-nums tracking-tight text-gray-950 dark:text-white">
            {{ formatElapsed(currentElapsedMs) }}
          </p>
          <p class="mt-3 max-w-md truncate text-sm text-gray-500 dark:text-gray-400">{{ currentBatch.prompt }}</p>
          <p class="mt-1 font-mono text-[10px] text-gray-400">{{ currentBatch.model }} · {{ currentBatch.size }}</p>
        </div>

        <div v-else-if="currentBatch.status === 'error'" class="flex h-full min-h-[340px] flex-col items-center justify-center text-center" data-testid="image-error-state">
          <span class="mb-5 flex h-16 w-16 items-center justify-center border border-red-300 text-red-600 dark:border-red-900 dark:text-red-400">
            <Icon name="exclamationTriangle" size="lg" />
          </span>
          <p class="text-lg font-semibold text-gray-950 dark:text-white">{{ t('playground.imageCanvasErrorTitle') }}</p>
          <p class="mt-2 max-w-lg break-words text-sm leading-6 text-red-700 dark:text-red-300">{{ currentBatch.error }}</p>
          <button type="button" class="btn btn-secondary mt-5 min-h-11" :disabled="isGenerating || !resolvedKey" data-testid="image-retry" @click="retryBatch()">
            <Icon name="refresh" size="sm" />
            {{ t('playground.imageRetry') }}
          </button>
        </div>

        <div v-else-if="currentResult" class="flex h-full min-h-0 flex-col" data-testid="image-complete-state">
          <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
            <div class="min-w-0">
              <p class="truncate text-xs font-medium text-gray-900 dark:text-white">{{ currentBatch.model }}</p>
              <p class="mt-0.5 font-mono text-[10px] text-gray-500 dark:text-gray-400">
                {{ currentBatch.size }} · {{ currentResultIndex + 1 }}/{{ currentBatch.results.length }}
              </p>
            </div>
            <div class="flex items-center gap-1.5">
              <button type="button" class="image-action" :title="t('playground.download')" @click="downloadResult">
                <Icon name="download" size="sm" />
                <span class="hidden sm:inline">{{ t('playground.download') }}</span>
              </button>
              <button type="button" class="image-action" :title="t('playground.imageCopy')" @click="copyResult">
                <Icon name="copy" size="sm" />
                <span class="hidden sm:inline">{{ t('playground.imageCopy') }}</span>
              </button>
              <button type="button" class="image-action" :title="t('playground.imageFullscreen')" @click="previewUrl = currentResult.url">
                <Icon name="eye" size="sm" />
                <span class="hidden sm:inline">{{ t('playground.imageFullscreen') }}</span>
              </button>
            </div>
          </div>

          <button
            type="button"
            class="min-h-0 flex-1 cursor-zoom-in overflow-hidden border border-gray-300 bg-white/40 p-2 outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:border-dark-600 dark:bg-black/20"
            @click="previewUrl = currentResult.url"
          >
            <img :src="currentResult.url" :alt="currentBatch.prompt" class="h-full w-full object-contain" data-testid="canvas-image" />
          </button>

          <div v-if="currentBatch.results.length > 1" class="mt-3 flex gap-2 overflow-x-auto pb-1" data-testid="result-switcher">
            <button
              v-for="(result, index) in currentBatch.results"
              :key="result.id"
              type="button"
              class="h-16 w-16 flex-shrink-0 overflow-hidden border-2 bg-white outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:bg-dark-900"
              :class="index === currentResultIndex ? 'border-primary-700 dark:border-primary-400' : 'border-transparent opacity-60 hover:opacity-100'"
              :aria-label="t('playground.imageResultNumber', { number: index + 1 })"
              @click="selectResult(index)"
            >
              <img :src="result.url" alt="" class="h-full w-full object-cover" />
            </button>
          </div>

          <p v-if="currentResult.revisedPrompt" class="mt-3 line-clamp-2 text-xs leading-5 text-gray-500 dark:text-gray-400">
            {{ currentResult.revisedPrompt }}
          </p>
          <p v-if="actionFeedback" class="mt-2 text-xs text-primary-700 dark:text-primary-300" role="status">{{ actionFeedback }}</p>
        </div>
      </div>
    </main>

    <aside class="min-w-0 bg-white p-3 dark:bg-dark-900 lg:col-span-2 xl:col-span-1 xl:overflow-y-auto">
      <div class="mb-3 flex items-center justify-between border-b border-gray-200 pb-2 dark:border-dark-700">
        <div>
          <p class="font-mono text-[10px] uppercase tracking-[0.16em] text-gray-500 dark:text-gray-400">{{ t('playground.imageHistory') }}</p>
          <p class="mt-0.5 text-[11px] text-gray-400">{{ t('playground.imageHistorySession') }}</p>
        </div>
        <button
          v-if="history.length"
          type="button"
          class="min-h-9 px-2 text-[11px] font-medium text-gray-500 outline-none hover:text-red-600 focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-gray-400 dark:hover:text-red-400"
          @click="clearAllHistory"
        >
          {{ t('playground.imageHistoryClear') }}
        </button>
      </div>

      <div v-if="history.length === 0" class="flex min-h-28 items-center justify-center border border-dashed border-gray-200 px-3 text-center text-xs leading-5 text-gray-400 dark:border-dark-700 dark:text-gray-500">
        {{ t('playground.imageHistoryEmpty') }}
      </div>

      <div v-else class="grid grid-cols-[repeat(auto-fill,minmax(150px,1fr))] gap-2 xl:grid-cols-1" data-testid="image-history">
        <div
          v-for="batch in history"
          :key="batch.id"
          class="group/history relative border bg-white transition-colors dark:bg-dark-900"
          :class="batch.id === currentBatchId
            ? 'border-primary-700 dark:border-primary-400'
            : 'border-gray-200 hover:border-gray-400 dark:border-dark-700 dark:hover:border-dark-500'"
        >
          <button type="button" class="w-full p-1.5 text-left outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500" @click="selectBatch(batch.id)">
            <div class="relative aspect-square overflow-hidden bg-gray-100 dark:bg-dark-800">
              <img v-if="batch.results[0]" :src="batch.results[0].url" alt="" class="h-full w-full object-cover" />
              <span v-else class="flex h-full items-center justify-center" :class="batch.status === 'error' ? 'text-red-500' : 'text-gray-400'">
                <Icon :name="batch.status === 'error' ? 'exclamationTriangle' : 'refresh'" size="md" :class="batch.status === 'generating' ? 'animate-spin' : ''" />
              </span>
              <span class="absolute bottom-1 left-1 bg-black/75 px-1.5 py-0.5 font-mono text-[9px] uppercase text-white">
                {{ historyStatusLabel(batch.status) }}
              </span>
            </div>
            <p class="mt-1.5 truncate text-[11px] font-medium text-gray-800 dark:text-gray-200">{{ batch.prompt }}</p>
            <p class="mt-0.5 truncate font-mono text-[9px] text-gray-400">{{ batch.model }}</p>
            <p class="mt-1 flex items-center justify-between font-mono text-[9px] text-gray-400">
              <span>{{ batch.size }}</span>
              <span>{{ formatHistoryTime(batch.createdAt) }}</span>
            </p>
          </button>
          <button
            type="button"
            class="absolute right-2 top-2 flex h-8 w-8 items-center justify-center bg-black/70 text-white opacity-100 outline-none focus-visible:ring-2 focus-visible:ring-white sm:opacity-0 sm:group-hover/history:opacity-100"
            :aria-label="t('playground.imageHistoryDelete')"
            @click="deleteHistoryBatch(batch.id)"
          >
            <Icon name="trash" size="xs" />
          </button>
        </div>
      </div>
    </aside>

    <Teleport to="body">
      <div
        v-if="previewUrl"
        class="fixed inset-0 z-[9999] flex items-center justify-center bg-black/90 p-3 sm:p-8"
        role="dialog"
        aria-modal="true"
        :aria-label="t('playground.imageFullscreen')"
        @click.self="previewUrl = ''"
      >
        <button
          type="button"
          class="absolute right-4 top-4 flex h-12 w-12 items-center justify-center border border-white/30 bg-black/60 text-white outline-none hover:border-white focus-visible:ring-2 focus-visible:ring-white"
          :aria-label="t('common.close')"
          @click="previewUrl = ''"
        >
          <Icon name="x" size="md" />
        </button>
        <img :src="previewUrl" class="max-h-full max-w-full object-contain" :alt="currentBatch?.prompt || ''" />
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import KeyModelPicker from '@/components/playground/KeyModelPicker.vue'
import { IMAGE_REFERENCE_TYPES, useImageWorkbench } from '@/composables/useImageWorkbench'
import type {
  PlaygroundImageBatchStatus,
  PlaygroundImageQuality,
  PlaygroundImageStage,
  PlaygroundModelOption
} from '@/types/playground'

const props = defineProps<{
  keyId: number | null
  resolvedKey: string
  option: PlaygroundModelOption | null
}>()

const emit = defineEmits<{
  (event: 'update:keyId', value: number | null): void
  (event: 'update:resolvedKey', value: string): void
  (event: 'update:option', value: PlaygroundModelOption | null): void
}>()

const { t } = useI18n()
const fileInput = ref<HTMLInputElement | null>(null)
const replaceTarget = ref<number | null>(null)
const previewUrl = ref('')
const actionFeedback = ref('')
let feedbackTimer: ReturnType<typeof setTimeout> | null = null

const {
  prompt,
  size,
  quality,
  count,
  references,
  referenceError,
  isDragging,
  history,
  currentBatchId,
  currentResultIndex,
  qualityOptions,
  currentBatch,
  currentResult,
  currentElapsedMs,
  isGenerating,
  hasInvalidReferences,
  addReferenceFiles,
  replaceReference,
  removeReference,
  markReferenceLoadFailed,
  selectBatch,
  selectResult,
  deleteBatch,
  clearHistory,
  generate,
  retryBatch
} = useImageWorkbench({
  apiKey: () => props.resolvedKey,
  option: () => props.option,
  t
})

const referenceAccept = IMAGE_REFERENCE_TYPES.join(',')
const disabledReason = computed(() => {
  if (isGenerating.value) return t('playground.imageDisabledGenerating')
  if (props.keyId == null) return t('playground.imageDisabledNoKey')
  if (!props.resolvedKey) return t('playground.imageDisabledKeyUnavailable')
  if (!props.option) return t('playground.imageDisabledNoModel')
  if (!prompt.value.trim()) return t('playground.imageDisabledNoPrompt')
  if (hasInvalidReferences.value) return t('playground.imageDisabledInvalidReference')
  return ''
})
const canvasStatusLabel = computed(() => {
  if (!currentBatch.value) return t('playground.imageStatusIdle')
  if (currentBatch.value.status === 'generating') return stageLabel(currentBatch.value.stage)
  if (currentBatch.value.status === 'error') return t('playground.imageStatusError')
  return t('playground.imageStatusComplete')
})
const canvasSignalClass = computed(() => {
  if (!currentBatch.value) return 'bg-gray-400'
  if (currentBatch.value.status === 'generating') return 'animate-pulse bg-primary-700 dark:bg-primary-400'
  if (currentBatch.value.status === 'error') return 'bg-red-600'
  return 'bg-emerald-600'
})

function qualityLabel(value: PlaygroundImageQuality): string {
  return t(`playground.imageQuality_${value}`)
}

function openFilePicker(index: number | null = null): void {
  replaceTarget.value = index
  fileInput.value?.click()
}

function handleFileInput(event: Event): void {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files ?? [])
  if (replaceTarget.value != null && files[0]) replaceReference(replaceTarget.value, files[0])
  else addReferenceFiles(files)
  replaceTarget.value = null
  input.value = ''
}

function handleDrop(event: DragEvent): void {
  isDragging.value = false
  addReferenceFiles(Array.from(event.dataTransfer?.files ?? []))
}

function handlePaste(event: ClipboardEvent): void {
  const files = Array.from(event.clipboardData?.items ?? [])
    .filter((item) => item.kind === 'file')
    .map((item) => item.getAsFile())
    .filter((file): file is File => !!file)
  if (files.length === 0) return
  event.preventDefault()
  addReferenceFiles(files)
}

function stageLabel(stage?: PlaygroundImageStage): string {
  if (stage === 'preparing') return t('playground.imageStagePreparing')
  if (stage === 'decoding') return t('playground.imageStageDecoding')
  return t('playground.imageStageRequesting')
}

function historyStatusLabel(status: PlaygroundImageBatchStatus): string {
  if (status === 'generating') return t('playground.imageHistoryGenerating')
  if (status === 'error') return t('playground.imageHistoryError')
  return t('playground.imageHistoryComplete')
}

function formatElapsed(milliseconds: number): string {
  return `${(milliseconds / 1000).toFixed(1)}s`
}

function formatHistoryTime(timestamp: number): string {
  return new Date(timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function setActionFeedback(message: string): void {
  actionFeedback.value = message
  if (feedbackTimer) clearTimeout(feedbackTimer)
  feedbackTimer = setTimeout(() => {
    actionFeedback.value = ''
  }, 2200)
}

function resultExtension(mimeType: string): string {
  if (mimeType.includes('webp')) return 'webp'
  if (mimeType.includes('jpeg')) return 'jpg'
  return 'png'
}

function downloadResult(): void {
  const result = currentResult.value
  if (!result) return
  const link = document.createElement('a')
  link.href = result.url
  link.download = `playground-${currentBatch.value?.id ?? 'image'}-${currentResultIndex.value + 1}.${resultExtension(result.mimeType)}`
  document.body.appendChild(link)
  link.click()
  link.remove()
}

async function copyResult(): Promise<void> {
  const result = currentResult.value
  if (!result) return
  try {
    const response = await fetch(result.url)
    const blob = await response.blob()
    if (!navigator.clipboard?.write || typeof ClipboardItem === 'undefined') throw new Error('clipboard unavailable')
    await navigator.clipboard.write([new ClipboardItem({ [blob.type || result.mimeType]: blob })])
    setActionFeedback(t('playground.imageCopied'))
  } catch {
    setActionFeedback(t('playground.imageCopyFailed'))
  }
}

function deleteHistoryBatch(batchId: string): void {
  const closingPreview = currentBatch.value?.id === batchId
  deleteBatch(batchId)
  if (closingPreview) previewUrl.value = ''
}

function clearAllHistory(): void {
  previewUrl.value = ''
  clearHistory()
}

function handleEscape(event: KeyboardEvent): void {
  if (event.key === 'Escape') previewUrl.value = ''
}

onMounted(() => window.addEventListener('keydown', handleEscape))
onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleEscape)
  if (feedbackTimer) clearTimeout(feedbackTimer)
})
</script>

<style scoped>
.image-label {
  @apply mb-1.5 block text-[11px] font-semibold uppercase tracking-[0.08em] text-gray-500 dark:text-gray-400;
}

.image-action {
  @apply flex min-h-10 items-center gap-1.5 border border-gray-300 bg-white px-2.5 text-xs font-medium text-gray-700 outline-none transition-colors hover:border-gray-600 hover:text-gray-950 focus-visible:ring-2 focus-visible:ring-primary-500 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-300 dark:hover:border-dark-400 dark:hover:text-white;
}

.image-canvas-grid {
  background-image:
    linear-gradient(to right, rgb(15 23 42 / 0.055) 1px, transparent 1px),
    linear-gradient(to bottom, rgb(15 23 42 / 0.055) 1px, transparent 1px);
  background-size: 24px 24px;
}

:global(.dark) .image-canvas-grid {
  background-image:
    linear-gradient(to right, rgb(255 255 255 / 0.04) 1px, transparent 1px),
    linear-gradient(to bottom, rgb(255 255 255 / 0.04) 1px, transparent 1px);
}

.image-progress-mark {
  width: 3.5rem;
  height: 3.5rem;
  border: 1px solid rgb(156 163 175 / 0.7);
  border-top-color: rgb(29 78 216);
  animation: image-spin 0.9s linear infinite;
}

@keyframes image-spin {
  to { transform: rotate(360deg); }
}

@media (prefers-reduced-motion: reduce) {
  .image-progress-mark {
    animation-duration: 2.4s;
  }
}
</style>

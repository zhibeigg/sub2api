import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import playgroundAPI, { imageQualityOptions, type GeneratedImage } from '@/api/playground'
import type {
  PlaygroundImageBatch,
  PlaygroundImageQuality,
  PlaygroundImageReference,
  PlaygroundImageResult,
  PlaygroundModelOption
} from '@/types/playground'

export const IMAGE_REFERENCE_LIMIT = 4
export const IMAGE_REFERENCE_MAX_BYTES = 20 * 1024 * 1024
export const IMAGE_REFERENCE_TYPES = ['image/png', 'image/jpeg', 'image/webp'] as const
export const IMAGE_HISTORY_LIMIT = 12

interface UseImageWorkbenchOptions {
  apiKey: () => string
  option: () => PlaygroundModelOption | null
  t: (key: string, params?: Record<string, unknown>) => string
}

let imageIdSeed = 0

function createId(prefix: string): string {
  imageIdSeed += 1
  return `${prefix}-${Date.now().toString(36)}-${imageIdSeed.toString(36)}`
}

function isAbortError(error: unknown): boolean {
  const name = (error as Error | undefined)?.name
  return name === 'AbortError' || name === 'CanceledError'
}

function revokeUrl(url: string): void {
  if (url) URL.revokeObjectURL(url)
}

function releaseResult(result: PlaygroundImageResult): void {
  if (result.revokeOnRelease) revokeUrl(result.url)
}

function releaseBatch(batch: PlaygroundImageBatch): void {
  batch.results.forEach(releaseResult)
  batch.results = []
}

function base64ToBlob(value: string, mimeType = 'image/png'): Blob {
  const binary = atob(value)
  const bytes = new Uint8Array(binary.length)
  for (let index = 0; index < binary.length; index += 1) bytes[index] = binary.charCodeAt(index)
  return new Blob([bytes], { type: mimeType })
}

function resultFromResponse(image: GeneratedImage): PlaygroundImageResult | null {
  if (image.b64_json) {
    const blob = base64ToBlob(image.b64_json)
    return {
      id: createId('result'),
      url: URL.createObjectURL(blob),
      mimeType: blob.type,
      revisedPrompt: image.revised_prompt,
      revokeOnRelease: true
    }
  }
  if (image.url) {
    return {
      id: createId('result'),
      url: image.url,
      mimeType: 'image/png',
      revisedPrompt: image.revised_prompt,
      revokeOnRelease: false
    }
  }
  return null
}

function resultsFromResponse(images: GeneratedImage[]): PlaygroundImageResult[] {
  const results: PlaygroundImageResult[] = []
  try {
    for (const image of images) {
      const result = resultFromResponse(image)
      if (result) results.push(result)
    }
    return results
  } catch (error) {
    results.forEach(releaseResult)
    throw error
  }
}

export function useImageWorkbench({ apiKey, option, t }: UseImageWorkbenchOptions) {
  const prompt = ref('')
  const size = ref('1024x1024')
  const quality = ref<PlaygroundImageQuality>('')
  const count = ref(1)
  const references = ref<PlaygroundImageReference[]>([])
  const referenceError = ref('')
  const isDragging = ref(false)
  const history = ref<PlaygroundImageBatch[]>([])
  const currentBatchId = ref('')
  const currentResultIndex = ref(0)
  const now = ref(Date.now())

  let activeController: AbortController | null = null
  let activeBatchId = ''
  let elapsedTimer: ReturnType<typeof setInterval> | null = null

  const qualityOptions = computed(() => imageQualityOptions(option()?.model ?? ''))
  const currentBatch = computed(() => history.value.find((batch) => batch.id === currentBatchId.value) ?? null)
  const currentResult = computed(() => currentBatch.value?.results[currentResultIndex.value] ?? null)
  const isGenerating = computed(() => history.value.some((batch) => batch.status === 'generating'))
  const hasInvalidReferences = computed(() => references.value.some((reference) => reference.status === 'error'))
  const currentElapsedMs = computed(() => {
    const batch = currentBatch.value
    if (!batch) return 0
    if (batch.status === 'generating') return Math.max(0, now.value - batch.createdAt)
    return batch.elapsedMs ?? 0
  })

  watch(qualityOptions, (options) => {
    if (quality.value && !options.includes(quality.value)) quality.value = ''
  })

  function startElapsedTimer(): void {
    stopElapsedTimer()
    now.value = Date.now()
    elapsedTimer = setInterval(() => {
      now.value = Date.now()
    }, 250)
  }

  function stopElapsedTimer(): void {
    if (elapsedTimer) {
      clearInterval(elapsedTimer)
      elapsedTimer = null
    }
  }

  function validationError(file: File): string {
    if (!IMAGE_REFERENCE_TYPES.includes(file.type as (typeof IMAGE_REFERENCE_TYPES)[number])) {
      return t('playground.imageReferenceType')
    }
    if (file.size > IMAGE_REFERENCE_MAX_BYTES) {
      return t('playground.imageReferenceTooLarge', { size: '20 MiB' })
    }
    return ''
  }

  function createReference(file: File): PlaygroundImageReference {
    return {
      id: createId('reference'),
      name: file.name,
      size: file.size,
      mimeType: file.type,
      file,
      previewUrl: URL.createObjectURL(file),
      status: 'ready'
    }
  }

  function addReferenceFiles(files: Iterable<File>): void {
    referenceError.value = ''
    for (const file of files) {
      if (references.value.length >= IMAGE_REFERENCE_LIMIT) {
        referenceError.value = t('playground.imageReferenceLimit', { count: IMAGE_REFERENCE_LIMIT })
        break
      }
      const error = validationError(file)
      if (error) {
        referenceError.value = error
        continue
      }
      references.value.push(createReference(file))
    }
  }

  function replaceReference(index: number, file: File): void {
    referenceError.value = ''
    const current = references.value[index]
    if (!current) return
    const error = validationError(file)
    if (error) {
      referenceError.value = error
      return
    }
    const replacement = createReference(file)
    revokeUrl(current.previewUrl)
    references.value.splice(index, 1, replacement)
  }

  function removeReference(index: number): void {
    const [removed] = references.value.splice(index, 1)
    if (removed) revokeUrl(removed.previewUrl)
    referenceError.value = ''
  }

  function markReferenceLoadFailed(index: number): void {
    const reference = references.value[index]
    if (!reference || reference.status === 'error') return
    revokeUrl(reference.previewUrl)
    reference.previewUrl = ''
    reference.status = 'error'
    reference.error = t('playground.imageReferenceLoadFailed')
    referenceError.value = reference.error
  }

  function releaseReferences(): void {
    references.value.forEach((reference) => revokeUrl(reference.previewUrl))
    references.value = []
  }

  function selectBatch(batchId: string): void {
    if (!history.value.some((batch) => batch.id === batchId)) return
    currentBatchId.value = batchId
    currentResultIndex.value = 0
  }

  function selectResult(index: number): void {
    const resultCount = currentBatch.value?.results.length ?? 0
    if (index >= 0 && index < resultCount) currentResultIndex.value = index
  }

  function trimHistory(): void {
    if (history.value.length <= IMAGE_HISTORY_LIMIT) return
    const removed = history.value.splice(IMAGE_HISTORY_LIMIT)
    removed.forEach(releaseBatch)
  }

  function deleteBatch(batchId: string): void {
    const index = history.value.findIndex((batch) => batch.id === batchId)
    if (index < 0) return
    if (activeBatchId === batchId) {
      activeController?.abort()
      activeController = null
      activeBatchId = ''
      stopElapsedTimer()
    }
    const [removed] = history.value.splice(index, 1)
    releaseBatch(removed)
    if (currentBatchId.value === batchId) {
      currentBatchId.value = history.value[0]?.id ?? ''
      currentResultIndex.value = 0
    }
  }

  function clearHistory(): void {
    activeController?.abort()
    activeController = null
    activeBatchId = ''
    stopElapsedTimer()
    history.value.forEach(releaseBatch)
    history.value = []
    currentBatchId.value = ''
    currentResultIndex.value = 0
  }

  async function generate(snapshot?: {
    prompt: string
    option: PlaygroundModelOption
    size: string
    quality: PlaygroundImageQuality
    count: number
  }): Promise<void> {
    if (activeController) return
    const selectedOption = snapshot?.option ?? option()
    const selectedPrompt = (snapshot?.prompt ?? prompt.value).trim()
    const selectedKey = apiKey()
    if (!selectedKey || !selectedOption || !selectedPrompt || hasInvalidReferences.value) return

    const selectedSize = snapshot?.size ?? size.value
    const selectedQuality = snapshot?.quality ?? quality.value
    const selectedCount = snapshot?.count ?? count.value
    const batch = reactive<PlaygroundImageBatch>({
      id: createId('batch'),
      status: 'generating',
      stage: 'preparing',
      prompt: selectedPrompt,
      option: { ...selectedOption },
      model: selectedOption.model,
      size: selectedSize,
      quality: selectedQuality,
      count: selectedCount,
      referenceCount: references.value.length,
      createdAt: Date.now(),
      results: []
    })

    history.value.unshift(batch)
    trimHistory()
    currentBatchId.value = batch.id
    currentResultIndex.value = 0
    referenceError.value = ''
    const controller = new AbortController()
    activeController = controller
    activeBatchId = batch.id
    startElapsedTimer()

    try {
      batch.stage = 'requesting'
      const response = await playgroundAPI.generateImage({
        apiKey: selectedKey,
        groupId: selectedOption.group_id,
        model: selectedOption.model,
        prompt: selectedPrompt,
        size: selectedSize === 'auto' ? undefined : selectedSize,
        quality: selectedQuality,
        n: selectedCount,
        images: references.value.filter((reference) => reference.status === 'ready').map((reference) => reference.file),
        signal: controller.signal
      })
      if (!history.value.includes(batch)) return

      batch.stage = 'decoding'
      const results = resultsFromResponse(response)
      if (results.length === 0) throw new Error(t('playground.imageNoResult'))
      batch.results = results
      batch.status = 'completed'
      batch.completedAt = Date.now()
      batch.elapsedMs = batch.completedAt - batch.createdAt
      batch.stage = undefined
    } catch (error) {
      if (isAbortError(error) || !history.value.includes(batch)) return
      batch.status = 'error'
      batch.error = (error as Error).message || t('playground.requestFailed')
      batch.completedAt = Date.now()
      batch.elapsedMs = batch.completedAt - batch.createdAt
      batch.stage = undefined
    } finally {
      if (activeController === controller) {
        activeController = null
        activeBatchId = ''
        stopElapsedTimer()
      }
      now.value = Date.now()
    }
  }

  async function retryBatch(batch = currentBatch.value): Promise<void> {
    if (!batch || batch.status !== 'error') return
    prompt.value = batch.prompt
    size.value = batch.size
    quality.value = batch.quality
    count.value = batch.count
    await generate({
      prompt: batch.prompt,
      option: batch.option,
      size: batch.size,
      quality: batch.quality,
      count: batch.count
    })
  }

  onBeforeUnmount(() => {
    activeController?.abort()
    activeController = null
    activeBatchId = ''
    stopElapsedTimer()
    releaseReferences()
    history.value.forEach(releaseBatch)
  })

  return {
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
  }
}

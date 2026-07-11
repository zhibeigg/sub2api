<template>
  <div class="flex h-full min-h-0 flex-col gap-4 lg:flex-row">
    <!-- Controls -->
    <div class="w-full flex-shrink-0 space-y-4 lg:w-72">
      <div>
        <label class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
          {{ t('playground.prompt') }}
        </label>
        <textarea
          v-model="prompt"
          rows="5"
          :placeholder="t('playground.videoPromptPlaceholder')"
          class="input resize-none"
        ></textarea>
      </div>

      <div class="grid grid-cols-3 gap-3">
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('playground.resolution') }}
          </label>
          <select v-model="resolution" class="input">
            <option value="480p">480P</option>
            <option value="720p">720P</option>
            <option value="1080p">1080P</option>
          </select>
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('playground.duration') }}
          </label>
          <select v-model.number="seconds" class="input">
            <option :value="5">5s</option>
            <option :value="10">10s</option>
          </select>
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('playground.ratio') }}
          </label>
          <select v-model="ratio" class="input">
            <option value="16:9">16:9</option>
            <option value="9:16">9:16</option>
            <option value="1:1">1:1</option>
            <option value="adaptive">{{ t('playground.auto') }}</option>
          </select>
        </div>
      </div>

      <button
        class="btn btn-primary w-full"
        :disabled="loading || !prompt.trim() || !resolvedKey || !option"
        @click="generate"
      >
        <Icon v-if="loading" name="refresh" size="sm" class="animate-spin" />
        <Icon v-else name="sparkles" size="sm" />
        {{ loading ? t('playground.videoGenerating') : t('playground.generate') }}
      </button>
      <p class="text-[11px] text-gray-400">{{ t('playground.videoHint') }}</p>
      <p v-if="statusText" class="text-xs text-gray-500 dark:text-gray-400">{{ statusText }}</p>
      <p v-if="error" class="flex items-start gap-1.5 text-xs text-red-500">
        <Icon name="exclamationTriangle" size="xs" class="mt-0.5 flex-shrink-0" />
        {{ error }}
      </p>
    </div>

    <!-- Result -->
    <div class="min-h-0 flex-1 overflow-y-auto rounded-xl border border-gray-100 bg-gray-50/50 p-4 dark:border-dark-700 dark:bg-dark-800/30">
      <div v-if="loading" class="flex h-full flex-col items-center justify-center text-center">
        <Icon name="refresh" size="xl" class="mb-3 animate-spin text-gray-300 dark:text-gray-600" />
        <p class="text-sm text-gray-400">{{ statusText || t('playground.videoGenerating') }}</p>
      </div>

      <div v-else-if="!videoUrl" class="flex h-full flex-col items-center justify-center text-center">
        <Icon name="sparkles" size="xl" class="mb-3 text-gray-300 dark:text-gray-600" />
        <p class="text-sm text-gray-400">{{ t('playground.videoEmpty') }}</p>
      </div>

      <div v-else class="flex flex-col items-center gap-3">
        <video :src="videoUrl" controls autoplay loop class="max-h-[60vh] w-full rounded-xl border border-gray-200 bg-black dark:border-dark-600"></video>
        <a
          :href="videoUrl"
          :download="`playground-video.mp4`"
          class="btn btn-secondary"
          target="_blank"
          rel="noopener"
        >
          <Icon name="download" size="sm" />
          {{ t('playground.download') }}
        </a>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import playgroundAPI from '@/api/playground'
import type { PlaygroundModelOption } from '@/types/playground'

const props = defineProps<{
  resolvedKey: string
  option: PlaygroundModelOption | null
}>()

const { t } = useI18n()

const prompt = ref('')
const resolution = ref('720p')
const seconds = ref(5)
const ratio = ref('16:9')
const loading = ref(false)
const error = ref('')
const statusText = ref('')
const videoUrl = ref('')

let pollTimer: ReturnType<typeof setTimeout> | null = null
let cancelled = false

const POLL_INTERVAL_MS = 5000
const MAX_POLL_MS = 6 * 60 * 1000 // 6 分钟上限

function stopPolling(): void {
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
}

async function generate(): Promise<void> {
  if (!prompt.value.trim() || !props.resolvedKey || !props.option) return
  stopPolling()
  cancelled = false
  loading.value = true
  error.value = ''
  videoUrl.value = ''
  statusText.value = t('playground.videoSubmitting')

  try {
    const option = props.option
    const apiKey = props.resolvedKey
    const submit = await playgroundAPI.generateVideo({
      apiKey,
      groupId: option.group_id,
      model: option.model,
      prompt: prompt.value.trim(),
      seconds: seconds.value,
      resolution: resolution.value,
      ratio: ratio.value
    })
    statusText.value = t('playground.videoGenerating')
    pollStatus(apiKey, option.group_id, submit.request_id, Date.now())
  } catch (err) {
    loading.value = false
    error.value = (err as Error).message || t('playground.requestFailed')
  }
}

function pollStatus(apiKey: string, groupId: number, requestId: string, startedAt: number): void {
  if (cancelled) return
  pollTimer = setTimeout(async () => {
    if (cancelled) return
    if (Date.now() - startedAt > MAX_POLL_MS) {
      loading.value = false
      error.value = t('playground.videoTimeout')
      return
    }
    try {
      const task = await playgroundAPI.getVideoStatus(apiKey, groupId, requestId)
      if (cancelled) return
      if (task.status === 'completed') {
        const url = task.url || task.video_url || ''
        if (url) {
          videoUrl.value = url
          loading.value = false
          statusText.value = ''
        } else {
          loading.value = false
          error.value = t('playground.videoNoResult')
        }
        return
      }
      if (task.status === 'failed') {
        loading.value = false
        error.value = task.error || t('playground.videoFailed')
        return
      }
      statusText.value = t('playground.videoGenerating')
      pollStatus(apiKey, groupId, requestId, startedAt)
    } catch (err) {
      loading.value = false
      error.value = (err as Error).message || t('playground.requestFailed')
    }
  }, POLL_INTERVAL_MS)
}

onBeforeUnmount(() => {
  cancelled = true
  stopPolling()
})
</script>

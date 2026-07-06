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
          :placeholder="t('playground.promptPlaceholder')"
          class="input resize-none"
        ></textarea>
      </div>

      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('playground.size') }}
          </label>
          <select v-model="size" class="input">
            <option value="1024x1024">1024×1024</option>
            <option value="1536x1024">1536×1024</option>
            <option value="1024x1536">1024×1536</option>
            <option value="auto">{{ t('playground.auto') }}</option>
          </select>
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('playground.count') }}
          </label>
          <select v-model.number="count" class="input">
            <option :value="1">1</option>
            <option :value="2">2</option>
            <option :value="4">4</option>
          </select>
        </div>
      </div>

      <button
        class="btn btn-primary w-full"
        :disabled="loading || !prompt.trim() || !resolvedKey || !model"
        @click="generate"
      >
        <Icon v-if="loading" name="refresh" size="sm" class="animate-spin" />
        <Icon v-else name="sparkles" size="sm" />
        {{ loading ? t('playground.generating') : t('playground.generate') }}
      </button>
      <p class="text-[11px] text-gray-400">{{ t('playground.consumeHint') }}</p>
      <p v-if="error" class="flex items-start gap-1.5 text-xs text-red-500">
        <Icon name="exclamationTriangle" size="xs" class="mt-0.5 flex-shrink-0" />
        {{ error }}
      </p>
    </div>

    <!-- Gallery -->
    <div class="min-h-0 flex-1 overflow-y-auto rounded-xl border border-gray-100 bg-gray-50/50 p-4 dark:border-dark-700 dark:bg-dark-800/30">
      <div v-if="loading" class="grid grid-cols-2 gap-4">
        <div
          v-for="n in count"
          :key="`sk-${n}`"
          class="aspect-square animate-pulse rounded-xl bg-gray-200 dark:bg-dark-700"
        ></div>
      </div>

      <div v-else-if="images.length === 0" class="flex h-full flex-col items-center justify-center text-center">
        <Icon name="sparkles" size="xl" class="mb-3 text-gray-300 dark:text-gray-600" />
        <p class="text-sm text-gray-400">{{ t('playground.imageEmpty') }}</p>
      </div>

      <div v-else class="grid grid-cols-2 gap-4">
        <div
          v-for="(img, i) in images"
          :key="i"
          class="group/img relative overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-600"
        >
          <img :src="img" class="aspect-square w-full cursor-zoom-in object-cover" @click="preview = img" />
          <a
            :href="img"
            :download="`playground-${i + 1}.png`"
            class="absolute right-2 top-2 flex h-8 w-8 items-center justify-center rounded-lg bg-black/50 text-white opacity-0 transition-opacity group-hover/img:opacity-100"
            :title="t('playground.download')"
            @click.stop
          >
            <Icon name="download" size="sm" />
          </a>
        </div>
      </div>
    </div>

    <!-- Lightbox -->
    <Teleport to="body">
      <div
        v-if="preview"
        class="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-6"
        @click="preview = ''"
      >
        <img :src="preview" class="max-h-full max-w-full rounded-lg object-contain" />
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import playgroundAPI from '@/api/playground'
import { usePlaygroundSettings } from '@/composables/usePlaygroundSettings'

const props = defineProps<{
  resolvedKey: string
}>()

const { t } = useI18n()
const settings = usePlaygroundSettings()
const model = settings.model

const prompt = ref('')
const size = ref('1024x1024')
const count = ref(1)
const loading = ref(false)
const error = ref('')
const images = ref<string[]>([])
const preview = ref('')

async function generate(): Promise<void> {
  if (!prompt.value.trim() || !props.resolvedKey || !model.value) return
  loading.value = true
  error.value = ''
  images.value = []
  try {
    const result = await playgroundAPI.generateImage({
      apiKey: props.resolvedKey,
      model: model.value,
      prompt: prompt.value.trim(),
      size: size.value === 'auto' ? undefined : size.value,
      n: count.value
    })
    images.value = result
      .map((img) => (img.b64_json ? `data:image/png;base64,${img.b64_json}` : img.url || ''))
      .filter(Boolean)
    if (images.value.length === 0) {
      error.value = t('playground.imageNoResult')
    }
  } catch (err) {
    error.value = (err as Error).message || t('playground.requestFailed')
  } finally {
    loading.value = false
  }
}
</script>

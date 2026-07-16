<template>
  <AppLayout>
    <div class="flex h-[calc(100dvh-6.5rem)] min-h-[520px] flex-col gap-3 sm:h-[calc(100vh-8rem)] sm:gap-4">
      <div class="flex flex-col gap-3 rounded-xl border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800/50">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="max-w-full overflow-x-auto">
            <div class="inline-flex min-w-max rounded-lg bg-gray-100 p-0.5 dark:bg-dark-700" role="tablist" :aria-label="t('playground.title')">
              <button
                v-for="item in modes"
                :key="item.value"
                class="flex min-h-11 items-center gap-1.5 rounded-md px-3 text-sm font-medium outline-none transition-colors focus-visible:ring-2 focus-visible:ring-primary-500"
                :class="mode === item.value ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white' : 'text-gray-500 hover:text-gray-700 dark:text-gray-400'"
                role="tab"
                :aria-selected="mode === item.value"
                @click="mode = item.value"
              >
                <Icon :name="item.icon as 'chat'" size="sm" />
                {{ item.label }}
              </button>
            </div>
          </div>

          <button
            v-if="mode === 'chat' || mode === 'compare'"
            class="flex min-h-11 max-w-full items-center gap-2 rounded-lg border border-gray-200 px-3 text-xs text-gray-600 outline-none transition-colors hover:border-gray-300 hover:text-gray-900 focus-visible:ring-2 focus-visible:ring-primary-500 dark:border-dark-600 dark:text-gray-300 dark:hover:text-white"
            :aria-expanded="showParams"
            @click="showParams = !showParams"
          >
            <Icon name="cog" size="sm" />
            <span class="truncate">{{ parameterSummary }}</span>
            <Icon :name="showParams ? 'chevronUp' : 'chevronDown'" size="xs" />
          </button>
        </div>

        <KeyModelPicker
          v-if="mode !== 'compare' && mode !== 'image'"
          v-model:key-id="settings.keyId.value"
          v-model:resolved-key="resolvedKey"
          v-model:option="currentOption"
          :capability="mode"
        />

        <PlaygroundParametersPanel
          v-if="showParams && (mode === 'chat' || mode === 'compare')"
          v-model="parameters"
          :mode="mode"
          :option="mode === 'chat' ? currentOption : null"
        />
      </div>

      <div class="min-h-0 flex-1">
        <ChatPanel
          v-if="mode === 'chat'"
          :key-id="settings.keyId.value"
          :resolved-key="resolvedKey"
          :option="currentOption"
          :parameters="parameters"
        />
        <ImagePanel
          v-else-if="mode === 'image'"
          v-model:key-id="settings.keyId.value"
          v-model:resolved-key="resolvedKey"
          v-model:option="currentOption"
        />
        <VideoPanel v-else-if="mode === 'video'" :resolved-key="resolvedKey" :option="currentOption" />
        <ComparePanel v-else :parameters="parameters" />
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import KeyModelPicker from '@/components/playground/KeyModelPicker.vue'
import ChatPanel from '@/components/playground/ChatPanel.vue'
import ImagePanel from '@/components/playground/ImagePanel.vue'
import VideoPanel from '@/components/playground/VideoPanel.vue'
import ComparePanel from '@/components/playground/ComparePanel.vue'
import PlaygroundParametersPanel from '@/components/playground/PlaygroundParametersPanel.vue'
import { usePlaygroundSettings } from '@/composables/usePlaygroundSettings'
import type { PlaygroundMode, PlaygroundModelOption } from '@/types/playground'
import type { PlaygroundParameterValues } from '@/components/playground/playgroundUiTypes'

const { t } = useI18n()
const settings = usePlaygroundSettings()
const mode = ref<PlaygroundMode>('chat')
const showParams = ref(false)
const resolvedKey = ref('')

const parameters = computed<PlaygroundParameterValues>({
  get: () => ({
    systemPrompt: settings.systemPrompt.value,
    temperature: settings.temperature.value,
    maxTokens: settings.maxTokens.value,
    topP: settings.topP.value,
    reasoningEffort: settings.reasoningEffort.value,
    webSearch: settings.webSearch.value,
    codeExecution: settings.codeExecution.value,
    webFetch: settings.webFetch.value
  }),
  set: (value) => {
    settings.systemPrompt.value = value.systemPrompt
    settings.temperature.value = value.temperature
    settings.maxTokens.value = value.maxTokens
    settings.topP.value = value.topP
    settings.reasoningEffort.value = value.reasoningEffort
    settings.webSearch.value = value.webSearch
    settings.codeExecution.value = value.codeExecution
    settings.webFetch.value = value.webFetch
  }
})

const currentOption = computed<PlaygroundModelOption | null>({
  get: () => {
    if (mode.value === 'image') return settings.imageOption.value
    if (mode.value === 'video') return settings.videoOption.value
    return settings.chatOption.value
  },
  set: (option) => {
    if (mode.value === 'image') settings.imageOption.value = option
    else if (mode.value === 'video') settings.videoOption.value = option
    else settings.chatOption.value = option
  }
})

const enabledToolCount = computed(() => mode.value === 'chat'
  ? [parameters.value.webSearch, parameters.value.codeExecution, parameters.value.webFetch].filter(Boolean).length
  : 0)
const parameterSummary = computed(() => {
  const sampling = `T ${parameters.value.temperature.toFixed(1)} · P ${parameters.value.topP.toFixed(2).replace(/0$/, '')}`
  const reasoning = !parameters.value.reasoningEffort || parameters.value.reasoningEffort === 'none' ? '' : ` · ${t('playground.reasoningShort')} ${parameters.value.reasoningEffort}`
  const tools = enabledToolCount.value ? ` · ${t('playground.toolCount', { count: enabledToolCount.value })}` : ''
  return `${sampling}${reasoning}${tools}`
})

const modes = computed<{ value: PlaygroundMode; label: string; icon: string }[]>(() => [
  { value: 'chat', label: t('playground.modeChat'), icon: 'chat' },
  { value: 'image', label: t('playground.modeImage'), icon: 'sparkles' },
  { value: 'video', label: t('playground.modeVideo'), icon: 'play' },
  { value: 'compare', label: t('playground.modeCompare'), icon: 'grid' }
])

function supportsFeature(option: PlaygroundModelOption | null, feature: 'web_search' | 'code_execution' | 'web_fetch'): boolean {
  if (!option?.features) return false
  if (Array.isArray(option.features)) return option.features.includes(feature)
  return option.features[feature] === true
}

watch(mode, () => {
  if (mode.value === 'image' || mode.value === 'video') showParams.value = false
})
watch(currentOption, (option) => {
  if (mode.value !== 'chat' || !option) return
  if (!supportsFeature(option, 'web_search')) settings.webSearch.value = false
  if (!supportsFeature(option, 'code_execution')) settings.codeExecution.value = false
  if (!supportsFeature(option, 'web_fetch')) settings.webFetch.value = false
})
</script>

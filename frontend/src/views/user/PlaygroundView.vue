<template>
  <AppLayout>
    <div class="flex h-[calc(100vh-8rem)] min-h-[520px] flex-col gap-4">
      <!-- Mode tabs + shared toolbar -->
      <div class="flex flex-col gap-3 rounded-xl border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800/50">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="inline-flex rounded-lg bg-gray-100 p-0.5 dark:bg-dark-700">
            <button
              v-for="m in modes"
              :key="m.value"
              class="flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
              :class="
                mode === m.value
                  ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white'
                  : 'text-gray-500 hover:text-gray-700 dark:text-gray-400'
              "
              @click="mode = m.value"
            >
              <Icon :name="m.icon as 'chat'" size="sm" />
              {{ m.label }}
            </button>
          </div>

          <button
            class="flex items-center gap-1.5 text-xs text-gray-500 hover:text-gray-700 dark:text-gray-400"
            @click="showParams = !showParams"
          >
            <Icon name="cog" size="sm" />
            {{ t('playground.parameters') }}
            <Icon :name="showParams ? 'chevronUp' : 'chevronDown'" size="xs" />
          </button>
        </div>

        <!-- Shared key + model picker (chat & image share it; compare has per-column) -->
        <KeyModelPicker
          v-if="mode !== 'compare'"
          v-model:key-id="settings.keyId.value"
          v-model:model="settings.model.value"
          @resolved-key="(k) => (resolvedKey = k)"
        />

        <!-- Advanced params -->
        <div v-if="showParams && mode !== 'compare'" class="grid gap-3 border-t border-gray-100 pt-3 dark:border-dark-700 sm:grid-cols-2 lg:grid-cols-4">
          <div class="sm:col-span-2 lg:col-span-4">
            <label class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
              {{ t('playground.systemPrompt') }}
            </label>
            <input
              v-model="settings.systemPrompt.value"
              type="text"
              :placeholder="t('playground.systemPromptPlaceholder')"
              class="input"
            />
          </div>
          <div>
            <label class="mb-1 flex items-center justify-between text-xs font-medium text-gray-500 dark:text-gray-400">
              <span>{{ t('playground.temperature') }}</span>
              <span class="tabular-nums text-gray-400">{{ settings.temperature.value.toFixed(1) }}</span>
            </label>
            <input v-model.number="settings.temperature.value" type="range" min="0" max="2" step="0.1" class="w-full accent-primary-500" />
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
              {{ t('playground.maxTokens') }}
            </label>
            <input
              v-model.number="settings.maxTokens.value"
              type="number"
              min="0"
              :placeholder="t('playground.maxTokensPlaceholder')"
              class="input"
            />
          </div>
        </div>
      </div>

      <!-- Panel -->
      <div class="min-h-0 flex-1">
        <ChatPanel v-if="mode === 'chat'" :resolved-key="resolvedKey" />
        <ImagePanel v-else-if="mode === 'image'" :resolved-key="resolvedKey" />
        <ComparePanel v-else />
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import KeyModelPicker from '@/components/playground/KeyModelPicker.vue'
import ChatPanel from '@/components/playground/ChatPanel.vue'
import ImagePanel from '@/components/playground/ImagePanel.vue'
import ComparePanel from '@/components/playground/ComparePanel.vue'
import { usePlaygroundSettings } from '@/composables/usePlaygroundSettings'

const { t } = useI18n()
const settings = usePlaygroundSettings()

type Mode = 'chat' | 'image' | 'compare'
const mode = ref<Mode>('chat')
const showParams = ref(false)
const resolvedKey = ref('')

const modes = computed<{ value: Mode; label: string; icon: string }[]>(() => [
  { value: 'chat', label: t('playground.modeChat'), icon: 'chat' },
  { value: 'image', label: t('playground.modeImage'), icon: 'sparkles' },
  { value: 'compare', label: t('playground.modeCompare'), icon: 'grid' }
])
</script>

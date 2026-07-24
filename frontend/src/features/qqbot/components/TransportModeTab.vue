<template>
  <div class="space-y-6">
    <div>
      <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.qqbot.transport.title') }}</h2>
      <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-dark-300">{{ t('admin.qqbot.transport.description') }}</p>
    </div>

    <div v-if="inherited" class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-200" role="status">
      {{ t('admin.qqbot.transport.inherited') }}
    </div>
    <div v-if="error" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300" role="alert">
      {{ error }}
    </div>

    <div class="grid gap-4 lg:grid-cols-2" role="radiogroup" :aria-label="t('admin.qqbot.transport.title')">
      <button
        v-for="option in options"
        :key="option.mode"
        type="button"
        role="radio"
        class="rounded-xl border p-5 text-left transition-colors"
        :class="option.mode === mode ? selectedClass : defaultClass"
        :aria-checked="option.mode === mode"
        :disabled="loading"
        :data-test="`transport-${option.mode}`"
        @click="$emit('select', option.mode)"
      >
        <div class="flex items-start justify-between gap-4">
          <div>
            <h3 class="text-sm font-semibold">{{ option.title }}</h3>
            <p class="mt-2 text-sm opacity-80">{{ option.description }}</p>
          </div>
          <span v-if="option.mode === mode" class="shrink-0 rounded-full border border-current px-2 py-0.5 text-xs font-medium">
            {{ t('admin.qqbot.transport.selected') }}
          </span>
        </div>
      </button>
    </div>

    <p v-if="loading" class="text-sm text-gray-500 dark:text-dark-300">{{ t('admin.qqbot.transport.switching') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { QQBotTransportMode } from '../types'

defineProps<{
  mode: QQBotTransportMode
  inherited: boolean
  loading: boolean
  error: string
}>()

defineEmits<{ select: [mode: QQBotTransportMode] }>()
const { t } = useI18n()
const selectedClass = 'border-primary-500 bg-primary-50 text-primary-900 dark:border-primary-400 dark:bg-primary-950/30 dark:text-primary-100'
const defaultClass = 'border-gray-200 bg-white text-gray-900 hover:border-primary-300 dark:border-dark-700 dark:bg-dark-800 dark:text-white dark:hover:border-primary-600'

const options = computed(() => [
  {
    mode: 'botgo' as const,
    title: t('admin.qqbot.transport.modes.botgo'),
    description: t('admin.qqbot.transport.botgoDescription'),
  },
  {
    mode: 'onebot' as const,
    title: t('admin.qqbot.transport.modes.onebot'),
    description: t('admin.qqbot.transport.onebotDescription'),
  },
])
</script>

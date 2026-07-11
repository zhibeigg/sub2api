<template>
  <section
    class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800/70"
    :aria-label="t('playground.parameters')"
  >
    <div class="mb-4 flex items-start justify-between gap-3">
      <div>
        <h2 class="text-sm font-semibold text-gray-800 dark:text-gray-100">{{ t('playground.parameters') }}</h2>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ compatibilitySummary }}</p>
      </div>
      <button
        type="button"
        class="min-h-11 rounded-lg px-3 text-xs font-medium text-gray-500 outline-none transition-colors hover:bg-gray-100 hover:text-gray-800 focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-gray-100"
        @click="restoreDefaults"
      >
        {{ t('playground.restoreDefaults') }}
      </button>
    </div>

    <div v-if="isChatMode" class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <label class="sm:col-span-2 lg:col-span-4">
        <span class="mb-1.5 block text-xs font-medium text-gray-600 dark:text-gray-300">{{ t('playground.systemPrompt') }}</span>
        <textarea
          :value="modelValue.systemPrompt"
          rows="2"
          class="input min-h-[72px] resize-y"
          :placeholder="t('playground.systemPromptPlaceholder')"
          @input="update('systemPrompt', ($event.target as HTMLTextAreaElement).value)"
        ></textarea>
      </label>

      <ParameterRange
        :label="t('playground.temperature')"
        :value="modelValue.temperature"
        :disabled="!supports.temperature"
        :unsupported-label="t('playground.unsupportedByModel')"
        :min="0"
        :max="2"
        :step="0.1"
        @update:value="update('temperature', $event)"
      />
      <ParameterRange
        :label="t('playground.topP')"
        :value="modelValue.topP"
        :disabled="!supports.topP"
        :unsupported-label="t('playground.unsupportedByModel')"
        :min="0"
        :max="1"
        :step="0.05"
        @update:value="update('topP', $event)"
      />

      <label>
        <span class="mb-1.5 flex items-center justify-between gap-2 text-xs font-medium text-gray-600 dark:text-gray-300">
          <span>{{ t('playground.maxTokens') }}</span>
          <span v-if="!supports.maxTokens" class="font-normal text-amber-600 dark:text-amber-400">{{ t('playground.unsupportedByModel') }}</span>
        </span>
        <input
          :value="modelValue.maxTokens"
          type="number"
          min="0"
          inputmode="numeric"
          class="input min-h-11"
          :disabled="!supports.maxTokens"
          :placeholder="t('playground.maxTokensPlaceholder')"
          @input="update('maxTokens', Number(($event.target as HTMLInputElement).value) || 0)"
        />
      </label>

      <label>
        <span class="mb-1.5 flex items-center justify-between gap-2 text-xs font-medium text-gray-600 dark:text-gray-300">
          <span>{{ t('playground.reasoningEffort') }}</span>
          <span v-if="!supports.reasoning" class="font-normal text-amber-600 dark:text-amber-400">{{ t('playground.unsupportedByModel') }}</span>
        </span>
        <select
          :value="modelValue.reasoningEffort"
          class="input min-h-11"
          :disabled="!supports.reasoning"
          @change="update('reasoningEffort', ($event.target as HTMLSelectElement).value as PlaygroundReasoningEffort)"
        >
          <option value="">{{ t('playground.reasoningNone') }}</option>
          <option value="minimal">{{ t('playground.reasoningMinimal') }}</option>
          <option value="low">{{ t('playground.reasoningLow') }}</option>
          <option value="medium">{{ t('playground.reasoningMedium') }}</option>
          <option value="high">{{ t('playground.reasoningHigh') }}</option>
          <option value="xhigh">{{ t('playground.reasoningXHigh') }}</option>
        </select>
      </label>
    </div>

    <div v-if="showTools" class="mt-5 border-t border-gray-100 pt-4 dark:border-dark-700">
      <h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
        {{ t('playground.tools') }}
      </h3>
      <div class="grid gap-2 sm:grid-cols-3">
        <ToolToggle
          :label="t('playground.webSearch')"
          :description="t('playground.webSearchHint')"
          :enabled="modelValue.webSearch"
          :supported="supports.webSearch"
          @update:enabled="update('webSearch', $event)"
        />
        <ToolToggle
          :label="t('playground.codeExecution')"
          :description="t('playground.codeExecutionHint')"
          :enabled="modelValue.codeExecution"
          :supported="supports.codeExecution"
          @update:enabled="update('codeExecution', $event)"
        />
        <ToolToggle
          :label="t('playground.webFetch')"
          :description="t('playground.webFetchHint')"
          :enabled="modelValue.webFetch"
          :supported="supports.webFetch"
          @update:enabled="update('webFetch', $event)"
        />
      </div>
    </div>

    <slot v-else />
  </section>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, type PropType } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PlaygroundModelOption, PlaygroundMode } from '@/types/playground'
import type {
  PlaygroundParameterValues,
  PlaygroundReasoningEffort
} from './playgroundUiTypes'

const DEFAULTS: PlaygroundParameterValues = {
  systemPrompt: '',
  temperature: 0.7,
  topP: 1,
  maxTokens: 0,
  reasoningEffort: '',
  webSearch: false,
  codeExecution: false,
  webFetch: false
}

const props = defineProps<{
  mode: PlaygroundMode
  option: PlaygroundModelOption | null
  modelValue: PlaygroundParameterValues
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: PlaygroundParameterValues): void
}>()

const { t } = useI18n()
const isChatMode = computed(() => props.mode === 'chat' || props.mode === 'compare')
const showTools = computed(() => props.mode === 'chat')

function featureInfo(): { features: Set<string>; hasMetadata: boolean } {
  const option = props.option as (PlaygroundModelOption & { supported_features?: string[] }) | null
  const raw = option?.features
  const names = Array.isArray(raw)
    ? raw
    : raw && typeof raw === 'object'
      ? Object.entries(raw).filter(([, enabled]) => enabled).map(([name]) => name)
      : option?.supported_features ?? []
  return {
    features: new Set(names.map((name) => String(name).toLowerCase().replace(/[\s_-]/g, ''))),
    hasMetadata: raw != null || Array.isArray(option?.supported_features)
  }
}

const supports = computed(() => {
  const { features, hasMetadata } = featureInfo()
  const has = (...names: string[]) => hasMetadata && names.some((name) => features.has(name))
  return {
    temperature: true,
    topP: true,
    maxTokens: true,
    reasoning: true,
    webSearch: has('websearch', 'search'),
    codeExecution: has('codeexecution', 'codeinterpreter'),
    webFetch: has('webfetch', 'urlcontext')
  }
})

const compatibilitySummary = computed(() => {
  if (props.mode === 'compare') return t('playground.compareParametersHint')
  if (!props.option) return t('playground.selectModelForCompatibility')
  const values = showTools.value
    ? Object.values(supports.value)
    : [supports.value.temperature, supports.value.topP, supports.value.maxTokens, supports.value.reasoning]
  const supported = values.filter(Boolean).length
  return supported === values.length
    ? t('playground.featuresCompatible')
    : t('playground.featuresCompatibleCount', { supported, total: values.length })
})

function update<K extends keyof PlaygroundParameterValues>(key: K, value: PlaygroundParameterValues[K]): void {
  emit('update:modelValue', { ...props.modelValue, [key]: value })
}

function restoreDefaults(): void {
  emit('update:modelValue', { ...DEFAULTS })
}

const ParameterRange = defineComponent({
  props: {
    label: { type: String, required: true },
    value: { type: Number, required: true },
    min: { type: Number, required: true },
    max: { type: Number, required: true },
    step: { type: Number, required: true },
    disabled: Boolean,
    unsupportedLabel: { type: String, required: true }
  },
  emits: ['update:value'],
  setup(rangeProps, { emit: rangeEmit }) {
    return () => h('label', {}, [
      h('span', { class: 'mb-1.5 flex items-center justify-between gap-2 text-xs font-medium text-gray-600 dark:text-gray-300' }, [
        h('span', rangeProps.label),
        h('span', { class: rangeProps.disabled ? 'font-normal text-amber-600 dark:text-amber-400' : 'tabular-nums font-normal text-gray-400' }, rangeProps.disabled ? rangeProps.unsupportedLabel : rangeProps.value.toFixed(2).replace(/0$/, ''))
      ]),
      h('input', {
        type: 'range',
        min: rangeProps.min,
        max: rangeProps.max,
        step: rangeProps.step,
        value: rangeProps.value,
        disabled: rangeProps.disabled,
        class: 'min-h-11 w-full accent-primary-500 disabled:cursor-not-allowed disabled:opacity-40',
        onInput: (event: Event) => rangeEmit('update:value', Number((event.target as HTMLInputElement).value))
      })
    ])
  }
})

const ToolToggle = defineComponent({
  props: {
    label: { type: String, required: true },
    description: { type: String, required: true },
    enabled: Boolean,
    supported: Boolean as PropType<boolean>
  },
  emits: ['update:enabled'],
  setup(toggleProps, { emit: toggleEmit }) {
    return () => h('label', {
      class: [
        'flex min-h-16 cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors focus-within:ring-2 focus-within:ring-primary-500',
        toggleProps.supported ? 'border-gray-200 hover:border-gray-300 dark:border-dark-600' : 'cursor-not-allowed border-gray-100 opacity-55 dark:border-dark-700'
      ]
    }, [
      h('input', {
        type: 'checkbox',
        checked: toggleProps.enabled,
        disabled: !toggleProps.supported,
        class: 'mt-0.5 h-5 w-5 rounded border-gray-300 text-primary-500 focus:ring-primary-500',
        onChange: (event: Event) => toggleEmit('update:enabled', (event.target as HTMLInputElement).checked)
      }),
      h('span', { class: 'min-w-0' }, [
        h('span', { class: 'block text-sm font-medium text-gray-700 dark:text-gray-200' }, toggleProps.label),
        h('span', { class: 'mt-0.5 block text-xs leading-4 text-gray-500 dark:text-gray-400' }, toggleProps.supported ? toggleProps.description : t('playground.unsupportedByModel'))
      ])
    ])
  }
})
</script>

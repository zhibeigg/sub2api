<template>
  <div class="space-y-4">
    <SortableGroupBindingPicker
      :model-value="modelValue"
      :groups="groups"
      :user-group-rates="userGroupRates"
      @update:model-value="emit('update:modelValue', $event)"
    />

    <div v-if="orderedBindings.length" class="space-y-3" data-test="group-models-list">
      <section
        v-for="binding in orderedBindings"
        :key="`models-${binding.group_id}`"
        :class="[
          'rounded-lg border p-3',
          groupFor(binding.group_id)
            ? platformBadgeClass(groupFor(binding.group_id)!.platform)
            : 'border-gray-200 bg-gray-50 text-gray-600 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300'
        ]"
        :data-test="`group-models-${binding.group_id}`"
      >
        <div class="mb-2 flex flex-wrap items-center gap-1.5 text-xs font-semibold">
          <PlatformIcon
            v-if="groupFor(binding.group_id)"
            :platform="groupFor(binding.group_id)!.platform"
            size="xs"
          />
          <span>{{ groupFor(binding.group_id)?.name ?? `#${binding.group_id}` }}</span>
          <span class="opacity-50">·</span>
          <span class="opacity-70">
            {{ t('keys.groupModels', { count: modelsFor(binding.group_id).length }) }}
          </span>
        </div>
        <div v-if="modelsFor(binding.group_id).length" class="flex flex-wrap gap-1.5">
          <span
            v-for="model in modelsFor(binding.group_id)"
            :key="`${binding.group_id}-${model}`"
            class="inline-flex items-center gap-1 rounded-md bg-white/80 px-2 py-0.5 text-[11px] text-gray-700 shadow-sm dark:bg-dark-700/80 dark:text-gray-200"
          >
            <ModelIcon :model="model" size="14px" />
            {{ model }}
          </span>
        </div>
        <p v-else class="text-[11px] opacity-60">{{ t('keys.groupNoModels') }}</p>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import SortableGroupBindingPicker from '@/components/keys/SortableGroupBindingPicker.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import userChannelsAPI from '@/api/channels'
import { platformBadgeClass } from '@/utils/platformColors'
import type { ApiKeyGroupBindingInput, Group } from '@/types'

const props = defineProps<{
  groups: Group[]
  userGroupRates: Record<number, number>
  modelValue: ApiKeyGroupBindingInput[]
}>()

const emit = defineEmits<{
  (event: 'update:modelValue', value: ApiKeyGroupBindingInput[]): void
}>()

const { t } = useI18n()
const modelsByGroup = ref<Record<number, string[]>>({})
let modelsAbortController: AbortController | null = null

const orderedBindings = computed(() =>
  [...(props.modelValue ?? [])].sort((left, right) => left.priority - right.priority)
)
const groupMap = computed(() => new Map(props.groups.map((group) => [group.id, group])))

function groupFor(groupId: number): Group | undefined {
  return groupMap.value.get(groupId)
}

function modelsFor(groupId: number): string[] {
  return modelsByGroup.value[groupId] ?? []
}

async function loadModels(): Promise<void> {
  modelsAbortController?.abort()
  const controller = new AbortController()
  modelsAbortController = controller
  try {
    const channels = await userChannelsAPI.getModelSquare({ signal: controller.signal })
    if (controller.signal.aborted) return

    const groupedModels = new Map<number, Set<string>>()
    for (const channel of channels) {
      for (const section of channel.platforms) {
        for (const group of section.groups) {
          const models = groupedModels.get(group.id) ?? new Set<string>()
          for (const model of section.supported_models) models.add(model.name)
          groupedModels.set(group.id, models)
        }
      }
    }

    modelsByGroup.value = Object.fromEntries(
      [...groupedModels.entries()].map(([groupId, models]) => [
        groupId,
        [...models].sort((left, right) => left.localeCompare(right))
      ])
    )
  } catch (error) {
    if (!controller.signal.aborted) {
      console.error('Failed to load API key group models:', error)
      modelsByGroup.value = {}
    }
  }
}

onMounted(() => {
  void loadModels()
})

onUnmounted(() => {
  modelsAbortController?.abort()
})
</script>

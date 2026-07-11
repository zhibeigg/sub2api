<template>
  <SortableGroupBindingPicker
    :model-value="modelValue"
    :groups="groups"
    :user-group-rates="userGroupRates"
    @update:model-value="emit('update:modelValue', $event)"
  />
</template>

<script setup lang="ts">
import SortableGroupBindingPicker from '@/components/keys/SortableGroupBindingPicker.vue'
import type { ApiKeyGroupBindingInput, Group } from '@/types'

defineProps<{
  groups: Group[]
  userGroupRates: Record<number, number>
  modelValue: ApiKeyGroupBindingInput[]
}>()

const emit = defineEmits<{
  (event: 'update:modelValue', value: ApiKeyGroupBindingInput[]): void
}>()
</script>

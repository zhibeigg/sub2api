<template>
  <div class="space-y-6">
    <div>
      <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.qqbot.messages.title') }}</h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('admin.qqbot.messages.description') }}</p>
    </div>

    <section class="grid gap-4 rounded-xl border border-gray-200 bg-white p-5 md:grid-cols-2 xl:grid-cols-4 dark:border-dark-700 dark:bg-dark-800">
      <ToggleField :checked="draft.binding_enabled" :label="t('admin.qqbot.messages.bindingEnabled')" :hint="t('admin.qqbot.messages.bindingEnabledHint')" @change="update('binding_enabled', $event)" />
      <ToggleField :checked="draft.welcome_enabled" :label="t('admin.qqbot.messages.welcomeEnabled')" :hint="t('admin.qqbot.messages.welcomeEnabledHint')" @change="update('welcome_enabled', $event)" />
      <ToggleField :checked="draft.first_interaction_enabled" :label="t('admin.qqbot.messages.firstInteraction')" :hint="t('admin.qqbot.messages.firstInteractionHint')" @change="update('first_interaction_enabled', $event)" />
      <ToggleField :checked="draft.channel_check_enabled" :label="t('admin.qqbot.messages.channelCheckEnabled')" :hint="t('admin.qqbot.messages.channelCheckEnabledHint')" @change="update('channel_check_enabled', $event)" />
    </section>

    <section class="grid gap-5 rounded-xl border border-gray-200 bg-white p-5 sm:grid-cols-2 dark:border-dark-700 dark:bg-dark-800">
      <div>
        <label class="input-label" for="qqbot-bonus">{{ t('admin.qqbot.messages.firstBindBonus') }}</label>
        <input id="qqbot-bonus" type="number" min="0" step="0.01" class="input" :value="draft.first_bind_bonus" @input="update('first_bind_bonus', Number(valueOf($event)))" />
      </div>
      <div>
        <label class="input-label" for="qqbot-link-ttl">{{ t('admin.qqbot.messages.linkTtl') }}</label>
        <input id="qqbot-link-ttl" type="number" min="5" max="1440" class="input" :value="draft.link_ttl_minutes" @input="update('link_ttl_minutes', Number(valueOf($event)))" />
      </div>
      <div>
        <label class="input-label" for="qqbot-command-cooldown">{{ t('admin.qqbot.messages.commandCooldown') }}</label>
        <input id="qqbot-command-cooldown" type="number" min="10" max="3600" class="input" :value="draft.command_cooldown_seconds" @input="update('command_cooldown_seconds', Number(valueOf($event)))" />
        <p class="input-hint">{{ t('admin.qqbot.messages.commandCooldownHint') }}</p>
      </div>
      <div class="sm:col-span-2">
        <label class="input-label" for="qqbot-welcome-message">{{ t('admin.qqbot.messages.welcomeMessage') }}</label>
        <textarea id="qqbot-welcome-message" rows="7" maxlength="4000" class="input resize-y font-mono text-xs" :value="draft.welcome_message" @input="update('welcome_message', valueOf($event))"></textarea>
        <p class="input-hint">{{ t('admin.qqbot.messages.welcomeMessageHint', { count: draft.welcome_message.length }) }}</p>
      </div>
      <div class="sm:col-span-2">
        <label class="input-label" for="qqbot-help-message">{{ t('admin.qqbot.messages.helpMessage') }}</label>
        <textarea id="qqbot-help-message" rows="7" maxlength="4000" class="input resize-y font-mono text-xs" :value="draft.help_message" @input="update('help_message', valueOf($event))"></textarea>
        <p class="input-hint">{{ t('admin.qqbot.messages.helpMessageHint', { count: draft.help_message.length }) }}</p>
      </div>
    </section>

    <section class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
      <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.qqbot.messages.allowlistTitle') }}</h3>
      <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.qqbot.messages.allowlistHint') }}</p>
      <div class="mt-4 grid gap-5 lg:grid-cols-2">
        <div>
          <label class="input-label" for="qqbot-groups">{{ t('admin.qqbot.messages.allowedGroups') }}</label>
          <textarea id="qqbot-groups" rows="6" class="input resize-y font-mono text-xs" :value="draft.allowed_group_ids_text" :placeholder="t('admin.qqbot.messages.onePerLine')" @input="update('allowed_group_ids_text', valueOf($event))"></textarea>
        </div>
        <div>
          <label class="input-label" for="qqbot-guilds">{{ t('admin.qqbot.messages.allowedGuilds') }}</label>
          <textarea id="qqbot-guilds" rows="6" class="input resize-y font-mono text-xs" :value="draft.allowed_guild_ids_text" :placeholder="t('admin.qqbot.messages.onePerLine')" @input="update('allowed_guild_ids_text', valueOf($event))"></textarea>
        </div>
        <div class="lg:col-span-2">
          <label class="input-label" for="qqbot-welcome-map">{{ t('admin.qqbot.messages.welcomeChannels') }}</label>
          <textarea id="qqbot-welcome-map" rows="6" class="input resize-y font-mono text-xs" :value="draft.guild_welcome_channels_text" placeholder="guild_id = channel_id" @input="update('guild_welcome_channels_text', valueOf($event))"></textarea>
          <p class="input-hint">{{ t('admin.qqbot.messages.welcomeChannelsHint') }}</p>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { defineComponent, h } from 'vue'
import { useI18n } from 'vue-i18n'
import type { QQBotDraft } from '../types'

const props = defineProps<{ draft: QQBotDraft }>()
const emit = defineEmits<{ 'update:draft': [value: QQBotDraft] }>()
const { t } = useI18n()

const ToggleField = defineComponent({
  props: { checked: Boolean, label: { type: String, required: true }, hint: { type: String, required: true } },
  emits: ['change'],
  setup(toggleProps, { emit: toggleEmit }) {
    return () => h('label', { class: 'flex cursor-pointer items-start gap-3 rounded-xl border border-gray-200 p-4 dark:border-dark-700' }, [
      h('input', {
        type: 'checkbox',
        checked: toggleProps.checked,
        class: 'mt-0.5 h-4 w-4 accent-primary-600',
        onChange: (event: Event) => toggleEmit('change', (event.target as HTMLInputElement).checked),
      }),
      h('span', {}, [
        h('span', { class: 'block text-sm font-medium text-gray-900 dark:text-white' }, toggleProps.label),
        h('span', { class: 'mt-1 block text-xs text-gray-500 dark:text-dark-400' }, toggleProps.hint),
      ]),
    ])
  },
})

function update<K extends keyof QQBotDraft>(key: K, value: QQBotDraft[K]) {
  emit('update:draft', { ...props.draft, [key]: value })
}
function valueOf(event: Event) { return (event.target as HTMLInputElement | HTMLTextAreaElement).value }
</script>

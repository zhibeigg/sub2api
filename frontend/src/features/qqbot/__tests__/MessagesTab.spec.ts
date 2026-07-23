import { baseCompile } from '@intlify/message-compiler'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import en from '../../../i18n/locales/en/admin/qqbot'
import zh from '../../../i18n/locales/zh/admin/qqbot'
import MessagesTab from '../components/MessagesTab.vue'
import type { QQBotConfig } from '../types'
import { configToDraft } from '../viewModel'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

const config = (): QQBotConfig => ({
  enabled: false,
  app_id: '102087449',
  app_secret_configured: true,
  webhook_secret_configured: true,
  sandbox: false,
  public_base_url: 'https://qqbot.example.com',
  worker_count: 4,
  queue_capacity: 1000,
  api_timeout_ms: 5000,
  binding_enabled: true,
  first_bind_bonus: 5,
  link_ttl_minutes: 15,
  welcome_enabled: true,
  welcome_message: 'welcome {user}',
  first_interaction_enabled: true,
  channel_check_enabled: false,
  help_message: 'help',
  allowed_group_ids: ['group-1'],
  allowed_guild_ids: ['guild-1'],
  guild_welcome_channels: { 'guild-1': 'channel-1' },
  config_version: 7,
})

function localeKeys(value: unknown, prefix = ''): string[] {
  if (!value || typeof value !== 'object') return [prefix]
  return Object.entries(value).flatMap(([key, child]) => localeKeys(child, prefix ? `${prefix}.${key}` : key))
}

describe('QQBot MessagesTab', () => {
  it('renders and updates the /check channel status image switch', async () => {
    const wrapper = mount(MessagesTab, { props: { draft: configToDraft(config()) } })
    const toggle = wrapper.findAll('label').find((label) => label.text().includes('admin.qqbot.messages.channelCheckEnabled'))

    expect(toggle).toBeDefined()
    expect(wrapper.text()).toContain('admin.qqbot.messages.channelCheckEnabledHint')

    const checkbox = toggle!.get('input[type="checkbox"]')
    expect((checkbox.element as HTMLInputElement).checked).toBe(false)
    await checkbox.setValue(true)

    const updates = wrapper.emitted('update:draft')
    expect(updates).toBeTruthy()
    expect(updates?.at(-1)?.[0]).toMatchObject({ channel_check_enabled: true })
  })

  it('renders and updates the standalone member welcome message', async () => {
    const wrapper = mount(MessagesTab, { props: { draft: configToDraft(config()) } })
    const textarea = wrapper.get('#qqbot-welcome-message')

    expect((textarea.element as HTMLTextAreaElement).value).toBe('welcome {user}')
    expect(wrapper.text()).toContain('admin.qqbot.messages.welcomeMessageHint')

    await textarea.setValue('hello {site} {user}')
    const updates = wrapper.emitted('update:draft')
    expect(updates?.at(-1)?.[0]).toMatchObject({ welcome_message: 'hello {site} {user}' })
  })

  it('keeps Chinese and English QQBot keys symmetric and describes allowlists as fail-closed', () => {
    expect(localeKeys(zh).sort()).toEqual(localeKeys(en).sort())
    expect(zh.qqbot.messages.channelCheckEnabled).toBe('允许 /check 渠道状态图')
    expect(en.qqbot.messages.channelCheckEnabled).toBe('Allow /check Channel Status Image')
    expect(zh.qqbot.messages.welcomeMessageHint).toContain('bind_command')
    expect(en.qqbot.messages.welcomeMessageHint).toContain('bind_command')
    expect(() => baseCompile(zh.qqbot.messages.welcomeMessageHint)).not.toThrow()
    expect(() => baseCompile(en.qqbot.messages.welcomeMessageHint)).not.toThrow()
    expect(zh.qqbot.messages.allowlistHint).toContain('fail-closed')
    expect(zh.qqbot.messages.allowlistHint).toContain('并非不限制')
    expect(en.qqbot.messages.allowlistHint).toContain('fail-closed')
    expect(en.qqbot.messages.allowlistHint).toContain('not unrestricted')
  })
})

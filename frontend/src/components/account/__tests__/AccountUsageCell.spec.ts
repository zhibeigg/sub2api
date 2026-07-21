import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountUsageCell from '../AccountUsageCell.vue'
import type { Account } from '@/types'

const { getUsage } = vi.hoisted(() => ({
  getUsage: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getUsage
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

function makeAccount(overrides: Partial<Account>): Account {
  return {
    id: 1,
    name: 'account',
    platform: 'antigravity',
    type: 'oauth',
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: true,
    created_at: '2026-03-15T00:00:00Z',
    updated_at: '2026-03-15T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...overrides,
  }
}

describe('AccountUsageCell', () => {
  beforeEach(() => {
    getUsage.mockReset()
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation(() => ({
        matches: true,
        media: '(min-width: 768px)',
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      }))
    })
  })

  it('OpenCode Go 按 open_code_quota schema 展示滚动、周、月窗口并支持刷新', async () => {
    getUsage.mockResolvedValue({
      open_code_quota: {
        configured: true,
        state: 'verified',
        workspace_id: 'workspace-1',
        fetched_at: '2026-07-18T10:00:00Z',
        rolling: { status: 'active', usage_percent: 25, reset_in_seconds: 7200 },
        weekly: { status: 'active', usage_percent: 50, reset_in_seconds: 0, reset_at: '2026-07-20T00:00:00Z' },
        monthly: { status: 'active', usage_percent: 75, reset_in_seconds: 0, reset_at: '2026-08-01T00:00:00Z' }
      }
    })
    const wrapper = mount(AccountUsageCell, {
      props: { account: makeAccount({ id: 8801, platform: 'opencode', type: 'apikey', credentials_status: { has_quota_cookie: true } }) },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ resetsAt }}</div>'
          },
          AccountQuotaInfo: true
        }
      }
    })
    await flushPromises()

    expect(getUsage).toHaveBeenCalledWith(8801)
    expect(wrapper.text()).toContain('admin.accounts.opencode.rolling|25|2026-07-18T12:00:00.000Z')
    expect(wrapper.text()).toContain('admin.accounts.opencode.weekly|50|2026-07-20T00:00:00Z')
    expect(wrapper.text()).toContain('admin.accounts.opencode.monthly|75|2026-08-01T00:00:00Z')

    await wrapper.get('[data-testid="opencode-usage-refresh"]').trigger('click')
    await flushPromises()
    expect(getUsage).toHaveBeenLastCalledWith(8801, 'active', true)
  })

  it('OpenCode Go 按 configured/state/message 区分未配置与错误状态', async () => {
    getUsage.mockResolvedValueOnce({
      open_code_quota: {
        configured: false,
        state: 'missing',
        rolling: { status: 'missing', usage_percent: 0, reset_in_seconds: 0 },
        weekly: { status: 'missing', usage_percent: 0, reset_in_seconds: 0 }
      }
    })
    const missing = mount(AccountUsageCell, {
      props: { account: makeAccount({ id: 8802, platform: 'opencode', type: 'apikey' }) },
      global: { stubs: { UsageProgressBar: true, AccountQuotaInfo: true } }
    })
    await flushPromises()
    expect(missing.get('[data-testid="opencode-quota-not-configured"]').text()).toContain('admin.accounts.opencode.quotaNotConfigured')

    getUsage.mockResolvedValueOnce({
      open_code_quota: {
        configured: true,
        state: 'unavailable',
        message: 'Go entitlement is inactive'
      }
    })
    const unavailable = mount(AccountUsageCell, {
      props: { account: makeAccount({ id: 8804, platform: 'opencode', type: 'apikey', credentials_status: { has_quota_cookie: true } }) },
      global: { stubs: { UsageProgressBar: true, AccountQuotaInfo: true } }
    })
    await flushPromises()
    expect(unavailable.get('[data-testid="opencode-quota-unavailable"]').text()).toContain('admin.accounts.opencode.quotaNoEntitlement')
    expect(unavailable.get('[data-testid="opencode-quota-unavailable"]').attributes('title')).toBe('Go entitlement is inactive')

    getUsage.mockResolvedValueOnce({
      open_code_quota: {
        configured: true,
        state: 'error',
        message: 'quota upstream failed',
        rolling: { status: 'error', usage_percent: 0, reset_in_seconds: 0 },
        weekly: { status: 'error', usage_percent: 0, reset_in_seconds: 0 }
      }
    })
    const failed = mount(AccountUsageCell, {
      props: { account: makeAccount({ id: 8803, platform: 'opencode', type: 'apikey', credentials_status: { has_quota_cookie: true } }) },
      global: { stubs: { UsageProgressBar: true, AccountQuotaInfo: true } }
    })
    await flushPromises()
    expect(failed.get('[data-testid="opencode-quota-error"]').attributes('title')).toBe('quota upstream failed')
  })

  it.each([
    [{ state: 'unknown', unknown: true, available: null }, 'admin.accounts.adobe.creditsUnknown'],
    [{ state: 'available', available: 0, checked_at: '2026-08-01T00:00:00Z' }, '0'],
    [{ state: 'available', available: 42 }, '42'],
  ])('Adobe credits 区分 unknown、0 和正常余额', async (adobeCredits, expected) => {
    getUsage.mockResolvedValue({ adobe_credits: adobeCredits })
    const wrapper = mount(AccountUsageCell, {
      props: { account: makeAccount({ id: 9000 + Math.random(), platform: 'adobe', type: 'oauth' }) },
      global: { stubs: { UsageProgressBar: true, AccountQuotaInfo: true } }
    })
    await flushPromises()
    expect(wrapper.get('[data-testid="adobe-credits"]').text()).toContain(expected)
  })

  it('Adobe credits 错误独立展示', async () => {
    getUsage.mockResolvedValue({ adobe_credits: { state: 'error', error: 'IMS unavailable' } })
    const wrapper = mount(AccountUsageCell, {
      props: { account: makeAccount({ id: 9010, platform: 'adobe', type: 'oauth' }) },
      global: { stubs: { UsageProgressBar: true, AccountQuotaInfo: true } }
    })
    await flushPromises()
    expect(wrapper.get('[data-testid="adobe-credits-error"]').text()).toContain('IMS unavailable')
  })

  it('Antigravity 图片用量会聚合新旧 image 模型', async () => {
    getUsage.mockResolvedValue({
      antigravity_quota: {
        'gemini-2.5-flash-image': {
          utilization: 45,
          reset_time: '2026-03-01T11:00:00Z'
        },
        'gemini-3.1-flash-image': {
          utilization: 20,
          reset_time: '2026-03-01T10:00:00Z'
        },
        'gemini-3-pro-image': {
          utilization: 70,
          reset_time: '2026-03-01T09:00:00Z'
        }
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 1001,
          platform: 'antigravity',
          type: 'oauth',
          extra: {}
        })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'color'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ resetsAt }}</div>'
          },
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('admin.accounts.usageWindow.gemini3Image|70|2026-03-01T09:00:00Z')
  })

  it('Antigravity 会显示 AI Credits 余额信息', async () => {
    getUsage.mockResolvedValue({
      ai_credits: [
        {
          credit_type: 'GOOGLE_ONE_AI',
          amount: 25,
          minimum_balance: 5
        }
      ]
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 1002,
          platform: 'antigravity',
          type: 'oauth',
          extra: {}
        })
      },
      global: {
        stubs: {
          UsageProgressBar: true,
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('admin.accounts.aiCreditsBalance')
    expect(wrapper.text()).toContain('25')
  })


  it('OpenAI OAuth 快照已过期时首屏会重新请求 usage', async () => {
    getUsage.mockResolvedValue({
      five_hour: {
        utilization: 15,
        resets_at: '2026-03-08T12:00:00Z',
        remaining_seconds: 3600,
        window_stats: {
          requests: 3,
          tokens: 300,
          cost: 0.03,
          standard_cost: 0.03,
          user_cost: 0.03
        }
      },
      seven_day: {
        utilization: 77,
        resets_at: '2026-03-13T12:00:00Z',
        remaining_seconds: 3600,
        window_stats: {
          requests: 3,
          tokens: 300,
          cost: 0.03,
          standard_cost: 0.03,
          user_cost: 0.03
        }
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 2000,
          platform: 'openai',
          type: 'oauth',
          extra: {
            codex_usage_updated_at: '2026-03-07T00:00:00Z',
            codex_5h_used_percent: 12,
            codex_5h_reset_at: '2026-03-08T12:00:00Z',
            codex_7d_used_percent: 34,
            codex_7d_reset_at: '2026-03-13T12:00:00Z'
          }
        })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'windowStats', 'color'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ windowStats?.tokens }}</div>'
          },
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()

    expect(getUsage).toHaveBeenCalledWith(2000)
    expect(wrapper.text()).toContain('5h|15|300')
    expect(wrapper.text()).toContain('7d|77|300')
  })

  it('OpenAI OAuth 有 codex 快照时仍然使用 /usage API 数据渲染', async () => {
    getUsage.mockResolvedValue({
      five_hour: {
        utilization: 18,
        resets_at: '2099-03-07T12:00:00Z',
        remaining_seconds: 3600,
        window_stats: {
          requests: 9,
          tokens: 900,
          cost: 0.09,
          standard_cost: 0.09,
          user_cost: 0.09
        }
      },
      seven_day: {
        utilization: 36,
        resets_at: '2099-03-13T12:00:00Z',
        remaining_seconds: 3600,
        window_stats: {
          requests: 9,
          tokens: 900,
          cost: 0.09,
          standard_cost: 0.09,
          user_cost: 0.09
        }
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 2001,
          platform: 'openai',
          type: 'oauth',
          extra: {
            codex_usage_updated_at: '2099-03-07T10:00:00Z',
            codex_5h_used_percent: 12,
            codex_5h_reset_at: '2099-03-07T12:00:00Z',
            codex_7d_used_percent: 34,
            codex_7d_reset_at: '2099-03-13T12:00:00Z'
          }
        })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'windowStats', 'color'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ windowStats?.tokens }}</div>'
          },
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()

    expect(getUsage).toHaveBeenCalledWith(2001)
    // 单一数据源：始终使用 /usage API 返回值，忽略 codex 快照
    expect(wrapper.text()).toContain('5h|18|900')
    expect(wrapper.text()).toContain('7d|36|900')
  })

  it('OpenAI OAuth 有现成快照时，手动刷新信号会触发 usage 重拉', async () => {
    getUsage.mockResolvedValue({
      five_hour: {
        utilization: 18,
        resets_at: '2099-03-07T12:00:00Z',
        remaining_seconds: 3600,
        window_stats: {
          requests: 9,
          tokens: 900,
          cost: 0.09,
          standard_cost: 0.09,
          user_cost: 0.09
        }
      },
      seven_day: {
        utilization: 36,
        resets_at: '2099-03-13T12:00:00Z',
        remaining_seconds: 3600,
        window_stats: {
          requests: 9,
          tokens: 900,
          cost: 0.09,
          standard_cost: 0.09,
          user_cost: 0.09
        }
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 2010,
          platform: 'openai',
          type: 'oauth',
          extra: {
            codex_usage_updated_at: '2099-03-07T10:00:00Z',
            codex_5h_used_percent: 12,
            codex_5h_reset_at: '2099-03-07T12:00:00Z',
            codex_7d_used_percent: 34,
            codex_7d_reset_at: '2099-03-13T12:00:00Z'
          },
          rate_limit_reset_at: null
        }),
        manualRefreshToken: 0
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'windowStats', 'color'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ windowStats?.tokens }}</div>'
          },
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()
    // mount 时已经拉取一次
    expect(getUsage).toHaveBeenCalledTimes(1)

    await wrapper.setProps({ manualRefreshToken: 1 })
    await flushPromises()

    // 手动刷新再拉一次
    expect(getUsage).toHaveBeenCalledTimes(2)
    expect(getUsage).toHaveBeenCalledWith(2010)
    // 单一数据源：始终使用 /usage API 值
    expect(wrapper.text()).toContain('5h|18|900')
  })

  it('OpenAI OAuth 在无 codex 快照时会回退显示 usage 接口窗口', async () => {
	getUsage.mockResolvedValue({
	  five_hour: {
	    utilization: 0,
	    resets_at: null,
	    remaining_seconds: 0,
	    window_stats: {
	      requests: 2,
	      tokens: 27700,
	      cost: 0.06,
	      standard_cost: 0.06,
	      user_cost: 0.06
	    }
	  },
	  seven_day: {
	    utilization: 0,
	    resets_at: null,
	    remaining_seconds: 0,
	    window_stats: {
	      requests: 2,
	      tokens: 27700,
	      cost: 0.06,
	      standard_cost: 0.06,
	      user_cost: 0.06
	    }
	  }
	})

		const wrapper = mount(AccountUsageCell, {
		  props: {
		    account: makeAccount({
		      id: 2002,
		      platform: 'openai',
		      type: 'oauth',
		      extra: {}
		    })
		  },
	  global: {
	    stubs: {
	      UsageProgressBar: {
	        props: ['label', 'utilization', 'resetsAt', 'windowStats', 'color'],
	        template: '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ windowStats?.tokens }}</div>'
	      },
	      AccountQuotaInfo: true
	    }
	  }
	})

	await flushPromises()

	expect(getUsage).toHaveBeenCalledWith(2002)
	expect(wrapper.text()).toContain('5h|0|27700')
	expect(wrapper.text()).toContain('7d|0|27700')
  })

  it('OpenAI OAuth 在行数据刷新但仍无 codex 快照时会重新拉取 usage', async () => {
	getUsage
	  .mockResolvedValueOnce({
	    five_hour: {
	      utilization: 0,
	      resets_at: null,
	      remaining_seconds: 0,
	      window_stats: {
	        requests: 1,
	        tokens: 100,
	        cost: 0.01,
	        standard_cost: 0.01,
	        user_cost: 0.01
	      }
	    },
	    seven_day: null
	  })
	  .mockResolvedValueOnce({
	    five_hour: {
	      utilization: 0,
	      resets_at: null,
	      remaining_seconds: 0,
	      window_stats: {
	        requests: 2,
	        tokens: 200,
	        cost: 0.02,
	        standard_cost: 0.02,
	        user_cost: 0.02
	      }
	    },
	    seven_day: null
	  })

		const wrapper = mount(AccountUsageCell, {
		  props: {
		    account: makeAccount({
		      id: 2003,
		      platform: 'openai',
		      type: 'oauth',
		      updated_at: '2026-03-07T10:00:00Z',
		      extra: {}
		    })
		  },
	  global: {
	    stubs: {
	      UsageProgressBar: {
	        props: ['label', 'utilization', 'resetsAt', 'windowStats', 'color'],
	        template: '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ windowStats?.tokens }}</div>'
	      },
	      AccountQuotaInfo: true
	    }
	  }
	})

	await flushPromises()
	expect(wrapper.text()).toContain('5h|0|100')
	expect(getUsage).toHaveBeenCalledTimes(1)

	await wrapper.setProps({
	  account: {
	    id: 2003,
	    platform: 'openai',
	    type: 'oauth',
	    updated_at: '2026-03-07T10:01:00Z',
	    extra: {}
	  }
	})

	await flushPromises()
	expect(getUsage).toHaveBeenCalledTimes(2)
	expect(wrapper.text()).toContain('5h|0|200')
  })

  it('OpenAI OAuth 已限额时显示 /usage API 返回的限额数据', async () => {
	getUsage.mockResolvedValue({
	  five_hour: {
	    utilization: 100,
	    resets_at: '2026-03-07T12:00:00Z',
	    remaining_seconds: 3600,
	    window_stats: {
	      requests: 211,
	      tokens: 106540000,
	      cost: 38.13,
	      standard_cost: 38.13,
	      user_cost: 38.13
	    }
	  },
	  seven_day: {
	    utilization: 100,
	    resets_at: '2026-03-13T12:00:00Z',
	    remaining_seconds: 3600,
	    window_stats: {
	      requests: 211,
	      tokens: 106540000,
	      cost: 38.13,
	      standard_cost: 38.13,
	      user_cost: 38.13
	    }
	  }
	})

		const wrapper = mount(AccountUsageCell, {
		  props: {
		    account: makeAccount({
		      id: 2004,
		      platform: 'openai',
		      type: 'oauth',
		      rate_limit_reset_at: '2099-03-07T12:00:00Z',
		      extra: {
		        codex_5h_used_percent: 0,
		        codex_7d_used_percent: 0
		      }
		    })
		  },
	  global: {
	    stubs: {
	      UsageProgressBar: {
	        props: ['label', 'utilization', 'resetsAt', 'windowStats', 'color'],
	        template: '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ windowStats?.tokens }}</div>'
	      },
	      AccountQuotaInfo: true
	    }
	  }
	})

	await flushPromises()

  expect(getUsage).toHaveBeenCalledWith(2004)
  expect(wrapper.text()).toContain('5h|100|106540000')
  expect(wrapper.text()).toContain('7d|100|106540000')
  })

  it('Cursor API Key 自动加载本地窗口并展示缓存 Token 与额度进度', async () => {
    getUsage.mockResolvedValue({
      source: 'local',
      cursor_api_key_configured: true,
      cursor_probe_state: 'configured',
      cursor_local_usage: {
        requests: 12,
        input_tokens: 10_000,
        output_tokens: 8_000,
        cache_write_tokens: 6_000,
        cache_read_tokens: 10_000,
        tokens: 34_000,
        cost: 1.25,
        standard_cost: 1.25,
        user_cost: 1.5
      }
    })
    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 2953,
          platform: 'cursor',
          type: 'apikey',
          credentials: {},
          credentials_status: { has_api_key: true },
          quota_daily_used: 25,
          quota_daily_limit: 100
        })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'windowStats'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}|CW {{ windowStats?.cache_write_tokens }}|CR {{ windowStats?.cache_read_tokens }}</div>'
          },
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()

    expect(getUsage).toHaveBeenCalledWith(2953)
    expect(wrapper.text()).toContain('1d|25|CW 6000|CR 10000')
    expect(wrapper.text()).toContain('admin.accounts.cursor.apiKeyConfigured')
  })

  it('Cursor 展示官方 Total、First-party 与 API 套餐进度并保留本地用量', async () => {
    getUsage.mockResolvedValue({
      source: 'local',
      cursor_api_key_configured: true,
      cursor_probe_state: 'configured',
      cursor_dashboard_configured: true,
      cursor_dashboard_state: 'cached',
      cursor_plan_usage: {
        enabled: true,
        total_percent_used: 1,
        first_party_percent_used: 0,
        api_percent_used: 1,
        limit_cents: 2000,
        total_spend_cents: 20,
        remaining_cents: 1980,
        billing_cycle_end: '2099-08-10T00:00:00Z',
        updated_at: '2026-08-01T00:00:00Z'
      },
      cursor_local_usage: { requests: 2, tokens: 100, cost: 0.02 }
    })
    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 2955,
          platform: 'cursor',
          type: 'apikey',
          credentials_status: { has_api_key: true, has_dashboard_access_token: true }
        })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt'],
            template: '<div class="official-bar">{{ label }}|{{ utilization }}|{{ resetsAt }}</div>'
          },
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('Total|1|2099-08-10T00:00:00Z')
    expect(wrapper.text()).toContain('admin.accounts.usageWindow.cursorFirstParty|0|')
    expect(wrapper.text()).toContain('admin.accounts.usageWindow.cursorAPI|1|')
    expect(wrapper.text()).toContain('admin.accounts.usageWindow.cursorPlanSpend')
    expect(wrapper.text()).toContain('2 req')
  })

  it('Cursor 刷新按钮与账号列表手动刷新都会强制检测 API Key', async () => {
    getUsage.mockResolvedValue({
      source: 'active',
      cursor_api_key_configured: true,
      cursor_probe_state: 'verified',
      cursor_checked_at: '2026-08-01T00:00:00Z',
      cursor_local_usage: { requests: 1, tokens: 10, cost: 0.01 }
    })
    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 2956,
          platform: 'cursor',
          type: 'apikey',
          credentials_status: { has_api_key: true }
        }),
        manualRefreshToken: 0
      },
      global: { stubs: { UsageProgressBar: true, AccountQuotaInfo: true } }
    })
    await flushPromises()

    await wrapper.get('[data-testid="cursor-usage-refresh"]').trigger('click')
    await flushPromises()
    expect(getUsage).toHaveBeenCalledWith(2956, 'active', true)
    expect(wrapper.text()).toContain('admin.accounts.usageWindow.cursorVerified')

    await wrapper.setProps({ manualRefreshToken: 1 })
    await flushPromises()
    expect(getUsage).toHaveBeenCalledWith(2956, undefined, true)
  })

  it('Kiro OAuth 用量窗口会展示今日请求、Token 与 A/U 双口径计费', async () => {
    getUsage.mockResolvedValue({
      source: 'passive',
      kiro_subscription_type: 'PRO',
      kiro_usage_current: 2964,
      kiro_usage_limit: 5000,
      kiro_usage_percent: 0.5928,
      kiro_next_reset_date: '2026-08-01',
      kiro_context_usage_pct: 27
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 2954,
          platform: 'kiro',
          type: 'oauth',
          extra: {}
        }),
        todayStats: {
          requests: 108,
          tokens: 12_800_000,
          cost: 16.07,
          standard_cost: 16.07,
          user_cost: 12.86
        }
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'color'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ resetsAt }}</div>'
          },
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()

    expect(getUsage).toHaveBeenCalledWith(2954)
    expect(wrapper.get('[data-testid="kiro-today-stats"]').attributes('title'))
      .toBe('admin.accounts.usageWindow.kiroTodayStats')
    expect(wrapper.text()).toContain('108 req')
    expect(wrapper.text()).toContain('12.8M')
    expect(wrapper.text()).toContain('A $16.07')
    expect(wrapper.text()).toContain('U $12.86')

    const badges = wrapper.findAll('span[title]')
    expect(badges.some(node => node.attributes('title') === 'usage.accountBilled')).toBe(true)
    expect(badges.some(node => node.attributes('title') === 'usage.userBilled')).toBe(true)
  })

  it('Kiro OAuth 在 user_cost 缺失时隐藏用户扣费，统计加载中显示骨架', async () => {
    getUsage.mockResolvedValue({
      kiro_subscription_type: 'PRO',
      kiro_usage_percent: 0.2
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 2955,
          platform: 'kiro',
          type: 'oauth',
          extra: {}
        }),
        todayStats: {
          requests: 2,
          tokens: 800,
          cost: 0.25,
          standard_cost: 0.25
        }
      },
      global: {
        stubs: {
          UsageProgressBar: true,
          AccountQuotaInfo: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('A $0.25')
    expect(wrapper.findAll('span[title="usage.userBilled"]')).toHaveLength(0)

    await wrapper.setProps({ todayStats: null, todayStatsLoading: true })
    expect(wrapper.find('[data-testid="kiro-today-stats-loading"]').exists()).toBe(true)
  })

  it('Key 账号会展示 today stats 徽章并带 A/U 提示', async () => {
		const wrapper = mount(AccountUsageCell, {
		  props: {
		    account: makeAccount({
		      id: 3001,
		      platform: 'anthropic',
		      type: 'apikey'
		    }),
		    todayStats: {
		      requests: 1_000_000,
		      tokens: 1_000_000_000,
		      cost: 12.345,
		      standard_cost: 12.345,
		      user_cost: 6.789
		    }
		  },
		  global: {
		    stubs: {
		      UsageProgressBar: true,
		      AccountQuotaInfo: true
		    }
		  }
		})

		await flushPromises()

		expect(wrapper.text()).toContain('1.0M req')
		expect(wrapper.text()).toContain('1.0B')
		expect(wrapper.text()).toContain('A $12.35')
		expect(wrapper.text()).toContain('U $6.79')

		const badges = wrapper.findAll('span[title]')
		expect(badges.some(node => node.attributes('title') === 'usage.accountBilled')).toBe(true)
		expect(badges.some(node => node.attributes('title') === 'usage.userBilled')).toBe(true)
  })

  it('Grok OAuth 会展示本地 user billed 用量并把耗尽配额显示为 0% 剩余', async () => {
    getUsage.mockResolvedValue({
      grok_local_usage: {
        requests: 4,
        tokens: 1200,
        cost: 0.12,
        standard_cost: 0.12,
        user_cost: 0.34
      },
      grok_request_quota: {
        limit: 10,
        remaining: -2,
        reset_at: '2026-07-09T16:00:00Z'
      },
      grok_quota_snapshot_state: 'observed'
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 3861,
          platform: 'grok',
          type: 'oauth',
          extra: {}
        })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'color'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ resetsAt }}</div>'
          },
          AccountQuotaInfo: true,
          GrokQuotaProbeCell: true
        }
      }
    })

    await flushPromises()

    expect(getUsage).toHaveBeenCalledWith(3861)
    expect(wrapper.text()).toContain('4 req')
    expect(wrapper.text()).toContain('1.2K')
    expect(wrapper.text()).toContain('A $0.12')
    expect(wrapper.text()).toContain('U $0.34')
    expect(wrapper.text()).toContain('admin.accounts.usageWindow.grokRequests|0|2026-07-09T16:00:00Z')

    const badges = wrapper.findAll('span[title]')
    expect(badges.some(node => node.attributes('title') === 'usage.accountBilled')).toBe(true)
    expect(badges.some(node => node.attributes('title') === 'usage.userBilled')).toBe(true)
  })

  it('Grok OAuth 配额条按剩余容量显示 100% 满格和 25% 低量', async () => {
    getUsage.mockResolvedValue({
      grok_request_quota: {
        limit: 100,
        remaining: 100,
        reset_at: '2026-07-09T16:00:00Z'
      },
      grok_token_quota: {
        limit: 1000,
        remaining: 250,
        reset_at: '2026-07-09T16:00:00Z'
      },
      grok_quota_snapshot_state: 'observed'
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 4073,
          platform: 'grok',
          type: 'oauth',
          extra: {}
        })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'color', 'remainingCapacity'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ remainingCapacity }}</div>'
          },
          AccountQuotaInfo: true,
          GrokQuotaProbeCell: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('admin.accounts.usageWindow.grokRequests|100|true')
    expect(wrapper.text()).toContain('admin.accounts.usageWindow.grokTokens|25|true')
  })

  it('Grok OAuth uses the official weekly billing percentage when available', async () => {
    getUsage.mockResolvedValue({
      grok_billing: {
        period_type: 'weekly',
        usage_percent: 37,
        period_end: '2026-07-16T03:25:00Z',
        plan: 'SuperGrok'
      },
      grok_local_usage: {
        requests: 5,
        tokens: 2_200_000,
        cost: 4.42,
        standard_cost: 4.42,
        user_cost: 0.44
      },
      grok_request_quota: { limit: 100, remaining: 100 },
      grok_token_quota: { limit: 2_000_000, remaining: 2_000_000 }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4201, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'remainingCapacity'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ resetsAt }}|{{ remainingCapacity }}</div>'
          },
          AccountQuotaInfo: true,
          GrokQuotaProbeCell: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('7d|37|2026-07-16T03:25:00Z')
    expect(wrapper.text()).not.toContain('admin.accounts.usageWindow.grokRequests|')
    expect(wrapper.text()).not.toContain('admin.accounts.usageWindow.grokTokens|')
    expect(wrapper.text()).not.toContain('2M|')
  })

  it.each([
    { tokens: 0, expected: 0, compact: '0' },
    { tokens: 500_000, expected: 50, compact: '500.0K' },
    { tokens: 1_000_000, expected: 100, compact: '1.0M' },
    { tokens: 1_100_000, expected: 100, compact: '1.1M' }
  ])('Grok Free derives its 1M quota from local tokens: $tokens -> $expected%', async ({ tokens, expected, compact }) => {
    getUsage.mockResolvedValue({
      grok_free_token_limit: 1_000_000,
      grok_billing: {
        period_type: 'weekly',
        usage_percent: null,
        plan: ''
      },
      grok_local_usage_24h: {
        requests: 5,
        tokens,
        cost: 0,
        standard_cost: 0,
        user_cost: 0
      },
      grok_request_quota: { limit: 100, remaining: 100 },
      grok_token_quota: { limit: 1_000_000, remaining: 1_000_000 }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4300 + expected, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}</div>'
          },
          AccountQuotaInfo: true,
          GrokQuotaProbeCell: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain(`24h|${expected}`)
    expect(wrapper.findAll('span').filter((node) => node.text() === compact)).toHaveLength(1)
    expect(wrapper.findAll('.usage-bar')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('admin.accounts.usageWindow.grokRequests|')
    expect(wrapper.text()).not.toContain('admin.accounts.usageWindow.grokTokens|')
  })

  it('Grok Free uses rolling 24h usage instead of today-only usage', async () => {
    getUsage.mockResolvedValue({
      grok_free_token_limit: 1_000_000,
      grok_billing: { period_type: 'weekly', usage_percent: null, plan: '' },
      grok_local_usage: {
        requests: 2,
        tokens: 250_000,
        cost: 0,
        standard_cost: 0
      },
      grok_local_usage_24h: {
        requests: 12,
        tokens: 750_000,
        cost: 0,
        standard_cost: 0
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4398, platform: 'grok', type: 'oauth', extra: {} }),
        todayStats: {
          requests: 2,
          tokens: 200_000,
          cost: 0,
          standard_cost: 0
        }
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'title'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ title }}</div>'
          },
          AccountQuotaInfo: true,
          GrokQuotaProbeCell: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('24h|75|admin.accounts.usageWindow.grokFreeQuota24hHint')
    expect(wrapper.text()).toContain('750.0K')
    expect(wrapper.text()).not.toContain('7d|')
    expect(wrapper.text()).not.toContain('200.0K')
    expect(wrapper.text()).not.toContain('250.0K')
  })

  it('Grok Free does not substitute today stats when rolling 24h usage is unavailable', async () => {
    getUsage.mockResolvedValue({
      grok_free_token_limit: 1_000_000,
      grok_billing: { period_type: 'weekly', usage_percent: null, plan: '' },
      grok_local_usage: {
        requests: 1,
        tokens: 250_000,
        cost: 0,
        standard_cost: 0,
        user_cost: 0
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4399, platform: 'grok', type: 'oauth', extra: {} }),
        todayStats: {
          requests: 4,
          tokens: 1_000_000,
          cost: 0,
          standard_cost: 0,
          user_cost: 0
        }
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}</div>'
          },
          AccountQuotaInfo: true,
          GrokQuotaProbeCell: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.findAll('.usage-bar')).toHaveLength(0)
    expect(wrapper.text()).not.toContain('24h|')
    expect(wrapper.text()).not.toContain('1.0M')
    expect(wrapper.text()).not.toContain('250.0K')
  })

  it('Grok paid plans are not mistaken for Free when weekly usage is temporarily missing', async () => {
    getUsage.mockResolvedValue({
      grok_billing: {
        period_type: 'weekly',
        usage_percent: null,
        plan: 'SuperGrok Heavy'
      },
      grok_entitlement_status: 'free',
      grok_local_usage: {
        requests: 2,
        tokens: 2_000_000,
        cost: 1,
        standard_cost: 1
      },
      grok_token_quota: { limit: 1_000, remaining: 250 }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4401, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}</div>'
          },
          AccountQuotaInfo: true,
          GrokQuotaProbeCell: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('admin.accounts.usageWindow.grokTokens|25')
    expect(wrapper.text()).not.toContain('2M|')
  })

  it('Grok custom paid monthly limits override stale Free entitlement', async () => {
    getUsage.mockResolvedValue({
      grok_billing: {
        period_type: 'weekly',
        usage_percent: null,
        monthly_limit_cents: 25_000,
        plan: ''
      },
      grok_entitlement_status: 'free',
      grok_local_usage: {
        requests: 2,
        tokens: 2_000_000,
        cost: 1,
        standard_cost: 1
      },
      grok_token_quota: { limit: 1_000, remaining: 250 }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4402, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}</div>'
          },
          AccountQuotaInfo: true,
          GrokQuotaProbeCell: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('admin.accounts.usageWindow.grokTokens|25')
    expect(wrapper.text()).not.toContain('2M|')
  })

  it('Grok credential Free tier keeps the 1M fallback when billing is unavailable', async () => {
    getUsage.mockResolvedValue({
      grok_free_token_limit: 1_000_000,
      subscription_tier: 'FREE',
      grok_local_usage_24h: {
        requests: 3,
        tokens: 1_000_000,
        cost: 0,
        standard_cost: 0
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4403, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}</div>'
          },
          AccountQuotaInfo: true,
          GrokQuotaProbeCell: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('24h|100')
  })

  it('Grok paid manual probes keep the weekly/local summary when 24h usage is returned', async () => {
    getUsage.mockResolvedValue({
      grok_quota_snapshot_state: 'no_headers',
      error: 'stale error',
      error_code: 'quota_unknown'
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4501, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ resetsAt }}</div>'
          },
          AccountQuotaInfo: true,
          GrokQuotaProbeCell: {
            emits: ['probed'],
            template: `<button class="probe" @click="$emit('probed', {
              source: 'hybrid_probe',
              billing: { period_type: 'weekly', usage_percent: 42, period_end: '2026-07-17T00:00:00Z' },
              snapshot: {
                headers_observed: true,
                updated_at: '2026-07-13T00:00:00Z',
                entitlement_status: 'ACTIVE',
                requests: { limit: 100, remaining: 20 }
              },
              local_usage_24h: { requests: 3, tokens: 750000, cost: 0.75, standard_cost: 0.75, user_cost: 0.25 },
              local_usage_7d: { requests: 4, tokens: 1000000, cost: 1, standard_cost: 1, user_cost: 0.5 },
              local_usage_monthly: { requests: 7, tokens: 1500000, cost: 2, standard_cost: 2, user_cost: 1 },
              status_code: 200,
              headers_observed: true,
              reset_supported: false,
              fetched_at: 1
            })">probe</button>`
          }
        }
      }
    })

    await flushPromises()
    await wrapper.get('.probe').trigger('click')

    expect(wrapper.text()).toContain('7d|42|2026-07-17T00:00:00Z')
    expect(wrapper.text()).toContain('1.0M')
    expect(wrapper.text()).not.toContain('750.0K')
    expect(wrapper.text()).toContain('ACTIVE')
    expect(wrapper.text()).not.toContain('stale error')
  })

  it('Grok successful probes immediately clear stale forbidden state', async () => {
    getUsage.mockResolvedValue({
      is_forbidden: true,
      forbidden_reason: 'stale forbidden response',
      forbidden_type: 'validation',
      validation_url: 'https://example.com/verify',
      needs_verify: true,
      is_banned: true,
      grok_entitlement_status: 'forbidden',
      grok_quota_snapshot_state: 'no_headers',
      error: 'stale forbidden response',
      error_code: 'forbidden'
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4503, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: true,
          AccountQuotaInfo: true,
          GrokQuotaProbeCell: true
        }
      }
    })

    await flushPromises()
    expect(wrapper.text()).toContain('forbidden')

    const setupState = wrapper.vm.$.setupState as {
      handleGrokProbed: (result: Record<string, unknown>) => void
      usageInfo: Record<string, unknown> | null
    }
    setupState.handleGrokProbed({
      source: 'active_probe',
      snapshot: {
        headers_observed: false,
        updated_at: '2026-07-18T00:00:00Z',
        status_code: 200
      },
      status_code: 200,
      headers_observed: false,
      reset_supported: false,
      fetched_at: 1
    })
    await wrapper.vm.$nextTick()

    expect(setupState.usageInfo).toMatchObject({
      is_forbidden: false,
      needs_verify: false,
      is_banned: false,
      grok_last_status_code: 200
    })
    expect(setupState.usageInfo?.forbidden_reason).toBeUndefined()
    expect(setupState.usageInfo?.forbidden_type).toBeUndefined()
    expect(setupState.usageInfo?.validation_url).toBeUndefined()
    expect(setupState.usageInfo?.grok_entitlement_status).toBeUndefined()
    expect(wrapper.text()).not.toContain('admin.accounts.forbidden')
  })

  it('Grok successful probes preserve the entitlement reported by the latest snapshot', async () => {
    getUsage.mockResolvedValue({
      is_forbidden: true,
      grok_entitlement_status: 'forbidden'
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4504, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: true,
          AccountQuotaInfo: true,
          GrokQuotaProbeCell: true
        }
      }
    })

    await flushPromises()

    const setupState = wrapper.vm.$.setupState as {
      handleGrokProbed: (result: Record<string, unknown>) => void
      usageInfo: Record<string, unknown> | null
    }
    setupState.handleGrokProbed({
      source: 'active_probe',
      snapshot: {
        headers_observed: true,
        updated_at: '2026-07-18T00:00:00Z',
        entitlement_status: 'ACTIVE',
        status_code: 200
      },
      status_code: 200,
      headers_observed: true,
      reset_supported: false,
      fetched_at: 1
    })
    await wrapper.vm.$nextTick()

    expect(setupState.usageInfo?.grok_entitlement_status).toBe('ACTIVE')
    expect(wrapper.text()).toContain('ACTIVE')
    expect(wrapper.text()).not.toContain('admin.accounts.forbidden')
  })

  it('Grok billing-only success does not clear an active-probe forbidden state', async () => {
    getUsage.mockResolvedValue({
      is_forbidden: true,
      forbidden_type: 'forbidden',
      needs_verify: true,
      is_banned: true,
      grok_entitlement_status: 'forbidden'
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4505, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: true,
          AccountQuotaInfo: true,
          GrokQuotaProbeCell: true
        }
      }
    })

    await flushPromises()

    const setupState = wrapper.vm.$.setupState as {
      handleGrokProbed: (result: Record<string, unknown>) => void
      usageInfo: Record<string, unknown> | null
    }
    setupState.handleGrokProbed({
      source: 'billing_probe',
      billing: {
        period_type: 'weekly',
        usage_percent: 10,
        plan: 'SuperGrok'
      },
      status_code: 200,
      headers_observed: false,
      reset_supported: false,
      fetched_at: 1
    })
    await wrapper.vm.$nextTick()

    expect(setupState.usageInfo).toMatchObject({
      is_forbidden: true,
      forbidden_type: 'forbidden',
      needs_verify: true,
      is_banned: true,
      grok_entitlement_status: 'forbidden'
    })
    expect(wrapper.text()).toContain('forbidden')
  })

  it('Grok Free manual probes merge rolling 24h usage', async () => {
    getUsage.mockResolvedValue({
      grok_free_token_limit: 1_000_000,
      subscription_tier: 'FREE',
      grok_quota_snapshot_state: 'no_headers'
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 4502, platform: 'grok', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}</div>'
          },
          AccountQuotaInfo: true,
          GrokQuotaProbeCell: {
            emits: ['probed'],
            template: `<button class="probe" @click="$emit('probed', {
              source: 'hybrid_probe',
              billing: { period_type: 'weekly', usage_percent: null, plan: '' },
              local_usage_24h: { requests: 12, tokens: 750000, cost: 0, standard_cost: 0 },
              headers_observed: false,
              reset_supported: false,
              fetched_at: 1
            })">probe</button>`
          }
        }
      }
    })

    await flushPromises()
    await wrapper.get('.probe').trigger('click')

    expect(wrapper.text()).toContain('24h|75')
    expect(wrapper.text()).toContain('750.0K')
    expect(wrapper.text()).not.toContain('7d|')
  })

  it('Key 账号在 today stats loading 时显示骨架屏', async () => {
		const wrapper = mount(AccountUsageCell, {
		  props: {
		    account: makeAccount({
		      id: 3002,
		      platform: 'anthropic',
		      type: 'apikey'
		    }),
		    todayStats: null,
		    todayStatsLoading: true
		  },
		  global: {
		    stubs: {
		      UsageProgressBar: true,
		      AccountQuotaInfo: true
		    }
		  }
		})

		await flushPromises()

		expect(wrapper.findAll('.animate-pulse').length).toBeGreaterThan(0)
  })

  it('Key 账号在无 today stats 且无配额时显示兜底短横线', async () => {
		const wrapper = mount(AccountUsageCell, {
		  props: {
		    account: makeAccount({
		      id: 3003,
		      platform: 'anthropic',
		      type: 'apikey',
		      quota_limit: 0,
		      quota_daily_limit: 0,
		      quota_weekly_limit: 0
		    }),
		    todayStats: null,
		    todayStatsLoading: false
		  },
		  global: {
		    stubs: {
		      UsageProgressBar: true,
		      AccountQuotaInfo: true
		    }
		  }
		})

		await flushPromises()

		expect(wrapper.text().trim()).toBe('-')
  })

  it('Vertex 账号会在 Gemini 用量窗口里展示 today stats 徽章', async () => {
		const wrapper = mount(AccountUsageCell, {
		  props: {
		    account: makeAccount({
		      id: 4001,
		      platform: 'gemini',
		      type: 'service_account',
          credentials: {
            tier_id: 'vertex',
            project_id: 'vertex-proj',
            client_email: 'svc@vertex-proj.iam.gserviceaccount.com',
            location: 'global'
          },
		      extra: {}
		    }),
		    todayStats: {
		      requests: 0,
		      tokens: 0,
		      cost: 0,
		      standard_cost: 0,
		      user_cost: 0
		    }
		  },
		  global: {
		    stubs: {
		      UsageProgressBar: true,
		      AccountQuotaInfo: true
		    }
		  }
		})

		await flushPromises()

		expect(wrapper.text()).toContain('0 req')
		expect(wrapper.text()).toContain('0')
		expect(wrapper.text()).toContain('A $0.00')
		expect(wrapper.text()).toContain('U $0.00')
  })

  it('Anthropic OAuth 会渲染 7d F (Fable) 进度条，且 7d S 逻辑保留', async () => {
    getUsage.mockResolvedValue({
      source: 'passive',
      five_hour: {
        utilization: 41,
        resets_at: '2026-07-03T10:00:00Z',
        remaining_seconds: 3600
      },
      seven_day: {
        utilization: 56,
        resets_at: '2026-07-06T22:00:00Z',
        remaining_seconds: 300000
      },
      seven_day_sonnet: {
        utilization: 30,
        resets_at: '2026-07-06T22:00:00Z',
        remaining_seconds: 300000
      },
      seven_day_fable: {
        utilization: 100,
        resets_at: '2026-07-06T22:00:00Z',
        remaining_seconds: 300000
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 3001,
          platform: 'anthropic',
          type: 'oauth',
          extra: {}
        })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'color'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}</div>'
          },
          AccountQuotaInfo: true,
          GrokQuotaProbeCell: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('5h|41')
    expect(wrapper.text()).toContain('7d|56')
    expect(wrapper.text()).toContain('7d S|30')
    expect(wrapper.text()).toContain('7d F|100')
  })

  it('Anthropic OAuth 无 Fable 数据时不渲染 7d F 进度条', async () => {
    getUsage.mockResolvedValue({
      source: 'passive',
      five_hour: {
        utilization: 41,
        resets_at: '2026-07-03T10:00:00Z',
        remaining_seconds: 3600
      },
      seven_day: {
        utilization: 56,
        resets_at: '2026-07-06T22:00:00Z',
        remaining_seconds: 300000
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 3002,
          platform: 'anthropic',
          type: 'oauth',
          extra: {}
        })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'color'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}</div>'
          },
          AccountQuotaInfo: true,
          GrokQuotaProbeCell: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('5h|41')
    expect(wrapper.text()).toContain('7d|56')
    expect(wrapper.text()).not.toContain('7d S')
    expect(wrapper.text()).not.toContain('7d F')
  })

  it.each([
    { type: 'apikey' as const, id: 5101 },
    { type: 'bedrock' as const, id: 5102 }
  ])('池模式 $type 账号会自动查询容量并在手动刷新时强制更新', async ({ type, id }) => {
    getUsage.mockResolvedValue({
      capacity: {
        mode: 'upstream_balance',
        state: 'verified',
        provider: 'sub2api',
        authoritative: true,
        remaining: 42.5,
        total: 100,
        used: 57.5,
        unit: 'USD',
        estimated_remaining_requests: 85,
        average_cost_per_request: 0.5,
        sample_requests: 20,
        fetched_at: '2026-07-21T08:00:00Z'
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id,
          platform: 'anthropic',
          type,
          credentials: { pool_mode: true }
        }),
        manualRefreshToken: 0,
        todayStats: {
          requests: 3,
          tokens: 1200,
          cost: 1.25,
          standard_cost: 1.25,
          user_cost: 1
        }
      },
      global: { stubs: { UsageProgressBar: true, AccountQuotaInfo: true } }
    })

    await flushPromises()

    expect(getUsage).toHaveBeenCalledWith(id)
    expect(wrapper.get('[data-testid="capacity-state"]').text())
      .toBe('admin.accounts.usageWindow.capacity.verifiedUpstream')
    expect(wrapper.get('[data-testid="capacity-remaining"]').text()).toContain('$42.5')
    expect(wrapper.find('[data-testid="capacity-estimated-requests"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('3 req')

    await wrapper.setProps({ manualRefreshToken: 1 })
    await flushPromises()
    expect(getUsage).toHaveBeenLastCalledWith(id, undefined, true)
  })

  it.each([
    {
      id: 5201,
      state: 'stale' as const,
      expected: 'admin.accounts.usageWindow.capacity.staleSnapshot'
    },
    {
      id: 5202,
      state: 'estimated' as const,
      expected: 'admin.accounts.usageWindow.capacity.estimatedWindow'
    }
  ])('非池官方窗口会保留窗口展示并明确标注 $state capacity', async ({ id, state, expected }) => {
    getUsage.mockResolvedValue({
      five_hour: {
        utilization: 25,
        resets_at: '2026-07-21T12:00:00Z',
        remaining_seconds: 3600
      },
      capacity: {
        mode: 'usage_window',
        state,
        authoritative: false,
        remaining: 75,
        total: 100,
        used: 25,
        unit: '%',
        estimated_remaining_requests: 30,
        sample_requests: 10,
        fetched_at: '2026-07-21T08:00:00Z'
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id, platform: 'openai', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization'],
            template: '<div class="usage-bar">{{ label }}|{{ utilization }}</div>'
          },
          AccountQuotaInfo: true,
          OpenAIQuotaResetCell: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('5h|25')
    expect(wrapper.get('[data-testid="capacity-state"]').text()).toBe(expected)
    expect(wrapper.get('[data-testid="capacity-remaining"]').text()).toContain('75 %')
  })

  it('非池 API Key 配置本地额度时自动查询并展示估算容量', async () => {
    getUsage.mockResolvedValue({
      source: 'local',
      capacity: {
        mode: 'local_quota',
        state: 'estimated',
        provider: 'local',
        authoritative: false,
        remaining: 8,
        total: 10,
        used: 2,
        unit: 'USD',
        estimated_remaining_requests: 16,
        average_cost_per_request: 0.5,
        sample_requests: 4
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({
          id: 5250,
          platform: 'openai',
          type: 'apikey',
          quota_limit: 10,
          quota_used: 2,
          extra: {}
        })
      },
      global: { stubs: { UsageProgressBar: true, AccountQuotaInfo: true } }
    })

    await flushPromises()

    expect(getUsage).toHaveBeenCalledWith(5250)
    expect(wrapper.get('[data-testid="capacity-state"]').text())
      .toBe('admin.accounts.usageWindow.capacity.estimatedWindow')
    expect(wrapper.get('[data-testid="capacity-remaining"]').text()).toContain('$8')
    expect(wrapper.find('[data-testid="capacity-estimated-requests"]').exists()).toBe(true)
  })

  it('unknown capacity 保持未知状态且不渲染为 0', async () => {
    getUsage.mockResolvedValue({
      capacity: {
        mode: 'upstream_balance',
        state: 'unknown',
        authoritative: false,
        remaining: null,
        total: null,
        used: null,
        estimated_remaining_requests: null
      }
    })

    const wrapper = mount(AccountUsageCell, {
      props: {
        account: makeAccount({ id: 5301, platform: 'openai', type: 'oauth', extra: {} })
      },
      global: {
        stubs: {
          UsageProgressBar: true,
          AccountQuotaInfo: true,
          OpenAIQuotaResetCell: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.get('[data-testid="capacity-state"]').text())
      .toBe('admin.accounts.usageWindow.capacity.unknown')
    expect(wrapper.find('[data-testid="capacity-metrics"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Remaining 0')
    expect(wrapper.text()).not.toContain('剩余 0')
  })
})

import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import UsageStatsCards from '../UsageStatsCards.vue'

const messages: Record<string, string> = {
  'usage.totalRequests': 'Total Requests',
  'usage.inSelectedRange': 'in selected range',
  'usage.totalTokens': 'Total Tokens',
  'usage.in': 'In',
  'usage.out': 'Out',
  'usage.cacheHit': 'Cache Hit',
  'usage.cacheCreate': 'Cache Create',
  'usage.cacheHitRate': 'Cache Hit Rate',
  'usage.totalCost': 'Total Cost',
  'usage.accountCost': 'Cost',
  'usage.standardCost': 'Standard',
  'usage.avgDuration': 'Avg Duration',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

const stats = {
  total_requests: 1,
  total_input_tokens: 100,
  total_output_tokens: 50,
  total_cache_tokens: 34,
  total_cache_creation_tokens: 12,
  total_cache_read_tokens: 22,
  total_tokens: 184,
  total_cost: 0.001,
  total_actual_cost: 0.001,
  total_account_cost: 0.001,
  average_duration_ms: 250,
}

describe('UsageStatsCards', () => {
  it('renders token categories and cache hit rate directly in the card', () => {
    const wrapper = mount(UsageStatsCards, {
      props: {
        stats,
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    const breakdown = wrapper.get('[data-testid="token-breakdown"]')
    expect(breakdown.text()).toContain('In')
    expect(breakdown.text()).toContain('100')
    expect(breakdown.text()).toContain('Out')
    expect(breakdown.text()).toContain('50')
    expect(breakdown.text()).toContain('Cache Hit')
    expect(breakdown.text()).toContain('22')
    expect(breakdown.text()).toContain('Cache Create')
    expect(breakdown.text()).toContain('12')

    const hitRate = wrapper.get('[data-testid="cache-hit-rate"]')
    expect(hitRate.text()).toContain('Cache Hit Rate')
    expect(hitRate.text()).toContain('22 / 134')
    expect(hitRate.text()).toContain('16.4%')
  })

  it('hides cache hit rate when no prompt tokens are available', () => {
    const wrapper = mount(UsageStatsCards, {
      props: {
        stats: {
          ...stats,
          total_input_tokens: 0,
          total_cache_tokens: 0,
          total_cache_creation_tokens: 0,
          total_cache_read_tokens: 0,
        },
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    expect(wrapper.find('[data-testid="cache-hit-rate"]').exists()).toBe(false)
  })
})

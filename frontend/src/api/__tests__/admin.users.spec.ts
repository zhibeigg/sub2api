import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    get,
    post,
    put,
  },
}))

import {
  batchUpdateLimits,
  bindUserAuthIdentity,
  getGroupConfig,
  updateGroupConfig,
  type AdminBindAuthIdentityRequest,
  type AdminBoundAuthIdentity,
  type BatchUpdateUserLimitsRequest,
  type BatchUpdateUserLimitsResponse,
} from '@/api/admin/users'
import type { UpdateUserGroupConfigRequest, UserGroupConfig } from '@/types'

type Assert<T extends true> = T
type IsExact<T, U> = (
  (<G>() => G extends T ? 1 : 2) extends (<G>() => G extends U ? 1 : 2)
    ? ((<G>() => G extends U ? 1 : 2) extends (<G>() => G extends T ? 1 : 2) ? true : false)
    : false
)

type ExpectedAdminBindAuthIdentityRequest = {
  provider_type: string
  provider_key: string
  provider_subject: string
  issuer?: string
  metadata?: Record<string, unknown>
  channel?: {
    channel: string
    channel_app_id: string
    channel_subject: string
    metadata?: Record<string, unknown>
  }
}

type ExpectedAdminBoundAuthIdentity = {
  user_id: number
  provider_type: string
  provider_key: string
  provider_subject: string
  verified_at?: string | null
  issuer?: string | null
  metadata: Record<string, unknown> | null
  created_at: string
  updated_at: string
  channel?: {
    channel: string
    channel_app_id: string
    channel_subject: string
    metadata: Record<string, unknown> | null
    created_at: string
    updated_at: string
  } | null
}

const requestContractExact: Assert<
  IsExact<AdminBindAuthIdentityRequest, ExpectedAdminBindAuthIdentityRequest>
> = true
const responseContractExact: Assert<
  IsExact<AdminBoundAuthIdentity, ExpectedAdminBoundAuthIdentity>
> = true
const batchRequestContractExact: Assert<
  IsExact<
    BatchUpdateUserLimitsRequest,
    {
      user_ids: number[]
      all?: boolean
      concurrency?: number
      rpm_limit?: number
    }
  >
> = true
const batchResponseContractExact: Assert<
  IsExact<BatchUpdateUserLimitsResponse, { affected: number }>
> = true
const groupConfigContractExact: Assert<
  IsExact<UserGroupConfig, {
    access_mode: 'inherit' | 'restricted'
    restricted_group_ids: number[]
    exclusive_group_ids: number[]
    group_rates: Record<number, number>
  }>
> = true
const updateGroupConfigContractExact: Assert<
  IsExact<UpdateUserGroupConfigRequest, {
    access_mode: 'inherit' | 'restricted'
    restricted_group_ids: number[]
    exclusive_group_ids: number[]
    group_rates: Record<number, number | null>
  }>
> = true

describe('admin users api auth identity binding', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
  })

  it('posts the backend-compatible auth identity bind payload and returns the backend response shape', async () => {
    const payload: AdminBindAuthIdentityRequest = {
      provider_type: 'wechat',
      provider_key: 'wechat-main',
      provider_subject: 'union-123',
      metadata: { source: 'admin-repair' },
      channel: {
        channel: 'open',
        channel_app_id: 'wx-open',
        channel_subject: 'openid-123',
        metadata: { scene: 'migration' },
      },
    }

    const response: AdminBoundAuthIdentity = {
      user_id: 9,
      provider_type: 'wechat',
      provider_key: 'wechat-main',
      provider_subject: 'union-123',
      verified_at: '2026-04-22T00:00:00Z',
      issuer: null,
      metadata: { source: 'admin-repair' },
      created_at: '2026-04-22T00:00:00Z',
      updated_at: '2026-04-22T00:00:00Z',
      channel: {
        channel: 'open',
        channel_app_id: 'wx-open',
        channel_subject: 'openid-123',
        metadata: { scene: 'migration' },
        created_at: '2026-04-22T00:00:00Z',
        updated_at: '2026-04-22T00:00:00Z',
      },
    }
    post.mockResolvedValue({ data: response })

    const result = await bindUserAuthIdentity(9, payload)

    expect(post).toHaveBeenCalledWith('/admin/users/9/auth-identities', payload)
    expect(result).toEqual(response)
  })

  it('keeps bind auth identity request and response types aligned with the backend contract', () => {
    expect(requestContractExact).toBe(true)
    expect(responseContractExact).toBe(true)
  })

  it('posts batch limit updates once with only the supplied limit fields', async () => {
    const request: BatchUpdateUserLimitsRequest = {
      user_ids: [4, 7],
      all: false,
      rpm_limit: 0,
    }
    post.mockResolvedValue({ data: { affected: 2 } satisfies BatchUpdateUserLimitsResponse })

    const result = await batchUpdateLimits(request)

    expect(post).toHaveBeenCalledWith('/admin/users/batch-limits', request)
    expect(result).toEqual({ affected: 2 })
    expect(batchRequestContractExact).toBe(true)
    expect(batchResponseContractExact).toBe(true)
  })

  it('gets the dedicated user group configuration endpoint', async () => {
    const response: UserGroupConfig = {
      access_mode: 'inherit',
      restricted_group_ids: [],
      exclusive_group_ids: [8],
      group_rates: { 2: 1.25 },
    }
    get.mockResolvedValue({ data: response })

    await expect(getGroupConfig(19)).resolves.toEqual(response)

    expect(get).toHaveBeenCalledWith('/admin/users/19/group-config')
    expect(groupConfigContractExact).toBe(true)
  })

  it('puts the complete group configuration payload including nullable rate clears', async () => {
    const request: UpdateUserGroupConfigRequest = {
      access_mode: 'restricted',
      restricted_group_ids: [2],
      exclusive_group_ids: [8],
      group_rates: { 2: 1.5, 8: null },
    }
    put.mockResolvedValue({
      data: {
        access_mode: 'restricted',
        restricted_group_ids: [2],
        exclusive_group_ids: [8],
        group_rates: { 2: 1.5 },
      } satisfies UserGroupConfig,
    })

    await updateGroupConfig(19, request)

    expect(put).toHaveBeenCalledWith('/admin/users/19/group-config', request)
    expect(updateGroupConfigContractExact).toBe(true)
  })
})

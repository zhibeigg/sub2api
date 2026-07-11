/**
 * Admin API Keys API endpoints
 * Handles API key management for administrators
 */

import { apiClient } from '../client'
import type {
  AdminUpdateApiKeyGroupBindingsRequest,
  ApiKey,
  ApiKeyGroupBindingInput
} from '@/types'

export interface UpdateApiKeyGroupResult {
  api_key: ApiKey
  auto_granted_group_access: boolean
  granted_group_id?: number
  granted_group_name?: string
}

/**
 * Update an API key's group binding
 * @param id - API Key ID
 * @param groupId - Group ID (0 to unbind, positive to bind, null/undefined to skip)
 * @returns Updated API key with auto-grant info
 */
export async function updateApiKeyGroup(id: number, groupId: number | null): Promise<UpdateApiKeyGroupResult> {
  const { data } = await apiClient.put<UpdateApiKeyGroupResult>(`/admin/api-keys/${id}`, {
    group_id: groupId === null ? 0 : groupId
  })
  return data
}

/** Update the complete ordered group binding list without overwriting it via group_id. */
export async function updateApiKeyGroupBindings(
  id: number,
  groupBindings: ApiKeyGroupBindingInput[]
): Promise<UpdateApiKeyGroupResult> {
  const payload: AdminUpdateApiKeyGroupBindingsRequest = {
    group_bindings: groupBindings.map((binding, priority) => ({
      group_id: binding.group_id,
      priority
    }))
  }
  const { data } = await apiClient.put<UpdateApiKeyGroupResult>(`/admin/api-keys/${id}`, payload)
  return data
}

export const apiKeysAPI = {
  updateApiKeyGroup,
  updateApiKeyGroupBindings
}

export default apiKeysAPI

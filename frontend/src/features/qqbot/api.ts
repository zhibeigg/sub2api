import { apiClient } from '@/api/client'
import type {
  BindingInspection,
  CompleteBindingResponse,
  QQBotBindingFilters,
  QQBotBindingPage,
  QQBotConfig,
  QQBotProbeRequest,
  QQBotProbeResult,
  QQBotRuntime,
  QQBotStats,
  QQBotUpdateRequest,
} from './types'
import { bindingQueryParams } from './viewModel'

const adminBasePath = '/admin/qqbot'

export async function getConfig(): Promise<QQBotConfig> {
  const { data } = await apiClient.get<QQBotConfig>(`${adminBasePath}/config`)
  return data
}

export async function updateConfig(payload: QQBotUpdateRequest): Promise<QQBotConfig> {
  const { data } = await apiClient.put<QQBotConfig>(`${adminBasePath}/config`, payload)
  return data
}

export async function probe(payload: QQBotProbeRequest): Promise<QQBotProbeResult> {
  const { data } = await apiClient.post<QQBotProbeResult>(`${adminBasePath}/probe`, payload)
  return data
}

export async function getRuntime(): Promise<QQBotRuntime> {
  const { data } = await apiClient.get<QQBotRuntime>(`${adminBasePath}/runtime`)
  return data
}

export async function getStats(): Promise<QQBotStats> {
  const { data } = await apiClient.get<QQBotStats>(`${adminBasePath}/stats`)
  return data
}

export async function listBindings(
  filters: QQBotBindingFilters,
  page: number,
  pageSize: number,
): Promise<QQBotBindingPage> {
  const { data } = await apiClient.get<QQBotBindingPage>(`${adminBasePath}/bindings`, {
    params: bindingQueryParams(filters, page, pageSize),
  })
  return data
}

export async function unbind(id: string, reason: string): Promise<{ status: string }> {
  const { data } = await apiClient.post<{ status: string }>(
    `${adminBasePath}/bindings/${encodeURIComponent(id)}/unbind`,
    { reason },
  )
  return data
}

export async function inspectBinding(token: string): Promise<BindingInspection> {
  const { data } = await apiClient.post<BindingInspection>('/public/bindings/inspect', { token })
  return data
}

export async function completeBinding(token: string, qqNumber: string): Promise<CompleteBindingResponse> {
  const { data } = await apiClient.post<CompleteBindingResponse>('/public/bindings/complete', {
    token,
    qq_number: qqNumber,
  })
  return data
}

export const qqbotAPI = {
  getConfig,
  updateConfig,
  probe,
  getRuntime,
  getStats,
  listBindings,
  unbind,
  inspectBinding,
  completeBinding,
}

export default qqbotAPI

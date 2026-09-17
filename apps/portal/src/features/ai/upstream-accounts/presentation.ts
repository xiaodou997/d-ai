import type { AccountDTO } from '@/api/types/ai'
import type { Stability, StabilityWindow } from '@/features/ai/upstream-stability/api'

export const stabilityWindowLabels: Record<StabilityWindow, string> = {
  '1h': '最近 1 小时',
  '24h': '最近 24 小时',
  '7d': '最近 7 天'
}

export const availabilityLabels: Record<string, string> = {
  available: '全部可用',
  partial: '部分受限',
  unavailable: '全部受限',
  unknown: '状态未知'
}

export function accountHost(baseUrl?: string) {
  if (!baseUrl) return ''
  try {
    return new URL(baseUrl).host
  } catch {
    return baseUrl.replace(/^https?:\/\//, '').split('/')[0]
  }
}

export function accountEndpointHosts(account: AccountDTO) {
  return [...new Set((account.endpoints || []).map((endpoint) => accountHost(endpoint.base_url)).filter(Boolean))].join(' · ')
}

export function successRateLabel(rate?: number | null) {
  return rate == null ? '—' : `${rate.toFixed(1)}%`
}

export function successRateTitle(stability: Stability, window: StabilityWindow) {
  const sampleHint = stability.samples > 0 && stability.samples < 20 ? '；样本较少' : ''
  return `${stabilityWindowLabels[window]}成功率；${stability.samples} 次有效尝试${sampleHint}；取消、冷却跳过和本地拒绝不计入统计`
}

export function availabilityTone(value?: string): 'positive' | 'warning' | 'danger' | 'neutral' {
  if (value === 'available') return 'positive'
  if (value === 'partial' || value === 'unknown') return 'warning'
  if (value === 'unavailable') return 'danger'
  return 'neutral'
}

export function endpointSuccessRates(detail?: Stability | null) {
  const rates = new Map<string, number>()
  if (!detail) return rates
  const totals = new Map<string, { successes: number; samples: number }>()
  const failures = new Set(['server_error', 'timeout', 'network_error', 'unauthorized', 'rate_limited', 'model_error'])
  for (const row of detail.models || []) {
    if (!row.endpoint_id || (row.outcome !== 'success' && !failures.has(row.outcome))) continue
    const total = totals.get(row.endpoint_id) || { successes: 0, samples: 0 }
    total.samples += row.count
    if (row.outcome === 'success') total.successes += row.count
    totals.set(row.endpoint_id, total)
  }
  for (const [id, total] of totals) {
    if (total.samples > 0) rates.set(id, total.successes / total.samples * 100)
  }
  return rates
}

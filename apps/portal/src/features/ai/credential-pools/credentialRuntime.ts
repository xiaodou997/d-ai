import type { PoolCredentialDTO } from '@/api/types/ai'
import type { Stability } from '@/features/ai/upstream-stability/api'

type RuntimeState = 'available' | 'partial' | 'cooling' | 'recovering' | 'expired' | 'disabled' | 'unknown'
export interface CredentialRuntime {
  state: RuntimeState
  label: string
  tone: 'positive' | 'warning' | 'danger' | 'info'
  reason: string
  retryAt?: number
}
const reasons: Record<string, string> = { unauthorized: '认证失败，可能需要更新凭据', rate_limited: '上游限流', quota_exhausted: '上游额度已耗尽', capacity_exhausted: '上游共享容量不足', model_error: '模型或权限受限', server_error: '上游服务异常', timeout: '请求超时', network_error: '连接异常' }
export function credentialRuntime(account: PoolCredentialDTO, snapshot: Stability | null | undefined, now = Date.now()): CredentialRuntime {
  if (account.status === 'disabled' || snapshot?.config_status === 'disabled') return { state: 'disabled', label: '已停用', tone: 'info', reason: '' }
  const exhausted = (account.quotas || []).filter(quota => !quota.credits_available).flatMap(quota => quota.windows.filter(window => ((window.used_percent ?? -1) >= 100 || window.remaining === 0) && (window.reset_at || 0) > now).map(window => ({ model: quota.model, resetAt: window.reset_at! })))
  if (exhausted.some(window => !window.model)) return { state: 'cooling', label: '额度已耗尽', tone: 'warning', reason: '等待上游配额窗口重置', retryAt: Math.max(...exhausted.map(window => window.resetAt)) }
  const states = (snapshot?.states || []).filter(state => !state.scope.credential_id || state.scope.credential_id === account.id)
  const cooling = states.filter(state => state.phase === 'cooling' && (!state.retry_at || state.retry_at > now))
  const retryAt = Math.max(account.cooldown_until || 0, ...cooling.map(state => state.retry_at || 0)) || undefined
  const paths = (snapshot?.paths || []).filter(path => path.credential_id === account.id)
  const blockedPaths = paths.filter(path => cooling.some(state => (!state.scope.model || state.scope.model === path.model) && (!state.scope.operation || state.scope.operation === path.operation)))
  const globalCooling = cooling.some(state => !state.scope.model && !state.scope.operation)
  const reason = cooling.map(state => reasons[state.reason || ''] || state.reason || '临时冷却').filter((value, index, all) => all.indexOf(value) === index).join('；') || account.invalid_reason || ''
  if ((account.cooldown_until || 0) > now || globalCooling || paths.length > 0 && blockedPaths.length === paths.length) return { state: 'cooling', label: '冷却中', tone: 'warning', reason, retryAt }
  if (exhausted.length) return { state: 'partial', label: '部分模型额度耗尽', tone: 'warning', reason: exhausted.map(window => window.model).join('、'), retryAt: Math.max(...exhausted.map(window => window.resetAt)) }
  if (cooling.length) return { state: 'partial', label: '部分模型受限', tone: 'warning', reason, retryAt }
  if (states.some(state => state.phase === 'recovering')) return { state: 'recovering', label: '恢复试用中', tone: 'warning', reason }
  if (account.expires_at && account.expires_at <= now) return { state: 'expired', label: '令牌已过期', tone: 'danger', reason: '等待刷新，或编辑账号更新凭据' }
  if (!snapshot || snapshot.state_error) return { state: 'unknown', label: '状态待确认', tone: 'info', reason: snapshot?.state_error || '正在读取运行状态' }
  return { state: 'available', label: '可调度', tone: 'positive', reason: '' }
}

export function credentialQuotaLines(account: PoolCredentialDTO, now = Date.now()) {
  return (account.quotas || []).flatMap(quota => quota.windows.map(window => {
    const minutes = window.window_minutes
    const label = minutes ? minutes >= 1440 && minutes % 1440 === 0 ? `${minutes / 1440} 天窗口` : minutes >= 60 && minutes % 60 === 0 ? `${minutes / 60} 小时窗口` : `${minutes} 分钟窗口` : ({ requests: '请求数', tokens: 'Token', 'input-tokens': '输入 Token', 'output-tokens': '输出 Token', primary: '主窗口', secondary: '次窗口', five_hour: '5 小时窗口', seven_day: '7 天窗口', REQUESTS: '请求数', quota: '模型配额' } as Record<string, string>)[window.key] || window.key
    const stale = Boolean(window.reset_at && window.reset_at <= now)
    const value = stale ? '等待更新' : window.used_percent !== undefined ? `剩余 ${Math.max(0, 100 - window.used_percent).toFixed(1)}%` : window.remaining !== undefined ? `剩余 ${window.remaining.toLocaleString()}${window.limit !== undefined ? ` / ${window.limit.toLocaleString()}` : ''}` : '未知'
    return { key: `${quota.model || '*'}:${window.key}`, label: `${quota.model ? quota.model + ' · ' : ''}${label}`, value, resetAt: window.reset_at, observedAt: quota.observed_at }
  }))
}

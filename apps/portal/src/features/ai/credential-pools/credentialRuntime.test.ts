import { describe, expect, it } from 'vitest'
import type { PoolCredentialDTO } from '@/api/types/ai'
import type { Stability } from '@/features/ai/upstream-stability/api'
import { credentialQuotaLines, credentialRuntime } from './credentialRuntime'

const now = 1800000000000
const account = { id: 'a', status: 'active' } as PoolCredentialDTO
const snapshot = { config_status: 'active', states: [], paths: [{ credential_id: 'a', model: 'one', operation: 'responses' }, { credential_id: 'a', model: 'two', operation: 'responses' }] } as unknown as Stability
describe('credential runtime state', () => {
  it('distinguishes enabled configuration from unknown, cooling and disabled state', () => {
    expect(credentialRuntime(account, null, now).state).toBe('unknown')
    expect(credentialRuntime({ ...account, cooldown_until: now + 1000 }, snapshot, now).state).toBe('cooling')
    expect(credentialRuntime({ ...account, status: 'disabled' }, snapshot, now).state).toBe('disabled')
  })
  it('keeps other models available after one model is cooled', () => {
    const partial = { ...snapshot, states: [{ phase: 'cooling', retry_at: now + 1000, reason: 'rate_limited', scope: { credential_id: 'a', model: 'one', operation: 'responses' } }] } as Stability
    expect(credentialRuntime(account, partial, now).state).toBe('partial')
    expect(credentialRuntime(account, { ...partial, states: [{ ...partial.states![0], scope: { ...partial.states![0].scope, credential_id: 'other' } }] }, now).state).toBe('available')
  })
  it('honors quota windows and their reset time without mixing short and long windows', () => {
    const quota = { ...account, quotas: [{ source: 'response_headers', observed_at: now, windows: [{ key: 'primary', used_percent: 100, window_minutes: 10080, reset_at: now + 1000 }, { key: 'secondary', used_percent: 25, window_minutes: 300 }] }] }
    expect(credentialRuntime(quota, snapshot, now).label).toBe('额度已耗尽')
    expect(credentialQuotaLines(quota, now).map(line => line.label)).toEqual(['7 天窗口', '5 小时窗口'])
    expect(credentialQuotaLines(quota, now + 1001)[0].value).toBe('等待更新')
    expect(credentialRuntime(quota, snapshot, now + 1001).state).toBe('available')
  })
})

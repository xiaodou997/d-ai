import { describe, expect, it } from 'vitest'
import type { PoolCredentialDTO } from '@/api/types/ai'
import { credentialIdentity, markCredentialDuplicates, normalizeCredential, parseCredentialFile, pendingCredentialImports } from './credentialImport'

const raw = (workspace: string) => ({ email: 'same@example.com', tokens: { access_token: 'access', refresh_token: 'refresh', account_id: workspace }, user_id: 'user' })
describe('credential import', () => {
  it('reads native Codex tokens, account arrays and millisecond expiry', () => {
    const entries = parseCredentialFile({ accounts: [raw('one'), raw('two')] }, 'accounts.json', 'codex')
    expect(entries).toHaveLength(2)
    expect(entries[0].data).toMatchObject({ access_token: 'access', refresh_token: 'refresh', auth_metadata: { account_id: 'one', user_id: 'user' } })
    expect(normalizeCredential({ access_token: 'token', expires_at: 1800000000000 }, 'codex').expires_at).toBe(1800000000)
    expect(normalizeCredential({ credentials: { access_token: 'token', expiry_date: '2027-01-15T08:00:00Z' } }, 'codex').expires_at).toBe(Date.parse('2027-01-15T08:00:00Z') / 1000)
  })
  it('previews duplicates while keeping different workspaces for the same user', () => {
    const entries = parseCredentialFile([raw('one'), raw('two'), raw('two')], 'accounts.json', 'codex')
    const existing = [{ id: 'existing', pool_id: 'pool', status: 'active', token_type: 'bearer', scope: '', weight: 100, consecutive_fail_count: 0, success_count: 0, fail_count: 0, name: '原账号', provider_type: 'codex', email: 'same@example.com', auth_metadata: { account_id: 'one', user_id: 'user' } }] as PoolCredentialDTO[]
    markCredentialDuplicates(entries, existing)
    expect(entries[0].duplicate).toContain('原账号')
    expect(entries[1].duplicate).toBeUndefined()
    expect(entries[2].duplicate).toContain('本批次')
    expect(pendingCredentialImports(entries)).toEqual([entries[1]])
    expect(credentialIdentity({ provider_type: 'codex', email: 'same@example.com' })).toBe('')
  })
  it('retries only failures and never resubmits successful entries', () => {
    const entries = parseCredentialFile([raw('one'), raw('two')], 'accounts.json', 'codex')
    entries[0].importStatus = 'success'; entries[1].importStatus = 'failed'
    expect(pendingCredentialImports(entries)).toEqual([entries[1]])
  })
  it('rejects incompatible providers and malformed data before submission', () => {
    expect(parseCredentialFile([{ provider_type: 'claude_oauth', access_token: 'a' }, { tokens: {} }], 'bad.json', 'codex').every(entry => entry.error)).toBe(true)
    expect(() => normalizeCredential({ access_token: 'token', expires_at: 'invalid' }, 'codex')).toThrow('到期时间')
  })
})

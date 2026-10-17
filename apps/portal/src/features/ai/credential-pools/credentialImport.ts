import type { PoolCredentialDTO, PoolCredentialWriteRequest } from '@/api/types/ai'

type AccountIdentity = Pick<PoolCredentialDTO, 'provider_type' | 'email' | 'auth_metadata'>
export interface CredentialImportEntry {
  name: string
  data?: PoolCredentialWriteRequest
  error?: string
  duplicate?: string
  importStatus?: 'pending' | 'success' | 'failed'
  importError?: string
}

const record = (value: unknown): Record<string, unknown> => value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
const string = (value: unknown) => typeof value === 'string' ? value.trim() : ''
const identityFields = ['account_id', 'plan_type', 'user_id', 'account_user_id', 'workspace_id', 'organization_id', 'project_id']

export function credentialIdentity(account: AccountIdentity): string {
  const meta = account.auth_metadata || {}
  const value = (...keys: string[]) => keys.map(key => string(meta[key])).find(Boolean) || ''
  const accountId = value('account_id', 'chatgpt_account_id', 'accountId')
  const workspace = value('workspace_id', 'organization_id', 'project_id')
  let user = value('account_user_id', 'user_id')
  if (!accountId && !workspace && !user) return ''
  if (!user) user = account.email.trim().toLowerCase()
  return JSON.stringify([account.provider_type, accountId, workspace, user])
}

export function normalizeCredential(raw: unknown, provider: string): PoolCredentialWriteRequest {
  const source = record(raw)
  const tokens = { ...source, ...record(source.credentials), ...record(source.tokens) }
  const providerType = string(source.provider_type) || provider
  if (providerType !== provider) throw new Error('供应商类型与当前账号池不一致')
  const accessToken = string(tokens.access_token)
  if (!accessToken) throw new Error('缺少 access_token')
  const metadata = { ...record(source.auth_metadata), ...record(tokens.auth_metadata) }
  for (const key of identityFields) {
    const value = string(tokens[key]) || string(source[key])
    if (value) metadata[key] = value
  }
  const rawExpiry = tokens.expires_at ?? tokens.expiry_date ?? source.expires_at
  let expiry: number | undefined
  if (rawExpiry !== undefined && rawExpiry !== null && rawExpiry !== '') {
    const numeric = Number(rawExpiry)
    const parsed = Number.isFinite(numeric) ? numeric : Date.parse(String(rawExpiry)) / 1000
    if (!Number.isFinite(parsed) || parsed <= 0) throw new Error('到期时间格式无效')
    expiry = Math.floor(parsed > 1e12 ? parsed / 1000 : parsed)
  }
  const weight = source.weight === undefined ? undefined : Number(source.weight)
  if (weight !== undefined && (!Number.isInteger(weight) || weight < 0)) throw new Error('权重必须是非负整数')
  return {
    name: string(source.name), email: string(source.email), provider_type: providerType,
    access_token: accessToken, refresh_token: string(tokens.refresh_token), token_type: string(tokens.token_type),
    scope: string(tokens.scope), expires_at: expiry, weight,
    auth_metadata: Object.keys(metadata).length ? metadata : undefined
  }
}

export function parseCredentialFile(raw: unknown, fileName: string, provider: string): CredentialImportEntry[] {
  const source = record(raw)
  const data = record(source.data)
  const accounts = Array.isArray(raw) ? raw : Array.isArray(source.accounts) ? source.accounts : Array.isArray(data.accounts) ? data.accounts : [raw]
  if (!accounts.length) return [{ name: fileName, error: '文件中没有账号' }]
  return accounts.map((account, index) => {
    const name = accounts.length > 1 ? `${fileName} · ${index + 1}` : fileName
    try { return { name, data: normalizeCredential(account, provider) } }
    catch (error) { return { name, error: error instanceof Error ? error.message : '账号格式无效' } }
  })
}

export function markCredentialDuplicates(entries: CredentialImportEntry[], existing: PoolCredentialDTO[]) {
  const known = new Map(existing.map(account => [credentialIdentity(account), account.name] as const).filter(([key]) => key))
  const seen = new Set<string>()
  for (const entry of entries) {
    entry.duplicate = undefined
    if (!entry.data) continue
    const data = entry.data
    const identity = credentialIdentity({ provider_type: data.provider_type || '', email: data.email || '', auth_metadata: data.auth_metadata })
    const key = identity || `token:${data.refresh_token || data.access_token}`
    if (identity && known.has(identity)) entry.duplicate = `已存在账号「${known.get(identity)}」，请编辑原账号更新凭据`
    else if (seen.has(key)) entry.duplicate = '本批次中存在重复账号'
    seen.add(key)
  }
}

export function pendingCredentialImports(entries: CredentialImportEntry[]) {
  return entries.filter(entry => entry.data && !entry.error && !entry.duplicate && entry.importStatus !== 'success')
}

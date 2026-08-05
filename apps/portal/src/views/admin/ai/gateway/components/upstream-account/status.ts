export type UpstreamAccountStatus = 'active' | 'invalid' | 'disabled'

export function upstreamAccountStatusLabel(status: string): string {
  switch (status) {
    case 'active':
      return '启用'
    case 'invalid':
      return '失效'
    case 'disabled':
      return '停用'
    default:
      return status || '未知'
  }
}

export function upstreamAccountStatusTagType(status: string): 'success' | 'danger' | 'info' {
  if (status === 'active') return 'success'
  if (status === 'invalid') return 'danger'
  return 'info'
}

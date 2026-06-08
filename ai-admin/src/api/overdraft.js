import request from '@/utils/request'

// ============================================
// 透支额度管理（Phase 1d，2026-05）
//
// accountType: 1 = 租户 (iam_tenants), 2 = 用户 (iam_users)
// 后端路由：/api/v1/admin/overdraft
// 仅管理员可调整；每次调整必填 reason，落 bill_overdraft_adjustments 审计表。
// ============================================

/**
 * 查询账户当前透支额度 + 调整历史
 * @param {Object} params - { accountType, accountId, limit }
 * @returns {Promise<{accountType, accountId, overdraftLimit, currentOverdraft, history}>}
 */
export function getOverdraftStatus(params) {
  return request({
    url: '/urm/v1/admin/overdraft',
    method: 'get',
    params,
  })
}

/**
 * 调整账户透支额度上限
 * @param {Object} payload - { accountType, accountId, newLimit, reason }
 * @returns {Promise<Object>}
 */
export function setOverdraftLimit(payload) {
  return request({
    url: '/urm/v1/admin/overdraft',
    method: 'put',
    data: payload,
  })
}

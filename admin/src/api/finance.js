import request from '@/utils/request'

/**
 * 平台管理员租户充值快捷入口。
 * 金额单位按 URM 约定传分，积分按整数积分传递。
 */
export function rechargeTenant(data) {
  return request({
    url: '/urm/v1/admin/operations/recharge',
    method: 'post',
    data: {
      ...data,
      packageType: 1,
      tenantId: data.customerId
    }
  })
}

/**
 * 查询租户充值记录，不作为全量交易流水替代。
 */
export function getTenantRechargeRecords(params) {
  return request({
    url: '/urm/v1/account/recharge-records',
    method: 'get',
    params: {
      ...params,
      rechargeType: 1
    }
  })
}

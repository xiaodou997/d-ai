import request from '@/utils/request'

/**
 * 获取我的余额（积分包总览）
 */
export const getBalance = () => {
  return request.get('/urm/v1/account/balance?detail=true')
}

/**
 * 获取我的积分包列表
 */
export const getMyPackages = () => {
  return request.get('/urm/v1/account/balance?detail=true')
}

/**
 * 获取我的积分流水（分页）
 */
export const getTransactions = (params) => {
  return request.get('/urm/v1/account/transactions', { params })
}

/**
 * 获取我的充值记录（分页）
 */
export const getRechargeRecords = (params) => {
  return request.get('/urm/v1/account/recharge-records', { params })
}

/**
 * 修改密码
 */
export const changePasswordApi = (oldPassword, newPassword) => {
  return request({
    url: '/urm/oauth2/password',
    method: 'put',
    data: { oldPassword, newPassword }
  })
}

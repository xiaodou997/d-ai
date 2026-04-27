import request from '@/utils/request'

export function getConsumptionTrend(params) {
  return request({ url: '/urm/v1/admin/analytics/consumption-trend', method: 'get', params })
}

export function getResourceStatistics(params) {
  return request({ url: '/urm/v1/admin/analytics/resource-statistics', method: 'get', params })
}

/**
 * 全局统计（支持时间段）
 * @param {Object} params - { days: 7|30|90|0 }  0=全部时间
 */
export function getGlobalStats(params) {
  return request({ url: '/urm/v1/admin/analytics/global-stats', method: 'get', params })
}

/**
 * 获取仪表板告警信息
 * 返回透支账户、超时预授权、失败交易等告警数据
 */
export function getDashboardAlerts() {
  return request({ url: '/urm/v1/admin/dashboard/alerts', method: 'get' })
}

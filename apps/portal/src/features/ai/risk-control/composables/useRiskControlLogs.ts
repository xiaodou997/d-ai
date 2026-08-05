import { reactive, shallowRef } from 'vue'
import { ElMessage } from 'element-plus'

import { riskControlApi, type RiskControlApi } from '../api'
import type { RiskControlLogDTO, RiskControlLogFilters } from '../types'

function errorMessage(error: unknown): string {
  return error instanceof Error && error.message ? error.message : '加载风控日志失败'
}

export function useRiskControlLogs(api: RiskControlApi = riskControlApi) {
  const logsLoading = shallowRef(false)
  const logs = shallowRef<RiskControlLogDTO[]>([])
  const logsTotal = shallowRef(0)
  const logFilters = reactive<RiskControlLogFilters>({
    tenant_id: '',
    user_id: '',
    mode: '',
    action: '',
    flagged: ''
  })

  async function fetchLogs() {
    logsLoading.value = true
    try {
      const result = await api.listRiskControlLogs({
        tenant_id: logFilters.tenant_id || undefined,
        user_id: logFilters.user_id || undefined,
        mode: logFilters.mode || undefined,
        action: logFilters.action || undefined,
        flagged: logFilters.flagged || undefined,
        limit: 100
      })
      logs.value = result.items || []
      logsTotal.value = result.total || 0
    } catch (error: unknown) {
      ElMessage.error(errorMessage(error))
    } finally {
      logsLoading.value = false
    }
  }

  return { fetchLogs, logFilters, logs, logsLoading, logsTotal }
}

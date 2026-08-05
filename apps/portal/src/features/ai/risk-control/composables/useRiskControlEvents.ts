import { computed, reactive, shallowRef } from 'vue'
import { ElMessage } from 'element-plus'

import { riskControlApi, type RiskControlApi } from '../api'
import type {
  RiskControlEventResolution,
  RiskControlEventStatus,
  RiskEventDTO
} from '../types'

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback
}

export function useRiskControlEvents(api: RiskControlApi = riskControlApi) {
  const eventsLoading = shallowRef(false)
  const events = shallowRef<RiskEventDTO[]>([])
  const eventsTotal = shallowRef(0)
  const eventStatusFilter = shallowRef<RiskControlEventStatus | ''>('open')
  const resolveDialogVisible = shallowRef(false)
  const resolving = shallowRef(false)
  const resolveTarget = shallowRef<RiskEventDTO | null>(null)
  const resolveForm = reactive<RiskControlEventResolution>({ status: 'resolved', note: '' })

  const openEventCount = computed(() =>
    events.value.filter((item) => item.status === 'open').length
  )

  async function fetchEvents() {
    eventsLoading.value = true
    try {
      const result = await api.listRiskControlEvents({
        status: eventStatusFilter.value || undefined,
        limit: 100
      })
      events.value = result.items || []
      eventsTotal.value = result.total || 0
    } catch (error: unknown) {
      ElMessage.error(errorMessage(error, '加载风险事件失败'))
    } finally {
      eventsLoading.value = false
    }
  }

  function openResolveDialog(row: RiskEventDTO) {
    resolveTarget.value = row
    resolveForm.status = 'resolved'
    resolveForm.note = ''
    resolveDialogVisible.value = true
  }

  async function submitResolve() {
    if (!resolveTarget.value) return
    resolving.value = true
    try {
      await api.resolveRiskControlEvent(resolveTarget.value.id, {
        status: resolveForm.status,
        note: resolveForm.note || undefined
      })
      resolveDialogVisible.value = false
      ElMessage.success('风险事件已更新')
      await fetchEvents()
    } catch (error: unknown) {
      ElMessage.error(errorMessage(error, '处置失败'))
    } finally {
      resolving.value = false
    }
  }

  return {
    eventStatusFilter,
    events,
    eventsLoading,
    eventsTotal,
    fetchEvents,
    openEventCount,
    openResolveDialog,
    resolveDialogVisible,
    resolveForm,
    resolveTarget,
    resolving,
    submitResolve
  }
}

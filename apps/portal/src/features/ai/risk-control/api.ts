import { aiAdminApi } from '@/api/aiAdmin'
import type {
  RiskControlConfigDTO,
  RiskControlConfigWriteRequest,
  RiskControlEventQuery,
  RiskControlEventResolution,
  RiskControlLogQuery,
  RiskControlLogsOutputBody,
  RiskControlTestResultDTO,
  RiskEventDTO,
  RiskEventsOutputBody
} from './types'

export type { RiskControlConfigDTO, RiskControlTestResultDTO } from './types'

export interface RiskControlApi {
  getRiskControlConfig(): Promise<RiskControlConfigDTO>
  updateRiskControlConfig(body: RiskControlConfigWriteRequest): Promise<RiskControlConfigDTO>
  testRiskControlModeration(text: string): Promise<RiskControlTestResultDTO>
  listRiskControlLogs(params: RiskControlLogQuery): Promise<RiskControlLogsOutputBody>
  listRiskControlEvents(params: RiskControlEventQuery): Promise<RiskEventsOutputBody>
  resolveRiskControlEvent(eventId: string, body: RiskControlEventResolution): Promise<RiskEventDTO>
}

export const riskControlApi: RiskControlApi = {
  getRiskControlConfig: () => aiAdminApi.getRiskControlConfig(),
  updateRiskControlConfig: (body) => aiAdminApi.updateRiskControlConfig(body),
  testRiskControlModeration: (text) => aiAdminApi.testRiskControlModeration(text),
  listRiskControlLogs: (params) => aiAdminApi.listRiskControlLogs({ ...params }),
  listRiskControlEvents: (params) => aiAdminApi.listRiskControlEvents({ ...params }),
  resolveRiskControlEvent: (eventId, body) => aiAdminApi.resolveRiskControlEvent(eventId, body)
}

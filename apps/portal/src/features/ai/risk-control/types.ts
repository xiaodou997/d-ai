import type {
  RiskControlConfigDTO as LegacyRiskControlConfigDTO,
  RiskEventDTO as LegacyRiskEventDTO
} from '../../../types/ai'

export type {
  KeywordEntryDTO,
  KeywordConfigDTO,
  PinyinConfigDTO,
  RiskControlConfigDTO,
  RiskControlConfigWriteRequest,
  RiskControlLogDTO,
  RiskControlLogsOutputBody,
  RiskControlTestResultDTO,
  RiskEventDTO,
  RiskEventsOutputBody
} from '../../../types/ai'

export type RiskControlLogMode = Extract<LegacyRiskControlConfigDTO['mode'], 'observe' | 'pre_block'>
export type RiskControlLogAction = 'allow' | 'block' | 'keyword_block' | 'error'
export type RiskControlFlaggedFilter = 'true' | 'false'
export type RiskControlEventStatus = LegacyRiskEventDTO['status']
export type RiskControlEventResolutionStatus = Exclude<RiskControlEventStatus, 'open'>

export interface RiskControlLogFilters {
  tenant_id: string
  user_id: string
  mode: RiskControlLogMode | ''
  action: RiskControlLogAction | ''
  flagged: RiskControlFlaggedFilter | ''
}

export interface RiskControlLogQuery {
  tenant_id?: string
  user_id?: string
  mode?: RiskControlLogMode
  action?: RiskControlLogAction
  flagged?: RiskControlFlaggedFilter
  limit: number
}

export interface RiskControlEventQuery {
  status?: RiskControlEventStatus
  limit: number
}

export interface RiskControlEventResolution {
  status: RiskControlEventResolutionStatus
  note?: string
}

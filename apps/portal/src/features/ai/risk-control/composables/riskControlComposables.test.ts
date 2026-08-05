import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { RiskControlApi } from '../api'
import type { RiskControlConfigDTO, RiskEventDTO } from '../types'
import { useRiskControlConfig } from './useRiskControlConfig'
import { useRiskControlEvents } from './useRiskControlEvents'
import { useRiskControlLogs } from './useRiskControlLogs'

const dependencies = vi.hoisted(() => ({
  api: {
    getRiskControlConfig: vi.fn(),
    updateRiskControlConfig: vi.fn(),
    testRiskControlModeration: vi.fn(),
    listRiskControlLogs: vi.fn(),
    listRiskControlEvents: vi.fn(),
    resolveRiskControlEvent: vi.fn()
  },
  messages: {
    error: vi.fn(),
    success: vi.fn(),
    warning: vi.fn()
  }
}))

vi.mock('../api', () => ({ riskControlApi: dependencies.api }))
vi.mock('element-plus', () => ({ ElMessage: dependencies.messages }))

const config: RiskControlConfigDTO = {
  enabled: true,
  mode: 'observe',
  config_revision: 1,
  keyword: {
    enabled: true,
    entries: [{ word: 'existing', level: 'block', require_with: [], note: '' }],
    homoglyph_map_extra: {},
    pinyin: { enabled: false, entries: [], include_initials: false }
  },
  provider: {
    base_url: 'https://api.openai.com',
    model: 'omni-moderation-latest',
    has_api_key: true,
    timeout_ms: 5000
  },
  thresholds: { violence: 0.8 },
  scope_group_ids: ['group-1', 'group-2'],
  sample_rate: 1,
  verdict_cache_ttl_seconds: 600,
  violation_window_hours: 24,
  risk_event_threshold: 3,
  record_non_hits: false,
  block_status_code: 451,
  block_message: '请求内容未通过安全审核'
}

const event: RiskEventDTO = {
  id: 'event-1',
  event_type: 'moderation_violation',
  severity: 'high',
  summary: '连续命中内容安全规则',
  status: 'open',
  created_at: 1
}

function createApi(overrides: Partial<RiskControlApi> = {}): RiskControlApi {
  return {
    getRiskControlConfig: async () => config,
    updateRiskControlConfig: async () => config,
    testRiskControlModeration: async () => ({ flagged: false, from_cache: false }),
    listRiskControlLogs: async () => ({ items: [], total: 0 }),
    listRiskControlEvents: async () => ({ items: [], total: 0 }),
    resolveRiskControlEvent: async () => event,
    ...overrides
  }
}

describe('risk control composables', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('preserves hidden group scopes and filters empty keyword entries when saving config', async () => {
    const updateRiskControlConfig = vi.fn(async () => config)
    const state = useRiskControlConfig(createApi({ updateRiskControlConfig }))

    await state.fetchConfig()
    state.openConfigDialog()
    // Add a new entry with a word, and leave one empty (should be filtered out).
    state.form.keyword_entries[0].word = 'first'
    state.form.keyword_entries.push({ word: '', level: 'block', require_with: [], note: '' })
    state.form.keyword_entries.push({ word: 'second', level: 'block', require_with: [], note: '' })
    await state.saveConfig()

    expect(updateRiskControlConfig).toHaveBeenCalledWith(expect.objectContaining({
      keyword: expect.objectContaining({
        entries: [
          expect.objectContaining({ word: 'first' }),
          expect.objectContaining({ word: 'second' })
        ]
      }),
      scope_group_ids: ['group-1', 'group-2']
    }))
    expect(state.configDialogVisible.value).toBe(false)
    expect(dependencies.messages.success).toHaveBeenCalledWith('风控配置已保存')
  })

  it('maps empty log filters to undefined and keeps the query limit at 100', async () => {
    const listRiskControlLogs = vi.fn(async () => ({ items: [], total: 0 }))
    const state = useRiskControlLogs(createApi({ listRiskControlLogs }))
    state.logFilters.tenant_id = 'tenant-1'

    await state.fetchLogs()

    expect(listRiskControlLogs).toHaveBeenCalledWith({
      tenant_id: 'tenant-1',
      user_id: undefined,
      mode: undefined,
      action: undefined,
      flagged: undefined,
      limit: 100
    })
  })

  it('closes the dialog and refreshes events after resolving an event', async () => {
    const listRiskControlEvents = vi.fn(async () => ({ items: [event], total: 1 }))
    const resolveRiskControlEvent = vi.fn(async () => ({ ...event, status: 'resolved' as const }))
    const state = useRiskControlEvents(createApi({
      listRiskControlEvents,
      resolveRiskControlEvent
    }))

    await state.fetchEvents()
    state.openResolveDialog(event)
    state.resolveForm.note = '人工复核完成'
    await state.submitResolve()

    expect(resolveRiskControlEvent).toHaveBeenCalledWith('event-1', {
      status: 'resolved',
      note: '人工复核完成'
    })
    expect(state.resolveDialogVisible.value).toBe(false)
    expect(listRiskControlEvents).toHaveBeenCalledTimes(2)
    expect(dependencies.messages.success).toHaveBeenCalledWith('风险事件已更新')
  })
})

import { computed, reactive, shallowRef } from 'vue'
import { ElMessage } from 'element-plus'

import { riskControlApi, type RiskControlApi } from '../api'
import type {
  KeywordEntryDTO,
  RiskControlConfigDTO,
  RiskControlConfigWriteRequest,
  RiskControlTestResultDTO
} from '../types'

const modeLabels: Record<RiskControlConfigDTO['mode'], string> = {
  off: '关闭',
  observe: '旁路观察（不拦截，仅记录）',
  pre_block: '同步拦截'
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback
}

function emptyKeywordEntry(): KeywordEntryDTO {
  return { word: '', level: 'block', require_with: [], note: '' }
}

function entriesFromConfig(entries: KeywordEntryDTO[] | undefined): KeywordEntryDTO[] {
  if (!entries || entries.length === 0) return [emptyKeywordEntry()]
  return entries.map((e) => ({ ...e, require_with: [...(e.require_with || [])] }))
}

interface RiskControlConfigForm {
  enabled: boolean
  mode: RiskControlConfigDTO['mode']
  base_url: string
  model: string
  timeout_ms: number
  keyword_enabled: boolean
  keyword_entries: KeywordEntryDTO[]
  pinyin_enabled: boolean
  pinyin_entries: KeywordEntryDTO[]
  sample_rate: number
  verdict_cache_ttl_seconds: number
  violation_window_hours: number
  risk_event_threshold: number
  record_non_hits: boolean
  block_status_code: number
  block_message: string
  thresholds: Record<string, number>
}

export function useRiskControlConfig(api: RiskControlApi = riskControlApi) {
  const configLoading = shallowRef(false)
  const configSaving = shallowRef(false)
  const configDialogVisible = shallowRef(false)
  const testing = shallowRef(false)
  const testText = shallowRef('')
  const testResult = shallowRef<RiskControlTestResultDTO | null>(null)
  const config = shallowRef<RiskControlConfigDTO | null>(null)
  const apiKeyInput = shallowRef('')
  const form = reactive<RiskControlConfigForm>({
    enabled: false,
    mode: 'observe',
    base_url: 'https://api.openai.com',
    model: 'omni-moderation-latest',
    timeout_ms: 5000,
    keyword_enabled: true,
    keyword_entries: [emptyKeywordEntry()],
    pinyin_enabled: false,
    pinyin_entries: [emptyKeywordEntry()],
    sample_rate: 1,
    verdict_cache_ttl_seconds: 600,
    violation_window_hours: 24,
    risk_event_threshold: 3,
    record_non_hits: false,
    block_status_code: 451,
    block_message: '请求内容未通过安全审核',
    thresholds: {}
  })

  const statusSummary = computed(() => {
    if (!config.value) return '未加载'
    if (!config.value.enabled) return '已关闭'
    return modeLabels[config.value.mode] || config.value.mode
  })

  async function fetchConfig() {
    configLoading.value = true
    try {
      config.value = await api.getRiskControlConfig()
    } catch (error: unknown) {
      ElMessage.error(errorMessage(error, '加载风控配置失败'))
    } finally {
      configLoading.value = false
    }
  }

  function openConfigDialog() {
    const cfg = config.value
    Object.assign(form, {
      enabled: cfg?.enabled ?? false,
      mode: cfg?.mode ?? 'observe',
      base_url: cfg?.provider?.base_url || 'https://api.openai.com',
      model: cfg?.provider?.model || 'omni-moderation-latest',
      timeout_ms: cfg?.provider?.timeout_ms || 5000,
      keyword_enabled: cfg?.keyword?.enabled ?? true,
      keyword_entries: entriesFromConfig(cfg?.keyword?.entries),
      pinyin_enabled: cfg?.keyword?.pinyin?.enabled ?? false,
      pinyin_entries: entriesFromConfig(cfg?.keyword?.pinyin?.entries),
      sample_rate: cfg?.sample_rate ?? 1,
      verdict_cache_ttl_seconds: cfg?.verdict_cache_ttl_seconds ?? 600,
      violation_window_hours: cfg?.violation_window_hours ?? 24,
      risk_event_threshold: cfg?.risk_event_threshold ?? 3,
      record_non_hits: cfg?.record_non_hits ?? false,
      block_status_code: cfg?.block_status_code ?? 451,
      block_message: cfg?.block_message ?? '请求内容未通过安全审核',
      thresholds: { ...(cfg?.thresholds || {}) }
    })
    apiKeyInput.value = ''
    testText.value = ''
    testResult.value = null
    configDialogVisible.value = true
  }

  function filterEntries(entries: KeywordEntryDTO[]): KeywordEntryDTO[] {
    return entries
      .filter((e) => e.word.trim())
      .map((e) => ({
        word: e.word.trim(),
        level: e.level,
        require_with: e.require_with.filter((w: string) => w.trim()).map((w: string) => w.trim()),
        note: e.note || ''
      }))
  }

  async function saveConfig() {
    configSaving.value = true
    try {
      const payload: RiskControlConfigWriteRequest = {
        enabled: form.enabled,
        mode: form.mode,
        keyword: {
          enabled: form.keyword_enabled,
          entries: filterEntries(form.keyword_entries),
          homoglyph_map_extra: config.value?.keyword?.homoglyph_map_extra || {},
          pinyin: {
            enabled: form.pinyin_enabled,
            entries: filterEntries(form.pinyin_entries),
            include_initials: false
          }
        },
        provider: {
          base_url: form.base_url.trim(),
          model: form.model.trim(),
          timeout_ms: form.timeout_ms,
          ...(apiKeyInput.value ? { api_key: apiKeyInput.value } : {})
        },
        thresholds: form.thresholds,
        scope_group_ids: config.value?.scope_group_ids || [],
        sample_rate: form.sample_rate,
        verdict_cache_ttl_seconds: form.verdict_cache_ttl_seconds,
        violation_window_hours: form.violation_window_hours,
        risk_event_threshold: form.risk_event_threshold,
        record_non_hits: form.record_non_hits,
        block_status_code: form.block_status_code,
        block_message: form.block_message
      }
      config.value = await api.updateRiskControlConfig(payload)
      configDialogVisible.value = false
      ElMessage.success('风控配置已保存')
    } catch (error: unknown) {
      ElMessage.error(errorMessage(error, '保存失败'))
    } finally {
      configSaving.value = false
    }
  }

  async function runTest() {
    if (!testText.value.trim()) {
      ElMessage.warning('请输入待检测文本')
      return
    }
    testing.value = true
    try {
      testResult.value = await api.testRiskControlModeration(testText.value)
    } catch (error: unknown) {
      ElMessage.error(errorMessage(error, '检测失败'))
    } finally {
      testing.value = false
    }
  }

  function addKeywordEntry() {
    form.keyword_entries.push(emptyKeywordEntry())
  }

  function removeKeywordEntry(index: number) {
    form.keyword_entries.splice(index, 1)
  }

  function addPinyinEntry() {
    form.pinyin_entries.push(emptyKeywordEntry())
  }

  function removePinyinEntry(index: number) {
    form.pinyin_entries.splice(index, 1)
  }

  return {
    apiKeyInput,
    config,
    configDialogVisible,
    configLoading,
    configSaving,
    fetchConfig,
    form,
    openConfigDialog,
    runTest,
    saveConfig,
    statusSummary,
    testResult,
    testText,
    testing,
    addKeywordEntry,
    removeKeywordEntry,
    addPinyinEntry,
    removePinyinEntry
  }
}

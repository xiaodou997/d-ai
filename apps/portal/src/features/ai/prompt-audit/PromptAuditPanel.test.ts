import { flushPromises, mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { DsButton } from '@/shared/ui'
import PromptAuditPanel from './PromptAuditPanel.vue'

const api = vi.hoisted(() => ({
  getPromptAuditConfig: vi.fn(),
  listPromptAuditEvents: vi.fn(),
  probePromptAuditEndpoint: vi.fn(),
  updatePromptAuditConfig: vi.fn(),
  getPromptAuditRuntime: vi.fn(),
  deletePromptAuditEvent: vi.fn()
}))

vi.mock('./api', () => api)

const config = {
  enabled: false,
  mode: 'off',
  latest_turn_only: false,
  store_pass_events: false,
  worker_count: 4,
  queue_capacity: 4096,
  scanners: ['jailbreak'],
  tenant_ids: [],
  endpoints: [],
  config_revision: 1
}

describe('PromptAuditPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.getPromptAuditConfig.mockResolvedValue(config)
    api.listPromptAuditEvents.mockResolvedValue({
      total: 1,
      items: [{
        id: 'event-1', decision: 'critical', risk_level: 'critical', action: 'Block', safety: 'Unsafe',
        categories: ['jailbreak'], matched_scanners: ['jailbreak'], scanner_scores: { jailbreak: 1 },
        scanner_version: 'sileader/qwen3guard:0.6b', endpoint_id: 'guard-1', config_revision: 1,
        chunk_total: 1, latency_ms: 23, error_code: '', created_at: '2026-09-04T00:00:00Z',
        snapshot: { request_id: 'req-1', tenant_id: 'tenant-1', user_id: 'user-1', api_key_id: '', model_code: 'gpt-test', capability_type: 'chat', protocol: 'openai_chat', prompt_hash: 'hash', redacted_preview: 'danger***…', prompt_length: 40, message_count: 1 }
      }]
    })
    api.getPromptAuditRuntime.mockResolvedValue({ mode: 'off', queue_depth: 0, queue_capacity: 4096, submitted: 0, dropped: 0, processed: 1, failed: 0, allowed: 0, flagged: 0, blocked: 1, unavailable: 0, invalid: 0 })
  })

  afterEach(() => { document.body.innerHTML = '' })

  it('loads privacy-safe events and opens configuration drawer', async () => {
    const wrapper = mount(PromptAuditPanel, { attachTo: document.body, global: { plugins: [ElementPlus] } })
    await flushPromises()
    expect(api.getPromptAuditConfig).toHaveBeenCalledOnce()
    expect(api.listPromptAuditEvents).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('danger***…')
    expect(wrapper.text()).toContain('jailbreak')
    const configButton = wrapper.findAllComponents(DsButton).find((button) => button.text() === '配置')
    await configButton!.trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('Guard 出站默认只允许 HTTPS 公网目标')
    wrapper.unmount()
  })
})

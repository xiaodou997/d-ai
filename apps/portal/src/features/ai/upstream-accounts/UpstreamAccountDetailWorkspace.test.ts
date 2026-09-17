import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import UpstreamAccountDetailWorkspace from './UpstreamAccountDetailWorkspace.vue'

const api = vi.hoisted(() => ({
  listPriceBooks: vi.fn(),
  listUpstreamAccounts: vi.fn(),
  updateUpstreamAccountStatus: vi.fn(),
  deleteUpstreamAccount: vi.fn()
}))
vi.mock('@/api/aiAdmin', () => ({ aiAdminApi: api }))
vi.mock('@/features/ai/upstream-model-bindings/UpstreamModelBindingsPanel.vue', () => ({ default: { name: 'UpstreamModelBindingsPanel', template: '<div class="model-panel">模型面板</div>' } }))
vi.mock('@/features/ai/upstream-stability/UpstreamStabilityPanel.vue', () => ({ default: {
  name: 'UpstreamStabilityPanel',
  props: ['window'],
  emits: ['snapshot', 'update:window'],
  template: '<div class="stability-panel">运行诊断</div>'
} }))
vi.mock('@/platform', () => ({
  PortalPagePanel: defineComponent({ template: '<section><slot name="actions"/><slot/></section>' }),
  PortalContentCard: defineComponent({ template: '<section><slot name="actions"/><slot/></section>' })
}))

const SlotStub = defineComponent({ template: '<div><slot/><slot name="dropdown"/></div>' })
const StabilityStub = defineComponent({
  name: 'UpstreamStabilityPanel',
  props: { window: { type: String, default: '24h' } },
  emits: ['snapshot', 'update:window'],
  setup(_props, { emit }) {
    emit('snapshot', { resource_id: 'account-1', window: '24h', availability: 'partial', success_rate: 75, samples: 12, models: [{ endpoint_id: 'endpoint-1', outcome: 'success', count: 3 }, { endpoint_id: 'endpoint-1', outcome: 'timeout', count: 1 }] })
  },
  template: '<div class="stability-panel">运行诊断</div>'
})

const account = {
  id: 'account-1', name: 'Primary upstream', description: 'Main provider', tenant_display_name: 'Provider',
  tenant_access_mode: 'public', tenant_multiplier: 1, price_book_id: 'book-1', status: 'active',
  endpoints: [{ id: 'endpoint-1', api_format: 'openai_responses', base_url: 'https://api.example.com/v1', path_override: '', status: 'active' }]
}

async function mountDetail(path = '/admin/ai/upstreams/accounts/account-1') {
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/admin/ai/upstreams/accounts', component: { template: '<div class="list-page" />' } },
    { path: '/admin/ai/upstreams/accounts/:accountId', component: UpstreamAccountDetailWorkspace }
  ] })
  await router.push(path); await router.isReady()
  const wrapper = mount(UpstreamAccountDetailWorkspace, {
    global: {
      plugins: [router], directives: { loading: {} },
      stubs: {
        ElButton: defineComponent({ emits: ['click'], template: '<button @click="$emit(\'click\')"><slot/></button>' }),
        ElAlert: true, ElDropdown: SlotStub, ElDropdownMenu: SlotStub, ElDropdownItem: SlotStub,
        ElDescriptions: SlotStub, ElDescriptionsItem: SlotStub, ElTable: SlotStub, ElTableColumn: true,
        ElDialog: true, ElForm: SlotStub, ElFormItem: SlotStub, ElSelect: SlotStub, ElOption: true,
        ElInput: true, ElRadioGroup: SlotStub, ElRadio: SlotStub,
        UpstreamAccountStatusControl: true, UpstreamAccountEditorDialog: true, UpstreamAccountTestDialog: true,
        UpstreamModelBindingsPanel: defineComponent({ template: '<div class="model-panel">模型面板</div>' }),
        UpstreamStabilityPanel: StabilityStub,
        KeyValueEditor: true
      }
    }
  })
  await flushPromises()
  return { wrapper, router }
}

describe('UpstreamAccountDetailWorkspace', () => {
  beforeEach(() => {
    api.listUpstreamAccounts.mockReset().mockResolvedValue({ items: [account] })
    api.listPriceBooks.mockReset().mockResolvedValue({ items: [{ id: 'book-1', name: 'Base price', status: 'active' }] })
    api.updateUpstreamAccountStatus.mockReset().mockResolvedValue({})
    api.deleteUpstreamAccount.mockReset().mockResolvedValue({})
  })

  it('shows account identity, success rate and four focused detail tabs', async () => {
    const { wrapper } = await mountDetail()
    expect(wrapper.text()).toContain('Primary upstream')
    expect(wrapper.text()).toContain('成功率 75.0%')
    expect(wrapper.text()).toContain('样本较少')
    expect(wrapper.findAll('[role="tab"]').map(tab => tab.text())).toEqual(['账号概览', '请求端点', '模型绑定', '运行诊断'])
    wrapper.unmount()
  })

  it('restores the selected tab and window from the URL', async () => {
    const { wrapper, router } = await mountDetail('/admin/ai/upstreams/accounts/account-1?tab=runtime&window=1h')
    expect(wrapper.get('.stability-panel').isVisible()).toBe(true)
    expect(wrapper.getComponent(StabilityStub).props('window')).toBe('1h')
    await wrapper.findAll('[role="tab"]').find(tab => tab.text() === '模型绑定')!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.tab).toBe('models')
    expect(router.currentRoute.value.query.window).toBe('1h')
    wrapper.unmount()
  })

  it('shows an explicit empty state for an unknown account', async () => {
    const { wrapper } = await mountDetail('/admin/ai/upstreams/accounts/missing')
    expect(wrapper.text()).toContain('上游账号不存在')
    expect(wrapper.text()).not.toContain('Primary upstream')
    wrapper.unmount()
  })

  it('returns to the exact originating list URL', async () => {
    const from = encodeURIComponent('/admin/ai/upstreams/accounts?q=Primary&page=2')
    const { wrapper, router } = await mountDetail(`/admin/ai/upstreams/accounts/account-1?from=${from}`)
    await wrapper.findAll('button').find(button => button.text() === '返回列表')!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/admin/ai/upstreams/accounts?q=Primary&page=2')
    wrapper.unmount()
  })
})

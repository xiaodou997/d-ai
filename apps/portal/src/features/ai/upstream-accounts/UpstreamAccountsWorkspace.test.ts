import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import UpstreamAccountsWorkspace from './UpstreamAccountsWorkspace.vue'
import { stabilityApi } from '@/features/ai/upstream-stability/api'

const api = vi.hoisted(() => ({
  listPriceBooks: vi.fn(),
  listUpstreamAccounts: vi.fn(),
  previewImportUpstreamAccounts: vi.fn(),
  updateUpstreamAccountStatus: vi.fn()
}))

vi.mock('@/api/aiAdmin', () => ({ aiAdminApi: api }))
vi.mock('@/features/ai/upstream-stability/api', () => ({ stabilityApi: { list: vi.fn(), detail: vi.fn(), resume: vi.fn() } }))
vi.mock('@/platform', () => ({
  PortalPagePanel: defineComponent({ template: '<section><slot name="actions"/><slot name="filters"/><slot/><slot name="pagination"/></section>' })
}))

const SlotStub = defineComponent({ template: '<div><slot/><slot name="prefix"/></div>' })
const InputStub = defineComponent({
  props: { modelValue: { type: String, default: '' } },
  emits: ['update:modelValue'],
  template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />'
})
const SelectStub = defineComponent({
  props: { modelValue: { type: [String, Number], default: '' } },
  emits: ['update:modelValue'],
  template: '<div class="select-stub"><slot/></div>'
})
const StatusStub = defineComponent({
  props: { status: { type: String, default: '' } },
  template: '<span class="status-stub">{{ status }}</span>'
})

function account(index: number) {
  return {
    id: `account-${index}`,
    name: `Account ${index}`,
    tenant_display_name: `Display ${index}`,
    tenant_access_mode: index % 2 ? 'public' : 'restricted',
    tenant_multiplier: 1,
    status: 'active',
    endpoints: [{ id: `endpoint-${index}`, api_format: 'openai_responses', base_url: `https://api-${index}.example.com`, status: 'active' }]
  }
}

async function mountPage(path = '/admin/ai/upstreams/accounts') {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/admin/ai/upstreams/accounts', component: UpstreamAccountsWorkspace }, { path: '/admin/ai/upstreams/accounts/:accountId', component: { template: '<div />' } }] })
  await router.push(path)
  await router.isReady()
  const wrapper = mount(UpstreamAccountsWorkspace, {
    global: {
      plugins: [router],
      directives: { loading: {} },
      stubs: {
        ElButton: defineComponent({ emits: ['click'], template: '<button @click="$emit(\'click\')"><slot/></button>' }),
        ElInput: InputStub,
        ElSelect: SelectStub,
        ElOption: true,
        ElAlert: true,
        ElDialog: true,
        ElTag: SlotStub,
        ElCheckbox: SlotStub,
        ElForm: SlotStub,
        ElFormItem: SlotStub,
        ElInputNumber: true,
        UpstreamAccountStatusControl: StatusStub,
        UpstreamAccountEditorDialog: true,
        UpstreamAccountTestDialog: true
      }
    }
  })
  await flushPromises()
  return { wrapper, router }
}

describe('UpstreamAccountsWorkspace', () => {
  beforeEach(() => {
    api.listPriceBooks.mockReset().mockResolvedValue({ items: [] })
    api.listUpstreamAccounts.mockReset().mockResolvedValue({ items: [account(1), account(2)] })
    api.previewImportUpstreamAccounts.mockReset()
    api.updateUpstreamAccountStatus.mockReset().mockResolvedValue({})
    vi.mocked(stabilityApi.list).mockReset().mockResolvedValue({ items: [{ resource_id: 'account-1', window: '24h', availability: 'partial', success_rate: 80, samples: 10 }] } as any)
  })

  it('renders a full-width account table with separate config, runtime and success-rate information', async () => {
    const { wrapper } = await mountPage()
    expect(wrapper.get('table').attributes('aria-label')).toBe('上游账号列表')
    expect(wrapper.text()).toContain('Account 1')
    expect(wrapper.text()).toContain('成功率 80.0%')
    expect(wrapper.text()).toContain('样本较少')
    expect(wrapper.text()).toContain('部分受限')
    expect(wrapper.find('.account-content-column').exists()).toBe(false)
    wrapper.unmount()
  })

  it('filters by name and persists filters in the URL', async () => {
    const { wrapper, router } = await mountPage()
    await wrapper.get('input').setValue('api-2.example.com')
    await flushPromises()
    expect(wrapper.text()).not.toContain('Account 1')
    expect(wrapper.text()).toContain('Account 2')
    expect(router.currentRoute.value.query.q).toBe('api-2.example.com')
    wrapper.findAllComponents(SelectStub)[0]!.vm.$emit('update:modelValue', 'disabled')
    await flushPromises()
    expect(router.currentRoute.value.query.status).toBe('disabled')
    wrapper.unmount()
  })

  it('keeps selections when moving between client-side pages', async () => {
    api.listUpstreamAccounts.mockResolvedValue({ items: Array.from({ length: 25 }, (_, index) => account(index + 1)) })
    const { wrapper } = await mountPage()
    const rowCheckboxes = wrapper.findAll('tbody input[type="checkbox"]')
    await rowCheckboxes[0]!.setValue(true)
    await wrapper.get('button[aria-label="下一页"]').trigger('click')
    await flushPromises()
    await wrapper.findAll('tbody input[type="checkbox"]')[0]!.setValue(true)
    expect(wrapper.findAll('button').find(button => button.text().includes('导出'))!.text()).toContain('2')
    wrapper.unmount()
  })

  it('opens a dedicated detail URL and preserves the list query as the return target', async () => {
    const { wrapper, router } = await mountPage('/admin/ai/upstreams/accounts?q=Account&runtime=partial')
    await wrapper.findAll('button').find(button => button.text() === '查看详情')!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/admin/ai/upstreams/accounts/account-1')
    expect(router.currentRoute.value.query.from).toBe('/admin/ai/upstreams/accounts?q=Account&runtime=partial')
    wrapper.unmount()
  })
})

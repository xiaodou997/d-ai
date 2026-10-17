import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import CredentialPoolsWorkspace from './CredentialPoolsWorkspace.vue'

const api = vi.hoisted(() => ({ listCredentialPools: vi.fn(), listPriceBooks: vi.fn(), listPoolCredentials: vi.fn(), createPoolCredential: vi.fn(), getPoolAvailableModels: vi.fn() }))
vi.mock('@/api/aiAdmin', () => ({ aiAdminApi: api }))
vi.mock('@/features/ai/upstream-stability/useUpstreamStability', async () => {
  const { ref } = await import('vue')
  return { useUpstreamStability: () => ({ items: ref([]) }) }
})
vi.mock('@/features/ai/upstream-stability/UpstreamStabilityPanel.vue', () => ({ default: { template: '<div/>' } }))
vi.mock('@/features/ai/upstream-stability/StabilityBadge.vue', () => ({ default: { template: '<div/>' } }))
vi.mock('@/features/ai/upstream-model-bindings/UpstreamModelBindingsPanel.vue', () => ({ default: { template: '<div/>' } }))
vi.mock('./PoolAccountEditorDialog.vue', () => ({ default: { template: '<div/>' } }))
const Slot = defineComponent({ template: '<section><slot name="actions"/><slot/><slot name="footer"/></section>' })
vi.mock('@/platform', () => ({ PortalPagePanel: { template: '<main><slot/></main>' }, PortalContentCard: { template: '<section><slot name="actions"/><slot/></section>' } }))
const pools = ['A', 'B'].map(id => ({ id, name: `Pool ${id}`, fixed_provider_type: 'codex', status: 'active', oauth_strategy: 'round_robin' }))
const deferred = <T,>() => { let resolve!: (value: T) => void; const promise = new Promise<T>(done => { resolve = done }); return { promise, resolve } }
async function mountWorkspace() {
  const wrapper = mount(CredentialPoolsWorkspace, { global: { directives: { loading: {} }, stubs: {
    DsTable: defineComponent({ props: ['rows'], template: '<div class="credentials"><div v-for="row in rows" :key="row.id" :data-id="row.id">{{ row.name }}<slot name="cell-models" :row="row"/></div></div>' }),
    DsTag: Slot, DsEmpty: Slot, DsNumberInput: true,
    ElDialog: defineComponent({ props: ['modelValue', 'title'], template: '<section v-if="modelValue" :aria-label="title"><slot/><slot name="footer"/></section>' }),
    ElTabs: Slot, ElTabPane: Slot, ElForm: Slot, ElFormItem: Slot, ElSelect: Slot, ElOption: true,
    ElInput: true, ElDatePicker: true, ElSwitch: true, ElTag: Slot,
    ElButton: defineComponent({ props: ['disabled'], emits: ['click'], template: '<button :disabled="disabled" @click="$emit(\'click\')"><slot/></button>' })
  } } })
  await flushPromises()
  return wrapper
}
describe('credential pool workspace', () => {
  beforeEach(() => {
    api.listCredentialPools.mockReset().mockResolvedValue({ items: pools })
    api.listPriceBooks.mockReset().mockResolvedValue({ items: [] })
    api.listPoolCredentials.mockReset().mockResolvedValue({ items: [] })
    api.createPoolCredential.mockReset().mockResolvedValue({})
    api.getPoolAvailableModels.mockReset().mockResolvedValue({ models: [], accounts: [] })
  })
  it('ignores late credentials from an earlier pool selection', async () => {
    const old = deferred<{ items: { id: string; name: string }[] }>()
    api.listPoolCredentials.mockImplementation((id: string) => id === 'A' ? old.promise : Promise.resolve({ items: [{ id: 'b', name: 'B credential' }] }))
    const wrapper = await mountWorkspace()
    await wrapper.findAll('.pool-item')[0].trigger('click')
    await wrapper.findAll('.pool-item')[1].trigger('click'); await flushPromises()
    expect(wrapper.get('.credentials').text()).toContain('B credential')
    old.resolve({ items: [{ id: 'a', name: 'A credential' }] }); await flushPromises()
    expect(wrapper.get('.credentials').text()).toContain('B credential')
    expect(wrapper.get('.credentials').text()).not.toContain('A credential')
    wrapper.unmount()
  })
  it('ignores a quota refresh that finishes after switching pools', async () => {
    const old = deferred<{ items: { id: string; name: string }[] }>()
    api.listPoolCredentials.mockImplementation((id: string, refresh?: boolean) => refresh ? old.promise : Promise.resolve({ items: [{ id: id.toLowerCase(), name: `Account ${id}` }] }))
    const wrapper = await mountWorkspace()
    await wrapper.findAll('.pool-item')[0].trigger('click'); await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '更新上游配额')!.trigger('click')
    expect(api.listPoolCredentials).toHaveBeenLastCalledWith('A', true)
    await wrapper.findAll('.pool-item')[1].trigger('click'); await flushPromises()
    old.resolve({ items: [{ id: 'a', name: 'Wrong pool quota response' }] }); await flushPromises()
    expect(wrapper.get('.credentials').text()).toContain('Account B')
    expect(wrapper.get('.credentials').text()).not.toContain('Wrong pool quota response')
    wrapper.unmount()
  })
  it('keeps successful imports out of a partial-failure retry', async () => {
    api.createPoolCredential.mockResolvedValueOnce({}).mockRejectedValueOnce(new Error('temporary failure')).mockResolvedValueOnce({})
    const wrapper = await mountWorkspace()
    await wrapper.findAll('.pool-item')[0].trigger('click'); await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '导入凭据')!.trigger('click')
    const file = new File(['unused'], 'accounts.json', { type: 'application/json' })
    Object.defineProperty(file, 'text', { value: async () => JSON.stringify([{ access_token: 'one' }, { access_token: 'two' }]) })
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
    await input.trigger('change'); await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '导入 2 条凭据')!.trigger('click'); await flushPromises()
    expect(api.createPoolCredential).toHaveBeenCalledTimes(2)
    await wrapper.findAll('button').find(button => button.text() === '导入 1 条凭据')!.trigger('click'); await flushPromises()
    expect(api.createPoolCredential).toHaveBeenCalledTimes(3)
    expect(api.createPoolCredential.mock.calls[2][0]).toBe('A')
    expect(api.createPoolCredential.mock.calls[2][1].access_token).toBe('two')
    wrapper.unmount()
  })

  it('shows independent model observations after explicit discovery', async () => {
    api.listPoolCredentials.mockResolvedValue({ items: [{ id: 'a', name: 'Account A' }, { id: 'b', name: 'Account B' }] })
    api.getPoolAvailableModels.mockResolvedValue({ models: ['model-a', 'model-b'], accounts: [
      { credential_id: 'a', models: ['model-a', 'model-b'], source: 'live', observed_at: Date.now(), valid_until: Date.now() + 60000 },
      { credential_id: 'b', models: [], source: 'unknown' }
    ] })
    const wrapper = await mountWorkspace()
    await wrapper.findAll('.pool-item')[0].trigger('click'); await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '识别账号模型')!.trigger('click'); await flushPromises()
    expect(api.getPoolAvailableModels).toHaveBeenCalledWith('A', true)
    expect(wrapper.get('[data-id="a"]').text()).toContain('已确认 2 个模型')
    expect(wrapper.get('[data-id="a"]').text()).toContain('model-a、model-b')
    expect(wrapper.get('[data-id="b"]').text()).toContain('模型权限未知')
    expect(wrapper.get('[data-id="b"]').text()).not.toContain('model-a')
    wrapper.unmount()
  })

  it('ignores a model observation that finishes after switching pools', async () => {
    const old = deferred<{ accounts: { credential_id: string; source: string; models: string[]; valid_until: number }[] }>()
    api.getPoolAvailableModels.mockReturnValue(old.promise)
    api.listPoolCredentials.mockImplementation((id: string) => Promise.resolve({ items: [{ id: 'same-id', name: `Account ${id}` }] }))
    const wrapper = await mountWorkspace()
    await wrapper.findAll('.pool-item')[0].trigger('click'); await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '识别账号模型')!.trigger('click')
    await wrapper.findAll('.pool-item')[1].trigger('click'); await flushPromises()
    old.resolve({ accounts: [{ credential_id: 'same-id', source: 'live', models: ['wrong-pool-model'], valid_until: Date.now() + 60000 }] }); await flushPromises()
    expect(wrapper.get('.credentials').text()).toContain('Account B')
    expect(wrapper.get('.credentials').text()).not.toContain('wrong-pool-model')
    wrapper.unmount()
  })
})

import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import UpstreamAccountTestDialog from './UpstreamAccountTestDialog.vue'

const api = vi.hoisted(() => ({ listAccountModelBindings: vi.fn(), testUpstreamAccount: vi.fn() }))
vi.mock('@/api/aiAdmin', () => ({ aiAdminApi: api }))

const SlotStub = defineComponent({ template: '<div><slot/></div>' })
const DialogStub = defineComponent({ props: { modelValue: Boolean }, template: '<section v-if="modelValue"><slot/><slot name="footer"/></section>' })
const ButtonStub = defineComponent({ props: { disabled: Boolean }, emits: ['click'], template: '<button :disabled="disabled" @click="$emit(\'click\')"><slot/></button>' })
const global = { stubs: {
  ElDialog: DialogStub, ElForm: SlotStub, ElFormItem: SlotStub, ElSelect: SlotStub, ElOption: true,
  ElInput: true, ElRadioGroup: SlotStub, ElRadio: SlotStub, ElButton: ButtonStub, ElIcon: SlotStub, ElTag: SlotStub,
  UpstreamImageTestUpload: true
} }
const account = { id: 'account-1', name: 'Test', status: 'active', endpoints: [{ id: 'ep', api_format: 'openai_responses', base_url: 'https://example.com', status: 'active' }] }

describe('UpstreamAccountTestDialog', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    api.listAccountModelBindings.mockReset().mockResolvedValue({ items: [{ model_code: 'gpt-test', capability_type: 'chat' }] })
    api.testUpstreamAccount.mockReset()
  })
  afterEach(() => vi.useRealTimers())

  it('shows elapsed wait time and prevents duplicate test submissions', async () => {
    let resolve!: (value: unknown) => void
    api.testUpstreamAccount.mockReturnValue(new Promise(done => { resolve = done }))
    const wrapper = mount(UpstreamAccountTestDialog, { props: { modelValue: false, account: account as any }, global })
    await wrapper.setProps({ modelValue: true }); await flushPromises()
    const run = wrapper.findAll('button').find(button => button.text() === '开始测试')!
    await run.trigger('click'); await run.trigger('click')
    vi.advanceTimersByTime(2100); await flushPromises()
    expect(api.testUpstreamAccount).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('已等待 2 秒')
    expect(wrapper.text()).toContain('暂时无法判断上游处理阶段')
    resolve({ ok: false, http_status: 200, latency_ms: 2100, api_format: 'openai_responses', upstream_model: 'gpt-test', error: 'rate limited' })
    await flushPromises()
    expect(wrapper.text()).toContain('测试失败')
    expect(wrapper.text()).toContain('rate limited')
  })
})

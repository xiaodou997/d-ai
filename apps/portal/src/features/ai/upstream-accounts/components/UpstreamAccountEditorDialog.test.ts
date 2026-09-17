import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import UpstreamAccountEditorDialog from './UpstreamAccountEditorDialog.vue'

const api = vi.hoisted(() => ({ createUpstreamAccount: vi.fn(), updateUpstreamAccount: vi.fn() }))
vi.mock('@/api/aiAdmin', () => ({ aiAdminApi: api }))

const SlotStub = defineComponent({ template: '<div><slot/></div>' })
const DialogStub = defineComponent({
  props: { modelValue: Boolean, title: String },
  emits: ['update:modelValue'],
  template: '<section v-if="modelValue" :data-title="title"><slot/><slot name="footer"/></section>'
})
const InputStub = defineComponent({
  props: { modelValue: { type: String, default: '' }, placeholder: String }, emits: ['update:modelValue'],
  template: '<input :value="modelValue" :placeholder="placeholder" @input="$emit(\'update:modelValue\', $event.target.value)" />'
})

const global = { stubs: {
  ElDialog: DialogStub, ElForm: SlotStub, ElFormItem: SlotStub,
  ElInput: InputStub, ElSelect: SlotStub, ElOption: true, ElSwitch: true,
  ElButton: defineComponent({ emits: ['click'], template: '<button @click="$emit(\'click\')"><slot/></button>' }),
  ElAlert: true, KeyValueEditor: true, DsNumberInput: true
} }

describe('UpstreamAccountEditorDialog', () => {
  beforeEach(() => {
    api.createUpstreamAccount.mockReset().mockResolvedValue({ id: 'created', name: 'Created' })
    api.updateUpstreamAccount.mockReset().mockResolvedValue({ id: 'existing', name: 'Updated' })
  })

  it('creates an account with the default Responses endpoint and first active price book', async () => {
    const wrapper = mount(UpstreamAccountEditorDialog, { props: { modelValue: false, priceBooks: [{ id: 'disabled', name: 'Old', status: 'disabled' }, { id: 'active', name: 'Base', status: 'active' }] as any }, global })
    await wrapper.setProps({ modelValue: true })
    await wrapper.get('input[placeholder="如 OpenAI 官方 / 某中转"]').setValue('OpenAI')
    await wrapper.get('input[placeholder="https://api.example.com"]').setValue('https://api.openai.com')
    await wrapper.get('input[placeholder="输入上游 API Key（密文存储）"]').setValue('secret')
    await wrapper.findAll('button').find(button => button.text() === '保存')!.trigger('click')
    await flushPromises()
    expect(api.createUpstreamAccount).toHaveBeenCalledWith(expect.objectContaining({
      name: 'OpenAI', price_book_id: 'active',
      endpoints: [expect.objectContaining({ api_format: 'openai_responses', base_url: 'https://api.openai.com' })]
    }))
  })

  it('edits account metadata without overwriting endpoints or managed status', async () => {
    const existing = { id: 'existing', name: 'Existing', description: 'Before', tenant_display_name: 'Existing', tenant_access_mode: 'public', tenant_multiplier: 1, status: 'invalid', endpoints: [{ id: 'ep', api_format: 'openai_responses', base_url: 'https://example.com', status: 'active' }] }
    const wrapper = mount(UpstreamAccountEditorDialog, { props: { modelValue: false, account: existing as any, priceBooks: [] }, global })
    await wrapper.setProps({ modelValue: true })
    await wrapper.get('input[placeholder="给租户展示的一句话说明（可选）"]').setValue('After')
    await wrapper.findAll('button').find(button => button.text() === '保存')!.trigger('click')
    await flushPromises()
    expect(api.updateUpstreamAccount).toHaveBeenCalledWith('existing', expect.objectContaining({ description: 'After' }))
    const body = api.updateUpstreamAccount.mock.calls[0]![1]
    expect(body).not.toHaveProperty('endpoints')
    expect(body).not.toHaveProperty('status')
  })
})

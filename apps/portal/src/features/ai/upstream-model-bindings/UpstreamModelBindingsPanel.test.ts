import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import UpstreamModelBindingsPanel from './UpstreamModelBindingsPanel.vue'

const api = vi.hoisted(() => ({
  listAccountModelBindings: vi.fn(),
  listPoolModelBindings: vi.fn()
}))

const batchApi = vi.hoisted(() => ({
  deleteAccountBindings: vi.fn(),
  deletePoolBindings: vi.fn()
}))

const messages = vi.hoisted(() => ({
  confirm: vi.fn(),
  error: vi.fn(),
  success: vi.fn(),
  warning: vi.fn()
}))

vi.mock('@/api/aiAdmin', () => ({ aiAdminApi: api }))
vi.mock('@/features/ai/upstream-model-bindings/api', () => ({
  upstreamModelBindingBatchApi: batchApi
}))
vi.mock('element-plus', () => ({
  ElMessage: {
    error: messages.error,
    success: messages.success,
    warning: messages.warning
  },
  ElMessageBox: { confirm: messages.confirm }
}))

const bindings = [
  {
    id: '00000000-0000-0000-0000-000000000101',
    model_code: 'gpt-5',
    capability_type: 'chat',
    api_format: 'openai_responses',
    upstream_model_name: 'gpt-5',
    status: 'active'
  },
  {
    id: '00000000-0000-0000-0000-000000000102',
    model_code: 'gpt-image-1',
    capability_type: 'image',
    api_format: 'openai_images',
    upstream_model_name: 'gpt-image-1',
    status: 'active'
  }
]

const ElTableStub = defineComponent({
  props: { data: { type: Array, default: () => [] } },
  emits: ['selection-change'],
  methods: { clearSelection() {} },
  template: `
    <div>
      <button data-test="select-bindings" @click="$emit('selection-change', data)">select</button>
    </div>
  `
})

const ElButtonStub = defineComponent({
  props: { disabled: Boolean, loading: Boolean },
  emits: ['click'],
  template: '<button :disabled="disabled || loading" @click="$emit(\'click\')"><slot /></button>'
})

const global = {
  directives: { loading: {} },
  stubs: {
    ElAlert: true,
    ElButton: ElButtonStub,
    ElCheckbox: true,
    ElDialog: true,
    ElEmpty: true,
    ElForm: true,
    ElFormItem: true,
    ElInput: true,
    ElInputNumber: true,
    ElOption: true,
    ElOptionGroup: true,
    ElRadio: true,
    ElRadioGroup: true,
    ElSelect: true,
    ElSwitch: true,
    ElTable: ElTableStub,
    ElTableColumn: true,
    ElTag: true
  }
}

describe('UpstreamModelBindingsPanel feature batch deletion', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    messages.confirm.mockResolvedValue('confirm')
    api.listAccountModelBindings
      .mockResolvedValueOnce({ items: bindings, total: bindings.length })
      .mockResolvedValue({ items: [], total: 0 })
    api.listPoolModelBindings
      .mockResolvedValueOnce({ items: bindings, total: bindings.length })
      .mockResolvedValue({ items: [], total: 0 })
    batchApi.deleteAccountBindings.mockResolvedValue({ deleted: bindings.length })
    batchApi.deletePoolBindings.mockResolvedValue({ deleted: bindings.length })
  })

  it('deletes selected account bindings in one request', async () => {
    const wrapper = mount(UpstreamModelBindingsPanel, {
      props: { targetKind: 'account', targetId: 'account-1' },
      global
    })
    await flushPromises()

    await wrapper.get('[data-test="select-bindings"]').trigger('click')
    await wrapper.get('[data-test="batch-delete-bindings"]').trigger('click')
    await flushPromises()

    expect(messages.confirm).toHaveBeenCalledWith(
      expect.stringContaining('2 条'),
      '批量删除模型绑定',
      expect.objectContaining({ type: 'warning' })
    )
    expect(batchApi.deleteAccountBindings).toHaveBeenCalledWith('account-1', bindings.map((item) => item.id))
    expect(messages.success).toHaveBeenCalledWith('已删除 2 条模型绑定')
    expect(wrapper.find('[data-test="batch-delete-bindings"]').exists()).toBe(false)
  })

  it('uses the pool batch endpoint for credential pools', async () => {
    const wrapper = mount(UpstreamModelBindingsPanel, {
      props: { targetKind: 'pool', targetId: 'pool-1' },
      global
    })
    await flushPromises()

    await wrapper.get('[data-test="select-bindings"]').trigger('click')
    await wrapper.get('[data-test="batch-delete-bindings"]').trigger('click')
    await flushPromises()

    expect(batchApi.deletePoolBindings).toHaveBeenCalledWith('pool-1', bindings.map((item) => item.id))
    expect(wrapper.find('[data-test="batch-delete-bindings"]').exists()).toBe(false)
  })
})

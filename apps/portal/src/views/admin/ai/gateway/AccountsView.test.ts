import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AccountsView from './AccountsView.vue'

const api = vi.hoisted(() => ({
  createUpstreamAccount: vi.fn(),
  listAccountModelBindings: vi.fn(),
  listLinkedGroupsByTarget: vi.fn(),
  listPriceBooks: vi.fn(),
  listUpstreamAccounts: vi.fn(),
  previewImportUpstreamAccounts: vi.fn(),
  testUpstreamAccount: vi.fn(),
  updateUpstreamAccount: vi.fn(),
  updateUpstreamAccountStatus: vi.fn()
}))

vi.mock('../../../api/aiAdmin', () => ({ aiAdminApi: api }))
vi.mock('../../../features/ai/upstream-model-bindings', () => ({
  useModelBindingBatchDelete: vi.fn()
}))
vi.mock('@dai/app-core', () => ({
  PortalContentCard: { template: '<div><slot name="header" /><slot name="actions" /><slot /></div>' },
  PortalPagePanel: { template: '<div><slot name="actions" /><slot name="filters" /><slot /><slot name="pagination" /></div>' }
}))

const SlotStub = defineComponent({
  template: '<div><slot name="header" /><slot name="actions" /><slot /></div>'
})

const ElDialogStub = defineComponent({
  props: {
    modelValue: { type: Boolean, default: false },
    title: { type: String, default: '' }
  },
  template: '<section v-if="modelValue" :data-dialog-title="title"><slot /><slot name="footer" /></section>'
})

const ElInputStub = defineComponent({
  props: {
    modelValue: { type: String, default: '' },
    placeholder: { type: String, default: '' }
  },
  emits: ['update:modelValue'],
  template: '<input :value="modelValue" :placeholder="placeholder" @input="$emit(\'update:modelValue\', $event.target.value)" />'
})

const ElButtonStub = defineComponent({
  emits: ['click'],
  template: '<button @click="$emit(\'click\')"><slot /></button>'
})

const ElSelectStub = defineComponent({
  props: { placeholder: { type: String, default: '' } },
  template: '<div class="select-placeholder">{{ placeholder }}<slot /></div>'
})

const ElRadioGroupStub = defineComponent({
  props: { modelValue: { type: [String, Boolean], default: '' } },
  emits: ['update:modelValue'],
  template: '<div><slot /></div>'
})

const UpstreamModelBindingsPanelStub = defineComponent({
  name: 'UpstreamModelBindingsPanelStub',
  props: { defaultBindingProtocol: { type: String, default: '' } },
  template: '<div />'
})

const global = {
  directives: { loading: {} },
  stubs: {
    PortalPagePanel: SlotStub,
    PortalContentCard: SlotStub,
    UpstreamModelBindingsPanel: UpstreamModelBindingsPanelStub,
    KeyValueEditor: true,
    ElDialog: ElDialogStub,
    ElButton: ElButtonStub,
    ElRow: SlotStub,
    ElCol: SlotStub,
    ElForm: SlotStub,
    ElFormItem: SlotStub,
    ElCollapse: SlotStub,
    ElCollapseItem: SlotStub,
    ElInput: ElInputStub,
    ElSelect: ElSelectStub,
    ElOption: true,
    ElRadioGroup: ElRadioGroupStub,
    ElRadio: SlotStub,
    ElDescriptions: SlotStub,
    ElDescriptionsItem: SlotStub,
    ElInputNumber: true,
    ElCheckbox: SlotStub,
    ElCheckboxGroup: SlotStub,
    ElSwitch: true,
    ElEmpty: true,
    ElAlert: true,
    ElTable: SlotStub,
    ElTableColumn: true,
    ElTag: SlotStub,
    ElIcon: SlotStub,
    ElTooltip: SlotStub
  }
}

describe('AccountsView', () => {
  beforeEach(() => {
    api.createUpstreamAccount.mockReset().mockResolvedValue({ id: 'account-1' })
    api.listAccountModelBindings.mockReset().mockResolvedValue({ items: [], total: 0 })
    api.listLinkedGroupsByTarget.mockReset().mockResolvedValue({ items: [], total: 0 })
    api.listUpstreamAccounts.mockReset().mockResolvedValue({ items: [] })
    api.previewImportUpstreamAccounts.mockReset().mockResolvedValue({
      items: [],
      summary: {
        create_accounts: 1,
        skip_accounts: 0,
        error_accounts: 0,
        create_model_bindings: 0,
        skip_model_bindings: 0
      }
    })
    api.testUpstreamAccount.mockReset().mockResolvedValue({
      ok: true,
      http_status: 200,
      latency_ms: 100,
      capability: 'image',
      api_format: 'openai_images',
      upstream_model: 'gpt-image-2',
      image_b64: 'aW1n'
    })
    api.updateUpstreamAccount.mockReset().mockResolvedValue({})
    api.updateUpstreamAccountStatus.mockReset().mockResolvedValue({})
    api.listPriceBooks.mockReset().mockResolvedValue({
      items: [
        { id: 'disabled-oldest', name: '停用表', description: '', status: 'disabled' },
        { id: 'active-oldest', name: '基础价格', description: '', status: 'active' },
        { id: 'active-newer', name: '特殊价格', description: '', status: 'active' }
      ]
    })
  })

  it('uses the first active price book when creating an upstream account', async () => {
    const wrapper = mount(AccountsView, { global })
    await flushPromises()

    await wrapper.findAll('button').find((button) => button.text().includes('新增账号'))!.trigger('click')
    const dialog = wrapper.get('[data-dialog-title="新增上游账号"]')
    await dialog.get('input[placeholder="如 OpenAI 官方 / 某中转"]').setValue('OpenAI 官方')
    await dialog.get('input[placeholder="https://api.openai.com"]').setValue('https://api.openai.com')
    await dialog.get('input[placeholder="输入上游 API Key（密文存储）"]').setValue('secret')
    await dialog.findAll('button').find((button) => button.text() === '保存')!.trigger('click')
    await flushPromises()

    expect(api.createUpstreamAccount).toHaveBeenCalledWith(expect.objectContaining({
      name: 'OpenAI 官方',
      price_book_id: 'active-oldest'
    }))
  })

  it('uses concise labels and keeps the API Key update hint in the input', async () => {
    api.listUpstreamAccounts.mockResolvedValue({
      items: [{
        id: 'account-existing',
        name: 'Existing',
        base_url: 'https://existing.example.com',
        default_provider_family: 'openai_compatible',
        status: 'active'
      }]
    })
    const wrapper = mount(AccountsView, { global })
    await flushPromises()

    await wrapper.findAll('button').find((button) => button.text() === '编辑账号')!.trigger('click')
    const dialog = wrapper.get('[data-dialog-title="编辑上游账号"]')

    expect(dialog.get('[label="展示名称"]')).toBeTruthy()
    expect(dialog.find('[label="租户展示名称"]').exists()).toBe(false)
    expect(dialog.get('[label="API Key"]')).toBeTruthy()
    expect(dialog.get('input[placeholder="留空不改；密文存储"]')).toBeTruthy()
    expect(dialog.get('[label="协议"]')).toBeTruthy()
    expect(dialog.find('[label="权重"]').exists()).toBe(false)
    expect(dialog.get('[label="价格表"]')).toBeTruthy()
    expect(dialog.get('[label="租户倍率"]')).toBeTruthy()
    expect(dialog.find('[label="结算价格表"]').exists()).toBe(false)
    expect(dialog.find('[label="租户扣费倍率"]').exists()).toBe(false)
    expect(dialog.findAll('el-option-stub').slice(0, 3).map((option) => option.attributes('label')))
      .toEqual(['OpenAI', 'Anthropic', 'Gemini'])

    await dialog.findAll('button').find((button) => button.text() === '保存')!.trigger('click')
    await flushPromises()
    expect(api.updateUpstreamAccount.mock.calls[0]![1]).not.toHaveProperty('weight')
  })

  it('defaults OpenAI-compatible model bindings to Responses', async () => {
    api.listUpstreamAccounts.mockResolvedValue({
      items: [{
        id: 'account-openai',
        name: 'OpenAI Compatible',
        base_url: 'https://api.example.com',
        default_provider_family: 'openai_compatible',
        status: 'active'
      }]
    })
    const wrapper = mount(AccountsView, { global })
    await flushPromises()

    expect(wrapper.getComponent(UpstreamModelBindingsPanelStub).props('defaultBindingProtocol')).toBe('openai_responses')
  })

  it('uses one switch instead of separate status and stop controls for an active account', async () => {
    api.listUpstreamAccounts.mockResolvedValue({
      items: [{
        id: 'account-active',
        name: 'Active account',
        base_url: 'https://api.example.com',
        default_provider_family: 'openai_compatible',
        status: 'active'
      }]
    })
    const wrapper = mount(AccountsView, { global })
    await flushPromises()

    expect(wrapper.findAll('el-switch-stub')).toHaveLength(1)
    expect(wrapper.findAll('button').filter((button) => button.text() === '停用')).toHaveLength(0)
  })

  it('does not expose tenant group association controls', async () => {
    api.listUpstreamAccounts.mockResolvedValue({
      items: [{
        id: 'account-1',
        name: 'OpenAI 官方',
        base_url: 'https://api.openai.com',
        default_provider_family: 'openai_compatible',
        status: 'active'
      }]
    })
    const wrapper = mount(AccountsView, { global })
    await flushPromises()

    expect(wrapper.text()).not.toContain('关联分组')
    expect(api.listLinkedGroupsByTarget).not.toHaveBeenCalled()
  })

  it('uses the first active price book for an import preview', async () => {
    const wrapper = mount(AccountsView, { global })
    await flushPromises()

    await wrapper.findAll('button').find((button) => button.text() === '导入')!.trigger('click')
    const dialog = wrapper.get('[data-dialog-title="导入上游账号"]')
    const file = new File([
      JSON.stringify({ accounts: [{ name: 'Imported', base_url: 'https://api.example.com', api_key: 'secret' }] })
    ], 'accounts.json', { type: 'application/json' })
    const input = dialog.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
    await input.trigger('change')
    await flushPromises()

    expect(api.previewImportUpstreamAccounts).toHaveBeenCalledWith(expect.objectContaining({
      default_price_book_id: 'active-oldest'
    }))
  })

  it('preserves an existing price book when editing an upstream account', async () => {
    api.listUpstreamAccounts.mockResolvedValue({
      items: [{
        id: 'account-existing',
        name: 'Existing',
        base_url: 'https://existing.example.com',
        default_provider_family: 'openai_compatible',
        price_book_id: 'active-newer',
        status: 'active'
      }]
    })
    const wrapper = mount(AccountsView, { global })
    await flushPromises()

    await wrapper.findAll('button').find((button) => button.text() === '编辑账号')!.trigger('click')
    const dialog = wrapper.get('[data-dialog-title="编辑上游账号"]')
    await dialog.findAll('button').find((button) => button.text() === '保存')!.trigger('click')
    await flushPromises()

    expect(api.updateUpstreamAccount).toHaveBeenCalledWith(
      'account-existing',
      expect.objectContaining({ price_book_id: 'active-newer' })
    )
  })

  it('preserves the system-managed invalid state when editing account details', async () => {
    api.listUpstreamAccounts.mockResolvedValue({
      items: [{
        id: 'account-invalid',
        name: 'Invalid account',
        tenant_display_name: 'Invalid account',
        tenant_access_mode: 'public',
        base_url: 'https://invalid.example.com',
        default_provider_family: 'openai_compatible',
        price_book_id: 'active-oldest',
        status: 'invalid',
        invalid_reason: 'upstream returned HTTP 401'
      }]
    })
    const wrapper = mount(AccountsView, { global })
    await flushPromises()

    expect(wrapper.text()).toContain('失效')
    expect(wrapper.text()).toContain('重新验证')
    expect(wrapper.findAll('el-switch-stub')).toHaveLength(0)
    expect(wrapper.findAll('button').filter((button) => button.text() === '测试连通')).toHaveLength(1)
    await wrapper.findAll('button').find((button) => button.text() === '编辑账号')!.trigger('click')
    const dialog = wrapper.get('[data-dialog-title="编辑上游账号"]')
    await dialog.findAll('button').find((button) => button.text() === '保存')!.trigger('click')
    await flushPromises()

    expect(api.updateUpstreamAccount).toHaveBeenCalledOnce()
    const payload = api.updateUpstreamAccount.mock.calls[0]![1]
    expect(payload).not.toHaveProperty('status')
  })

  it('explains when no active price book is available for a new account', async () => {
    api.listPriceBooks.mockResolvedValue({
      items: [{ id: 'disabled', name: '停用表', description: '', status: 'disabled' }]
    })
    const wrapper = mount(AccountsView, { global })
    await flushPromises()

    await wrapper.findAll('button').find((button) => button.text().includes('新增账号'))!.trigger('click')

    expect(wrapper.get('[data-dialog-title="新增上游账号"]').text()).toContain('暂无启用价格表')
  })

  it('fills the default when price books finish loading after the create dialog opens', async () => {
    let resolvePriceBooks!: (value: { items: Array<Record<string, string>> }) => void
    api.listPriceBooks.mockReturnValue(new Promise((resolve) => { resolvePriceBooks = resolve }))
    const wrapper = mount(AccountsView, { global })
    await flushPromises()

    await wrapper.findAll('button').find((button) => button.text().includes('新增账号'))!.trigger('click')
    const dialog = wrapper.get('[data-dialog-title="新增上游账号"]')
    await dialog.get('input[placeholder="如 OpenAI 官方 / 某中转"]').setValue('慢加载账号')
    await dialog.get('input[placeholder="https://api.openai.com"]').setValue('https://slow.example.com')
    await dialog.get('input[placeholder="输入上游 API Key（密文存储）"]').setValue('secret')

    resolvePriceBooks({
      items: [{ id: 'active-after-load', name: '基础价格', description: '', status: 'active' }]
    })
    await flushPromises()
    await dialog.findAll('button').find((button) => button.text() === '保存')!.trigger('click')
    await flushPromises()

    expect(api.createUpstreamAccount).toHaveBeenCalledWith(expect.objectContaining({
      price_book_id: 'active-after-load'
    }))
  })

  it('refreshes an import preview when the default price book arrives late', async () => {
    let resolvePriceBooks!: (value: { items: Array<Record<string, string>> }) => void
    api.listPriceBooks.mockReturnValue(new Promise((resolve) => { resolvePriceBooks = resolve }))
    const wrapper = mount(AccountsView, { global })
    await flushPromises()

    await wrapper.findAll('button').find((button) => button.text() === '导入')!.trigger('click')
    const dialog = wrapper.get('[data-dialog-title="导入上游账号"]')
    const file = new File([
      JSON.stringify({ accounts: [{ name: 'Imported', base_url: 'https://api.example.com', api_key: 'secret' }] })
    ], 'accounts.json', { type: 'application/json' })
    const input = dialog.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
    await input.trigger('change')
    await flushPromises()

    resolvePriceBooks({
      items: [{ id: 'active-after-load', name: '基础价格', description: '', status: 'active' }]
    })
    await flushPromises()

    expect(api.previewImportUpstreamAccounts).toHaveBeenLastCalledWith(expect.objectContaining({
      default_price_book_id: 'active-after-load'
    }))
  })

  it('ignores a stale import preview that finishes after the priced preview', async () => {
    let resolvePriceBooks!: (value: { items: Array<Record<string, string>> }) => void
    let resolveStalePreview!: (value: Record<string, unknown>) => void
    let resolvePricedPreview!: (value: Record<string, unknown>) => void
    api.listPriceBooks.mockReturnValue(new Promise((resolve) => { resolvePriceBooks = resolve }))
    api.previewImportUpstreamAccounts
      .mockImplementationOnce(() => new Promise((resolve) => { resolveStalePreview = resolve }))
      .mockImplementationOnce(() => new Promise((resolve) => { resolvePricedPreview = resolve }))
    const wrapper = mount(AccountsView, { global })
    await flushPromises()

    await wrapper.findAll('button').find((button) => button.text() === '导入')!.trigger('click')
    const dialog = wrapper.get('[data-dialog-title="导入上游账号"]')
    const file = new File([
      JSON.stringify({ accounts: [{ name: 'Imported', base_url: 'https://api.example.com', api_key: 'secret' }] })
    ], 'accounts.json', { type: 'application/json' })
    const input = dialog.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
    await input.trigger('change')
    await flushPromises()

    resolvePriceBooks({
      items: [{ id: 'active-after-load', name: '基础价格', description: '', status: 'active' }]
    })
    await flushPromises()
    resolvePricedPreview({
      items: [],
      summary: {
        create_accounts: 2,
        skip_accounts: 0,
        error_accounts: 0,
        create_model_bindings: 0,
        skip_model_bindings: 0
      }
    })
    await flushPromises()
    resolveStalePreview({
      items: [],
      summary: {
        create_accounts: 1,
        skip_accounts: 0,
        error_accounts: 0,
        create_model_bindings: 0,
        skip_model_bindings: 0
      }
    })
    await flushPromises()

    expect(dialog.findAll('.import-stats strong')[0]!.text()).toBe('2')
  })

  it('submits the selected image for an image edit connectivity test', async () => {
    api.listUpstreamAccounts.mockResolvedValue({
      items: [{
        id: 'image-account',
        name: 'Image upstream',
        base_url: 'https://images.example.com',
        default_provider_family: 'openai_compatible',
        status: 'active'
      }]
    })
    api.listAccountModelBindings.mockResolvedValue({
      items: [{
        model_code: 'gpt-image-2',
        capability_type: 'image',
        api_format: 'openai_images'
      }],
      total: 1
    })
    const wrapper = mount(AccountsView, { global })
    await flushPromises()

    await wrapper.findAll('button').find((button) => button.text().includes('测试连通'))!.trigger('click')
    await flushPromises()
    const dialog = wrapper.get('[data-dialog-title="测试账号连通"]')
    dialog.getComponent(ElRadioGroupStub).vm.$emit('update:modelValue', true)
    await flushPromises()

    const file = new File([new Uint8Array([137, 80, 78, 71])], 'reference.png', { type: 'image/png' })
    const input = dialog.get('input[accept="image/png,image/jpeg,image/webp"]')
    Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
    await input.trigger('change')
    await flushPromises()
    await dialog.findAll('button').find((button) => button.text().includes('开始测试'))!.trigger('click')
    await flushPromises()

    expect(api.testUpstreamAccount).toHaveBeenCalledWith('image-account', {
      model_code: 'gpt-image-2',
      prompt: undefined,
      image_edit: true,
      image: {
        filename: 'reference.png',
        mime_type: 'image/png',
        b64_json: 'iVBORw=='
      }
    })
  })
})

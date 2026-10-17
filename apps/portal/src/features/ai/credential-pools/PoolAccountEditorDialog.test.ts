import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PoolCredentialDTO } from '@/api/types/ai'
import PoolAccountEditorDialog from './PoolAccountEditorDialog.vue'

const api = vi.hoisted(() => ({ patchPoolCredential: vi.fn() }))
vi.mock('@/api/aiAdmin', () => ({ aiAdminApi: api }))
const Input = defineComponent({ props: ['modelValue'], emits: ['update:modelValue'], template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />' })
const account = { id: 'credential', pool_id: 'pool', provider_type: 'codex', status: 'active', token_type: 'bearer', scope: '', consecutive_fail_count: 0, success_count: 0, fail_count: 0, name: 'Account', email: 'a@example.com', weight: 100, auth_metadata: { account_id: 'workspace', user_id: 'user' } } as PoolCredentialDTO
const mountEditor = () => mount(PoolAccountEditorDialog, { props: { open: true, poolId: 'pool', account }, global: { stubs: {
  DsModal: defineComponent({ props: ['open'], template: '<section v-if="open"><slot/><slot name="footer"/></section>' }),
  ElForm: defineComponent({ template: '<form><slot/></form>' }), ElFormItem: defineComponent({ template: '<div><slot/></div>' }),
  ElInput: Input, DsNumberInput: Input, ElDatePicker: Input,
  ElButton: defineComponent({ emits: ['click'], template: '<button @click="$emit(\'click\')"><slot/></button>' })
} } })
describe('pool account editor', () => {
  beforeEach(() => api.patchPoolCredential.mockReset().mockResolvedValue(account))
  it('saves identity and weight once without submitting blank replacement tokens', async () => {
    const wrapper = mountEditor()
    await wrapper.findAll('input')[0].setValue('Renamed')
    await wrapper.findAll('button').find(button => button.text() === '保存账号')!.trigger('click')
    await flushPromises()
    expect(api.patchPoolCredential).toHaveBeenCalledTimes(1)
    expect(api.patchPoolCredential).toHaveBeenCalledWith('pool', 'credential', expect.objectContaining({ name: 'Renamed', weight: 100, auth_metadata: expect.objectContaining({ account_id: 'workspace' }) }))
    const payload = api.patchPoolCredential.mock.calls[0][2]
    expect(payload).not.toHaveProperty('access_token'); expect(payload).not.toHaveProperty('refresh_token')
    wrapper.unmount()
  })
  it('replaces credentials in place and clears secret inputs when closed', async () => {
    const wrapper = mountEditor()
    await wrapper.findAll('input')[6].setValue('new-access')
    await wrapper.findAll('input')[7].setValue('new-refresh')
    await wrapper.findAll('button').find(button => button.text() === '保存账号')!.trigger('click')
    await flushPromises()
    expect(api.patchPoolCredential.mock.calls[0][2]).toMatchObject({ access_token: 'new-access', refresh_token: 'new-refresh' })
    expect(wrapper.findAll('input')[6].element.value).toBe('')
    expect(wrapper.emitted('saved')).toEqual([['pool']])
    wrapper.unmount()
  })
})

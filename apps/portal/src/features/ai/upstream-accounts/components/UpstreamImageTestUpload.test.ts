import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { describe, expect, it } from 'vitest'

import UpstreamImageTestUpload from './UpstreamImageTestUpload.vue'

const ElButtonStub = defineComponent({
  props: { disabled: Boolean },
  emits: ['click'],
  template: '<button :disabled="disabled" @click="$emit(\'click\')"><slot /></button>'
})

const global = {
  stubs: {
    ElButton: ElButtonStub
  }
}

describe('UpstreamImageTestUpload feature component', () => {
  it('emits the selected image bytes and metadata', async () => {
    const wrapper = mount(UpstreamImageTestUpload, {
      props: { modelValue: null },
      global
    })
    const file = new File([new Uint8Array([137, 80, 78, 71])], 'reference.png', { type: 'image/png' })
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })

    await input.trigger('change')
    await flushPromises()

    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual({
      filename: 'reference.png',
      mime_type: 'image/png',
      b64_json: 'iVBORw=='
    })
    expect(wrapper.text()).toContain('reference.png')
  })

  it('rejects unsupported files before reading them', async () => {
    const wrapper = mount(UpstreamImageTestUpload, {
      props: { modelValue: null },
      global
    })
    const file = new File(['hello'], 'notes.txt', { type: 'text/plain' })
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })

    await input.trigger('change')
    await flushPromises()

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.text()).toContain('仅支持 PNG、JPEG 或 WebP 图片')
  })

  it('uses the production request raw-image limit', async () => {
    const wrapper = mount(UpstreamImageTestUpload, {
      props: { modelValue: null },
      global
    })
    const file = new File(['image'], 'large.png', { type: 'image/png' })
    Object.defineProperty(file, 'size', { configurable: true, value: (32 * 1024 * 1024) + 1 })
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })

    await input.trigger('change')
    await flushPromises()

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.text()).toContain('图片不能超过 32 MiB')
  })
})

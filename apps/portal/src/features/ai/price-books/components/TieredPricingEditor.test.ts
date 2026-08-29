import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { defineComponent } from 'vue'

import type { TokenPriceTier } from '../pricingTypes'
import TieredPricingEditor from './TieredPricingEditor.vue'

const ElButtonStub = defineComponent({
  emits: ['click'],
  template: '<button @click="$emit(\'click\')"><slot /></button>'
})

const global = {
  stubs: {
    ElButton: ElButtonStub,
    ElAlert: { props: ['title'], template: '<div class="alert">{{ title }}</div>' },
    ElInputNumber: true,
    ElSelect: true,
    ElOption: true
  }
}

function tier(upToInputTokens: number | null, input = 3): TokenPriceTier {
  return {
    up_to_input_tokens: upToInputTokens,
    input_per_1m_usd: input,
    output_per_1m_usd: 6,
    cache_write_per_1m_usd: 3.75,
    cache_read_per_1m_usd: 0.3
  }
}

describe('TieredPricingEditor', () => {
  it('copies the terminal price when adding a finite tier', async () => {
    const wrapper = mount(TieredPricingEditor, {
      props: { modelValue: [tier(null)] },
      global
    })

    await wrapper.get('button').trigger('click')

    const updated = wrapper.emitted<TokenPriceTier[]>('update:modelValue')?.[0]?.[0]
    expect(updated).toEqual([tier(64_000), tier(null)])
  })

  it('keeps the remaining terminal tier unbounded after deletion', async () => {
    const wrapper = mount(TieredPricingEditor, {
      props: { modelValue: [tier(200_000, 2), tier(null, 5)] },
      global
    })

    await wrapper.findAll('button')[0]!.trigger('click')

    const updated = wrapper.emitted<TokenPriceTier[]>('update:modelValue')?.[0]?.[0]
    expect(updated).toEqual([tier(null, 5)])
  })

  it('reports non-increasing finite thresholds', () => {
    const wrapper = mount(TieredPricingEditor, {
      props: { modelValue: [tier(200_000), tier(128_000), tier(null)] },
      global
    })

    expect(wrapper.get('.alert').text()).toContain('严格递增')
  })

  it('reports an empty tier list consistently', () => {
    const wrapper = mount(TieredPricingEditor, {
      props: { modelValue: [] },
      global
    })

    expect(wrapper.get('.alert').text()).toContain('至少需要一个价格档位')
  })
})

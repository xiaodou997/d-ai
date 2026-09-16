import { flushPromises, mount } from '@vue/test-utils';
import { defineComponent, h, ref } from 'vue';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useUpstreamStability } from './useUpstreamStability';
import type { Stability } from './api';
import StabilityBadge from './StabilityBadge.vue';
const api = vi.hoisted(() => ({ list: vi.fn(), detail: vi.fn(), resume: vi.fn() }));
vi.mock('./api', () => ({ stabilityApi: api }));
function sample(id = 'a'): Stability {
 return { resource_id: id, resource_kind: 'direct_upstream', config_status: 'active', availability: 'available', endpoint_count: 1, credential_count: 0, model_count: 1, paths: [], window: '24h', coverage_started_at: 1, successes: 1, failures: 0, excluded: 0, samples: 1, success_rate: 100, cooldowns_last_hour: 0, repeated_failure: false, stability_declining: false, models: [], states: [] };
}

describe('upstream stability', () => {
 beforeEach(() => { vi.useFakeTimers(); vi.spyOn(document, 'hidden', 'get').mockReturnValue(false); api.list.mockResolvedValue({ items: [sample()] }); api.detail.mockResolvedValue(sample()); });
 afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); vi.clearAllMocks(); });
 it('refreshes every ten seconds only while visible and releases polling on unmount', async () => {
  const wrapper = mount(defineComponent({ setup() { useUpstreamStability('direct_upstream'); return () => h('div'); } }));
  await flushPromises(); expect(api.list).toHaveBeenCalledTimes(1);
  await vi.advanceTimersByTimeAsync(10_000); expect(api.list).toHaveBeenCalledTimes(2);
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(true); document.dispatchEvent(new Event('visibilitychange'));
  await vi.advanceTimersByTimeAsync(30_000); expect(api.list).toHaveBeenCalledTimes(2);
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false); document.dispatchEvent(new Event('visibilitychange')); await flushPromises(); expect(api.list).toHaveBeenCalledTimes(3);
  wrapper.unmount(); await vi.advanceTimersByTimeAsync(30_000); expect(api.list).toHaveBeenCalledTimes(3);
 });
 it('does not overwrite a newly selected account with an old response', async () => {
  let resolveOld!: (value: Stability) => void;
  api.detail.mockImplementationOnce(() => new Promise<Stability>((resolve) => { resolveOld = resolve; })).mockResolvedValueOnce(sample('b'));
  const id = ref('a'); let state!: ReturnType<typeof useUpstreamStability>;
  const wrapper = mount(defineComponent({ setup() { state = useUpstreamStability('direct_upstream', () => id.value); return () => h('div'); } }));
  id.value = 'b'; await flushPromises(); expect(state.detail.value?.resource_id).toBe('b');
  resolveOld(sample('a')); await flushPromises(); expect(state.detail.value?.resource_id).toBe('b'); wrapper.unmount();
 });
 it('always attaches sample count and distinguishes missing data from low confidence', () => {
  const wrapper = mount(StabilityBadge, { props: { value: sample() } });
  expect(wrapper.text()).toContain('100.0%'); expect(wrapper.text()).toContain('1 次'); expect(wrapper.text()).toContain('样本不足'); wrapper.unmount();
  const empty = mount(StabilityBadge); expect(empty.text()).toContain('暂无稳定性样本'); expect(empty.text()).not.toContain('100'); empty.unmount();
  const stateUnavailable = mount(StabilityBadge, { props: { value: { ...sample(), availability: 'unknown', state_error: '运行状态暂不可读取' } } });
  expect(stateUnavailable.text()).not.toContain('状态服务不可用'); expect(stateUnavailable.text()).toContain('100.0%'); stateUnavailable.unmount();
 });
});

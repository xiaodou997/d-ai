import { onBeforeUnmount, onMounted, shallowRef, watch } from 'vue';
import { stabilityApi, type ResourceKind, type Stability, type StabilityWindow } from './api';

export function useUpstreamStability(kind: ResourceKind, resourceId?: () => string) {
  const window = shallowRef<StabilityWindow>('24h');
  const items = shallowRef<Stability[]>([]);
  const detail = shallowRef<Stability | null>(null);
  const error = shallowRef('');
  let controller: AbortController | null = null;
  let timer: ReturnType<typeof setInterval> | undefined;
  let generation = 0;
  async function refresh() {
    if (document.hidden) return;
    const current = ++generation;
    controller?.abort(); controller = new AbortController();
    try {
      if (resourceId) {
        const id = resourceId();
        if (!id) { detail.value = null; return; }
        const result = await stabilityApi.detail(kind, id, window.value, controller.signal);
        if (current === generation) detail.value = result;
      } else {
        const result = await stabilityApi.list(kind, window.value, controller.signal);
        if (current === generation) items.value = result.items || [];
      }
      if (current === generation) error.value = '';
    } catch (cause) {
      if (current === generation && !controller?.signal.aborted) error.value = cause instanceof Error ? cause.message : '运行信息加载失败';
    }
  }
  function visibilityChanged() {
    if (document.hidden) { ++generation; controller?.abort(); if (timer) clearInterval(timer); timer = undefined; }
    else { void refresh(); if (!timer) timer = setInterval(() => void refresh(), 10_000); }
  }
  onMounted(() => { document.addEventListener('visibilitychange', visibilityChanged); visibilityChanged(); });
  onBeforeUnmount(() => { ++generation; controller?.abort(); if (timer) clearInterval(timer); document.removeEventListener('visibilitychange', visibilityChanged); });
  watch(window, refresh);
  if (resourceId) watch(resourceId, () => { detail.value = null; void refresh(); });
  return { window, items, detail, error, refresh };
}

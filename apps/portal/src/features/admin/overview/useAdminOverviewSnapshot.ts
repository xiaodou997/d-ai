import { computed, ref } from "vue";

import { adminOverviewApi } from "@/api/adminOverview";
import {
  buildWorkbenchRangeWindow,
  DEFAULT_WORKBENCH_RANGE_ID,
  getWorkbenchRangeOption,
  type WorkbenchRangeId
} from "@/components/workbench/workbenchRanges";
import type { OverviewSnapshot } from "./overviewModel";

export type OverviewView = "dashboard" | "business" | "usage" | "cost" | "operations";

const emptySnapshot = (): OverviewSnapshot => ({
  meta: { view: "dashboard", date_from: 0, date_to: 0, generated_at: 0 },
  summary: {
    total_requests: 0, successful_requests: 0, failed_requests: 0, total_tokens: 0, total_prompt_tokens: 0,
    total_completion_tokens: 0, cache_read_tokens: 0, cache_write_tokens: 0, active_tenants: 0, active_users: 0,
    active_accounts: 0, new_tenants: 0, new_users: 0, total_catalog_base_usd: 0, total_tenant_payable_usd: 0, total_user_charged_usd: 0,
    gross_margin_usd: 0, avg_request_total_ms: 0, p95_request_total_ms: 0, avg_first_response_byte_ms: 0,
    p95_first_response_byte_ms: 0
  },
  trends: [], accounts: [], models: [], tenants: [], users: []
});

export function useAdminOverviewSnapshot(view: OverviewView, initialRangeId: WorkbenchRangeId = DEFAULT_WORKBENCH_RANGE_ID) {
  const selectedRangeId = ref<WorkbenchRangeId>(initialRangeId);
  const selectedRange = computed(() => getWorkbenchRangeOption(selectedRangeId.value));
  const snapshot = ref<OverviewSnapshot>(emptySnapshot());
  const loading = ref(false);
  const lastUpdatedAt = ref<Date | null>(null);
  const error = ref<unknown>(null);
  let sequence = 0;

  async function refresh() {
    const current = ++sequence;
    loading.value = true;
    error.value = null;
    const window = buildWorkbenchRangeWindow(selectedRange.value);
    try {
      const response = await adminOverviewApi.getSnapshot({
        view,
        date_from: window.date_from,
        date_to: window.date_to,
        compare: "previous"
      });
      if (current !== sequence) return;
      snapshot.value = response as unknown as OverviewSnapshot;
      lastUpdatedAt.value = new Date();
    } catch (cause) {
      if (current !== sequence) return;
      error.value = cause;
      snapshot.value = emptySnapshot();
      lastUpdatedAt.value = null;
    } finally {
      if (current === sequence) loading.value = false;
    }
  }

  function changeRange(rangeId: WorkbenchRangeId) {
    selectedRangeId.value = rangeId;
    void refresh();
  }

  return { selectedRangeId, selectedRange, snapshot, loading, lastUpdatedAt, error, refresh, changeRange };
}

import { computed, nextTick, onBeforeUnmount, onMounted, ref, type Ref } from "vue";
import { ElMessage } from "element-plus";
import { buildWorkbenchRangeWindow, getWorkbenchRangeOption, type WorkbenchRangeId } from "@/components/workbench/workbenchRanges";
import type { RecordFilterValues } from "../components/RecordFilters.vue";
import { recordsApi, type RecordQuery, type RequestRecord, type RecordSummary } from "../recordsApi";
import { recordMetrics, type RecordRole } from "../recordPresentation";
const emptyFilters = (): RecordFilterValues => ({ model: "", tenant_name: "", user_name: "", group: "", api_key_name: "", request_id: "", source: "" });
export function useRecordList(role: Ref<RecordRole>, stateKey: string, fixedUser: string | undefined, container: Ref<HTMLElement | null>) {
  const filters = ref(emptyFilters());
  const range = ref<WorkbenchRangeId>("today");
  const customRange = ref<[Date, Date] | null>(null);
  const appliedRange = ref<WorkbenchRangeId>("today");
  const applied = ref<RecordQuery>({});
  const tab = ref<"requests" | "errors">("requests");
  const pageSize = ref(20);
  const cursor = ref<string | undefined>();
  const history = ref<Array<string | undefined>>([]);
  const nextCursor = ref<string>();
  const rows = ref<RequestRecord[]>([]);
  const summary = ref<RecordSummary | null>(null);
  const busy = ref(false);
  const failure = ref("");
  let controller: AbortController | undefined;
  let savedScroll: number[] = [];
  let savedTableScroll = 0;
  const rangeLabel = computed(() => getWorkbenchRangeOption(appliedRange.value).label);
  const metrics = computed(() => recordMetrics(summary.value, role.value, rangeLabel.value));
  function ancestors() { const result: HTMLElement[] = []; let el = container.value; while (el) { result.push(el); el = el.parentElement; } return result; }
  function save() {
    try { sessionStorage.setItem(stateKey, JSON.stringify({ filters: filters.value, range: range.value, customRange: customRange.value, applied: applied.value, appliedRange: appliedRange.value, tab: tab.value, pageSize: pageSize.value, cursor: cursor.value, history: history.value, tableScroll: container.value?.querySelector(".record-table")?.scrollLeft || 0, scroll: ancestors().map(el => el.scrollTop) })); } catch { /* Storage is optional. */ }
  }
  function restore() {
    try {
      const raw = sessionStorage.getItem(stateKey); if (!raw) return;
      const state = JSON.parse(raw);
      filters.value = { ...emptyFilters(), ...state.filters };
      range.value = getWorkbenchRangeOption(state.range).id;
      appliedRange.value = getWorkbenchRangeOption(state.appliedRange).id;
      customRange.value = state.customRange ? state.customRange.map((date: string) => new Date(date)) : null;
      applied.value = state.applied || {};
      // Scope is always supplied by the current embedding context and enforced by the server.
      applied.value.user_id = fixedUser;
      tab.value = state.tab === "errors" ? "errors" : "requests";
      pageSize.value = [20, 50, 100].includes(state.pageSize) ? state.pageSize : 20;
      cursor.value = state.cursor; history.value = (state.history || []).map((item: string | null) => item || undefined);
      if (range.value === "90d" || appliedRange.value === "90d") {
        range.value = "30d";
        applyDraft(); firstPage();
      }
      savedScroll = state.scroll || []; savedTableScroll = Number(state.tableScroll) || 0;
    } catch { /* Invalid stored state falls back to defaults. */ }
  }
  function applyDraft() {
    if (range.value === "custom" && (!customRange.value || !customRange.value.every(date => Number.isFinite(date.getTime())) || customRange.value[0] >= customRange.value[1])) { ElMessage.warning("请选择有效的起止时间"); return false; }
    const window = buildWorkbenchRangeWindow(getWorkbenchRangeOption(range.value));
    const f = filters.value;
    applied.value = { user_id: fixedUser, model: f.model.trim() || undefined, tenant_name: role.value === "admin" ? f.tenant_name.trim() || undefined : undefined, user_name: role.value !== "customer" && !fixedUser ? f.user_name.trim() || undefined : undefined, group: f.group.trim() || undefined, api_key_name: f.api_key_name.trim() || undefined, request_id: f.request_id.trim() || undefined, source: f.source || undefined, from: range.value === "custom" ? customRange.value![0].toISOString() : window.date_from, to: range.value === "custom" ? customRange.value![1].toISOString() : window.date_to };
    appliedRange.value = range.value;
    return true;
  }
  function firstPage() { cursor.value = undefined; nextCursor.value = undefined; history.value = []; }
  async function load() {
    controller?.abort(); const active = new AbortController(); controller = active; busy.value = true; failure.value = "";
    try {
      const [page, totals] = await Promise.all([recordsApi.list(tab.value, { ...applied.value, cursor: cursor.value, limit: pageSize.value }, active.signal), recordsApi.summary(applied.value, active.signal)]);
      if (active.signal.aborted) return;
      rows.value = page.records || []; nextCursor.value = page.next_cursor; summary.value = totals;
    } catch (error) {
      if (!active.signal.aborted) { failure.value = error instanceof Error ? error.message : "读取记录失败"; rows.value = []; summary.value = null; nextCursor.value = undefined; }
    } finally { if (!active.signal.aborted) busy.value = false; }
  }
  async function search() { if (!applyDraft()) return; firstPage(); await load(); }
  async function changeRange(value: WorkbenchRangeId) {
    range.value = value;
    if (value === "custom" && !validCustomRange()) return;
    await search();
  }
  function validCustomRange() {
    return customRange.value?.length === 2 && customRange.value.every(date => Number.isFinite(date.getTime())) && customRange.value[0] < customRange.value[1];
  }
  async function changeCustomRange(value: [Date, Date] | null) {
    customRange.value = value;
    if (range.value === "custom" && validCustomRange()) await search();
  }
  async function refresh() {
    if (appliedRange.value !== "custom") { const window = buildWorkbenchRangeWindow(getWorkbenchRangeOption(appliedRange.value)); applied.value = { ...applied.value, from: window.date_from, to: window.date_to }; }
    firstPage(); await load();
  }
  async function reset() { filters.value = emptyFilters(); range.value = "today"; customRange.value = null; await search(); }
  async function changeTab(key: string) { tab.value = key === "errors" ? "errors" : "requests"; firstPage(); await load(); }
  async function changePageSize(size: number) { pageSize.value = size; firstPage(); await load(); }
  async function next() { if (!nextCursor.value || busy.value) return; history.value.push(cursor.value); cursor.value = nextCursor.value; await load(); }
  async function previous() { if (!history.value.length || busy.value) return; cursor.value = history.value.pop(); await load(); }
  onMounted(async () => {
    restore(); if (!applied.value.from && !applyDraft()) return;
    await load(); await nextTick();
    const table = container.value?.querySelector(".record-table"); if (table) table.scrollLeft = savedTableScroll;
    ancestors().forEach((el, index) => { el.scrollTop = savedScroll[index] || 0; });
  });
  onBeforeUnmount(() => { save(); controller?.abort(); });
  return { filters, range, customRange, tab, pageSize, rows, summary, busy, failure, metrics, rangeLabel, applied, history, nextCursor, search, changeRange, changeCustomRange, refresh, reset, changeTab, changePageSize, next, previous, save };
}

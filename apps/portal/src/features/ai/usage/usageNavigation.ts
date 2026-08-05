import type { LocationQuery } from "vue-router";

import {
  WORKBENCH_RANGE_OPTIONS,
  type WorkbenchRangeId
} from "../../../components/workbench/workbenchRanges";
import type { UsageFilters } from "./model";

const recordFilterKeys = ["tenant_id", "user_id", "model_code", "request_status", "request_source"] as const;

export function restoreUsageRecordRouteQuery(
  query: LocationQuery,
  filters: UsageFilters,
  setRange: (range: WorkbenchRangeId) => void
) {
  for (const key of recordFilterKeys) {
    const value = query[key];
    filters[key] = typeof value === "string" ? value : "";
  }
  const range = query.range;
  if (typeof range === "string" && isWorkbenchRangeId(range)) setRange(range);
}

export function buildUsageRecordsRouteQuery(
  range: WorkbenchRangeId,
  filters: Partial<UsageFilters> = {}
): Record<string, string> {
  const query: Record<string, string> = { range };
  for (const key of recordFilterKeys) {
    const value = filters[key];
    if (value) query[key] = value;
  }
  return query;
}

function isWorkbenchRangeId(value: string): value is WorkbenchRangeId {
  return WORKBENCH_RANGE_OPTIONS.some((option) => option.id === value);
}

import type {
  DashboardRecentErrorDTO,
  DashboardSummaryDTO,
  DashboardTopModelDTO,
  DashboardTopTenantDTO,
  IdentityIncludedDTO,
  SystemStatusDTO
} from "@/api/types/ai";
import type { GlobalStatsRow } from "@/api/types/admin";
import type { DailyTrendRowDTO, UsageUpstreamSummaryRowDTO } from "@/features/ai/usage/model";
import type { ProxyNode, SystemModuleStatus } from "@/api/systemModules";

export type OverviewSection =
  | "summary"
  | "models"
  | "tenants"
  | "errors"
  | "trend"
  | "upstreams"
  | "system"
  | "global"
  | "modules"
  | "proxy";

export interface AdminOverviewSnapshot {
  summary: DashboardSummaryDTO;
  models: DashboardTopModelDTO[];
  tenants: DashboardTopTenantDTO[];
  tenantIncluded: IdentityIncludedDTO;
  errors: DashboardRecentErrorDTO[];
  trend: DailyTrendRowDTO[];
  upstreams: UsageUpstreamSummaryRowDTO[];
  system: SystemStatusDTO | null;
  global: GlobalStatsRow | null;
  modules: SystemModuleStatus[];
  proxyNodes: ProxyNode[];
}

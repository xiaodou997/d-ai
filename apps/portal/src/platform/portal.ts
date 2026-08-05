import type { PortalKind } from "./env";
import type { PortalMenuNode } from "./menu";

export interface PortalMetric {
  label: string;
  value: string;
  hint?: string;
}

export interface PortalHomeState {
  crumb: string;
  title: string;
  subtitle: string;
  metrics: PortalMetric[];
  nav: PortalMenuNode[];
}

/** Shared landing state for the one Portal shell. */
export function createPortalHomeState(_portal: PortalKind, nav: PortalMenuNode[]): PortalHomeState {
  return {
    crumb: "D-AI / Overview",
    title: "D-AI 统一平台",
    subtitle: "在一个 Portal 中管理用户中心、计费与 AI 服务。",
    metrics: [
      { label: "业务模块", value: "2", hint: "URM · AI" },
      { label: "业务准入", value: "动态", hint: "基于 /me/capabilities" },
      { label: "统一主题", value: "已启用", hint: "按 userType 切换" },
      { label: "部署形态", value: "单体", hint: "Portal embed 到 Go" }
    ],
    nav
  };
}

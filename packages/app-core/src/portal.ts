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

export function createPortalHomeState(portal: PortalKind, nav: PortalMenuNode[]): PortalHomeState {
  const copy = {
    admin: {
      crumb: "管理总览 / Dashboard",
      title: "平台控制概览",
      subtitle: "统一查看 URM、AI Gateway 与 Proxy Gateway 的运行态势。",
      metrics: [
        { label: "业务类别", value: "3", hint: "URM · AI · Proxy" },
        { label: "业务准入", value: "动态", hint: "基于 /me/capabilities" },
        { label: "统一主题", value: "已启用", hint: "Direction A Enterprise" },
        { label: "下一步", value: "T4.3", hint: "迁移 admin 模块页面" }
      ]
    },
    tenant: {
      crumb: "租户工作台 / Overview",
      title: "租户统一入口",
      subtitle: "账单、AI 配额、代理路由都会收口到一个租户工作台。",
      metrics: [
        { label: "统一入口", value: "1", hint: "URM + AI + Proxy" },
        { label: "鉴权链路", value: "已接入", hint: "OAuth2 + refresh" },
        { label: "业务入口", value: "静态", hint: "capabilities 过滤" },
        { label: "下一步", value: "T4.4", hint: "迁移租户模块页面" }
      ]
    },
    customer: {
      crumb: "用户中心 / Overview",
      title: "用户统一工作台",
      subtitle: "终端用户将在一个入口内查看余额、API key、调用日志与代理路由。",
      metrics: [
        { label: "统一入口", value: "1", hint: "账单 + AI + Proxy" },
        { label: "认证状态", value: "可刷新", hint: "共享 token 策略" },
        { label: "主题切换", value: "赤陶", hint: "Customer token" },
        { label: "下一步", value: "T4.5", hint: "迁移用户模块页面" }
      ]
    }
  }[portal];

  return {
    ...copy,
    nav
  };
}

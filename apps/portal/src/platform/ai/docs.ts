import type { PortalAppScope } from "./apps/types";

export type PortalAiDocsScope = PortalAppScope;
export type PortalAiDocsSectionKey = "overview" | "tooling" | "chat" | "images" | "app-keys";

export interface PortalAiDocsSection {
  key: PortalAiDocsSectionKey;
  slug: string;
  navLabel: string;
  title: string;
  description: string;
  keyType: "sk" | "rk" | "mixed";
}

export const PORTAL_AI_DOC_SECTIONS: ReadonlyArray<PortalAiDocsSection> = [
  {
    key: "overview",
    slug: "overview",
    navLabel: "接入总览",
    title: "接入总览",
    description: "接入地址、认证方式，以及 API 密钥和应用密钥的区别与适用场景。",
    keyType: "mixed"
  },
  {
    key: "tooling",
    slug: "tooling",
    navLabel: "工具接入",
    title: "工具接入",
    description: "Codex CLI 和 Claude Code 的配置方法，替换 Base URL 和密钥即可使用。",
    keyType: "sk"
  },
  {
    key: "chat",
    slug: "api-chat",
    navLabel: "API · 对话",
    title: "API · 对话",
    description: "OpenAI / Anthropic 兼容的对话与向量接口，适合直接调用模型能力的开发者。",
    keyType: "sk"
  },
  {
    key: "images",
    slug: "api-images",
    navLabel: "API · 生图",
    title: "API · 生图",
    description: "直接调用模型的生图接口；不确定时先只传 model、prompt、size、response_format 就够了。",
    keyType: "sk"
  },
  {
    key: "app-keys",
    slug: "app-keys",
    navLabel: "应用密钥",
    title: "应用密钥",
    description: "绑定单个应用的最小权限密钥，统一通过 /v1/run 调用：模板变量、附件结构、固定分辨率和统一返回都在这里。",
    keyType: "rk"
  }
] as const;

export function portalAiDocsSectionByKey(key?: string): PortalAiDocsSection {
  return PORTAL_AI_DOC_SECTIONS.find((item) => item.key === key) ?? PORTAL_AI_DOC_SECTIONS[0];
}

export function portalAiDocsAppLabel(scope: PortalAiDocsScope): string {
  switch (scope) {
    case "user":
      return "我的应用";
    default:
      return "租户应用";
  }
}

export function portalAiDocsScopeLabel(scope: PortalAiDocsScope): string {
  switch (scope) {
    case "user":
      return "用户侧";
    default:
      return "租户侧";
  }
}

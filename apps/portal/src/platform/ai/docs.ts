export type PortalAiDocsScope = "tenant" | "user";
export type PortalAiDocsSectionKey = "tooling";

export interface PortalAiDocsSection {
  key: PortalAiDocsSectionKey;
  slug: string;
  navLabel: string;
  title: string;
  description: string;
  keyType: "sk";
}

export const PORTAL_AI_DOC_SECTIONS: ReadonlyArray<PortalAiDocsSection> = [
  {
    key: "tooling",
    slug: "tooling",
    navLabel: "工具接入指南",
    title: "工具接入指南",
    description: "Codex CLI 和 Claude Code 的配置方法，替换 Base URL 和密钥即可使用。",
    keyType: "sk"
  }
] as const;

export function portalAiDocsSectionByKey(key?: string): PortalAiDocsSection {
  return PORTAL_AI_DOC_SECTIONS.find((item) => item.key === key) ?? PORTAL_AI_DOC_SECTIONS[0];
}

export function portalAiDocsScopeLabel(scope: PortalAiDocsScope): string {
  switch (scope) {
    case "user":
      return "用户侧";
    default:
      return "租户侧";
  }
}

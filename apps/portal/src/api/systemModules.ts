import { authenticatedRequest, apiHeaders, apiBaseUrl } from "./request";

export type SystemModuleStatus = {
  name: string;
  displayName: string;
  description: string;
  category: "integration" | "security" | string;
  adminRoute: string;
  order: number;
  available: boolean;
  enabled: boolean;
  active: boolean;
  configValidated: boolean;
  configError?: string;
  health: string;
};

export type PIIRuleConfig = {
  id: string;
  name: string;
  pattern: string;
  enabled: boolean;
  system: boolean;
};

export type PIIProtectionConfig = {
  enabled: boolean;
  rules: PIIRuleConfig[];
  placeholderPrefix: string;
};

export type ProxyNode = {
  id: string;
  name: string;
  proxyType: "http" | "socks5";
  endpoint: string;
  username?: string;
  weight: number;
  status: "active" | "disabled";
  healthStatus: string;
  lastCheckedAt?: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
};

export type DataCleanupPolicy = {
  enabled: boolean;
  requestBodyDays: number;
  requestPayloadDays: number;
  notificationDays: number;
  moderationDays: number;
  riskEventDays: number;
  adminAuditDays: number;
  auditBlobDays: number;
  usageRollupDays: number;
  batchSize: number;
};

export type DataCleanupPreviewItem = {
  target: string;
  label: string;
  retentionDays: number;
  cutoff: string;
  eligibleRows: number;
};

export type DataCleanupPreview = {
  policy: DataCleanupPolicy;
  generatedAt: string;
  items: DataCleanupPreviewItem[];
};

export type DataCleanupRun = {
  id: string;
  trigger: "automatic" | "manual";
  status: "queued" | "running" | "completed" | "failed";
  requestedBy?: string;
  targets: string[];
  summary: Record<string, number>;
  error?: string;
  createdAt: string;
  startedAt?: string;
  completedAt?: string;
};

type ProxyNodeWrite = {
  name: string;
  proxyType: "http" | "socks5";
  endpoint: string;
  username?: string;
  password?: string;
  weight?: number;
  status?: "active" | "disabled";
};

function request() {
  return authenticatedRequest();
}

export const systemModulesApi = {
  list() {
    return request()<SystemModuleStatus[]>({ method: "GET", path: "/api/v1/admin/modules", headers: apiHeaders, baseUrl: apiBaseUrl });
  },
  setEnabled(name: string, enabled: boolean) {
    return request()<SystemModuleStatus>({ method: "PUT", path: `/api/v1/admin/modules/${encodeURIComponent(name)}/enabled`, headers: apiHeaders, body: { enabled }, baseUrl: apiBaseUrl });
  },
  getPIIProtectionConfig() {
    return request()<PIIProtectionConfig>({ method: "GET", path: "/api/v1/admin/modules/pii-protection/config", headers: apiHeaders, baseUrl: apiBaseUrl });
  },
  getPIIProtectionDefaults() {
    return request()<PIIProtectionConfig>({ method: "GET", path: "/api/v1/admin/modules/pii-protection/defaults", headers: apiHeaders, baseUrl: apiBaseUrl });
  },
  updatePIIProtectionConfig(body: PIIProtectionConfig) {
    return request()<PIIProtectionConfig>({ method: "PUT", path: "/api/v1/admin/modules/pii-protection/config", headers: apiHeaders, body, baseUrl: apiBaseUrl });
  },
  previewPIIProtection(body: { config: PIIProtectionConfig; text: string }) {
    return request()<{ protectedText: string }>({ method: "POST", path: "/api/v1/admin/modules/pii-protection/preview", headers: apiHeaders, body, baseUrl: apiBaseUrl });
  },
  listProxyNodes() {
    return request()<ProxyNode[]>({ method: "GET", path: "/api/v1/admin/proxy-nodes", headers: apiHeaders, baseUrl: apiBaseUrl });
  },
  createProxyNode(body: ProxyNodeWrite) {
    return request()<ProxyNode>({ method: "POST", path: "/api/v1/admin/proxy-nodes", headers: apiHeaders, body, baseUrl: apiBaseUrl });
  },
  updateProxyNode(id: string, body: ProxyNodeWrite) {
    return request()<ProxyNode>({ method: "PUT", path: `/api/v1/admin/proxy-nodes/${encodeURIComponent(id)}`, headers: apiHeaders, body, baseUrl: apiBaseUrl });
  },
  deleteProxyNode(id: string) {
    return request()<void>({ method: "DELETE", path: `/api/v1/admin/proxy-nodes/${encodeURIComponent(id)}`, headers: apiHeaders, baseUrl: apiBaseUrl });
  },
  sendNotification(body: { eventKey: string; channel: "in_app" | "webhook"; recipientUserId?: string; recipientUserType?: number; tenantId?: string; title: string; body: string; webhookUrl?: string }) {
    return request()<{ id: string; status: string }>({ method: "POST", path: "/api/v1/admin/notifications/send", headers: apiHeaders, body, baseUrl: apiBaseUrl });
  },
  getCleanupPolicy() {
    return request()<DataCleanupPolicy>({ method: "GET", path: "/api/v1/admin/data-cleanup/policy", headers: apiHeaders, baseUrl: apiBaseUrl });
  },
  updateCleanupPolicy(body: DataCleanupPolicy) {
    return request()<DataCleanupPolicy>({ method: "PUT", path: "/api/v1/admin/data-cleanup/policy", headers: apiHeaders, body, baseUrl: apiBaseUrl });
  },
  previewCleanup() {
    return request()<DataCleanupPreview>({ method: "GET", path: "/api/v1/admin/data-cleanup/preview", headers: apiHeaders, baseUrl: apiBaseUrl });
  },
  listCleanupRuns() {
    return request()<DataCleanupRun[]>({ method: "GET", path: "/api/v1/admin/data-cleanup/runs", headers: apiHeaders, baseUrl: apiBaseUrl });
  },
  startCleanup(body: { targets: string[]; confirmation: string }) {
    return request()<DataCleanupRun>({ method: "POST", path: "/api/v1/admin/data-cleanup/runs", headers: apiHeaders, body, baseUrl: apiBaseUrl });
  }
};

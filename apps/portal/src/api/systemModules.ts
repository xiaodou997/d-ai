import { authenticatedRequest, apiHeaders, apiBaseUrl } from "./request";
import {
  createTypedOperationRequest,
  type OperationBody,
  type OperationResponse
} from ".";

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

type ProxyNodeWrite = OperationBody<"admin-create-proxy-node">;
type SystemModuleTransport = OperationResponse<"admin-set-module-enabled">;
type PIIConfigTransport = OperationResponse<"admin-get-pii-protection-config">;
type ProxyNodeTransport = OperationResponse<"admin-create-proxy-node">;
type CleanupPolicyTransport = OperationResponse<"admin-get-data-cleanup-policy">;
type CleanupPreviewTransport = OperationResponse<"admin-preview-data-cleanup">;
type CleanupRunTransport = OperationResponse<"admin-start-data-cleanup">;

export type NotificationSendResult = {
  id: string;
  status: string;
};

function request() {
  return authenticatedRequest();
}

const typedRequest = createTypedOperationRequest(request());

function toSystemModule(value: SystemModuleTransport): SystemModuleStatus {
  return {
    name: value.name,
    displayName: value.displayName,
    description: value.description,
    category: value.category,
    adminRoute: value.adminRoute,
    order: value.order,
    available: value.available,
    enabled: value.enabled,
    active: value.active,
    configValidated: value.configValidated,
    configError: value.configError,
    health: value.health
  };
}

function toPIIConfig(value: PIIConfigTransport): PIIProtectionConfig {
  return {
    enabled: value.enabled,
    rules: value.rules?.map((rule) => ({ ...rule })) ?? [],
    placeholderPrefix: value.placeholderPrefix
  };
}

function toProxyType(value: string): ProxyNode["proxyType"] {
  if (value === "http" || value === "socks5") return value;
  throw new Error(`Unexpected proxy type: ${value}`);
}

function toProxyStatus(value: string): ProxyNode["status"] {
  if (value === "active" || value === "disabled") return value;
  throw new Error(`Unexpected proxy status: ${value}`);
}

function toProxyNode(value: ProxyNodeTransport): ProxyNode {
  return {
    id: value.id,
    name: value.name,
    proxyType: toProxyType(value.proxyType),
    endpoint: value.endpoint,
    username: value.username,
    weight: value.weight,
    status: toProxyStatus(value.status),
    healthStatus: value.healthStatus,
    lastCheckedAt: value.lastCheckedAt,
    lastError: value.lastError,
    createdAt: value.createdAt,
    updatedAt: value.updatedAt
  };
}

function toCleanupPolicy(value: CleanupPolicyTransport): DataCleanupPolicy {
  return {
    enabled: value.enabled,
    requestBodyDays: value.requestBodyDays,
    requestPayloadDays: value.requestPayloadDays,
    notificationDays: value.notificationDays,
    moderationDays: value.moderationDays,
    riskEventDays: value.riskEventDays,
    adminAuditDays: value.adminAuditDays,
    auditBlobDays: value.auditBlobDays,
    usageRollupDays: value.usageRollupDays,
    batchSize: value.batchSize
  };
}

function toCleanupPreview(value: CleanupPreviewTransport): DataCleanupPreview {
  return {
    policy: toCleanupPolicy(value.policy),
    generatedAt: value.generatedAt,
    items: value.items?.map((item) => ({ ...item })) ?? []
  };
}

function toCleanupTrigger(value: string): DataCleanupRun["trigger"] {
  if (value === "automatic" || value === "manual") return value;
  throw new Error(`Unexpected cleanup trigger: ${value}`);
}

function toCleanupStatus(value: string): DataCleanupRun["status"] {
  switch (value) {
    case "queued":
    case "running":
    case "completed":
    case "failed":
      return value;
    default:
      throw new Error(`Unexpected cleanup status: ${value}`);
  }
}

function toCleanupRun(value: CleanupRunTransport): DataCleanupRun {
  return {
    id: value.id,
    trigger: toCleanupTrigger(value.trigger),
    status: toCleanupStatus(value.status),
    requestedBy: value.requestedBy,
    targets: value.targets ?? [],
    summary: { ...value.summary },
    error: value.error,
    createdAt: value.createdAt,
    startedAt: value.startedAt,
    completedAt: value.completedAt
  };
}

export const systemModulesApi = {
  list() {
    return typedRequest<"admin-list-modules">({
      method: "GET",
      path: "/api/v1/admin/modules",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then((items) => items?.map(toSystemModule) ?? []);
  },
  setEnabled(name: string, enabled: boolean) {
    return typedRequest<"admin-set-module-enabled">({
      method: "PUT",
      path: `/api/v1/admin/modules/${encodeURIComponent(name)}/enabled`,
      pathParams: { name },
      headers: apiHeaders,
      body: { enabled },
      baseUrl: apiBaseUrl
    }).then(toSystemModule);
  },
  getPIIProtectionConfig() {
    return typedRequest<"admin-get-pii-protection-config">({
      method: "GET",
      path: "/api/v1/admin/modules/pii-protection/config",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toPIIConfig);
  },
  getPIIProtectionDefaults() {
    return typedRequest<"admin-get-pii-protection-defaults">({
      method: "GET",
      path: "/api/v1/admin/modules/pii-protection/defaults",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toPIIConfig);
  },
  updatePIIProtectionConfig(body: PIIProtectionConfig) {
    return typedRequest<"admin-update-pii-protection-config">({
      method: "PUT",
      path: "/api/v1/admin/modules/pii-protection/config",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then(toPIIConfig);
  },
  previewPIIProtection(body: { config: PIIProtectionConfig; text: string }) {
    return typedRequest<"admin-preview-pii-protection">({
      method: "POST",
      path: "/api/v1/admin/modules/pii-protection/preview",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    });
  },
  listProxyNodes() {
    return typedRequest<"admin-list-proxy-nodes">({
      method: "GET",
      path: "/api/v1/admin/proxy-nodes",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then((items) => items?.map(toProxyNode) ?? []);
  },
  createProxyNode(body: ProxyNodeWrite) {
    return typedRequest<"admin-create-proxy-node">({
      method: "POST",
      path: "/api/v1/admin/proxy-nodes",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then(toProxyNode);
  },
  updateProxyNode(id: string, body: ProxyNodeWrite) {
    return typedRequest<"admin-update-proxy-node">({
      method: "PUT",
      path: `/api/v1/admin/proxy-nodes/${encodeURIComponent(id)}`,
      pathParams: { id },
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then(toProxyNode);
  },
  deleteProxyNode(id: string) {
    return typedRequest<"admin-delete-proxy-node">({
      method: "DELETE",
      path: `/api/v1/admin/proxy-nodes/${encodeURIComponent(id)}`,
      pathParams: { id },
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(() => undefined);
  },
  sendNotification(body: OperationBody<"admin-send-notification">) {
    return typedRequest<"admin-send-notification">({
      method: "POST",
      path: "/api/v1/admin/notifications/send",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then((value): NotificationSendResult => ({ id: value.id, status: value.status }));
  },
  getCleanupPolicy() {
    return typedRequest<"admin-get-data-cleanup-policy">({
      method: "GET",
      path: "/api/v1/admin/data-cleanup/policy",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toCleanupPolicy);
  },
  updateCleanupPolicy(body: DataCleanupPolicy) {
    return typedRequest<"admin-update-data-cleanup-policy">({
      method: "PUT",
      path: "/api/v1/admin/data-cleanup/policy",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then(toCleanupPolicy);
  },
  previewCleanup() {
    return typedRequest<"admin-preview-data-cleanup">({
      method: "GET",
      path: "/api/v1/admin/data-cleanup/preview",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then(toCleanupPreview);
  },
  listCleanupRuns() {
    return typedRequest<"admin-list-data-cleanup-runs">({
      method: "GET",
      path: "/api/v1/admin/data-cleanup/runs",
      headers: apiHeaders,
      baseUrl: apiBaseUrl
    }).then((items) => items?.map(toCleanupRun) ?? []);
  },
  startCleanup(body: { targets: string[]; confirmation: string }) {
    return typedRequest<"admin-start-data-cleanup">({
      method: "POST",
      path: "/api/v1/admin/data-cleanup/runs",
      headers: apiHeaders,
      body,
      baseUrl: apiBaseUrl
    }).then(toCleanupRun);
  }
};

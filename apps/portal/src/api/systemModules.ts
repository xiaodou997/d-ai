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
  }
};

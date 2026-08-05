import type { BackendService, PortalEnv } from "./env";

// PKCE verifier 暂存于 sessionStorage（按服务隔离），授权回跳后一次性取出用于换 token。
const PKCE_STORAGE_PREFIX = "dai:pkce:";

function base64UrlEncode(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function randomVerifier(): string {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return base64UrlEncode(bytes);
}

async function challengeFromVerifier(verifier: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier));
  return base64UrlEncode(new Uint8Array(digest));
}

// beginSSOAuthorize 发起标准授权码 + PKCE 流程：生成 verifier 暂存、构造带 S256 challenge 的
// authorize URL。返回的 URL 由调用方整页跳转。未配置 ssoAuthorizeUrl 时返回 null。
export async function beginSSOAuthorize(
  env: PortalEnv,
  redirectUri: string,
  state = "",
  service: BackendService = "urm"
): Promise<string | null> {
  if (!env.ssoAuthorizeUrl) return null;
  const verifier = randomVerifier();
  const challenge = await challengeFromVerifier(verifier);
  sessionStorage.setItem(PKCE_STORAGE_PREFIX + service, verifier);

  const url = new URL(env.ssoAuthorizeUrl);
  url.searchParams.set("response_type", "code");
  url.searchParams.set("client_id", env.serviceClientIds?.[service] || env.xClientId);
  url.searchParams.set("client_type", env.clientTypeHeader);
  url.searchParams.set("redirect_uri", redirectUri);
  url.searchParams.set("code_challenge", challenge);
  url.searchParams.set("code_challenge_method", "S256");
  if (state) {
    url.searchParams.set("state", state);
  }
  return url.toString();
}

// consumePKCEVerifier 取出并清除某服务暂存的 verifier，供授权码交换时携带。
export function consumePKCEVerifier(service: BackendService = "urm"): string {
  const key = PKCE_STORAGE_PREFIX + service;
  const verifier = sessionStorage.getItem(key) || "";
  sessionStorage.removeItem(key);
  return verifier;
}

export function currentRedirectUri(path = window.location.pathname): string {
  return new URL(path, window.location.origin).toString();
}

// SSO 重定向熔断：若短时间内反复发起授权（登录态始终建立不起来），说明陷入风暴，
// 应停下而非无限重定向打爆后端。阈值：10 秒内超过 5 次。
const SSO_LOOP_KEY = "dai:sso:loop";
const SSO_LOOP_WINDOW_MS = 10_000;
const SSO_LOOP_LIMIT = 5;

export function ssoLoopTripped(): boolean {
  const now = Date.now();
  let count = 0;
  let first = now;
  try {
    const raw = sessionStorage.getItem(SSO_LOOP_KEY);
    if (raw) {
      const parsed = JSON.parse(raw) as { count: number; first: number };
      count = parsed.count;
      first = parsed.first;
    }
  } catch {
    // 忽略损坏的计数，按新窗口重来
  }
  if (now - first > SSO_LOOP_WINDOW_MS) {
    count = 0;
    first = now;
  }
  count += 1;
  sessionStorage.setItem(SSO_LOOP_KEY, JSON.stringify({ count, first }));
  return count > SSO_LOOP_LIMIT;
}

export function clearSSOAttempts(): void {
  sessionStorage.removeItem(SSO_LOOP_KEY);
}

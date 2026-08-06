export type ApiKeyUsageTool = "codex" | "claude" | "openai";
export type ApiKeyUsagePlatform = "macos" | "windows";

export interface ApiKeyUsageFile {
  id: string;
  label: string;
  path: string;
  filename: string;
  content: string;
  mimeType: string;
  containsSecret: boolean;
}

export interface ApiKeyUsageConfigOptions {
  baseUrl: string;
  apiKey: string;
  platform: ApiKeyUsagePlatform;
  model?: string;
}

const DEFAULT_CODEX_MODEL = "gpt-5.5";

function normalizedBaseUrl(baseUrl: string) {
  return baseUrl.replace(/\/$/, "");
}

function quoteShellValue(value: string) {
  return value.replace(/([\\"$`])/g, "\\$1");
}

function codexPath(platform: ApiKeyUsagePlatform, filename: string) {
  return platform === "windows" ? `%USERPROFILE%\\.codex\\${filename}` : `~/.codex/${filename}`;
}

function claudePath(platform: ApiKeyUsagePlatform) {
  return platform === "windows" ? "%USERPROFILE%\\.claude\\settings.json" : "~/.claude/settings.json";
}

export function buildApiKeyUsageFiles(
  tool: ApiKeyUsageTool,
  options: ApiKeyUsageConfigOptions
): ApiKeyUsageFile[] {
  const baseUrl = normalizedBaseUrl(options.baseUrl);
  const model = options.model || DEFAULT_CODEX_MODEL;

  if (tool === "codex") {
    return [
      {
        id: "codex-config",
        label: "Codex 配置",
        path: codexPath(options.platform, "config.toml"),
        filename: "config.toml",
        content: `model_provider = "OpenAI"
model = "${model}"
review_model = "${model}"
model_reasoning_effort = "xhigh"
disable_response_storage = true

[model_providers.OpenAI]
name = "OpenAI"
base_url = "${baseUrl}"
wire_api = "responses"
requires_openai_auth = true`,
        mimeType: "application/toml;charset=utf-8",
        containsSecret: false
      },
      {
        id: "codex-auth",
        label: "Codex 认证",
        path: codexPath(options.platform, "auth.json"),
        filename: "auth.json",
        content: JSON.stringify({ OPENAI_API_KEY: options.apiKey }, null, 2),
        mimeType: "application/json;charset=utf-8",
        containsSecret: true
      }
    ];
  }

  if (tool === "claude") {
    return [
      {
        id: "claude-settings",
        label: "Claude Code 配置",
        path: claudePath(options.platform),
        filename: "settings.json",
        content: JSON.stringify(
          {
            env: {
              ANTHROPIC_BASE_URL: baseUrl,
              ANTHROPIC_AUTH_TOKEN: options.apiKey,
              CLAUDE_CODE_OAUTH_TOKEN: options.apiKey
            }
          },
          null,
          2
        ),
        mimeType: "application/json;charset=utf-8",
        containsSecret: true
      }
    ];
  }

  const shellContent = options.platform === "windows"
    ? `$env:OPENAI_BASE_URL = "${quoteShellValue(baseUrl)}"
$env:OPENAI_API_KEY = "${quoteShellValue(options.apiKey)}"`
    : `OPENAI_BASE_URL="${quoteShellValue(baseUrl)}"
OPENAI_API_KEY="${quoteShellValue(options.apiKey)}"`;

  return [
    {
      id: "openai-env",
      label: "OpenAI 兼容环境变量",
      path: options.platform === "windows" ? "PowerShell" : "项目 .env",
      filename: options.platform === "windows" ? "dai-openai.ps1" : ".env",
      content: shellContent,
      mimeType: "text/plain;charset=utf-8",
      containsSecret: true
    }
  ];
}

export function downloadApiKeyUsageFile(file: ApiKeyUsageFile) {
  const blob = new Blob([file.content], { type: file.mimeType });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = file.filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
}

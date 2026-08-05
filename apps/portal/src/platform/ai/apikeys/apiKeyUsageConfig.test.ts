import { describe, expect, it } from "vitest";

import { buildApiKeyUsageFiles } from "./apiKeyUsageConfig";

describe("buildApiKeyUsageFiles", () => {
  const options = {
    baseUrl: "https://api.example.com/",
    apiKey: "sk-test-key",
    platform: "macos" as const
  };

  it("keeps the API key out of Codex config.toml and writes it to auth.json", () => {
    const [config, auth] = buildApiKeyUsageFiles("codex", options);

    expect(config.path).toBe("~/.codex/config.toml");
    expect(config.content).toContain('base_url = "https://api.example.com"');
    expect(config.content).not.toContain(options.apiKey);
    expect(auth.filename).toBe("auth.json");
    expect(JSON.parse(auth.content)).toEqual({ OPENAI_API_KEY: options.apiKey });
  });

  it("uses the selected platform in destination paths", () => {
    const [file] = buildApiKeyUsageFiles("claude", { ...options, platform: "windows" });

    expect(file.path).toBe("%USERPROFILE%\\.claude\\settings.json");
  });

  it("creates safe OpenAI compatible environment variables", () => {
    const [file] = buildApiKeyUsageFiles("openai", {
      ...options,
      apiKey: 'sk-test"$value'
    });

    expect(file.filename).toBe(".env");
    expect(file.content).toContain('OPENAI_API_KEY="sk-test\\"\\$value"');
    expect(file.content).not.toContain("export ");
    expect(file.containsSecret).toBe(true);
  });
});

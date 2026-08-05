<script setup lang="ts">
import { computed } from "vue";

import PortalContentCard from "../../../page/PortalContentCard.vue";
import PortalAiDocsCodeBlock from "../PortalAiDocsCodeBlock.vue";

const props = defineProps<{
  baseUrl: string;
}>();

const codexConfigToml = computed(() => `# ~/.codex/config.toml
model_provider = "OpenAI"
model = "gpt-5.5"
review_model = "gpt-5.5"
model_reasoning_effort = "xhigh"
disable_response_storage = true

[model_providers.OpenAI]
name = "OpenAI"
base_url = "${props.baseUrl}"
wire_api = "responses"
requires_openai_auth = true`);

const codexAuthJson = computed(() =>
  JSON.stringify(
    {
      OPENAI_API_KEY: "sk-你的API密钥"
    },
    null,
    2
  )
);

const codexEnvFile = computed(() => `OPENAI_API_KEY=sk-你的API密钥
OPENAI_BASE_URL=${props.baseUrl}`);

const claudeSettingsJson = computed(() =>
  JSON.stringify(
    {
      env: {
        ANTHROPIC_BASE_URL: props.baseUrl,
        ANTHROPIC_AUTH_TOKEN: "sk-你的API密钥"
      }
    },
    null,
    2
  )
);

const claudeOAuthEnv = computed(() => `export ANTHROPIC_BASE_URL="${props.baseUrl}"
export CLAUDE_CODE_OAUTH_TOKEN="sk-你的API密钥"`);

const toolingBaseUrlRules = computed(() => [
  {
    title: "Codex CLI",
    value: props.baseUrl,
    desc: "直接使用公开 Base URL；平台会规范化 Codex 的无版本请求路径。"
  },
  {
    title: "Claude Code",
    value: props.baseUrl,
    desc: "直接用公开 Base URL；如果地址里已经带 /ai，保持不变。"
  }
]);
</script>

<template>
  <div class="ai-docs-stack">
    <PortalContentCard title="工具支持范围" description="这两类编程助手都可以直接连接当前平台，不需要额外中间层。">
      <div class="ai-docs-grid">
        <article class="ai-docs-note">
          <strong class="ai-docs-note__head">Codex CLI</strong>
          <div class="ai-docs-note__body">走 OpenAI Responses API 协议，Base URL 与 Claude Code 一样使用公开根地址。</div>
        </article>
        <article class="ai-docs-note">
          <strong class="ai-docs-note__head">Claude Code</strong>
          <div class="ai-docs-note__body">走 Anthropic Messages API，Base URL 保持公开根地址，不要手动补 <span class="ai-docs-inline-code">/v1</span> 或删掉已有的 <span class="ai-docs-inline-code">/ai</span>。</div>
        </article>
      </div>
      <div class="ai-docs-rule-grid">
        <article v-for="item in toolingBaseUrlRules" :key="item.title" class="ai-docs-rule-card">
          <span class="ai-docs-rule-card__label">{{ item.title }}</span>
          <strong class="ai-docs-rule-card__value">{{ item.value }}</strong>
          <p class="ai-docs-rule-card__desc">{{ item.desc }}</p>
        </article>
      </div>
    </PortalContentCard>

    <div class="ai-docs-grid">
      <PortalContentCard title="Codex CLI" description="推荐写全局配置文件；项目级 .env 适合临时验证。">
        <ul class="ai-docs-list">
          <li>在平台「API 密钥」页面创建一个 <code>sk-</code> 前缀的密钥并复制。</li>
          <li>终端里安装 <code>@openai/codex</code>，然后把 Base URL 指向当前平台。</li>
          <li>Responses API 协议要保留 <code>wire_api = "responses"</code>。</li>
          <li>平台会把 Codex 请求的 <code>/responses</code> 规范化为 <code>/v1/responses</code>；已有的 <code>/v1</code> 配置仍可继续使用。</li>
        </ul>
        <PortalAiDocsCodeBlock title="~/.codex/config.toml" :code="codexConfigToml" />
        <PortalAiDocsCodeBlock title="~/.codex/auth.json" :code="codexAuthJson" />
        <PortalAiDocsCodeBlock title="项目 .env（可选）" :code="codexEnvFile" />
      </PortalContentCard>

      <PortalContentCard title="Claude Code" description="优先用 settings.json；如果碰到登录校验，再加 OAUTH_TOKEN 环境变量。">
        <ul class="ai-docs-list">
          <li>在平台「API 密钥」页面创建一个 <code>sk-</code> 前缀的密钥并复制。</li>
          <li>用 <code>ANTHROPIC_AUTH_TOKEN</code> 而不是 <code>ANTHROPIC_API_KEY</code>。</li>
          <li><code>ANTHROPIC_BASE_URL</code> 保持公开 Base URL，Claude Code 会自己请求 <code>/v1/messages</code>。</li>
          <li>如果启动时提示未登录，可以再注入 <code>CLAUDE_CODE_OAUTH_TOKEN</code>。</li>
        </ul>
        <PortalAiDocsCodeBlock title="~/.claude/settings.json" :code="claudeSettingsJson" />
        <PortalAiDocsCodeBlock title="临时环境变量" :code="claudeOAuthEnv" />
      </PortalContentCard>
    </div>

    <PortalContentCard title="排错要点" description="这类工具出问题时，绝大多数都是 Base URL 和密钥字段名写错。">
      <table class="ai-docs-table">
        <thead>
          <tr><th>现象</th><th>通常原因</th><th>处理方式</th></tr>
        </thead>
        <tbody>
          <tr>
            <td><code>401 Unauthorized</code></td>
            <td>密钥无效、未生效，或者把 <code>rk_</code> 当成 <code>sk_</code> 用了。</td>
            <td>重新确认使用 API 密钥，并检查环境变量是否被工具进程读取。</td>
          </tr>
          <tr>
            <td><code>404 / Not Found</code></td>
            <td>把接口路径当成 Base URL，或者把公开地址里已有的 <code>/ai</code> 又重复拼了一层。</td>
            <td>Codex CLI 和 Claude Code 都使用 <code>{{ baseUrl }}</code>；不要二次拼前缀。</td>
          </tr>
          <tr>
            <td><code>Not logged in</code></td>
            <td>Claude Code 触发了官方登录校验。</td>
            <td>补上 <code>CLAUDE_CODE_OAUTH_TOKEN</code>，跳过本地登录状态依赖。</td>
          </tr>
        </tbody>
      </table>
    </PortalContentCard>
  </div>
</template>

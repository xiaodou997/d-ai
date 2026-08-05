<script setup lang="ts">
import { computed } from "vue";

import PortalContentCard from "../../../page/PortalContentCard.vue";
import PortalAiDocsTabbedCode from "../PortalAiDocsTabbedCode.vue";

const props = defineProps<{
  baseUrl: string;
}>();

const endpoints = [
  { method: "GET", path: "/v1/models", desc: "列出当前账号可用的模型。" },
  { method: "POST", path: "/v1/chat/completions", desc: "对话接口，和 OpenAI SDK 兼容，适合大多数场景。" },
  { method: "POST", path: "/v1/responses", desc: "OpenAI Responses API，支持多轮对话状态。" },
  { method: "POST", path: "/v1/messages", desc: "Anthropic 对话接口，可以直接用 Claude SDK。" },
  { method: "POST", path: "/v1/embeddings", desc: "文本向量化接口，适合搜索和 RAG 场景。" }
];

const chatExamples = computed(() => [
  {
    key: "curl",
    label: "cURL",
    code: `curl ${props.baseUrl}/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer sk_xxx" \\
  -d '{
    "model": "gpt-4.1-mini",
    "messages": [
      {"role": "system", "content": "You are a concise assistant."},
      {"role": "user", "content": "请用三点总结这个需求"}
    ],
    "stream": false
  }'`
  },
  {
    key: "python",
    label: "Python",
    code: `from openai import OpenAI

client = OpenAI(
    api_key="sk_xxx",
    base_url="${props.baseUrl}",
)

resp = client.chat.completions.create(
    model="gpt-4.1-mini",
    messages=[
        {"role": "system", "content": "You are a concise assistant."},
        {"role": "user", "content": "请用三点总结这个需求"},
    ],
)

print(resp.choices[0].message.content)`
  },
  {
    key: "node",
    label: "Node",
    code: `import OpenAI from "openai";

const client = new OpenAI({
  apiKey: "sk_xxx",
  baseURL: "${props.baseUrl}",
});

const resp = await client.chat.completions.create({
  model: "gpt-4.1-mini",
  messages: [
    { role: "system", content: "You are a concise assistant." },
    { role: "user", content: "请用三点总结这个需求" }
  ]
});

console.log(resp.choices[0].message.content);`
  }
]);

const anthropicExamples = computed(() => [
  {
    key: "curl",
    label: "cURL",
    code: `curl ${props.baseUrl}/v1/messages \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer sk_xxx" \\
  -H "x-api-key: sk_xxx" \\
  -H "anthropic-version: 2023-06-01" \\
  -d '{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 512,
    "messages": [
      {"role": "user", "content": "请用三点总结这个需求"}
    ]
  }'`
  },
  {
    key: "python",
    label: "Python",
    code: `from anthropic import Anthropic

client = Anthropic(
    api_key="sk_xxx",
    base_url="${props.baseUrl}",
)

message = client.messages.create(
    model="claude-sonnet-4-20250514",
    max_tokens=512,
    messages=[
        {"role": "user", "content": "请用三点总结这个需求"},
    ],
)

print(message.content[0].text)`
  },
  {
    key: "node",
    label: "Node",
    code: `import Anthropic from "@anthropic-ai/sdk";

const client = new Anthropic({
  apiKey: "sk_xxx",
  baseURL: "${props.baseUrl}",
});

const message = await client.messages.create({
  model: "claude-sonnet-4-20250514",
  max_tokens: 512,
  messages: [
    { role: "user", content: "请用三点总结这个需求" }
  ],
});

console.log(message.content[0].text);`
  }
]);

const baseUrlRules = computed(() => [
  {
    title: "OpenAI 兼容 SDK",
    value: props.baseUrl,
    desc: "直接使用公开根地址；平台兼容 SDK 追加的无版本路径和标准 /v1 路径。"
  },
  {
    title: "Anthropic SDK / HTTP",
    value: props.baseUrl,
    desc: "直接用公开 Base URL；不要再手动裁掉或重复拼路径。"
  }
]);
</script>

<template>
  <div class="ai-docs-stack">
    <PortalContentCard title="接口清单" description="这组接口都使用 API 密钥（sk_），可以把平台当成 OpenAI / Anthropic 的替代地址。">
      <div class="ai-docs-endpoint-list">
        <article v-for="endpoint in endpoints" :key="endpoint.path" class="ai-docs-endpoint">
          <span class="ai-docs-endpoint__method">{{ endpoint.method }}</span>
          <div>
            <div class="ai-docs-endpoint__path">{{ endpoint.path }}</div>
            <p class="ai-docs-endpoint__desc">{{ endpoint.desc }}</p>
          </div>
        </article>
      </div>
      <div class="ai-docs-rule-grid">
        <article v-for="item in baseUrlRules" :key="item.title" class="ai-docs-rule-card">
          <span class="ai-docs-rule-card__label">{{ item.title }}</span>
          <strong class="ai-docs-rule-card__value">{{ item.value }}</strong>
          <p class="ai-docs-rule-card__desc">{{ item.desc }}</p>
        </article>
      </div>
    </PortalContentCard>

    <div class="ai-docs-grid">
      <PortalContentCard title="OpenAI 兼容调用" description="如果你已经在用 OpenAI SDK，通常只需要替换 Base URL 和 API key。">
        <PortalAiDocsTabbedCode title="POST /v1/chat/completions" :tabs="chatExamples" caption="SDK Base URL 使用平台根地址；/v1/chat/completions 与 /v1/responses 均保持兼容。" />
      </PortalContentCard>

      <PortalContentCard title="Anthropic 原生调用" description="用 Claude SDK 或直接发 HTTP 请求时，根地址不要补 /v1。">
        <PortalAiDocsTabbedCode
          title="POST /v1/messages"
          :tabs="anthropicExamples"
          :caption="`Authorization: Bearer 与 x-api-key 都可使用；同时发送时必须是同一密钥。根地址保持 ${baseUrl}。`"
        />
      </PortalContentCard>
    </div>

    <PortalContentCard title="调用要点" description="和官方接口的用法一致，直接沿用 OpenAI / Anthropic SDK 的习惯就好。">
      <table class="ai-docs-table">
        <thead>
          <tr><th>主题</th><th>说明</th></tr>
        </thead>
        <tbody>
          <tr>
            <td>模型名</td>
            <td>直接用平台“模型列表”里展示的模型编码，不需要猜测底层 provider 名称。</td>
          </tr>
          <tr>
            <td>流式输出</td>
            <td>把 <code>stream</code> 设为 <code>true</code> 即可；SDK 会按各自原生方式处理 SSE。</td>
          </tr>
          <tr>
            <td>Responses vs Chat</td>
            <td>如果你的工具链已围绕 OpenAI Responses API 设计，走 <code>/v1/responses</code>；否则普通对话走 <code>/v1/chat/completions</code> 就够。</td>
          </tr>
          <tr>
            <td>Embeddings</td>
            <td><code>/v1/embeddings</code> 是原始向量接口，不参与应用模板渲染，也不使用运行密钥。</td>
          </tr>
        </tbody>
      </table>
    </PortalContentCard>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";

import PortalContentCard from "../../../page/PortalContentCard.vue";
import type { PortalAiDocsScope } from "../../docs";
import { portalAiDocsAppLabel } from "../../docs";
import PortalAiDocsCodeBlock from "../PortalAiDocsCodeBlock.vue";

const props = defineProps<{
  baseUrl: string;
  scope: PortalAiDocsScope;
}>();

const authHeader = 'Authorization: Bearer YOUR_API_KEY';
const appLabel = computed(() => portalAiDocsAppLabel(props.scope));

const startItems = computed(() => [
  {
    title: "直接调模型",
    keyType: "sk_",
    desc: "使用 API 密钥访问 OpenAI / Anthropic 兼容接口，自己决定 model、messages、size 等参数。"
  },
  {
    title: "调封装后的应用",
    keyType: "rk_",
    desc: `使用应用密钥命中已经配置好的${appLabel.value}，调用方统一传 input 和所需 variables。`
  },
  {
    title: "公开地址",
    keyType: props.baseUrl,
    desc: "直接使用平台展示的 Base URL，不要手动追加管理接口或推理接口路径。"
  }
]);
</script>

<template>
  <div class="ai-docs-stack">
    <PortalContentCard title="开始之前" description="先选对密钥，再选对接口。两类入口解决的是两种完全不同的调用问题。">
      <div class="ai-docs-grid ai-docs-grid--three">
        <article v-for="item in startItems" :key="item.title" class="ai-docs-note">
          <strong class="ai-docs-note__head">{{ item.title }}</strong>
          <div class="ai-docs-note__chips">
            <span class="ai-docs-chip">{{ item.keyType }}</span>
          </div>
          <div class="ai-docs-note__body">{{ item.desc }}</div>
        </article>
      </div>
    </PortalContentCard>

    <div class="ai-docs-grid">
      <PortalContentCard title="公开 Base URL" description="直接用下面这个地址，不要自己拼路径。">
        <div class="ai-docs-rule-grid">
          <article class="ai-docs-rule-card">
            <span class="ai-docs-rule-card__label">OpenAI SDK</span>
            <strong class="ai-docs-rule-card__value">{{ baseUrl }}</strong>
            <p class="ai-docs-rule-card__desc">直接使用根地址；平台同时兼容标准 <code>/v1</code> 路径与 SDK 追加的无版本路径。</p>
          </article>
          <article class="ai-docs-rule-card">
            <span class="ai-docs-rule-card__label">Anthropic SDK</span>
            <strong class="ai-docs-rule-card__value">{{ baseUrl }}</strong>
            <p class="ai-docs-rule-card__desc">直接用 Base URL；SDK 会自动请求 <code>/v1/messages</code>。</p>
          </article>
          <article class="ai-docs-rule-card">
            <span class="ai-docs-rule-card__label">应用密钥</span>
            <strong class="ai-docs-rule-card__value">{{ baseUrl }}/v1/run</strong>
            <p class="ai-docs-rule-card__desc">应用密钥统一走这一个入口，绑定的应用类型决定具体行为。</p>
          </article>
        </div>
      </PortalContentCard>

      <PortalContentCard title="认证与密钥" description="所有入口支持标准 Bearer Token；Anthropic 原生接口同时兼容 x-api-key，两者并存时必须使用同一密钥。">
        <PortalAiDocsCodeBlock title="HTTP Header" :code="authHeader" :copy-text="authHeader" />
        <table class="ai-docs-table">
          <thead>
            <tr><th>密钥</th><th>适用接口</th><th>典型场景</th></tr>
          </thead>
          <tbody>
            <tr>
              <td><code>sk-</code></td>
              <td><code>/v1/chat/completions</code>、<code>/v1/responses</code>、<code>/v1/messages</code>、<code>/v1/images/generations</code></td>
              <td>你自己控制模型参数、自己组装请求体。</td>
            </tr>
            <tr>
              <td><code>rk_</code></td>
              <td><code>/v1/run</code>（统一入口，密钥绑定的应用类型决定行为）</td>
              <td>你调用的是一个已经封装好的应用，不接触底层模板和大多数原生图片参数。</td>
            </tr>
          </tbody>
        </table>
      </PortalContentCard>
    </div>

    <PortalContentCard title="应用是什么" description="应用不是另一个模型接口，而是给普通开发者消费的封装层。">
      <div class="ai-docs-grid ai-docs-grid--three">
        <article class="ai-docs-note">
          <strong class="ai-docs-note__head">提示词模板</strong>
          <div class="ai-docs-note__body">服务端持有模板正文，调用方只看到变量名，不直接接触底层提示词。</div>
        </article>
        <article class="ai-docs-note">
          <strong class="ai-docs-note__head">绑定模型与分组</strong>
          <div class="ai-docs-note__body">应用已经绑定好模型、分组和定价链路，调用方不用再选上游。</div>
        </article>
        <article class="ai-docs-note">
          <strong class="ai-docs-note__head">运行配置</strong>
          <div class="ai-docs-note__body">对话应用的温度等参数、生图应用的分辨率都由应用预设好，调用方不用自己设。</div>
        </article>
      </div>
      <p class="ai-docs-lead">
        真正的创建步骤放在“应用管理”页内联帮助里，这里只说明调用契约。创建完应用后，再去“应用密钥”页生成
        <span class="ai-docs-inline-code">rk_</span> 分发给调用方。
      </p>
    </PortalContentCard>

    <PortalContentCard title="从哪里开始" description="按你的接入方式选文档。">
      <table class="ai-docs-table">
        <thead>
          <tr><th>你要做什么</th><th>去哪里</th><th>为什么</th></tr>
        </thead>
        <tbody>
          <tr>
            <td>给服务端或脚本直接接模型</td>
            <td>API · 对话 / API · 生图</td>
            <td>你需要自己传模型名和参数，直接用 <code>sk_</code> 密钥更清晰。</td>
          </tr>
          <tr>
            <td>给业务方提供稳定能力</td>
            <td>应用密钥</td>
            <td>模型、模板、参数都由应用统一管理，调用方只需传 input 和变量。</td>
          </tr>
          <tr>
            <td>接 Codex CLI 或 Claude Code</td>
            <td>工具接入</td>
            <td>两者的 Base URL 规则不同，直接照配置片段抄就行。</td>
          </tr>
        </tbody>
      </table>
    </PortalContentCard>
  </div>
</template>

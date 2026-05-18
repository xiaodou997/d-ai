<script setup>
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { CopyDocument, Key, Link, Document, Cpu } from '@element-plus/icons-vue'

const BASE_URL = (import.meta.env.VITE_API_BASE_URL || window.location.origin).replace(/\/$/, '')

const activeTab = ref('curl')
const activeSection = ref('base-url')

const sections = [
  { id: 'base-url', label: '接入地址' },
  { id: 'auth', label: '认证方式' },
  { id: 'openai', label: 'OpenAI 兼容接口' },
  { id: 'anthropic', label: 'Anthropic 原生接口' },
  { id: 'code', label: '代码示例' },
]

const openaiEndpoints = [
  { method: 'GET',  path: '/v1/models',             desc: '列出当前账号可用的全部模型' },
  { method: 'POST', path: '/v1/chat/completions',    desc: '对话补全（流式 / 非流式），兼容 OpenAI SDK' },
  { method: 'POST', path: '/v1/responses',           desc: 'OpenAI Responses API，支持多轮对话状态' },
  { method: 'POST', path: '/v1/embeddings',          desc: '文本向量化，适用于 RAG 场景' },
  { method: 'POST', path: '/v1/images/generations',  desc: '文生图，兼容 DALL·E 格式' },
]

const anthropicEndpoints = [
  { method: 'POST', path: '/v1/messages',              desc: '原生 Anthropic Messages API，可直接使用 Anthropic SDK' },
  { method: 'POST', path: '/v1/messages/count_tokens', desc: '预估 Token 数量，不产生实际调用费用' },
]

const codeSamples = {
  curl: `curl ${BASE_URL}/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -d '{
    "model": "gpt-4o",
    "messages": [
      {
        "role": "user",
        "content": "Hello!"
      }
    ],
    "stream": false
  }'`,

  python: `from openai import OpenAI

client = OpenAI(
    api_key="YOUR_API_KEY",
    base_url="${BASE_URL}/v1",
)

response = client.chat.completions.create(
    model="gpt-4o",
    messages=[
        {"role": "user", "content": "Hello!"}
    ],
)

print(response.choices[0].message.content)`,

  nodejs: `import OpenAI from "openai";

const client = new OpenAI({
  apiKey: "YOUR_API_KEY",
  baseURL: "${BASE_URL}/v1",
});

const response = await client.chat.completions.create({
  model: "gpt-4o",
  messages: [{ role: "user", content: "Hello!" }],
});

console.log(response.choices[0].message.content);`,
}

function copy(text) {
  navigator.clipboard.writeText(text).then(() => {
    ElMessage({ message: '已复制到剪贴板', type: 'success', duration: 1500 })
  })
}

function scrollTo(id) {
  activeSection.value = id
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
</script>

<template>
  <div class="docs-root">

    <!-- 页头 -->
    <div class="flex flex-col lg:flex-row lg:items-center justify-between gap-4 bg-white p-6 rounded-2xl border border-slate-50 shadow-soft mb-6">
      <div>
        <p class="text-[10px] font-black text-slate-400 uppercase tracking-widest mb-1">Uni AI API</p>
        <h1 class="text-2xl font-black text-slate-800 tracking-tight">接入文档</h1>
        <p class="text-slate-400 text-sm font-medium mt-1">
          API 接入地址、认证方式与全部接口参考 — 与任何 OpenAI / Anthropic 兼容客户端配合使用。
        </p>
      </div>
    </div>

    <!-- 双栏布局 -->
    <div class="docs-layout">

      <!-- 左侧目录 -->
      <aside class="docs-toc">
        <p class="toc-title">本页内容</p>
        <nav>
          <button
            v-for="s in sections"
            :key="s.id"
            class="toc-item"
            :class="{ 'toc-item--active': activeSection === s.id }"
            @click="scrollTo(s.id)"
          >
            {{ s.label }}
          </button>
        </nav>
      </aside>

      <!-- 右侧内容 -->
      <main class="docs-content">

        <!-- 1. 接入地址 -->
        <section id="base-url" class="doc-card">
          <div class="section-header">
            <el-icon class="section-icon"><Link /></el-icon>
            <h2>接入地址</h2>
          </div>
          <p class="section-desc">
            所有 API 请求均以此地址为前缀，兼容 OpenAI SDK 的 <code>baseURL</code> 参数和 Anthropic SDK 的 <code>base_url</code> 参数。
          </p>
          <div class="url-bar">
            <span class="url-label">Base URL</span>
            <span class="url-value">{{ BASE_URL }}</span>
            <el-button size="small" :icon="CopyDocument" circle @click="copy(BASE_URL)" />
          </div>
          <div class="tip-box tip-box--green">
            <span class="tip-dot tip-dot--green" />
            将此地址填入 SDK 的 <code>base_url</code> / <code>baseURL</code> 参数，即可无缝替换 OpenAI / Anthropic 官方端点。
          </div>
        </section>

        <!-- 2. 认证方式 -->
        <section id="auth" class="doc-card">
          <div class="section-header">
            <el-icon class="section-icon"><Key /></el-icon>
            <h2>认证方式</h2>
          </div>
          <p class="section-desc">
            每个请求须在 HTTP 请求头中携带 API Key，格式为标准的 Bearer Token。
          </p>

          <div class="code-block">
            <div class="code-block__bar">
              <span>HTTP Header</span>
              <el-button size="small" :icon="CopyDocument" link @click="copy('Authorization: Bearer YOUR_API_KEY')">复制</el-button>
            </div>
            <pre><span class="kw">Authorization</span><span class="pu">:</span> Bearer <span class="str">YOUR_API_KEY</span></pre>
          </div>

          <table class="info-table">
            <thead>
              <tr><th>项目</th><th>说明</th></tr>
            </thead>
            <tbody>
              <tr><td>API Key 格式</td><td><code>sk-</code> 前缀的字符串</td></tr>
              <tr><td>Key 获取途径</td><td>在"API Key"页面创建，或分发给终端用户</td></tr>
              <tr><td>传输要求</td><td>生产环境必须使用 HTTPS，明文传输会导致 Key 泄漏</td></tr>
              <tr><td>Key 权限范围</td><td>每个 Key 只能访问其被授权的模型列表</td></tr>
            </tbody>
          </table>
        </section>

        <!-- 3. OpenAI 兼容接口 -->
        <section id="openai" class="doc-card">
          <div class="section-header">
            <el-icon class="section-icon"><Cpu /></el-icon>
            <h2>OpenAI 兼容接口</h2>
          </div>
          <p class="section-desc">
            以下接口完全兼容 OpenAI 协议，可直接替换 <code>https://api.openai.com</code> 地址使用，无需修改客户端代码。
          </p>
          <div class="endpoint-list">
            <div v-for="ep in openaiEndpoints" :key="ep.path" class="endpoint-row">
              <span class="method-badge" :class="`method-badge--${ep.method.toLowerCase()}`">{{ ep.method }}</span>
              <code class="ep-path">{{ ep.path }}</code>
              <span class="ep-desc">{{ ep.desc }}</span>
            </div>
          </div>
        </section>

        <!-- 4. Anthropic 原生接口 -->
        <section id="anthropic" class="doc-card">
          <div class="section-header">
            <el-icon class="section-icon"><Document /></el-icon>
            <h2>Anthropic 原生接口</h2>
          </div>
          <p class="section-desc">
            以下接口兼容 Anthropic 官方协议，可直接替换 <code>https://api.anthropic.com</code>，配合 <code>anthropic</code> SDK 使用。
          </p>
          <div class="endpoint-list">
            <div v-for="ep in anthropicEndpoints" :key="ep.path" class="endpoint-row">
              <span class="method-badge method-badge--post">{{ ep.method }}</span>
              <code class="ep-path">{{ ep.path }}</code>
              <span class="ep-desc">{{ ep.desc }}</span>
            </div>
          </div>
          <div class="tip-box tip-box--amber">
            <span class="tip-dot tip-dot--amber" />
            使用 Anthropic SDK 时，需同时设置 <code>base_url</code> 和 <code>api_key</code> 参数，并可将 <code>x-api-key</code> Header 省略（系统自动路由）。
          </div>
        </section>

        <!-- 5. 代码示例 -->
        <section id="code" class="doc-card">
          <div class="section-header">
            <el-icon class="section-icon"><CopyDocument /></el-icon>
            <h2>代码示例</h2>
          </div>
          <p class="section-desc">
            以下示例演示如何调用 <code>POST /v1/chat/completions</code>，将 <code>YOUR_API_KEY</code> 替换为实际 Key 即可运行。
          </p>

          <div class="code-block">
            <div class="code-block__bar">
              <div class="tab-group">
                <button
                  v-for="tab in [{ key: 'curl', label: 'cURL' }, { key: 'python', label: 'Python' }, { key: 'nodejs', label: 'Node.js' }]"
                  :key="tab.key"
                  class="tab-btn"
                  :class="{ 'tab-btn--active': activeTab === tab.key }"
                  @click="activeTab = tab.key"
                >
                  {{ tab.label }}
                </button>
              </div>
              <el-button size="small" :icon="CopyDocument" link @click="copy(codeSamples[activeTab])">复制代码</el-button>
            </div>
            <pre class="code-body">{{ codeSamples[activeTab] }}</pre>
          </div>
        </section>

      </main>
    </div>
  </div>
</template>

<style scoped lang="scss">
.docs-root {
  min-height: 100%;
}

.docs-layout {
  display: flex;
  gap: 24px;
  align-items: flex-start;
}

/* ── 左侧目录 ─────────────────────────────────── */
.docs-toc {
  position: sticky;
  top: 24px;
  width: 180px;
  flex-shrink: 0;
  background: #fff;
  border: 1px solid #f1f5f9;
  border-radius: 16px;
  padding: 16px 12px;
  box-shadow: 0 4px 12px rgba(15, 23, 42, 0.04);
}

.toc-title {
  margin: 0 0 10px 8px;
  font-size: 11px;
  font-weight: 900;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: #94a3b8;
}

.toc-item {
  display: block;
  width: 100%;
  text-align: left;
  background: none;
  border: none;
  cursor: pointer;
  padding: 8px 12px;
  border-radius: 10px;
  font-size: 13px;
  font-weight: 500;
  color: #64748b;
  transition: all 0.15s ease;
  margin-bottom: 2px;

  &:hover {
    background: #f8fafc;
    color: #334155;
  }

  &--active {
    background: #eff6ff;
    color: #3b82f6;
    font-weight: 700;
  }
}

/* ── 右侧内容 ─────────────────────────────────── */
.docs-content {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* ── 文档卡片 ──────────────────────────────────── */
.doc-card {
  background: #fff;
  border: 1px solid #f1f5f9;
  border-radius: 16px;
  padding: 28px;
  box-shadow: 0 4px 16px rgba(15, 23, 42, 0.04);
  scroll-margin-top: 24px;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;

  h2 {
    margin: 0;
    font-size: 17px;
    font-weight: 800;
    color: #0f172a;
  }
}

.section-icon {
  color: #0ea5e9;
  font-size: 20px;
}

.section-desc {
  margin: 0 0 20px;
  font-size: 14px;
  color: #64748b;
  line-height: 1.7;

  code {
    font-family: 'JetBrains Mono', 'Fira Code', monospace;
    font-size: 12.5px;
    background: #f1f5f9;
    color: #475569;
    padding: 1px 5px;
    border-radius: 4px;
  }
}

/* ── 接入地址栏 ────────────────────────────────── */
.url-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  padding: 12px 16px;
  margin-bottom: 16px;
}

.url-label {
  flex-shrink: 0;
  font-size: 11px;
  font-weight: 900;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: #0ea5e9;
  background: #e0f2fe;
  padding: 3px 8px;
  border-radius: 6px;
}

.url-value {
  flex: 1;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 13.5px;
  color: #1e293b;
  font-weight: 600;
  word-break: break-all;
}

/* ── 提示框 ────────────────────────────────────── */
.tip-box {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  border-radius: 10px;
  padding: 12px 16px;
  font-size: 13px;
  line-height: 1.6;
  margin-top: 16px;

  code {
    font-family: 'JetBrains Mono', 'Fira Code', monospace;
    font-size: 12px;
    background: rgba(0, 0, 0, 0.06);
    padding: 1px 4px;
    border-radius: 3px;
  }

  &--green {
    background: #f0fdf4;
    border: 1px solid #bbf7d0;
    color: #166534;
  }

  &--amber {
    background: #fffbeb;
    border: 1px solid #fde68a;
    color: #92400e;
  }
}

.tip-dot {
  flex-shrink: 0;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  margin-top: 5px;

  &--green { background: #22c55e; }
  &--amber { background: #f59e0b; }
}

/* ── 信息表格 ──────────────────────────────────── */
.info-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13.5px;
  margin-top: 16px;

  th {
    text-align: left;
    padding: 10px 14px;
    background: #f8fafc;
    color: #64748b;
    font-size: 11px;
    font-weight: 900;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    border-bottom: 1px solid #f1f5f9;

    &:first-child { border-radius: 10px 0 0 10px; }
    &:last-child  { border-radius: 0 10px 10px 0; }
  }

  td {
    padding: 11px 14px;
    color: #334155;
    border-bottom: 1px solid #f8fafc;
    vertical-align: top;

    code {
      font-family: 'JetBrains Mono', 'Fira Code', monospace;
      font-size: 12px;
      background: #f1f5f9;
      color: #475569;
      padding: 1px 5px;
      border-radius: 4px;
    }
  }

  tr:last-child td { border-bottom: none; }
}

/* ── 接口列表 ──────────────────────────────────── */
.endpoint-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.endpoint-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 13px 16px;
  background: #f8fafc;
  border: 1px solid #f1f5f9;
  border-radius: 12px;
  transition: border-color 0.15s;

  &:hover {
    border-color: #e2e8f0;
    background: #fff;
  }
}

.method-badge {
  flex-shrink: 0;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 11px;
  font-weight: 800;
  padding: 3px 8px;
  border-radius: 6px;
  min-width: 44px;
  text-align: center;

  &--get  { background: #dcfce7; color: #15803d; }
  &--post { background: #dbeafe; color: #1d4ed8; }
}

.ep-path {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 13px;
  color: #1e293b;
  font-weight: 600;
  flex-shrink: 0;
}

.ep-desc {
  font-size: 13px;
  color: #64748b;
  flex: 1;
  min-width: 0;
}

/* ── 代码块 ────────────────────────────────────── */
.code-block {
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  overflow: hidden;
}

.code-block__bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px;
  background: #f8fafc;
  border-bottom: 1px solid #e2e8f0;
  font-size: 12px;
  font-weight: 600;
  color: #64748b;
}

.tab-group {
  display: flex;
  gap: 4px;
}

.tab-btn {
  background: none;
  border: none;
  cursor: pointer;
  padding: 5px 12px;
  border-radius: 8px;
  font-size: 12.5px;
  font-weight: 600;
  color: #94a3b8;
  transition: all 0.15s;

  &:hover { background: #e2e8f0; color: #475569; }

  &--active {
    background: #fff;
    color: #0ea5e9;
    box-shadow: 0 1px 4px rgba(0, 0, 0, 0.08);
  }
}

pre {
  margin: 0;
  padding: 20px 22px;
  background: #1e293b;
  color: #e2e8f0;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 13px;
  line-height: 1.75;
  overflow-x: auto;
  white-space: pre;

  .kw  { color: #7dd3fc; }
  .pu  { color: #94a3b8; }
  .str { color: #86efac; }
}
</style>

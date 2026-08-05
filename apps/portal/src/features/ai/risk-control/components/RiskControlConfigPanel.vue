<!--
  风控中心配置入口 — 渲染在 RiskControlWorkspace 页头 #actions:状态徽章 + 配置按钮,
  以及风控配置 el-dialog(过渡期仍为 element-plus)。
  重构：PortalPageHeader 外壳移除(页头由 Workspace 的 PortalPagePanel 承担),
       状态 el-tag 换成 DsTag;弹窗、表单与业务逻辑不变。
-->
<script setup lang="ts">
import { onMounted } from 'vue'
import { Setting } from '@element-plus/icons-vue'
import { DsTag } from '@dai/ui'

import { useRiskControlConfig } from '../composables/useRiskControlConfig'

const modeOptions = [
  { label: '关闭', value: 'off' },
  { label: '旁路观察（不拦截，仅记录）', value: 'observe' },
  { label: '同步拦截', value: 'pre_block' }
]
const levelOptions = [
  { label: '拦截 (block)', value: 'block' },
  { label: '观察 (suspect)', value: 'suspect' }
]
const hitLayerLabels: Record<string, string> = {
  cache: '缓存',
  keyword: '关键词',
  pinyin: '拼音',
  api: 'API'
}
const thresholdLabels: Record<string, string> = {
  harassment: '骚扰',
  'harassment/threatening': '骚扰（威胁性）',
  hate: '仇恨言论',
  'hate/threatening': '仇恨言论（威胁性）',
  illicit: '违法内容',
  'illicit/violent': '违法内容（暴力）',
  'self-harm': '自残',
  'self-harm/intent': '自残（意图）',
  'self-harm/instructions': '自残（教程）',
  sexual: '色情',
  'sexual/minors': '涉未成年色情',
  violence: '暴力',
  'violence/graphic': '暴力（血腥画面）'
}

const {
  apiKeyInput,
  config,
  configDialogVisible,
  configLoading,
  configSaving,
  fetchConfig,
  form,
  openConfigDialog,
  runTest,
  saveConfig,
  statusSummary,
  testResult,
  testText,
  testing,
  addKeywordEntry,
  removeKeywordEntry,
  addPinyinEntry,
  removePinyinEntry
} = useRiskControlConfig()

onMounted(fetchConfig)
</script>

<template>
  <DsTag :tone="config?.enabled ? 'positive' : 'info'">{{ statusSummary }}</DsTag>
  <el-button :icon="Setting" type="primary" :loading="configLoading" @click="openConfigDialog">配置</el-button>

  <el-dialog v-model="configDialogVisible" title="风控中心配置" width="860px" append-to-body>
    <el-form :model="form" label-position="top" class="risk-control-form">
      <!-- 基础开关 -->
      <section class="dialog-section">
        <div class="dialog-section-head">
          <h4>基础开关</h4>
        </div>
        <div class="dialog-grid">
          <el-form-item label="总开关">
            <el-switch v-model="form.enabled" />
          </el-form-item>
          <el-form-item label="运行模式">
            <el-select v-model="form.mode" class="full-field">
              <el-option v-for="item in modeOptions" :key="item.value" :label="item.label" :value="item.value" />
            </el-select>
          </el-form-item>
        </div>
      </section>

      <!-- 关键词引擎 -->
      <section class="dialog-section">
        <div class="dialog-section-head">
          <h4>关键词引擎（L1）</h4>
          <p>AC 自动机 + 归一化（全角/繁体/同形字/干扰符）+ 可选拼音匹配。</p>
        </div>
        <el-form-item label="关键词检测">
          <el-switch v-model="form.keyword_enabled" />
        </el-form-item>

        <div v-if="form.keyword_enabled" class="keyword-editor">
          <div class="keyword-editor-head">
            <span class="keyword-editor-title">关键词词库</span>
            <el-button size="small" @click="addKeywordEntry">+ 添加</el-button>
          </div>
          <el-table :data="form.keyword_entries" border size="small" class="keyword-table">
            <el-table-column label="词条" min-width="140">
              <template #default="{ row }">
                <el-input v-model="row.word" placeholder="敏感词" size="small" />
              </template>
            </el-table-column>
            <el-table-column label="级别" width="130">
              <template #default="{ row }">
                <el-select v-model="row.level" size="small" class="full-field">
                  <el-option v-for="opt in levelOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
                </el-select>
              </template>
            </el-table-column>
            <el-table-column label="共现词（逗号分隔）" min-width="160">
              <template #default="{ row }">
                <el-input
                  :model-value="row.require_with.join(', ')"
                  placeholder="可选，如：买, 获取"
                  size="small"
                  @update:model-value="(v: string) => (row.require_with = v.split(',').map((s) => s.trim()).filter(Boolean))"
                />
              </template>
            </el-table-column>
            <el-table-column label="备注" min-width="100">
              <template #default="{ row }">
                <el-input v-model="row.note" placeholder="" size="small" />
              </template>
            </el-table-column>
            <el-table-column label="" width="60" align="center">
              <template #default="{ $index }">
                <el-button type="danger" link size="small" @click="removeKeywordEntry($index)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>

        <!-- 拼音匹配 -->
        <el-divider content-position="left">
          <el-switch v-model="form.pinyin_enabled" inline-prompt active-text="拼音匹配" inactive-text="拼音匹配" />
        </el-divider>
        <p class="pinyin-hint">
          拼音词库独立维护，只放容易被谐音绕过的词（如品牌名、违禁品名），避免多音字误报。
        </p>
        <div v-if="form.pinyin_enabled" class="keyword-editor">
          <div class="keyword-editor-head">
            <span class="keyword-editor-title">拼音词库</span>
            <el-button size="small" @click="addPinyinEntry">+ 添加</el-button>
          </div>
          <el-table :data="form.pinyin_entries" border size="small" class="keyword-table">
            <el-table-column label="词条" min-width="140">
              <template #default="{ row }">
                <el-input v-model="row.word" placeholder="如：微信" size="small" />
              </template>
            </el-table-column>
            <el-table-column label="级别" width="130">
              <template #default="{ row }">
                <el-select v-model="row.level" size="small" class="full-field">
                  <el-option v-for="opt in levelOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
                </el-select>
              </template>
            </el-table-column>
            <el-table-column label="备注" min-width="100">
              <template #default="{ row }">
                <el-input v-model="row.note" placeholder="" size="small" />
              </template>
            </el-table-column>
            <el-table-column label="" width="60" align="center">
              <template #default="{ $index }">
                <el-button type="danger" link size="small" @click="removePinyinEntry($index)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </section>

      <!-- 审核 API -->
      <section class="dialog-section">
        <div class="dialog-section-head">
          <h4>审核 API（Provider）</h4>
          <p>兼容 OpenAI Moderation 协议（POST {base_url}/v1/moderations）。</p>
        </div>
        <div class="dialog-grid">
          <el-form-item label="Base URL">
            <el-input v-model="form.base_url" placeholder="https://api.openai.com" />
          </el-form-item>
          <el-form-item label="模型">
            <el-input v-model="form.model" placeholder="omni-moderation-latest" />
          </el-form-item>
          <el-form-item label="超时（毫秒）">
            <el-input-number v-model="form.timeout_ms" :min="500" :step="500" :controls="false" class="full-field" />
          </el-form-item>
          <el-form-item label="API Key">
            <el-input v-model="apiKeyInput" show-password placeholder="留空则保留原值" />
          </el-form-item>
        </div>
        <el-form-item label="审核 API 采样率">
          <el-slider v-model="form.sample_rate" :min="0" :max="1" :step="0.05" show-input />
        </el-form-item>
      </section>

      <!-- 裁决缓存 -->
      <section class="dialog-section">
        <div class="dialog-section-head">
          <h4>L0 裁决缓存</h4>
          <p>同一段文本在 TTL 内只检测一次，不重复写日志、不计入违规累计。</p>
        </div>
        <el-form-item label="缓存 TTL（秒，0=禁用）">
          <el-input-number v-model="form.verdict_cache_ttl_seconds" :min="0" :step="60" :controls="false" class="full-field" />
        </el-form-item>
      </section>

      <!-- 分类阈值 -->
      <section class="dialog-section">
        <div class="dialog-section-head">
          <h4>分类阈值</h4>
          <p>达到阈值即判定命中该分类；分数来自审核 API 返回。</p>
        </div>
        <div class="threshold-grid">
          <div v-for="(_, key) in form.thresholds" :key="key" class="threshold-item">
            <span class="threshold-label">{{ thresholdLabels[key] || key }}</span>
            <el-slider v-model="form.thresholds[key]" :min="0" :max="1" :step="0.01" show-input size="small" />
          </div>
        </div>
      </section>

      <!-- 风险事件与拦截响应 -->
      <section class="dialog-section">
        <div class="dialog-section-head">
          <h4>风险事件与拦截响应</h4>
        </div>
        <div class="dialog-grid">
          <el-form-item label="违规滚动窗口（小时）">
            <el-input-number v-model="form.violation_window_hours" :min="1" :controls="false" class="full-field" />
          </el-form-item>
          <el-form-item label="生成风险事件的累计命中次数">
            <el-input-number v-model="form.risk_event_threshold" :min="1" :controls="false" class="full-field" />
          </el-form-item>
          <el-form-item label="记录未命中结果">
            <el-switch v-model="form.record_non_hits" />
          </el-form-item>
          <el-form-item label="拦截 HTTP 状态码">
            <el-input-number v-model="form.block_status_code" :min="400" :max="599" :controls="false" class="full-field" />
          </el-form-item>
        </div>
        <el-form-item label="拦截提示文案">
          <el-input v-model="form.block_message" />
        </el-form-item>
      </section>

      <!-- 测试检测 -->
      <section class="dialog-section">
        <div class="dialog-section-head">
          <h4>测试检测</h4>
          <p>用当前弹窗中的设置试跑一段文本，不落库。保存前先点一次"保存"以生效最新配置。</p>
        </div>
        <el-input v-model="testText" type="textarea" :rows="2" placeholder="输入一段文本试跑检测" />
        <div class="test-actions">
          <el-button :loading="testing" @click="runTest">测试检测</el-button>
        </div>
        <el-alert
          v-if="testResult"
          :type="testResult.flagged ? 'error' : 'success'"
          :closable="false"
          class="test-result"
        >
          <pre class="test-result-json">{{ JSON.stringify(testResult, null, 2) }}</pre>
        </el-alert>
      </section>
    </el-form>
    <template #footer>
      <el-button @click="configDialogVisible = false">取消</el-button>
      <el-button type="primary" :loading="configSaving" @click="saveConfig">保存</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.dialog-section {
  margin-bottom: 20px;
  padding-bottom: 16px;
  border-bottom: 1px solid var(--ds-line);
}

.dialog-section:last-child {
  border-bottom: none;
}

.dialog-section-head h4 {
  margin: 0 0 4px;
  font-size: 14px;
  font-weight: 600;
}

.dialog-section-head p {
  margin: 0 0 10px;
  color: var(--ds-muted);
  font-size: 12px;
}

.dialog-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 20px;
}

.full-field {
  width: 100%;
}

.threshold-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 6px 24px;
}

.threshold-item {
  display: flex;
  align-items: center;
  gap: 12px;
}

.threshold-label {
  flex: 0 0 140px;
  font-size: 13px;
  color: var(--ds-ink);
}

.keyword-editor {
  margin-top: 8px;
}

.keyword-editor-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.keyword-editor-title {
  font-size: 13px;
  font-weight: 500;
}

.keyword-table {
  margin-bottom: 8px;
}

.pinyin-hint {
  margin: 0 0 8px;
  color: var(--ds-muted);
  font-size: 12px;
}

.test-actions {
  margin: 10px 0;
}

.test-result-json {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
  font-size: 12px;
}
</style>

<!--
  管理端路由策略（多维评分权重）。
  适配 v4：aiGateway getRouteWeights/putRouteWeights → aiAdminApi（v4 GET/PUT /api/v1/route-weights/{scope}
  返回 strong-typed {scope, weights:{cost,latency,load,health}}，与 V1 形状一致）；错误读 err.message。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       两张内容卡收进同卡 body 的 24px 容器）；权重条颜色改用 --ds-* token,无硬编码色值;
       业务逻辑与请求参数保持不变。无分页接口,不加分页。
-->
<script setup lang="ts">
import { computed, onMounted, reactive, shallowRef } from 'vue'
import { ElMessage } from 'element-plus'
import { Route } from 'lucide-vue-next'
import { PortalContentCard, PortalPagePanel } from '@/platform'
import { aiAdminApi } from '@/api/aiAdmin'
import type { RouteWeightsOutputBody, ScoreWeightsDTO } from '@/api/types/ai'

type RouteDimensionKey = keyof ScoreWeightsDTO

interface RouteDimensionOption {
  key: RouteDimensionKey
  label: string
  code: RouteDimensionKey
  color: string
  hint: string
  detail: string
}

interface RoutingPreset {
  key: string
  name: string
  values: ScoreWeightsDTO
  desc: string
}

const routeDimensionKeys: RouteDimensionKey[] = ['cost', 'latency', 'load', 'health']

// ── State ───────────────────────────────────────────────────────────────────
const loading = shallowRef(false)
const saving = shallowRef(false)
const scope = shallowRef('global')

const weights = reactive<ScoreWeightsDTO>({
  cost: 0.4,
  latency: 0.3,
  load: 0.2,
  health: 0.1
})

const dimensionOptions: RouteDimensionOption[] = [
  {
    key: 'cost',
    label: '成本优先',
    code: 'cost',
    color: 'var(--ds-info)',
    hint: '越重视成本，系统越倾向选择单价更低或账号池类路线。',
    detail: '适合预算敏感、批量任务、后台分析等场景。调得太高时，可能会牺牲一部分响应速度。'
  },
  {
    key: 'latency',
    label: '响应速度',
    code: 'latency',
    color: 'var(--ds-positive)',
    hint: '越重视速度，系统越倾向选择最近响应更快的路线。',
    detail: '适合聊天、代码补全、实时交互等场景。系统使用最近一段时间的平均耗时来判断，不只看某一次请求。'
  },
  {
    key: 'load',
    label: '繁忙程度',
    code: 'load',
    color: 'var(--ds-warning)',
    hint: '越重视空闲，系统越倾向选择当前排队更少的路线。',
    detail: '适合高并发场景，可减少所有请求挤到同一路线。调高后流量会更分散，但单次可能不总是走最低成本路线。'
  },
  {
    key: 'health',
    label: '可用状态',
    code: 'health',
    color: 'var(--ds-danger)',
    hint: '越重视稳定，系统越会避开刚失败过、正在恢复探测的路线。',
    detail: '建议至少保留 0.10。完全调低会让系统更少参考健康状态，故障恢复期的路线可能更早拿到流量。'
  }
]

const routingSteps = [
  {
    title: '先找可用路线',
    desc: '系统会根据当前模型找出可用的上游路线，已经不可用或本次已失败过的路线会被跳过。'
  },
  {
    title: '再按四项打分',
    desc: '每条路线都会同时看成本、速度、繁忙程度和可用状态。你在上面设置的比例越高，这一项对最终选择影响越大。'
  },
  {
    title: '按概率选择',
    desc: '系统不是永远选择分数最高的一条，而是让高分路线更容易被选中，这样可以避免流量全部压到同一条路线。'
  },
  {
    title: '失败后自动换路',
    desc: '如果一次调用失败，系统会在重试预算内换其他候选路线；连续失败的路线会进入保护期，暂时不再承接正常流量。'
  }
]

const tuningPresets: RoutingPreset[] = [
  {
    key: 'balanced',
    name: '默认均衡',
    values: { cost: 0.4, latency: 0.3, load: 0.2, health: 0.1 },
    desc: '适合大多数模型调用，成本、速度和均衡分流都有考虑。'
  },
  {
    key: 'cost-first',
    name: '成本优先',
    values: { cost: 0.55, latency: 0.2, load: 0.15, health: 0.1 },
    desc: '适合批量生成、离线任务、预算压力较大的业务。'
  },
  {
    key: 'experience-first',
    name: '体验优先',
    values: { cost: 0.2, latency: 0.5, load: 0.2, health: 0.1 },
    desc: '适合用户正在等待结果的在线请求，例如聊天和实时助手。'
  },
  {
    key: 'stability-first',
    name: '稳定优先',
    values: { cost: 0.25, latency: 0.25, load: 0.2, health: 0.3 },
    desc: '适合对失败率更敏感的场景，例如付费用户、关键业务或高峰期。'
  }
]

const termExplanations = [
  {
    title: '归一化',
    desc: '把四个数字按原来的比例自动换算成合计 1.00。比如 4、3、2、1 会变成 0.40、0.30、0.20、0.10。'
  },
  {
    title: '概率选择',
    desc: '分数越高越容易被选中，但不是每次都固定选同一条。这样既能偏向好路线，也能保留分流和容错空间。'
  },
  {
    title: '会话保持',
    desc: '请求带有 X-Conversation-Id 时，系统会尽量复用上次成功的路线或账号，让同一段对话更连续。'
  },
  {
    title: '保护期',
    desc: '一条路线连续失败后会暂时停止承接正常流量。等待一段时间后，系统只放少量探测请求确认它是否恢复。'
  }
]

// ── Computed ────────────────────────────────────────────────────────────────
const weightSum = computed(() => +(weights.cost + weights.latency + weights.load + weights.health).toFixed(4))
const weightSumOk = computed(() => Math.abs(weightSum.value - 1.0) < 0.001)
const sumGapText = computed(() => {
  const diff = +(weightSum.value - 1).toFixed(3)
  if (diff > 0) return `超出 ${diff.toFixed(3)}`
  return `还差 ${Math.abs(diff).toFixed(3)}`
})

function weightPercent(key: RouteDimensionKey) {
  return ((weights[key] || 0) * 100).toFixed(1)
}

function matchesPreset(preset: RoutingPreset) {
  return routeDimensionKeys.every((key) => {
    return Math.abs(weights[key] - preset.values[key]) < 0.001
  })
}

const activePreset = computed(() => tuningPresets.find(matchesPreset)?.key || '')

function applyPreset(preset: RoutingPreset) {
  weights.cost = preset.values.cost
  weights.latency = preset.values.latency
  weights.load = preset.values.load
  weights.health = preset.values.health
  ElMessage.success(`已套用“${preset.name}”`)
}

// ── Actions ──────────────────────────────────────────────────────────────────
async function fetchWeights() {
  loading.value = true
  try {
    const data: RouteWeightsOutputBody = await aiAdminApi.getRouteWeights(scope.value)
    if (data?.weights) {
      weights.cost = data.weights.cost ?? 0.4
      weights.latency = data.weights.latency ?? 0.3
      weights.load = data.weights.load ?? 0.2
      weights.health = data.weights.health ?? 0.1
    }
  } catch (e: any) {
    ElMessage.error('加载评分权重失败：' + (e?.message || e))
  } finally {
    loading.value = false
  }
}

async function saveWeights() {
  if (!weightSumOk.value) {
    ElMessage.warning('四项权重之和必须等于 1.0，当前合计 ' + weightSum.value)
    return
  }
  saving.value = true
  try {
    await aiAdminApi.putRouteWeights(scope.value, {
      cost: weights.cost,
      latency: weights.latency,
      load: weights.load,
      health: weights.health
    })
    ElMessage.success('评分权重已保存，下次请求生效')
  } catch (e: any) {
    ElMessage.error('保存失败：' + (e?.message || e))
  } finally {
    saving.value = false
  }
}

function normalize() {
  const sum = weightSum.value
  if (sum <= 0) return
  weights.cost = +(weights.cost / sum).toFixed(3)
  weights.latency = +(weights.latency / sum).toFixed(3)
  weights.load = +(weights.load / sum).toFixed(3)
  weights.health = +(1 - weights.cost - weights.latency - weights.load).toFixed(3)
  ElMessage.success('已按当前比例归一化为合计 1.0')
}

onMounted(fetchWeights)
</script>

<template>
  <div class="routing-page">
    <PortalPagePanel
      :icon="Route"
      :breadcrumbs="[{ label: 'AI 网关' }, { label: '路由策略' }]"
      description="设置系统选上游路线时更看重什么：更省钱、更快、更空闲，还是更稳定。"
    >
      <!-- 配置 + 说明两张内容卡:body 无内边距,用 24px 容器承载 -->
      <div class="routing-body">
    <!-- 评分权重配置卡 -->
    <PortalContentCard class="config-card" v-loading="loading">
      <template #header>
        <div>
          <h3 class="card-title">多维评分路由权重</h3>
          <p class="card-desc">
            四项合计必须为 <strong>1.0</strong>（当前合计：
            <span :class="weightSumOk ? 'sum-ok' : 'sum-err'">{{ weightSum }}</span>）。
            <span v-if="!weightSumOk" class="sum-err-msg">权重之和需为 1.0（{{ sumGapText }}）</span>
          </p>
        </div>
      </template>
      <template #actions>
        <el-button size="small" @click="fetchWeights" :loading="loading">刷新</el-button>
        <el-button size="small" type="warning" plain @click="normalize">按比例修正</el-button>
        <el-button size="small" type="primary" :loading="saving" :disabled="!weightSumOk" @click="saveWeights">保存权重</el-button>
      </template>

      <el-form label-width="120px" label-position="left">
        <el-form-item
          v-for="dim in dimensionOptions"
          :key="dim.key"
          :label="dim.label"
        >
          <div class="weight-row">
            <el-slider
              v-model="weights[dim.key]"
              :min="0" :max="1" :step="0.01"
              :show-tooltip="true"
              class="weight-slider"
            />
            <el-input-number
              v-model="weights[dim.key]"
              :min="0" :max="1" :step="0.01" :precision="2"
              :controls="false"
              size="small" style="width:90px"
            />
          </div>
          <div class="weight-hint">
            <span class="dim-code">{{ dim.code }}</span>
            {{ dim.hint }}
          </div>
          <div class="weight-detail">{{ dim.detail }}</div>
        </el-form-item>
      </el-form>

      <!-- 可视化条状图 -->
      <div class="weight-bars">
        <div
          v-for="dim in dimensionOptions"
          :key="dim.key"
          class="weight-bar-row"
        >
          <span class="bar-label">{{ dim.label }}</span>
          <div class="bar-track">
            <div class="bar-fill" :style="{ width: weightPercent(dim.key) + '%', background: dim.color }" />
          </div>
          <span class="bar-pct">{{ weightPercent(dim.key) }}%</span>
        </div>
      </div>

      <div class="preset-section">
        <div class="preset-section-head">
          <h4 class="guide-title">推荐预设</h4>
          <p class="preset-section-desc">先点一个方向，系统会自动填好四个比例，你再按需要微调。</p>
        </div>
        <div class="preset-grid preset-grid-buttons">
          <el-button
            v-for="preset in tuningPresets"
            :key="preset.key"
            class="preset-button"
            :type="activePreset === preset.key ? 'primary' : 'default'"
            :plain="activePreset !== preset.key"
            @click="applyPreset(preset)"
          >
            <span class="preset-button-name">{{ preset.name }}</span>
            <span class="preset-button-values">
              成本 {{ preset.values.cost.toFixed(2) }} / 速度 {{ preset.values.latency.toFixed(2) }} / 空闲 {{ preset.values.load.toFixed(2) }} / 稳定 {{ preset.values.health.toFixed(2) }}
            </span>
            <span class="preset-button-desc">{{ preset.desc }}</span>
          </el-button>
        </div>
      </div>
    </PortalContentCard>

    <!-- 说明卡 -->
    <PortalContentCard
      class="info-card"
      title="这页功能是做什么的"
      description="当一个模型配置了多条上游路线时，系统会用这里的比例决定“更偏向哪类路线”。保存后会影响后续新请求，不会修改模型价格、授权关系或已有调用记录。"
    >
      <div class="guide-section">
        <h4 class="guide-title">系统会怎么选路</h4>
        <div class="step-grid">
          <div v-for="(step, index) in routingSteps" :key="step.title" class="step-card">
            <span class="step-index">{{ index + 1 }}</span>
            <div>
              <strong>{{ step.title }}</strong>
              <p>{{ step.desc }}</p>
            </div>
          </div>
        </div>
      </div>

      <div class="guide-section">
        <h4 class="guide-title">容易误解的词</h4>
        <div class="term-list">
          <div v-for="term in termExplanations" :key="term.title" class="term-item">
            <span class="term-name">{{ term.title }}</span>
            <span>{{ term.desc }}</span>
          </div>
        </div>
        <p class="guide-note">
          如果 Redis 暂时不可用，系统拿不到实时速度和繁忙程度，会自动退回到“优先级 + 权重”的基础选路方式，保证请求仍可继续处理。
        </p>
      </div>
    </PortalContentCard>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.routing-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* PortalPagePanel body 无内边距,用 24px 容器排布两张内容卡 */
.routing-body {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 24px;
}

.card-title {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
  color: var(--ds-ink);
}

.card-desc {
  margin: 6px 0 0;
  font-size: 13px;
  color: var(--ds-muted);
}

.sum-ok  { color: var(--ds-positive); font-weight: 700; }
.sum-err { color: var(--ds-danger); font-weight: 700; }
.sum-err-msg { font-size: 12px; color: var(--ds-danger); margin-left: 12px; }

.weight-row {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
}

.weight-slider {
  flex: 1;
}

.weight-hint {
  font-size: 12px;
  color: var(--ds-muted);
  margin-top: 4px;
}

.dim-code {
  display: inline-block;
  margin-right: 6px;
  color: var(--ds-ink-soft);
  font-weight: 700;
}

.weight-detail {
  margin-top: 4px;
  color: var(--ds-faint);
  font-size: 12px;
  line-height: 1.6;
}

.weight-bars {
  margin-top: 24px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 16px;
  background: var(--ds-panel-muted);
  border-radius: var(--ds-radius-control);
}

.weight-bar-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.bar-label {
  width: 72px;
  font-size: 12px;
  font-weight: 600;
  color: var(--ds-ink-soft);
  flex-shrink: 0;
}

.bar-track {
  flex: 1;
  height: 8px;
  background: var(--ds-line);
  border-radius: 4px;
  overflow: hidden;
}

.bar-fill {
  height: 100%;
  border-radius: 4px;
  transition: width 0.3s ease;
}

.bar-pct {
  width: 44px;
  font-size: 12px;
  color: var(--ds-muted);
  text-align: right;
  flex-shrink: 0;
}

.preset-section {
  margin-top: 18px;
  padding-top: 18px;
  border-top: 1px solid var(--ds-line);
}

.preset-section-head {
  margin-bottom: 12px;
}

.preset-section-desc {
  margin: 6px 0 0;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.6;
}

.guide-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.guide-section + .guide-section {
  margin-top: 24px;
  padding-top: 20px;
  border-top: 1px solid var(--ds-line);
}

.guide-title {
  margin: 0;
  color: var(--ds-ink);
  font-size: 14px;
  font-weight: 700;
}

.step-grid,
.preset-grid {
  display: grid;
  gap: 12px;
}

.step-grid {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.preset-grid {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.step-card {
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel);
  display: flex;
  gap: 12px;
  padding: 14px;
}

.step-index {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: var(--ds-accent-soft);
  color: var(--ds-accent-hover);
  font-size: 12px;
  font-weight: 700;
  flex-shrink: 0;
}

.step-card strong {
  display: block;
  color: var(--ds-ink);
  font-size: 13px;
}

.step-card p {
  margin: 6px 0 0;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.6;
}

.preset-grid-buttons .preset-button + .preset-button {
  margin-left: 0;
}

.preset-button {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: flex-start;
  width: 100%;
  height: auto;
  min-height: 116px;
  padding: 14px 16px;
  border-radius: var(--ds-radius-control);
  text-align: left;
  white-space: normal;
  line-height: 1.4;
}

.preset-button :deep(> span) {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
}

.preset-button-name {
  display: block;
  color: inherit;
  font-size: 13px;
  font-weight: 700;
}

.preset-button-values {
  display: block;
  margin-top: 8px;
  color: inherit;
  font-size: 12px;
  line-height: 1.55;
}

.preset-button-desc {
  display: block;
  margin-top: 8px;
  color: inherit;
  font-size: 12px;
  line-height: 1.6;
  opacity: 0.86;
}

.guide-note {
  margin: 0;
  padding: 12px 14px;
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel-muted);
  color: var(--ds-ink-soft);
  font-size: 12px;
  line-height: 1.7;
}

.term-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.term-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  font-size: 13px;
  color: var(--ds-ink-soft);
  line-height: 1.6;
}

.term-name {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 4px;
  background: var(--ds-panel-muted);
  color: var(--ds-ink-soft);
  font-size: 11px;
  font-weight: 700;
  flex-shrink: 0;
  margin-top: 1px;
}

@media (max-width: 960px) {
  .preset-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 720px) {
  .step-grid,
  .preset-grid {
    grid-template-columns: 1fr;
  }

  .weight-row {
    align-items: stretch;
    flex-direction: column;
  }

  .bar-label {
    width: 64px;
  }
}
</style>

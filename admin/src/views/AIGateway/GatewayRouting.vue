<script setup>
import { onMounted, reactive, shallowRef } from 'vue'
import { ElMessage } from 'element-plus'
import { getRouteWeights, putRouteWeights } from '@/api/aiGateway'

// ── State ───────────────────────────────────────────────────────────────────
const loading = shallowRef(false)
const saving = shallowRef(false)
const scope = shallowRef('global')

const weights = reactive({
  cost: 0.4,
  latency: 0.3,
  load: 0.2,
  health: 0.1
})

// ── Computed ────────────────────────────────────────────────────────────────
function weightSum() {
  return +(weights.cost + weights.latency + weights.load + weights.health).toFixed(4)
}

function weightSumOk() {
  return Math.abs(weightSum() - 1.0) < 0.001
}

// ── Actions ──────────────────────────────────────────────────────────────────
async function fetchWeights() {
  loading.value = true
  try {
    const data = await getRouteWeights(scope.value)
    if (data?.weights) {
      weights.cost    = data.weights.cost    ?? 0.4
      weights.latency = data.weights.latency ?? 0.3
      weights.load    = data.weights.load    ?? 0.2
      weights.health  = data.weights.health  ?? 0.1
    }
  } catch (e) {
    ElMessage.error('加载评分权重失败：' + (e?.message || e))
  } finally {
    loading.value = false
  }
}

async function saveWeights() {
  if (!weightSumOk()) {
    ElMessage.warning('四项权重之和必须等于 1.0，当前合计 ' + weightSum())
    return
  }
  saving.value = true
  try {
    await putRouteWeights(scope.value, {
      cost:    weights.cost,
      latency: weights.latency,
      load:    weights.load,
      health:  weights.health
    })
    ElMessage.success('评分权重已保存，下次请求生效')
  } catch (e) {
    ElMessage.error('保存失败：' + (e?.message || e))
  } finally {
    saving.value = false
  }
}

function normalize() {
  const sum = weightSum()
  if (sum <= 0) return
  weights.cost    = +(weights.cost    / sum).toFixed(3)
  weights.latency = +(weights.latency / sum).toFixed(3)
  weights.load    = +(weights.load    / sum).toFixed(3)
  weights.health  = +(1 - weights.cost - weights.latency - weights.load).toFixed(3)
}

function barColor(key) {
  const map = { cost: '#3b82f6', latency: '#10b981', load: '#f59e0b', health: '#ef4444' }
  return map[key] || '#94a3b8'
}

onMounted(fetchWeights)
</script>

<template>
  <div class="routing-page">
    <!-- 评分权重配置卡 -->
    <el-card shadow="never" class="config-card" v-loading="loading">
      <template #header>
        <div class="card-header">
          <div>
            <h3 class="card-title">多维评分路由权重</h3>
            <p class="card-desc">
              调整 cost / latency / load / health 四维权重，控制 Softmax 路由选路倾向。
              权重之和须为 <strong>1.0</strong>（当前合计：
              <span :class="weightSumOk() ? 'sum-ok' : 'sum-err'">{{ weightSum() }}</span>）。
            </p>
          </div>
          <div class="header-actions">
            <el-button size="small" @click="fetchWeights" :loading="loading">刷新</el-button>
            <el-button size="small" @click="normalize">归一化</el-button>
          </div>
        </div>
      </template>

      <el-form label-width="120px" label-position="left">
        <el-form-item label="Cost 权重">
          <div class="weight-row">
            <el-slider
              v-model="weights.cost"
              :min="0" :max="1" :step="0.01"
              :show-tooltip="true"
              class="weight-slider"
            />
            <el-input-number
              v-model="weights.cost"
              :min="0" :max="1" :step="0.01" :precision="2"
              size="small" style="width:90px"
            />
          </div>
          <div class="weight-hint">路由成本越低分越高（适合控费场景调大）</div>
        </el-form-item>

        <el-form-item label="Latency 权重">
          <div class="weight-row">
            <el-slider
              v-model="weights.latency"
              :min="0" :max="1" :step="0.01"
              :show-tooltip="true"
              class="weight-slider"
            />
            <el-input-number
              v-model="weights.latency"
              :min="0" :max="1" :step="0.01" :precision="2"
              size="small" style="width:90px"
            />
          </div>
          <div class="weight-hint">EWMA 延迟越低分越高（适合低延迟场景调大）</div>
        </el-form-item>

        <el-form-item label="Load 权重">
          <div class="weight-row">
            <el-slider
              v-model="weights.load"
              :min="0" :max="1" :step="0.01"
              :show-tooltip="true"
              class="weight-slider"
            />
            <el-input-number
              v-model="weights.load"
              :min="0" :max="1" :step="0.01" :precision="2"
              size="small" style="width:90px"
            />
          </div>
          <div class="weight-hint">当前 inflight 越少分越高（均衡流量调大）</div>
        </el-form-item>

        <el-form-item label="Health 权重">
          <div class="weight-row">
            <el-slider
              v-model="weights.health"
              :min="0" :max="1" :step="0.01"
              :show-tooltip="true"
              class="weight-slider"
            />
            <el-input-number
              v-model="weights.health"
              :min="0" :max="1" :step="0.01" :precision="2"
              size="small" style="width:90px"
            />
          </div>
          <div class="weight-hint">熔断 OPEN 路由惩罚系数，建议保留 ≥ 0.1</div>
        </el-form-item>
      </el-form>

      <!-- 可视化条状图 -->
      <div class="weight-bars">
        <div
          v-for="(dim, key) in {cost: weights.cost, latency: weights.latency, load: weights.load, health: weights.health}"
          :key="key"
          class="weight-bar-row"
        >
          <span class="bar-label">{{ key }}</span>
          <div class="bar-track">
            <div class="bar-fill" :style="{width: (dim * 100).toFixed(1) + '%', background: barColor(key)}" />
          </div>
          <span class="bar-pct">{{ (dim * 100).toFixed(1) }}%</span>
        </div>
      </div>

      <template #footer>
        <div class="card-footer">
          <el-button
            type="primary"
            :loading="saving"
            :disabled="!weightSumOk()"
            @click="saveWeights"
          >保存权重</el-button>
          <span v-if="!weightSumOk()" class="sum-err-msg">权重之和需为 1.0（差 {{ (1 - weightSum()).toFixed(3) }}）</span>
        </div>
      </template>
    </el-card>

    <!-- 说明卡 -->
    <el-card shadow="never" class="info-card">
      <template #header><h3 class="card-title">路由选路说明</h3></template>
      <div class="info-list">
        <div class="info-item">
          <span class="info-badge" style="background:#3b82f6">Softmax</span>
          <span>评分经 Softmax 加权后随机抽样，避免流量完全集中。温度参数 T=1.0（越大越均匀）。</span>
        </div>
        <div class="info-item">
          <span class="info-badge" style="background:#10b981">降级</span>
          <span>Redis 不可用时自动降级为 priority+weighted 随机（忽略 cost/latency/load 权重）。</span>
        </div>
        <div class="info-item">
          <span class="info-badge" style="background:#f59e0b">Sticky</span>
          <span>携带 X-Conversation-Id header 的请求粘性路由到同一凭据（24 h TTL）。</span>
        </div>
        <div class="info-item">
          <span class="info-badge" style="background:#ef4444">熔断</span>
          <span>连续失败触发 OPEN 状态（60s 起步指数退避），半开探测成功后恢复。</span>
        </div>
      </div>
    </el-card>
  </div>
</template>

<style scoped>
.routing-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.config-card, .info-card {
  border-radius: 12px;
  border: 1px solid #f1f5f9;
}

.card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.header-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.card-title {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
  color: #0f172a;
}

.card-desc {
  margin: 6px 0 0;
  font-size: 13px;
  color: #64748b;
}

.sum-ok  { color: #10b981; font-weight: 700; }
.sum-err { color: #ef4444; font-weight: 700; }
.sum-err-msg { font-size: 12px; color: #ef4444; margin-left: 12px; }

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
  color: #94a3b8;
  margin-top: 4px;
}

.weight-bars {
  margin-top: 24px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 16px;
  background: #f8fafc;
  border-radius: 8px;
}

.weight-bar-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.bar-label {
  width: 60px;
  font-size: 12px;
  font-weight: 600;
  color: #475569;
  text-transform: uppercase;
  flex-shrink: 0;
}

.bar-track {
  flex: 1;
  height: 8px;
  background: #e2e8f0;
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
  color: #64748b;
  text-align: right;
  flex-shrink: 0;
}

.card-footer {
  display: flex;
  align-items: center;
  gap: 8px;
}

.info-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.info-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  font-size: 13px;
  color: #475569;
  line-height: 1.6;
}

.info-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 700;
  color: #fff;
  flex-shrink: 0;
  margin-top: 1px;
}
</style>


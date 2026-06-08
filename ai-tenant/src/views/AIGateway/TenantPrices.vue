<script setup>
import { computed, onMounted, reactive, shallowRef } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  capabilityOptions,
  deleteMyUserSellBinding,
  formatCredits,
  getMySellBinding,
  getMyUserSellBinding,
  getMyEffectivePrices,
  upsertMyUserSellBinding
} from '@/api/aiGateway'

const loading = shallowRef(false)
const saving = shallowRef(false)

// 平台给我的售价绑定（只读）
const platformBinding = shallowRef(null)

// 我给用户的售价绑定（可编辑）
const userBinding = reactive({ user_multiplier: 1, cache_billing_enabled: false, exists: false })

// 生效积分单价
const scope = shallowRef('user') // 'tenant' = 我的成本 | 'user' = 卖给用户
const prices = shallowRef([])
const pricesLoading = shallowRef(false)

const capabilityLabel = (v) => capabilityOptions.find((c) => c.value === v)?.label || v

const hasPlatformBinding = computed(() => !!platformBinding.value)

async function loadBindings() {
  loading.value = true
  try {
    const [pb, ub] = await Promise.all([getMySellBinding(), getMyUserSellBinding()])
    platformBinding.value = pb || null
    if (ub) {
      userBinding.user_multiplier = ub.user_multiplier ?? 1
      userBinding.cache_billing_enabled = !!ub.cache_billing_enabled
      userBinding.exists = true
    } else {
      userBinding.user_multiplier = 1
      userBinding.cache_billing_enabled = false
      userBinding.exists = false
    }
  } finally {
    loading.value = false
  }
}

async function loadPrices() {
  pricesLoading.value = true
  try {
    const res = await getMyEffectivePrices(scope.value)
    prices.value = Array.isArray(res) ? res : []
  } finally {
    pricesLoading.value = false
  }
}

async function saveUserBinding() {
  if (!(userBinding.user_multiplier >= 0)) {
    ElMessage.warning('用户倍率必须为非负数')
    return
  }
  saving.value = true
  try {
    await upsertMyUserSellBinding({
      user_multiplier: userBinding.user_multiplier,
      cache_billing_enabled: userBinding.cache_billing_enabled
    })
    userBinding.exists = true
    ElMessage.success('已保存')
    if (scope.value === 'user') loadPrices()
  } finally {
    saving.value = false
  }
}

async function removeUserBinding() {
  await ElMessageBox.confirm('删除后，用户售价将回退为「平台给你的售价 × 1」（缓存按输入价）。', '确认删除', { type: 'warning' })
  await deleteMyUserSellBinding()
  userBinding.user_multiplier = 1
  userBinding.cache_billing_enabled = false
  userBinding.exists = false
  ElMessage.success('已删除')
  if (scope.value === 'user') loadPrices()
}

function refreshAll() {
  loadBindings()
  loadPrices()
}

onMounted(() => {
  loadBindings()
  loadPrices()
})
</script>

<template>
  <div class="page-container">
    <header class="page-header">
      <div class="page-title">
        <h2>租户定价</h2>
        <p>查看平台给你的售价，设置你卖给终端用户的加价倍率。</p>
      </div>
      <el-button :icon="Refresh" @click="refreshAll">刷新</el-button>
    </header>

    <main class="page-main">
      <el-row :gutter="16">
        <!-- 平台给我的售价（只读） -->
        <el-col :span="12">
          <el-card shadow="never" v-loading="loading">
            <template #header><span class="card-title">平台给我的售价</span></template>
            <template v-if="hasPlatformBinding">
              <el-descriptions :column="1" border size="small">
                <el-descriptions-item label="价格表">{{ platformBinding.price_book_name || platformBinding.price_book_id }}</el-descriptions-item>
                <el-descriptions-item label="售价倍率">×{{ platformBinding.sell_multiplier }}</el-descriptions-item>
                <el-descriptions-item label="缓存计价">
                  <el-tag size="small" :type="platformBinding.cache_billing_enabled ? 'success' : 'info'" effect="plain">
                    {{ platformBinding.cache_billing_enabled ? '开（缓存单独计价）' : '关（缓存按输入价）' }}
                  </el-tag>
                </el-descriptions-item>
              </el-descriptions>
              <p class="hint mt-3">这是平台向你结算的价格基准，由平台管理员配置，你不可修改。</p>
            </template>
            <el-alert
              v-else
              type="warning"
              :closable="false"
              show-icon
              title="平台尚未给你配置售价"
              description="在平台配置售价绑定前，你的请求会被拒绝。请联系平台管理员。"
            />
          </el-card>
        </el-col>

        <!-- 我给用户的售价（可编辑） -->
        <el-col :span="12">
          <el-card shadow="never">
            <template #header><span class="card-title">我给用户的售价</span></template>
            <p class="hint mb-3">
              用户售价 = 平台给你的售价 × 用户倍率。例如平台给你 ×1，你设 ×1.5，则用户按平台价的 1.5 倍计费，差额即你的利润。
            </p>
            <el-form label-width="96px">
              <el-form-item label="用户倍率">
                <el-input-number v-model="userBinding.user_multiplier" :min="0" :step="0.1" :precision="4" />
              </el-form-item>
              <el-form-item label="缓存计价">
                <el-switch v-model="userBinding.cache_billing_enabled" />
                <span class="hint ml-2">关 = 缓存按输入价计</span>
              </el-form-item>
              <el-form-item>
                <el-button type="primary" :loading="saving" @click="saveUserBinding">保存</el-button>
                <el-button v-if="userBinding.exists" type="danger" plain @click="removeUserBinding">删除</el-button>
              </el-form-item>
            </el-form>
          </el-card>
        </el-col>
      </el-row>

      <!-- 生效积分单价 -->
      <el-card shadow="never" class="mt-4">
        <template #header>
          <div class="flex items-center justify-between">
            <span class="card-title">生效积分单价</span>
            <el-radio-group v-model="scope" size="small" @change="loadPrices">
              <el-radio-button label="user">卖给用户</el-radio-button>
              <el-radio-button label="tenant">我的成本</el-radio-button>
            </el-radio-group>
          </div>
        </template>
        <el-table v-loading="pricesLoading" :data="prices" size="small" stripe>
          <el-table-column prop="model_code" label="模型" min-width="180" show-overflow-tooltip />
          <el-table-column label="能力" width="90">
            <template #default="{ row }">{{ capabilityLabel(row.capability_type) }}</template>
          </el-table-column>
          <el-table-column label="输入 积分/1M" width="130">
            <template #default="{ row }">{{ formatCredits(row.input_per_1m_credits) }}</template>
          </el-table-column>
          <el-table-column label="输出 积分/1M" width="130">
            <template #default="{ row }">{{ formatCredits(row.output_per_1m_credits) }}</template>
          </el-table-column>
          <el-table-column label="缓存读 积分/1M" width="140">
            <template #default="{ row }">{{ formatCredits(row.cache_read_per_1m_credits) }}</template>
          </el-table-column>
          <template #empty>
            <span class="hint">{{ hasPlatformBinding ? '该价格表暂无条目' : '平台未配置售价，无生效单价' }}</span>
          </template>
        </el-table>
      </el-card>
    </main>
  </div>
</template>

<style scoped>
.page-container { padding: 4px; }
.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 16px;
}
.page-title h2 { margin: 0; font-size: 20px; font-weight: 800; color: #0f172a; }
.page-title p { margin: 6px 0 0; font-size: 13px; color: #64748b; }
.card-title { font-weight: 700; color: #0f172a; }
.hint { color: #94a3b8; font-size: 12px; }
</style>

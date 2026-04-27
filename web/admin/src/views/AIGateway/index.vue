<script setup>
import { shallowRef } from 'vue'
import { Connection, Cpu, Key, Lock, Tickets, DocumentChecked } from '@element-plus/icons-vue'
import GatewayProviders from './GatewayProviders.vue'
import GatewayModels from './GatewayModels.vue'
import GatewayAccess from './GatewayAccess.vue'
import GatewayUsage from './GatewayUsage.vue'
import GatewayLimits from './GatewayLimits.vue'
import GatewayAudit from './GatewayAudit.vue'

const activeTab = shallowRef('providers')
</script>

<template>
  <div class="page-container space-y-5 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <section class="bg-white border border-slate-100 rounded-2xl shadow-soft p-6">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <p class="text-xs font-black text-slate-400 uppercase">AI Gateway</p>
          <h2 class="mt-1 text-xl font-black text-slate-900">统一模型接入</h2>
          <p class="mt-2 text-sm text-slate-500">
            维护厂商接入点、模型映射、租户授权和运行时 API Key。
          </p>
        </div>
        <el-alert
          class="gateway-alert"
          type="info"
          :closable="false"
          show-icon
          title="登录后默认使用 URM JWT；本地调试可设置 localStorage.uni-ai-api-admin-token。"
        />
      </div>
    </section>

    <section class="bg-white border border-slate-100 rounded-2xl shadow-soft p-4">
      <el-tabs v-model="activeTab" class="gateway-tabs">
        <el-tab-pane name="providers">
          <template #label>
            <span class="tab-label"><el-icon><Connection /></el-icon>厂商接入</span>
          </template>
          <GatewayProviders />
        </el-tab-pane>

        <el-tab-pane name="models">
          <template #label>
            <span class="tab-label"><el-icon><Cpu /></el-icon>模型映射</span>
          </template>
          <GatewayModels />
        </el-tab-pane>

        <el-tab-pane name="access">
          <template #label>
            <span class="tab-label"><el-icon><Key /></el-icon>授权与 Key</span>
          </template>
          <GatewayAccess />
        </el-tab-pane>

        <el-tab-pane name="usage">
          <template #label>
            <span class="tab-label"><el-icon><Tickets /></el-icon>调用日志</span>
          </template>
          <GatewayUsage />
        </el-tab-pane>

        <el-tab-pane name="limits">
          <template #label>
            <span class="tab-label"><el-icon><Lock /></el-icon>限流策略</span>
          </template>
          <GatewayLimits />
        </el-tab-pane>

        <el-tab-pane name="audit">
          <template #label>
            <span class="tab-label"><el-icon><DocumentChecked /></el-icon>网关审计</span>
          </template>
          <GatewayAudit />
        </el-tab-pane>
      </el-tabs>
    </section>
  </div>
</template>

<style scoped>
.gateway-alert {
  max-width: 520px;
}

.tab-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-weight: 700;
}

.gateway-tabs :deep(.el-tabs__header) {
  margin-bottom: 18px;
}
</style>

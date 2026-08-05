<!--
  模型定价 — 按上游资源查看可用模型及当前生效的结算价格(只读目录式呈现)。
  重构:迁移至新设计系统一体面板(PortalPagePanel:图标徽章+面包屑标题+描述同行,
       左侧上游资源 rail + 右侧模型定价网格收进同卡 body 的 24px 容器,fill 撑满视口);
       业务逻辑与请求保持不变。rail/grid 子组件本次未动。
-->
<script setup lang="ts">
import { computed, onMounted, shallowRef } from "vue";
import { Refresh } from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
import { Network } from "lucide-vue-next";
import { PortalPagePanel } from "@/platform";

import { aiTenantApi } from "@/api/aiTenant";
import type { TenantAiUpstreamResource } from "@/api/types/aiTenant";
import UpstreamAccountRail from "./components/UpstreamAccountRail.vue";
import UpstreamModelPricingGrid from "./components/UpstreamModelPricingGrid.vue";

const loading = shallowRef(false);
const resources = shallowRef<TenantAiUpstreamResource[]>([]);
const selectedResourceId = shallowRef("");
const creditsPerUSD = shallowRef(0);

const selectedResource = computed(
  () => resources.value.find((resource) => resource.id === selectedResourceId.value) ?? null
);

function selectResource(resourceId: string) {
  selectedResourceId.value = resourceId;
}

async function loadResources() {
  loading.value = true;
  try {
    const [response, rateResponse] = await Promise.all([
      aiTenantApi.listUpstreamResources(),
      aiTenantApi.getCreditsPerUSD().catch(() => null)
    ]);
    resources.value = response.items ?? [];
    creditsPerUSD.value = rateResponse?.credits_per_usd ?? 0;

    if (!resources.value.some((resource) => resource.id === selectedResourceId.value)) {
      selectedResourceId.value = resources.value[0]?.id ?? "";
    }
  } catch (error: unknown) {
    const message = error instanceof Error ? error.message : "加载模型定价失败";
    ElMessage.error(message || "加载模型定价失败");
  } finally {
    loading.value = false;
  }
}

onMounted(loadResources);
</script>

<template>
  <div class="page-container upstream-pricing-page">
    <PortalPagePanel
      fill
      :icon="Network"
      :breadcrumbs="[{ label: '智能服务' }, { label: '模型与定价' }, { label: '模型定价' }]"
      description="按上游资源查看可用模型及当前生效的结算价格。"
    >
      <template #actions>
        <el-button :icon="Refresh" :loading="loading" @click="loadResources">刷新</el-button>
      </template>

      <!-- 主从布局:body 无内边距,用 24px 容器承载上游资源 rail + 模型定价网格 -->
      <div class="pricing-body">
        <div class="pricing-workspace">
          <UpstreamAccountRail
            :accounts="resources"
            :selected-id="selectedResourceId"
            :loading="loading"
            @select="selectResource"
          />
          <UpstreamModelPricingGrid
            :resource="selectedResource"
            :credits-per-usd="creditsPerUSD"
            :loading="loading"
          />
        </div>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.upstream-pricing-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.pricing-body {
  display: flex;
  flex-direction: column;
  padding: 24px;
  flex: 1;
  min-height: 0;
}

.pricing-workspace {
  display: grid;
  grid-template-columns: minmax(240px, 292px) minmax(0, 1fr);
  align-items: stretch;
  gap: 20px;
  flex: 1;
  min-height: 0;
}

@media (max-width: 960px) {
  .pricing-workspace {
    grid-template-columns: 1fr;
  }
}
</style>

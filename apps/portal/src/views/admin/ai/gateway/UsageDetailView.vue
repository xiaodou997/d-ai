<!--
  管理端 AI 网关请求详情页:按路由参数 requestId 拉取单条请求详情,
  替代原右侧抽屉(列表上下文不再可用,上一条/下一条导航随之下线,由面包屑返回列表)。
  内容渲染由 UsageDetailContent 承担,本页只负责加载态/失败态与面板骨架。
-->
<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ScrollText } from "lucide-vue-next";

import { PortalPagePanel } from "@/platform";
import { DsEmpty } from "@/shared/ui";

import { adminUsageApi } from "@/features/ai/usage/api";
import UsageDetailContent from "@/features/ai/usage/components/UsageDetailContent.vue";
import type { UsageLogDetailDTO } from "@/features/ai/usage/model";

const route = useRoute();
const router = useRouter();

const detail = ref<UsageLogDetailDTO | null>(null);
const loading = ref(false);
const loadFailed = ref(false);

// 防止路由参数快速切换时旧响应覆盖新数据
let loadSeq = 0;

async function loadDetail(requestId: string) {
  const seq = ++loadSeq;
  if (!requestId) {
    detail.value = null;
    loadFailed.value = true;
    return;
  }
  loading.value = true;
  loadFailed.value = false;
  try {
    const data = await adminUsageApi.getDetail(requestId);
    if (seq !== loadSeq) return;
    detail.value = data;
  } catch {
    if (seq !== loadSeq) return;
    detail.value = null;
    loadFailed.value = true;
  } finally {
    if (seq === loadSeq) loading.value = false;
  }
}

function backToUsage() {
  void router.push({ name: "ai-usage" });
}

onMounted(() => {
  void loadDetail(String(route.params.requestId ?? ""));
});

watch(
  () => route.params.requestId,
  (value) => {
    void loadDetail(String(value ?? ""));
  }
);
</script>

<template>
  <div class="page-container usage-detail-view">
    <PortalPagePanel
      fill
      :icon="ScrollText"
      :breadcrumbs="[
        { label: '智能服务' },
        { label: '日志审计' },
        { label: '请求记录', to: '/admin/ai/security/usage' },
        { label: '请求详情' }
      ]"
      description="按请求 ID 查看单条请求的完整链路、耗时拆解、计费档位与原始载荷。"
    >
      <div class="usage-detail-view__body">
        <DsEmpty
          v-if="loadFailed && !loading"
          title="未找到该请求的详情"
          description="请求可能不存在或加载失败,请返回请求记录重新选择。"
        >
          <template #action>
            <el-button type="primary" @click="backToUsage">返回请求记录</el-button>
          </template>
        </DsEmpty>
        <UsageDetailContent v-else :detail="detail" :loading="loading" />
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.usage-detail-view {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

/* body 无内边距,24px 容器承载详情内容并负责纵向滚动 */
.usage-detail-view__body {
  flex: 1;
  min-height: 0;
  padding: 24px;
  overflow: auto;
}
</style>

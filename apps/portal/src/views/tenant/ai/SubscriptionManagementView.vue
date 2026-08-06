<!--
  租户端 AI 订阅管理 — 套餐管理 / 订阅实例 / 订单记录 三个分区通过 DsTabs 切换。
  重构:迁移至新设计系统一体面板(PortalPagePanel:图标徽章+面包屑标题+描述同行),
       el-tabs 换为 DsTabs,Tab 面板经 v-show 切换且保持原 el-tab-pane lazy 的
       首次激活才挂载行为(各面板已包单根 div);业务逻辑与请求参数不变。
-->
<script setup lang="ts">
import { computed, shallowRef, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { Layers } from "lucide-vue-next";
import { PortalPagePanel } from "@/platform";
import { DsTabs } from "@/shared/ui";

import SubscriptionInstancesPanel from "@/features/ai/subscriptions/SubscriptionInstancesPanel.vue";
import SubscriptionOrdersPanel from "@/features/ai/subscriptions/SubscriptionOrdersPanel.vue";
import SubscriptionPlansPanel from "@/features/ai/subscriptions/SubscriptionPlansPanel.vue";

type SubscriptionTab = "plans" | "subscriptions" | "orders";

const route = useRoute();
const router = useRouter();
const validTabs = new Set<SubscriptionTab>(["plans", "subscriptions", "orders"]);
const activeTab = computed<SubscriptionTab>({
  get() {
    const tab = route.query.tab;
    return typeof tab === "string" && validTabs.has(tab as SubscriptionTab) ? tab as SubscriptionTab : "plans";
  },
  set(tab) {
    void router.replace({ query: { ...route.query, tab } });
  }
});

const tabs = [
  { key: "plans", label: "套餐管理" },
  { key: "subscriptions", label: "订阅实例" },
  { key: "orders", label: "订单记录" }
];

// 与原 el-tab-pane lazy 行为一致:面板首次激活时才挂载(触发各自的首次拉取)
const activatedTabs = shallowRef<Set<SubscriptionTab>>(new Set([activeTab.value]));
watch(activeTab, (tab) => {
  if (activatedTabs.value.has(tab)) return;
  const next = new Set(activatedTabs.value);
  next.add(tab);
  activatedTabs.value = next;
});
</script>

<template>
  <div class="page-container subscription-management-page">
    <PortalPagePanel
      fill
      :icon="Layers"
      :breadcrumbs="[{ label: '智能服务' }, { label: '用户与订阅' }, { label: '订阅管理' }]"
      description="管理面向终端用户的套餐，并查看购买订单和订阅额度的实际生效情况。"
    >

      <!-- 面板 body 无内边距,Tab 页内容用 24px 容器承载;fill 模式下伸展撑满 -->
      <div class="subscription-body">
        <DsTabs v-model="activeTab" :tabs="tabs" />
        <div class="subscription-pane">
          <SubscriptionPlansPanel v-if="activatedTabs.has('plans')" v-show="activeTab === 'plans'" />
          <SubscriptionInstancesPanel v-if="activatedTabs.has('subscriptions')" v-show="activeTab === 'subscriptions'" />
          <SubscriptionOrdersPanel v-if="activatedTabs.has('orders')" v-show="activeTab === 'orders'" />
        </div>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
/* wrapper div: PortalPagePanel 不直接作为路由组件根元素,避免内部弹窗 append-to-body
   的 Teleport 干扰 <transition mode="out-in"> 的 leave→enter 状态机导致白屏 */
.subscription-management-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.subscription-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 16px 24px 24px;
}

.subscription-pane {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding-top: 16px;
}

@media (max-width: 768px) {
  .subscription-body {
    padding-inline: 16px;
  }
}
</style>

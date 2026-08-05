<!--
  风控中心 — 智能服务 / AI 网关内容安全审核工作台。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行）,
       el-card+el-tabs 改为 DsTabs(审核日志/风险事件,未处理计数走 DsTabs count,0 时不显示);
       配置入口与状态徽章收进页头 #actions(见 RiskControlConfigPanel);
       弹窗仍为 element-plus,业务逻辑与请求参数不变。
-->
<script setup lang="ts">
import { computed, shallowRef, watch } from 'vue'
import { ShieldAlert } from 'lucide-vue-next'
import { PortalPagePanel } from '@/platform'
import { DsTabs } from '@/shared/ui'

import RiskControlConfigPanel from './RiskControlConfigPanel.vue'
import RiskControlEventsPanel from './RiskControlEventsPanel.vue'
import RiskControlLogsPanel from './RiskControlLogsPanel.vue'

const activeTab = shallowRef('logs')
// 与原 el-tab-pane 行为一致:风险事件面板首次激活时才挂载(触发 fetchEvents)
const eventsActivated = shallowRef(false)
const openEventCount = shallowRef(0)

const tabs = computed(() => [
  { key: 'logs', label: '审核日志' },
  { key: 'events', label: '风险事件', count: openEventCount.value || undefined }
])

watch(activeTab, (key) => {
  if (key === 'events') eventsActivated.value = true
})
</script>

<template>
  <div class="risk-control-page">
    <PortalPagePanel
      :icon="ShieldAlert"
      :breadcrumbs="[{ label: '智能服务' }, { label: '日志审计' }, { label: '风控中心' }]"
      description="内容安全审核：关键词引擎（AC 自动机 + 归一化 + 拼音）+ AI 审核 API 检测用户输入，命中可拦截/记录；累计违规达到阈值生成风险事件，交由人工处置（不自动封禁账号）。"
      fill
    >
      <template #actions>
        <RiskControlConfigPanel />
      </template>

      <div class="risk-control-body">
        <DsTabs v-model="activeTab" :tabs="tabs" />
        <div class="risk-control-pane">
          <RiskControlLogsPanel v-show="activeTab === 'logs'" />
          <RiskControlEventsPanel
            v-if="eventsActivated"
            v-show="activeTab === 'events'"
            @open-event-count="openEventCount = $event"
          />
        </div>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
/* wrapper div: PortalPagePanel 不直接作为路由组件根元素,避免内部 el-dialog append-to-body
   的 Teleport 干扰 <transition mode="out-in"> 的 leave→enter 状态机导致白屏 */
.risk-control-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

/* 面板 body 无内边距,Tab 页内容用 24px 容器承载(与支付配置页同模式);fill 模式下伸展撑满 */
.risk-control-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 16px 24px 24px;
}

.risk-control-pane {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding-top: 16px;
}
</style>

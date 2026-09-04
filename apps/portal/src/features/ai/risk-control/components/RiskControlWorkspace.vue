<!--
  风控中心 — 智能服务 / AI 网关内容安全审核工作台。
  风控中心统一工作台：审核日志、关键词引擎、审核 API、提示词审核和风险事件
  通过 DsTabs 切换；网关审计与风控中心为两个并列的独立菜单。
-->
<script setup lang="ts">
import { computed, shallowRef, watch } from 'vue'
import { ShieldAlert } from 'lucide-vue-next'
import { PortalPagePanel } from '@/platform'
import { DsTabs } from '@/shared/ui'

import RiskControlConfigPanel from './RiskControlConfigPanel.vue'
import RiskControlEventsPanel from './RiskControlEventsPanel.vue'
import RiskControlLogsPanel from './RiskControlLogsPanel.vue'
import { PromptAuditPanel } from '../../prompt-audit'
import RiskControlKeywordPanel from './RiskControlKeywordPanel.vue'
import RiskControlProviderPanel from './RiskControlProviderPanel.vue'

const activeTab = shallowRef('logs')
// 与原 el-tab-pane 行为一致:风险事件面板首次激活时才挂载(触发 fetchEvents)
const eventsActivated = shallowRef(false)
const openEventCount = shallowRef(0)

const tabs = computed(() => [
  { key: 'logs', label: '审核日志' },
  { key: 'keywords', label: '关键词引擎' },
  { key: 'provider', label: '审核 API' },
  { key: 'prompt-audit', label: '提示词审核' },
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
      :breadcrumbs="[{ label: '智能服务' }, { label: '风控中心' }]"
      description="统一管理内容审核与提示词安全审计：关键词引擎、OpenAI Moderations API、Qwen3Guard 提示词审核和人工风险事件。"
      fill
    >
      <template #actions>
        <RiskControlConfigPanel />
      </template>

      <div class="risk-control-body">
        <DsTabs v-model="activeTab" :tabs="tabs" />
        <div class="risk-control-pane">
          <RiskControlLogsPanel v-show="activeTab === 'logs'" />
          <RiskControlKeywordPanel v-if="activeTab === 'keywords'" />
          <RiskControlProviderPanel v-if="activeTab === 'provider'" />
          <RiskControlEventsPanel
            v-if="eventsActivated"
            v-show="activeTab === 'events'"
            @open-event-count="openEventCount = $event"
          />
          <PromptAuditPanel v-if="activeTab === 'prompt-audit'" />
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

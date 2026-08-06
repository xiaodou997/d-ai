<!--
  用户端 AI 模型定价页：按分组查看当前可用模型及实际扣费价格。
  重构：随 PortalGroupPricingWorkspace 迁移至 DsUI 一体面板,页头改传
       icon/breadcrumbs(智能服务 / 我的服务 / 模型定价);业务逻辑与请求不变。
-->
<script setup lang="ts">
import { useRouter } from "vue-router";
import { Layers } from "lucide-vue-next";
import { notifyError } from "@/platform";
import {
  portalVisibleGroupsWorkspaceIconProps,
  PortalGroupPricingWorkspace,
  type PortalGroupPricingApi
} from "@/platform/ai/groups";

import { aiCustomerApi } from "@/api/aiCustomer";
import type { AiGroupEffectivePrice, AiVisibleGroup } from "@/api/types/aiCustomer";

const router = useRouter();

const pricingApi: PortalGroupPricingApi<AiVisibleGroup, AiGroupEffectivePrice> = {
  listGroups: () => aiCustomerApi.listMyGroups(),
  getGroupEffectivePrices: (groupId: string) => aiCustomerApi.getMyGroupEffectivePrices(groupId)
};

const capabilityOptions = [
  { label: "文本对话", value: "chat" },
  { label: "生图", value: "image" },
  { label: "视频", value: "video" },
  { label: "Embedding", value: "embedding" },
  { label: "语音合成", value: "audio_tts" },
  { label: "语音识别", value: "audio_stt" },
  { label: "重排", value: "rerank" }
];

const openApiKeys = () => {
  router.push("/customer/developer/keys");
};
</script>

<template>
  <PortalGroupPricingWorkspace
    :api="pricingApi"
    :icon="Layers"
    :breadcrumbs="[{ label: '智能服务' }, { label: '我的服务' }, { label: '模型定价' }]"
    description="按分组查看当前可用模型及实际会扣除的积分价格。"
    :capability-options="capabilityOptions"
    :notify-error="notifyError"
    v-bind="portalVisibleGroupsWorkspaceIconProps"
  >
    <template #actions>
      <el-button type="primary" plain @click="openApiKeys">管理 API 密钥</el-button>
    </template>
  </PortalGroupPricingWorkspace>
</template>

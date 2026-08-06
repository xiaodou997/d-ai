<!--
  使用记录页(智能服务 / 用量与分析 / 使用记录,/workspace/usage-records):
  路由视图壳,实际内容全部由 features/ai/usage 的 TenantUsageWorkspace 渲染
  (指标卡 + 筛选带 + 使用记录表格 + 调用详情抽屉)。
-->
<script setup lang="ts">
import { TenantUsageWorkspace } from "@/features/ai/usage";
</script>

<template>
  <TenantUsageWorkspace />
</template>

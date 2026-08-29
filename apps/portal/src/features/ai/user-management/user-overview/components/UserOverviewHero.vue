<!--
  用户详情 Hero 区块:标题/状态/快捷操作 + 基础资料卡。
  重构:el-tag → DsTag(tone 语义色),颜色全部走 --ds-* token;props/emits 不变。
-->
<script setup lang="ts">
import { computed } from "vue";

import { DsTag } from "@/shared/ui";

import type { EndUserItem } from "@/api/types/platformTenant";
import { formatDateTime } from "../formatters";

const props = defineProps<{
  userId: string;
  user: EndUserItem | null;
  loading: boolean;
  lastActivityTime: number | null;
  groupAccessibleCount: number;
  warnings: string[];
  aiAvailable: boolean;
}>()

const emit = defineEmits<{
  (e: "back"): void;
  (e: "edit-user"): void;
  (e: "open-group-policy"): void;
  (e: "refresh"): void;
}>()

const statusText = computed(() => {
  if (!props.user) return "资料未命中";
  return props.user.status === 1 ? "正常" : "停用";
});

const statusTone = computed<"neutral" | "positive" | "danger">(() => {
  if (!props.user) return "neutral";
  return props.user.status === 1 ? "positive" : "danger";
});

const detailItems = computed(() => [
  { label: "用户 ID", value: props.user?.userId || props.userId },
  { label: "邮箱", value: props.user?.email || "—" },
  { label: "手机号", value: props.user?.phone || "—" },
  { label: "内部备注", value: props.user?.internalNote || "—" },
  { label: "注册时间", value: formatDateTime(props.user?.createdTime) },
  { label: "上次登录", value: formatDateTime(props.user?.lastLoginTime) },
  { label: "最近活跃", value: formatDateTime(props.lastActivityTime) },
  { label: "AI 可见分组", value: `${props.groupAccessibleCount}` }
]);
</script>

<template>
  <section v-loading="loading" class="hero">
    <div class="hero-main">
      <div class="hero-copy">
        <p class="hero-eyebrow">User Detail</p>
        <div class="hero-title-row">
          <h1 class="hero-title">{{ user?.username || `用户 ${userId}` }}</h1>
          <DsTag :tone="statusTone">{{ statusText }}</DsTag>
        </div>
        <p class="hero-subtitle">
          把基础资料、充值、AI 配置和风险信号放在一个上下文里看，避免在多个业务模块之间来回切换。
        </p>
      </div>

      <div class="hero-actions">
        <el-button plain @click="emit('back')">返回列表</el-button>
        <el-button plain :disabled="!user" @click="emit('edit-user')">编辑用户</el-button>
        <el-button plain :disabled="!user || !aiAvailable" @click="emit('open-group-policy')">分组策略</el-button>
        <el-button type="primary" @click="emit('refresh')">刷新数据</el-button>
      </div>
    </div>

    <div class="hero-details">
      <div v-for="item in detailItems" :key="item.label" class="detail-card">
        <span class="detail-label">{{ item.label }}</span>
        <span class="detail-value">{{ item.value }}</span>
      </div>
    </div>

    <el-alert
      v-if="warnings.length"
      type="warning"
      :closable="false"
      class="hero-alert"
      title="部分数据未完全命中"
      :description="warnings.join(' ')"
    />
  </section>
</template>

<style scoped>
.hero {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 24px;
  border: 1px solid color-mix(in srgb, var(--ds-accent) 14%, var(--ds-line));
  border-radius: var(--ds-radius-panel);
  background: linear-gradient(
    90deg,
    color-mix(in srgb, var(--ds-accent) 9%, var(--ds-panel)) 0%,
    color-mix(in srgb, var(--ds-accent) 3%, var(--ds-panel)) 42%,
    var(--ds-panel) 70%
  );
  box-shadow: var(--ds-shadow-sm);
}

.hero-main {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
}

.hero-copy {
  max-width: 760px;
}

.hero-eyebrow {
  margin: 0 0 10px;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--ds-faint);
}

.hero-title-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.hero-title {
  margin: 0;
  font-size: 32px;
  line-height: 1.05;
  font-weight: 680;
  letter-spacing: -0.03em;
  color: var(--ds-ink);
}

.hero-subtitle {
  margin: 14px 0 0;
  max-width: 760px;
  font-size: 14px;
  line-height: 1.7;
  color: var(--ds-muted);
}

.hero-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  flex-wrap: wrap;
}

.hero-details {
  display: grid;
  grid-template-columns: repeat(7, minmax(0, 1fr));
  gap: 12px;
}

.detail-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-height: 90px;
  padding: 14px 16px;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-control);
  background: var(--ds-panel);
}

.detail-label {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--ds-muted);
}

.detail-value {
  font-size: 14px;
  line-height: 1.5;
  font-weight: 700;
  color: var(--ds-ink);
  word-break: break-word;
}

.hero-alert {
  border-radius: var(--ds-radius-panel);
}

@media (max-width: 1280px) {
  .hero-details {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}

@media (max-width: 960px) {
  .hero-main {
    flex-direction: column;
  }

  .hero-actions {
    justify-content: flex-start;
  }

  .hero-details {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .hero {
    padding: 18px;
  }

  .hero-title {
    font-size: 28px;
  }

  .hero-details {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>

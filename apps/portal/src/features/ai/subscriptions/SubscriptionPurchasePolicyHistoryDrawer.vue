<script setup lang="ts">
import { shallowRef, watch } from "vue";
import { ElMessage } from "element-plus";

import { aiTenantApi } from "@/api/aiTenant";
import type {
  TenantSubPlan,
  TenantSubPurchasePolicyRevision
} from "@/api/types/aiTenant";
import { subscriptionPurchasePolicyLabel } from "./subscriptionPurchasePolicy";

const props = defineProps<{
  plan: TenantSubPlan | null;
}>();

const visible = defineModel<boolean>({ required: true });
const loading = shallowRef(false);
const revisions = shallowRef<TenantSubPurchasePolicyRevision[]>([]);

function formatTime(value: string): string {
  return new Date(value).toLocaleString("zh-CN", { hour12: false });
}

async function loadRevisions() {
  if (!props.plan) return;
  loading.value = true;
  try {
    const result =
      await aiTenantApi.listSubscriptionPlanPurchasePolicyRevisions(
        props.plan.id
      );
    revisions.value = result?.items ?? [];
  } catch (error) {
    revisions.value = [];
    const detail = error as { detail?: string; message?: string };
    ElMessage.error(
      detail.detail ||
        (detail.message && !/^HTTP \d+$/.test(detail.message)
          ? detail.message
          : "加载购买限制记录失败")
    );
  } finally {
    loading.value = false;
  }
}

watch(visible, (isOpen) => {
  if (isOpen) void loadRevisions();
});
</script>

<template>
  <el-drawer
    v-model="visible"
    :title="plan ? `${plan.name} · 购买限制记录` : '购买限制记录'"
    size="min(520px, 100vw)"
    append-to-body
  >
    <div v-loading="loading" class="history-list">
      <el-empty v-if="!loading && !revisions.length" description="暂无变更记录" />
      <el-timeline v-else>
        <el-timeline-item
          v-for="revision in revisions"
          :key="revision.version"
          :timestamp="formatTime(revision.changed_at)"
          placement="top"
        >
          <div class="history-item">
            <div class="history-item__head">
              <strong>版本 {{ revision.version }}</strong>
              <el-tag
                v-if="revision.version === plan?.purchase_policy.version"
                size="small"
                type="success"
              >当前版本</el-tag>
            </div>
            <p>{{ subscriptionPurchasePolicyLabel(revision.policy) }}</p>
            <small v-if="revision.policy.period_type === 'calendar'">
              周期时区：{{ revision.policy.calendar_timezone }}
            </small>
            <small>操作人：{{ revision.changed_by || "系统迁移" }}</small>
          </div>
        </el-timeline-item>
      </el-timeline>
    </div>
  </el-drawer>
</template>

<style scoped>
.history-list {
  min-height: 160px;
}

.history-item {
  display: flex;
  flex-direction: column;
  gap: 5px;
}

.history-item__head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.history-item p {
  margin: 0;
  color: var(--ds-ink);
  font-size: 13px;
  line-height: 1.5;
}

.history-item small {
  color: var(--ds-muted);
  font-size: 12px;
}
</style>

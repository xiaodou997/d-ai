<!-- 批量操作结果弹窗：成功/失败计数 + 失败明细列表 -->
<script setup lang="ts">
import { ref } from 'vue'

const visible = ref(false)
const successCount = ref(0)
const failCount = ref(0)
const failed = ref<{ eventId: string; reason: string }[]>([])

// 兼容两种返回结构：{ successCount, failCount, failed } 或 { succeeded, failed }
const open = (res: any) => {
  successCount.value = res.successCount ?? (res.succeeded?.length ?? 0)
  failCount.value = res.failCount ?? (res.failed?.length ?? 0)
  failed.value = res.failed || []
  visible.value = true
}

defineExpose({ open })
</script>

<template>
  <el-dialog v-model="visible" title="操作结果" width="520" :append-to-body="true">
    <div class="space-y-4">
      <div class="tx-result-stats">
        <div class="tx-result-stat tx-result-stat--success">
          <p class="tx-result-stat__num">{{ successCount }}</p>
          <p class="tx-result-stat__label">成功</p>
        </div>
        <div class="tx-result-stat tx-result-stat--fail">
          <p class="tx-result-stat__num">{{ failCount }}</p>
          <p class="tx-result-stat__label">失败</p>
        </div>
      </div>
      <div v-if="failed.length > 0" class="tx-result-failed">
        <p class="tx-result-failed__title">失败明细</p>
        <div class="tx-result-failed__list">
          <div v-for="f in failed" :key="f.eventId" class="tx-result-failed__item">
            <span class="tx-result-failed__id">{{ f.eventId }}</span>
            <span class="tx-result-failed__reason">{{ f.reason }}</span>
          </div>
        </div>
      </div>
    </div>
    <template #footer>
      <el-button type="primary" @click="visible = false" class="rounded-xl!">关闭</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
/* 结果统计卡 */
.tx-result-stats {
  display: flex;
  gap: 16px;
}

.tx-result-stat {
  flex: 1;
  text-align: center;
  padding: 16px;
  border: 1px solid transparent;
  border-radius: var(--ds-radius-panel);
}

.tx-result-stat__num {
  margin: 0;
  font-size: 26px;
  font-weight: 800;
  letter-spacing: -0.02em;
}

.tx-result-stat__label {
  margin: 4px 0 0;
  font-size: 12px;
  font-weight: 700;
}

.tx-result-stat--success {
  background: var(--ds-positive-soft);
  border-color: color-mix(in srgb, var(--ds-positive) 20%, transparent);
}

.tx-result-stat--success .tx-result-stat__num,
.tx-result-stat--success .tx-result-stat__label {
  color: var(--ds-positive);
}

.tx-result-stat--fail {
  background: var(--ds-danger-soft);
  border-color: color-mix(in srgb, var(--ds-danger) 20%, transparent);
}

.tx-result-stat--fail .tx-result-stat__num,
.tx-result-stat--fail .tx-result-stat__label {
  color: var(--ds-danger);
}

/* 失败明细 */
.tx-result-failed {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.tx-result-failed__title {
  margin: 0;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--ds-muted);
}

.tx-result-failed__list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 192px;
  overflow-y: auto;
}

.tx-result-failed__item {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 6px 12px;
  border-radius: var(--ds-radius-control);
  background: var(--ds-danger-soft);
  font-size: 12px;
}

.tx-result-failed__id {
  font-family: var(--ds-font-mono);
  color: var(--ds-muted);
}

.tx-result-failed__reason {
  color: var(--ds-danger);
}
</style>

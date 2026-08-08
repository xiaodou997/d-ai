<!-- 批量退款弹窗：对选中的 succeeded 记录执行全额退款，不可撤销 -->
<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { platformAdminApi } from '@/api/platformAdmin'
import TransactionAlert from './TransactionAlert.vue'

const emit = defineEmits<{
  done: [result: any]
}>()

const visible = ref(false)
const loading = ref(false)
const rows = ref<any[]>([])
const form = reactive({ reason: '' })

const tenantTotal = computed(() => rows.value.reduce((sum, r) => sum + (r.tenantAmountUsd || 0), 0))
const userTotal = computed(() => rows.value.reduce((sum, r) => sum + (r.userAmountUsd || 0), 0))

const open = (targets: any[]) => {
  rows.value = targets
  form.reason = ''
  visible.value = true
}

defineExpose({ open })

const submit = async () => {
  if (!form.reason.trim()) return ElMessage.warning('请输入退款原因')
  loading.value = true
  try {
    const res = await platformAdminApi.batchRefundEvents({
      eventIds: rows.value.map((r) => r.eventId),
      reason: form.reason
    })
    visible.value = false
    emit('done', res)
  } catch (err: any) {
    ElMessage.error(err?.message || '批量退款失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <el-dialog v-model="visible" title="批量退款" width="520" :close-on-click-modal="false" :append-to-body="true">
    <div class="space-y-4">
      <TransactionAlert tone="danger">
        <p>⚠ 将对以下 {{ rows.length }} 条记录执行全额退款，不可撤销。</p>
        <div class="tx-alert__row">
          <span>租户金额合计：<b>${{ tenantTotal.toLocaleString('en-US', { maximumFractionDigits: 6 }) }}</b></span>
          <span>用户金额合计：<b>${{ userTotal.toLocaleString('en-US', { maximumFractionDigits: 6 }) }}</b></span>
        </div>
      </TransactionAlert>
      <el-form :model="form" label-position="top">
        <el-form-item label="退款原因" required>
          <el-input v-model="form.reason" type="textarea" :rows="3" placeholder="请说明批量退款原因" />
        </el-form-item>
      </el-form>
    </div>
    <template #footer>
      <el-button @click="visible = false" class="rounded-xl!">取消</el-button>
      <el-button type="danger" :loading="loading" @click="submit" class="rounded-xl!">确认退款（{{ rows.length }}条）</el-button>
    </template>
  </el-dialog>
</template>

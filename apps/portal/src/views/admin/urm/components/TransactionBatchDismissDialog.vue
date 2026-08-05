<!-- 批量免除扣费弹窗：逐条调用 adminDismissEvent，汇总成功/失败结果 -->
<script setup lang="ts">
import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { urmAdminApi } from '@/api/urmAdmin'
import TransactionAlert from './TransactionAlert.vue'

const emit = defineEmits<{
  done: [result: any]
}>()

const visible = ref(false)
const loading = ref(false)
const rows = ref<any[]>([])
const form = reactive({ note: '' })

const open = (targets: any[]) => {
  rows.value = targets
  form.note = ''
  visible.value = true
}

defineExpose({ open })

const submit = async () => {
  if (!form.note.trim()) return ElMessage.warning('请输入原因')
  loading.value = true
  const succeeded: string[] = []
  const failed: { eventId: string; reason: string }[] = []
  try {
    for (const row of rows.value) {
      try {
        await urmAdminApi.adminDismissEvent(row.eventId, { note: form.note })
        succeeded.push(row.eventId)
      } catch (err: any) {
        failed.push({ eventId: row.eventId, reason: err?.message || '失败' })
      }
    }
    visible.value = false
    emit('done', { successCount: succeeded.length, failCount: failed.length, succeeded, failed })
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <el-dialog v-model="visible" title="批量免除扣费" width="480" :close-on-click-modal="false" :append-to-body="true">
    <div class="space-y-4">
      <TransactionAlert tone="warning">⚠ 将对 {{ rows.length }} 条记录执行免除，不会扣除任何积分。</TransactionAlert>
      <el-form :model="form" label-position="top">
        <el-form-item label="免除原因" required>
          <el-input v-model="form.note" type="textarea" :rows="3" placeholder="请说明批量免除原因" />
        </el-form-item>
      </el-form>
    </div>
    <template #footer>
      <el-button @click="visible = false" class="rounded-xl!">取消</el-button>
      <el-button type="warning" :loading="loading" @click="submit" class="rounded-xl!">确认免除（{{ rows.length }}条）</el-button>
    </template>
  </el-dialog>
</template>

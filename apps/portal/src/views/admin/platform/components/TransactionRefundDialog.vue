<!-- 单条退款弹窗：全额退回该交易扣除的积分，不可撤销 -->
<script setup lang="ts">
import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { platformAdminApi } from '@/api/platformAdmin'
import TransactionAlert from './TransactionAlert.vue'
import EventSummary from './TransactionEventSummary.vue'

const emit = defineEmits<{
  success: []
}>()

const visible = ref(false)
const loading = ref(false)
const row = ref<any>(null)
const form = reactive({ reason: '' })

const open = (target: any) => {
  row.value = target
  form.reason = ''
  visible.value = true
}

defineExpose({ open })

const submit = async () => {
  if (!form.reason.trim()) return ElMessage.warning('请输入退款原因')
  loading.value = true
  try {
    await platformAdminApi.refund({ eventId: row.value.eventId, reason: form.reason })
    visible.value = false
    ElMessage.success('退款成功')
    emit('success')
  } catch (err: any) {
    ElMessage.error(err?.message || '退款失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <el-dialog v-model="visible" title="确认退款" width="480" :close-on-click-modal="false" :append-to-body="true">
    <div class="space-y-4">
      <TransactionAlert tone="danger">⚠ 此操作将全额退回该交易扣除的积分，不可撤销。</TransactionAlert>
      <EventSummary :row="row" />
      <el-form :model="form" label-position="top">
        <el-form-item label="退款原因" required>
          <el-input v-model="form.reason" type="textarea" :rows="3" placeholder="请输入退款原因" />
        </el-form-item>
      </el-form>
    </div>
    <template #footer>
      <el-button @click="visible = false" class="rounded-xl!">取消</el-button>
      <el-button type="danger" :loading="loading" @click="submit" class="rounded-xl!">确认退款</el-button>
    </template>
  </el-dialog>
</template>

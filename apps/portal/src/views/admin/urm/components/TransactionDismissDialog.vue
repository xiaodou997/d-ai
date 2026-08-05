<!-- 单条免除扣费弹窗：将事件标记为“取消”，不扣除任何积分 -->
<script setup lang="ts">
import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { urmAdminApi } from '@/api/urmAdmin'
import TransactionAlert from './TransactionAlert.vue'
import EventSummary from './TransactionEventSummary.vue'

const emit = defineEmits<{
  success: []
}>()

const visible = ref(false)
const loading = ref(false)
const row = ref<any>(null)
const form = reactive({ note: '' })

const open = (target: any) => {
  row.value = target
  form.note = ''
  visible.value = true
}

defineExpose({ open })

const submit = async () => {
  if (!form.note.trim()) return ElMessage.warning('请输入免除原因')
  loading.value = true
  try {
    await urmAdminApi.adminDismissEvent(row.value.eventId, { note: form.note })
    visible.value = false
    ElMessage.success('已免除扣费')
    emit('success')
  } catch (err: any) {
    ElMessage.error(err?.message || '操作失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <el-dialog v-model="visible" title="免除扣费" width="480" :close-on-click-modal="false" :append-to-body="true">
    <div class="space-y-4">
      <TransactionAlert tone="warning">⚠ 此操作将该事件标记为"取消"，不会扣除任何积分。</TransactionAlert>
      <EventSummary :row="row" />
      <el-form :model="form" label-position="top">
        <el-form-item label="免除原因" required>
          <el-input v-model="form.note" type="textarea" :rows="3" placeholder="请说明免除原因" />
        </el-form-item>
      </el-form>
    </div>
    <template #footer>
      <el-button @click="visible = false" class="rounded-xl!">取消</el-button>
      <el-button type="warning" :loading="loading" @click="submit" class="rounded-xl!">确认免除</el-button>
    </template>
  </el-dialog>
</template>

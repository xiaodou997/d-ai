<!-- 单条手动确认扣款弹窗：事件已自动释放时，从账户余额中重新扣除积分 -->
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
const form = reactive({ actualTenantCredits: 0, actualUserCredits: 0, note: '' })

const open = (target: any) => {
  row.value = target
  form.actualTenantCredits = target.tenantCredits || 0
  form.actualUserCredits = target.userCredits || 0
  form.note = ''
  visible.value = true
}

defineExpose({ open })

const submit = async () => {
  if (!form.note.trim()) return ElMessage.warning('请输入备注')
  loading.value = true
  try {
    await urmAdminApi.manualConfirmEvent(row.value.eventId, {
      actualTenantCredits: form.actualTenantCredits,
      actualUserCredits: form.actualUserCredits,
      note: form.note
    })
    visible.value = false
    ElMessage.success('手动确认成功')
    emit('success')
  } catch (err: any) {
    ElMessage.error(err?.message || '操作失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <el-dialog v-model="visible" title="手动确认扣款" width="520" :close-on-click-modal="false" :append-to-body="true">
    <div class="space-y-4">
      <TransactionAlert tone="info">💡 该事件已被自动释放，冻结积分已归还。此操作将从账户余额中重新扣除积分。</TransactionAlert>
      <EventSummary :row="row" label="原冻结额" />
      <el-form :model="form" label-position="top" class="space-y-2">
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="实扣租户积分">
            <el-input-number v-model="form.actualTenantCredits" :min="0" :controls="false" class="w-full" />
          </el-form-item>
          <el-form-item label="实扣用户积分">
            <el-input-number v-model="form.actualUserCredits" :min="0" :controls="false" class="w-full" />
          </el-form-item>
        </div>
        <el-form-item label="备注" required>
          <el-input v-model="form.note" type="textarea" :rows="3" placeholder="请说明手动确认原因" />
        </el-form-item>
      </el-form>
    </div>
    <template #footer>
      <el-button @click="visible = false" class="rounded-xl!">取消</el-button>
      <el-button type="primary" :loading="loading" @click="submit" class="rounded-xl!">确认扣款</el-button>
    </template>
  </el-dialog>
</template>

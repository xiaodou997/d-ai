<!-- 批量确认扣款弹窗：按原冻结额对选中的 released 记录执行扣款 -->
<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
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

const tenantTotal = computed(() => rows.value.reduce((sum, r) => sum + (r.tenantCredits || 0), 0))
const userTotal = computed(() => rows.value.reduce((sum, r) => sum + (r.userCredits || 0), 0))

const open = (targets: any[]) => {
  rows.value = targets
  form.note = ''
  visible.value = true
}

defineExpose({ open })

const submit = async () => {
  if (!form.note.trim()) return ElMessage.warning('请输入备注')
  loading.value = true
  try {
    const res = await urmAdminApi.batchConfirmEvents({
      eventIds: rows.value.map((r) => r.eventId),
      note: form.note
    })
    visible.value = false
    emit('done', res)
  } catch (err: any) {
    ElMessage.error(err?.message || '批量确认失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <el-dialog v-model="visible" title="批量确认扣款" width="520" :close-on-click-modal="false" :append-to-body="true">
    <div class="space-y-4">
      <TransactionAlert tone="info">
        <p>将按原冻结额对以下 {{ rows.length }} 条记录执行扣款</p>
        <div class="tx-alert__row">
          <span>租户积分合计：<b>{{ tenantTotal.toLocaleString() }}</b></span>
          <span>用户积分合计：<b>{{ userTotal.toLocaleString() }}</b></span>
        </div>
      </TransactionAlert>
      <el-form :model="form" label-position="top">
        <el-form-item label="备注" required>
          <el-input v-model="form.note" type="textarea" :rows="3" placeholder="请说明批量确认原因（将记录在每条流水的操作日志中）" />
        </el-form-item>
      </el-form>
    </div>
    <template #footer>
      <el-button @click="visible = false" class="rounded-xl!">取消</el-button>
      <el-button type="primary" :loading="loading" @click="submit" class="rounded-xl!">确认扣款（{{ rows.length }}条）</el-button>
    </template>
  </el-dialog>
</template>

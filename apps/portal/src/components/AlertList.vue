<template>
  <div class="bg-white rounded-3xl border border-slate-100 shadow-soft overflow-hidden h-full flex flex-col">
    <div class="px-6 py-5 border-b border-slate-100 flex items-center justify-between bg-white sticky top-0 z-10">
      <div class="flex items-center">
        <div class="w-10 h-10 rounded-xl bg-rose-50 flex items-center justify-center mr-3">
          <el-icon class="text-rose-500" :size="20"><WarningFilled /></el-icon>
        </div>
        <div>
          <h3 class="text-base font-bold text-slate-800 leading-tight">核心风险警示</h3>
          <p class="text-xs text-slate-400 font-medium mt-0.5">待处理异常记录</p>
        </div>
      </div>
      <el-tag :type="alertCount > 0 ? 'danger' : 'success'" effect="plain" class="border-none bg-slate-50 text-xs font-bold rounded-lg px-3">
        {{ alertCount }} 项待审
      </el-tag>
    </div>

    <div class="flex-1 overflow-y-auto p-2 space-y-2 custom-scrollbar">
      <div v-if="alertCount === 0" class="py-12 flex flex-col items-center justify-center">
        <div class="w-20 h-20 rounded-full bg-emerald-50 flex items-center justify-center mb-4">
          <el-icon class="text-emerald-500" :size="32"><CircleCheck /></el-icon>
        </div>
        <p class="text-sm font-semibold text-slate-600">系统运行良好</p>
        <p class="text-xs text-slate-400 mt-1">目前没有发现任何风险记录</p>
      </div>

      <!-- 交易失败 -->
      <div v-for="item in failedTransactions" :key="item.eventId" class="group p-4 bg-white hover:bg-amber-50/30 border border-slate-100 rounded-2xl transition-all relative overflow-hidden">
        <div class="absolute left-0 top-0 bottom-0 w-1 bg-amber-500"></div>
        <div class="flex items-start justify-between mb-1">
          <div class="flex items-center">
            <span class="w-8 h-8 rounded-lg bg-amber-50 flex items-center justify-center mr-3 text-amber-500">
              <el-icon><CircleCloseFilled /></el-icon>
            </span>
            <div class="overflow-hidden">
              <p class="text-sm font-bold text-slate-700 truncate">扣费失败: {{ item.eventId }}</p>
              <p class="text-[11px] text-slate-400 font-medium uppercase mt-0.5">{{ formatDateTime(item.createdTime) }}</p>
            </div>
          </div>
        </div>
        <div class="mt-2 bg-slate-50 p-2 rounded-lg">
          <p class="text-xs text-slate-500 leading-relaxed font-medium">
            <span class="text-amber-600 font-bold">原因:</span> {{ item.terminalNote }}
          </p>
        </div>
      </div>

      <!-- 超时预授权 -->
      <div v-for="item in timeoutPreAuths" :key="item.eventId" class="group p-4 bg-white hover:bg-primary-50/30 border border-slate-100 rounded-2xl transition-all relative overflow-hidden">
        <div class="absolute left-0 top-0 bottom-0 w-1 bg-primary-500"></div>
        <div class="flex items-center justify-between mb-2">
          <div class="flex items-center">
            <span class="w-8 h-8 rounded-lg bg-primary-50 flex items-center justify-center mr-3 text-primary-500">
              <el-icon><Clock /></el-icon>
            </span>
            <span class="text-sm font-bold text-slate-700 truncate">授权超时: {{ item.eventId }}</span>
          </div>
        </div>
        <div class="flex justify-between items-center text-[11px]">
          <p class="text-slate-400 font-medium italic">于 {{ formatDateTime(item.createdTime) }} 创建</p>
          <el-button size="small" type="danger" plain :loading="cancellingId === item.eventId" @click="handleCancel(item.eventId)">手动取消</el-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { WarningFilled, Clock, CircleCloseFilled, CircleCheck } from '@element-plus/icons-vue'
import { platformAdminApi } from '../api/platformAdmin'

const props = defineProps<{
  timeoutPreAuths: any[]
  failedTransactions: any[]
}>()

const emit = defineEmits<{ (e: 'refresh'): void }>()

const cancellingId = ref('')

const alertCount = computed(() => props.timeoutPreAuths.length + props.failedTransactions.length)

const formatDateTime = (ts?: number) => {
  if (!ts) return '-'
  const d = new Date(ts)
  return `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

const handleCancel = async (eventId: string) => {
  cancellingId.value = eventId
  try {
    await platformAdminApi.cancelPreAuth(eventId)
    ElMessage.success('已手动取消预授权，积分冻结已释放')
    emit('refresh')
  } catch (e: any) {
    ElMessage.error(e?.message || '取消失败')
  } finally {
    cancellingId.value = ''
  }
}
</script>

<style scoped>
.custom-scrollbar::-webkit-scrollbar {
  width: 4px;
}
</style>

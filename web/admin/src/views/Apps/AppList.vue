<template>
  <div class="page-container space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <!-- 筛选面板 -->
    <div class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
      <div class="flex justify-between items-center mb-6">
        <h2 class="text-lg font-bold text-slate-800">业务系统管理</h2>
        <el-button type="primary" class="!rounded-2xl font-bold px-6 h-11" @click="handleCreate">
          <el-icon class="mr-1"><Plus /></el-icon>
          创建系统
        </el-button>
      </div>
      <div class="flex gap-3 items-end">
        <div class="space-y-1">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">系统名称</label>
          <el-input
            v-model="keyword"
            placeholder="搜索系统名称"
            class="modern-input w-72"
            clearable
            @clear="loadApps"
            @keyup.enter="loadApps"
          />
        </div>
        <el-button type="primary" class="!rounded-2xl font-bold px-8 h-11" @click="loadApps">筛选</el-button>
      </div>
    </div>

    <!-- 数据表格 -->
    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
      <el-table
        :data="apps"
        v-loading="loading"
        style="width: 100%"
        border
        stripe
        class="modern-table"
      >
        <el-table-column prop="appKey" label="AppKey" width="180">
          <template #default="{ row }">
            <span class="text-xs font-mono font-bold text-slate-500">{{ row.appKey }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="appName" label="系统名称" width="180" />
        <el-table-column prop="description" label="业务描述" min-width="200" show-overflow-tooltip />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'danger'" size="small" class="rounded-2xl font-bold">
              {{ row.status === 1 ? '启用' : '禁用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="170">
          <template #default="{ row }">
            <span class="text-xs text-slate-400 font-medium">{{ formatTime(row.createdAt) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="handleEdit(row)">编辑</el-button>
            <el-button link type="warning" @click="handleResetSecret(row)">重置 Secret</el-button>
            <el-button link type="danger" @click="handleDelete(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="p-6 border-t border-slate-50 flex justify-end">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="size"
          :total="total"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          @size-change="loadApps"
          @current-change="loadApps"
          class="modern-pagination"
        />
      </div>
    </div>

    <!-- 创建/编辑弹窗 -->
    <AppFormDialog
      v-model:visible="dialogVisible"
      :edit-data="editData"
      @success="loadApps"
    />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { getApps, deleteApp, resetSecret } from '@/api/apps'
import AppFormDialog from './AppFormDialog.vue'
import dayjs from 'dayjs'

const formatTime = (timestamp) => {
  if (!timestamp) return '-'
  return dayjs(timestamp).format('YYYY-MM-DD HH:mm:ss')
}

const loading = ref(false)
const apps = ref([])
const keyword = ref('')
const page = ref(1)
const size = ref(15)
const total = ref(0)
const dialogVisible = ref(false)
const editData = ref(null)

const loadApps = async () => {
  loading.value = true
  try {
    const res = await getApps({ page: page.value, size: size.value, keyword: keyword.value })
    apps.value = res.records || []; total.value = res.total || 0
  } catch (error) {
    ElMessage.error('加载失败：' + error.message)
  } finally { loading.value = false }
}

const handleCreate = () => { editData.value = null; dialogVisible.value = true }
const handleEdit = (row) => { editData.value = { ...row }; dialogVisible.value = true }

const handleDelete = (row) => {
  ElMessageBox.confirm(`确定要删除业务系统 "${row.appName}" 吗？`, '警告', {
    confirmButtonText: '确定', cancelButtonText: '取消', type: 'warning', roundButton: true
  }).then(async () => {
    try {
      await deleteApp(row.id); ElMessage.success('删除成功'); loadApps()
    } catch (error) { ElMessage.error('删除失败：' + error.message) }
  }).catch(() => {})
}

const handleResetSecret = (row) => {
  ElMessageBox.confirm(`确定要重置 "${row.appName}" 的 Secret 吗？`, '警告', {
    confirmButtonText: '确定', cancelButtonText: '取消', type: 'warning', roundButton: true
  }).then(async () => {
    try {
      const res = await resetSecret(row.id)
      ElMessageBox.alert(`新凭证（只显示一次）：<br/><br/>AppKey: ${res.appKey}<br/>Secret: ${res.appSecret}`, '重置成功', {
        dangerouslyUseHTMLString: true, type: 'warning', roundButton: true
      })
      loadApps()
    } catch (error) { ElMessage.error('重置失败：' + error.message) }
  }).catch(() => {})
}

onMounted(loadApps)
</script>


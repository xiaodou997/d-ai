<template>
  <div class="page-container space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <!-- 筛选面板 -->
    <div class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
      <div class="flex justify-between items-center mb-6">
        <h2 class="text-lg font-bold text-slate-800">终端用户</h2>
      </div>
      <el-form :model="queryForm" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 xl:grid-cols-5 gap-6 items-end">
        <div class="space-y-1">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">租户名称</label>
          <el-input v-model="queryForm.tenantName" placeholder="租户名称" clearable class="modern-input" />
        </div>
        <div class="space-y-1">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">用户名</label>
          <el-input v-model="queryForm.username" placeholder="用户名" clearable class="modern-input" />
        </div>
        <div class="space-y-1">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">状态</label>
          <el-select v-model="queryForm.status" placeholder="状态" clearable class="modern-select w-full">
            <el-option label="全部" value="" />
            <el-option label="正常" :value="1" />
            <el-option label="禁用" :value="2" />
            <el-option label="锁定" :value="3" />
          </el-select>
        </div>
        <div class="flex gap-2 items-end">
          <el-button type="primary" class="!rounded-2xl font-bold px-8 h-11 flex-1" @click="handleSearch">筛选</el-button>
          <el-button @click="handleReset" class="!rounded-2xl h-11 px-6">重置</el-button>
        </div>
      </el-form>
    </div>

    <!-- 数据表格 -->
    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
      <el-table v-loading="loading" :data="tableData" border stripe class="modern-table w-full">
        <el-table-column prop="userId" label="用户 ID" min-width="120" />
        <el-table-column prop="username" label="用户名" min-width="120" />
        <el-table-column label="归属租户" min-width="200">
          <template #default="{ row }">
            <el-link type="primary" class="font-bold" @click="$router.push(`/tenants/${row.tenantId}`)">
              {{ row.tenantName || row.tenantId }}
            </el-link>
          </template>
        </el-table-column>
        <el-table-column prop="email" label="邮箱" min-width="180" show-overflow-tooltip />
        <el-table-column label="状态" min-width="100">
          <template #default="{ row }">
            <el-tag :type="getStatusTag(row.status)" size="small" class="rounded-2xl font-bold">
              {{ row.statusDisplay }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="账户积分" min-width="130" align="right">
          <template #default="{ row }">
            <span class="font-black text-slate-700">{{ row.credits?.toLocaleString() }} 积分</span>
          </template>
        </el-table-column>
        <el-table-column prop="lastLoginTime" label="最后登录" min-width="170">
          <template #default="{ row }">
            <span class="text-xs text-slate-400">{{ formatTime(row.lastLoginTime) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="createdTime" label="注册时间" min-width="170">
          <template #default="{ row }">
            <span class="text-xs text-slate-400">{{ formatTime(row.createdTime) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" fixed="right" width="100">
          <template #default="{ row }">
            <el-button 
              link 
              :type="row.status === 1 ? 'warning' : 'success'"
              @click="handleToggleStatus(row)"
            >
              {{ row.status === 1 ? '禁用' : '启用' }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="p-6 border-t border-slate-50 flex justify-end">
        <el-pagination
          v-model:current-page="pagination.page"
          v-model:page-size="pagination.size"
          :total="pagination.total"
          layout="total, prev, pager, next"
          @current-change="fetchData"
          class="modern-pagination"
        />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { queryUsers, disableEndUser, enableEndUser } from '@/api/tenant'

const queryForm = reactive({ tenantId: '', userId: '', username: '', status: '' })
const pagination = reactive({ page: 1, size: 15, total: 0 })
const tableData = ref([])
const loading = ref(false)

const handleToggleStatus = async (row) => {
  const targetStatus = row.status === 1 ? 2 : 1
  const actionText = targetStatus === 1 ? '启用' : '禁用'
  
  try {
    await ElMessageBox.confirm(`确定要${actionText}该用户吗？`, '状态变更确认', { 
      confirmButtonText: `立即${actionText}`,
      cancelButtonText: '取消',
      type: 'warning',
      roundButton: true
    })
    
    if (targetStatus === 2) {
      await disableEndUser(row.userId)
    } else {
      await enableEndUser(row.userId)
    }
    
    ElMessage.success(`${actionText}成功`)
    fetchData()
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(`${actionText}失败`)
  }
}

const getStatusTag = (status) => {
  const map = { 1: 'success', 2: 'info', 3: 'danger', 4: 'warning' }
  return map[status] || ''
}

const getStatusDisplay = (status) => {
  const map = { 1: '正常', 2: '禁用', 3: '锁定', 4: '级联停用' }
  return map[status] || '未知'
}

const formatTime = (ts) => {
  if (!ts) return ''
  const date = new Date(ts)
  return date.toLocaleString()
}

const fetchData = async () => {
  loading.value = true
  try {
    const data = await queryUsers({ ...queryForm, page: pagination.page, size: pagination.size })
    // 前端转换状态显示
    tableData.value = data.records.map(r => ({
      ...r,
      statusDisplay: getStatusDisplay(r.status)
    }))
    pagination.total = data.total
  } catch (error) {
    ElMessage.error('查询用户列表失败')
  } finally {
    loading.value = false
  }
}

const handleSearch = () => { pagination.page = 1; fetchData() }
const handleReset = () => { Object.assign(queryForm, { tenantId: '', userId: '', username: '', status: '' }); handleSearch() }

onMounted(fetchData)
</script>

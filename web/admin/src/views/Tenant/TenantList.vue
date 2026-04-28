<template>
  <div class="page-container space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <!-- 筛选面板 -->
    <div class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
      <div class="flex justify-between items-center mb-6">
        <h2 class="text-lg font-bold text-slate-800">租户管理</h2>
        <el-button type="primary" class="!rounded-2xl font-bold px-6 h-11" @click="handleCreate">
          <el-icon class="mr-1"><Plus /></el-icon>
          创建租户
        </el-button>
      </div>

      <el-form :model="queryForm" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 items-end">
        <div class="space-y-1">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">租户名称 / 联系人 / 邮箱</label>
          <el-input v-model="queryForm.keyword" placeholder="搜索名称、联系人或邮箱" clearable class="modern-input" />
        </div>
        <div class="space-y-1">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">状态</label>
          <el-select v-model="queryForm.status" placeholder="状态" clearable class="modern-select w-full">
            <el-option label="全部" value="" />
            <el-option label="正常" :value="1" />
            <el-option label="停用" :value="2" />
            <el-option label="欠费封禁" :value="3" />
          </el-select>
        </div>
        <div class="flex gap-2">
          <el-button type="primary" class="!rounded-2xl font-bold px-8 h-11" @click="handleSearch">筛选</el-button>
          <el-button @click="handleReset" class="!rounded-2xl h-11 px-6">重置</el-button>
        </div>
      </el-form>
    </div>

    <!-- 数据表格 -->
    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
      <el-table v-loading="loading" :data="tableData" border stripe class="modern-table w-full">
        <el-table-column prop="tenantId" label="租户 ID" width="150" />
        <el-table-column prop="tenantName" label="租户名称" min-width="200">
          <template #default="{ row }">
            <el-link type="primary" class="font-bold" @click="$router.push(`/tenants/${row.tenantId}`)">
              {{ row.tenantName }}
            </el-link>
          </template>
        </el-table-column>
        <el-table-column prop="contactPerson" label="联系人" min-width="120" />
        <el-table-column prop="contactEmail" label="联系邮箱" min-width="200" show-overflow-tooltip />
        <el-table-column label="状态" min-width="120">
          <template #default="{ row }">
            <el-tag :type="getStatusTag(row.status)" size="small" class="rounded-2xl font-bold">
              {{ row.statusDisplay }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="平台积分" min-width="150" align="right">
          <template #default="{ row }">
            <span class="font-black" :class="row.status === 3 ? 'text-rose-500' : 'text-slate-700'">
              {{ row.credits?.toLocaleString() }} 积分
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="userCount" label="用户数" min-width="100" align="center" />
        <el-table-column prop="createdTime" label="入驻时间" min-width="170">
          <template #default="{ row }">
            <span class="text-xs text-slate-400">{{ formatTime(row.createdTime) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" fixed="right" width="190">
          <template #default="{ row }">
            <el-button link type="primary" @click="handleEdit(row)">编辑</el-button>
            <el-button
              link
              :type="row.status === 1 ? 'warning' : 'success'"
              @click="handleToggleStatus(row)"
            >
              {{ row.status === 1 ? '停用' : '启用' }}
            </el-button>
            <el-popconfirm title="确定删除该租户吗？" @confirm="handleDelete(row.tenantId)">
              <template #reference>
                <el-button link type="danger">删除</el-button>
              </template>
            </el-popconfirm>
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

    <!-- 租户表单对话框 -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? '编辑租户' : '创建租户'"
      width="560px"
      append-to-body
      custom-class="modern-dialog"
    >
      <el-form :model="form" :rules="rules" ref="formRef" label-position="top">
        <el-form-item label="租户名称" prop="tenantName">
          <el-input v-model="form.tenantName" placeholder="请输入租户名称" />
        </el-form-item>
        <div class="grid grid-cols-2 gap-4">
          <el-form-item label="联系人" prop="contactPerson">
            <el-input v-model="form.contactPerson" placeholder="姓名" />
          </el-form-item>
          <el-form-item label="联系邮箱" prop="contactEmail">
            <el-input v-model="form.contactEmail" placeholder="email@example.com" />
          </el-form-item>
        </div>
        <el-form-item label="状态" prop="status">
          <el-select v-model="form.status" class="w-full">
            <el-option label="正常启用" :value="1" />
            <el-option label="初始停用" :value="2" />
          </el-select>
        </el-form-item>

        <!-- 创建时才显示以下选项 -->
        <template v-if="!isEdit">
          <el-divider content-position="left" class="!my-4">
            <span class="text-xs text-slate-400">初始管理员账号（可选）</span>
          </el-divider>
          <div class="grid grid-cols-2 gap-4">
            <el-form-item label="用户名" prop="initUsername">
              <el-input v-model="form.initUsername" placeholder="留空则不创建" />
            </el-form-item>
            <el-form-item label="邮箱" prop="initEmail">
              <el-input v-model="form.initEmail" placeholder="管理员邮箱（可选）" />
            </el-form-item>
          </div>
          <p v-if="form.initUsername" class="text-xs text-amber-500 -mt-2 mb-4">初始密码为 <strong>123456</strong>，请提醒租户及时修改</p>
        </template>
      </el-form>
      <template #footer>
        <div class="flex justify-end gap-2">
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" @click="handleSubmit" :loading="submitting">确定</el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { queryTenants, createTenant, updateTenant, deleteTenant, disableTenant, enableTenant } from '@/api/tenant'

const queryForm = reactive({ keyword: '', status: '' })
const pagination = reactive({ page: 1, size: 15, total: 0 })
const tableData = ref([])
const loading = ref(false)

// 表单相关
const dialogVisible = ref(false)
const isEdit = ref(false)
const submitting = ref(false)
const formRef = ref(null)
const form = reactive({
  tenantId: '',
  tenantName: '',
  contactPerson: '',
  contactEmail: '',
  status: 1,
  appKeys: [],
  initUsername: '',
  initEmail: ''
})

const rules = {
  tenantName: [{ required: true, message: '请输入租户名称', trigger: 'blur' }],
  contactEmail: [{ type: 'email', message: '请输入正确的邮箱地址', trigger: 'blur' }]
}

const getStatusTag = (status) => {
  const map = { 1: 'success', 2: 'info', 3: 'danger' }
  return map[status] || ''
}

const formatTime = (ts) => {
  if (!ts) return ''
  const date = new Date(ts)
  return date.toLocaleString()
}

const fetchData = async () => {
  loading.value = true
  try {
    const data = await queryTenants({ ...queryForm, page: pagination.page, size: pagination.size })
    tableData.value = data.records
    pagination.total = data.total
  } catch (error) {
    ElMessage.error('获取列表失败')
  } finally {
    loading.value = false
  }
}

const handleSearch = () => { pagination.page = 1; fetchData() }
const handleReset = () => { Object.assign(queryForm, { keyword: '', status: '' }); handleSearch() }

const handleCreate = () => {
  isEdit.value = false
  Object.assign(form, { tenantId: '', tenantName: '', contactPerson: '', contactEmail: '', status: 1, appKeys: [], initUsername: '', initEmail: '' })
  dialogVisible.value = true
}

const handleEdit = (row) => {
  isEdit.value = true
  Object.assign(form, { ...row })
  dialogVisible.value = true
}

const handleToggleStatus = async (row) => {
  const isDisable = row.status === 1
  const actionText = isDisable ? '停用' : '启用'
  const tips = isDisable ? '停用后将级联停用该租户下的所有组织用户和终端用户，确定继续吗？' : `确定要启用该租户吗？启用后将恢复被级联停用的用户。`

  try {
    await ElMessageBox.confirm(tips, '状态变更确认', {
      confirmButtonText: `立即${actionText}`,
      cancelButtonText: '取消',
      type: 'warning',
      roundButton: true
    })

    if (isDisable) {
      await disableTenant(row.tenantId)
    } else {
      await enableTenant(row.tenantId)
    }

    ElMessage.success(`${actionText}成功`)
    fetchData()
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(`${actionText}失败`)
  }
}

const handleSubmit = async () => {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (valid) {
      submitting.value = true
      try {
        if (isEdit.value) {
          await updateTenant(form.tenantId, form)
          ElMessage.success('更新成功')
        } else {
          const res = await createTenant(form)
          if (res.initUserId) {
            ElMessage({ type: 'success', message: `创建成功！初始管理员「${res.initUsername}」已创建，默认密码：123456`, duration: 6000 })
          } else {
            ElMessage.success('创建成功')
          }
        }
        dialogVisible.value = false
        fetchData()
      } catch (error) {
        ElMessage.error(error.response?.data?.message || '操作失败')
      } finally {
        submitting.value = false
      }
    }
  })
}

const handleDelete = async (id) => {
  try {
    await deleteTenant(id)
    ElMessage.success('删除成功')
    fetchData()
  } catch (error) {
    ElMessage.error('删除失败')
  }
}

onMounted(fetchData)
</script>

<style scoped lang="scss">
.modern-dialog {
  :deep(.el-dialog__header) {
    border-bottom: 1px solid #f8fafc;
    margin-right: 0;
    padding: 24px;
  }
  :deep(.el-dialog__body) {
    padding: 24px;
  }
}
</style>

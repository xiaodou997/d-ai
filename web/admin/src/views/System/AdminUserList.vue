<template>
  <div class="page-container space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
    <div class="bg-white p-6 rounded-2xl border border-slate-50 shadow-soft">
      <div class="flex justify-between items-center mb-6">
        <div>
          <h2 class="text-lg font-bold text-slate-800">平台管理员</h2>
          <p class="text-xs text-slate-400 mt-1">管理能够登录控制台的平台级管理员账号</p>
        </div>
        <el-button type="primary" class="!rounded-2xl font-bold px-6 h-11" @click="handleCreate">
          <el-icon class="mr-1"><Plus /></el-icon>
          添加管理员
        </el-button>
      </div>

      <!-- 搜索栏 -->
      <div class="flex gap-3 items-end">
        <div class="space-y-1">
          <label class="text-[10px] font-black text-slate-400 uppercase ml-1 block">用户名 / 邮箱</label>
          <el-input
            v-model="keyword"
            placeholder="搜索用户名或邮箱"
            clearable
            class="modern-input w-72"
            @keyup.enter="handleSearch"
          />
        </div>
        <el-button type="primary" class="!rounded-2xl font-bold px-8 h-11" @click="handleSearch">筛选</el-button>
        <el-button class="!rounded-2xl h-11 px-6" @click="keyword = ''; handleSearch()">重置</el-button>
      </div>
    </div>

    <!-- 数据表格 -->
    <div class="bg-white rounded-2xl border border-slate-50 shadow-soft overflow-hidden">
      <el-table v-loading="loading" :data="tableData" border stripe class="modern-table w-full">
        <el-table-column prop="userId" label="用户 ID" min-width="160" />
        <el-table-column prop="username" label="用户名" min-width="130" />
        <el-table-column prop="email" label="邮箱" min-width="200" show-overflow-tooltip />
        <el-table-column label="状态" min-width="100">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small" class="rounded-2xl font-bold">
              {{ row.statusText }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="createdTime" label="创建时间" min-width="170">
          <template #default="{ row }">
            <span class="text-xs text-slate-400">{{ formatTime(row.createdTime) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" fixed="right" width="180">
          <template #default="{ row }">
            <el-button link type="primary" @click="handleEdit(row)">编辑</el-button>
            <el-button link type="warning" @click="handleResetPassword(row)">重置密码</el-button>
            <el-popconfirm title="确定删除该管理员吗？" @confirm="handleDelete(row.userId)">
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

    <!-- 创建/编辑对话框 -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? '编辑管理员' : '创建管理员'"
      width="440px"
      append-to-body
    >
      <el-form :model="form" :rules="rules" ref="formRef" label-position="top">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" placeholder="登录用户名" :disabled="isEdit" />
        </el-form-item>
        <el-form-item label="邮箱" prop="email">
          <el-input v-model="form.email" placeholder="邮箱地址（可选）" />
        </el-form-item>
        <el-form-item v-if="isEdit" label="状态" prop="status">
          <el-select v-model="form.status" class="w-full">
            <el-option label="正常" :value="1" />
            <el-option label="禁用" :value="2" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleSubmit">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { listSystemAdmins, createSystemAdmin, updateSystemAdmin, deleteSystemAdmin } from '@/api/system'

const keyword = ref('')
const pagination = reactive({ page: 1, size: 20, total: 0 })
const tableData = ref([])
const loading = ref(false)
const dialogVisible = ref(false)
const isEdit = ref(false)
const submitting = ref(false)
const formRef = ref(null)
const form = reactive({ userId: '', username: '', email: '', status: 1 })

const rules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }]
}

const formatTime = (ts) => {
  if (!ts) return ''
  return new Date(ts).toLocaleString()
}

const fetchData = async () => {
  loading.value = true
  try {
    const data = await listSystemAdmins({ keyword: keyword.value, page: pagination.page, size: pagination.size })
    tableData.value = data.records
    pagination.total = data.total
  } catch {
    ElMessage.error('获取列表失败')
  } finally {
    loading.value = false
  }
}

const handleSearch = () => { pagination.page = 1; fetchData() }

const handleCreate = () => {
  isEdit.value = false
  Object.assign(form, { userId: '', username: '', email: '', status: 1 })
  dialogVisible.value = true
}

const handleEdit = (row) => {
  isEdit.value = true
  Object.assign(form, { userId: row.userId, username: row.username, email: row.email, status: row.status })
  dialogVisible.value = true
}

const handleSubmit = async () => {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    submitting.value = true
    try {
      if (isEdit.value) {
        await updateSystemAdmin(form.userId, { email: form.email, status: form.status })
        ElMessage.success('更新成功')
      } else {
        await createSystemAdmin({ username: form.username, email: form.email })
        ElMessage({ type: 'success', message: `管理员「${form.username}」已创建，默认密码：123456`, duration: 6000 })
      }
      dialogVisible.value = false
      fetchData()
    } catch (err) {
      ElMessage.error(err.message || '操作失败')
    } finally {
      submitting.value = false
    }
  })
}

const handleResetPassword = async (row) => {
  try {
    await ElMessageBox.confirm(`确定将「${row.username}」的密码重置为 123456 吗？`, '重置密码', {
      confirmButtonText: '确定重置',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await updateSystemAdmin(row.userId, { email: row.email, status: row.status, password: '123456' })
    ElMessage.success('密码已重置为 123456')
  } catch (err) {
    if (err !== 'cancel') ElMessage.error('重置失败')
  }
}

const handleDelete = async (id) => {
  try {
    await deleteSystemAdmin(id)
    ElMessage.success('已删除')
    fetchData()
  } catch {
    ElMessage.error('删除失败')
  }
}

onMounted(fetchData)
</script>

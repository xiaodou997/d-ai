<template>
  <el-dialog
    v-model="dialogVisible"
    :title="isEdit ? '编辑业务系统' : '创建业务系统'"
    width="500px"
    :close-on-click-modal="false"
    :append-to-body="true"
    @close="handleClose"
  >
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      label-width="100px"
      label-position="right"
    >
      <el-form-item label="系统名称" prop="appName">
        <el-input
          v-model="form.appName"
          placeholder="请输入系统名称"
          maxlength="50"
          show-word-limit
        />
      </el-form-item>

      <el-form-item label="描述" prop="description">
        <el-input
          v-model="form.description"
          type="textarea"
          :rows="3"
          placeholder="请输入描述信息（可选）"
          maxlength="200"
          show-word-limit
        />
      </el-form-item>

      <el-form-item label="状态" prop="status">
        <el-switch
          v-model="form.status"
          :active-value="1"
          :inactive-value="0"
          active-text="启用"
          inactive-text="禁用"
        />
      </el-form-item>

    </el-form>

    <template #footer>
      <el-button @click="dialogVisible = false">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">
        确定
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, reactive, computed, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { createApp, updateApp } from '@/api/apps'

// Props
const props = defineProps({
  visible: {
    type: Boolean,
    default: false
  },
  editData: {
    type: Object,
    default: null
  }
})

// Emit
const emit = defineEmits(['update:visible', 'success'])

// 数据
const formRef = ref(null)
const dialogVisible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val)
})

const isEdit = computed(() => !!props.editData)
const submitting = ref(false)

// 表单
const form = reactive({
  appName: '',
  description: '',
  status: 1
})

// 验证规则
const rules = {
  appName: [
    { required: true, message: '请输入系统名称', trigger: 'blur' },
    { min: 2, max: 50, message: '长度在 2 到 50 个字符', trigger: 'blur' }
  ]
}

// 监听编辑数据变化
watch(() => props.editData, (newVal) => {
  if (newVal) {
    form.appName = newVal.appName
    form.description = newVal.description || ''
    form.status = newVal.status
  } else {
    // 重置表单
    form.appName = ''
    form.description = ''
    form.status = 1
  }
}, { immediate: true })

// 关闭
const handleClose = () => {
  formRef.value?.resetFields()
}

// 提交
const handleSubmit = async () => {
  if (!formRef.value) return

  await formRef.value.validate(async (valid) => {
    if (!valid) return

    submitting.value = true
    try {
      if (isEdit.value) {
        // 更新
        await updateApp(props.editData.id, {
          appName: form.appName,
          description: form.description,
          status: form.status
        })
        ElMessage.success('更新成功')
      } else {
        // 创建
        const res = await createApp({
          appName: form.appName,
          description: form.description,
          status: form.status
        })

        ElMessage.success('创建成功')

        // 显示 AppKey 和 Secret
        ElMessageBox.alert(
          `业务系统创建成功！请妥善保存以下凭证：<br/><br/>
           <strong>AppKey:</strong> ${res.appKey}<br/>
           <strong>Secret:</strong> ${res.appSecret}<br/><br/>
           <span style="color: #f56c6c">⚠️ 此信息只显示一次，请立即复制保存！</span>`,
          '创建成功',
          {
            dangerouslyUseHTMLString: true,
            confirmButtonText: '复制并关闭',
            type: 'warning'
          }
        ).then(() => {
          // 复制到剪贴板
          const text = `AppKey: ${res.appKey}\nSecret: ${res.appSecret}`
          navigator.clipboard.writeText(text)
          ElMessage.success('凭证已复制到剪贴板')
        })
      }

      dialogVisible.value = false
      emit('success')
    } catch (error) {
      ElMessage.error((isEdit.value ? '更新' : '创建') + '失败：' + error.message)
    } finally {
      submitting.value = false
    }
  })
}
</script>


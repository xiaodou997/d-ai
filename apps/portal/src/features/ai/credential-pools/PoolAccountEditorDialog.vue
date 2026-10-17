<script setup lang="ts">
import { reactive, ref, useId, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { aiAdminApi } from '@/api/aiAdmin'
import type { PoolCredentialDTO } from '@/api/types/ai'
import { DsModal, DsNumberInput } from '@/shared/ui'

const props = defineProps<{ open: boolean; poolId: string; account: PoolCredentialDTO | null }>()
const emit = defineEmits<{ close: []; saved: [poolId: string] }>()
const saving = ref(false)
const weightInputId = `pool-account-weight-${useId()}`
const form = reactive({ name: '', email: '', weight: 100, account_id: '', user_id: '', account_user_id: '', access_token: '', refresh_token: '', expires_at: '' })
watch(() => [props.open, props.account] as const, () => {
  if (!props.open || !props.account) { form.access_token = ''; form.refresh_token = ''; return }
  const account = props.account
  Object.assign(form, {
    name: account.name, email: account.email, weight: account.weight,
    account_id: String(account.auth_metadata?.account_id || account.auth_metadata?.chatgpt_account_id || account.auth_metadata?.accountId || ''),
    user_id: String(account.auth_metadata?.user_id || ''), account_user_id: String(account.auth_metadata?.account_user_id || ''),
    access_token: '', refresh_token: '', expires_at: ''
  })
}, { immediate: true })
function close() { if (!saving.value) emit('close') }
async function save() {
  if (!props.account || !form.name.trim()) { ElMessage.error('请填写账号名称'); return }
  const poolId = props.poolId
  const accountId = props.account.id
  saving.value = true
  try {
    const authMetadata = { ...(props.account.auth_metadata || {}), account_id: form.account_id.trim(), user_id: form.user_id.trim(), account_user_id: form.account_user_id.trim() }
    // Keep the account ID aliases consistent when an account is reassigned.
    if ('chatgpt_account_id' in authMetadata) authMetadata.chatgpt_account_id = form.account_id.trim()
    if ('accountId' in authMetadata) authMetadata.accountId = form.account_id.trim()
    await aiAdminApi.patchPoolCredential(poolId, accountId, {
      name: form.name.trim(), email: form.email.trim(), weight: form.weight, auth_metadata: authMetadata,
      ...(form.access_token.trim() ? { access_token: form.access_token.trim() } : {}),
      ...(form.refresh_token.trim() ? { refresh_token: form.refresh_token.trim() } : {}),
      ...(form.expires_at ? { expires_at: Math.floor(Number(form.expires_at) / 1000) } : {})
    })
    form.access_token = ''; form.refresh_token = ''
    ElMessage.success('账号已保存')
    emit('saved', poolId); emit('close')
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : '保存失败') }
  finally { saving.value = false }
}
</script>

<template>
  <DsModal :open="open" title="编辑池内账号" @close="close">
    <div class="account-editor-scroll">
      <el-form label-position="top" @submit.prevent="save">
        <el-form-item label="账号名称" required><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="邮箱"><el-input v-model="form.email" /></el-form-item>
        <el-form-item label="调度权重" :for="weightInputId"><DsNumberInput :id="weightInputId" v-model="form.weight" :min="0" :precision="0" /></el-form-item>
        <p class="editor-hint">加权模式下，权重为 0 的账号不接收新会话；已有会话继续保持绑定。</p>
        <el-form-item label="账号 / 工作区 ID"><el-input v-model="form.account_id" /></el-form-item>
        <el-form-item label="用户 ID"><el-input v-model="form.user_id" /></el-form-item>
        <el-form-item label="工作区用户 ID"><el-input v-model="form.account_user_id" /></el-form-item>
        <p class="editor-hint">更新凭据时填写新令牌，留空会保留原值。</p>
        <el-form-item label="Access Token"><el-input v-model="form.access_token" type="password" show-password autocomplete="new-password" placeholder="留空保留原值" /></el-form-item>
        <el-form-item label="Refresh Token"><el-input v-model="form.refresh_token" type="password" show-password autocomplete="new-password" placeholder="留空保留原值" /></el-form-item>
        <el-form-item label="新令牌到期时间"><el-date-picker v-model="form.expires_at" type="datetime" value-format="x" placeholder="可选" /></el-form-item>
      </el-form>
    </div>
    <template #footer>
      <el-button :disabled="saving" @click="close">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">保存账号</el-button>
    </template>
  </DsModal>
</template>

<style scoped>
.account-editor-scroll { max-height: min(64vh, 620px); overflow-y: auto; padding-right: 8px; }
.editor-hint { color: var(--ds-muted); font-size: 12px; line-height: 1.6; margin: 0 0 16px; }
</style>

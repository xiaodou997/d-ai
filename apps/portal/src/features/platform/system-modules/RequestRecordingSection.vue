<script setup lang="ts">
import { ref } from "vue";
import { ElMessage } from "element-plus";
import { recordsApi } from "@/features/ai/usage/recordsApi";
const tenant = ref("");
const key = ref("");
const model = ref("");
const hours = ref(1);
const busy = ref(false);
const expires = ref("");
async function enable() {
  busy.value = true;
  try {
    const session = await recordsApi.startDebug({ tenant_id: tenant.value.trim(), api_key_id: key.value.trim(), model: model.value.trim(), hours: hours.value });
    expires.value = new Date(session.expires_at).toLocaleString("zh-CN");
    ElMessage.success("限时调试记录已开启");
  } catch (error) { ElMessage.error(error instanceof Error ? error.message : "开启调试记录失败"); }
  finally { busy.value = false; }
}
</script>
<template>
  <section class="recording-settings">
    <h3>限时请求调试</h3>
    <p>默认只记录执行事实和用量，不保存完整正文。排障时选择租户、Key 或模型开启限时记录；多个条件同时填写时取交集。</p>
    <el-form label-position="top">
      <el-form-item label="租户 ID"><el-input v-model="tenant" placeholder="限定租户" /></el-form-item>
      <el-form-item label="API Key ID"><el-input v-model="key" placeholder="填写 Key 的 ID，无需密钥" /></el-form-item>
      <el-form-item label="模型编码"><el-input v-model="model" placeholder="限定模型" /></el-form-item>
      <el-form-item label="开启时长（小时）"><el-input-number v-model="hours" :min="1" :max="24" /></el-form-item>
      <el-button type="primary" :disabled="!tenant.trim() && !key.trim() && !model.trim()" :loading="busy" @click="enable">开启限时记录</el-button>
    </el-form>
    <p v-if="expires">本次记录将在 {{ expires }} 自动停止。</p>
    <small>内容脱敏后压缩保存，最多保留 7 天；超过 1 MiB 的正文会标明超限并省略，认证信息和内嵌媒体不保存。可在请求详情中查看调试内容。</small>
  </section>
</template>
<style scoped>
.recording-settings { display: grid; gap: 16px; }
.recording-settings p, .recording-settings small { color: var(--ds-muted); line-height: 1.6; margin: 0; }
.recording-settings h3 { margin: 0; }
</style>

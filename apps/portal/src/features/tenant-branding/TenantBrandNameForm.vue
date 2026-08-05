<script setup lang="ts">
const props = defineProps<{
  tenantName: string;
  customerSiteName: string;
  saving: boolean;
}>();

const emit = defineEmits<{
  "update:tenantName": [value: string];
  "update:customerSiteName": [value: string];
  save: [];
}>();
</script>

<template>
  <el-form label-position="top" class="tenant-brand-name-form" @submit.prevent="emit('save')">
    <el-form-item label="租户名称" required>
      <el-input
        :model-value="props.tenantName"
        maxlength="80"
        show-word-limit
        placeholder="请输入租户名称"
        @update:model-value="emit('update:tenantName', $event)"
      />
      <p class="tenant-brand-name-form__hint">用于平台内的租户识别、邀请信息和账户归属。</p>
    </el-form-item>

    <el-form-item label="用户门户网站名称">
      <el-input
        :model-value="props.customerSiteName"
        maxlength="80"
        show-word-limit
        placeholder="留空时向用户展示租户名称"
        @update:model-value="emit('update:customerSiteName', $event)"
      />
      <p class="tenant-brand-name-form__hint">用于终端用户门户、邀请注册页和浏览器标签页。</p>
    </el-form-item>

    <div class="tenant-brand-name-form__actions">
      <el-button native-type="submit" type="primary" :loading="props.saving">保存名称</el-button>
    </div>
  </el-form>
</template>

<style scoped>
.tenant-brand-name-form {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.tenant-brand-name-form__hint {
  margin: 6px 0 0;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.5;
}

.tenant-brand-name-form__actions {
  display: flex;
  justify-content: flex-end;
  padding-top: 4px;
}
</style>

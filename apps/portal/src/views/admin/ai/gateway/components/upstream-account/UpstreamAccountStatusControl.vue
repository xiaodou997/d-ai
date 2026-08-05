<script setup lang="ts">
import { CircleClose, Refresh } from '@element-plus/icons-vue'

import type { UpstreamAccountStatus } from './status'

interface Props {
  status: UpstreamAccountStatus
  invalidReason?: string
  loading?: boolean
}

defineProps<Props>()

const emit = defineEmits<{
  change: [status: 'active' | 'disabled']
  verify: []
}>()

function handleToggle(value: string | number | boolean) {
  emit('change', value === true ? 'active' : 'disabled')
}
</script>

<template>
  <div class="account-status-control" @click.stop>
    <el-switch
      v-if="status !== 'invalid'"
      class="account-status-switch"
      :model-value="status === 'active'"
      :loading="loading"
      inline-prompt
      active-text="启用"
      inactive-text="停用"
      :width="52"
      @change="handleToggle"
    />

    <template v-else>
      <el-tooltip :content="invalidReason || '上游拒绝了账号凭据，请重新验证。'" placement="top">
        <el-tag type="danger" size="small" effect="light">凭据失效</el-tag>
      </el-tooltip>
      <el-button size="small" text type="primary" :icon="Refresh" :disabled="loading" @click="emit('verify')">
        重新验证
      </el-button>
      <el-tooltip content="停用账号" placement="top">
        <el-button
          class="disable-account-button"
          size="small"
          text
          circle
          type="danger"
          :icon="CircleClose"
          :loading="loading"
          aria-label="停用账号"
          @click="emit('change', 'disabled')"
        />
      </el-tooltip>
    </template>
  </div>
</template>

<style scoped>
.account-status-control {
  min-height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 4px;
}

.account-status-switch {
  flex: 0 0 auto;
}

.disable-account-button {
  margin-left: 0;
}
</style>

<script setup>
import { computed } from 'vue'
import { Delete, Promotion } from '@element-plus/icons-vue'

const props = defineProps({
  modelValue: {
    type: String,
    default: ''
  },
  sending: {
    type: Boolean,
    default: false
  },
  canSend: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['update:modelValue', 'send', 'stop', 'clear'])

const draft = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value)
})

const handleKeydown = (event) => {
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    emit('send')
  }
}
</script>

<template>
  <footer class="composer">
    <el-input
      v-model="draft"
      type="textarea"
      :rows="3"
      resize="none"
      placeholder="输入消息，Enter 发送，Shift + Enter 换行"
      @keydown="handleKeydown"
    />
    <div class="composer-actions">
      <el-button :icon="Delete" @click="emit('clear')">清空</el-button>
      <el-button v-if="sending" type="warning" @click="emit('stop')">停止生成</el-button>
      <el-button v-else type="primary" :icon="Promotion" :disabled="!canSend" @click="emit('send')">发送</el-button>
    </div>
  </footer>
</template>

<style scoped>
.composer {
  flex-shrink: 0;
  padding: 16px;
  border-top: 1px solid #e5e7eb;
  background: #fff;
}

.composer-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 10px;
}
</style>

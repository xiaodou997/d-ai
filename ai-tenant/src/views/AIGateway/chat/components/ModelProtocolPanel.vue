<script setup>
import { computed, shallowRef } from 'vue'
import { ArrowDown, ArrowUp, ChatDotRound, Setting } from '@element-plus/icons-vue'

const props = defineProps({
  models: {
    type: Array,
    default: () => []
  },
  loadingModels: {
    type: Boolean,
    default: false
  },
  selectedModel: {
    type: String,
    default: ''
  },
  protocolPolicy: {
    type: String,
    default: 'auto'
  },
  selectedProtocol: {
    type: String,
    default: ''
  },
  temperature: {
    type: Number,
    default: 0.7
  },
  maxTokens: {
    type: Number,
    default: 2048
  },
  showAdvanced: {
    type: Boolean,
    default: false
  },
  selectedModelInfo: {
    type: Object,
    default: null
  },
  activeProtocolLabel: {
    type: String,
    default: '自动'
  },
  protocolLabels: {
    type: Object,
    required: true
  }
})

const emit = defineEmits([
  'update:selectedModel',
  'update:protocolPolicy',
  'update:selectedProtocol',
  'update:temperature',
  'update:maxTokens',
  'update:showAdvanced'
])

const protocolOptions = computed(() =>
  (props.selectedModelInfo?.available_protocols || []).map((protocol) => ({
    label: props.protocolLabels[protocol] || protocol,
    value: protocol
  }))
)

const panelOpen = shallowRef(true)
</script>

<template>
  <section class="model-panel">
    <header class="panel-head">
      <div class="panel-title">
        <span>模型与协议</span>
        <strong>{{ selectedModel || '未选择模型' }}</strong>
        <small>{{ activeProtocolLabel }} · {{ protocolPolicy === 'auto' ? '自动' : '指定' }}</small>
      </div>
      <el-button link type="primary" :icon="panelOpen ? ArrowUp : ArrowDown" @click="panelOpen = !panelOpen">
        {{ panelOpen ? '收起' : '展开' }}
      </el-button>
    </header>

    <div v-show="panelOpen" class="panel-body">
      <label class="field-label">模型</label>
      <el-select
        class="w-full"
        filterable
        :loading="loadingModels"
        :model-value="selectedModel"
        placeholder="选择模型"
        @update:model-value="emit('update:selectedModel', $event)"
      >
        <el-option v-for="model in models" :key="model.model_code" :label="model.model_code" :value="model.model_code">
          <div class="model-option">
            <span>{{ model.model_code }}</span>
            <small>Auto: {{ protocolLabels[model.default_protocol] || model.default_protocol }}</small>
          </div>
        </el-option>
      </el-select>

      <div class="protocol-card">
        <span>当前协议</span>
        <strong>{{ activeProtocolLabel }}</strong>
        <small>{{ protocolPolicy === 'auto' ? '后台自动选择' : '高级模式指定' }}</small>
      </div>

      <button class="advanced-toggle" type="button" @click="emit('update:showAdvanced', !showAdvanced)">
        <el-icon><Setting /></el-icon>
        <span>高级设置</span>
      </button>

      <div v-show="showAdvanced" class="advanced-panel">
        <label class="field-label">协议策略</label>
        <el-segmented
          class="w-full"
          :model-value="protocolPolicy"
          :options="[{ label: '自动', value: 'auto' }, { label: '指定', value: 'manual' }]"
          @update:model-value="emit('update:protocolPolicy', $event)"
        />
        <template v-if="protocolPolicy === 'manual'">
          <label class="field-label mt-3">指定协议</label>
          <el-select
            class="w-full"
            :model-value="selectedProtocol"
            @update:model-value="emit('update:selectedProtocol', $event)"
          >
            <el-option v-for="item in protocolOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </template>
        <label class="field-label mt-3">Temperature</label>
        <el-slider :model-value="temperature" :min="0" :max="2" :step="0.1" @update:model-value="emit('update:temperature', $event)" />
        <label class="field-label">Max tokens</label>
        <el-input-number
          class="w-full"
          :model-value="maxTokens"
          :min="256"
          :max="32768"
          :step="256"
          @update:model-value="emit('update:maxTokens', $event)"
        />
      </div>

      <div class="source-note">
        <el-icon><ChatDotRound /></el-icon>
        <span>租户网页对话</span>
      </div>
    </div>
  </section>
</template>

<style scoped>
.model-panel {
  flex-shrink: 0;
  padding: 16px;
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 12px;
}

.panel-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.panel-title {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.panel-title span {
  color: #475569;
  font-size: 12px;
  font-weight: 900;
}

.panel-title strong {
  overflow: hidden;
  color: #0f172a;
  font-size: 15px;
  font-weight: 900;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.panel-title small {
  color: #64748b;
  font-size: 11px;
  font-weight: 800;
}

.panel-body {
  margin-top: 14px;
}

.field-label {
  display: block;
  margin: 0 0 8px;
  color: #475569;
  font-size: 12px;
  font-weight: 800;
}

.mt-3 {
  margin-top: 12px;
}

.model-option {
  display: flex;
  justify-content: space-between;
  gap: 18px;
}

.model-option small {
  color: #64748b;
}

.protocol-card {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-top: 14px;
  padding: 12px;
  border: 1px solid #dbeafe;
  border-radius: 10px;
  background: #eff6ff;
}

.protocol-card span,
.protocol-card small {
  color: #64748b;
  font-size: 11px;
  font-weight: 800;
}

.protocol-card strong {
  color: #1d4ed8;
  font-size: 14px;
  font-weight: 900;
}

.advanced-toggle {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 16px 0 10px;
  padding: 10px 0;
  color: #334155;
  font-size: 13px;
  font-weight: 800;
  background: transparent;
  border: 0;
  cursor: pointer;
}

.advanced-panel {
  padding: 12px;
  border: 1px solid #eef2f7;
  border-radius: 10px;
  background: #f8fafc;
}

.source-note {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 16px;
  padding: 10px 12px;
  color: #0369a1;
  font-size: 12px;
  font-weight: 700;
  border-radius: 10px;
  background: #e0f2fe;
}
</style>

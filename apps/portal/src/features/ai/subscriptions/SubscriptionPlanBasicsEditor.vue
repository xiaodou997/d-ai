<script setup lang="ts">
import { computed, shallowRef } from "vue";

const props = defineProps<{
  supports7d: boolean;
  soldCount: number;
  reservedCount: number;
  priceHint: string;
  totalHint: string;
  window5hHint: string;
  window7dHint: string;
}>();

const name = defineModel<string>("name", { required: true });
const description = defineModel<string>("description", { required: true });
const durationDays = defineModel<number>("durationDays", { required: true });
const priceCredits = defineModel<number>("priceCredits", { required: true });
const saleLimit = defineModel<number | null>("saleLimit", { required: true });
const totalLimitCredits = defineModel<number>("totalLimitCredits", { required: true });
const window5hLimitCredits = defineModel<number | null>("window5hLimitCredits", { required: true });
const window7dLimitCredits = defineModel<number | null>("window7dLimitCredits", { required: true });

const expandedQuotaSettings = shallowRef<string[]>([]);
const committedCount = computed(() => props.soldCount + props.reservedCount);
const minimumSaleLimit = computed(() => Math.max(1, committedCount.value));
const saleMode = computed({
  get: () => (saleLimit.value == null ? "unlimited" : "limited"),
  set: (mode: string) => {
    saleLimit.value = mode === "limited"
      ? Math.max(saleLimit.value ?? 100, minimumSaleLimit.value)
      : null;
  }
});
</script>

<template>
  <div class="basics-editor">
    <section class="form-section">
      <div class="section-heading">
        <strong>对外展示</strong>
        <span>名称和说明会显示在用户套餐商城中</span>
      </div>
      <el-form-item label="套餐名称" prop="name">
        <el-input v-model="name" maxlength="60" placeholder="例如：AI 入门周卡" />
      </el-form-item>
      <el-form-item label="套餐说明">
        <el-input
          v-model="description"
          type="textarea"
          :rows="3"
          maxlength="200"
          show-word-limit
          placeholder="简要说明套餐适合的人群和使用场景"
        />
      </el-form-item>
    </section>

    <section class="form-section">
      <div class="section-heading">
        <strong>价格与销售</strong>
        <span>限量套餐售完后会自动显示售罄，无需手动下架</span>
      </div>
      <div class="field-grid">
        <el-form-item label="售价（积分）" prop="price_credits">
          <el-input-number
            v-model="priceCredits"
            :min="1"
            :step="1"
            :precision="0"
            :controls="false"
          />
          <small>{{ priceHint }}</small>
        </el-form-item>
        <el-form-item label="有效期">
          <el-select v-model="durationDays">
            <el-option :value="1" label="1 天" />
            <el-option :value="3" label="3 天" />
            <el-option :value="7" label="7 天" />
            <el-option :value="30" label="30 天" />
          </el-select>
        </el-form-item>
      </div>

      <el-form-item label="销售数量">
        <div class="sale-limit-control">
          <el-radio-group v-model="saleMode">
            <el-radio-button value="unlimited">不限量</el-radio-button>
            <el-radio-button value="limited">限量销售</el-radio-button>
          </el-radio-group>
          <div v-if="saleMode === 'limited'" class="quantity-input">
            <el-input-number
              v-model="saleLimit"
              :min="minimumSaleLimit"
              :max="2147483647"
              :precision="0"
              :step="10"
            />
            <span>份</span>
          </div>
        </div>
        <small v-if="committedCount > 0">
          已售 {{ soldCount }} 份，待支付 {{ reservedCount }} 份；销售数量不能低于 {{ committedCount }} 份
        </small>
      </el-form-item>
    </section>

    <section class="form-section">
      <div class="section-heading">
        <strong>套餐额度</strong>
        <span>用户在有效期内最多可使用的总积分额度</span>
      </div>
      <el-form-item label="总额度（积分）" prop="total_limit_credits">
        <el-input-number
          v-model="totalLimitCredits"
          :min="0.0001"
          :step="100"
          :precision="4"
          :controls="false"
        />
        <small>{{ totalHint }}</small>
      </el-form-item>

      <el-collapse v-model="expandedQuotaSettings" class="advanced-collapse">
        <el-collapse-item name="window-quotas" title="高级额度控制（可选）">
          <div class="field-grid">
            <el-form-item label="任意连续 5 小时内的额度">
              <el-input-number
                v-model="window5hLimitCredits"
                :min="1"
                :step="1"
                :precision="0"
                :controls="false"
                clearable
              />
              <small>{{ window5hHint }}</small>
            </el-form-item>
            <el-form-item label="任意连续 7 天内的额度">
              <el-input-number
                v-model="window7dLimitCredits"
                :disabled="!supports7d"
                :min="1"
                :step="1"
                :precision="0"
                :controls="false"
                clearable
              />
              <small>{{ supports7d ? window7dHint : "当前有效期不足 7 天，无法设置" }}</small>
            </el-form-item>
          </div>
        </el-collapse-item>
      </el-collapse>
    </section>
  </div>
</template>

<style scoped>
.basics-editor,
.form-section {
  display: flex;
  flex-direction: column;
}

.basics-editor {
  gap: 26px;
}

.form-section {
  gap: 14px;
}

.section-heading {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--ds-line);
}

.section-heading strong {
  color: var(--ds-ink);
  font-size: 15px;
}

.section-heading span,
.basics-editor small {
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.5;
}

.field-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.basics-editor :deep(.el-form-item) {
  margin-bottom: 0;
}

.basics-editor :deep(.el-input-number),
.basics-editor :deep(.el-select) {
  width: 100%;
}

.sale-limit-control,
.quantity-input {
  display: flex;
  align-items: center;
  gap: 12px;
}

.quantity-input {
  flex: 1;
  max-width: 260px;
}

.quantity-input span {
  flex: 0 0 auto;
  color: var(--ds-muted);
  font-size: 13px;
}

.advanced-collapse {
  border-top: 0;
}

.advanced-collapse :deep(.el-collapse-item__header) {
  color: var(--ds-ink-soft);
  font-weight: 600;
}

@media (max-width: 720px) {
  .field-grid {
    grid-template-columns: 1fr;
  }

  .sale-limit-control {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>

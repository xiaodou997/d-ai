<!--
  用户充值规则(租户端支付设置) — 配置终端用户充值的快捷套餐与自定义金额到账规则。
  重构:PortalPageHeader + PortalDataCard → PortalPagePanel 一体面板(图标徽章+面包屑标题+描述同行,
       fill 链撑满短页),表单内容置于同卡 body 内 24px 容器;表单仍为 element-plus,
       业务逻辑与请求参数完全不变。
-->
<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { ElMessage } from "element-plus";
import { Wallet } from "lucide-vue-next";
import { PortalPagePanel } from "@dai/app-core";
import { tenantApi } from "../../api/tenant";
import type { TenantPaymentSettings, TopupPackage } from "../../types/tenant";

interface PackageForm {
  id: string;
  name: string;
  amountYuan: number;
  credits: number;
  badge: string;
  enabled: boolean;
  sortOrder: number;
}

const loading = ref(false);
const submitting = ref(false);
const form = reactive({
  userCreditsPerCny: 100,
  feePercent: 1.6,
  packages: [] as PackageForm[]
});

const customPreview = computed(() => {
  const gross = 100 * form.userCreditsPerCny;
  const fee = Math.ceil((gross * percentToBp(form.feePercent)) / 10000);
  return { gross, fee, net: Math.max(0, gross - fee) };
});

function percentToBp(value: number) {
  return Math.round(Number(value || 0) * 100);
}

function bpToPercent(value: number) {
  return Number(((value || 0) / 100).toFixed(2));
}

function packageToForm(item: TopupPackage): PackageForm {
  return {
    id: item.id,
    name: item.name,
    amountYuan: Number((item.amount / 100).toFixed(2)),
    credits: item.credits,
    badge: item.badge || "",
    enabled: item.enabled,
    sortOrder: item.sortOrder
  };
}

function formToPackage(item: PackageForm): TopupPackage {
  return {
    id: item.id.trim() || `p${Date.now()}`,
    name: item.name.trim() || `${item.amountYuan} 元充值包`,
    amount: Math.round(Number(item.amountYuan || 0) * 100),
    credits: Math.round(Number(item.credits || 0)),
    badge: item.badge.trim() || undefined,
    enabled: item.enabled,
    sortOrder: Number(item.sortOrder || 0)
  };
}

function addPackage() {
  const next = form.packages.length + 1;
  form.packages.push({
    id: `p${Date.now()}`,
    name: `${next * 10} 元充值包`,
    amountYuan: next * 10,
    credits: next * 1000,
    badge: "",
    enabled: true,
    sortOrder: next * 10
  });
}

function removePackage(index: number) {
  form.packages.splice(index, 1);
}

async function fetchSettings() {
  loading.value = true;
  try {
    const settings = await tenantApi.getPaymentSettings();
    form.userCreditsPerCny = settings.userCreditsPerCny;
    form.feePercent = bpToPercent(settings.userCustomTopupFeeBp);
    form.packages = (settings.userTopupPackages || []).map(packageToForm);
  } catch (e) {
    console.error("获取用户充值设置失败:", e);
  } finally {
    loading.value = false;
  }
}

async function submit() {
  submitting.value = true;
  try {
    const body: TenantPaymentSettings = {
      userCreditsPerCny: Math.round(Number(form.userCreditsPerCny || 0)),
      userCustomTopupFeeBp: percentToBp(form.feePercent),
      userTopupPackages: form.packages.map(formToPackage)
    };
    await tenantApi.updatePaymentSettings(body);
    ElMessage.success("用户充值设置已保存");
    void fetchSettings();
  } catch (err) {
    const e = err as { detail?: string; message?: string };
    ElMessage.error(e?.detail || e?.message || "保存失败");
  } finally {
    submitting.value = false;
  }
}

onMounted(fetchSettings);
</script>

<template>
  <div class="page-container payment-settings-page">
    <PortalPagePanel
      :icon="Wallet"
      :breadcrumbs="[{ label: '租户运营' }, { label: '用户运营' }, { label: '用户充值规则' }]"
      description="配置终端用户充值时看到的快捷套餐和自定义金额到账规则。"
      fill
    >
      <div class="settings-card-body">
        <el-form v-loading="loading" :model="form" label-position="top" class="settings-form">
          <section class="settings-section">
            <div class="section-heading">
              <h4>兑换与手续费</h4>
              <p>用户自己输入任意金额时按这里计算到账积分，金额固定限制为 10~10000 元。</p>
            </div>
            <div class="form-grid">
              <el-form-item label="1 元到账多少积分">
                <el-input-number v-model="form.userCreditsPerCny" :min="1" :controls="false" class="w-full" />
              </el-form-item>
              <el-form-item label="微信手续费">
                <el-input-number v-model="form.feePercent" :min="0" :max="100" :precision="2" :step="0.1" :controls="false" class="w-full">
                  <template #suffix>%</template>
                </el-input-number>
              </el-form-item>
            </div>
            <div class="preview-strip">
              <span class="preview-strip__tag">充值举例</span>
              用户自定义充值 ¥100.00，原本可得 {{ customPreview.gross }} 积分，扣除手续费 {{ customPreview.fee }} 积分，实际到账 {{ customPreview.net }} 积分。
            </div>
          </section>

          <section class="settings-section">
            <div class="section-heading">
              <h4>快捷充值套餐</h4>
              <p>套餐到账积分固定，不再扣手续费，适合做赠送积分活动。卡片上半部分即用户看到的样子。</p>
            </div>

            <div class="package-grid">
              <article
                v-for="(pkg, index) in form.packages"
                :key="pkg.id"
                :class="['package-card', { 'package-card--disabled': !pkg.enabled }]"
              >
                <div class="package-preview">
                  <span v-if="pkg.badge" class="package-preview__badge">{{ pkg.badge }}</span>
                  <span v-if="!pkg.enabled" class="package-preview__hidden">已隐藏</span>
                  <strong>¥{{ Number(pkg.amountYuan || 0).toFixed(2) }}</strong>
                  <em>{{ pkg.name || "未命名套餐" }}</em>
                  <span class="package-preview__credits">到账 {{ Number(pkg.credits || 0).toLocaleString() }} 积分</span>
                </div>
                <div class="package-fields">
                  <label class="package-field">
                    <span>名称</span>
                    <el-input v-model="pkg.name" placeholder="10 元体验包" />
                  </label>
                  <label class="package-field">
                    <span>金额（元）</span>
                    <el-input-number v-model="pkg.amountYuan" :min="10" :max="10000" :precision="2" :controls="false" class="w-full" />
                  </label>
                  <label class="package-field">
                    <span>到账积分</span>
                    <el-input-number v-model="pkg.credits" :min="1" :controls="false" class="w-full" />
                  </label>
                  <div class="package-field-row">
                    <label class="package-field">
                      <span>角标</span>
                      <el-input v-model="pkg.badge" placeholder="送 20%" />
                    </label>
                    <label class="package-field package-field--narrow">
                      <span>排序</span>
                      <el-input-number v-model="pkg.sortOrder" :controls="false" class="w-full" />
                    </label>
                  </div>
                </div>
                <div class="package-card__ops">
                  <el-switch v-model="pkg.enabled" active-text="显示" inactive-text="隐藏" />
                  <el-button type="danger" link @click="removePackage(index)">删除</el-button>
                </div>
              </article>

              <button type="button" class="package-add" @click="addPackage">
                <span class="package-add__plus">＋</span>
                添加套餐
              </button>
            </div>
          </section>

          <div class="settings-actions">
            <el-button type="primary" :loading="submitting" @click="submit">保存用户充值设置</el-button>
          </div>
        </el-form>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
/* fill 链:页面根 flex:1 + 面板 fill,短内容(无套餐)时面板也撑满一屏 */
.payment-settings-page {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 面板 body 无内边距,表单内容用 24px 容器承载 */
.settings-card-body {
  flex: 1;
  min-height: 0;
  padding: 24px;
}

.settings-form {
  display: flex;
  flex-direction: column;
  gap: 22px;
}

.settings-section + .settings-section,
.settings-form > .settings-actions {
  border-top: 1px solid var(--ds-line);
  padding-top: 20px;
}

.section-heading {
  margin-bottom: 14px;
}

.section-heading h4 {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0;
  color: var(--ds-ink);
  font-size: 14px;
  font-weight: 650;
}

.section-heading h4::before {
  width: 3px;
  height: 14px;
  border-radius: var(--ds-radius-pill);
  background: var(--ds-accent);
  content: "";
}

.section-heading p {
  margin: 4px 0 0 11px;
  color: var(--ds-muted);
  font-size: 12px;
  line-height: 1.6;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.preview-strip {
  margin-top: 14px;
  border-radius: var(--ds-radius-control);
  border-left: 3px solid var(--ds-accent);
  padding: 10px 14px;
  background: var(--ds-panel-muted);
  color: var(--ds-ink-soft);
  font-size: 13px;
  line-height: 1.7;
}

.preview-strip__tag {
  display: block;
  color: var(--ds-accent);
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.08em;
}

.package-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 14px;
}

.package-card {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  background: var(--ds-panel);
  box-shadow: var(--ds-shadow-sm);
}

.package-preview {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 4px;
  align-items: flex-start;
  padding: 16px 16px 14px;
  background: linear-gradient(180deg, var(--ds-accent-soft), var(--ds-panel));
  border-bottom: 1px solid var(--ds-line);
}

.package-card--disabled .package-preview {
  background: var(--ds-panel-muted);
}

.package-card--disabled .package-preview > strong,
.package-card--disabled .package-preview > em,
.package-card--disabled .package-preview__credits,
.package-card--disabled .package-preview__badge {
  opacity: 0.45;
}

.package-preview strong {
  color: var(--ds-ink);
  font-size: 24px;
  line-height: 1.2;
}

.package-preview em {
  color: var(--ds-muted);
  font-size: 13px;
  font-style: normal;
}

.package-preview__credits {
  color: var(--ds-accent);
  font-size: 12px;
  font-weight: 700;
}

.package-preview__badge {
  position: absolute;
  top: 12px;
  right: 12px;
  border-radius: var(--ds-radius-pill);
  padding: 3px 8px;
  background: var(--ds-warning-soft);
  color: var(--ds-warning);
  font-size: 12px;
  font-weight: 700;
}

.package-preview__hidden {
  border-radius: var(--ds-radius-pill);
  padding: 3px 8px;
  border: 1px solid var(--ds-line);
  background: var(--ds-panel);
  color: var(--ds-muted);
  font-size: 11px;
  font-weight: 700;
}

.package-fields {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px 16px;
}

.package-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
  flex: 1;
}

.package-field > span {
  color: var(--ds-muted);
  font-size: 12px;
  font-weight: 700;
}

.package-field--narrow {
  flex: 0 0 76px;
}

.package-field-row {
  display: flex;
  gap: 10px;
}

.package-card__ops {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: auto;
  padding: 10px 16px;
  border-top: 1px solid var(--ds-line);
  background: var(--ds-panel-muted);
}

.package-add {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 200px;
  border: 1px dashed var(--ds-line-strong);
  border-radius: var(--ds-radius-panel);
  background: transparent;
  color: var(--ds-muted);
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;
  transition: border-color 0.15s ease, color 0.15s ease, background-color 0.15s ease;
}

.package-add:hover {
  border-color: var(--ds-accent);
  background: var(--ds-accent-soft);
  color: var(--ds-accent);
}

.package-add__plus {
  font-size: 22px;
  line-height: 1;
}

.settings-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
}

.w-full {
  width: 100%;
}

:deep(.el-form-item) {
  margin-bottom: 0;
}

:deep(.el-form-item__label) {
  margin-bottom: 6px;
  color: var(--ds-ink-soft);
  font-size: 12px;
  font-weight: 700;
}

@media (max-width: 1000px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>

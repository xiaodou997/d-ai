<!--
  支付配置 — 微信收款 / 租户充值与提现 两个分区通过 DsTabs 切换。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行）,
       两大配置块改为同卡 Tab 切换(与租户详情页同模式);颜色全部使用 var(--ds-*) token;
       表单仍为 element-plus，业务逻辑与请求参数完全不变。
-->
<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { ElMessage } from "element-plus";
import { CreditCard } from "lucide-vue-next";
import { PortalPagePanel } from "@/platform";
import { DsTabs } from "@/shared/ui";
import { platformAdminApi } from "@/api/platformAdmin";
import type { PaymentGlobalSettings, TopupPackage, WechatConfig } from "@/api/types/admin";

const activeTab = ref("wechat");
const tabs = [
  { key: "wechat", label: "微信收款" },
  { key: "rules", label: "租户充值与提现" }
];

interface PackageForm {
  id: string;
  name: string;
  amountYuan: number;
  credits: number;
  badge: string;
  enabled: boolean;
  sortOrder: number;
}

const wechatLoading = ref(false);
const wechatSubmitting = ref(false);
const wechatConfig = ref<WechatConfig | null>(null);
const wechatForm = reactive({
  enabled: false,
  mock: true,
  verifyMode: "platform_cert" as "platform_cert" | "public_key",
  appId: "",
  mchId: "",
  mchCertSerialNo: "",
  notifyBaseUrl: "",
  orderTtlSeconds: 7200,
  mchPrivateKey: "",
  apiv3Key: "",
  wechatPayPublicKeyId: "",
  wechatPayPublicKey: ""
});

const settingsLoading = ref(false);
const settingsSubmitting = ref(false);
const ruleForm = reactive({
  creditsPerCny: 100,
  topupFeePercent: 1.6,
  withdrawFeePercent: 1.6,
  packages: [] as PackageForm[]
});

const customPreview = computed(() => {
  const amount = 100;
  const gross = amount * ruleForm.creditsPerCny;
  const fee = Math.ceil((gross * percentToBp(ruleForm.topupFeePercent)) / 10000);
  return { amount, gross, fee, net: Math.max(0, gross - fee) };
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
  const next = ruleForm.packages.length + 1;
  ruleForm.packages.push({
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
  ruleForm.packages.splice(index, 1);
}

async function fetchWechatConfig() {
  wechatLoading.value = true;
  try {
    const cfg = await platformAdminApi.getWechatConfig();
    wechatConfig.value = cfg;
    wechatForm.enabled = cfg.enabled;
    wechatForm.mock = cfg.mock;
    wechatForm.verifyMode = cfg.verifyMode || "platform_cert";
    wechatForm.appId = cfg.appId;
    wechatForm.mchId = cfg.mchId;
    wechatForm.mchCertSerialNo = cfg.mchCertSerialNo;
    wechatForm.notifyBaseUrl = cfg.notifyBaseUrl;
    wechatForm.orderTtlSeconds = cfg.orderTtlSeconds;
    wechatForm.mchPrivateKey = "";
    wechatForm.apiv3Key = "";
    wechatForm.wechatPayPublicKeyId = cfg.wechatPayPublicKeyId;
    wechatForm.wechatPayPublicKey = "";
  } catch (e) {
    console.error("获取微信商户配置失败:", e);
  } finally {
    wechatLoading.value = false;
  }
}

async function submitWechatConfig() {
  wechatSubmitting.value = true;
  try {
    await platformAdminApi.updateWechatConfig({
      enabled: wechatForm.enabled,
      mock: wechatForm.mock,
      verifyMode: wechatForm.verifyMode,
      appId: wechatForm.appId,
      mchId: wechatForm.mchId,
      mchCertSerialNo: wechatForm.mchCertSerialNo,
      notifyBaseUrl: wechatForm.notifyBaseUrl,
      orderTtlSeconds: wechatForm.orderTtlSeconds,
      mchPrivateKey: wechatForm.mchPrivateKey || null,
      apiv3Key: wechatForm.apiv3Key || null,
      wechatPayPublicKeyId: wechatForm.wechatPayPublicKeyId || null,
      wechatPayPublicKey: wechatForm.wechatPayPublicKey || null
    });
    ElMessage.success("微信收款配置已保存");
    void fetchWechatConfig();
  } catch (err) {
    const e = err as { detail?: string; message?: string };
    ElMessage.error(e?.detail || e?.message || "保存失败");
  } finally {
    wechatSubmitting.value = false;
  }
}

async function fetchPaymentSettings() {
  settingsLoading.value = true;
  try {
    const settings = await platformAdminApi.getPaymentSettings();
    ruleForm.creditsPerCny = settings.creditsPerCny;
    ruleForm.topupFeePercent = bpToPercent(settings.tenantCustomTopupFeeBp);
    ruleForm.withdrawFeePercent = bpToPercent(settings.tenantWithdrawFeeBp);
    ruleForm.packages = (settings.tenantTopupPackages || []).map(packageToForm);
  } catch (e) {
    console.error("获取支付设置失败:", e);
  } finally {
    settingsLoading.value = false;
  }
}

async function submitPaymentSettings() {
  settingsSubmitting.value = true;
  try {
    const body: PaymentGlobalSettings = {
      creditsPerCny: Math.round(Number(ruleForm.creditsPerCny || 0)),
      tenantCustomTopupFeeBp: percentToBp(ruleForm.topupFeePercent),
      tenantWithdrawFeeBp: percentToBp(ruleForm.withdrawFeePercent),
      tenantTopupPackages: ruleForm.packages.map(formToPackage)
    };
    await platformAdminApi.updatePaymentSettings(body);
    ElMessage.success("平台充值与提现规则已保存");
    void fetchPaymentSettings();
  } catch (err) {
    const e = err as { detail?: string; message?: string };
    ElMessage.error(e?.detail || e?.message || "保存失败");
  } finally {
    settingsSubmitting.value = false;
  }
}

onMounted(() => {
  void fetchWechatConfig();
  void fetchPaymentSettings();
});
</script>

<template>
  <div class="payment-settings-page">
    <PortalPagePanel
      :icon="CreditCard"
      :breadcrumbs="[{ label: '用户中心' }, { label: '财务中心' }, { label: '支付配置' }]"
      description="用业务语言配置收款、充值到账和提现到账。这里的设置主要给租户充值使用，租户可单独配置终端用户充值。"
    >
      <div class="settings-body">
        <div class="settings-tabs">
          <DsTabs v-model="activeTab" :tabs="tabs" />
        </div>

        <!-- 微信收款 -->
        <div v-show="activeTab === 'wechat'" class="settings-pane">
          <header class="settings-pane__head">
            <p class="settings-pane__desc">控制微信支付是否开放，以及真实商户号、回调地址和验签方式。</p>
            <div class="status-pills">
            <span :class="['status-pill', wechatForm.enabled ? 'status-pill--positive' : '']">
              {{ wechatForm.enabled ? "收款已开启" : "收款已关闭" }}
            </span>
            <span :class="['status-pill', wechatForm.mock ? 'status-pill--warning' : 'status-pill--positive']">
              {{ wechatForm.mock ? "测试模式" : "真实收款" }}
            </span>
            <span class="status-pill">
              {{ wechatForm.verifyMode === "public_key" ? "公钥验签" : "平台证书验签" }}
            </span>
            </div>
          </header>
        <el-form v-loading="wechatLoading" :model="wechatForm" label-position="top" class="settings-form">
          <section class="settings-section">
            <div class="section-heading">
              <h4>开关</h4>
              <p>关闭后充值入口不可用。测试模式用于内部联调，不会发起真实微信收款。</p>
            </div>
            <div class="form-grid">
              <el-form-item label="允许在线充值">
                <el-switch v-model="wechatForm.enabled" />
              </el-form-item>
              <el-form-item label="测试模式">
                <el-switch v-model="wechatForm.mock" />
              </el-form-item>
            </div>
          </section>

          <section class="settings-section">
            <div class="section-heading">
              <h4>商户信息</h4>
              <p>真实收款时需要填写微信支付商户后台里的参数。</p>
            </div>
            <div class="form-grid">
              <el-form-item label="验签方式">
                <el-select v-model="wechatForm.verifyMode" class="w-full">
                  <el-option label="微信支付公钥（推荐）" value="public_key" />
                  <el-option label="平台证书下载" value="platform_cert" />
                </el-select>
              </el-form-item>
              <el-form-item label="订单保留时间（秒）">
                <el-input-number v-model="wechatForm.orderTtlSeconds" :min="300" :max="86400" :step="60" :controls="false" class="w-full" />
              </el-form-item>
              <el-form-item label="AppID">
                <el-input v-model="wechatForm.appId" placeholder="微信支付 AppID" />
              </el-form-item>
              <el-form-item label="商户号">
                <el-input v-model="wechatForm.mchId" placeholder="微信支付商户号" />
              </el-form-item>
              <el-form-item label="商户证书序列号">
                <el-input v-model="wechatForm.mchCertSerialNo" placeholder="apiclient_cert 序列号" />
              </el-form-item>
              <el-form-item label="回调地址根路径">
                <el-input v-model="wechatForm.notifyBaseUrl" placeholder="https://your-domain/platform" />
              </el-form-item>
            </div>
          </section>

          <section class="settings-section">
            <div class="section-heading">
              <h4>密钥</h4>
              <p>留空表示不修改已保存的密钥。</p>
            </div>
            <div v-if="wechatForm.verifyMode === 'public_key'" class="form-grid">
              <el-form-item label="微信支付公钥 ID">
                <el-input v-model="wechatForm.wechatPayPublicKeyId" placeholder="PUB_KEY_ID_..." />
              </el-form-item>
              <el-form-item :label="`微信支付公钥（${wechatConfig?.hasWechatPayPublicKey ? '已配置' : '未配置'}）`">
                <el-input v-model="wechatForm.wechatPayPublicKey" type="textarea" :rows="4" placeholder="留空=不修改；粘贴微信支付公钥" />
              </el-form-item>
            </div>
            <div class="form-grid">
              <el-form-item :label="`商户私钥（${wechatConfig?.hasPrivateKey ? '已配置' : '未配置'}）`">
                <el-input v-model="wechatForm.mchPrivateKey" type="textarea" :rows="4" placeholder="留空=不修改；粘贴 apiclient_key.pem" />
              </el-form-item>
              <el-form-item :label="`APIv3Key（${wechatConfig?.hasApiv3Key ? '已配置' : '未配置'}）`">
                <el-input v-model="wechatForm.apiv3Key" type="password" show-password placeholder="留空=不修改" />
              </el-form-item>
            </div>
          </section>

          <div class="settings-actions">
            <el-button type="primary" :loading="wechatSubmitting" @click="submitWechatConfig">保存微信收款</el-button>
          </div>
        </el-form>
        </div>

        <!-- 租户充值与提现 -->
        <div v-show="activeTab === 'rules'" class="settings-pane">
          <header class="settings-pane__head">
            <p class="settings-pane__desc">快捷套餐固定到账；自定义充值和提现按手续费折算。租户可单独配置终端用户充值。</p>
          </header>
        <el-form v-loading="settingsLoading" :model="ruleForm" label-position="top" class="settings-form">
          <section class="settings-section">
            <div class="section-heading">
              <h4>兑换与手续费</h4>
              <p>自定义充值金额固定限制为 10~10000 元；提现时申请金额冻结，实际打款 = 申请金额 - 手续费。</p>
            </div>
            <div class="form-grid form-grid--3">
              <el-form-item label="1 元到账多少积分">
                <el-input-number v-model="ruleForm.creditsPerCny" :min="1" :controls="false" class="w-full" />
              </el-form-item>
              <el-form-item label="充值手续费">
                <el-input-number v-model="ruleForm.topupFeePercent" :min="0" :max="100" :precision="2" :step="0.1" :controls="false" class="w-full">
                  <template #suffix>%</template>
                </el-input-number>
              </el-form-item>
              <el-form-item label="提现手续费">
                <el-input-number v-model="ruleForm.withdrawFeePercent" :min="0" :max="100" :precision="2" :step="0.1" :controls="false" class="w-full">
                  <template #suffix>%</template>
                </el-input-number>
              </el-form-item>
            </div>
            <div class="preview-row">
              <div class="preview-strip">
                <span class="preview-strip__tag">充值举例</span>
                自定义充值 ¥{{ customPreview.amount }}，原本可得 {{ customPreview.gross }} 积分，扣除手续费 {{ customPreview.fee }} 积分，实际到账 {{ customPreview.net }} 积分。
              </div>
              <div class="preview-strip">
                <span class="preview-strip__tag">提现举例</span>
                申请提现 ¥100.00，手续费 {{ ruleForm.withdrawFeePercent }}%，预计实际打款 ¥{{ (100 - (100 * ruleForm.withdrawFeePercent) / 100).toFixed(2) }}。
              </div>
            </div>
          </section>

          <section class="settings-section">
            <div class="section-heading">
              <h4>快捷充值套餐</h4>
              <p>套餐适合做活动，比如“10 元到账 1200 积分”。套餐到账积分固定，不再扣手续费。卡片上半部分即租户看到的样子。</p>
            </div>

            <div class="package-grid">
              <article
                v-for="(pkg, index) in ruleForm.packages"
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
            <el-button type="primary" :loading="settingsSubmitting" @click="submitPaymentSettings">保存充值与提现规则</el-button>
          </div>
        </el-form>
        </div>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.payment-settings-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 面板 body 无内边距,用 24px 容器承载 Tab 与表单(与租户详情页同模式) */
.settings-body {
  padding: 24px;
}

.settings-tabs {
  padding-bottom: 6px;
}

.settings-pane {
  padding-top: 16px;
}

.settings-pane__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 10px 16px;
  margin-bottom: 20px;
}

.settings-pane__desc {
  margin: 0;
  color: var(--ds-faint);
  font-size: 12.5px;
}

.status-pills {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border-radius: var(--ds-radius-pill);
  padding: 5px 10px;
  border: 1px solid var(--ds-line);
  background: var(--ds-panel-muted);
  color: var(--ds-muted);
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
}

.status-pill::before {
  width: 7px;
  height: 7px;
  border-radius: var(--ds-radius-pill);
  background: currentColor;
  content: "";
}

.status-pill--positive {
  border-color: color-mix(in srgb, var(--ds-positive) 30%, transparent);
  background: var(--ds-positive-soft);
  color: var(--ds-positive);
}

.status-pill--warning {
  border-color: color-mix(in srgb, var(--ds-warning) 30%, transparent);
  background: var(--ds-warning-soft);
  color: var(--ds-warning);
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

.form-grid--3 {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.preview-row {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
  margin-top: 14px;
}

.preview-strip {
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
  .form-grid,
  .form-grid--3,
  .preview-row {
    grid-template-columns: 1fr;
  }
}
</style>

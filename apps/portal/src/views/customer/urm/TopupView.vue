<!--
  充值积分 — 微信扫码充值,实时到账个人积分。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       GuideHelpLink 收进 #actions);充值方式/订单收进同卡 body 的 24px 容器,
       订单 el-table → DsTable(:frame="false",金额/积分右对齐),el-tag → DsTag,
       未开放空态 el-empty → DsEmpty;全部色值改用 --ds-* token,无硬编码 hex。
       DsPagination 始终渲染;扫码支付弹窗(PortalQrPayDialog)与业务逻辑保持不变。
-->
<template>
  <div class="page-container topup-page">
    <PortalPagePanel
      fill
      :icon="CreditCard"
      :breadcrumbs="[{ label: '用户中心' }, { label: '充值与明细' }, { label: '充值积分' }]"
      description="微信扫码充值，实时到账个人积分"
    >
      <template #actions>
        <GuideHelpLink to="/help/account/topup" />
      </template>

      <div class="topup-body">
        <DsEmpty
          v-if="!configLoading && !config?.enabled"
          title="在线充值暂未开放"
          description="如需充值，请联系管理员"
        />

        <template v-else>
          <section class="topup-section">
            <header class="topup-section__head">
              <h2 class="topup-section__title">选择充值方式</h2>
              <p class="topup-section__desc">快捷套餐到账积分固定；自定义金额会扣除手续费对应的积分。</p>
            </header>

            <div class="topup-form">
              <div class="rate-panel" v-if="config">
                <strong>1 元 = {{ config.exchangeRate }} 积分</strong>
                <span>自定义充值手续费 {{ (config.feeRateBp / 100).toFixed(2) }}%，单笔金额 10~10000 元。</span>
              </div>

              <div class="package-grid">
                <div
                  v-for="pkg in packages"
                  :key="pkg.id"
                  class="package-card"
                  :class="{ 'package-card--active': selectedPackage?.id === pkg.id }"
                  role="button"
                  tabindex="0"
                  @click="choosePackage(pkg)"
                  @keydown.enter="choosePackage(pkg)"
                >
                  <span v-if="pkg.badge" class="package-badge">{{ pkg.badge }}</span>
                  <strong>¥{{ (pkg.amount / 100).toFixed(2) }}</strong>
                  <em>{{ pkg.name }}</em>
                  <span>到账 {{ pkg.credits.toLocaleString() }} 积分</span>
                  <el-button type="primary" :loading="submitting && selectedPackage?.id === pkg.id" @click.stop="submitPackage(pkg)">立即充值</el-button>
                </div>
              </div>

              <div class="custom-box">
                <div>
                  <strong>自定义金额</strong>
                  <p>输入任意金额，系统会自动计算实际到账积分。</p>
                </div>
                <el-input v-model.number="amountYuan" type="number" :min="10" :max="10000" placeholder="输入金额（元）" class="topup-input" @focus="chooseCustom">
                  <template #prepend>¥</template>
                </el-input>
                <div class="custom-preview" v-if="customPreview.gross > 0">
                  原本 {{ customPreview.gross }} 积分 - 手续费 {{ customPreview.fee }} 积分 = 到账 <b>{{ customPreview.net }}</b> 积分
                </div>
                <el-button type="primary" size="large" :loading="submitting && !selectedPackage" :disabled="!canSubmitCustom" @click="submitCustom">
                  自定义充值
                </el-button>
              </div>
            </div>
          </section>

          <section class="topup-section">
            <header class="topup-section__head">
              <h2 class="topup-section__title">充值订单</h2>
            </header>

            <DsTable
              :frame="false"
              :columns="columns"
              :rows="list"
              row-key="orderId"
              :loading="loading"
            >
              <template #cell-type="{ row }">
                {{ row.topupMode === "package" ? row.packageName || "快捷套餐" : "自定义金额" }}
              </template>
              <template #cell-amount="{ row }">
                ¥{{ (row.amount / 100).toFixed(2) }}
              </template>
              <template #cell-grossCredits="{ row }">
                {{ (row.grossCredits || row.creditAmount || 0).toLocaleString() }}
              </template>
              <template #cell-feeCredits="{ row }">
                {{ (row.feeCredits || 0).toLocaleString() }}
              </template>
              <template #cell-creditAmount="{ row }">
                <span class="topup-credits">+{{ row.creditAmount.toLocaleString() }}</span>
              </template>
              <template #cell-status="{ row }">
                <DsTag :tone="statusTone(row.status)">{{ statusText(row.status) }}</DsTag>
              </template>
              <template #cell-createdAt="{ row }">
                {{ formatTime(row.createdAt) }}
              </template>
            </DsTable>
          </section>
        </template>
      </div>

      <template #pagination>
        <DsPagination
          :page="page"
          :page-size="pageSize"
          :total="total"
          @update:page="handlePageChange"
          @update:page-size="handleSizeChange"
        />
      </template>
    </PortalPagePanel>

    <PortalQrPayDialog
      v-if="activeOrder"
      :visible="dialogVisible"
      :order-id="activeOrder.orderId"
      :code-url="activeOrder.codeUrl"
      :amount="activeOrder.amount"
      :credit-amount="activeOrder.creditAmount"
      :expires-at="activeOrder.expiresAt"
      :poll="pollActiveOrder"
      @close="dialogVisible = false"
      @success="handleSuccess"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { CreditCard } from "lucide-vue-next";
import { PortalPagePanel, PortalQrPayDialog } from "@dai/app-core";
import { GuideHelpLink } from "@dai/app-core/guide";
import type { QrPayPollResult } from "@dai/app-core";
import { DsEmpty, DsPagination, DsTable, DsTag, type DsTableColumn } from "@dai/ui";
import { urmCustomerApi } from "../../api/urmCustomer";
import type { TopupConfig, TopupOrderCreated, TopupOrderItem, TopupPackage } from "../../types/urmCustomer";

const columns: DsTableColumn[] = [
  { key: "type", title: "类型", width: 140 },
  { key: "amount", title: "金额（元）", width: 130, align: "right" },
  { key: "grossCredits", title: "原本积分", width: 130, align: "right" },
  { key: "feeCredits", title: "扣除积分", width: 130, align: "right" },
  { key: "creditAmount", title: "到账积分", width: 130, align: "right" },
  { key: "status", title: "状态", width: 110 },
  { key: "createdAt", title: "创建时间", width: 180 }
];

const config = ref<TopupConfig | null>(null);
const configLoading = ref(true);
const amountYuan = ref<number | null>(null);
const selectedPackage = ref<TopupPackage | null>(null);
const submitting = ref(false);

const activeOrder = ref<TopupOrderCreated | null>(null);
const dialogVisible = ref(false);

const page = ref(1);
const pageSize = ref(20);
const total = ref(0);
const loading = ref(false);
const list = ref<TopupOrderItem[]>([]);

const packages = computed(() => (config.value?.packages || []).filter((item) => item.enabled));
const customPreview = computed(() => {
  if (!config.value || !amountYuan.value) return { gross: 0, fee: 0, net: 0 };
  const gross = Math.floor(amountYuan.value * config.value.exchangeRate);
  const fee = Math.ceil((gross * config.value.feeRateBp) / 10000);
  return { gross, fee, net: Math.max(0, gross - fee) };
});

const canSubmitCustom = computed(() => {
  if (!config.value || !amountYuan.value || amountYuan.value <= 0) return false;
  const amountFen = Math.round(amountYuan.value * 100);
  return amountFen >= config.value.min && amountFen <= config.value.max && customPreview.value.net > 0;
});

function choosePackage(pkg: TopupPackage) {
  selectedPackage.value = pkg;
  amountYuan.value = null;
}

function chooseCustom() {
  selectedPackage.value = null;
}

function formatTime(ts?: number | null) {
  if (!ts) return "-";
  return new Date(ts).toLocaleString("zh-CN");
}

function statusTone(status: string): "positive" | "warning" | "danger" | "neutral" {
  const map: Record<string, "positive" | "warning" | "danger" | "neutral"> = {
    paid: "positive",
    created: "warning",
    paying: "warning",
    closed: "neutral",
    expired: "danger"
  };
  return map[status] || "neutral";
}

function statusText(status: string) {
  const map: Record<string, string> = {
    paid: "已到账",
    created: "待支付",
    paying: "支付确认中",
    closed: "已关闭",
    expired: "已过期"
  };
  return map[status] || status;
}

async function fetchConfig() {
  configLoading.value = true;
  try {
    config.value = await urmCustomerApi.getTopupConfig();
  } catch (e) {
    console.error("获取充值配置失败:", e);
  } finally {
    configLoading.value = false;
  }
}

async function fetchList() {
  loading.value = true;
  try {
    const res = await urmCustomerApi.listTopupOrders({ page: page.value, size: pageSize.value });
    list.value = res?.items ?? [];
    total.value = res?.total ?? 0;
  } catch (e) {
    console.error("获取充值订单失败:", e);
  } finally {
    loading.value = false;
  }
}

function handlePageChange(value: number) {
  page.value = value;
  void fetchList();
}

function handleSizeChange(value: number) {
  pageSize.value = value;
  page.value = 1;
  void fetchList();
}

async function submitPackage(pkg: TopupPackage) {
  submitting.value = true;
  try {
    activeOrder.value = await urmCustomerApi.createTopupOrder({ packageId: pkg.id });
    dialogVisible.value = true;
  } catch (err) {
    const e = err as { detail?: string; message?: string };
    ElMessage.error(e?.detail || e?.message || "发起充值失败");
  } finally {
    submitting.value = false;
  }
}

async function submitCustom() {
  if (!canSubmitCustom.value || !amountYuan.value) return;
  submitting.value = true;
  try {
    activeOrder.value = await urmCustomerApi.createTopupOrder({ amount: Math.round(amountYuan.value * 100) });
    dialogVisible.value = true;
  } catch (err) {
    const e = err as { detail?: string; message?: string };
    ElMessage.error(e?.detail || e?.message || "发起充值失败");
  } finally {
    submitting.value = false;
  }
}

async function pollActiveOrder(): Promise<QrPayPollResult> {
  if (!activeOrder.value) throw new Error("no active order");
  const status = await urmCustomerApi.getTopupOrder(activeOrder.value.orderId);
  return { status: status.status, creditAmount: status.creditAmount, transactionId: status.transactionId };
}

function handleSuccess() {
  ElMessage.success("充值成功，积分已到账");
  amountYuan.value = null;
  selectedPackage.value = null;
  void fetchList();
}

onMounted(() => {
  void fetchConfig();
  void fetchList();
});
</script>

<style scoped>
.topup-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

/* PortalPagePanel body 无内边距,用 24px 容器排布充值方式与订单两个分区 */
.topup-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 24px;
  padding: 24px;
}

.topup-section {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.topup-section + .topup-section {
  padding-top: 24px;
  border-top: 1px solid var(--ds-line);
}

.topup-section__head {
  display: flex;
  align-items: baseline;
  gap: 12px;
}

.topup-section__title {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--ds-ink);
}

.topup-section__desc {
  margin: 0;
  font-size: 12.5px;
  color: var(--ds-faint);
}

.topup-form {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.rate-panel,
.custom-box {
  border-radius: var(--ds-radius-panel);
  padding: 18px;
  background: var(--ds-accent-soft);
  color: var(--ds-ink-soft);
}

.rate-panel {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}

.package-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.package-card {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: flex-start;
  border: 1px solid var(--ds-line);
  border-radius: var(--ds-radius-panel);
  padding: 18px;
  background: var(--ds-panel);
  color: var(--ds-ink);
  text-align: left;
  cursor: pointer;
}

.package-card--active {
  border-color: var(--ds-accent);
  box-shadow: 0 14px 30px color-mix(in srgb, var(--ds-accent) 14%, transparent);
}

.package-card strong {
  font-size: 26px;
}

.package-card em {
  color: var(--ds-muted);
  font-style: normal;
}

.package-badge {
  position: absolute;
  top: 12px;
  right: 12px;
  border-radius: var(--ds-radius-pill);
  padding: 4px 8px;
  background: var(--ds-warning-soft);
  color: var(--ds-warning);
  font-size: 12px;
}

.custom-box {
  display: grid;
  grid-template-columns: 1.2fr 1fr 1.5fr auto;
  gap: 14px;
  align-items: center;
}

.custom-box p {
  margin: 4px 0 0;
}

.custom-preview {
  color: var(--ds-muted);
  font-size: 13px;
}

.topup-input {
  width: 100%;
}

.topup-credits {
  font-weight: 700;
  color: var(--ds-positive);
}

@media (max-width: 1100px) {
  .package-grid,
  .custom-box {
    grid-template-columns: 1fr;
  }

  .rate-panel {
    flex-direction: column;
  }
}
</style>

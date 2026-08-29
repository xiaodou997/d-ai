<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { CreditCard, Gift } from "lucide-vue-next";
import { PortalPagePanel, PortalQrPayDialog, type QrPayPollResult } from "@/platform";
import { DsEmpty, DsPagination, DsTable, DsTag, type DsTableColumn } from "@/shared/ui";
import { platformCustomerApi } from "@/api/platformCustomer";
import type { TopupConfig, TopupOrderCreated, TopupOrderItem, TopupPackage } from "@/api/types/platformCustomer";
import { formatDisplayMicroUSD as formatMicroUSD } from "@/shared/currency";

const MICRO_USD = 1_000_000;
const columns: DsTableColumn[] = [
  { key: "type", title: "类型", width: 140 }, { key: "paid", title: "支付金额", width: 130, align: "right" },
  { key: "gross", title: "充值金额", width: 130, align: "right" }, { key: "fee", title: "手续费", width: 120, align: "right" },
  { key: "gift", title: "赠送", width: 120, align: "right" }, { key: "credited", title: "到账", width: 130, align: "right" },
  { key: "status", title: "状态", width: 110 }, { key: "createdAt", title: "创建时间", width: 180 }
];
const config = ref<TopupConfig | null>(null);
const configLoading = ref(true);
const amountUsd = ref<number | null>(null);
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
  const gross = Math.round(Number(amountUsd.value ?? 0) * MICRO_USD);
  const fee = config.value ? Math.ceil((gross * config.value.feeRateBp) / 10_000) : 0;
  return { gross, fee, credited: Math.max(0, gross - fee) };
});
const canSubmitCustom = computed(() => Boolean(config.value
  && customPreview.value.gross >= config.value.minMicroUsd
  && customPreview.value.gross <= config.value.maxMicroUsd
  && customPreview.value.credited > 0));
function formatTime(value?: number | null) { return value ? new Date(value).toLocaleString("zh-CN") : "—"; }
function statusTone(status: string): "positive" | "warning" | "danger" | "neutral" {
  if (status === "paid") return "positive"; if (status === "created" || status === "paying") return "warning"; if (status === "expired") return "danger"; return "neutral";
}
function statusText(status: string) { return ({ paid: "已到账", created: "待支付", paying: "确认中", closed: "已关闭", expired: "已过期" } as Record<string, string>)[status] || status; }
function choosePackage(pkg: TopupPackage) { selectedPackage.value = pkg; amountUsd.value = null; }
function chooseCustom() { selectedPackage.value = null; }
async function fetchConfig() {
  configLoading.value = true;
  try { config.value = await platformCustomerApi.getTopupConfig(); }
  catch (error) { console.error("获取充值配置失败:", error); }
  finally { configLoading.value = false; }
}
async function fetchList() {
  loading.value = true;
  try { const result = await platformCustomerApi.listTopupOrders({ page: page.value, size: pageSize.value }); list.value = result.items || []; total.value = result.total || 0; }
  catch (error) { console.error("获取充值订单失败:", error); }
  finally { loading.value = false; }
}
async function createOrder(body: { amountMicroUsd?: number; packageId?: string }) {
  submitting.value = true;
  try { activeOrder.value = await platformCustomerApi.createTopupOrder(body); dialogVisible.value = true; }
  catch (error) { const e = error as { detail?: string; message?: string }; ElMessage.error(e.detail || e.message || "发起充值失败"); }
  finally { submitting.value = false; }
}
function submitPackage(pkg: TopupPackage) { choosePackage(pkg); void createOrder({ packageId: pkg.id }); }
function submitCustom() { if (canSubmitCustom.value) void createOrder({ amountMicroUsd: customPreview.value.gross }); }
async function pollActiveOrder(): Promise<QrPayPollResult> {
  if (!activeOrder.value) throw new Error("没有待支付订单");
  const result = await platformCustomerApi.getTopupOrder(activeOrder.value.orderId);
  return { status: result.status, creditedAmountMicroUsd: result.creditedAmountMicroUsd, transactionId: result.transactionId };
}
  function handleSuccess() { dialogVisible.value = false; amountUsd.value = null; selectedPackage.value = null; ElMessage.success("充值成功，USD 额度已到账"); void fetchList(); }
function handlePageChange(value: number) { page.value = value; void fetchList(); }
function handleSizeChange(value: number) { pageSize.value = value; page.value = 1; void fetchList(); }
onMounted(() => { void fetchConfig(); void fetchList(); });
</script>

<template>
  <div class="topup-page">
    <PortalPagePanel fill :icon="CreditCard" :breadcrumbs="[{ label: '用户中心' }, { label: 'USD 账户' }, { label: '额度充值' }]" description="使用微信支付充值 USD 额度，订单明确记录支付、手续费、赠送和到账金额。">
      <div class="topup-body">
        <DsEmpty v-if="!configLoading && !config?.enabled" title="在线充值暂未开放" description="如需充值，请联系管理员" />
        <template v-else>
          <section class="topup-section">
            <header><h2>选择充值金额</h2><p>额度包可包含赠送金额；自定义金额按配置扣除手续费。</p></header>
            <div v-if="configLoading" class="topup-loading" v-loading="true" />
            <template v-else-if="config">
              <div class="rule-bar"><strong>USD 充值</strong><span>自定义手续费 {{ (config.feeRateBp / 100).toFixed(2) }}%</span><span>单笔 {{ formatMicroUSD(config.minMicroUsd) }} 至 {{ formatMicroUSD(config.maxMicroUsd) }}</span></div>
              <div v-if="packages.length" class="package-grid" aria-label="额度包">
                <button v-for="pkg in packages" :key="pkg.id" type="button" :class="['package-card', { 'is-active': selectedPackage?.id === pkg.id }]" @click="submitPackage(pkg)">
                  <span v-if="pkg.badge" class="package-badge">{{ pkg.badge }}</span><strong>{{ formatMicroUSD(pkg.paymentAmountMicroUsd) }}</strong><em>{{ pkg.name }}</em>
                  <span v-if="pkg.giftAmountMicroUsd" class="package-gift"><Gift :size="13" />赠送 {{ formatMicroUSD(pkg.giftAmountMicroUsd) }}</span><small>到账 {{ formatMicroUSD(pkg.paymentAmountMicroUsd + pkg.giftAmountMicroUsd) }}</small>
                </button>
              </div>
              <div class="custom-box">
                <div><strong>自定义金额</strong><p>最多支持 6 位小数，内部按 micro-USD 记账。</p></div>
                <el-input-number v-model="amountUsd" :min="config.minMicroUsd / MICRO_USD" :max="config.maxMicroUsd / MICRO_USD" :precision="6" :controls="false" class="custom-input" placeholder="输入 USD 金额" @focus="chooseCustom" />
                <div class="custom-preview"><span>充值 {{ formatMicroUSD(customPreview.gross) }}</span><span>手续费 {{ formatMicroUSD(customPreview.fee) }}</span><strong>到账 {{ formatMicroUSD(customPreview.credited) }}</strong></div>
                <el-button type="primary" size="large" :loading="submitting && !selectedPackage" :disabled="!canSubmitCustom" @click="submitCustom">去支付</el-button>
              </div>
            </template>
          </section>
          <section class="topup-section"><header><h2>充值订单</h2><p>订单保留支付、手续费、赠送和最终到账金额快照。</p></header>
            <DsTable :frame="false" :columns="columns" :rows="list" row-key="orderId" :loading="loading" empty-title="暂无充值订单">
              <template #cell-type="{ row }">{{ row.topupMode === "package" ? row.packageName || "额度包" : "自定义充值" }}</template>
              <template #cell-paid="{ row }">${{ (row.paymentAmountMinor / 100).toFixed(2) }}</template><template #cell-gross="{ row }">{{ formatMicroUSD(row.grossAmountMicroUsd) }}</template><template #cell-fee="{ row }">{{ formatMicroUSD(row.feeAmountMicroUsd) }}</template><template #cell-gift="{ row }">{{ formatMicroUSD(row.giftAmountMicroUsd) }}</template><template #cell-credited="{ row }"><span class="amount-positive">+{{ formatMicroUSD(row.creditedAmountMicroUsd) }}</span></template>
              <template #cell-status="{ row }"><DsTag :tone="statusTone(row.status)">{{ statusText(row.status) }}</DsTag></template><template #cell-createdAt="{ row }">{{ formatTime(row.createdAt) }}</template>
            </DsTable>
          </section>
        </template>
      </div>
      <template #pagination><DsPagination :page="page" :page-size="pageSize" :total="total" @update:page="handlePageChange" @update:page-size="handleSizeChange" /></template>
    </PortalPagePanel>
    <PortalQrPayDialog v-if="activeOrder" :visible="dialogVisible" :order-id="activeOrder.orderId" :code-url="activeOrder.codeUrl" :payment-amount-minor="activeOrder.paymentAmountMinor" :credited-amount-micro-usd="activeOrder.creditedAmountMicroUsd" :expires-at="activeOrder.expiresAt" :poll="pollActiveOrder" @close="dialogVisible = false" @success="handleSuccess" />
  </div>
</template>

<style scoped>
.topup-page { display:flex; flex:1; min-height:0; flex-direction:column; }.topup-body { display:flex; flex:1; min-height:0; flex-direction:column; gap:24px; padding:24px; }.topup-section { display:flex; flex-direction:column; gap:16px; }.topup-section+.topup-section { border-top:1px solid var(--ds-line); padding-top:24px; }.topup-section header { display:flex; align-items:baseline; gap:12px; }.topup-section h2 { margin:0; color:var(--ds-ink); font-size:14px; }.topup-section p { margin:0; color:var(--ds-muted); font-size:12px; }.topup-loading { min-height:160px; }.rule-bar { display:flex; flex-wrap:wrap; gap:8px 20px; border-left:3px solid var(--ds-accent); padding:10px 14px; background:var(--ds-panel-muted); color:var(--ds-muted); font-size:12px; }.rule-bar strong { color:var(--ds-ink); }.package-grid { display:grid; grid-template-columns:repeat(4,minmax(0,1fr)); gap:12px; }.package-card { position:relative; display:flex; min-height:130px; flex-direction:column; align-items:flex-start; gap:5px; overflow:hidden; border:1px solid var(--ds-line); border-radius:8px; padding:14px; background:var(--ds-panel); color:var(--ds-ink); cursor:pointer; text-align:left; }.package-card:hover,.package-card.is-active { border-color:var(--ds-accent); box-shadow:0 0 0 1px var(--ds-accent); }.package-card strong { font-size:20px; }.package-card em { font-size:12px; font-style:normal; }.package-card small { color:var(--ds-muted); }.package-gift { display:flex; align-items:center; gap:4px; color:var(--ds-positive); font-size:12px; }.package-badge { position:absolute; top:0; right:0; padding:3px 7px; border-bottom-left-radius:6px; background:var(--ds-accent-soft); color:var(--ds-accent); font-size:11px; }.custom-box { display:grid; grid-template-columns:minmax(150px,1fr) minmax(180px,1fr) minmax(240px,1.4fr) auto; align-items:center; gap:16px; border:1px solid var(--ds-line); border-radius:8px; padding:16px; background:var(--ds-panel-muted); }.custom-box p { margin-top:3px; }.custom-input { width:100%; }.custom-preview { display:flex; flex-direction:column; gap:2px; color:var(--ds-muted); font-size:12px; }.custom-preview strong,.amount-positive { color:var(--ds-positive); }.amount-positive { font-weight:700; }
@media (max-width:1000px) { .package-grid { grid-template-columns:repeat(2,minmax(0,1fr)); }.custom-box { grid-template-columns:1fr 1fr; } } @media (max-width:600px) { .topup-body { padding:16px; }.package-grid,.custom-box { grid-template-columns:1fr; } }
</style>

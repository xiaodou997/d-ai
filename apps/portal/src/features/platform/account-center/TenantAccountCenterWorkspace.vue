<script setup lang="ts">
import { computed, shallowRef, watch } from "vue";
import { ElMessage } from "element-plus";
import { CircleDollarSign, RefreshCw, Wallet } from "lucide-vue-next";
import { useRoute, useRouter } from "vue-router";
import { PortalPagePanel } from "@/platform";
import { DsPagination } from "@/shared/ui";
import AccountAssetOverview from "./components/AccountAssetOverview.vue";
import AccountRecordsTabs from "./components/AccountRecordsTabs.vue";
import CreditPurchaseDrawer from "./components/CreditPurchaseDrawer.vue";
import { useTenantAccountCenter } from "./composables/useTenantAccountCenter";
import { formatTime, formatUSD, normalizeAccountTab, type AccountCenterTab } from "./model";

const route = useRoute();
const router = useRouter();
const center = useTenantAccountCenter({ initialTab: route.query.tab });
const purchaseVisible = shallowRef(route.query.action === "topup");
const lotsVisible = shallowRef(false);
const activeTab = computed(() => normalizeAccountTab(route.query.tab ?? center.activeTab.value));
const currentRecordsPage = computed(() => {
  if (activeTab.value === "recharges") return center.rechargeRecords.value;
  return center.balanceLedger.value;
});
function replaceQuery(next: Record<string, string | undefined>) {
  const query = { ...route.query } as Record<string, string | undefined>;
  for (const [key, value] of Object.entries(next)) value ? query[key] = value : delete query[key];
  void router.replace({ query });
}
function openPurchase() { purchaseVisible.value = true; center.openPurchase(); replaceQuery({ action: "topup" }); }
function closePurchase() { purchaseVisible.value = false; replaceQuery({ action: undefined }); }
async function selectTab(tab: AccountCenterTab) { replaceQuery({ tab: tab === "ledger" ? undefined : tab }); await center.selectTab(tab); }
async function handleTopup(body: { amountMicroUsd?: number; packageId?: string }) {
  try { await center.createTopupOrder(body); }
  catch (error) { ElMessage.error(error instanceof Error ? error.message : "发起充值失败"); }
}
async function handleQrSuccess() { await center.handleTopupSuccess(); closePurchase(); }
watch(() => route.query.action, (action) => {
  purchaseVisible.value = action === "topup";
  if (action === "topup") center.openPurchase();
}, { immediate: true });
watch(() => route.query.tab, (tab) => {
  const normalized = normalizeAccountTab(tab);
  if (center.activeTab.value !== normalized) void center.selectTab(normalized);
}, { immediate: true });
</script>

<template>
  <div class="account-center-page">
    <PortalPagePanel fill :icon="Wallet" :breadcrumbs="[{ label: '租户运营' }, { label: '财务' }, { label: '账户中心' }]" description="USD 充值、服务消费和用户充值收入统一归集到一个额度账户。">
      <template #actions><el-button type="primary" :icon="CircleDollarSign" @click="openPurchase">充值</el-button><el-button :icon="RefreshCw" :loading="center.loading.balances" @click="center.refresh">刷新</el-button></template>
      <div class="account-center-body">
        <div class="account-center-assets"><AccountAssetOverview :balance="center.balance.value" :balance-error="center.errors.balance" :nearest-expiry="center.nearestExpiry.value" @purchase="openPurchase" @lots="lotsVisible = true" @retry="center.refresh" /></div>
        <AccountRecordsTabs :active-tab="activeTab" :recharge-records="center.rechargeRecords.value" :pending-orders="center.pendingOrders.value" :balance-ledger="center.balanceLedger.value" :loading="center.loading" @tab="selectTab" />
      </div>
      <template #pagination><DsPagination :page="currentRecordsPage.page" :page-size="currentRecordsPage.size" :total="currentRecordsPage.total" @update:page="center.changePage(activeTab, $event)" /></template>
    </PortalPagePanel>

    <CreditPurchaseDrawer :visible="purchaseVisible" :config="center.topupConfig.value" :config-loading="center.loading.topupConfig" :submitting="center.loading.purchase" :active-order="center.activeOrder.value" :qr-visible="center.qrVisible.value" :poll="center.pollOrder" @close="closePurchase" @topup="handleTopup" @qr-close="center.qrVisible.value = false" @qr-success="handleQrSuccess" />
    <el-dialog v-model="lotsVisible" title="余额有效期" width="620px">
      <div class="lot-list">
        <div v-for="lot in center.balance.value.balanceLots ?? []" :key="lot.balanceLotId" class="lot-row"><div><strong>{{ lot.source || "余额" }}</strong><span>{{ lot.expiresAt ? `有效期至 ${formatTime(lot.expiresAt)}` : "长期有效" }}</span></div><b>{{ formatUSD(lot.remainingUsd) }} / {{ formatUSD(lot.totalUsd) }}</b></div>
        <el-empty v-if="!(center.balance.value.balanceLots ?? []).length" description="暂无额度包" />
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.account-center-page { display:flex; flex:1; min-height:0; flex-direction:column; }.account-center-body { display:flex; min-width:0; flex:1; min-height:0; flex-direction:column; }.account-center-assets { flex:0 0 auto; padding:16px 24px; border-bottom:1px solid var(--ds-line); }
.lot-list { display:flex; flex-direction:column; gap:10px; }.lot-row { display:flex; align-items:center; justify-content:space-between; gap:14px; border-bottom:1px solid var(--ds-line); padding:12px 0; }.lot-row>div { display:flex; min-width:0; flex-direction:column; gap:3px; }.lot-row strong { color:var(--ds-ink); font-size:13px; }.lot-row span { color:var(--ds-muted); font-size:12px; }.lot-row b { color:var(--ds-accent); font-size:13px; font-variant-numeric:tabular-nums; }
@media (max-width:768px) { .account-center-assets { padding-inline:16px; } }
</style>

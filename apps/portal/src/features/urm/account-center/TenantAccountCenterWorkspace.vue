<!--
  租户端账户中心工作台:资产总览(积分/余额) + 账户记录(积分记录/余额明细/提现记录)。
  重构:迁移至新设计系统一体面板(PortalPagePanel:图标徽章+面包屑标题+描述同行,
       资产总览与记录 Tab 工作区置于同卡 body 内 24px 容器);
       抽屉/弹窗仍为 element-plus(过渡期);业务逻辑与请求参数不变。
-->
<script setup lang="ts">
import { computed, shallowRef, watch } from "vue";
import { ElMessage } from "element-plus";
import { Coins, RefreshCw, Wallet } from "lucide-vue-next";
import { GuideHelpLink } from "@dai/app-core/guide";
import { PortalPagePanel } from "@dai/app-core";
import { DsPagination } from "@dai/ui";

import { useRoute, useRouter } from "vue-router";
import AccountAssetOverview from "./components/AccountAssetOverview.vue";
import AccountRecordsTabs from "./components/AccountRecordsTabs.vue";
import CreditPurchaseDrawer from "./components/CreditPurchaseDrawer.vue";
import WithdrawalDrawer from "./components/WithdrawalDrawer.vue";
import { useTenantAccountCenter } from "./composables/useTenantAccountCenter";
import { normalizeAccountTab, type AccountCenterTab } from "./model";

const route = useRoute();
const router = useRouter();
const center = useTenantAccountCenter({ initialTab: route.query.tab });
const purchaseVisible = shallowRef(route.query.action === "buy");
const withdrawalVisible = shallowRef(route.query.action === "withdraw");
const packageVisible = shallowRef(false);

const activeTab = computed(() => normalizeAccountTab(route.query.tab ?? center.activeTab.value));

// 面板分页脚跟随当前 Tab
const currentRecordsPage = computed(() => {
  if (activeTab.value === "balance") return center.cashLedger.value;
  if (activeTab.value === "withdrawals") return center.withdrawals.value;
  return center.pointRecords.value;
});

function replaceQuery(next: Record<string, string | undefined>) {
  const query = { ...route.query } as Record<string, string | undefined>;
  for (const [key, value] of Object.entries(next)) {
    if (value) query[key] = value;
    else delete query[key];
  }
  void router.replace({ query });
}

function openPurchase() {
  purchaseVisible.value = true;
  center.openPurchase();
  replaceQuery({ action: "buy" });
}

function closePurchase() {
  purchaseVisible.value = false;
  replaceQuery({ action: undefined });
}

function openWithdrawal() {
  withdrawalVisible.value = true;
  replaceQuery({ action: "withdraw", tab: "balance" });
}

function closeWithdrawal() {
  withdrawalVisible.value = false;
  replaceQuery({ action: undefined });
}

async function selectTab(tab: AccountCenterTab) {
  replaceQuery({ tab: tab === "points" ? undefined : tab });
  await center.selectTab(tab);
}

async function handleBalancePurchase(amountYuan: number) {
  try {
    await center.buyWithBalance(amountYuan);
    closePurchase();
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "购买积分失败");
  }
}

async function handleWechatPurchase(body: { amount?: number; packageId?: string }) {
  try {
    await center.createWechatOrder(body);
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "发起支付失败");
  }
}

async function handleQrSuccess() {
  await center.handleTopupSuccess();
  closePurchase();
}

async function handleWithdrawal(form: { amountYuan: number; accountName: string; bankName: string; accountNo: string; note?: string }) {
  try {
    await center.submitWithdrawal(form);
    closeWithdrawal();
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "提交提现申请失败");
  }
}

async function handlePageChange(payload: { tab: AccountCenterTab; page: number }) {
  await center.changePage(payload.tab, payload.page);
}

async function handleCancelWithdrawal(id: string) {
  try {
    await center.cancelWithdrawal(id);
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "取消提现失败");
  }
}

watch(
  () => route.query.action,
  (action) => {
    purchaseVisible.value = action === "buy";
    withdrawalVisible.value = action === "withdraw";
    if (action === "buy") center.openPurchase();
  },
  { immediate: true }
);

watch(
  () => route.query.tab,
  (tab) => {
    const normalized = normalizeAccountTab(tab);
    if (center.activeTab.value !== normalized) void center.selectTab(normalized);
  },
  { immediate: true }
);

</script>

<template>
  <div class="account-center-page">
    <PortalPagePanel
      fill
      :icon="Wallet"
      :breadcrumbs="[{ label: '租户运营' }, { label: '账户' }, { label: '账户中心' }]"
      description="查看你的积分和余额，完成购买积分、余额管理与提现。"
    >
      <template #actions>
        <GuideHelpLink to="/help/urm/account" />
        <el-button type="primary" :icon="Coins" @click="openPurchase">购买积分</el-button>
        <el-button :icon="RefreshCw" :loading="center.loading.balances" @click="center.refresh">刷新</el-button>
      </template>

      <!-- 面板 body 无内边距:资产总览用 24px 容器 + 分隔线,记录工作区通栏并撑满剩余高度 -->
      <div class="account-center-body">
        <div class="account-center-assets">
          <AccountAssetOverview
            :points="center.points.value"
            :cash="center.cash.value"
            :points-error="center.errors.points"
            :cash-error="center.errors.cash"
            :nearest-expiry="center.nearestExpiry.value"
            @purchase="openPurchase"
            @withdraw="openWithdrawal"
            @packages="packageVisible = true"
            @retry="center.refresh"
          />
        </div>

        <AccountRecordsTabs
          :active-tab="activeTab"
          :point-records="center.pointRecords.value"
          :pending-orders="center.pendingOrders.value"
          :cash-ledger="center.cashLedger.value"
          :withdrawals="center.withdrawals.value"
          :loading="center.loading"
          @tab="selectTab"
          @cancel-withdrawal="handleCancelWithdrawal"
        />
      </div>

      <!-- 三个 Tab 共用面板底部的分页脚,按当前 Tab 取对应分页数据 -->
      <template #pagination>
        <DsPagination
          :page="currentRecordsPage.page"
          :page-size="currentRecordsPage.size"
          :total="currentRecordsPage.total"
          @update:page="handlePageChange({ tab: activeTab, page: $event })"
        />
      </template>
    </PortalPagePanel>

    <CreditPurchaseDrawer
      :visible="purchaseVisible"
      :method="center.purchaseMethod.value"
      :cash="center.cash.value"
      :config="center.topupConfig.value"
      :config-loading="center.loading.topupConfig"
      :submitting="center.loading.purchase"
      :active-order="center.activeOrder.value"
      :qr-visible="center.qrVisible.value"
      :poll="center.pollOrder"
      @close="closePurchase"
      @update:method="center.purchaseMethod.value = $event"
      @balance-purchase="handleBalancePurchase"
      @wechat-purchase="handleWechatPurchase"
      @qr-close="center.qrVisible.value = false"
      @qr-success="handleQrSuccess"
    />

    <WithdrawalDrawer
      :visible="withdrawalVisible"
      :cash="center.cash.value"
      :submitting="center.loading.withdrawal"
      @close="closeWithdrawal"
      @submit="handleWithdrawal"
    />

    <el-dialog v-model="packageVisible" title="积分明细" width="620px">
      <div class="package-detail-list">
        <div v-for="pkg in center.points.value.packages ?? []" :key="pkg.packageId" class="package-detail-row">
          <div><strong>{{ pkg.source || "积分" }}</strong><span>{{ pkg.expiresAt ? `有效期至 ${new Date(pkg.expiresAt).toLocaleString('zh-CN')}` : "长期有效" }}</span></div>
          <b>{{ pkg.remainingCredits.toLocaleString() }} / {{ pkg.totalCredits.toLocaleString() }} 积分</b>
        </div>
        <el-empty v-if="!(center.points.value.packages ?? []).length" description="暂无积分包" />
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.account-center-page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

/* 面板 body 无内边距:资产区固定高度,记录工作区吃掉剩余空间 */
.account-center-body {
  display: flex;
  flex: 1;
  min-height: 0;
  min-width: 0;
  flex-direction: column;
}

.account-center-assets {
  flex: 0 0 auto;
  padding: 16px 24px;
  border-bottom: 1px solid var(--ds-line);
}

@media (max-width: 768px) {
  .account-center-assets {
    padding-inline: 16px;
  }
}

.package-detail-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.package-detail-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  border-bottom: 1px solid var(--ds-line);
  padding: 12px 0;
}

.package-detail-row > div {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;
}

.package-detail-row strong {
  color: var(--ds-ink);
  font-size: 13px;
}

.package-detail-row span {
  color: var(--ds-muted);
  font-size: 12px;
}

.package-detail-row > b {
  flex: 0 0 auto;
  color: var(--ds-accent-hover);
  font-size: 13px;
}

@media (max-width: 640px) {
  .package-detail-row {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>

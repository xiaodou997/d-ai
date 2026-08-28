<template>
  <div class="recharge-orders-page">
    <PortalPagePanel
      :icon="ReceiptText"
      :breadcrumbs="[{ label: '资金中心' }, { label: '充值订单' }]"
      description="统一查看手动充值、微信支付、退款和额度冲正状态。"
    >
      <template #filters>
        <DsFilterBar>
          <DsFilterField label="搜索">
            <el-input
              v-model="query.keyword"
              class="filter-keyword"
              placeholder="订单号、租户或用户"
              clearable
              @keyup.enter="search"
            >
              <template #prefix><Search :size="16" /></template>
            </el-input>
          </DsFilterField>
          <DsFilterField label="来源">
            <el-select v-model="query.method" class="filter-select" placeholder="全部来源" clearable>
              <el-option label="手动充值" value="manual" />
              <el-option label="微信在线" value="online" />
            </el-select>
          </DsFilterField>
          <DsFilterField label="充值对象">
            <el-select v-model="query.targetType" class="filter-select" placeholder="全部对象" clearable>
              <el-option label="租户" value="tenant" />
              <el-option label="终端用户" value="user" />
            </el-select>
          </DsFilterField>
          <DsFilterField label="支付状态">
            <el-select v-model="query.paymentStatus" class="filter-select" placeholder="全部状态" clearable>
              <el-option label="待支付" value="created" />
              <el-option label="确认中" value="paying" />
              <el-option label="已支付" value="paid" />
              <el-option label="已关闭" value="closed" />
              <el-option label="已过期" value="expired" />
              <el-option label="无需支付" value="not_required" />
            </el-select>
          </DsFilterField>
          <DsFilterField label="到账状态">
            <el-select v-model="query.fulfillmentStatus" class="filter-select" placeholder="全部状态" clearable>
              <el-option label="未到账" value="pending" />
              <el-option label="已到账" value="credited" />
              <el-option label="部分撤回" value="partially_reversed" />
              <el-option label="已撤回 / 冲正" value="reversed" />
            </el-select>
          </DsFilterField>
          <DsFilterField label="退款状态">
            <el-select v-model="query.refundStatus" class="filter-select" placeholder="全部状态" clearable>
              <el-option label="未退款" value="none" />
              <el-option label="已退款" value="refunded" />
              <el-option label="不适用" value="not_applicable" />
            </el-select>
          </DsFilterField>

          <template #actions>
            <el-button type="primary" @click="search"><Search :size="16" />筛选</el-button>
            <el-button @click="resetQuery"><RotateCcw :size="16" />重置</el-button>
          </template>
        </DsFilterBar>
      </template>

      <DsTable
        :frame="false"
        :columns="columns"
        :rows="rows"
        row-key="orderId"
        :loading="loading"
        empty-title="暂无充值订单"
        empty-description="当前筛选条件下没有充值或支付记录"
      >
        <template #cell-orderId="{ row }">
          <div class="primary-cell">
            <span class="mono truncate-id" :title="row.orderId">{{ row.orderId }}</span>
            <span class="secondary-text">{{ orderTypeText(row.orderType) }}</span>
          </div>
        </template>
        <template #cell-party="{ row }">
          <div class="primary-cell">
            <span>{{ row.targetType === 'user' ? row.username || '未知用户' : row.tenantName || '未知租户' }}</span>
            <span class="secondary-text">
              {{ row.targetType === 'user' ? row.tenantName || row.tenantId : row.tenantId }}
            </span>
          </div>
        </template>
        <template #cell-method="{ row }">
          <DsTag :tone="row.method === 'online' ? 'accent' : 'neutral'">
            {{ row.method === "online" ? "微信在线" : "手动充值" }}
          </DsTag>
        </template>
        <template #cell-reconciliation="{ row }">
          <div v-if="row.method === 'online'" class="reconciliation-cell">
            <div class="reference-line">
              <span class="reference-label">商户</span>
              <span class="mono truncate-id" :title="row.outTradeNo">{{ row.outTradeNo || "—" }}</span>
              <button v-if="row.outTradeNo" class="copy-button" title="复制商户订单号" @click="copyText(row.outTradeNo)"><Copy :size="13" /></button>
            </div>
            <div class="reference-line">
              <span class="reference-label">微信</span>
              <span class="mono truncate-id" :title="row.transactionId">{{ row.transactionId || "待生成" }}</span>
              <button v-if="row.transactionId" class="copy-button" title="复制微信支付订单号" @click="copyText(row.transactionId)"><Copy :size="13" /></button>
            </div>
          </div>
          <span v-else class="secondary-text">无需对账单号</span>
        </template>
        <template #cell-amount="{ row }">
          <div class="amount-cell">
            <span>实付 {{ formatPaid(row.paidAmountMinor) }}</span>
            <span class="credited-amount">到账 {{ formatMicroUSD(row.creditedAmountMicroUsd) }}</span>
          </div>
        </template>
        <template #cell-status="{ row }">
          <div class="status-cell">
            <DsTag :tone="paymentTone(row.paymentStatus)">{{ paymentStatusText(row.paymentStatus) }}</DsTag>
            <DsTag :tone="fulfillmentTone(row.fulfillmentStatus, row.paymentStatus)">{{ fulfillmentStatusText(row.fulfillmentStatus, row.refundStatus, row.paymentStatus) }}</DsTag>
            <DsTag v-if="row.refundStatus === 'refunded'" tone="danger">已退款</DsTag>
          </div>
        </template>
        <template #cell-createdAt="{ row }">
          <div class="primary-cell time-cell">
            <span>{{ formatTime(row.createdAt) }}</span>
            <span v-if="row.paidAt" class="secondary-text">支付 {{ formatTime(row.paidAt) }}</span>
          </div>
        </template>
        <template #cell-actions="{ row }">
          <div class="row-actions">
            <el-tooltip content="查看详情" placement="top">
              <el-button link type="primary" aria-label="查看详情" @click="openDetail(row)"><PanelRightOpen :size="17" /></el-button>
            </el-tooltip>
            <el-tooltip v-if="canSync(row)" content="同步支付状态" placement="top">
              <el-button link type="primary" aria-label="同步支付状态" @click="syncOrder(row)"><RefreshCw :size="17" /></el-button>
            </el-tooltip>
          </div>
        </template>
      </DsTable>

      <template #pagination>
        <DsPagination
          :page="page"
          :page-size="pageSize"
          :total="total"
          @update:page="handlePageChange"
          @update:page-size="handlePageSizeChange"
        />
      </template>
    </PortalPagePanel>

    <DsDrawer
      :open="drawerOpen"
      title="充值订单详情"
      :subtitle="detail?.orderId"
      width="min(720px, 100vw)"
      @close="drawerOpen = false"
    >
      <div v-if="detailLoading" class="detail-loading"><DsSkeleton :rows="8" /></div>
      <div v-else-if="detail" class="detail-content">
        <section class="detail-section detail-summary">
          <div>
            <span class="detail-label">支付状态</span>
            <DsTag :tone="paymentTone(detail.paymentStatus)">{{ paymentStatusText(detail.paymentStatus) }}</DsTag>
          </div>
          <div>
            <span class="detail-label">到账状态</span>
            <DsTag :tone="fulfillmentTone(detail.fulfillmentStatus, detail.paymentStatus)">{{ fulfillmentStatusText(detail.fulfillmentStatus, detail.refundStatus, detail.paymentStatus) }}</DsTag>
          </div>
          <div v-if="detail.method === 'online'">
            <span class="detail-label">退款状态</span>
            <DsTag :tone="detail.refundStatus === 'refunded' ? 'danger' : 'neutral'">{{ detail.refundStatus === "refunded" ? "已退款" : "未退款" }}</DsTag>
          </div>
          <div>
            <span class="detail-label">充值对象</span>
            <strong>{{ partyText(detail) }}</strong>
          </div>
        </section>

        <DetailSection title="金额明细">
          <DetailRow label="实付金额" :value="formatPaid(detail.paidAmountMinor)" />
          <DetailRow label="充值金额" :value="formatMicroUSD(detail.grossAmountMicroUsd)" />
          <DetailRow label="手续费" :value="formatMicroUSD(detail.feeAmountMicroUsd)" />
          <DetailRow label="赠送金额" :value="formatMicroUSD(detail.giftAmountMicroUsd)" />
          <DetailRow label="实际到账" :value="formatMicroUSD(detail.creditedAmountMicroUsd)" strong />
          <DetailRow v-if="detail.tenantIncomeMicroUsd > 0" label="租户收入额度" :value="formatMicroUSD(detail.tenantIncomeMicroUsd)" />
        </DetailSection>

        <DetailSection v-if="detail.refund" title="退款记录">
          <DetailRow label="退款方式" :value="refundMethodText(detail.refund.method)" />
          <DetailRow label="退款金额" :value="formatPaid(detail.refund.refundAmountMinor)" strong />
          <DetailRow :label="detail.refund.method === 'wechat' ? '商户退款单号' : '线下凭证号'" :value="detail.refund.refundReference" mono copyable @copy="copyText(detail.refund.refundReference)" />
          <DetailRow v-if="detail.refund.channelRefundId" label="微信退款单号" :value="detail.refund.channelRefundId" mono copyable @copy="copyText(detail.refund.channelRefundId)" />
          <DetailRow label="退款完成时间" :value="formatTime(detail.refund.refundedAt)" />
          <DetailRow label="退款原因" :value="detail.refund.reason" />
          <DetailRow label="操作人" :value="detail.refund.operatorId" mono />
          <DetailRow v-if="detail.refund.note" label="退款备注" :value="detail.refund.note" />
        </DetailSection>

        <DetailSection title="支付与对账">
          <DetailRow label="支付渠道" :value="channelText(detail.channel)" />
          <DetailRow label="商户订单号" :value="detail.outTradeNo || '—'" mono copyable @copy="copyText(detail.outTradeNo)" />
          <DetailRow label="微信支付订单号" :value="detail.transactionId || '—'" mono copyable @copy="copyText(detail.transactionId)" />
          <DetailRow label="业务订单号" :value="detail.orderId" mono copyable @copy="copyText(detail.orderId)" />
          <DetailRow label="创建时间" :value="formatTime(detail.createdAt)" />
          <DetailRow label="支付时间" :value="formatTime(detail.paidAt)" />
          <DetailRow label="支付截止" :value="formatTime(detail.paymentExpiresAt)" />
          <DetailRow v-if="detail.failNote" label="支付异常" :value="detail.failNote" danger />
        </DetailSection>

        <DetailSection title="额度记录">
          <div v-if="detail.credits?.length" class="credit-list">
            <div v-for="credit in detail.credits" :key="`${credit.balanceOrderId}-${credit.lotId}`" class="credit-item">
              <div class="credit-item__head">
                <div>
                  <strong>{{ credit.primary ? "目标到账额度" : "关联租户收入" }}</strong>
                  <span class="mono secondary-text">{{ credit.balanceOrderId }}</span>
                </div>
                <DsTag :tone="credit.status === 'reversed' ? 'info' : 'positive'">{{ creditStatusText(credit) }}</DsTag>
              </div>
              <dl v-if="credit.refundId" class="credit-metrics credit-metrics--refund">
                <div><dt>原到账</dt><dd>{{ formatMicroUSD(credit.creditAmountMicroUsd) }}</dd></div>
                <div><dt>收回可用额度</dt><dd>{{ formatMicroUSD(credit.refundAvailableMicroUsd) }}</dd></div>
                <div><dt>消费/清欠冲正</dt><dd>{{ formatMicroUSD(credit.refundNonAvailableMicroUsd) }}</dd></div>
                <div><dt>已过期未重复扣</dt><dd>{{ formatMicroUSD(credit.refundExpiredMicroUsd) }}</dd></div>
                <div><dt>账户实际扣减</dt><dd>{{ formatMicroUSD(credit.refundAccountDebitMicroUsd) }}</dd></div>
                <div><dt>冲正后余额</dt><dd :class="{ 'debt-value': credit.refundBalanceAfterMicroUsd < 0 }">{{ formatMicroUSD(credit.refundBalanceAfterMicroUsd) }}</dd></div>
              </dl>
              <dl v-else class="credit-metrics">
                <div><dt>到账</dt><dd>{{ formatMicroUSD(credit.creditAmountMicroUsd) }}</dd></div>
                <div><dt>已消费</dt><dd>{{ formatMicroUSD(consumedCredit(credit)) }}</dd></div>
                <div><dt>{{ credit.status === 'reversed' ? '已回收' : '剩余' }}</dt><dd>{{ formatMicroUSD(credit.status === 'reversed' ? credit.reversedAmountMicroUsd : credit.remainingAmountMicroUsd) }}</dd></div>
              </dl>
              <div v-if="credit.lotId" class="reference-line credit-lot"><span class="reference-label">额度包</span><span class="mono">{{ credit.lotId }}</span></div>
            </div>
          </div>
          <p v-else class="empty-detail">尚未生成额度记录</p>
        </DetailSection>

        <DetailSection title="备注与操作记录">
          <DetailRow label="充值备注" :value="detail.note || '—'" />
          <DetailRow label="额度有效期" :value="formatTime(detail.balanceExpiresAt)" />
          <DetailRow v-if="detail.reversalReason" label="撤回原因" :value="detail.reversalReason" />
          <DetailRow v-if="detail.reversedBy" label="操作人" :value="detail.reversedBy" mono />
          <DetailRow v-if="detail.reversedAt" label="撤回时间" :value="formatTime(detail.reversedAt)" />
        </DetailSection>
      </div>

      <template #footer>
        <el-button v-if="detail && canSync(detail)" :loading="syncing" @click="syncOrder(detail)"><RefreshCw :size="16" />同步支付状态</el-button>
        <el-button v-if="detail && canRefund(detail)" type="danger" @click="openRefund(detail)"><BanknoteArrowDown :size="16" />登记退款并冲正</el-button>
        <el-button v-else-if="detail?.method === 'manual' && detail.fulfillmentStatus === 'credited'" type="danger" @click="openReverse(detail)"><Undo2 :size="16" />撤回额度</el-button>
      </template>
    </DsDrawer>

    <el-dialog v-model="reverseDialogOpen" title="确认撤回额度" width="520px" :close-on-click-modal="false" append-to-body>
      <div v-if="reverseTarget" class="reverse-content">
        <div class="reverse-warning">
          此操作仅用于没有在线支付的手动充值，回收目标账户额度包中的剩余额度；已经消费的部分不会形成额外欠费。
        </div>
        <dl class="reverse-summary">
          <div><dt>订单</dt><dd class="mono">{{ reverseTarget.orderId }}</dd></div>
          <div><dt>原始到账</dt><dd>{{ formatMicroUSD(primaryCredit(reverseTarget)?.creditAmountMicroUsd ?? reverseTarget.creditedAmountMicroUsd) }}</dd></div>
          <div><dt>已消费</dt><dd>{{ formatMicroUSD(consumedPrimaryCredit(reverseTarget)) }}</dd></div>
          <div><dt>预计回收</dt><dd class="reclaim-value">{{ formatMicroUSD(primaryCredit(reverseTarget)?.remainingAmountMicroUsd ?? reverseTarget.creditedAmountMicroUsd) }}</dd></div>
        </dl>
        <el-form label-position="top">
          <el-form-item label="撤回原因" required>
            <el-input v-model="reverseReason" type="textarea" :rows="3" maxlength="500" show-word-limit placeholder="记录本次额度撤回的业务原因" />
          </el-form-item>
        </el-form>
      </div>
      <template #footer>
        <el-button @click="reverseDialogOpen = false">取消</el-button>
        <el-button type="danger" :loading="reversing" @click="confirmReverse">确认撤回额度</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="refundDialogOpen" title="登记已完成退款并整单冲正" width="640px" :close-on-click-modal="false" append-to-body>
      <div v-if="refundTarget" class="refund-content">
        <div class="refund-warning">
          仅在退款资金已经实际退回后执行。确认后将同时冲正用户到账额度和租户收入额度，已消费或曾用于清欠的部分可能使账户形成负余额。
        </div>
        <dl class="refund-order-summary">
          <div><dt>支付订单</dt><dd class="mono">{{ refundTarget.orderId }}</dd></div>
          <div><dt>原支付金额</dt><dd>{{ formatPaid(refundTarget.paidAmountMinor) }}</dd></div>
          <div><dt>用户到账</dt><dd>{{ formatMicroUSD(refundTarget.creditedAmountMicroUsd) }}</dd></div>
          <div v-if="refundTarget.tenantIncomeMicroUsd > 0"><dt>租户收入</dt><dd>{{ formatMicroUSD(refundTarget.tenantIncomeMicroUsd) }}</dd></div>
        </dl>
        <div v-if="refundTarget.credits?.length" class="refund-impact-list">
          <div v-for="credit in refundTarget.credits" :key="credit.balanceOrderId" class="refund-impact-row">
            <div><strong>{{ credit.primary ? "目标到账额度" : "关联租户收入" }}</strong><span class="secondary-text">{{ credit.balanceOrderId }}</span></div>
            <span>预计扣减 {{ formatMicroUSD(estimatedRefundDebit(credit)) }}</span>
          </div>
        </div>
        <el-form label-position="top">
          <div class="refund-form-grid">
            <el-form-item label="退款方式" required>
              <el-segmented v-model="refundForm.method" :options="refundMethodOptions" />
            </el-form-item>
            <el-form-item label="退款完成时间" required>
              <el-date-picker v-model="refundForm.refundedAt" type="datetime" placeholder="选择退款完成时间" :disabled-date="disableFutureDate" />
            </el-form-item>
            <el-form-item :label="refundForm.method === 'wechat' ? '商户退款单号' : '线下退款凭证号'" required>
              <el-input v-model="refundForm.refundReference" maxlength="128" placeholder="用于退款对账" />
            </el-form-item>
            <el-form-item v-if="refundForm.method === 'wechat'" label="微信退款单号" required>
              <el-input v-model="refundForm.channelRefundId" maxlength="128" placeholder="微信支付退款单号" />
            </el-form-item>
          </div>
          <el-form-item label="退款原因" required>
            <el-input v-model="refundForm.reason" type="textarea" :rows="2" maxlength="500" show-word-limit placeholder="记录退款与冲正原因" />
          </el-form-item>
          <el-form-item label="备注">
            <el-input v-model="refundForm.note" type="textarea" :rows="2" maxlength="1000" show-word-limit placeholder="可记录线下经办信息或其他说明" />
          </el-form-item>
        </el-form>
      </div>
      <template #footer>
        <el-button @click="refundDialogOpen = false">取消</el-button>
        <el-button type="danger" :loading="refunding" @click="confirmRefund">确认已退款并整单冲正</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { defineComponent, h, reactive, ref } from "vue";
import { ElMessage } from "element-plus";
import { BanknoteArrowDown, Copy, PanelRightOpen, ReceiptText, RefreshCw, RotateCcw, Search, Undo2 } from "lucide-vue-next";
import { platformAdminApi } from "@/api/platformAdmin";
import type { AdminRechargeOrder, RechargeCreditDetail } from "@/api/types/admin";
import { PortalPagePanel, useListPage } from "@/platform";
import { formatDisplayMicroUSD as formatMicroUSD } from "@/shared/currency";
import {
  DsDrawer,
  DsFilterBar,
  DsFilterField,
  DsPagination,
  DsSkeleton,
  DsTable,
  DsTag,
  type DsTableColumn
} from "@/shared/ui";

const columns: DsTableColumn[] = [
  { key: "orderId", title: "业务单号", width: 205 },
  { key: "party", title: "充值对象", width: 170 },
  { key: "method", title: "来源", width: 100 },
  { key: "reconciliation", title: "对账单号", width: 280 },
  { key: "amount", title: "支付 / 到账", width: 160, align: "right" },
  { key: "status", title: "支付 / 到账状态", width: 240 },
  { key: "createdAt", title: "时间", width: 180 },
  { key: "actions", title: "操作", width: 110, align: "center" }
];

const {
  rows, total, loading, page, pageSize, query, refresh, search, resetQuery,
  handlePageChange, handlePageSizeChange
} = useListPage<{
  keyword: string;
  method: "" | "manual" | "online";
  targetType: "" | "tenant" | "user";
  paymentStatus: string;
  fulfillmentStatus: string;
  refundStatus: "" | "none" | "refunded" | "not_applicable";
}, AdminRechargeOrder>({
  initialQuery: { keyword: "", method: "", targetType: "", paymentStatus: "", fulfillmentStatus: "", refundStatus: "" },
  pageSize: 20,
  fetcher: async (params) => {
    try {
      const data = await platformAdminApi.listAdminRechargeOrders({
        keyword: params.keyword || undefined,
        method: params.method || undefined,
        targetType: params.targetType || undefined,
        paymentStatus: params.paymentStatus || undefined,
        fulfillmentStatus: params.fulfillmentStatus || undefined,
        refundStatus: params.refundStatus || undefined,
        page: params.page,
        size: params.pageSize
      });
      return { items: data.items || [], total: data.total || 0 };
    } catch (error) {
      ElMessage.error("获取充值订单失败");
      throw error;
    }
  }
});

const drawerOpen = ref(false);
const detailLoading = ref(false);
const detail = ref<AdminRechargeOrder | null>(null);
const syncing = ref(false);
const reverseDialogOpen = ref(false);
const reverseTarget = ref<AdminRechargeOrder | null>(null);
const reverseReason = ref("");
const reversing = ref(false);
const refundDialogOpen = ref(false);
const refundTarget = ref<AdminRechargeOrder | null>(null);
const refunding = ref(false);
const refundMethodOptions = [
  { label: "微信退款", value: "wechat" },
  { label: "线下退款", value: "offline" }
];
const refundForm = reactive({
  method: "wechat" as "wechat" | "offline",
  refundReference: "",
  channelRefundId: "",
  refundedAt: new Date(),
  reason: "",
  note: ""
});

const DetailSection = defineComponent({
  props: { title: { type: String, required: true } },
  setup(props, { slots }) {
    return () => h("section", { class: "detail-section" }, [h("h3", props.title), h("div", { class: "detail-grid" }, slots.default?.())]);
  }
});

const DetailRow = defineComponent({
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
    mono: Boolean,
    strong: Boolean,
    danger: Boolean,
    copyable: Boolean
  },
  emits: ["copy"],
  setup(props, { emit }) {
    return () => h("div", { class: "detail-row" }, [
      h("dt", props.label),
      h("dd", { class: { mono: props.mono, "detail-value--strong": props.strong, "detail-value--danger": props.danger } }, [
        h("span", props.value),
        props.copyable && props.value !== "—" ? h("button", { class: "copy-button", title: `复制${props.label}`, onClick: () => emit("copy") }, [h(Copy, { size: 13 })]) : null
      ])
    ]);
  }
});

async function openDetail(row: AdminRechargeOrder) {
  drawerOpen.value = true;
  detail.value = null;
  detailLoading.value = true;
  try {
    detail.value = await platformAdminApi.getAdminRechargeOrder(row.orderId);
  } catch (error) {
    ElMessage.error("获取订单详情失败");
    drawerOpen.value = false;
  } finally {
    detailLoading.value = false;
  }
}

async function syncOrder(row: AdminRechargeOrder) {
  syncing.value = true;
  try {
    const updated = await platformAdminApi.syncAdminRechargeOrder(row.orderId);
    ElMessage.success(`同步完成：${paymentStatusText(updated.paymentStatus)}`);
    if (detail.value?.orderId === updated.orderId) detail.value = updated;
    await refresh();
  } catch (error: any) {
    ElMessage.error(error?.detail || error?.message || "同步失败");
  } finally {
    syncing.value = false;
  }
}

async function openReverse(row: AdminRechargeOrder) {
  reverseReason.value = "";
  reverseTarget.value = row;
  if (!row.credits) {
    try {
      reverseTarget.value = await platformAdminApi.getAdminRechargeOrder(row.orderId);
    } catch (error) {
      ElMessage.error("获取可撤回额度失败");
      return;
    }
  }
  reverseDialogOpen.value = true;
}

async function confirmReverse() {
  if (!reverseTarget.value) return;
  if (!reverseReason.value.trim()) {
    ElMessage.warning("请输入撤回原因");
    return;
  }
  reversing.value = true;
  try {
    const updated = await platformAdminApi.reverseAdminRechargeOrderCredit(reverseTarget.value.orderId, { reason: reverseReason.value.trim() });
    reverseDialogOpen.value = false;
    detail.value = detail.value?.orderId === updated.orderId ? updated : detail.value;
    ElMessage.success(updated.fulfillmentStatus === "partially_reversed" ? "剩余额度已回收，已消费部分保留" : "额度已撤回");
    await refresh();
  } catch (error: any) {
    ElMessage.error(error?.detail || error?.message || "撤回失败");
  } finally {
    reversing.value = false;
  }
}

async function openRefund(row: AdminRechargeOrder) {
  refundForm.method = "wechat";
  refundForm.refundReference = "";
  refundForm.channelRefundId = "";
  refundForm.refundedAt = new Date();
  refundForm.reason = "";
  refundForm.note = "";
  try {
    refundTarget.value = row.credits ? row : await platformAdminApi.getAdminRechargeOrder(row.orderId);
    refundDialogOpen.value = true;
  } catch {
    ElMessage.error("获取退款冲正信息失败");
  }
}

async function confirmRefund() {
  if (!refundTarget.value) return;
  if (!refundForm.refundReference.trim() || !refundForm.reason.trim()) {
    ElMessage.warning("请填写退款单号和退款原因");
    return;
  }
  if (refundForm.method === "wechat" && !refundForm.channelRefundId.trim()) {
    ElMessage.warning("请填写微信退款单号");
    return;
  }
  if (!refundForm.refundedAt || refundForm.refundedAt.getTime() > Date.now()) {
    ElMessage.warning("退款完成时间不能晚于当前时间");
    return;
  }
  refunding.value = true;
  try {
    const updated = await platformAdminApi.recordCompletedRechargeRefund(refundTarget.value.orderId, {
      method: refundForm.method,
      refundReference: refundForm.refundReference.trim(),
      channelRefundId: refundForm.method === "wechat" ? refundForm.channelRefundId.trim() : undefined,
      refundedAt: refundForm.refundedAt.getTime(),
      reason: refundForm.reason.trim(),
      note: refundForm.note.trim() || undefined
    });
    refundDialogOpen.value = false;
    if (detail.value?.orderId === updated.orderId) detail.value = updated;
    ElMessage.success("退款已登记，用户额度与租户收入已完成整单冲正");
    await refresh();
  } catch (error: any) {
    ElMessage.error(error?.detail || error?.message || "退款冲正失败");
  } finally {
    refunding.value = false;
  }
}

function primaryCredit(order: AdminRechargeOrder) {
  return order.credits?.find((credit) => credit.primary);
}

function consumedPrimaryCredit(order: AdminRechargeOrder) {
  const credit = primaryCredit(order);
  return credit ? consumedCredit(credit) : 0;
}

function canSync(order: AdminRechargeOrder) {
  return order.method === "online" && (order.paymentStatus === "created" || order.paymentStatus === "paying");
}

function canRefund(order: AdminRechargeOrder) {
  return order.method === "online" && order.paymentStatus === "paid" && order.fulfillmentStatus === "credited" && order.refundStatus === "none";
}

function estimatedRefundDebit(credit: RechargeCreditDetail) {
  const expired = credit.lotStatus === "expired" ? credit.remainingAmountMicroUsd : 0;
  return Math.max(credit.creditAmountMicroUsd - expired, 0);
}

function disableFutureDate(value: Date) {
  return value.getTime() > Date.now();
}

async function copyText(value?: string) {
  if (!value) return;
  try {
    await navigator.clipboard.writeText(value);
    ElMessage.success("已复制");
  } catch {
    ElMessage.error("复制失败");
  }
}

function formatPaid(value?: number | null) {
  return `$${(Number(value ?? 0) / 100).toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

function formatTime(value?: number | null) {
  return value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "—";
}

function partyText(order: AdminRechargeOrder) {
  return order.targetType === "user"
    ? `${order.username || "未知用户"} · ${order.tenantName || order.tenantId}`
    : order.tenantName || order.tenantId;
}

function orderTypeText(value: string) {
  return ({
    platform_to_tenant: "平台为租户充值",
    tenant_to_user: "租户为用户充值",
    online_tenant_topup: "租户在线充值",
    online_user_topup: "用户在线充值"
  } as Record<string, string>)[value] || value;
}

function channelText(value?: string) {
  return ({ wechat_native: "微信 Native 支付", manual: "手动记账" } as Record<string, string>)[value || ""] || value || "—";
}

function paymentStatusText(value: string) {
  return ({ not_required: "无需支付", created: "待支付", paying: "确认中", paid: "已支付", closed: "已关闭", expired: "已过期" } as Record<string, string>)[value] || value;
}

function fulfillmentStatusText(value: string, refundStatus?: string, paymentStatus?: string) {
  if (value === "reversed" && refundStatus === "refunded") return "已冲正";
  if (value === "pending" && paymentStatus === "closed") return "已关闭（未到账）";
  if (value === "pending" && paymentStatus === "expired") return "已过期（未到账）";
  return ({ pending: "待到账", credited: "已到账", partially_reversed: "部分撤回", reversed: "已撤回" } as Record<string, string>)[value] || value;
}

function refundMethodText(value: string) {
  return value === "wechat" ? "微信退款" : "线下退款";
}

type Tone = "positive" | "warning" | "danger" | "info" | "neutral" | "accent";
function paymentTone(value: string): Tone {
  return ({ paid: "positive", created: "warning", paying: "warning", closed: "info", expired: "danger", not_required: "neutral" } as Record<string, Tone>)[value] || "neutral";
}

function fulfillmentTone(value: string, paymentStatus?: string): Tone {
  if (value === "pending" && paymentStatus === "closed") return "info";
  if (value === "pending" && paymentStatus === "expired") return "danger";
  return ({ pending: "warning", credited: "accent", partially_reversed: "warning", reversed: "info" } as Record<string, Tone>)[value] || "neutral";
}

function creditStatusText(credit: RechargeCreditDetail) {
  if (credit.refundId) return "已冲正";
  if (credit.status === "reversed") return credit.lostAmountMicroUsd > 0 ? "部分撤回" : "已撤回";
  return ({ available: "可用", depleted: "已耗尽", expired: "已过期", revoked: "已撤回", no_lot: "无可用额度包" } as Record<string, string>)[credit.lotStatus] || "有效";
}

function consumedCredit(credit: RechargeCreditDetail) {
  if (credit.status === "reversed") return credit.lostAmountMicroUsd;
  return Math.max(credit.creditAmountMicroUsd - credit.remainingAmountMicroUsd, 0);
}
</script>

<style scoped>
.recharge-orders-page { display: flex; flex-direction: column; gap: 20px; }
.filter-keyword { width: min(260px, 100%); }
.filter-select { width: 140px; }
.primary-cell, .amount-cell { display: flex; min-width: 0; flex-direction: column; gap: 4px; }
.secondary-text { color: var(--ds-faint); font-size: 12px; }
.mono { font-family: "SFMono-Regular", Consolas, "Liberation Mono", monospace; }
.truncate-id { display: block; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.reconciliation-cell { display: flex; min-width: 0; flex-direction: column; gap: 5px; }
.reference-line { display: flex; min-width: 0; align-items: center; gap: 7px; }
.reference-label { width: 30px; flex: none; color: var(--ds-faint); font-size: 11px; }
.copy-button { display: inline-grid; width: 24px; height: 24px; flex: none; place-items: center; border: 0; border-radius: var(--ds-radius-control); background: transparent; color: var(--ds-muted); cursor: pointer; }
.copy-button:hover { background: var(--ds-panel-muted); color: var(--ds-accent); }
.amount-cell { align-items: flex-end; font-variant-numeric: tabular-nums; }
.credited-amount { color: var(--ds-positive); font-weight: 600; }
.status-cell { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
.time-cell { font-size: 12px; white-space: nowrap; }
.row-actions { display: flex; align-items: center; justify-content: center; gap: 5px; }
.detail-loading { padding: 8px 0; }
.detail-content { display: flex; flex-direction: column; }
.detail-section { padding: 20px 0; border-bottom: 1px solid var(--ds-line); }
.detail-section:first-child { padding-top: 0; }
.detail-section:last-child { border-bottom: 0; }
.detail-section h3 { margin: 0 0 15px; color: var(--ds-ink); font-size: 14px; font-weight: 700; }
.detail-summary { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 20px; }
.detail-summary > div { display: flex; min-width: 0; flex-direction: column; align-items: flex-start; gap: 8px; }
.detail-label { color: var(--ds-muted); font-size: 12px; }
:deep(.detail-grid) { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px 28px; }
:deep(.detail-row) { display: grid; grid-template-columns: 108px minmax(0, 1fr); gap: 12px; }
:deep(.detail-row dt) { color: var(--ds-muted); font-size: 12px; }
:deep(.detail-row dd) { display: flex; min-width: 0; align-items: flex-start; gap: 5px; margin: 0; overflow-wrap: anywhere; color: var(--ds-ink); font-size: 13px; }
:deep(.detail-value--strong) { color: var(--ds-positive) !important; font-weight: 700; }
:deep(.detail-value--danger) { color: var(--ds-danger) !important; }
.credit-list { display: flex; grid-column: 1 / -1; flex-direction: column; gap: 14px; }
.credit-item { padding: 14px 0; border-top: 1px solid var(--ds-line); }
.credit-item:first-child { padding-top: 0; border-top: 0; }
.credit-item__head { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.credit-item__head > div { display: flex; min-width: 0; flex-direction: column; gap: 4px; }
.credit-metrics { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; margin: 14px 0 0; }
.credit-metrics div { display: flex; flex-direction: column; gap: 3px; }
.credit-metrics dt { color: var(--ds-muted); font-size: 11px; }
.credit-metrics dd { margin: 0; font-size: 13px; font-variant-numeric: tabular-nums; }
.credit-metrics--refund { grid-template-columns: repeat(2, minmax(0, 1fr)); }
.debt-value { color: var(--ds-danger); font-weight: 700; }
.credit-lot { margin-top: 10px; font-size: 12px; overflow-wrap: anywhere; }
.empty-detail { grid-column: 1 / -1; margin: 0; color: var(--ds-faint); font-size: 13px; }
.reverse-content { display: flex; flex-direction: column; gap: 18px; }
.reverse-warning { padding: 12px 14px; border: 1px solid var(--ds-warning); border-radius: var(--ds-radius-control); background: var(--ds-warning-soft); color: var(--ds-ink); font-size: 13px; line-height: 1.6; }
.reverse-summary { display: grid; grid-template-columns: 1fr 1fr; gap: 12px 20px; margin: 0; }
.reverse-summary div { min-width: 0; }
.reverse-summary dt { margin-bottom: 4px; color: var(--ds-muted); font-size: 12px; }
.reverse-summary dd { margin: 0; overflow-wrap: anywhere; color: var(--ds-ink); font-size: 13px; }
.reverse-summary .reclaim-value { color: var(--ds-danger); font-weight: 700; }
.refund-content { display: flex; flex-direction: column; gap: 18px; }
.refund-warning { padding: 12px 14px; border: 1px solid var(--ds-danger); border-radius: var(--ds-radius-control); background: var(--ds-danger-soft); color: var(--ds-ink); font-size: 13px; line-height: 1.6; }
.refund-order-summary { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px 20px; margin: 0; }
.refund-order-summary div { min-width: 0; }
.refund-order-summary dt { margin-bottom: 4px; color: var(--ds-muted); font-size: 12px; }
.refund-order-summary dd { margin: 0; overflow-wrap: anywhere; color: var(--ds-ink); font-size: 13px; }
.refund-impact-list { display: flex; flex-direction: column; border-block: 1px solid var(--ds-line); }
.refund-impact-row { display: flex; align-items: center; justify-content: space-between; gap: 20px; padding: 12px 0; border-top: 1px solid var(--ds-line); font-size: 13px; }
.refund-impact-row:first-child { border-top: 0; }
.refund-impact-row > div { display: flex; min-width: 0; flex-direction: column; gap: 3px; }
.refund-form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 16px; }
.refund-form-grid :deep(.el-date-editor), .refund-form-grid :deep(.el-segmented) { width: 100%; }
@media (max-width: 720px) {
  .detail-summary, :deep(.detail-grid), .credit-metrics, .reverse-summary, .refund-order-summary, .refund-form-grid { grid-template-columns: 1fr; }
  :deep(.detail-row) { grid-template-columns: 1fr; gap: 4px; }
  .refund-impact-row { align-items: flex-start; flex-direction: column; gap: 5px; }
}
</style>

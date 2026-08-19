<!--
  我的 USD 账户 — 查看额度与有效期额度包。
  重构：迁移至新设计系统一体面板（PortalPagePanel:图标徽章+面包屑标题+描述同行,
       刷新按钮收进 #actions);指标条 PortalMetricGrid → DsMetricCard,
       el-tag → DsTag,空态 → DsEmpty;额度包卡片/进度条保留,业务逻辑与请求不变。
-->
<template>
  <div class="page-container account-page">
    <PortalPagePanel
      fill
      :icon="Wallet"
      :breadcrumbs="[{ label: '用户中心' }, { label: 'USD 账户' }, { label: '我的余额' }]"
      description="查看统一 USD 额度和有效期额度包"
    >
      <template #actions>
        <el-button type="primary" :icon="Refresh" :loading="loading" @click="fetchData">立即刷新</el-button>
      </template>

      <div class="account-body">
        <div class="account-metrics">
          <DsMetricCard
            label="额度余额"
            :value="loading ? '—' : formatUSD(stats.remainingUsd)"
            hint="所有额度包剩余总和"
          />
          <DsMetricCard
            label="可用额度"
            :value="loading ? '—' : formatUSD(stats.availableUsd)"
            hint="当前可直接使用的额度"
          />
          <DsMetricCard
            label="当前透支"
            :value="loading ? '—' : formatUSD(stats.outstandingDebtUsd)"
            hint="已完成请求的结算尾差，后续充值优先清欠"
          />
        </div>

        <section class="account-packages">
          <header class="account-packages__head">
            <h2 class="account-packages__title">额度包有效期</h2>
            <p class="account-packages__desc">共 {{ packages.length }} 个额度包</p>
          </header>

          <div v-if="pkgLoading" class="flex items-center justify-center py-16">
            <el-icon class="account-spinner animate-spin" :size="36"><Loading /></el-icon>
          </div>

          <DsEmpty v-else-if="packages.length === 0" title="暂无额度包" description="充值或等待管理员发放后即可在此查看" />

          <div v-else class="package-grid">
            <div
              v-for="pkg in packages"
              :key="pkg.balanceLotId"
              class="package-card"
              :class="{ 'package-card--active': pkg.status === 1 }"
            >
              <div class="flex items-start justify-between mb-3">
                <div class="flex items-center gap-2">
                  <DsTag :tone="pkg.status === 1 ? 'positive' : 'neutral'">
                    {{ pkg.status === 1 ? '可用' : pkg.status === 2 ? '已过期' : '已耗尽' }}
                  </DsTag>
                  <span class="package-card__id">{{ pkg.balanceLotId }}</span>
                </div>
              </div>

              <div class="space-y-2">
                <div class="flex items-center justify-between">
                  <span class="package-card__label">剩余额度</span>
                  <span class="package-card__value">{{ formatUSD(pkg.remainingUsd) }}</span>
                </div>
                <div class="flex items-center justify-between">
                  <span class="package-card__label">额度总量</span>
                  <span class="package-card__value-sm">{{ formatUSD(pkg.totalUsd) }}</span>
                </div>
                <div class="flex items-center justify-between">
                  <span class="package-card__label">过期时间</span>
                  <span class="package-card__meta">{{ pkg.expiresAt ? formatDate(pkg.expiresAt) : '永久有效' }}</span>
                </div>
                <div class="flex items-center justify-between">
                  <span class="package-card__label">来源</span>
                  <span class="package-card__meta">{{ pkg.source || '管理员充值' }}</span>
                </div>
              </div>

              <!-- 进度条 -->
              <div class="mt-3">
                <el-progress
                  :percentage="pkg.totalUsd > 0 ? Math.round((pkg.remainingUsd / pkg.totalUsd) * 100) : 0"
                  :stroke-width="6"
                  :show-text="false"
                  :color="pkg.status === 1 ? 'var(--ds-accent)' : 'var(--ds-faint)'"
                />
              </div>
            </div>
          </div>
        </section>
      </div>
    </PortalPagePanel>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from "vue";
import { Refresh, Loading } from "@element-plus/icons-vue";
import { Wallet } from "lucide-vue-next";
import { PortalPagePanel } from "@/platform";
import { DsEmpty, DsMetricCard, DsTag } from "@/shared/ui";
import { platformCustomerApi } from "@/api/platformCustomer";
import type { BalanceLotView } from "@/api/types/platformCustomer";

const loading = ref(false);
const pkgLoading = ref(false);

const stats = reactive({
  remainingUsd: 0,
  availableUsd: 0,
  outstandingDebtUsd: 0
});

const packages = ref<BalanceLotView[]>([]);
function formatUSD(value: number) { return `$${Number(value ?? 0).toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 6 })}`; }

function formatDate(ts?: string | number | null) {
  if (!ts) return "—";
  return new Date(ts).toLocaleDateString("zh-CN");
}

async function fetchData() {
  loading.value = true;
  pkgLoading.value = true;
  try {
    const data = await platformCustomerApi.getBalance(true);
    if (data) {
      stats.remainingUsd = data.remainingUsd ?? 0;
      stats.availableUsd = data.availableUsd ?? 0;
      stats.outstandingDebtUsd = Number(data.outstandingDebtMicroUsd ?? 0) / 1_000_000;
      if (Array.isArray(data.balanceLots)) {
        packages.value = data.balanceLots.map((pkg) => {
          const expiredMs = pkg.expiresAt ? new Date(pkg.expiresAt).getTime() : 0;
          return {
            balanceLotId: pkg.balanceLotId,
            totalUsd: pkg.totalUsd,
            remainingUsd: pkg.remainingUsd,
            createdAt: pkg.createdAt,
            expiresAt: pkg.expiresAt,
            source: pkg.source || "充值",
            status: expiredMs && expiredMs < Date.now() ? 2 : pkg.remainingUsd <= 0 ? 3 : 1
          };
        });
      } else {
        packages.value = [];
      }
    }
  } catch (e) {
    console.error("获取余额失败:", e);
  } finally {
    loading.value = false;
    pkgLoading.value = false;
  }
}

onMounted(() => {
  fetchData();
});
</script>

<style scoped>
.account-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

/* PortalPagePanel body 无内边距,用 24px 容器排布指标卡与额度包分区;fill 模式下随面板撑满 */
.account-body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 24px;
  padding: 24px;
}

.account-metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 16px;
}

.account-packages {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding-top: 24px;
  border-top: 1px solid var(--ds-line);
}

.account-packages__head {
  display: flex;
  align-items: baseline;
  gap: 12px;
}

.account-packages__title {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--ds-ink);
}

.account-packages__desc {
  margin: 0;
  font-size: 12.5px;
  color: var(--ds-faint);
}

.animate-spin {
  animation: spin 1s linear infinite;
}
@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

.account-spinner {
  color: var(--ds-faint);
}

.package-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 16px;
}

.package-card {
  padding: 16px;
  border-radius: var(--ds-radius-control);
  border: 1px solid var(--ds-line);
  background: var(--ds-panel-muted);
  transition: border-color 0.15s ease, background 0.15s ease;
}

.package-card--active {
  border-color: color-mix(in srgb, var(--ds-accent) 35%, var(--ds-line));
  background: var(--ds-accent-soft);
}

.package-card__id {
  color: var(--ds-faint);
  font-size: 12px;
}

.package-card__label {
  color: var(--ds-muted);
  font-size: 12px;
}

.package-card__value {
  color: var(--ds-ink);
  font-size: 18px;
  font-weight: 700;
}

.package-card__value-sm {
  color: var(--ds-ink-soft);
  font-size: 14px;
  font-weight: 600;
}

.package-card__meta {
  color: var(--ds-ink-soft);
  font-size: 12px;
}
</style>

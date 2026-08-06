<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { SlidersHorizontal } from "lucide-vue-next";
import { useRoute } from "vue-router";

import { platformAdminApi } from "@/api/platformAdmin";
import type { TenantDetailOutput } from "@/api/types/admin";
import { PortalPagePanel } from "@/platform";
import { DsTag } from "@/shared/ui";
import AdminTenantPolicyPanel from "@/views/admin/ai/gateway/tenant-management/components/AdminTenantPolicyPanel.vue";

const route = useRoute();
const tenantId = String(route.params.id || "");
const tenant = ref<TenantDetailOutput | null>(null);
const loading = ref(false);

const description = computed(() =>
  tenant.value
    ? `租户 ID：${tenant.value.tenantId} · 维护平台容量保护、上游访问范围和结算倍率。`
    : `租户 ID：${tenantId}`
);

async function loadTenant() {
  loading.value = true;
  try {
    tenant.value = await platformAdminApi.getTenant(tenantId);
  } catch (error: any) {
    ElMessage.error(error?.message || "加载租户信息失败");
  } finally {
    loading.value = false;
  }
}

onMounted(loadTenant);
</script>

<template>
  <div class="tenant-policy-page">
    <PortalPagePanel
      :icon="SlidersHorizontal"
      :breadcrumbs="[
        { label: '用户中心' },
        { label: '业务管理' },
        { label: '租户管理', to: '/admin/organization/tenants' },
        { label: '平台策略' }
      ]"
      :description="description"
    >
      <template #actions>
        <DsTag v-if="tenant" :tone="tenant.status === 1 ? 'positive' : 'danger'">
          {{ tenant.statusDisplay }}
        </DsTag>
      </template>

      <div v-loading="loading" class="tenant-policy-page__body">
        <AdminTenantPolicyPanel :tenant="tenant" />
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
.tenant-policy-page {
  display: flex;
  flex: 1;
  min-width: 0;
  flex-direction: column;
}

.tenant-policy-page__body {
  min-height: 420px;
  padding: 24px;
}
</style>

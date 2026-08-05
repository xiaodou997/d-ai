<script setup lang="ts">
import { ref } from "vue";
import { Megaphone } from "lucide-vue-next";
import {
  createAnnouncementManagementClient,
  PortalAnnouncementManagementWorkspace
} from "@dai/app-core/announcements";

import { urmAdminApi } from "../../../api/urmAdmin";
import { authenticatedRequest, portalHeaders, serviceBaseUrl } from "../../../api/request";

const client = createAnnouncementManagementClient({
  request: authenticatedRequest("urm"),
  baseUrl: serviceBaseUrl("urm"),
  headers: portalHeaders,
  basePath: "/api/v1/admin/announcements"
});

async function loadTenants(keyword: string) {
  const page = await urmAdminApi.listTenants({ keyword: keyword || undefined, page: 1, size: 30 });
  return page.items.map((tenant) => ({ tenantId: tenant.tenantId, tenantName: tenant.tenantName }));
}

const workspace = ref<InstanceType<typeof PortalAnnouncementManagementWorkspace>>();

function openCreate() {
  workspace.value?.openCreate();
}
</script>

<template>
  <PortalAnnouncementManagementWorkspace
    ref="workspace"
    mode="platform"
    :client="client"
    :load-tenants="loadTenants"
    :icon="Megaphone"
    :breadcrumbs="[{ label: '用户中心' }, { label: '业务管理' }, { label: '公告管理' }]"
    description="发布平台公告并控制通知范围。"
  >
    <template #actions>
      <el-button type="primary" @click="openCreate">新建公告</el-button>
    </template>
  </PortalAnnouncementManagementWorkspace>
</template>

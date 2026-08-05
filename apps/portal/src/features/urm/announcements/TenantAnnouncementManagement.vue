<script setup lang="ts">
import { ref } from "vue";
import { Megaphone } from "lucide-vue-next";
import {
  createAnnouncementManagementClient,
  PortalAnnouncementManagementWorkspace
} from "@dai/app-core/announcements";

import { authenticatedRequest, portalHeaders, serviceBaseUrl } from "../../../api/request";

const client = createAnnouncementManagementClient({
  request: authenticatedRequest("urm"),
  baseUrl: serviceBaseUrl("urm"),
  headers: portalHeaders,
  basePath: "/api/v1/tenants/me/announcements"
});

const workspace = ref<InstanceType<typeof PortalAnnouncementManagementWorkspace>>();

function openCreate() {
  workspace.value?.openCreate();
}
</script>

<template>
  <PortalAnnouncementManagementWorkspace
    ref="workspace"
    mode="tenant"
    :client="client"
    :icon="Megaphone"
    :breadcrumbs="[{ label: '租户运营' }, { label: '用户运营' }, { label: '公告管理' }]"
    description="向本租户的终端用户发布公告。"
  >
    <template #actions>
      <el-button type="primary" @click="openCreate">新建公告</el-button>
    </template>
  </PortalAnnouncementManagementWorkspace>
</template>

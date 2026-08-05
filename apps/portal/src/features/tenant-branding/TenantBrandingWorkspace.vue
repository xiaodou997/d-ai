<!--
  用户门户品牌(租户设置) — 维护租户名称、用户门户网站名称与小图标。
  重构:PortalPageHeader → PortalPagePanel 一体面板(图标徽章+面包屑标题+描述同行,fill 链撑满短页),
       两张 PortalContentCard 置于同卡 body 内 24px 容器;取色/上传控件仍为 element-plus,业务逻辑不变。
-->
<script setup lang="ts">
import { ElMessage } from "element-plus";
import { computed } from "vue";
import { Palette } from "lucide-vue-next";
import { PortalContentCard, PortalPagePanel, resolvePortalResourceUrl } from "@/platform";

import { portalEnv } from "@/env";
import { useAuthStore } from "@/stores/auth";
import TenantBrandIconControl from "./TenantBrandIconControl.vue";
import TenantBrandNameForm from "./TenantBrandNameForm.vue";
import { useTenantBranding } from "./useTenantBranding";

const authStore = useAuthStore();
const {
  form,
  loading,
  saving,
  uploadingFavicon,
  faviconPath,
  saveSettings,
  uploadFavicon,
  deleteFavicon
} = useTenantBranding();

const faviconUrl = computed(() => resolvePortalResourceUrl(portalEnv.urmBaseUrl, faviconPath.value));
const effectiveSiteName = computed(() => form.customerSiteName.trim() || form.tenantName || "用户平台");

async function handleSave() {
  try {
    await saveSettings();
    await authStore.fetchUserInfo();
    ElMessage.success("租户和网站名称已保存");
  } catch (error) {
    ElMessage.error(messageFromError(error, "保存失败"));
  }
}

async function handleFaviconSelected(file: File) {
  try {
    await uploadFavicon(file);
    ElMessage.success("用户门户小图标已更新");
  } catch (error) {
    ElMessage.error(messageFromError(error, "上传失败"));
  }
}

async function handleFaviconRemoved() {
  try {
    await deleteFavicon();
    ElMessage.success("已恢复默认小图标");
  } catch (error) {
    ElMessage.error(messageFromError(error, "恢复默认失败"));
  }
}

function messageFromError(error: unknown, fallback: string) {
  if (error && typeof error === "object" && "detail" in error && typeof error.detail === "string") {
    return error.detail;
  }
  return error instanceof Error ? error.message : fallback;
}
</script>

<template>
  <div v-loading="loading" class="tenant-branding-workspace">
    <PortalPagePanel
      :icon="Palette"
      :breadcrumbs="[{ label: '租户运营' }, { label: '用户运营' }, { label: '用户门户品牌' }]"
      description="维护租户名称，并配置终端用户看到的网站名称和小图标。"
      fill
    >
      <div class="tenant-branding-workspace__body">
        <PortalContentCard title="名称设置" description="网站名称留空时，用户门户会自动使用租户名称。">
          <TenantBrandNameForm
            :tenant-name="form.tenantName"
            :customer-site-name="form.customerSiteName"
            :saving="saving"
            @update:tenant-name="form.tenantName = $event"
            @update:customer-site-name="form.customerSiteName = $event"
            @save="handleSave"
          />
        </PortalContentCard>

        <PortalContentCard title="门户标识" :description="`用户门户将显示“${effectiveSiteName}”。`">
          <TenantBrandIconControl
            :favicon-url="faviconUrl"
            :loading="uploadingFavicon"
            @choose="handleFaviconSelected"
            @remove="handleFaviconRemoved"
          />
        </PortalContentCard>
      </div>
    </PortalPagePanel>
  </div>
</template>

<style scoped>
/* fill 链:页面根 flex:1 + 面板 fill,短内容时面板撑满一屏 */
.tenant-branding-workspace {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 面板 body 无内边距,两张内容卡用 24px 容器排布 */
.tenant-branding-workspace__body {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 24px;
}
</style>

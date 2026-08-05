import { onMounted, reactive, shallowRef } from "vue";

import { tenantApi } from "../../api/tenant";
import type { TenantPortalBranding } from "../../types/tenant";
import { normalizeFaviconToPngDataUrl } from "./normalizeFavicon";

export function useTenantBranding() {
  const loading = shallowRef(false);
  const saving = shallowRef(false);
  const uploadingFavicon = shallowRef(false);
  const faviconPath = shallowRef("");
  const form = reactive({
    tenantName: "",
    customerSiteName: ""
  });

  function applyBranding(branding: TenantPortalBranding) {
    form.tenantName = branding.tenantName;
    form.customerSiteName = branding.customerSiteName;
    faviconPath.value = branding.faviconPath || "";
  }

  async function load() {
    loading.value = true;
    try {
      applyBranding(await tenantApi.getPortalBranding());
    } finally {
      loading.value = false;
    }
  }

  async function saveSettings() {
    saving.value = true;
    try {
      const branding = await tenantApi.updatePortalBranding({
        tenantName: form.tenantName.trim(),
        customerSiteName: form.customerSiteName.trim()
      });
      applyBranding(branding);
    } finally {
      saving.value = false;
    }
  }

  async function uploadFavicon(file: File) {
    uploadingFavicon.value = true;
    try {
      // 前端统一裁剪为正方形、缩放并转成 PNG，压到后端限制内再上传
      const dataUrl = await normalizeFaviconToPngDataUrl(file);
      applyBranding(await tenantApi.updatePortalFavicon(dataUrl));
    } finally {
      uploadingFavicon.value = false;
    }
  }

  async function deleteFavicon() {
    uploadingFavicon.value = true;
    try {
      applyBranding(await tenantApi.deletePortalFavicon());
    } finally {
      uploadingFavicon.value = false;
    }
  }

  onMounted(() => {
    void load();
  });

  return {
    form,
    loading,
    saving,
    uploadingFavicon,
    faviconPath,
    saveSettings,
    uploadFavicon,
    deleteFavicon
  };
}

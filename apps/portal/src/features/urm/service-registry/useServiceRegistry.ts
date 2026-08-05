import { computed, shallowRef } from "vue";
import { ElMessage } from "element-plus";

import { urmAdminApi } from "../../../api/urmAdmin";
import type { ServiceRegistryDetail, ServiceRegistryItem } from "../../../types/admin";

export function useServiceRegistry() {
  const services = shallowRef<ServiceRegistryItem[]>([]);
  const selected = shallowRef<ServiceRegistryDetail | null>(null);
  const loading = shallowRef(false);
  const detailLoading = shallowRef(false);
  const keyword = shallowRef("");
  let selectionVersion = 0;

  const filteredServices = computed(() => {
    const query = keyword.value.trim().toLowerCase();
    if (!query) return services.value;
    return services.value.filter((service) =>
      [service.serviceId, service.displayName, service.description]
        .filter(Boolean)
        .some((value) => value!.toLowerCase().includes(query))
    );
  });

  async function loadServices() {
    loading.value = true;
    try {
      services.value = await urmAdminApi.listServices();
    } catch (error) {
      ElMessage.error(error instanceof Error ? error.message : "服务列表加载失败");
    } finally {
      loading.value = false;
    }
  }

  async function selectService(service: ServiceRegistryItem) {
    const requestVersion = ++selectionVersion;
    selected.value = { ...service, sources: [], instances: [] };
    detailLoading.value = true;
    try {
      const detail = await urmAdminApi.getService(service.serviceId);
      if (requestVersion !== selectionVersion) return;
      selected.value = {
        ...service,
        ...detail,
        serviceId: detail.serviceId || service.serviceId,
        displayName: detail.displayName || service.displayName,
        status: detail.status === "active" || detail.status === "disabled" ? detail.status : service.status,
        portalEnabled: typeof detail.portalEnabled === "boolean" ? detail.portalEnabled : service.portalEnabled,
        sources: Array.isArray(detail.sources) ? detail.sources : [],
        instances: Array.isArray(detail.instances) ? detail.instances : []
      };
    } catch (error) {
      if (requestVersion === selectionVersion) {
        ElMessage.error(error instanceof Error ? error.message : "服务详情加载失败");
      }
    } finally {
      if (requestVersion === selectionVersion) detailLoading.value = false;
    }
  }

  async function refreshSelected() {
    if (selected.value) await selectService(selected.value);
  }

  function clearSelection() {
    selectionVersion++;
    selected.value = null;
    detailLoading.value = false;
  }

  return { services, selected, loading, detailLoading, keyword, filteredServices, loadServices, selectService, refreshSelected, clearSelection };
}

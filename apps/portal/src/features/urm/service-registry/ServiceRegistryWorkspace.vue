<script setup lang="ts">
import { computed, onMounted, reactive, shallowRef } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { Server } from "lucide-vue-next";
import { portalModuleForClientID } from "@/platform";

import { urmAdminApi } from "@/api/urmAdmin";
import { portalEnv } from "@/env";
import { useAuthStore } from "@/stores/auth";
import type { ServiceRegistryItem, ServiceSourceItem } from "@/api/types/admin";
import ServiceDetailsDrawer from "./ServiceDetailsDrawer.vue";
import ServiceList from "./ServiceList.vue";
import ServiceSourceEditor from "./ServiceSourceEditor.vue";
import { useServiceRegistry } from "./useServiceRegistry";

const registry = useServiceRegistry();
const authStore = useAuthStore();
const drawerOpen = shallowRef(false);
const serviceDialogOpen = shallowRef(false);
const sourceDialogOpen = shallowRef(false);
const saving = shallowRef(false);
const pendingAction = shallowRef<"status" | "portal" | "delete" | null>(null);
const editingService = shallowRef(false);
const editingSource = shallowRef<ServiceSourceItem | null>(null);
const serviceForm = reactive({ serviceId: "", displayName: "", description: "", portalEnabled: false });
const portalModuleLabels = computed(() => Object.fromEntries(
  registry.services.value.flatMap((service) => {
    const module = portalModuleForClientID(portalEnv, service.serviceId);
    return module ? [[service.serviceId, module.label]] : [];
  })
));
const selectedPortalModuleLabel = computed(() => {
  const serviceID = registry.selected.value?.serviceId;
  return serviceID ? portalModuleForClientID(portalEnv, serviceID)?.label : undefined;
});

function openService(service: ServiceRegistryItem) {
  drawerOpen.value = true;
  void registry.selectService(service);
}

function createService() {
  editingService.value = false;
  Object.assign(serviceForm, { serviceId: "", displayName: "", description: "", portalEnabled: false });
  serviceDialogOpen.value = true;
}

function editService() {
  const service = registry.selected.value;
  if (!service) return;
  editingService.value = true;
  Object.assign(serviceForm, {
    serviceId: service.serviceId || "",
    displayName: service.displayName || "",
    description: service.description || "",
    portalEnabled: service.portalEnabled
  });
  serviceDialogOpen.value = true;
}

async function saveService() {
  if (!serviceForm.serviceId.trim() || !serviceForm.displayName.trim()) return;
  const selectedService = registry.selected.value;
  const serviceID = serviceForm.serviceId.trim();
  if (!editingService.value && serviceForm.portalEnabled && !await confirmUnknownPortalModule(serviceID)) return;
  saving.value = true;
  try {
    if (editingService.value) {
      if (!selectedService) throw new Error("当前服务选择已失效，请重新打开服务详情");
      await urmAdminApi.updateService(serviceForm.serviceId, {
        displayName: serviceForm.displayName.trim(),
        description: serviceForm.description.trim(),
        status: selectedService.status,
        portalEnabled: selectedService.portalEnabled
      });
    } else {
      await urmAdminApi.createService({ serviceId: serviceID, displayName: serviceForm.displayName.trim(), description: serviceForm.description.trim(), portalEnabled: serviceForm.portalEnabled });
    }
    serviceDialogOpen.value = false;
    await refreshServiceState(editingService.value);
    ElMessage.success("服务信息已保存");
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "保存失败");
  } finally {
    saving.value = false;
  }
}

function isCancelled(error: unknown) {
  return error === "cancel" || error === "close";
}

function showActionError(error: unknown, fallback: string) {
  ElMessage.error(error instanceof Error ? error.message : fallback);
}

async function confirmUnknownPortalModule(serviceID: string): Promise<boolean> {
  if (portalModuleForClientID(portalEnv, serviceID)) return true;
  try {
    await ElMessageBox.confirm(
      "该 Service ID 尚未匹配当前门户前端模块。开启后会参与服务准入，但不会自动生成顶部业务 Tab；需要先发布对应前端模块。",
      "前端模块未接入",
      { type: "warning", confirmButtonText: "仍然开启" }
    );
    return true;
  } catch (error) {
    if (!isCancelled(error)) showActionError(error, "门户模块确认失败");
    return false;
  }
}

async function toggleServiceStatus() {
  const service = registry.selected.value;
  if (!service) return;
  const nextStatus = service.status === "active" ? "disabled" : "active";
  try {
    await ElMessageBox.confirm(
      nextStatus === "disabled" ? "停用后该服务无法注册或续签，已有服务令牌最多继续有效 5 分钟。" : "重新允许该服务注册与续签？",
      nextStatus === "disabled" ? "停用注册续签" : "启用注册续签",
      { type: "warning" }
    );
  } catch (error) {
    if (!isCancelled(error)) showActionError(error, "服务状态确认失败");
    return;
  }
  pendingAction.value = "status";
  try {
    await urmAdminApi.updateService(service.serviceId, {
      displayName: service.displayName,
      description: service.description,
      status: nextStatus,
      portalEnabled: service.portalEnabled
    });
    await refreshServiceState(true);
    ElMessage.success(nextStatus === "disabled" ? "注册续签已停用" : "注册续签已启用");
  } catch (error) {
    showActionError(error, nextStatus === "disabled" ? "停用失败" : "启用失败");
  } finally {
    pendingAction.value = null;
  }
}

async function togglePortal() {
  const service = registry.selected.value;
  if (!service) return;
  const portalEnabled = !service.portalEnabled;
  if (portalEnabled) {
    if (!await confirmUnknownPortalModule(service.serviceId)) return;
  } else {
    try {
      await ElMessageBox.confirm(
        "关闭后，该服务会从所有指定服务授权中移除。重新开启时不会自动恢复，需要重新向平台管理员或租户授权。",
        "关闭门户入口",
        { type: "warning", confirmButtonText: "确认关闭" }
      );
    } catch (error) {
      if (!isCancelled(error)) showActionError(error, "门户入口确认失败");
      return;
    }
  }
  pendingAction.value = "portal";
  try {
    await urmAdminApi.updateService(service.serviceId, {
      displayName: service.displayName,
      description: service.description,
      status: service.status,
      portalEnabled
    });
    await refreshServiceState(true);
    if (portalEnabled && service.status === "disabled") {
      ElMessage.success("门户入口已开启，将在服务启用后显示");
    } else {
      ElMessage.success(portalEnabled ? "门户入口已开启" : "门户入口已关闭");
    }
  } catch (error) {
    showActionError(error, portalEnabled ? "开启门户入口失败" : "关闭门户入口失败");
  } finally {
    pendingAction.value = null;
  }
}

async function deleteService() {
  const service = registry.selected.value;
  if (!service) return;
  try {
    await ElMessageBox.confirm(`永久删除服务 ${service.serviceId} 及其来源和实例记录？`, "删除服务", { type: "warning" });
  } catch (error) {
    if (!isCancelled(error)) showActionError(error, "删除确认失败");
    return;
  }
  pendingAction.value = "delete";
  try {
    await urmAdminApi.deleteService(service.serviceId);
    drawerOpen.value = false;
    registry.clearSelection();
    await refreshServiceState(false);
    ElMessage.success("服务已删除");
  } catch (error) {
    showActionError(error, "删除服务失败");
  } finally {
    pendingAction.value = null;
  }
}

async function refreshServiceState(includeSelected: boolean) {
  const tasks: Promise<unknown>[] = [registry.loadServices(), authStore.refreshCapabilities()];
  if (includeSelected) tasks.push(registry.refreshSelected());
  await Promise.all(tasks);
}

function addSource() {
  editingSource.value = null;
  sourceDialogOpen.value = true;
}

function editSource(source: ServiceSourceItem) {
  editingSource.value = source;
  sourceDialogOpen.value = true;
}

async function saveSource(value: { sourceCidr: string; description: string }) {
  const service = registry.selected.value;
  if (!service) return;
  saving.value = true;
  try {
    if (editingSource.value) await urmAdminApi.updateServiceSource(service.serviceId, editingSource.value.id, value);
    else await urmAdminApi.createServiceSource(service.serviceId, value);
    sourceDialogOpen.value = false;
    await Promise.all([registry.loadServices(), registry.refreshSelected()]);
    ElMessage.success("来源绑定已保存");
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "来源保存失败");
  } finally {
    saving.value = false;
  }
}

async function deleteSource(source: ServiceSourceItem) {
  const service = registry.selected.value;
  if (!service) return;
  await ElMessageBox.confirm(`删除来源 ${source.sourceCidr}？该网段上的实例将无法再次续签。`, "删除来源", { type: "warning" });
  await urmAdminApi.deleteServiceSource(service.serviceId, source.id);
  await Promise.all([registry.loadServices(), registry.refreshSelected()]);
  ElMessage.success("来源绑定已删除");
}

onMounted(() => {
  void registry.loadServices();
  void authStore.refreshCapabilities().catch((error) => showActionError(error, "门户权限刷新失败"));
});
</script>

<template>
  <div class="service-registry-page">
    <ServiceList
      v-model:keyword="registry.keyword.value"
      :icon="Server"
      :breadcrumbs="[{ label: '用户中心' }, { label: '服务治理' }, { label: '服务注册' }]"
      description="管理稳定服务身份、外部来源绑定与运行实例。"
      :services="registry.filteredServices.value"
      :loading="registry.loading.value"
      :portal-module-labels="portalModuleLabels"
      @select="openService"
      @create="createService"
      @refresh="registry.loadServices"
    />

    <ServiceDetailsDrawer
      v-model="drawerOpen"
      :service="registry.selected.value"
      :loading="registry.detailLoading.value"
      :pending-action="pendingAction"
      :portal-module-label="selectedPortalModuleLabel"
      @edit="editService"
      @toggle-status="toggleServiceStatus"
      @toggle-portal="togglePortal"
      @delete="deleteService"
      @add-source="addSource"
      @edit-source="editSource"
      @delete-source="deleteSource"
    />

    <ServiceSourceEditor v-model="sourceDialogOpen" :source="editingSource" :saving="saving" @save="saveSource" />

    <el-dialog v-model="serviceDialogOpen" :title="editingService ? '编辑服务' : '注册服务'" width="min(540px, 92vw)" append-to-body>
      <el-form label-position="top">
        <el-form-item label="Service ID" required><el-input v-model="serviceForm.serviceId" data-test="service-id-input" :disabled="editingService" placeholder="uni-example-service" /></el-form-item>
        <el-form-item label="显示名称" required><el-input v-model="serviceForm.displayName" data-test="display-name-input" /></el-form-item>
        <el-form-item label="说明"><el-input v-model="serviceForm.description" type="textarea" :rows="3" /></el-form-item>
        <el-form-item v-if="!editingService" label="门户业务服务">
          <div class="portal-service-toggle">
            <div><strong>显示为顶部业务 Tab</strong><span>关闭时仍保留内部服务身份与服务会话</span></div>
            <el-switch v-model="serviceForm.portalEnabled" />
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="serviceDialogOpen = false">取消</el-button>
        <el-button type="primary" :loading="saving" :disabled="!serviceForm.serviceId.trim() || !serviceForm.displayName.trim()" @click="saveService">保存服务</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.service-registry-page { display: flex; flex-direction: column; gap: 20px; }
.portal-service-toggle { width: 100%; display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.portal-service-toggle > div { display: grid; gap: 2px; }
.portal-service-toggle strong { font-size: 13px; }
.portal-service-toggle span { color: var(--ds-muted); font-size: 12px; }
</style>

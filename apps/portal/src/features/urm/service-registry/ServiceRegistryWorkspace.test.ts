import { computed, defineComponent, shallowRef } from "vue";
import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { urmAdminApi } from "../../../api/urmAdmin";
import { useAuthStore } from "../../../stores/auth";
import type { ServiceRegistryDetail } from "../../../types/admin";
import ServiceRegistryWorkspace from "./ServiceRegistryWorkspace.vue";
import { useServiceRegistry } from "./useServiceRegistry";

vi.mock("../../../api/urmAdmin", () => ({
  urmAdminApi: {
    createService: vi.fn(),
    updateService: vi.fn(),
    deleteService: vi.fn()
  }
}));

vi.mock("../../../stores/auth", () => ({ useAuthStore: vi.fn() }));
vi.mock("./useServiceRegistry", () => ({ useServiceRegistry: vi.fn() }));
vi.mock("@dai/app-core", () => ({
  // ServiceList 被 stub 但模块仍会被加载，需保留其依赖的 PortalPagePanel 导出
  PortalPagePanel: { template: `<div />` },
  createStandardPortalEnv: ({ portal }: { portal: string }) => ({
    portal,
    serviceClientIds: { urm: "urm", ai: "uni-ai-api", proxy: "uni-api-proxy" }
  }),
  portalModuleForClientID: (_env: unknown, clientID: string) => ({
    urm: { service: "urm", label: "用户中心" },
    "uni-ai-api": { service: "ai", label: "智能服务" },
    "uni-api-proxy": { service: "proxy", label: "接口代理" }
  })[clientID]
}));

const { confirm, messageSuccess, messageError } = vi.hoisted(() => ({
  confirm: vi.fn(),
  messageSuccess: vi.fn(),
  messageError: vi.fn()
}));
vi.mock("element-plus", () => ({
  ElMessage: { success: messageSuccess, error: messageError },
  ElMessageBox: { confirm }
}));

const DetailsStub = defineComponent({
  props: { portalModuleLabel: String },
  emits: ["edit", "toggle-status", "toggle-portal", "delete"],
  template: `
    <div>
      <span data-test="detail-module-label">{{ portalModuleLabel }}</span>
      <button data-test="edit" @click="$emit('edit')">edit</button>
      <button data-test="toggle-status" @click="$emit('toggle-status')">status</button>
      <button data-test="toggle-portal" @click="$emit('toggle-portal')">portal</button>
      <button data-test="delete" @click="$emit('delete')">delete</button>
    </div>
  `
});

const ListStub = defineComponent({
  props: { services: Array, portalModuleLabels: Object },
  emits: ["create"],
  template: `
    <div>
      <span data-test="list-module-label">{{ portalModuleLabels?.[services?.[0]?.serviceId] }}</span>
      <button data-test="create-service" @click="$emit('create')">create</button>
    </div>
  `
});

const DialogStub = defineComponent({
  props: { modelValue: Boolean, title: String },
  emits: ["update:modelValue"],
  template: `<div v-if="modelValue" data-test="service-dialog"><slot /><slot name="footer" /></div>`
});

const InputStub = defineComponent({
  inheritAttrs: false,
  props: { modelValue: String, disabled: Boolean },
  emits: ["update:modelValue"],
  template: `<input v-bind="$attrs" :value="modelValue" :disabled="disabled" @input="$emit('update:modelValue', $event.target.value)" />`
});

const ButtonStub = defineComponent({
  emits: ["click"],
  template: `<button data-test="dialog-button" @click="$emit('click')"><slot /></button>`
});

const SwitchStub = defineComponent({
  props: { modelValue: Boolean },
  emits: ["update:modelValue"],
  template: `<button data-test="portal-switch" @click="$emit('update:modelValue', !modelValue)">{{ modelValue }}</button>`
});

function serviceDetail(overrides: Partial<ServiceRegistryDetail> = {}): ServiceRegistryDetail {
  return {
    id: 1,
    serviceId: "uni-ai-api",
    displayName: "AI Gateway",
    description: "AI service",
    status: "active",
    portalEnabled: true,
    sourceCount: 0,
    instanceCount: 1,
    onlineInstances: 1,
    createdAt: "2026-07-11T00:00:00Z",
    updatedAt: "2026-07-11T00:00:00Z",
    sources: [],
    instances: [],
    ...overrides
  };
}

function setup(overrides: Partial<ServiceRegistryDetail> = {}) {
  const selected = shallowRef<ServiceRegistryDetail | null>(serviceDetail(overrides));
  const services = shallowRef(selected.value ? [selected.value] : []);
  const loadServices = vi.fn().mockResolvedValue(undefined);
  const refreshSelected = vi.fn().mockResolvedValue(undefined);
  const clearSelection = vi.fn(() => { selected.value = null; });
  vi.mocked(useServiceRegistry).mockReturnValue({
    services,
    selected,
    loading: shallowRef(false),
    detailLoading: shallowRef(false),
    keyword: shallowRef(""),
    filteredServices: computed(() => services.value),
    loadServices,
    selectService: vi.fn(),
    refreshSelected,
    clearSelection
  });

  const wrapper = mount(ServiceRegistryWorkspace, {
    global: {
      stubs: {
        ServiceList: ListStub,
        ServiceDetailsDrawer: DetailsStub,
        ServiceSourceEditor: true,
        "el-dialog": DialogStub,
        "el-form": { template: `<form><slot /></form>` },
        "el-form-item": { template: `<div><slot /></div>` },
        "el-input": InputStub,
        "el-switch": SwitchStub,
        "el-button": ButtonStub
      }
    }
  });

  return { wrapper, selected, loadServices, refreshSelected, clearSelection };
}

describe("ServiceRegistryWorkspace", () => {
  const refreshCapabilities = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    confirm.mockResolvedValue("confirm");
    refreshCapabilities.mockResolvedValue([]);
    vi.mocked(useAuthStore).mockReturnValue({ refreshCapabilities } as unknown as ReturnType<typeof useAuthStore>);
    vi.mocked(urmAdminApi.updateService).mockResolvedValue({ status: "updated" });
    vi.mocked(urmAdminApi.createService).mockResolvedValue(serviceDetail({ serviceId: "created" }));
    vi.mocked(urmAdminApi.deleteService).mockResolvedValue({ status: "deleted" });
  });

  it("refreshes capabilities when the service registry page opens", async () => {
    setup();
    await flushPromises();

    expect(refreshCapabilities).toHaveBeenCalledOnce();
  });

  it("passes frontend module support to the list and details", () => {
    const { wrapper } = setup();

    expect(wrapper.get('[data-test="list-module-label"]').text()).toBe("智能服务");
    expect(wrapper.get('[data-test="detail-module-label"]').text()).toBe("智能服务");
  });

  it("toggles the portal entry with the stable service id and refreshes capabilities", async () => {
    const { wrapper, loadServices, refreshSelected } = setup();
    refreshCapabilities.mockClear();

    await wrapper.get('[data-test="toggle-portal"]').trigger("click");
    await flushPromises();

    expect(confirm).toHaveBeenCalledWith(
      expect.stringContaining("不会自动恢复"),
      "关闭门户入口",
      expect.objectContaining({ type: "warning" })
    );
    expect(urmAdminApi.updateService).toHaveBeenCalledWith("uni-ai-api", {
      displayName: "AI Gateway",
      description: "AI service",
      status: "active",
      portalEnabled: false
    });
    expect(refreshCapabilities).toHaveBeenCalledOnce();
    expect(loadServices).toHaveBeenCalled();
    expect(refreshSelected).toHaveBeenCalledOnce();
  });

  it("opens edit with the selected values and preserves portal state on save", async () => {
    const { wrapper } = setup();

    await wrapper.get('[data-test="edit"]').trigger("click");

    expect(wrapper.find('[data-test="service-dialog"]').exists()).toBe(true);
    expect(wrapper.get('[data-test="service-id-input"]').attributes("value")).toBe("uni-ai-api");
    expect(wrapper.get('[data-test="display-name-input"]').attributes("value")).toBe("AI Gateway");

    const saveButton = wrapper.findAll('[data-test="dialog-button"]').find((button) => button.text() === "保存服务");
    expect(saveButton).toBeDefined();
    await saveButton!.trigger("click");
    await flushPromises();

    expect(urmAdminApi.updateService).toHaveBeenCalledWith("uni-ai-api", expect.objectContaining({
      status: "active",
      portalEnabled: true
    }));
  });

  it("uses separate service-status and delete operations without undefined ids", async () => {
    const { wrapper, clearSelection } = setup();

    await wrapper.get('[data-test="toggle-status"]').trigger("click");
    await flushPromises();
    expect(urmAdminApi.updateService).toHaveBeenCalledWith("uni-ai-api", expect.objectContaining({
      status: "disabled",
      portalEnabled: true
    }));

    await wrapper.get('[data-test="delete"]').trigger("click");
    await flushPromises();
    expect(confirm).toHaveBeenCalledWith(
      expect.stringContaining("永久删除服务 uni-ai-api"),
      "删除服务",
      expect.objectContaining({ type: "warning" })
    );
    expect(urmAdminApi.deleteService).toHaveBeenCalledWith("uni-ai-api");
    expect(clearSelection).toHaveBeenCalledOnce();
  });

  it("shows API failures instead of silently ignoring portal actions", async () => {
    vi.mocked(urmAdminApi.updateService).mockRejectedValueOnce(new Error("service access unavailable"));
    const { wrapper } = setup({ portalEnabled: false });

    await wrapper.get('[data-test="toggle-portal"]').trigger("click");
    await flushPromises();

    expect(messageError).toHaveBeenCalledWith("service access unavailable");
    expect(messageSuccess).not.toHaveBeenCalled();
  });

  it("enables a known portal module without an extra warning", async () => {
    const { wrapper } = setup({ portalEnabled: false });

    await wrapper.get('[data-test="toggle-portal"]').trigger("click");
    await flushPromises();

    expect(confirm).not.toHaveBeenCalled();
    expect(urmAdminApi.updateService).toHaveBeenCalledWith(
      "uni-ai-api",
      expect.objectContaining({ portalEnabled: true })
    );
  });

  it("warns before enabling a service without a frontend module", async () => {
    const { wrapper } = setup({ serviceId: "report-service", portalEnabled: false });

    await wrapper.get('[data-test="toggle-portal"]').trigger("click");
    await flushPromises();

    expect(confirm).toHaveBeenCalledWith(
      expect.stringContaining("不会自动生成顶部业务 Tab"),
      "前端模块未接入",
      expect.objectContaining({ confirmButtonText: "仍然开启" })
    );
    expect(urmAdminApi.updateService).toHaveBeenCalledWith(
      "report-service",
      expect.objectContaining({ portalEnabled: true })
    );
  });

  it("does not update an unknown service when the warning is cancelled", async () => {
    confirm.mockRejectedValueOnce("cancel");
    const { wrapper } = setup({ serviceId: "report-service", portalEnabled: false });

    await wrapper.get('[data-test="toggle-portal"]').trigger("click");
    await flushPromises();

    expect(urmAdminApi.updateService).not.toHaveBeenCalled();
  });

  it("warns before creating an enabled service without a frontend module", async () => {
    const { wrapper } = setup();

    await wrapper.get('[data-test="create-service"]').trigger("click");
    await wrapper.get('[data-test="service-id-input"]').setValue("report-service");
    await wrapper.get('[data-test="display-name-input"]').setValue("Report Service");
    await wrapper.get('[data-test="portal-switch"]').trigger("click");
    const saveButton = wrapper.findAll('[data-test="dialog-button"]').find((button) => button.text() === "保存服务");
    await saveButton!.trigger("click");
    await flushPromises();

    expect(confirm).toHaveBeenCalledWith(
      expect.stringContaining("不会自动生成顶部业务 Tab"),
      "前端模块未接入",
      expect.any(Object)
    );
    expect(urmAdminApi.createService).toHaveBeenCalledWith(expect.objectContaining({
      serviceId: "report-service",
      portalEnabled: true
    }));
  });
});

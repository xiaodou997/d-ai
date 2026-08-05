import { nextTick, shallowRef } from "vue";
import { describe, expect, it, vi } from "vitest";

vi.mock("element-plus", () => ({
  ElMessage: { error: vi.fn(), success: vi.fn(), warning: vi.fn() },
  ElMessageBox: { confirm: vi.fn(async () => undefined) }
}));
vi.mock("../../../../api/aiTenant", () => ({ aiTenantApi: {} }));

import type { TenantAiClientSurfacePolicy } from "../../../../types/aiTenant";
import { useGroupClientSurfacePolicy, type GroupClientSurfacePolicyApi } from "./useGroupClientSurfacePolicy";

function policy(groupId: string, mode: "all" | "restricted", allowed: TenantAiClientSurfacePolicy["allowed_surfaces"]): TenantAiClientSurfacePolicy {
  return { group_id: groupId, mode, allowed_surfaces: allowed };
}

describe("useGroupClientSurfacePolicy", () => {
  it("applies capability presets and derives dirty state", async () => {
    const api: GroupClientSurfacePolicyApi = {
      get: vi.fn(async () => policy("group-1", "all", [])),
      replace: vi.fn()
    };
    const state = useGroupClientSurfacePolicy({ groupId: () => "group-1", api });
    await state.load();

    state.applyPreset("image");

    expect(state.mode.value).toBe("restricted");
    expect(state.selectedSurfaces.value).toEqual(["openai_images", "gemini_images"]);
    expect(state.isDirty.value).toBe(true);
  });

  it("does not mark a saved policy dirty when the API returns surfaces in another order", async () => {
    const api: GroupClientSurfacePolicyApi = {
      get: vi.fn(async () => policy("group-1", "restricted", ["gemini_images", "openai_chat"])),
      replace: vi.fn(async () => policy("group-1", "restricted", ["openai_images", "openai_chat"]))
    };
    const state = useGroupClientSurfacePolicy({ groupId: () => "group-1", api });
    await state.load();

    expect(state.selectedSurfaces.value).toEqual(["openai_chat", "gemini_images"]);
    expect(state.isDirty.value).toBe(false);

    state.setSelectedSurfaces(["openai_chat", "openai_images"]);
    await state.save();

    expect(state.isDirty.value).toBe(false);
  });

  it("ignores an older policy response after the group changes", async () => {
    let resolveFirst!: (value: TenantAiClientSurfacePolicy) => void;
    const groupId = shallowRef("group-1");
    const api: GroupClientSurfacePolicyApi = {
      get: vi.fn()
        .mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve; }))
        .mockResolvedValueOnce(policy("group-2", "restricted", ["openai_images"])),
      replace: vi.fn()
    };
    const state = useGroupClientSurfacePolicy({ groupId: () => groupId.value, api });
    groupId.value = "group-2";
    await nextTick();
    await nextTick();
    resolveFirst(policy("group-1", "all", []));
    await nextTick();

    expect(state.mode.value).toBe("restricted");
    expect(state.selectedSurfaces.value).toEqual(["openai_images"]);
  });
});

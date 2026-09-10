import { flushPromises, mount } from "@vue/test-utils";
import { ElInput, ElInputNumber } from "element-plus";
import { beforeEach, describe, expect, it, vi } from "vitest";

import GroupTargetsWorkspace from "./GroupTargetsWorkspace.vue";

const api = vi.hoisted(() => ({
  listGroupTargets: vi.fn(), listUpstreamResources: vi.fn(), replaceGroupTargets: vi.fn()
}));
vi.mock("@/api/aiTenant", () => ({ aiTenantApi: api }));

const binding = { id: "binding-1", group_id: "group-1", account_id: "account-1", account_name: "Primary", target_type: "account", api_formats: [], status: "active", priority: 100 };

describe("GroupTargetsWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.listGroupTargets.mockResolvedValue({ items: [binding], total: 1, route_policy_version: 7 });
    api.listUpstreamResources.mockResolvedValue({ items: [], total: 0 });
    api.replaceGroupTargets.mockImplementation(async (_groupId, body) => ({
      items: [{ ...binding, priority: body.targets[0].priority }], total: 1, route_policy_version: 8
    }));
  });

  it("edits an integer priority and saves it automatically", async () => {
    const wrapper = mount(GroupTargetsWorkspace, {
      props: { groupId: "group-1" },
      global: {
        components: { ElInput, ElInputNumber },
        stubs: {
          DsTable: { props: ["rows"], template: "<div><slot v-for='row in rows' name='cell-priority' :row='row' /></div>" },
          ElButton: true, ElAlert: true, ElSelect: true, ElOption: true, ElCheckbox: true, ElSwitch: true
        }
      }
    });
    await flushPromises();
    const input = wrapper.get(".priority-input input");
    expect((input.element as HTMLInputElement).value).toBe("100");
    await input.setValue("10");
    await input.trigger("change");
    await flushPromises();
    expect(api.replaceGroupTargets).toHaveBeenCalledWith("group-1", {
      expected_version: 7, targets: [{ account_id: "account-1", status: "active", priority: 10 }]
    });
    expect((wrapper.get(".priority-input input").element as HTMLInputElement).value).toBe("10");
    expect(wrapper.emitted("changed")).toHaveLength(1);
    wrapper.unmount();
  });
});

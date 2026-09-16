import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import type { RequestRecord } from "../recordsApi";
import RecordTable from "./RecordTable.vue";

const record = {
  request_id: "request-1",
  created_at: "2026-09-15T10:22:27Z",
  tenant_id: "tenant-1",
  user_id: "user-1",
  model: "gpt-5.6-sol",
  source: "api_key",
  stream: true,
  delivery: "complete",
  first_token_ms: 6172,
  total_ms: 8673,
  tokens: { input: 120728, output: 133, cache_read: 119936, cache_write: 0 },
  profile: { tenant_name: "租户", user_name: "用户", group_name_snapshot: "高级分组", group_default_user_multiplier_snapshot: 0.3, api_key_name: "工作 Key" },
  admin_context: { upstream_account_name: "上游分组", provider_code: "provider", tenant_multiplier: 0.27 },
  charge: { state: "posted", source: "payg", tenant_charged_micro: 18338, user_charged_micro: 20375 }
} as unknown as RequestRecord;

function render(errors = false) {
  return mount(RecordTable, {
    props: { role: "admin", errors, rows: [record], busy: false },
    global: { stubs: { "el-button": { template: "<button><slot /></button>" } } }
  });
}

describe("record table presentation", () => {
  it("shows compact usage and the two distinct multiplier snapshots", () => {
    const wrapper = render();
    const text = wrapper.text();
    expect(text).toContain("高级分组× 0.3");
    expect(wrapper.get(".record-key").text()).toBe("工作 Key");
    expect(text).toContain("上游分组× 0.27");
    expect(text).toContain("输入 120.7K");
    expect(text).toContain("输出 133");
    expect(text).toContain("缓存读 119.9K");
    expect(text).not.toContain("缓存写");
    expect(text).toContain("首：6,172ms");
    expect(text).toContain("总：8,673ms");
    expect(text).not.toContain("已结算");
    expect(text).not.toContain("复制 ID");
  });

  it("removes meaningless usage, timing and fee columns from errors", () => {
    const text = render(true).text();
    expect(text).toContain("错误说明");
    expect(text).not.toContain("用量");
    expect(text).not.toContain("耗时");
    expect(text).not.toContain("费用");
  });
});

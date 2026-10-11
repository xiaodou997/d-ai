import { defineComponent, h, type Plugin } from "vue";
import { describe, expect, it } from "vitest";
import { ElEmpty } from "element-plus";

import { bootstrapStandardPortalApp } from "./bootstrap";

const testRouter: Plugin = {
  install(app) {
    app.component("RouterView", defineComponent({ render: () => h(ElEmpty) }));
  }
};

describe("bootstrapStandardPortalApp", () => {
  it("provides the Chinese locale to locally imported components", () => {
    const mountTarget = document.createElement("div");
    mountTarget.id = "bootstrap-test-app";
    document.body.append(mountTarget);

    const app = bootstrapStandardPortalApp({
      mountSelector: "#bootstrap-test-app",
      router: testRouter
    });

    expect(mountTarget.textContent).toContain("暂无数据");

    app.unmount();
    mountTarget.remove();
  });
});

import { defineComponent, h, type Plugin } from "vue";
import { describe, expect, it } from "vitest";

import { bootstrapPortalApp } from "./bootstrap";

const noopRouter: Plugin = {
  install() {}
};

describe("bootstrapPortalApp", () => {
  it("registers the shared Element Plus component set globally", () => {
    const mountTarget = document.createElement("div");
    mountTarget.id = "bootstrap-test-app";
    document.body.append(mountTarget);

    const app = bootstrapPortalApp({
      mountSelector: "#bootstrap-test-app",
      root: defineComponent({ render: () => h("div") }),
      router: noopRouter
    });

    expect(app.component("ElButton")).toBeTruthy();
    expect(app.component("ElDialog")).toBeTruthy();
    expect(app.component("ElDrawer")).toBeTruthy();
    expect(app.component("ElInput")).toBeTruthy();

    app.unmount();
    mountTarget.remove();
  });
});

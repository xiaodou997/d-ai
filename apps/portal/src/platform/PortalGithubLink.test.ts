import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import PortalGithubLink from "./PortalGithubLink.vue";

describe("PortalGithubLink", () => {
  it("links to the public D-AI GitHub repository in a new tab", () => {
    const link = mount(PortalGithubLink).get("a");

    expect(link.attributes("href")).toBe("https://github.com/xiaodou997/d-ai");
    expect(link.attributes("target")).toBe("_blank");
    expect(link.attributes("rel")).toBe("noopener noreferrer");
    expect(link.attributes("aria-label")).toBe("访问 D-AI GitHub 仓库");
    expect(link.find("svg").exists()).toBe(true);
  });
});

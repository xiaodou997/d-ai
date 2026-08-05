import { effectScope } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

import { usePortalImageReferences } from "./usePortalImageReferences";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("usePortalImageReferences", () => {
  it("keeps submission order and releases every object URL", () => {
    const createObjectURL = vi
      .spyOn(URL, "createObjectURL")
      .mockImplementation((file) => `blob:${(file as File).name}`);
    const revokeObjectURL = vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => undefined);
    const scope = effectScope();
    const references = scope.run(() => usePortalImageReferences());
    if (!references) throw new Error("reference state was not created");

    const first = new File(["first"], "first.png", { type: "image/png" });
    const second = new File(["second"], "second.png", { type: "image/png" });
    const mask = new File(["mask"], "mask.png", { type: "image/png" });

    expect(references.addReferences([first, second])).toEqual({ added: 2, rejected: 0 });
    expect(references.references.value.map((item) => item.file)).toEqual([first, second]);

    const secondId = references.references.value[1].id;
    expect(references.moveReference(secondId, -1)).toEqual({ from: 1, to: 0 });
    expect(references.references.value.map((item) => item.file)).toEqual([second, first]);

    const removedId = references.references.value[1].id;
    expect(references.removeReference(removedId)).toBe(1);
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:first.png");

    references.setMask(mask);
    scope.stop();

    expect(createObjectURL).toHaveBeenCalledTimes(3);
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:second.png");
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:mask.png");
  });

  it("rejects non-image files", () => {
    vi.spyOn(URL, "createObjectURL").mockImplementation(() => "blob:image");
    const scope = effectScope();
    const references = scope.run(() => usePortalImageReferences());
    if (!references) throw new Error("reference state was not created");

    const text = new File(["text"], "notes.txt", { type: "text/plain" });
    expect(references.addReferences([text])).toEqual({ added: 0, rejected: 1 });
    expect(references.references.value).toEqual([]);

    scope.stop();
  });
});

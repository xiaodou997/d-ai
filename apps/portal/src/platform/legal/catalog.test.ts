import { describe, expect, it } from "vitest";

import { findLegalDocument, legalDocumentURL, legalDocuments } from "./catalog";

describe("legal catalog", () => {
  it("contains the documents linked from every portal footer", () => {
    expect(legalDocuments.map((document) => document.id)).toEqual([
      "privacy",
      "terms",
      "cookies",
      "acceptable-use",
    ]);
  });

  it("builds canonical document URLs without double slashes", () => {
    expect(
      legalDocumentURL("https://legal.example.com/legal/", "privacy"),
    ).toBe("https://legal.example.com/legal/privacy");
  });

  it("rejects unknown document IDs", () => {
    expect(findLegalDocument("unknown")).toBeUndefined();
  });
});

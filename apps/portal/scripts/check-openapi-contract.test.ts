import { describe, expect, it } from "vitest";
import { parse } from "yaml";

import { checkContract, operationIdsFromGenerated, operationIdsFromSpec } from "./check-openapi-contract.mjs";

const spec = `openapi: 3.1.0
paths:
  /health:
    get:
      operationId: health
  /users:
    post:
      operationId: create-user
`;
const generated = `export interface operations {
    "health": { responses: {} };
    "create-user": { responses: {} };
}`;

describe("OpenAPI operation contract gate", () => {
  it("extracts and matches operation IDs", () => {
    expect(operationIdsFromSpec(parse(spec))).toEqual(["health", "create-user"]);
    expect(operationIdsFromGenerated(generated)).toEqual(["health", "create-user"]);
    expect(checkContract({ specSource: spec, generatedSource: generated, marker: "hash" }).operationCount).toBe(2);
  });

  it("rejects generated drift and removed baseline operations", () => {
    expect(() => checkContract({ specSource: spec, generatedSource: generated.replace('"health"', '"stale"'), marker: "hash" })).toThrow(/differ/);
    expect(() => checkContract({ specSource: spec, generatedSource: generated, marker: "hash", baselineSource: spec.replace("operationId: health", "operationId: old-health") })).toThrow(/removed/);
  });
});

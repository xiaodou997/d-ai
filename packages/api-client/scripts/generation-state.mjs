import { createHash } from "node:crypto";

export const OPENAPI_TYPESCRIPT_VERSION = "7.10.1";

export function generationHash(inputs) {
  const hash = createHash("sha256");
  hash.update(`openapi-typescript@${OPENAPI_TYPESCRIPT_VERSION}\0`);
  for (const input of [...inputs].sort((left, right) => left.name.localeCompare(right.name))) {
    hash.update(input.name);
    hash.update("\0");
    hash.update(input.content);
    hash.update("\0");
  }
  return hash.digest("hex");
}

export function generationIsCurrent({ expectedHash, marker, outputsExist }) {
  return outputsExist && marker === expectedHash;
}

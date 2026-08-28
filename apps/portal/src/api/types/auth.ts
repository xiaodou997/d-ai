import type { components } from "../generated/dai";

export type ChangePasswordPayload = components["schemas"]["ChangePasswordInputBody"];
export type UpdateProfilePayload = components["schemas"]["UpdateProfileInputBody"];

/** Page-friendly profile input; the facade converts omitted fields to transport nulls. */
export interface ProfileUpdateInput {
  username?: string;
  email?: string;
}

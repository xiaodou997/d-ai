import { createStandardPortalEnv, type PortalEnv } from "@/platform/env";
import type { PortalThemeName } from "@/shared/ui";

export type { PortalEnv } from "@/platform/env";

const standardPortalEnv = createStandardPortalEnv({
  env: import.meta.env as unknown as Record<string, unknown>
});

export const portalEnv: PortalEnv = standardPortalEnv;

/**
 * The shell stays single, while its accent follows the authenticated user type.
 * 1/2 = admin, 3 = tenant, 4 = customer.
 */
export function themeForUserType(userType: number | undefined): PortalThemeName {
  if (userType === undefined || userType <= 2) return "admin";
  if (userType === 3) return "tenant";
  return "customer";
}

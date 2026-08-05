import { createStandardPortalAuthStore } from "@/platform/portal-auth";

import { portalEnv } from "../env";

/** One auth store for the unified shell; userType only changes the visible surface. */
export const useAuthStore = createStandardPortalAuthStore({
  env: portalEnv,
  storeId: "unified-auth",
  expectedUserTypes: [1, 2, 3, 4]
});

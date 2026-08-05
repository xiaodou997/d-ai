import { createPortalRequestBindings } from "@/platform";
export type { BackendService } from "@/platform";

import { portalEnv } from "@/env";
import { useAuthStore } from "@/stores/auth";

export const { portalHeaders, portalHeadersFor, serviceBaseUrl, authenticatedRequest } =
  createPortalRequestBindings({ env: portalEnv, useAuthStore });

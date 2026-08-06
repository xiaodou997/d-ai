import { createPortalRequestBindings } from "@/platform";
import { portalEnv } from "@/env";
import { useAuthStore } from "@/stores/auth";

export const { apiHeaders, apiBaseUrl, authenticatedRequest } =
  createPortalRequestBindings({ env: portalEnv, useAuthStore });

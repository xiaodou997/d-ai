import { onMounted, watch } from "vue";

import type { PortalThemeName } from "@/shared/ui";

import type { AppShellNavItem } from "./shell-nav";

export interface PortalShellUser {
  name: string;
  subtitle?: string;
}

export interface PortalShellUserMenuItem {
  id: string;
  label: string;
  to: string;
}

export interface PortalShellAuthLike {
  accessToken: string;
  userInfo?: unknown;
  userType?: number;
  init: () => void;
  fetchUserInfo: () => Promise<unknown>;
  logout: () => Promise<boolean>;
}

export interface PortalShellMenuStoreLike {
  items: AppShellNavItem[];
  refresh: (currentPath?: string) => void;
}

export interface PortalShellLayoutProps {
  theme: PortalThemeName;
  appVersion: string;
  brand?: string;
  brandIconUrl?: string;
  nav: AppShellNavItem[];
  user: PortalShellUser;
  userMenu?: PortalShellUserMenuItem[];
  legalBaseUrl: string;
}

export interface UsePortalShellScaffoldOptions {
  authStore: PortalShellAuthLike;
  menuStore: PortalShellMenuStoreLike;
  routePath: () => string;
  watchUserType?: boolean;
  reloadOnLogout?: boolean;
}

export function usePortalShellScaffold(options: UsePortalShellScaffoldOptions) {
  async function handleLogout() {
    const redirected = await options.authStore.logout();
    if (!redirected && (options.reloadOnLogout ?? true)) {
      window.location.reload();
    }
  }

  onMounted(async () => {
    options.authStore.init();
    if (options.authStore.accessToken && !options.authStore.userInfo) {
      await options.authStore.fetchUserInfo();
    }
    options.menuStore.refresh(options.routePath());
  });

  watch(options.routePath, (path) => {
    options.menuStore.refresh(path);
  });

  if (options.watchUserType) {
    watch(
      () => options.authStore.userType,
      () => {
        options.menuStore.refresh(options.routePath());
      },
    );
  }

  return {
    handleLogout,
  };
}

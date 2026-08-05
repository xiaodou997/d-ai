import { defineStore } from "pinia";
import { ref } from "vue";

export interface AppShellNavItem {
  id: string;
  label: string;
  to?: string;
  icon?: string;
  active?: boolean;
  disabled?: boolean;
  children?: AppShellNavItem[];
}

export function createPortalMenuStore<TArgs extends unknown[]>(
  storeId: string,
  buildNav: (...args: TArgs) => AppShellNavItem[],
  buildArgs: (currentPath: string) => TArgs
) {
  return defineStore(storeId, () => {
    const items = ref<AppShellNavItem[]>([]);

    function refresh(currentPath = "/") {
      items.value = buildNav(...buildArgs(currentPath));
    }

    return {
      items,
      refresh
    };
  });
}

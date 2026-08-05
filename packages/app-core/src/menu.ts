export interface PortalMenuNode {
  id: string;
  categoryId?: number;
  name?: string;
  label: string;
  to?: string;
  route?: string;
  icon?: string;
  type?: "directory" | "menu" | "button";
  permKey?: string;
  active?: boolean;
  disabled?: boolean;
  children?: PortalMenuNode[];
}

export interface PortalMenuCategory {
  id: number;
  name: string;
  clientId?: string;
  description?: string;
  sortOrder?: number;
  menus: PortalMenuNode[];
}

// 给 `to` 无法被前端 router 解析的菜单叶子打 disabled 标记，
// 供 shell 渲染为禁用态而非 RouterLink，规避 vue-router 的 "No match found" 告警。
export function markUnresolvableMenus(
  nodes: PortalMenuNode[],
  validPaths: Set<string>
): PortalMenuNode[] {
  return nodes.map((node) => ({
    ...node,
    disabled: node.to ? !validPaths.has(node.to) : node.disabled,
    children: node.children ? markUnresolvableMenus(node.children, validPaths) : node.children
  }));
}

export function mapMenuTree(categories: PortalMenuCategory[], currentPath: string): PortalMenuNode[] {
  return categories.map((category) => ({
    id: `category-${category.id}`,
    categoryId: category.id,
    label: category.name,
    active: category.menus.some((menu) => hasActiveMenu(menu, currentPath)),
    children: category.menus.map((menu, index) => mapMenuNode(menu, `${category.id}-${index}`, currentPath))
  }));
}

function mapMenuNode(menu: PortalMenuNode, fallbackId: string, currentPath: string): PortalMenuNode {
  const children = menu.children?.map((child, index) =>
    mapMenuNode(child, `${fallbackId}-${index}`, currentPath)
  );
  const active = hasActiveMenu(menu, currentPath);
  const to = menu.to || menu.route;
  return {
    ...menu,
    id: menu.id || fallbackId,
    label: menu.label || menu.name || fallbackId,
    to,
    active,
    children
  };
}

function hasActiveMenu(menu: PortalMenuNode, currentPath: string): boolean {
  const target = menu.to || menu.route;
  if (target && currentPath.startsWith(target)) return true;
  return Boolean(menu.children?.some((child) => hasActiveMenu(child, currentPath)));
}

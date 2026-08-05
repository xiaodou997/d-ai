// 从 DsUI token 解析实际颜色值。echarts(canvas)无法直接消费 CSS var;
// 且 .ds-theme-* 主题覆盖挂在 app-shell 子树上(documentElement 只有默认色),
// 所以必须对着主题子树内的元素取计算样式。
export function resolveDsColor(el: Element | null, token: string): string {
  if (!el || typeof getComputedStyle !== "function") return "";
  return getComputedStyle(el).getPropertyValue(token).trim();
}

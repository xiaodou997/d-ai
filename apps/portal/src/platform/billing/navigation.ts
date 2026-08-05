export function navigateBilling(basePath: string, path: string): string {
  const base = basePath.endsWith("/") ? basePath.slice(0, -1) : basePath;
  const target = path.startsWith("/") ? path : `/${path}`;
  return `${base}${target}`;
}

export function getBillingRouteParam(value: string | string[] | undefined): string {
  if (Array.isArray(value)) return value.join("/");
  return value || "";
}

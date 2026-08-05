export interface PortalDocumentBrand {
  siteName?: string;
  faviconUrl?: string;
  defaultTitle: string;
  defaultFaviconUrl?: string;
}

export function applyPortalDocumentBrand(input: PortalDocumentBrand) {
  if (typeof document === "undefined") return;

  document.title = input.siteName?.trim() || input.defaultTitle;
  const favicon = ensureFaviconLink();
  const url = input.faviconUrl?.trim() || input.defaultFaviconUrl?.trim();
  if (favicon && url) {
    favicon.href = url;
  }
}

export function resolvePortalResourceUrl(baseUrl: string, path?: string) {
  const normalizedPath = path?.trim() || "";
  if (!normalizedPath) return "";
  if (/^https?:\/\//i.test(normalizedPath)) return normalizedPath;
  return `${baseUrl.replace(/\/$/, "")}/${normalizedPath.replace(/^\//, "")}`;
}

function ensureFaviconLink() {
  let favicon = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
  if (favicon) return favicon;

  favicon = document.createElement("link");
  favicon.rel = "icon";
  document.head.append(favicon);
  return favicon;
}

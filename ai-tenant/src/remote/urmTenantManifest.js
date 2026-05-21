// dev 模式 remoteEntry.js 内部含 path-absolute import(/@id、/node_modules/.vite/deps），
// 必须从 urm-tenant 自己的 origin 加载，不能经 host 的 /mf proxy 转发。
const DEFAULT_URM_MANIFEST_URL = import.meta.env.DEV
  ? 'http://localhost:6902/mf/urm-manifest.json'
  : '/mf/urm-manifest.json'

const URM_MANIFEST_URL = import.meta.env.VITE_URM_MANIFEST_URL || DEFAULT_URM_MANIFEST_URL

let manifestPromise = null

export function getUrmTenantManifestUrl() {
  return URM_MANIFEST_URL
}

export async function loadUrmTenantManifest() {
  if (!manifestPromise) {
    manifestPromise = fetch(getUrmTenantManifestUrl(), {
      headers: { Accept: 'application/json' }
    }).then(async (response) => {
      if (!response.ok) {
        throw new Error(`URM manifest 加载失败：${response.status}`)
      }
      return response.json()
    })
  }

  return manifestPromise
}

export function resolveManifestUrl(path) {
  const manifestUrl = new URL(getUrmTenantManifestUrl(), window.location.origin)
  return new URL(path, manifestUrl).toString()
}

export function getUrmTenantPages(manifest) {
  const tenantApp = manifest?.applications?.tenant
  return tenantApp?.pages || tenantApp?.routes || []
}

export function getUrmTenantPagePath(page) {
  return page?.hostPath || page?.path || ''
}

export function getUrmTenantPageRemoteKey(page) {
  return page?.remotePage || page?.page || page?.key || ''
}

export function findUrmTenantPageByPath(manifest, path) {
  return getUrmTenantPages(manifest).find((page) => getUrmTenantPagePath(page) === path)
}

export function getUrmTenantMenuGroups(manifest) {
  const groups = new Map()
  const pages = getUrmTenantPages(manifest)
    .filter((page) => page.menuGroup && getUrmTenantPagePath(page))
    .sort((a, b) => (a.order || 0) - (b.order || 0))

  pages.forEach((page) => {
    if (!groups.has(page.menuGroup)) {
      groups.set(page.menuGroup, [])
    }
    groups.get(page.menuGroup).push(page)
  })

  return Array.from(groups, ([title, items]) => ({ title, items }))
}

export async function getUrmTenantStandaloneUrl(standalonePath = '/') {
  const manifest = await loadUrmTenantManifest()
  const manifestBase = manifest?.applications?.tenant?.standaloneBaseUrl
  const manifestOrigin = new URL(getUrmTenantManifestUrl(), window.location.origin).origin
  const base = manifestBase || manifestOrigin
  return new URL(standalonePath, base).toString()
}

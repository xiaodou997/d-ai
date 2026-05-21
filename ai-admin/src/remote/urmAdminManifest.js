// dev 模式 remoteEntry.js 内部含 path-absolute import(/@id、/node_modules/.vite/deps），
// 必须从 urm-admin 自己的 origin 加载，不能经 host 的 /mf proxy 转发。
const DEFAULT_URM_MANIFEST_URL = import.meta.env.DEV
  ? 'http://localhost:6901/mf/urm-manifest.json'
  : '/mf/urm-manifest.json'

const URM_MANIFEST_URL = import.meta.env.VITE_URM_MANIFEST_URL || DEFAULT_URM_MANIFEST_URL

let manifestPromise = null

export function getUrmAdminManifestUrl() {
  return URM_MANIFEST_URL
}

export async function loadUrmAdminManifest() {
  if (!manifestPromise) {
    manifestPromise = fetch(getUrmAdminManifestUrl(), {
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
  const manifestUrl = new URL(getUrmAdminManifestUrl(), window.location.origin)
  return new URL(path, manifestUrl).toString()
}

export function getUrmAdminPages(manifest) {
  const adminApp = manifest?.applications?.admin
  return adminApp?.pages || adminApp?.routes || []
}

export function getUrmAdminPagePath(page) {
  return page?.hostPath || page?.path || ''
}

export function getUrmAdminPageRemoteKey(page) {
  return page?.remotePage || page?.page || page?.key || ''
}

export function getUrmAdminStandalonePath(page, currentFullPath = '') {
  const standalonePath = page?.standalonePath || '/'
  const hostPath = getUrmAdminPagePath(page)
  if (!hostPath || !currentFullPath) return standalonePath

  const [currentPath, query = ''] = currentFullPath.split('?')
  const hostParts = hostPath.split('/')
  const currentParts = currentPath.split('/')
  const params = {}

  hostParts.forEach((part, index) => {
    if (part.startsWith(':')) {
      params[part.slice(1)] = currentParts[index]
    }
  })

  const resolvedPath = standalonePath.replace(/:([^/]+)/g, (_, key) => params[key] || '')
  return query ? `${resolvedPath}?${query}` : resolvedPath
}

function createPageMatcher(pattern) {
  const escaped = pattern
    .replace(/[-/\\^$*+?.()|[\]{}]/g, '\\$&')
    .replace(/:([^/]+)/g, '[^/]+')

  return new RegExp(`^${escaped}$`)
}

export function findUrmAdminPageByPath(manifest, path) {
  return getUrmAdminPages(manifest).find((page) => {
    const pagePath = getUrmAdminPagePath(page)
    if (!pagePath) return false
    if (pagePath === path) return true
    return createPageMatcher(pagePath).test(path)
  })
}

export function getUrmAdminMenuGroups(manifest) {
  const groups = new Map()
  const pages = getUrmAdminPages(manifest)
    .filter((page) => page.menu !== false && page.menuGroup && getUrmAdminPagePath(page))
    .sort((a, b) => (a.order || 0) - (b.order || 0))

  pages.forEach((page) => {
    if (!groups.has(page.menuGroup)) {
      groups.set(page.menuGroup, [])
    }
    groups.get(page.menuGroup).push(page)
  })

  return Array.from(groups, ([title, items]) => ({ title, items }))
}

export async function getUrmAdminStandaloneUrl(standalonePath = '/') {
  const manifest = await loadUrmAdminManifest()
  const manifestBase = manifest?.applications?.admin?.standaloneBaseUrl
  const manifestOrigin = new URL(getUrmAdminManifestUrl(), window.location.origin).origin
  const base = manifestBase || manifestOrigin
  return new URL(standalonePath, base).toString()
}

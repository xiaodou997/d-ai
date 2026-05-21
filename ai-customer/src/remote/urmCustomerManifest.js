const DEFAULT_URM_MANIFEST_URL = import.meta.env.DEV
  ? 'http://localhost:6903/mf/urm-manifest.json'
  : '/mf/urm-manifest.json'

const URM_MANIFEST_URL = import.meta.env.VITE_URM_MANIFEST_URL || DEFAULT_URM_MANIFEST_URL

let manifestPromise = null

export function getUrmCustomerManifestUrl() {
  return URM_MANIFEST_URL
}

export async function loadUrmCustomerManifest() {
  if (!manifestPromise) {
    manifestPromise = fetch(getUrmCustomerManifestUrl(), {
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
  const manifestUrl = new URL(getUrmCustomerManifestUrl(), window.location.origin)
  return new URL(path, manifestUrl).toString()
}

export function getUrmCustomerPages(manifest) {
  const customerApp = manifest?.applications?.customer
  return customerApp?.pages || customerApp?.routes || []
}

export function getUrmCustomerPagePath(page) {
  return page?.hostPath || page?.path || ''
}

export function getUrmCustomerPageRemoteKey(page) {
  return page?.remotePage || page?.page || page?.key || ''
}

export function findUrmCustomerPageByPath(manifest, path) {
  return getUrmCustomerPages(manifest).find((page) => getUrmCustomerPagePath(page) === path)
}

export function getUrmCustomerMenuGroups(manifest) {
  const groups = new Map()
  const pages = getUrmCustomerPages(manifest)
    .filter((page) => page.menuGroup && getUrmCustomerPagePath(page))
    .sort((a, b) => (a.order || 0) - (b.order || 0))

  pages.forEach((page) => {
    if (!groups.has(page.menuGroup)) {
      groups.set(page.menuGroup, [])
    }
    groups.get(page.menuGroup).push(page)
  })

  return Array.from(groups, ([title, items]) => ({ title, items }))
}

export async function getUrmCustomerStandaloneUrl(standalonePath = '/') {
  const manifest = await loadUrmCustomerManifest()
  const manifestBase = manifest?.applications?.customer?.standaloneBaseUrl
  const manifestOrigin = new URL(getUrmCustomerManifestUrl(), window.location.origin).origin
  const base = manifestBase || manifestOrigin
  return new URL(standalonePath, base).toString()
}

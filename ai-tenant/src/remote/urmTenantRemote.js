import { createInstance, getInstance, loadRemote, registerRemotes } from '@module-federation/runtime'
import { loadUrmTenantManifest, resolveManifestUrl } from './urmTenantManifest'

let remoteComponentPromise = null
let registeredRemoteName = ''

function ensureFederationRuntime() {
  if (getInstance()) return

  createInstance({
    name: 'ai_tenant_host',
    remotes: [],
    shared: {}
  })
}

export async function loadUrmTenantRemoteComponent() {
  if (remoteComponentPromise) return remoteComponentPromise

  remoteComponentPromise = (async () => {
    ensureFederationRuntime()

    const manifest = await loadUrmTenantManifest()
    const remote = manifest?.remote
    if (!remote?.name || !remote?.entry || !remote?.exposedModule) {
      throw new Error('URM manifest 缺少 remote 配置')
    }

    const entry = resolveManifestUrl(remote.entry)
    registeredRemoteName = remote.name

    registerRemotes(
      [
        {
          name: remote.name,
          entry,
          type: remote.type || 'module',
          entryGlobalName: remote.entryGlobalName || remote.name,
          shareScope: remote.shareScope || 'default'
        }
      ],
      { force: true }
    )

    const loadedModule = await loadRemote(`${remote.name}/${remote.exposedModule.replace(/^\.\//, '')}`)
    const component = loadedModule?.default || loadedModule
    if (!component) {
      throw new Error('URM remote 组件加载失败')
    }
    return component
  })().catch((err) => {
    remoteComponentPromise = null
    throw err
  })

  return remoteComponentPromise
}

export function getRegisteredUrmTenantRemoteName() {
  return registeredRemoteName
}

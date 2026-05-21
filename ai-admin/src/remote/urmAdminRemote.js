import { createInstance, getInstance, loadRemote, registerRemotes } from '@module-federation/runtime'
import { loadUrmAdminManifest, resolveManifestUrl } from './urmAdminManifest'

let remoteComponentPromise = null
let registeredRemoteName = ''

function ensureFederationRuntime() {
  if (getInstance()) return

  createInstance({
    name: 'ai_admin_host',
    remotes: [],
    shared: {}
  })
}

export async function loadUrmAdminRemoteComponent() {
  if (remoteComponentPromise) return remoteComponentPromise

  remoteComponentPromise = (async () => {
    ensureFederationRuntime()

    const manifest = await loadUrmAdminManifest()
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

export function getRegisteredUrmAdminRemoteName() {
  return registeredRemoteName
}

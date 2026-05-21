<script setup>
import { computed, markRaw, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Link, Loading } from '@element-plus/icons-vue'
import { createUrmHostContext } from '@/remote/urmHostContext'
import {
  findUrmAdminPageByPath,
  getUrmAdminPageRemoteKey,
  getUrmAdminStandalonePath,
  getUrmAdminStandaloneUrl,
  loadUrmAdminManifest
} from '@/remote/urmAdminManifest'
import { loadUrmAdminRemoteComponent } from '@/remote/urmAdminRemote'

const route = useRoute()
const router = useRouter()

const RemoteComponent = shallowRef(null)
const loading = shallowRef(true)
const error = shallowRef('')
const standaloneUrl = shallowRef('')
const activePage = shallowRef(null)

const remotePage = computed(() => getUrmAdminPageRemoteKey(activePage.value))
const standalonePath = computed(() => getUrmAdminStandalonePath(activePage.value, route.fullPath))

const hostContext = computed(() => createUrmHostContext({
  router,
  route,
  standalonePath: standaloneUrl.value
}))

const openStandalone = () => {
  if (standaloneUrl.value) {
    window.open(standaloneUrl.value, '_blank', 'noopener,noreferrer')
  }
}

async function loadPage() {
  loading.value = true
  error.value = ''
  activePage.value = null

  try {
    const manifest = await loadUrmAdminManifest()
    const page = findUrmAdminPageByPath(manifest, route.path)
    if (!page) {
      throw new Error(`URM manifest 未配置页面：${route.path}`)
    }

    activePage.value = page
    standaloneUrl.value = await getUrmAdminStandaloneUrl(standalonePath.value)
    RemoteComponent.value = markRaw(await loadUrmAdminRemoteComponent())
  } catch (err) {
    error.value = err?.message || 'URM 模块加载失败'
    if (!standaloneUrl.value) {
      standaloneUrl.value = await getUrmAdminStandaloneUrl(standalonePath.value).catch(() => '')
    }
  } finally {
    loading.value = false
  }
}

watch(() => route.path, loadPage, { immediate: true })
</script>

<template>
  <div v-if="loading" class="remote-state">
    <el-icon class="remote-state__icon is-loading" :size="28"><Loading /></el-icon>
    <span>正在加载 URM 模块</span>
  </div>

  <component
    :is="RemoteComponent"
    v-else-if="RemoteComponent"
    :key="route.fullPath"
    :page="remotePage"
    :host-context="hostContext"
  />

  <div v-else class="remote-state remote-state--error">
    <div>
      <h1 class="remote-state__title">URM 模块暂不可用</h1>
      <p class="remote-state__desc">{{ error || '请稍后重试，或打开 URM 独立页面继续操作。' }}</p>
    </div>
    <el-button type="primary" :icon="Link" :disabled="!standaloneUrl" @click="openStandalone">
      打开 URM 页面
    </el-button>
  </div>
</template>

<style scoped>
.remote-state {
  min-height: 280px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: #64748b;
  background: #fff;
  border: 1px solid #f1f5f9;
  border-radius: 16px;
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.04);
}

.remote-state--error {
  flex-direction: column;
  text-align: center;
  padding: 32px;
}

.remote-state__icon {
  color: #3b82f6;
}

.remote-state__title {
  margin: 0;
  font-size: 20px;
  font-weight: 900;
  color: #1e293b;
}

.remote-state__desc {
  margin: 8px 0 0;
  font-size: 13px;
  color: #94a3b8;
}
</style>

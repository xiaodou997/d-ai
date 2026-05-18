<script setup>
import { computed } from 'vue'
import {
  Connection,
  Cpu,
  DataAnalysis,
  DocumentChecked,
  Monitor,
  Key,
  Lock,
  Management,
  OfficeBuilding,
  SwitchButton,
  Tickets,
  User,
  UserFilled
} from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { ElMessageBox, ElMessage } from 'element-plus'

const authStore = useAuthStore()

const gatewayMenuItems = computed(() => {
  const shared = [
    { index: '/ai-gateway/access', label: '授权与 Key', icon: Key },
    { index: '/ai-gateway/usage', label: '调用日志', icon: Tickets }
  ]

  if (!authStore.isPlatformAdmin) {
    return shared
  }

  return [
    { index: '/ai-gateway/providers', label: '厂商接入', icon: Connection },
    { index: '/ai-gateway/models', label: '模型映射', icon: Cpu },
    { index: '/ai-gateway/credential-pools', label: '账号池', icon: UserFilled },
    ...shared,
    { index: '/ai-gateway/limits', label: '限流策略', icon: Lock },
    { index: '/ai-gateway/routing', label: '路由策略', icon: DataAnalysis },
    { index: '/ai-gateway/audit', label: '网关审计', icon: DocumentChecked }
  ]
})

const handleLogout = () => {
  ElMessageBox.confirm('您确定要退出 Uni AI API 管理后台吗？', '提示', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'warning',
    roundButton: true
  })
    .then(async () => {
      await authStore.logout()
      ElMessage.success('已安全退出')
    })
    .catch(() => {})
}
</script>

<template>
  <aside class="sidebar-shell">
    <div class="sidebar-brand">
      <div class="brand-icon">
        <el-icon color="#fff" :size="20"><Management /></el-icon>
      </div>
      <span class="brand-text">
        Uni<span>AI API</span>
      </span>
    </div>

    <nav class="sidebar-nav custom-scrollbar">
      <el-menu
        :default-active="$route.path"
        :router="true"
        class="modern-menu border-none"
      >
        <template v-if="authStore.isPlatformAdmin">
          <div class="menu-divider">数据监控</div>
          <el-menu-item index="/dashboard">
            <el-icon><DataAnalysis /></el-icon>
            <span>数据大盘</span>
          </el-menu-item>
          <el-menu-item index="/system-status">
            <el-icon><Monitor /></el-icon>
            <span>系统状态</span>
          </el-menu-item>
        </template>

        <div class="menu-divider">{{ authStore.isPlatformAdmin ? 'AI 网关' : '工作台' }}</div>
        <el-menu-item
          v-for="item in gatewayMenuItems"
          :key="item.index"
          :index="item.index"
        >
          <el-icon><component :is="item.icon" /></el-icon>
          <span>{{ item.label }}</span>
        </el-menu-item>

        <template v-if="authStore.isPlatformAdmin">
          <div class="menu-divider">运营管理</div>
          <el-menu-item index="/tenants">
            <el-icon><OfficeBuilding /></el-icon>
            <span>租户管理</span>
          </el-menu-item>
          <el-menu-item index="/users">
            <el-icon><User /></el-icon>
            <span>终端用户</span>
          </el-menu-item>
          <el-menu-item index="/finance/recharge-records">
            <el-icon><Tickets /></el-icon>
            <span>充值记录</span>
          </el-menu-item>
        </template>
      </el-menu>
    </nav>

    <div class="sidebar-user">
      <div class="user-card" @click="handleLogout">
        <el-avatar :size="32" class="bg-primary-100 text-primary-600 font-bold">
          {{ authStore.username?.[0]?.toUpperCase() || 'A' }}
        </el-avatar>
        <div class="user-info">
          <p class="user-name">
            {{ authStore.username || 'Admin' }}
          </p>
          <p class="user-role">{{ authStore.roleName }}</p>
        </div>
        <el-icon class="logout-icon">
          <SwitchButton />
        </el-icon>
      </div>
    </div>
  </aside>
</template>

<style scoped lang="scss">
.sidebar-shell {
  position: relative;
  z-index: 20;
  display: flex;
  width: 16rem;
  height: 100%;
  flex-direction: column;
  border-right: 1px solid #f8fafc;
  background: #ffffff;
  box-shadow: 0 20px 60px rgba(15, 23, 42, 0.06);
}

.sidebar-brand {
  display: flex;
  height: 5rem;
  align-items: center;
  border-bottom: 1px solid #f8fafc;
  padding: 0 1.5rem;
}

.brand-icon {
  display: flex;
  width: 2rem;
  height: 2rem;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  background: #8b5cf6;
  box-shadow: 0 10px 20px rgba(139, 92, 246, 0.2);
}

.brand-text {
  margin-left: 0.75rem;
  color: #1e293b;
  font-size: 1.25rem;
  font-weight: 800;
  letter-spacing: 0;
}

.brand-text span {
  color: #8b5cf6;
}

.sidebar-nav {
  flex: 1;
  overflow-y: auto;
  padding: 1rem 0.75rem;
}

.modern-menu {
  :deep(.el-menu-item) {
    height: 48px;
    margin: 4px 0;
    border-radius: 12px;
    color: #64748b;
    font-weight: 600;
    line-height: 48px;
    transition: all 0.2s ease;

    &:hover {
      background-color: #f8fafc;
      color: #334155;
    }

    .el-icon {
      margin-right: 12px;
      font-size: 18px;
    }
  }

  :deep(.el-menu-item.is-active) {
    background-color: #faf5ff;
    color: #8b5cf6;
    font-weight: 800;

    &::before {
      position: absolute;
      top: 12px;
      bottom: 12px;
      left: 0;
      width: 4px;
      border-radius: 0 4px 4px 0;
      background-color: #8b5cf6;
      content: '';
    }
  }
}

.menu-divider {
  padding: 16px 12px 8px;
  color: #cbd5e1;
  font-size: 10px;
  font-weight: 900;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.sidebar-user {
  border-top: 1px solid #f8fafc;
  padding: 1rem;
}

.user-card {
  display: flex;
  cursor: pointer;
  align-items: center;
  border-radius: 1rem;
  background: #f8fafc;
  padding: 0.75rem;
  transition: background 0.2s ease;
}

.user-card:hover {
  background: #f1f5f9;
}

.user-card:hover .logout-icon {
  color: #f43f5e;
}

.user-info {
  min-width: 0;
  flex: 1;
  margin-left: 0.75rem;
}

.user-name,
.user-role {
  overflow: hidden;
  margin: 0;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-name {
  color: #1e293b;
  font-size: 12px;
  font-weight: 800;
}

.user-role {
  color: #94a3b8;
  font-size: 10px;
}

.logout-icon {
  color: #cbd5e1;
  transition: color 0.2s ease;
}

.custom-scrollbar {
  scrollbar-color: #e2e8f0 transparent;
  scrollbar-width: thin;

  &::-webkit-scrollbar {
    width: 4px;
  }

  &::-webkit-scrollbar-thumb {
    border-radius: 10px;
    background-color: #e2e8f0;
  }
}
</style>

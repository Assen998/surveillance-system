<template>
  <el-container class="app-container">
    <el-aside :width="isCollapse ? '64px' : '240px'" class="sidebar">
      <div class="logo-container">
        <el-icon class="logo-icon" v-if="!isCollapse"><VideoCamera /></el-icon>
        <span v-if="!isCollapse" class="logo-text">监控系统</span>
      </div>
      <el-menu
        :default-active="activeMenu"
        :collapse="isCollapse"
        :unique-opened="true"
        :collapse-transition="false"
        router
        class="menu"
      >
        <template v-for="route in filteredRoutes" :key="route.fullPath">
          <el-menu-item v-if="!route.children || route.children.length === 0" :index="route.fullPath">
            <el-icon><component :is="route.meta.icon || 'Folder'" /></el-icon>
            <template v-if="!isCollapse">{{ route.meta.title }}</template>
          </el-menu-item>
          <el-sub-menu v-else :index="route.fullPath">
            <template #title>
              <el-icon><component :is="route.meta.icon || 'Folder'" /></el-icon>
              <template v-if="!isCollapse">{{ route.meta.title }}</template>
            </template>
            <template v-for="child in route.children" :key="child.fullPath">
              <el-menu-item v-if="!child.meta.hideInMenu" :index="child.fullPath">
                {{ child.meta.title }}
              </el-menu-item>
            </template>
          </el-sub-menu>
        </template>
      </el-menu>
      <div class="collapse-btn" @click="toggleCollapse">
        <el-icon><Fold v-if="!isCollapse" /><Expand v-else /></el-icon>
      </div>
    </el-aside>

    <el-container>
      <el-header class="header">
        <div class="header-left">
          <el-icon class="hamburger" @click="toggleCollapse"><Menu /></el-icon>
          <h1 class="page-title">{{ pageTitle }}</h1>
        </div>
        <div class="header-right">
          <el-dropdown trigger="click">
            <span class="user-info">
              <el-icon><User /></el-icon>
              <span>{{ userName }}</span>
              <el-icon><ArrowDown /></el-icon>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item @click="logout">
                  <el-icon><SwitchButton /></el-icon>
                  退出登录
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>

      <el-main class="main-content">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import {
  VideoCamera, Folder, Monitor, Film, Cpu, Memo, Setting,
  Menu, Fold, Expand, User, ArrowDown, SwitchButton
} from '@element-plus/icons-vue'

const router = useRouter()
const route = useRoute()

const isCollapse = ref(false)
const userName = 'admin'

const toggleCollapse = () => {
  isCollapse.value = !isCollapse.value
}

const filteredRoutes = computed(() => {
  const rootRoute = router.getRoutes().find(r => r.path === '/')
  if (!rootRoute) return []
  
  // 为每个路由及其子路由计算完整路径
  const addFullPath = (routes: any[], parentPath = '') => {
    return routes.map(route => {
      const fullPath = parentPath ? `${parentPath}/${route.path}`.replace(/\/+/g, '/') : `/${route.path}`.replace(/\/+/g, '/')
      const routeWithPath = { ...route, fullPath }
      if (route.children && route.children.length > 0) {
        routeWithPath.children = addFullPath(route.children, fullPath)
      }
      return routeWithPath
    })
  }
  
  return addFullPath(rootRoute.children || []).filter(r => !r.meta?.hideInMenu)
})

const activeMenu = computed(() => {
  // 查找当前路由对应的菜单项 fullPath
  const findFullPath = (routes: any[], currentPath: string): string => {
    for (const r of routes) {
      if (r.fullPath === currentPath) return r.fullPath
      if (r.children) {
        const found = findFullPath(r.children, currentPath)
        if (found) return found
      }
    }
    return currentPath
  }
  return findFullPath(filteredRoutes.value, route.path)
})

const pageTitle = computed(() => {
  const matched = route.matched[route.matched.length - 1]
  return (matched?.meta?.title as string) || '监控系统'
})

const logout = () => {
  localStorage.removeItem('token')
  router.push('/login')
}
</script>

<style scoped lang="scss">
.app-container {
  height: 100vh;
  display: flex;
}

.sidebar {
  height: 100%;
  background: #1f2d3d;
  border-right: 1px solid #2d3a4a;
  display: flex;
  flex-direction: column;
  transition: width 0.3s ease;

  .logo-container {
    height: 60px;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0 20px;
    border-bottom: 1px solid #2d3a4a;
    overflow: hidden;

    .logo-icon {
      font-size: 24px;
      color: #409eff;
    }

    .logo-text {
      margin-left: 12px;
      font-size: 18px;
      font-weight: 600;
      color: #fff;
      white-space: nowrap;
    }
  }

  .menu {
    flex: 1;
    overflow-y: auto;
    border: none;
    background: transparent;
    padding: 10px 0;

    ::v-deep {
      .el-menu-item, .el-sub-menu__title {
        height: 44px;
        line-height: 44px;
        color: #b4c2d4;
        padding: 0 20px;
        transition: all 0.3s;

        &:hover {
          background: #2d3a4a;
          color: #fff;
        }

        .el-icon {
          margin-right: 12px;
          font-size: 16px;
        }
      }

      .el-menu-item.is-active, .el-sub-menu.is-active > .el-sub-menu__title {
        background: #1e3a5f;
        color: #409eff;
        border-right: 3px solid #409eff;
      }

      .el-sub-menu__icon-arrow {
        transition: transform 0.3s;
      }

      .el-sub-menu.is-opened .el-sub-menu__icon-arrow {
        transform: rotate(90deg);
      }
    }
  }

  .collapse-btn {
    height: 40px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-top: 1px solid #2d3a4a;
    cursor: pointer;
    color: #6a7a8d;
    transition: color 0.3s;

    &:hover {
      color: #fff;
    }
  }
}

.header {
  height: 60px;
  background: #fff;
  border-bottom: 1px solid #e6e9ed;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  box-shadow: 0 1px 4px rgba(0,0,0,0.05);

  .header-left {
    display: flex;
    align-items: center;

    .hamburger {
      font-size: 20px;
      color: #606266;
      cursor: pointer;
      margin-right: 16px;
    }

    .page-title {
      margin: 0;
      font-size: 18px;
      font-weight: 600;
      color: #303133;
    }
  }

  .header-right {
    .user-info {
      display: flex;
      align-items: center;
      gap: 8px;
      padding: 6px 12px;
      border-radius: 6px;
      cursor: pointer;
      color: #606266;
      font-size: 14px;

      &:hover {
        background: #f5f7fa;
        color: #303133;
      }
    }
  }
}

.main-content {
  padding: 24px;
  background: #f0f2f5;
  min-height: calc(100vh - 60px);
  overflow-y: auto;
}
</style>
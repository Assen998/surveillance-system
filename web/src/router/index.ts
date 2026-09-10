import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'
import i18n from '@/i18n'

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/Login.vue'),
    meta: { title: 'layout.menu.login', hideInMenu: true },
  },
  {
    path: '/setup',
    name: 'Setup',
    component: () => import('@/views/Setup.vue'),
    meta: { title: 'layout.menu.setup', hideInMenu: true },
  },
  {
    path: '/',
    component: () => import('@/layouts/MainLayout.vue'),
    redirect: '/dashboard',
    children: [
      {
        path: 'dashboard',
        name: 'Dashboard',
        component: () => import('@/views/Dashboard.vue'),
        meta: { title: 'layout.menu.dashboard', icon: 'Monitor' },
      },
      {
        path: 'cameras',
        name: 'Cameras',
        component: () => import('@/views/cameras/Index.vue'),
        meta: { title: 'layout.menu.cameras', icon: 'VideoCamera' },
        children: [
          {
            path: '',
            name: 'CameraList',
            component: () => import('@/views/cameras/List.vue'),
            meta: { title: 'layout.menu.camerasList' },
          },
          {
            path: 'add',
            name: 'CameraAdd',
            component: () => import('@/views/cameras/Add.vue'),
            meta: { title: 'layout.menu.camerasAdd', hideInMenu: true },
          },
          {
            path: 'edit/:id',
            name: 'CameraEdit',
            component: () => import('@/views/cameras/Add.vue'),
            meta: { title: 'layout.menu.camerasEdit', hideInMenu: true },
          },
          {
            path: ':id',
            name: 'CameraDetail',
            component: () => import('@/views/cameras/Detail.vue'),
            meta: { title: 'layout.menu.camerasDetail', hideInMenu: true },
          },
        ],
      },
      {
        path: 'recordings',
        name: 'Recordings',
        component: () => import('@/views/recordings/Index.vue'),
        meta: { title: 'layout.menu.recordings', icon: 'Film' },
        children: [
          {
            path: '',
            name: 'RecordingList',
            component: () => import('@/views/recordings/List.vue'),
            meta: { title: 'layout.menu.recordingsList' },
          },
          {
            path: 'snapshots',
            name: 'SnapshotList',
            component: () => import('@/views/recordings/Snapshots.vue'),
            meta: { title: 'layout.menu.snapshots' },
          },
          {
            path: 'playback/:cameraId',
            name: 'Playback',
            component: () => import('@/views/recordings/Playback.vue'),
            meta: { title: 'layout.menu.playback', hideInMenu: true },
          },
        ],
      },
      {
        path: 'analytics',
        name: 'Analytics',
        component: () => import('@/views/analytics/Index.vue'),
        meta: { title: 'layout.menu.analytics', icon: 'Cpu' },
        children: [
          {
            path: '',
            name: 'AlertList',
            component: () => import('@/views/analytics/Alerts.vue'),
            meta: { title: 'layout.menu.alerts' },
          },
        ],
      },
      {
        path: 'storage',
        name: 'Storage',
        component: () => import('@/views/storage/Index.vue'),
        meta: { title: 'layout.menu.storage', icon: 'HardDrive' },
      },
      {
        path: 'settings',
        name: 'Settings',
        component: () => import('@/views/settings/Index.vue'),
        meta: { title: 'layout.menu.settings', icon: 'Setting' },
        children: [
          {
            path: '',
            name: 'SystemConfig',
            component: () => import('@/views/settings/System.vue'),
            meta: { title: 'layout.menu.settingsSystem' },
          },
          {
            path: 'camera-defaults',
            name: 'CameraDefaults',
            component: () => import('@/views/settings/CameraDefaults.vue'),
            meta: { title: 'layout.menu.settingsCameraDefaults' },
          },
          {
            path: 'storage',
            name: 'StorageSettings',
            component: () => import('@/views/settings/Storage.vue'),
            meta: { title: 'layout.menu.settingsStorage' },
          },
          {
            path: 'maintenance',
            name: 'SystemMaintenance',
            component: () => import('@/views/settings/Maintenance.vue'),
            meta: { title: 'layout.menu.settingsMaintenance' },
          },
          {
            path: 'alerts',
            name: 'AlertConfig',
            component: () => import('@/views/settings/Alerts.vue'),
            meta: { title: 'layout.menu.settingsAlerts' },
          },
          {
            path: 'users',
            name: 'UserManagement',
            component: () => import('@/views/settings/Users.vue'),
            meta: { title: 'layout.menu.settingsUsers', roles: ['admin'] },
          },
        ],
      },
    ],
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/dashboard',
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to, from, next) => {
  document.title = `${to.meta.title ? i18n.global.t(to.meta.title as string) : i18n.global.t('layout.system')} - ${i18n.global.t('layout.appName')}`
  const token = localStorage.getItem('token')
  if (to.path !== '/login' && to.path !== '/setup' && !token) {
    next('/login')
  } else {
    next()
  }
})

export default router

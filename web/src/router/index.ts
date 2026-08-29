import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/Login.vue'),
    meta: { title: '登录', hideInMenu: true },
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
        meta: { title: '仪表盘', icon: 'Monitor' },
      },
      {
        path: 'cameras',
        name: 'Cameras',
        component: () => import('@/views/cameras/Index.vue'),
        meta: { title: '摄像头管理', icon: 'VideoCamera' },
        children: [
          {
            path: '',
            name: 'CameraList',
            component: () => import('@/views/cameras/List.vue'),
            meta: { title: '摄像头列表' },
          },
          {
            path: 'add',
            name: 'CameraAdd',
            component: () => import('@/views/cameras/Add.vue'),
            meta: { title: '添加摄像头', hideInMenu: true },
          },
          {
            path: 'edit/:id',
            name: 'CameraEdit',
            component: () => import('@/views/cameras/Add.vue'),
            meta: { title: '编辑摄像头', hideInMenu: true },
          },
          {
            path: ':id',
            name: 'CameraDetail',
            component: () => import('@/views/cameras/Detail.vue'),
            meta: { title: '摄像头详情', hideInMenu: true },
          },
        ],
      },
      {
        path: 'recordings',
        name: 'Recordings',
        component: () => import('@/views/recordings/Index.vue'),
        meta: { title: '录像管理', icon: 'Film' },
        children: [
          {
            path: '',
            name: 'RecordingList',
            component: () => import('@/views/recordings/List.vue'),
            meta: { title: '录像列表' },
          },
          {
            path: 'snapshots',
            name: 'SnapshotList',
            component: () => import('@/views/recordings/Snapshots.vue'),
            meta: { title: '抓拍图片' },
          },
          {
            path: 'playback/:cameraId',
            name: 'Playback',
            component: () => import('@/views/recordings/Playback.vue'),
            meta: { title: '历史回放', hideInMenu: true },
          },
        ],
      },
      {
        path: 'analytics',
        name: 'Analytics',
        component: () => import('@/views/analytics/Index.vue'),
        meta: { title: '智能分析', icon: 'Cpu' },
        children: [
          {
            path: '',
            name: 'AlertList',
            component: () => import('@/views/analytics/Alerts.vue'),
            meta: { title: '报警记录' },
          },
        ],
      },
      {
        path: 'storage',
        name: 'Storage',
        component: () => import('@/views/storage/Index.vue'),
        meta: { title: '存储管理', icon: 'HardDrive' },
      },
      {
        path: 'settings',
        name: 'Settings',
        component: () => import('@/views/settings/Index.vue'),
        meta: { title: '系统设置', icon: 'Setting' },
        children: [
          {
            path: '',
            name: 'SystemConfig',
            component: () => import('@/views/settings/System.vue'),
            meta: { title: '系统配置' },
          },
          {
            path: 'alerts',
            name: 'AlertConfig',
            component: () => import('@/views/settings/Alerts.vue'),
            meta: { title: '报警配置' },
          },
          {
            path: 'users',
            name: 'UserManagement',
            component: () => import('@/views/settings/Users.vue'),
            meta: { title: '用户管理', roles: ['admin'] },
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
  document.title = `${to.meta.title || '监控系统'} - 监控录像系统`
  const token = localStorage.getItem('token')
  if (to.path !== '/login' && !token) {
    next('/login')
  } else {
    next()
  }
})

export default router
import axios, { AxiosInstance, InternalAxiosRequestConfig, AxiosResponse } from 'axios'
import { ElMessage } from 'element-plus'
import i18n from '@/i18n'

const API_BASE = '/api/v1'


function mediaUrl(path: string): string {
  const token = localStorage.getItem('token') || ''
  if (!token) return path
  const sep = path.includes('?') ? '&' : '?'
  return `${path}${sep}token=${encodeURIComponent(token)}`
}

const request: AxiosInstance = axios.create({
  baseURL: API_BASE,
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
})


request.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    const token = localStorage.getItem('token')
    if (token && config.headers) {
      config.headers.Authorization = `Bearer ${token}`
    }

    if (config.headers) {
      config.headers['Accept-Language'] = i18n.global.locale.value === 'en' ? 'en' : 'zh-CN'
    }
    return config
  },
  (error) => Promise.reject(error)
)


request.interceptors.response.use(
  (response: AxiosResponse) => {
    const { data } = response

    if (data.code !== undefined && data.code !== 0) {
      ElMessage.error(data.message || i18n.global.t('common.failed'))
      return Promise.reject(new Error(data.message))
    }
    if (data.data !== undefined) {
      const result = data.data

      if (data.total !== undefined) {
        ;(result as any)._total = data.total
      }
      return result
    }
    return data
  },
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      window.location.href = '/login'
    } else {


      const msg = error.response?.data?.message || error.response?.data?.error
      ElMessage.error(msg || error.message || i18n.global.t('common.networkError'))
    }
    return Promise.reject(error)
  }
)


export const api = {

  auth: {
    login: (username: string, password: string) =>
      request.post('/auth/login', { username, password }),
    logout: () => request.post('/auth/logout'),
    getMe: () => request.get('/auth/me'),
    changePassword: (oldPass: string, newPass: string) =>
      request.put('/auth/password', { old_password: oldPass, new_password: newPass }),
  },


  users: {
    list: () => request.get('/users'),
    create: (data: any) => request.post('/users', data),
    update: (id: number, data: any) => request.put(`/users/${id}`, data),
    remove: (id: number) => request.delete(`/users/${id}`),
    resetPassword: (id: number, password: string) =>
      request.post(`/users/${id}/reset-password`, { password }),
    listPermissions: (id: number) => request.get(`/users/${id}/permissions`),
    setPermissions: (id: number, permissions: any[]) =>
      request.put(`/users/${id}/permissions`, { permissions }),
  },


  cameras: {
    list: () => request.get('/cameras'),
    get: (id: number) => request.get(`/cameras/${id}`),
    create: (data: any) => request.post('/cameras', data),
    update: (id: number, data: any) => request.put(`/cameras/${id}`, data),
    delete: (id: number) => request.delete(`/cameras/${id}`),
    status: (id: number) => request.get(`/cameras/${id}/status`),
    start: (id: number) => request.post(`/cameras/${id}/start`),
    stop: (id: number) => request.post(`/cameras/${id}/stop`),
    restart: (id: number) => request.post(`/cameras/${id}/restart`),
    snapshot: (id: number) => request.post(`/cameras/${id}/snapshot`),
    ptz: (id: number, command: string, speed = 1) =>
      request.post(`/cameras/${id}/ptz`, { command, speed }),
    snapshots: (id: number, params?: any) => request.get(`/cameras/${id}/snapshots`, { params }),
    discover: (network = '192.168.1.0/24') =>
      request.get('/cameras/discover', { params: { network } }),
    discoverLAN: (timeout = 10) =>
      request.post('/cameras/discover/lan', { timeout }),
    probe: (ip: string, username?: string, password?: string) =>
      request.get('/cameras/probe', { params: { ip, username, password } }),
  },


  recordings: {
    list: (params?: any) => request.get('/recordings', { params }),
    get: (id: number) => request.get(`/recordings/${id}`),
    file: (id: number) => mediaUrl(`/api/v1/recordings/${id}/file`),
    download: (id: number) => request.get(`/recordings/${id}/download`, { responseType: 'blob' }),
    delete: (id: number) => request.delete(`/recordings/${id}`),
    byCamera: (cameraId: number, params?: any) =>
      request.get(`/recordings/camera/${cameraId}`, { params }),
    segments: (cameraId: number, start: string, end: string) =>
      request.get(`/recordings/camera/${cameraId}/segments`, { params: { start, end } }),
  },


  stream: {

    hlsPlaylist: (cameraId: number, stream?: string) => mediaUrl(`/api/v1/stream/camera/${cameraId}/hls${stream ? `?stream=${stream}` : ''}`),
    hlsSegment: (cameraId: number, file: string) => mediaUrl(`/api/v1/stream/camera/${cameraId}/hls/${file}`),
    mp4: (cameraId: number) => mediaUrl(`/api/v1/stream/camera/${cameraId}/mp4`),
    snapshot: (cameraId: number) => mediaUrl(`/api/v1/stream/camera/${cameraId}/snapshot`),
    recordingHLS: (recordingId: number) => mediaUrl(`/api/v1/stream/camera/recordings/${recordingId}/hls`),
  },


  analytics: {
    alerts: (params?: any) => request.get('/analytics/alerts', { params }),
    getAlert: (id: number) => request.get(`/analytics/alerts/${id}`),
    acknowledge: (id: number) => request.put(`/analytics/alerts/${id}/ack`),
    resolve: (id: number) => request.put(`/analytics/alerts/${id}/resolve`),
    deleteAlert: (id: number) => request.delete(`/analytics/alerts/${id}`),
    clearAlerts: () => request.delete('/analytics/alerts'),
  },


  storage: {
    stats: () => request.get('/storage/stats'),
    cleanup: () => request.post('/storage/cleanup'),
  },


  webdav: {
    list: (cameraId?: number) =>
      request.get('/webdav/list', { params: cameraId ? { camera_id: cameraId } : {} }),

    fileUrl: (path: string) =>
      mediaUrl(`/api/v1/webdav/file?path=${encodeURIComponent(path)}`),
  },


  minio: {
    list: (cameraId?: number) =>
      request.get('/minio/list', { params: cameraId ? { camera_id: cameraId } : {} }),

    fileUrl: (path: string) =>
      mediaUrl(`/api/v1/minio/file?path=${encodeURIComponent(path)}`),
  },


  snapshots: {
    list: (params: any) => request.get('/snapshots', { params }),
    remove: (id: number) => request.delete(`/snapshots/${id}`),
    clear: () => request.delete('/snapshots'),


    fileUrl: (path: string, cameraId: number) => {
      const name = (path || '').split('/').pop() || ''
      return mediaUrl(`/api/v1/stream/camera/${cameraId}/snapshots/${encodeURIComponent(name)}`)
    },
  },


  setup: {
    status: () => request.get('/setup/status'),
    create: (data: { username: string; password: string }) =>
      request.post('/setup', data),
  },


  alerts: {
    config: () => request.get('/alerts/config'),
    updateConfig: (data: any) => request.put('/alerts/config', data),
    test: (channel: string, data?: any) => request.post('/alerts/test', { channel, ...data }),
  },


  system: {
    config: () => request.get('/system/config'),
    updateConfig: (data: any) => request.put('/system/config', data),
    info: () => request.get('/system/info'),
    restart: () => request.post('/system/restart'),

    envCheck: () => request.get('/system/env'),

    logTail: (params: { lines?: number; keyword?: string }) =>
      request.get('/system/logs', { params }),
    logFiles: () => request.get('/system/logs/files'),
    clearLogs: () => request.post('/system/logs/clear'),

    createBackup: () => request.post('/system/backup'),
    listBackups: () => request.get('/system/backups'),
    downloadBackup: (name: string) =>
      request.get(`/system/backups/${encodeURIComponent(name)}/download`, { responseType: 'blob' }),
    deleteBackup: (name: string) =>
      request.delete(`/system/backups/${encodeURIComponent(name)}`),

    checkUpdate: () => request.get('/system/update/check'),
    performUpdate: () => request.post('/system/update', null, { timeout: 600000 }),
    getUpdateConfig: () => request.get('/system/update/config'),
    saveUpdateConfig: (data: { proxy?: string; github_repo?: string; base_url?: string }) =>
      request.put('/system/update/config', data),
  },


  settings: {
    getStorage: () => request.get('/settings/storage'),
    updateStorage: (data: any) => request.put('/settings/storage', data),
    getCamera: () => request.get('/settings/camera'),
    updateCamera: (data: { snapshot_enabled: boolean; snapshot_interval: number }) =>
      request.put('/settings/camera', data),
    testWebdav: (data: { url: string; username: string; password: string; base_path: string }) =>
      request.post('/settings/webdav/test', data),
    testMinio: (data: { endpoint: string; access_key: string; secret_key: string; bucket: string; use_ssl: boolean; base_path: string }) =>
      request.post('/settings/minio/test', data),
  },
}

export default request

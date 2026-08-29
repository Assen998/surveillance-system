import axios, { AxiosInstance, InternalAxiosRequestConfig, AxiosResponse } from 'axios'
import { ElMessage } from 'element-plus'

const API_BASE = '/api/v1'

// 浏览器通过 <video>/<img>/HLS.js 加载媒体时无法携带 Authorization 头，
// 因此媒体 URL 通过 ?token= 查询参数附加鉴权令牌。
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

// 请求拦截器
request.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    const token = localStorage.getItem('token')
    if (token && config.headers) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => Promise.reject(error)
)

// 响应拦截器
request.interceptors.response.use(
  (response: AxiosResponse) => {
    const { data } = response
    // 后端返回格式: { code: 0, data: ..., message: '' }
    if (data.code !== undefined && data.code !== 0) {
      ElMessage.error(data.message || '请求失败')
      return Promise.reject(new Error(data.message))
    }
    if (data.data !== undefined) {
      const result = data.data
      // 分页列表接口会附加 total；把它挂到返回数组上（非侵入），供分页器读取
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
      ElMessage.error(error.response?.data?.message || error.message || '网络错误')
    }
    return Promise.reject(error)
  }
)

// API 方法封装
export const api = {
  // 认证
  auth: {
    login: (username: string, password: string) =>
      request.post('/auth/login', { username, password }),
    logout: () => request.post('/auth/logout'),
    getMe: () => request.get('/auth/me'),
    changePassword: (oldPass: string, newPass: string) =>
      request.put('/auth/password', { old_password: oldPass, new_password: newPass }),
  },

  // 摄像头
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

  // 录像
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

  // 流媒体（浏览器直接加载，需通过 ?token= 携带鉴权令牌）
  stream: {
    hlsPlaylist: (cameraId: number) => mediaUrl(`/api/v1/stream/camera/${cameraId}/hls`),
    hlsSegment: (cameraId: number, file: string) => mediaUrl(`/api/v1/stream/camera/${cameraId}/hls/${file}`),
    mp4: (cameraId: number) => mediaUrl(`/api/v1/stream/camera/${cameraId}/mp4`),
    snapshot: (cameraId: number) => mediaUrl(`/api/v1/stream/camera/${cameraId}/snapshot`),
    recordingHLS: (recordingId: number) => mediaUrl(`/api/v1/stream/camera/recordings/${recordingId}/hls`),
  },

  // 智能分析（报警记录）
  analytics: {
    alerts: (params?: any) => request.get('/analytics/alerts', { params }),
    getAlert: (id: number) => request.get(`/analytics/alerts/${id}`),
    acknowledge: (id: number) => request.put(`/analytics/alerts/${id}/ack`),
    resolve: (id: number) => request.put(`/analytics/alerts/${id}/resolve`),
    deleteAlert: (id: number) => request.delete(`/analytics/alerts/${id}`),
    clearAlerts: () => request.delete('/analytics/alerts'),
  },

  // 存储
  storage: {
    stats: () => request.get('/storage/stats'),
    cleanup: () => request.post('/storage/cleanup'),
  },

  // WebDAV 远程录像
  webdav: {
    list: (cameraId?: number) =>
      request.get('/webdav/list', { params: cameraId ? { camera_id: cameraId } : {} }),
    // 浏览器 <video> 直接加载，走 ?token= 鉴权（支持 Range）
    fileUrl: (path: string) =>
      mediaUrl(`/api/v1/webdav/file?path=${encodeURIComponent(path)}`),
  },

  // 抓拍图片（全局列表）
  snapshots: {
    list: (params: any) => request.get('/snapshots', { params }),
    remove: (id: number) => request.delete(`/snapshots/${id}`),
    clear: () => request.delete('/snapshots'),
    // 图片地址：DB 路径 recordings/camera_N/snapshot_N_xxx.jpg
    // -> /api/v1/stream/camera/N/snapshots/snapshot_N_xxx.jpg?token=...
    fileUrl: (path: string, cameraId: number) => {
      const name = (path || '').split('/').pop() || ''
      return mediaUrl(`/api/v1/stream/camera/${cameraId}/snapshots/${encodeURIComponent(name)}`)
    },
  },

  // 报警配置
  alerts: {
    config: () => request.get('/alerts/config'),
    updateConfig: (data: any) => request.put('/alerts/config', data),
    test: (channel: string) => request.post('/alerts/test', { channel }),
  },

  // 系统
  system: {
    config: () => request.get('/system/config'),
    updateConfig: (data: any) => request.put('/system/config', data),
    info: () => request.get('/system/info'),
    restart: () => request.post('/system/restart'),
  },

  // 设置
  settings: {
    getStorage: () => request.get('/settings/storage'),
    updateStorage: (data: any) => request.put('/settings/storage', data),
    getCamera: () => request.get('/settings/camera'),
    updateCamera: (data: { snapshot_enabled: boolean; snapshot_interval: number }) =>
      request.put('/settings/camera', data),
    testWebdav: (data: { url: string; username: string; password: string; base_path: string }) =>
      request.post('/settings/webdav/test', data),
  },
}

export default request
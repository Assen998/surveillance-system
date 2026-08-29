import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { api } from '@/api'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem('token') || '')
  const user = ref<any>(null)

  const isLoggedIn = computed(() => !!token.value)

  const setToken = (newToken: string) => {
    token.value = newToken
    localStorage.setItem('token', newToken)
  }

  const setUser = (userInfo: any) => {
    user.value = userInfo
  }

  const logout = () => {
    token.value = ''
    user.value = null
    localStorage.removeItem('token')
  }

  return { token, user, isLoggedIn, setToken, setUser, logout }
})

export const useCameraStore = defineStore('camera', () => {
  const cameras = ref<any[]>([])
  const currentCamera = ref<any>(null)
  const loading = ref(false)

  const onlineCount = computed(() => cameras.value.filter(c => c.status === 'online').length)
  const offlineCount = computed(() => cameras.value.filter(c => c.status === 'offline').length)

  const fetchCameras = async () => {
    loading.value = true
    try {
      const res = await api.cameras.list()
      cameras.value = res.data || res
    } catch (e) {
      console.error('获取摄像头列表失败', e)
    } finally {
      loading.value = false
    }
  }

  const setCurrentCamera = (camera: any) => {
    currentCamera.value = camera
  }

  return { cameras, currentCamera, loading, onlineCount, offlineCount, fetchCameras, setCurrentCamera }
})

export const useAlertStore = defineStore('alert', () => {
  const alerts = ref<any[]>([])
  const unreadCount = ref(0)
  const ws = ref<WebSocket | null>(null)

  const connect = () => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    ws.value = new WebSocket(`${protocol}//${window.location.host}/ws`)

    ws.value.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data)
        if (msg.type === 'alert') {
          alerts.value.unshift(msg.data)
          unreadCount.value++
        }
      } catch (e) {
        console.error('WebSocket 消息解析失败', e)
      }
    }

    ws.value.onclose = () => {
      setTimeout(connect, 5000)
    }
  }

  const disconnect = () => {
    ws.value?.close()
    ws.value = null
  }

  const markRead = () => {
    unreadCount.value = 0
  }

  return { alerts, unreadCount, connect, disconnect, markRead }
})

export const useStorageStore = defineStore('storage', () => {
  const stats = ref<any>(null)
  const loading = ref(false)

  const fetchStats = async () => {
    loading.value = true
    try {
      stats.value = await api.storage.stats()
    } catch (e) {
      console.error('获取存储统计失败', e)
    } finally {
      loading.value = false
    }
  }

  return { stats, loading, fetchStats }
})
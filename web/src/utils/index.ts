// 通用工具函数

/** 格式化字节数 */
export function formatBytes(bytes: number, decimals = 2): string {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const dm = decimals < 0 ? 0 : decimals
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i]
}

/** 格式化时长(秒) */
export function formatDuration(seconds: number): string {
  if (!seconds || seconds < 0) return '0秒'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  const parts: string[] = []
  if (d) parts.push(`${d}天`)
  if (h) parts.push(`${h}小时`)
  if (m) parts.push(`${m}分`)
  if (s || parts.length === 0) parts.push(`${s}秒`)
  return parts.join('')
}

/** 格式化日期时间 */
export function formatDateTime(date: string | number | Date, format = 'YYYY-MM-DD HH:mm:ss'): string {
  const d = new Date(date)
  if (isNaN(d.getTime())) return '-'
  const pad = (n: number) => String(n).padStart(2, '0')
  return format
    .replace('YYYY', String(d.getFullYear()))
    .replace('MM', pad(d.getMonth() + 1))
    .replace('DD', pad(d.getDate()))
    .replace('HH', pad(d.getHours()))
    .replace('mm', pad(d.getMinutes()))
    .replace('ss', pad(d.getSeconds()))
}

/** 格式化相对时间 */
export function formatRelativeTime(date: string | number | Date): string {
  const now = Date.now()
  const then = new Date(date).getTime()
  const diff = Math.floor((now - then) / 1000)
  
  if (diff < 60) return '刚刚'
  if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`
  if (diff < 2592000) return `${Math.floor(diff / 86400)}天前`
  return formatDateTime(date, 'YYYY-MM-DD')
}

/** 防抖函数 */
export function debounce<T extends (...args: any[]) => any>(
  fn: T,
  delay: number
): (...args: Parameters<T>) => void {
  let timer: ReturnType<typeof setTimeout> | null = null
  return (...args: Parameters<T>) => {
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => fn(...args), delay)
  }
}

/** 节流函数 */
export function throttle<T extends (...args: any[]) => any>(
  fn: T,
  limit: number
): (...args: Parameters<T>) => void {
  let inThrottle = false
  return (...args: Parameters<T>) => {
    if (!inThrottle) {
      fn(...args)
      inThrottle = true
      setTimeout(() => (inThrottle = false), limit)
    }
  }
}

/** 深拷贝 */
export function deepClone<T>(obj: T): T {
  if (obj === null || typeof obj !== 'object') return obj
  if (obj instanceof Date) return new Date(obj.getTime()) as any
  if (obj instanceof Array) return obj.map(item => deepClone(item)) as any
  if (obj instanceof Object) {
    const cloned = {} as T
    for (const key in obj) {
      if (Object.prototype.hasOwnProperty.call(obj, key)) {
        cloned[key] = deepClone(obj[key])
      }
    }
    return cloned
  }
  return obj
}

/** 生成 UUID */
export function generateUUID(): string {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, c => {
    const r = (Math.random() * 16) | 0
    const v = c === 'x' ? r : (r & 0x3) | 0x8
    return v.toString(16)
  })
}

/** 颜色转换 */
export function hexToRgba(hex: string, alpha = 1): string {
  const clean = hex.replace('#', '')
  const r = parseInt(clean.slice(0, 2), 16)
  const g = parseInt(clean.slice(2, 4), 16)
  const b = parseInt(clean.slice(4, 6), 16)
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

/** 文件下载 */
export function downloadFile(blob: Blob, filename: string): void {
  const url = window.URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  window.URL.revokeObjectURL(url)
}

/** 复制到剪贴板 */
export async function copyToClipboard(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    // 降级方案
    const textarea = document.createElement('textarea')
    textarea.value = text
    textarea.style.position = 'fixed'
    textarea.style.opacity = '0'
    document.body.appendChild(textarea)
    textarea.select()
    const success = document.execCommand('copy')
    document.body.removeChild(textarea)
    return success
  }
}

/** 获取文件扩展名 */
export function getFileExtension(filename: string): string {
  return filename.slice((filename.lastIndexOf('.') - 1 >>> 0) + 2).toLowerCase()
}

/** 判断是否为视频文件 */
export function isVideoFile(filename: string): boolean {
  const videoExts = ['mp4', 'mkv', 'avi', 'mov', 'flv', 'ts', 'm3u8', 'webm']
  return videoExts.includes(getFileExtension(filename))
}

/** 判断是否为图片文件 */
export function isImageFile(filename: string): boolean {
  const imageExts = ['jpg', 'jpeg', 'png', 'gif', 'bmp', 'webp', 'svg']
  return imageExts.includes(getFileExtension(filename))
}

/** 解析查询字符串 */
export function parseQueryString(query: string): Record<string, string> {
  const params: Record<string, string> = {}
  new URLSearchParams(query).forEach((value, key) => {
    params[key] = value
  })
  return params
}

/** 构建查询字符串 */
export function buildQueryString(params: Record<string, any>): string {
  const searchParams = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') {
      searchParams.append(key, String(value))
    }
  })
  return searchParams.toString()
}

/** 睡眠/延迟 */
export function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms))
}

/** 重试函数 */
export async function retry<T>(
  fn: () => Promise<T>,
  retries = 3,
  delay = 1000,
  backoff = 2
): Promise<T> {
  try {
    return await fn()
  } catch (error) {
    if (retries <= 0) throw error
    await sleep(delay)
    return retry(fn, retries - 1, delay * backoff, backoff)
  }
}

/** 状态映射标签 */
export const statusLabels: Record<string, string> = {
  online: '在线',
  offline: '离线',
  error: '异常',
  recording: '录像中',
  connecting: '连接中',
}

export const alertLevelLabels: Record<string, string> = {
  low: '低',
  medium: '中',
  high: '高',
  critical: '严重',
}

export const alertTypeLabels: Record<string, string> = {
  motion: '运动检测',
  intrusion: '区域入侵',
  line_cross: '越界检测',
  object_detect: '目标检测',
  offline: '设备离线',
  storage_full: '存储满',
  error: '系统错误',
}

export const recordTypeLabels: Record<string, string> = {
  continuous: '连续录像',
  motion: '移动侦测',
  schedule: '定时录像',
  manual: '手动录像',
}

/** 获取状态标签类型 */
export function getStatusType(status: string): 'success' | 'warning' | 'danger' | 'info' | 'primary' {
  const map: Record<string, any> = {
    online: 'success',
    offline: 'warning',
    error: 'danger',
    recording: 'danger',
    connecting: 'info',
    new: 'danger',
    acknowledged: 'warning',
    resolved: 'success',
    low: 'success',
    medium: 'warning',
    high: 'danger',
    critical: 'danger',
    motion: 'primary',
    intrusion: 'warning',
    line_cross: 'warning',
    object_detect: 'success',
    storage_full: 'warning',
  }
  return map[status] || 'info'
}

/** 设备类型图标 */
export const deviceIcons: Record<string, string> = {
  rtsp: 'VideoCamera',
  onvif: 'Connection',
  gb28181: 'Setting',
}
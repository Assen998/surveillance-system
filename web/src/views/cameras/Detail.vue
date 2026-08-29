<template>
  <div class="camera-detail-page" v-if="camera">
    <!-- 顶部信息栏 -->
    <el-card :shadow="never" class="mb-16">
      <div class="camera-header">
        <div class="camera-basic">
          <h2>{{ camera.name }}</h2>
          <div class="camera-meta">
            <el-tag :class="['status-tag', camera.status]" size="small">
              {{ statusMap[camera.status] }}
            </el-tag>
            <el-tag :type="recordTypeTagType(camera.record_type)" size="small" effect="dark">
              录像模式：{{ recordTypeLabel(camera.record_type) }}
            </el-tag>
            <span class="meta-item">{{ camera.protocol.toUpperCase() }}</span>
            <span class="meta-item">{{ camera.ip }}:{{ camera.port }}</span>
            <span class="meta-item">{{ camera.width }}×{{ camera.height }} @{{ camera.fps }}fps</span>
            <span class="meta-item">{{ camera.codec.toUpperCase() }}</span>
          </div>
        </div>
        <div class="camera-actions">
          <el-button-group>
            <el-button :type="camera.record_enabled ? 'success' : ''" @click="toggleRecord" :loading="recordLoading">
              <el-icon><VideoCamera /></el-icon>
              {{ camera.record_enabled ? '停止录像' : '开始录像' }}
            </el-button>
            <el-button @click="takeSnapshot" :loading="snapshotLoading">
              <el-icon><Camera /></el-icon> 抓拍
            </el-button>
            <el-button type="primary" @click="restartStream" :loading="restartLoading">
              <el-icon><Refresh /></el-icon> 重连流
            </el-button>
            <el-button @click="goEdit">
              <el-icon><Edit /></el-icon> 编辑摄像头
            </el-button>
            <el-button @click="goBack">
              <el-icon><ArrowLeft /></el-icon> 返回
            </el-button>
          </el-button-group>
        </div>
      </div>
    </el-card>

    <el-row :gutter="24">
      <!-- 视频预览区 -->
      <el-col :xs="24" :lg="16">
        <el-card :shadow="never" class="video-card">
          <div class="video-container aspect-16-9" ref="videoContainer">
            <!-- HLS 播放器 -->
            <video
              ref="videoPlayer"
              class="hls-video"
              playsinline
              muted
              @error="handleVideoError"
              @loadeddata="onVideoLoad"
            ></video>
            <div class="loading-overlay" v-if="videoLoading">
              <el-icon class="loading-spinner"><Loading /></el-icon>
              <p>正在连接视频流...</p>
            </div>
            <div class="error-overlay" v-if="videoError">
              <el-icon><VideoPause /></el-icon>
              <p>{{ videoError }}</p>
              <el-button type="primary" @click="initPlayer">重试</el-button>
            </div>

            <!-- 控制栏 -->
            <div class="video-controls">
              <div class="controls-left">
                <el-tooltip content="全屏" placement="top">
                  <el-button circle size="small" @click="toggleFullscreen">
                    <el-icon><FullScreen v-if="!isFullscreen" /><Fold v-else /></el-icon>
                  </el-button>
                </el-tooltip>
                <el-tooltip content="声音" placement="top">
                  <el-button circle size="small" @click="toggleMute">
                    <el-icon><VideoPlay v-if="!isMuted" /><Mute v-else /></el-icon>
                  </el-button>
                </el-tooltip>
              </div>
              <div class="controls-center">
                <span class="stream-info" v-if="streamStats">
                  {{ streamStats.bitrate }} kbps · {{ streamStats.fps }} fps · {{ streamStats.resolution }}
                </span>
              </div>
              <div class="controls-right">
                <el-tooltip content="录像" placement="top">
                  <el-button circle size="small" :type="isRecording ? 'danger' : ''" @click="toggleRecord">
                    <el-icon><VideoCamera /></el-icon>
                  </el-button>
                </el-tooltip>
                <el-tooltip content="抓拍" placement="top">
                  <el-button circle size="small" @click="takeSnapshot">
                    <el-icon><Camera /></el-icon>
                  </el-button>
                </el-tooltip>
              </div>
            </div>
          </div>
        </el-card>

        <!-- PTZ 控制 -->
        <el-card :shadow="never" class="mt-16" v-if="camera.ptz_enabled">
          <template #header>
            <h3>云台控制 (PTZ)</h3>
          </template>
          <div class="ptz-control">
            <div class="ptz-direction">
              <el-button circle size="large" @mousedown="ptzStart('up')" @mouseup="ptzStop" @mouseleave="ptzStop">
                <el-icon><ArrowUp /></el-icon>
              </el-button>
            </div>
            <div class="ptz-direction">
              <el-button circle size="large" @mousedown="ptzStart('left')" @mouseup="ptzStop" @mouseleave="ptzStop">
                <el-icon><ArrowLeft /></el-icon>
              </el-button>
              <el-button circle size="large" @click="ptzStop">
                <el-icon><VideoPause /></el-icon>
              </el-button>
              <el-button circle size="large" @mousedown="ptzStart('right')" @mouseup="ptzStop" @mouseleave="ptzStop">
                <el-icon><ArrowRight /></el-icon>
              </el-button>
            </div>
            <div class="ptz-direction">
              <el-button circle size="large" @mousedown="ptzStart('down')" @mouseup="ptzStop" @mouseleave="ptzStop">
                <el-icon><ArrowDown /></el-icon>
              </el-button>
            </div>
            <div class="ptz-zoom">
              <el-button @mousedown="ptzStart('zoom_in')" @mouseup="ptzStop" @mouseleave="ptzStop">
                <el-icon><ZoomIn /></el-icon> 放大
              </el-button>
              <el-button @mousedown="ptzStart('zoom_out')" @mouseup="ptzStop" @mouseleave="ptzStop">
                <el-icon><ZoomOut /></el-icon> 缩小
              </el-button>
            </div>
            <div class="ptz-speed">
              <el-slider v-model="ptzSpeed" :min="0.1" :max="1" :step="0.1" show-stops style="width: 200px" />
              <span>速度: {{ ptzSpeed }}</span>
            </div>
          </div>
        </el-card>
      </el-col>

      <!-- 侧边栏：信息、录像列表、抓拍 -->
      <el-col :xs="24" :lg="8">
        <!-- 基本信息 -->
        <el-card :shadow="never" class="mb-16">
          <template #header>
            <h3>基本信息</h3>
          </template>
          <div class="info-grid">
            <div class="info-item"><span class="label">设备 ID</span><span class="value">{{ camera.id }}</span></div>
            <div class="info-item"><span class="label">制造商</span><span class="value">{{ camera.manufacturer || '未知' }}</span></div>
            <div class="info-item"><span class="label">型号</span><span class="value">{{ camera.model || '未知' }}</span></div>
            <div class="info-item"><span class="label">固件版本</span><span class="value">{{ camera.firmware || '未知' }}</span></div>
            <div class="info-item"><span class="label">串号</span><span class="value">{{ camera.serial_number || '未知' }}</span></div>
            <div class="info-item"><span class="label">最后在线</span><span class="value">{{ formatTime(camera.last_online) }}</span></div>
            <div class="info-item"><span class="label">错误信息</span><span class="value error">{{ camera.error_msg || '无' }}</span></div>
          </div>
        </el-card>

        <!-- 最近录像 -->
        <el-card :shadow="never" class="mb-16">
          <template #header>
            <div class="card-header">
              <h3>最近录像</h3>
              <el-button size="small" link @click="goToPlayback">查看全部</el-button>
            </div>
          </template>
          <div class="recording-list" v-if="recentRecordings.length > 0">
            <div class="recording-item" v-for="rec in recentRecordings" :key="rec.id" @click="playRecording(rec)">
              <div class="recording-thumb">
                <el-icon><VideoPlay /></el-icon>
              </div>
              <div class="recording-info">
                <p class="recording-time">{{ formatTime(rec.start_time) }} - {{ formatTime(rec.end_time) }}</p>
                <p class="recording-duration">{{ formatDuration(rec.duration) }} · {{ formatBytes(rec.file_size) }}</p>
              </div>
              <el-tag :type="rec.record_type === 'motion' ? 'warning' : 'info'" size="small">
                {{ rec.record_type }}
              </el-tag>
            </div>
          </div>
          <div class="empty-state" v-else>
            <el-icon><Film /></el-icon>
            <p>暂无录像</p>
          </div>
        </el-card>

        <!-- 最近抓拍 -->
        <el-card :shadow="never">
          <template #header>
            <div class="card-header">
              <h3>最近抓拍</h3>
              <el-button size="small" link>查看全部</el-button>
            </div>
          </template>
          <div class="snapshot-grid" v-if="recentSnapshots.length > 0">
            <div class="snapshot-item" v-for="snap in recentSnapshots" :key="snap.id">
              <img :src="getSnapshotUrl(snap.file_path)" :alt="snap.timestamp" @error="handleImageError($event)" />
              <div class="snapshot-meta">
                <span>{{ formatTime(snap.timestamp) }}</span>
                <el-tag size="small" :type="snap.type === 'motion' ? 'warning' : 'info'">{{ snap.type }}</el-tag>
              </div>
            </div>
          </div>
          <div class="empty-state" v-else>
            <el-icon><Picture /></el-icon>
            <p>暂无抓拍</p>
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
  <div class="loading-full" v-else>
    <el-icon class="loading-spinner"><Loading /></el-icon>
    <p>加载摄像头信息中...</p>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  VideoCamera, Camera, Refresh, ArrowLeft, ArrowUp, ArrowDown,
  ArrowRight, VideoPause, ZoomIn, ZoomOut,
  FullScreen, Fold, VideoPlay, Mute, Loading,
  Picture, Edit
} from '@element-plus/icons-vue'
import Hls from 'hls.js'
import { api } from '@/api'
import { useCameraStore } from '@/stores'

const route = useRoute()
const router = useRouter()
const cameraStore = useCameraStore()

const camera = ref<any>(null)
const videoContainer = ref<HTMLElement>()
const videoPlayer = ref<HTMLVideoElement>()
const videoLoading = ref(true)
const videoError = ref<string>('')
const isFullscreen = ref(false)
const isMuted = ref(true)
const isRecording = ref(false)
const recordLoading = ref(false)
const snapshotLoading = ref(false)
const restartLoading = ref(false)
const streamStats = ref<any>(null)

const recentRecordings = ref<any[]>([])
const recentSnapshots = ref<any[]>([])

const ptzSpeed = ref(0.5)
const ptzTimer = ref<any>(null)

const statusMap = { online: '在线', offline: '离线', error: '异常' }

// 录像模式显示（详情页展示摄像头当前录像模式）
const recordTypeLabel = (t: string) => {
  const m: Record<string, string> = { continuous: '连续录像', motion: '移动侦测录像', schedule: '定时录像' }
  return m[t] || t || '连续录像'
}
const recordTypeTagType = (t: string) => {
  const m: Record<string, string> = { continuous: 'success', motion: 'warning', schedule: 'primary' }
  return m[t] || 'info'
}
const goEdit = () => {
  if (camera.value) router.push(`/cameras/edit/${camera.value.id}`)
}

const formatTime = (time: string) => time ? new Date(time).toLocaleString('zh-CN') : '-'
const formatDuration = (sec: number) => {
  const m = Math.floor(sec / 60), s = sec % 60
  return `${m}分${s}秒`
}
const formatBytes = (bytes: number) => {
  if (!bytes) return '0 B'
  const k = 1024, sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

const getSnapshotUrl = (path: string) => {
  // 文件路径 recordings/camera_N/snapshot_N_XXX.jpg
  // -> /api/v1/stream/camera/N/snapshots/snapshot_N_XXX.jpg
  const name = (path || '').split('/').pop()
  if (!camera.value) return ''
  const token = localStorage.getItem('token') || ''
  const q = token ? `?token=${encodeURIComponent(token)}` : ''
  return `/api/v1/stream/camera/${camera.value.id}/snapshots/${name}${q}`
}

const loadCameraDetail = async () => {
  try {
    const res = await api.cameras.get(Number(route.params.id))
    camera.value = res.data || res
    isRecording.value = camera.value.record_enabled
    // 等待 v-if="camera" 区域渲染出 <video> 元素后再初始化播放器
    await nextTick()
    initPlayer()
    loadRecentData()
  } catch (e) {
    ElMessage.error('获取摄像头详情失败')
    router.push('/cameras')
  }
}

const loadRecentData = async () => {
  try {
    const [recRes, snapRes] = await Promise.all([
      api.recordings.byCamera(camera.value.id, { page: 1, page_size: 10 }),
      api.cameras.snapshots(camera.value.id, { page: 1, page_size: 12 }),
    ])
    recentRecordings.value = recRes.data || recRes || []
    recentSnapshots.value = snapRes.data || snapRes || []
  } catch (e) { console.error(e) }
}

let hls: Hls | null = null

const initPlayer = async () => {
  if (!camera.value || !videoPlayer.value) return

  videoLoading.value = true
  videoError.value = ''

  // 销毁旧实例
  if (hls) {
    hls.destroy()
    hls = null
  }

  const hlsUrl = api.stream.hlsPlaylist(camera.value.id)

  if (Hls.isSupported()) {
    hls = new Hls({
      enableWorker: true,
      lowLatencyMode: true,
      maxBufferLength: 30,
      maxMaxBufferLength: 60,
    })
    hls.loadSource(hlsUrl)
    hls.attachMedia(videoPlayer.value)

    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      videoPlayer.value?.play().catch(() => {})
      videoLoading.value = false
    })

    hls.on(Hls.Events.ERROR, (_, data) => {
      if (data.fatal) {
        videoError.value = '视频流加载失败，请检查摄像头连接'
        videoLoading.value = false
        hls?.destroy()
        hls = null
      }
    })

    // 监控统计
    setInterval(() => {
      if (hls && videoPlayer.value && !videoPlayer.value.paused) {
        const bandwidth = hls.bandwidthEstimate
        streamStats.value = {
          bitrate: bandwidth ? Math.round(bandwidth / 1000) : 0,
          fps: 25,
          resolution: `${camera.value.width}×${camera.value.height}`,
        }
      }
    }, 2000)
  } else if (videoPlayer.value.canPlayType('application/vnd.apple.mpegurl')) {
    videoPlayer.value.src = hlsUrl
    videoPlayer.value.addEventListener('loadedmetadata', () => {
      videoPlayer.value?.play().catch(() => {})
      videoLoading.value = false
    })
  } else {
    videoError.value = '浏览器不支持 HLS 播放，请使用现代浏览器'
    videoLoading.value = false
  }
}

const onVideoLoad = () => { videoLoading.value = false }
const handleVideoError = () => { videoError.value = '视频播放出错，请尝试重连' }

const toggleFullscreen = () => {
  if (!videoContainer.value) return
  if (!isFullscreen.value) {
    videoContainer.value.requestFullscreen?.()
  } else {
    document.exitFullscreen?.()
  }
  isFullscreen.value = !isFullscreen.value
}

const toggleMute = () => {
  if (videoPlayer.value) {
    videoPlayer.value.muted = !videoPlayer.value.muted
    isMuted.value = videoPlayer.value.muted
  }
}

const toggleRecord = async () => {
  if (!camera.value) return
  recordLoading.value = true
  try {
    // 传完整对象，避免后端 ShouldBindJSON 将未提交字段置零
    await api.cameras.update(camera.value.id, { ...camera.value, record_enabled: !camera.value.record_enabled })
    camera.value.record_enabled = !camera.value.record_enabled
    isRecording.value = camera.value.record_enabled
    ElMessage.success(isRecording.value ? '已开始录像' : '已停止录像')
  } catch (e) { ElMessage.error('操作失败') }
  finally { recordLoading.value = false }
}

const takeSnapshot = async () => {
  if (!camera.value) return
  snapshotLoading.value = true
  try {
    await api.cameras.snapshot(camera.value.id)
    ElMessage.success('抓拍成功')
    loadRecentData()
  } catch (e) { ElMessage.error('抓拍失败') }
  finally { snapshotLoading.value = false }
}

const restartStream = async () => {
  if (!camera.value) return
  restartLoading.value = true
  try {
    await api.cameras.restart(camera.value.id)
    ElMessage.success('重连请求已发送')
    setTimeout(initPlayer, 3000)
  } catch (e) { ElMessage.error('重连失败') }
  finally { restartLoading.value = false }
}

const ptzStart = (command: string) => {
  if (!camera.value) return
  ptzTimer.value = setInterval(async () => {
    try {
      await api.cameras.ptz(camera.value.id, command, ptzSpeed.value)
    } catch (e) { ptzStop() }
  }, 200)
}

const ptzStop = () => {
  if (ptzTimer.value) {
    clearInterval(ptzTimer.value)
    ptzTimer.value = null
  }
  if (camera.value) {
    api.cameras.ptz(camera.value.id, 'stop', 0).catch(() => {})
  }
}

const playRecording = (rec: any) => {
  router.push(`/recordings/playback/${camera.value.id}?recordingId=${rec.id}`)
}

const goToPlayback = () => router.push(`/recordings/playback/${camera.value.id}`)
const goBack = () => router.push('/cameras')

const handleImageError = (e: Event) => { (e.target as HTMLImageElement).style.display = 'none' }

onMounted(() => { loadCameraDetail() })
onUnmounted(() => { if (hls) { hls.destroy(); hls = null } ptzStop() })
</script>

<style scoped lang="scss">
.camera-detail-page {
  .camera-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    flex-wrap: wrap;
    gap: 16px;

    .camera-basic h2 { margin: 0 0 8px; font-size: 20px; font-weight: 600; }
    .camera-meta { display: flex; flex-wrap: wrap; gap: 12px; }
    .meta-item { font-size: 13px; color: #909399; }
  }

  .video-card {
    .video-container { position: relative; background: #000; border-radius: 8px; overflow: hidden; }
    .loading-overlay, .error-overlay {
      position: absolute; inset: 0; background: rgba(0,0,0,0.7);
      display: flex; flex-direction: column; align-items: center; justify-content: center;
      color: #fff; gap: 16px; z-index: 10;
      .loading-spinner { font-size: 32px; animation: spin 1s linear infinite; }
      @keyframes spin { to { transform: rotate(360deg); } }
    }
    .error-overlay { text-align: center; p { margin: 0 0 16px; } }

    .video-controls {
      position: absolute; bottom: 0; left: 0; right: 0;
      display: flex; justify-content: space-between; align-items: center;
      padding: 8px 12px;
      background: linear-gradient(transparent, rgba(0,0,0,0.6));
      z-index: 5;

      .controls-left, .controls-center, .controls-right { display: flex; align-items: center; gap: 8px; }
      .stream-info { font-size: 12px; color: #fff; background: rgba(0,0,0,0.5); padding: 2px 8px; border-radius: 4px; }
    }
  }

  .ptz-control {
    display: flex; flex-direction: column; align-items: center; gap: 16px; padding: 8px;
    .ptz-direction { display: flex; justify-content: center; gap: 8px; }
    .ptz-zoom { display: flex; gap: 12px; }
    .ptz-speed { display: flex; align-items: center; gap: 12px; color: #909399; font-size: 13px; }
  }

  .info-grid {
    display: grid; grid-template-columns: 1fr 1fr; gap: 12px;
    .info-item { display: flex; flex-direction: column; gap: 4px; padding: 8px 0; border-bottom: 1px solid #f0f0f0; }
    .label { font-size: 12px; color: #909399; }
    .value { font-size: 13px; color: #303133; word-break: break-all; &.error { color: #f56c6c; } }
  }

  .recording-list, .snapshot-grid {
    .recording-item { display: flex; align-items: center; gap: 12px; padding: 12px 0; border-bottom: 1px solid #f0f0f0; cursor: pointer; transition: background 0.2s; &:hover { background: #fafafa; } }
    .recording-thumb { width: 48px; height: 36px; background: #f0f0f0; border-radius: 4px; display: flex; align-items: center; justify-content: center; color: #909399; }
    .recording-info { flex: 1; min-width: 0; .recording-time { margin: 0 0 4px; font-size: 13px; color: #303133; } .recording-duration { margin: 0; font-size: 12px; color: #909399; } }
    .snapshot-item { position: relative; aspect-ratio: 4/3; border-radius: 8px; overflow: hidden; img { width: 100%; height: 100%; object-fit: cover; transition: transform 0.3s; &:hover { transform: scale(1.05); } } .snapshot-meta { position: absolute; bottom: 0; left: 0; right: 0; padding: 8px; background: linear-gradient(transparent, rgba(0,0,0,0.7)); color: #fff; font-size: 11px; display: flex; justify-content: space-between; align-items: center; } }
  }

  .empty-state { display: flex; flex-direction: column; align-items: center; justify-content: center; padding: 40px 20px; color: #909399; font-size: 14px; .el-icon { font-size: 32px; margin-bottom: 12px; opacity: 0.5; } }
}

.loading-full { display: flex; flex-direction: column; align-items: center; justify-content: center; height: 50vh; color: #909399; .loading-spinner { font-size: 32px; animation: spin 1s linear infinite; } @keyframes spin { to { transform: rotate(360deg); } } }
</style>
<template>
  <div class="playback-page" v-if="camera">
    <!-- 顶部栏 -->
    <el-card :shadow="never" class="mb-16">
      <div class="playback-header">
        <div class="camera-info">
          <el-button link @click="goBack"><el-icon><ArrowLeft /></el-icon></el-button>
          <h2>{{ camera.name }} - 历史回放</h2>
        </div>
        <div class="playback-controls">
          <el-date-picker v-model="playDate" type="date" placeholder="选择日期" value-format="YYYY-MM-DD" style="width: 160px" />
          <el-button :type="isPlaying ? 'success' : 'primary'" @click="togglePlay" :loading="playLoading">
            <el-icon><VideoPlay v-if="!isPlaying" /><VideoPause v-else /></el-icon>
            {{ isPlaying ? '暂停' : '播放' }}
          </el-button>
          <el-button @click="stopPlay"><el-icon><VideoPause /></el-icon> 停止</el-button>
          <el-button @click="downloadCurrent"><el-icon><Download /></el-icon> 下载</el-button>
        </div>
      </div>
    </el-card>

    <el-row :gutter="24">
      <!-- 视频播放区 -->
      <el-col :xs="24" :lg="16">
        <el-card :shadow="never" class="video-card">
          <div class="video-container aspect-16-9" ref="videoContainer">
            <video
              ref="videoPlayer"
              class="hls-video"
              playsinline
              controls
              @timeupdate="onTimeUpdate"
              @error="handleVideoError"
            ></video>
            <div class="loading-overlay" v-if="videoLoading">
              <el-icon class="loading-spinner"><Loading /></el-icon>
              <p>{{ loadingText }}</p>
            </div>
            <div class="video-placeholder" v-if="!currentSegment && !currentWebdav && !videoLoading">
              <el-icon><Film /></el-icon>
              <p>请从右侧列表或时间轴选择一个录像片段开始播放</p>
            </div>

            <!-- 进度条 -->
            <div class="playback-progress" @click="seek($event)">
              <div class="progress-track">
                <div class="progress-buffer" :style="{ width: bufferPercent + '%' }" />
                <div class="progress-played" :style="{ width: playedPercent + '%' }" />
                <div class="progress-thumb" :style="{ left: playedPercent + '%' }" v-show="isDragging || hoverProgress" />
              </div>
              <div class="progress-time">
                <span>{{ formatTime(currentTime) }}</span>
                <span>/</span>
                <span>{{ formatTime(duration) }}</span>
              </div>
            </div>
          </div>
        </el-card>

        <!-- 录像片段时间轴 -->
        <el-card :shadow="never" class="mt-16">
          <template #header>
            <h3>录像片段时间轴</h3>
          </template>
          <div class="timeline">
            <div class="timeline-track">
              <div class="timeline-segment" v-for="seg in segments" :key="seg.id" :style="segmentStyle(seg)" :class="{ 'segment-motion': seg.record_type === 'motion' }" @click="jumpToSegment(seg)">
                <div class="segment-indicator" />
              </div>
              <div class="timeline-cursor" :style="{ left: cursorPercent + '%' }" />
            </div>
            <div class="timeline-labels">
              <span v-for="i in 24" :key="i" :style="{ left: (i-1)/23*100 + '%' }">{{ String(i-1).padStart(2,'0') }}:00</span>
            </div>
          </div>
        </el-card>
      </el-col>

      <!-- 侧边栏：录像列表 -->
      <el-col :xs="24" :lg="8">
        <el-card :shadow="never">
          <template #header>
            <div class="card-header">
              <h3>当日录像列表</h3>
              <el-button size="small" @click="loadSegments"><el-icon><Refresh /></el-icon></el-button>
            </div>
          </template>

          <div class="segment-list" v-if="segments.length > 0">
            <div class="segment-item" v-for="seg in segments" :key="seg.id" :class="{ active: currentSegment?.id === seg.id }" @click="jumpToSegment(seg)">
              <div class="segment-type" :class="seg.record_type">
                {{ typeLabels[seg.record_type] }}
              </div>
              <div class="segment-info">
                <p class="segment-time">{{ formatTime(seg.start_time) }} - {{ formatTime(seg.end_time) }}</p>
                <p class="segment-duration">{{ formatDuration(seg.duration) }} · {{ formatBytes(seg.file_size) }}</p>
              </div>
              <el-icon v-if="currentSegment?.id === seg.id"><VideoPlay class="playing" /></el-icon>
            </div>
          </div>
          <div class="empty-state" v-else>
            <el-icon><Film /></el-icon>
            <p>该日期暂无录像</p>
          </div>
        </el-card>

        <!-- WebDAV 云存储录像 -->
        <el-card :shadow="never" class="mt-16">
          <template #header>
            <div class="card-header">
              <h3><el-icon><Cloudy /></el-icon> WebDAV 云录像</h3>
              <el-button size="small" @click="loadWebdavFiles" :loading="webdavLoading">
                <el-icon><Refresh /></el-icon>
              </el-button>
            </div>
          </template>
          <p class="webdav-hint" v-if="webdavEnabled === false">WebDAV 未启用（可在 存储管理 中配置）</p>
          <div class="segment-list" v-else-if="webdavFiles.length > 0">
            <div class="segment-item" v-for="f in webdavFiles" :key="f.path" :class="{ active: currentWebdav === f.path }" @click="playWebdavFile(f)">
              <div class="segment-type" :class="f.name.startsWith('motion_') ? 'motion' : 'continuous'">
                {{ f.name.startsWith('motion_') ? '移动' : '连续' }}
              </div>
              <div class="segment-info">
                <p class="segment-time webdav-file-name" :title="f.name">{{ f.name }}</p>
                <p class="segment-duration">{{ f.mod_time ? new Date(f.mod_time).toLocaleString('zh-CN') : '' }} · {{ formatBytes(f.size) }}</p>
              </div>
              <el-icon v-if="currentWebdav === f.path"><VideoPlay class="playing" /></el-icon>
            </div>
          </div>
          <div class="empty-state" v-else>
            <el-icon><Cloudy /></el-icon>
            <p>WebDAV 上暂无该摄像头的录像</p>
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
  <div class="loading-full" v-else>
    <el-icon class="loading-spinner"><Loading /></el-icon>
    <p>加载中...</p>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { ArrowLeft, VideoPlay, VideoPause, Download, Refresh, Loading, Film, Cloudy } from '@element-plus/icons-vue'
import { api } from '@/api'
import { useCameraStore } from '@/stores'

const route = useRoute()
const router = useRouter()
const cameraStore = useCameraStore()

const camera = ref<any>(null)
const videoContainer = ref<HTMLElement>()
const videoPlayer = ref<HTMLVideoElement>()
const videoLoading = ref(false)
const loadingText = ref('')
const isPlaying = ref(false)
const playLoading = ref(false)

// 默认今天（本地时区！toISOString 是 UTC，凌晨会错成昨天）
const today = new Date()
const pad2 = (n: number) => String(n).padStart(2, '0')
const playDate = ref(`${today.getFullYear()}-${pad2(today.getMonth() + 1)}-${pad2(today.getDate())}`)
const segments = ref<any[]>([])
const currentSegment = ref<any>(null)

// WebDAV 云录像
const webdavEnabled = ref<boolean | null>(null)
const webdavFiles = ref<any[]>([])
const webdavLoading = ref(false)
const currentWebdav = ref<string | null>(null)
const currentTime = ref(0)
const duration = ref(0)
const bufferPercent = ref(0)
const playedPercent = ref(0)
const isDragging = ref(false)
const hoverProgress = ref(false)
const cursorPercent = ref(0)

let segmentCheckTimer: any = null

const typeLabels = { continuous: '连续', motion: '移动', schedule: '定时', manual: '手动' }
const formatTime = (time: string | number) => {
  if (typeof time === 'number') { const m = Math.floor(time/60), s = Math.floor(time%60); return `${String(m).padStart(2,'0')}:${String(s).padStart(2,'0')}` }
  return time ? new Date(time).toLocaleTimeString('zh-CN', {hour:'2-digit',minute:'2-digit',second:'2-digit'}) : '00:00:00'
}
const formatDuration = (sec: number) => { const h = Math.floor(sec/3600), m = Math.floor((sec%3600)/60); return h>0?`${h}h${m}m`:`${m}m` }
const formatBytes = (bytes: number) => { if(!bytes) return '0 B'; const k=1024,sizes=['B','KB','MB','GB']; const i=Math.floor(Math.log(bytes)/Math.log(k)); return parseFloat((bytes/Math.pow(k,i)).toFixed(1))+' '+sizes[i] }

const loadCamera = async () => {
  try {
    const res = await api.cameras.get(Number(route.params.cameraId))
    camera.value = res.data || res
    loadSegments()
    loadWebdavFiles()
  } catch (e) { ElMessage.error('获取摄像头失败'); router.push('/recordings') }
}

const loadSegments = async () => {
  if (!camera.value) return
  try {
    const start = `${playDate.value} 00:00:00`
    const end = `${playDate.value} 23:59:59`
    const res = await api.recordings.segments(camera.value.id, start, end)
    segments.value = res.data || res || []
  } catch (e) { ElMessage.error('获取录像片段失败') }
}

// 加载 WebDAV 上该摄像头的录像文件
const loadWebdavFiles = async () => {
  if (!camera.value) return
  webdavLoading.value = true
  try {
    const res: any = await api.webdav.list(camera.value.id)
    webdavEnabled.value = res.enabled !== false
    webdavFiles.value = (res.files || []).map((f: any) => ({
      name: f.name, path: f.path, size: f.size,
      mod_time: f.mod_time ? new Date(f.mod_time) : null,
    }))
  } catch (e) {
    webdavEnabled.value = null
    webdavFiles.value = []
  } finally {
    webdavLoading.value = false
  }
}

// 播放 WebDAV 上的文件（流式代理，支持 Range 拖动）
const playWebdavFile = (f: any) => {
  currentWebdav.value = f.path
  currentSegment.value = null
  const video = videoPlayer.value
  if (!video) return
  videoLoading.value = true
  loadingText.value = '正在从 WebDAV 加载录像文件...'
  isPlaying.value = false
  video.pause()
  video.removeAttribute('src')
  video.load()
  video.src = api.webdav.fileUrl(f.path)
  video.onloadedmetadata = () => {
    videoLoading.value = false
    video.play().catch(() => {})
    isPlaying.value = true
  }
}

const jumpToSegment = (seg: any) => {
  currentSegment.value = seg
  playSegment(seg)
}

const playSegment = (seg: any) => {
  const video = videoPlayer.value
  if (!video) return
  currentWebdav.value = null
  videoLoading.value = true
  loadingText.value = '正在加载录像文件...'
  isPlaying.value = false
  video.pause()

  // 先释放旧源再设置新源（保证重复点击同一片段也能重新加载）
  video.removeAttribute('src')
  video.load()

  // 每个录像分段是独立 MP4 文件，浏览器原生播放（服务端支持 Range，可拖动进度）
  video.src = api.recordings.file(seg.id)
  video.onloadedmetadata = () => {
    videoLoading.value = false
    video.play().catch(() => {})
    isPlaying.value = true
  }
}

const onTimeUpdate = () => {
  if (!videoPlayer.value) return
  currentTime.value = videoPlayer.value.currentTime
  duration.value = videoPlayer.value.duration || 0
  playedPercent.value = duration.value ? (currentTime.value / duration.value) * 100 : 0
  bufferPercent.value = videoPlayer.value.buffered.length ? (videoPlayer.value.buffered.end(0) / duration.value) * 100 : 0

  // 时间轴游标：把"正在播放的时间点"映射到当天 24 小时时间轴上
  if (currentSegment.value && duration.value) {
    const segStartMs = new Date(currentSegment.value.start_time).getTime()
    const posMs = segStartMs + videoPlayer.value.currentTime * 1000
    const dayStartMs = new Date(`${playDate.value}T00:00:00`).getTime()
    cursorPercent.value = Math.max(0, Math.min(100, ((posMs - dayStartMs) / 86400000) * 100))
  }
}

const seek = (e: MouseEvent) => {
  if (!videoPlayer.value || !videoContainer.value) return
  const rect = videoContainer.value.getBoundingClientRect()
  const percent = (e.clientX - rect.left) / rect.width
  videoPlayer.value.currentTime = percent * (videoPlayer.value.duration || 0)
}

const togglePlay = async () => {
  if (!videoPlayer.value) return
  playLoading.value = true
  try {
    if (isPlaying.value) { await videoPlayer.value.pause() } else { await videoPlayer.value.play() }
    isPlaying.value = !isPlaying.value
  } catch (e) { ElMessage.error('操作失败') }
  finally { playLoading.value = false }
}

const stopPlay = () => {
  if (videoPlayer.value) {
    videoPlayer.value.pause()
    videoPlayer.value.removeAttribute('src')
    videoPlayer.value.load()
    videoPlayer.value.currentTime = 0
  }
  isPlaying.value = false
  currentSegment.value = null
  currentWebdav.value = null
}

const downloadCurrent = async () => {
  if (!currentSegment.value && !currentWebdav.value) { ElMessage.warning('请先选择录像片段'); return }
  try {
    // WebDAV 文件：直接按路径下载
    if (currentWebdav.value) {
      const name = currentWebdav.value.split('/').pop() || 'webdav_file.mp4'
      const a = document.createElement('a')
      a.href = api.webdav.fileUrl(currentWebdav.value)
      a.download = name
      a.target = '_blank'
      a.click()
      return
    }
    const res = await api.recordings.download(currentSegment.value.id)
    const url = window.URL.createObjectURL(new Blob([res]))
    const a = document.createElement('a')
    a.href = url; a.download = `playback_${currentSegment.value.id}.mp4`; a.click()
    URL.revokeObjectURL(url)
  } catch (e) { ElMessage.error('下载失败') }
}

const handleVideoError = () => { videoLoading.value = false; loadingText.value = '视频加载失败，请重试' }

const segmentStyle = (seg: any) => {
  const s = new Date(seg.start_time)
  const e = new Date(seg.end_time)
  let startH = s.getHours() + s.getMinutes() / 60 + s.getSeconds() / 3600
  let endH = e.getHours() + e.getMinutes() / 60 + e.getSeconds() / 3600
  if (endH <= startH) endH = startH + 0.02 // 跨午夜/超短片段：显示最小宽度标记
  return { left: `${startH / 24 * 100}%`, width: `${Math.max(0.3, (endH - startH) / 24 * 100)}%` }
}

const goBack = () => router.push('/recordings')

watch(playDate, loadSegments)

onMounted(() => { loadCamera() })
onUnmounted(() => { if (segmentCheckTimer) clearInterval(segmentCheckTimer) })
</script>

<style scoped lang="scss">
.playback-page {
  .playback-header { display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:16px; .camera-info{display:flex;align-items:center;gap:12px; h2{margin:0;font-size:20px;font-weight:600;}} .playback-controls{display:flex;gap:12px;align-items:center;} }
  .video-card { .video-container{position:relative;background:#000;border-radius:8px;overflow:hidden;} .loading-overlay{position:absolute;inset:0;background:rgba(0,0,0,0.7);display:flex;flex-direction:column;align-items:center;justify-content:center;color:#fff;gap:16px;z-index:10;.loading-spinner{font-size:32px;animation:spin 1s linear infinite;}} .video-placeholder{position:absolute;inset:0;display:flex;flex-direction:column;align-items:center;justify-content:center;color:#909399;gap:12px;z-index:4;font-size:13px;.el-icon{font-size:36px;opacity:0.5;}} .playback-progress{position:absolute;bottom:0;left:0;right:0;padding:8px 12px;background:linear-gradient(transparent,rgba(0,0,0,0.6));display:flex;flex-direction:column;gap:4px;z-index:5;pointer-events:none; .progress-track{height:4px;background:rgba(255,255,255,0.3);border-radius:2px;position:relative;cursor:pointer;pointer-events:auto; .progress-buffer{position:absolute;top:0;left:0;height:100%;background:rgba(255,255,255,0.4);border-radius:2px;} .progress-played{position:absolute;top:0;left:0;height:100%;background:#409eff;border-radius:2px;transition:width 0.1s;} .progress-thumb{position:absolute;top:-6px;width:16px;height:16px;background:#fff;border-radius:50%;transform:translateX(-50%);box-shadow:0 2px 6px rgba(0,0,0,0.3);} &:hover .progress-thumb{display:block;} } .progress-time{display:flex;justify-content:space-between;color:#fff;font-size:12px;} } }
  .timeline { .timeline-track{position:relative;height:40px;background:#f5f7fa;border-radius:8px;overflow:hidden; .timeline-segment{position:absolute;top:0;height:100%;background:#409eff;border-radius:4px;cursor:pointer;transition:all 0.2s;&:hover{opacity:0.8;transform:scaleY(1.2);}&.segment-motion{background:#f56c6c;} .segment-indicator{position:absolute;top:-4px;left:50%;width:8px;height:8px;background:#fff;border-radius:50%;transform:translateX(-50%);box-shadow:0 1px 3px rgba(0,0,0,0.2);} } .timeline-cursor{position:absolute;top:0;bottom:0;width:2px;background:#f56c6c;pointer-events:none;z-index:10;} } .timeline-labels{display:flex;justify-content:space-between;margin-top:8px;font-size:11px;color:#909399;} }
  .segment-list { .segment-item{display:flex;align-items:center;gap:12px;padding:12px;border-radius:8px;cursor:pointer;transition:background 0.2s;border:1px solid transparent;&:hover{background:#fafafa;border-color:#e6e9ed;}&.active{background:#eef7ff;border-color:#409eff;} .segment-type{width:48px;height:20px;border-radius:10px;display:flex;align-items:center;justify-content:center;font-size:10px;color:#fff;&.continuous{background:#409eff;}&.motion{background:#f56c6c;}&.schedule{background:#67c23a;}&.manual{background:#909399;} } .segment-info{flex:1;min-width:0;.segment-time{margin:0 0 2px;font-size:13px;color:#303133;}.segment-duration{margin:0;font-size:12px;color:#909399;}.webdav-file-name{font-size:12px;word-break:break-all;} } .playing{color:#409eff;animation:pulse 1s infinite;} }
  .webdav-hint{margin:0;padding:16px 0;font-size:13px;color:#909399;text-align:center;} .empty-state{display:flex;flex-direction:column;align-items:center;justify-content:center;padding:40px 20px;color:#909399;font-size:14px;.el-icon{font-size:32px;margin-bottom:12px;opacity:0.5;}} }
  .loading-full{display:flex;flex-direction:column;align-items:center;justify-content:center;height:50vh;color:#909399;.loading-spinner{font-size:32px;animation:spin 1s linear infinite;}}
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}
</style>
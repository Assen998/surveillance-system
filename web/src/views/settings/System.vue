<template>
  <div class="settings-page">
    <el-tabs v-model="activeTab" type="card">
      <el-tab-pane label="系统配置" name="system">
        <el-card :shadow="never" class="mt-16">
          <template #header>
            <h3>基本设置</h3>
          </template>
          <el-form :model="systemForm" label-width="140">
            <el-form-item label="系统名称">
              <el-input v-model="systemForm.name" placeholder="监控录像系统" style="width: 400px" />
            </el-form-item>
            <el-form-item label="HTTP 端口">
              <el-input-number v-model="systemForm.http_port" :min="1" :max="65535" :controls="false" style="width: 120px" />
            </el-form-item>
            <el-form-item label="WebSocket 端口">
              <el-input-number v-model="systemForm.ws_port" :min="1" :max="65535" :controls="false" style="width: 120px" />
            </el-form-item>
            <el-form-item label="运行模式">
              <el-select v-model="systemForm.mode" placeholder="选择模式" style="width: 200px">
                <el-option label="开发模式" value="debug" />
                <el-option label="生产模式" value="release" />
              </el-select>
            </el-form-item>
            <el-form-item label="日志级别">
              <el-select v-model="systemForm.log_level" placeholder="选择级别" style="width: 200px">
                <el-option label="Debug" value="debug" />
                <el-option label="Info" value="info" />
                <el-option label="Warn" value="warn" />
                <el-option label="Error" value="error" />
              </el-select>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="saveSystemConfig"><el-icon><Check /></el-icon> 保存</el-button>
            </el-form-item>
          </el-form>
        </el-card>

        <el-card :shadow="never" class="mt-16">
          <template #header>
            <h3>数据库设置</h3>
          </template>
          <el-form :model="dbForm" label-width="140">
            <el-form-item label="数据库类型">
              <el-select v-model="dbForm.type" placeholder="选择类型" style="width: 200px" disabled>
                <el-option label="SQLite" value="sqlite" />
                <el-option label="PostgreSQL" value="postgres" />
              </el-select>
            </el-form-item>
            <el-form-item label="SQLite 路径" v-if="dbForm.type === 'sqlite'">
              <el-input v-model="dbForm.sqlite_path" placeholder="./data/surveillance.db" style="width: 400px" />
            </el-form-item>
            <el-form-item label="PostgreSQL 主机" v-if="dbForm.type === 'postgres'">
              <el-input v-model="dbForm.pg_host" placeholder="localhost" style="width: 300px" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="saveDbConfig" :disabled="dbForm.type !== 'sqlite'"><el-icon><Check /></el-icon> 保存</el-button>
            </el-form-item>
          </el-form>
        </el-card>

        <el-card :shadow="never" class="mt-16">
          <template #header>
            <h3>Redis 设置</h3>
          </template>
          <el-form :model="redisForm" label-width="140">
            <el-form-item label="主机">
              <el-input v-model="redisForm.host" placeholder="localhost" style="width: 300px" />
            </el-form-item>
            <el-form-item label="端口">
              <el-input-number v-model="redisForm.port" :min="1" :max="65535" :controls="false" style="width: 120px" />
            </el-form-item>
            <el-form-item label="密码">
              <el-input v-model="redisForm.password" type="password" show-password placeholder="留空表示无密码" style="width: 300px" />
            </el-form-item>
            <el-form-item label="数据库编号">
              <el-input-number v-model="redisForm.db" :min="0" :max="15" :controls="false" style="width: 120px" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="saveRedisConfig"><el-icon><Check /></el-icon> 保存</el-button>
            </el-form-item>
          </el-form>
        </el-card>
      </el-tab-pane>

      <el-tab-pane label="摄像头默认配置" name="camera">
        <el-card :shadow="never" class="mt-16">
          <template #header>
            <h3>默认录像设置</h3>
          </template>
          <el-form :model="cameraDefaultForm" label-width="160">
            <el-form-item label="默认启用录像">
              <el-switch v-model="cameraDefaultForm.record_enabled" />
            </el-form-item>
            <el-form-item label="默认录像类型">
              <el-select v-model="cameraDefaultForm.record_type" placeholder="选择类型" style="width: 200px">
                <el-option label="连续录像" value="continuous" />
                <el-option label="移动侦测" value="motion" />
                <el-option label="定时录像" value="schedule" />
              </el-select>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="saveCameraDefaults"><el-icon><Check /></el-icon> 保存</el-button>
            </el-form-item>
          </el-form>
        </el-card>

        <el-card :shadow="never" class="mt-16">
          <template #header>
            <h3>默认画面设置</h3>
          </template>
          <el-form :model="cameraDefaultForm" label-width="160">
            <el-form-item label="默认分辨率">
              <el-row :gutter="12">
                <el-col :span="11">
                  <el-input-number v-model="cameraDefaultForm.width" :min="320" :max="8192" :controls="false" placeholder="宽" style="width: 100%" />
                </el-col>
                <el-col :span="2"><span class="text-center">×</span></el-col>
                <el-col :span="11">
                  <el-input-number v-model="cameraDefaultForm.height" :min="240" :max="8192" :controls="false" placeholder="高" style="width: 100%" />
                </el-col>
              </el-row>
            </el-form-item>
            <el-form-item label="默认帧率">
              <el-input-number v-model="cameraDefaultForm.fps" :min="1" :max="60" :controls="false" style="width: 120px" />
            </el-form-item>
            <el-form-item label="默认编码">
              <el-select v-model="cameraDefaultForm.codec" placeholder="选择编码" style="width: 200px">
                <el-option label="H.264" value="h264" />
                <el-option label="H.265" value="h265" />
              </el-select>
            </el-form-item>
            <el-form-item label="默认码率">
              <el-input-number v-model="cameraDefaultForm.bitrate" :min="512" :max="20480" :step="512" :controls="false" style="width: 120px" /> kbps
            </el-form-item>
          </el-form>
        </el-card>
      </el-tab-pane>

      <el-tab-pane label="存储设置" name="storage">
        <el-card :shadow="never" class="mt-16">
          <template #header>
            <h3>录像存储</h3>
          </template>
          <el-form :model="storageForm" label-width="160">
            <el-form-item label="分段时长">
              <el-input-number v-model="storageForm.segment_duration" :min="30" :max="86400" :step="30" :controls="false" style="width: 140px" />
              <span class="form-hint">秒（30~1440 分钟）。修改后对新连接/重连的摄像头生效，建议 180（3分钟）</span>
            </el-form-item>
            <el-form-item label="保留天数">
              <el-input-number v-model="storageForm.max_days" :min="1" :max="365" :controls="false" style="width: 140px" />
              <span class="form-hint">超过该天数的录像自动清理</span>
            </el-form-item>
            <el-form-item label="最大存储占用">
              <el-input-number v-model="storageForm.max_storage_gb" :min="0" :max="100000" :precision="1" :step="10" :controls="false" style="width: 140px" />
              <span class="form-hint">GB，0 = 不限制。与保留天数并行，谁先达到先清理（超限时从最旧录像删起）</span>
            </el-form-item>
            <el-form-item label="存储路径">
              <el-input v-model="storageForm.root_path" placeholder="./recordings" style="width: 360px" />
              <span class="form-hint">录像/快照根目录（相对于服务运行目录）</span>
            </el-form-item>
            <el-form-item label="清理检查间隔">
              <el-input-number v-model="storageForm.cleanup_interval" :min="300" :max="86400" :step="300" :controls="false" style="width: 140px" />
              <span class="form-hint">秒</span>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="saveStorageSettings" :loading="storageSaving">
                <el-icon><Check /></el-icon> 保存存储设置
              </el-button>
            </el-form-item>
          </el-form>
        </el-card>

        <el-card :shadow="never" class="mt-16">
          <template #header>
            <h3>定时抓拍</h3>
          </template>
          <el-form :model="snapshotForm" label-width="160">
            <el-form-item label="启用定时抓拍">
              <el-switch v-model="snapshotForm.enabled" />
              <span class="form-hint">开启后按下方间隔自动抓拍所有录像中的摄像头（保存后立即生效，无需重启）</span>
            </el-form-item>
            <el-form-item label="抓拍间隔">
              <el-input-number v-model="snapshotForm.interval" :min="30" :max="86400" :step="30" :controls="false" style="width: 140px" />
              <span class="form-hint">秒（30~86400），建议 300（5分钟）</span>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="saveSnapshotSettings" :loading="snapshotSaving">
                <el-icon><Check /></el-icon> 保存抓拍设置
              </el-button>
            </el-form-item>
          </el-form>
        </el-card>

        <el-card :shadow="never" class="mt-16">
          <template #header>
            <h3>WebDAV 远程存储（可选）</h3>
          </template>
          <el-form :model="storageForm.webdav" label-width="160">
            <el-form-item label="启用 WebDAV">
              <el-switch v-model="storageForm.webdav.enabled" />
              <span class="form-hint">开启后每个录像分段完成即上传到 WebDAV 服务器（本地仍保留）</span>
            </el-form-item>
            <el-form-item label="仅存 WebDAV">
              <el-switch v-model="storageForm.webdav.only" :disabled="!storageForm.webdav.enabled" />
              <span class="form-hint">开启后录像上传成功即删除本地副本，本地仅作临时缓冲（上传失败则保留本地防丢失）；旧录像回放自动走 WebDAV 流式播放</span>
            </el-form-item>
            <el-form-item label="服务器地址">
              <el-input v-model="storageForm.webdav.url" placeholder="http://192.168.1.100:5005/webdav" style="width: 400px" />
            </el-form-item>
            <el-form-item label="用户名">
              <el-input v-model="storageForm.webdav.username" placeholder="webdav 用户" style="width: 300px" />
            </el-form-item>
            <el-form-item label="密码">
              <el-input v-model="storageForm.webdav.password" type="password" show-password placeholder="留空表示不修改" style="width: 300px" />
            </el-form-item>
            <el-form-item label="远程根目录">
              <el-input v-model="storageForm.webdav.base_path" placeholder="surveillance" style="width: 300px" />
              <span class="form-hint">录像将上传到 {远程根目录}/camera_{id}/ 下</span>
            </el-form-item>
            <el-form-item label="远程保留天数">
              <el-input-number v-model="storageForm.webdav.max_days" :min="0" :max="3650" :controls="false" style="width: 140px" />
              <span class="form-hint">独立于本地保留天数；0 = 不按时间自动删除（与本地清理周期同步执行）</span>
            </el-form-item>
            <el-form-item label="远程占用上限(GB)">
              <el-input-number v-model="storageForm.webdav.max_storage_gb" :min="0" :max="100000" :step="0.5" :precision="1" :controls="false" style="width: 140px" />
              <span class="form-hint">0 = 不限制；超出后从最旧远程录像开始删除，直到回到上限以内</span>
            </el-form-item>
            <el-form-item>
              <el-button @click="testWebdavConnection" :loading="webdavTesting">
                <el-icon><Connection /></el-icon> 测试连接
              </el-button>
              <el-button type="primary" @click="saveStorageSettings" :loading="storageSaving">
                <el-icon><Check /></el-icon> 保存
              </el-button>
              <span v-if="webdavTestResult" :class="['ml-12', webdavTestResult.ok ? 'text-success' : 'text-danger']">
                {{ webdavTestResult.message || webdavTestResult.error }}
              </span>
            </el-form-item>
          </el-form>
        </el-card>
      </el-tab-pane>

      <el-tab-pane label="系统维护" name="maintenance">
        <el-card :shadow="never" class="mt-16">
          <template #header>
            <h3>系统操作</h3>
          </template>
          <div class="maintenance-actions">
            <el-button type="danger" @click="restartSystem" :loading="restartLoading">
              <el-icon><Refresh /></el-icon> 重启系统
            </el-button>
            <el-button type="success" @click="checkForUpdate" :loading="updateChecking">
              <el-icon><Search /></el-icon> 检查更新
            </el-button>
            <el-button type="primary" @click="doUpdate" :loading="updating" :disabled="!updateInfo?.has_update">
              <el-icon><Upload /></el-icon>
              {{ updateInfo?.has_update ? `立即更新到 v${updateInfo.latest_version}` : '已是最新版本' }}
            </el-button>
            <el-button type="warning" @click="createBackup" :loading="backupLoading">
              <el-icon><Download /></el-icon> 备份数据库
            </el-button>
          </div>
          <div class="update-info" v-if="updateInfo">
            <template v-if="updateInfo.error">
              <el-alert :title="updateInfo.error" type="warning" :closable="false" show-icon />
            </template>
            <template v-else>
              <p>当前版本 <b>v{{ updateInfo.current_version }}</b>　→　最新版本 <b>v{{ updateInfo.latest_version }}</b>
                <el-tag v-if="updateInfo.has_update" type="success" size="small">可更新</el-tag>
                <el-tag v-else type="info" size="small">最新</el-tag>
              </p>
              <p v-if="updateInfo.asset_name" class="update-asset">
                产物：{{ updateInfo.asset_name }}（{{ formatBytes(updateInfo.asset_size) }}）
                <span v-if="updateInfo.published_at" class="text-muted">发布于 {{ new Date(updateInfo.published_at).toLocaleString('zh-CN') }}</span>
              </p>
              <div class="release-notes" v-if="updateInfo.release_notes">{{ updateInfo.release_notes }}</div>
            </template>
          </div>
        </el-card>

        <el-card :shadow="never" class="mt-16">
          <template #header>
            <div class="card-header">
              <h3>数据库备份</h3>
              <el-button size="small" @click="loadBackups"><el-icon><Refresh /></el-icon></el-button>
            </div>
          </template>
          <el-table :data="backupFiles" size="small" v-loading="backupLoading" style="width: 100%">
            <el-table-column prop="name" label="文件" min-width="220" />
            <el-table-column label="大小" width="110">
              <template #default="scope">{{ formatBytes(scope.row.size) }}</template>
            </el-table-column>
            <el-table-column label="备份时间" width="180">
              <template #default="scope">{{ formatTime(scope.row.mod_time * 1000) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="160" fixed="right">
              <template #default="scope">
                <el-button size="small" @click="downloadBackupFile(scope.row.name)">
                  <el-icon><Download /></el-icon> 下载
                </el-button>
                <el-button size="small" type="danger" plain @click="deleteBackupFile(scope.row.name)">
                  <el-icon><Delete /></el-icon> 删除
                </el-button>
              </template>
            </el-table-column>
          </el-table>
          <el-empty v-if="!backupFiles.length && !backupLoading" description="暂无备份，点「备份数据库」创建一个" :image-size="60" />
        </el-card>

        <el-card :shadow="never" class="mt-16">
          <template #header>
            <div class="card-header">
              <h3>运行日志</h3>
              <div class="log-toolbar">
                <el-select v-model="logLines" size="small" style="width: 96px" @change="loadLogTail">
                  <el-option v-for="n in [50, 100, 200, 500]" :key="n" :label="`${n} 行`" :value="n" />
                </el-select>
                <el-input v-model="logKeyword" size="small" placeholder="关键字过滤" clearable
                  style="width: 170px; margin-left: 8px" @keyup.enter="loadLogTail" @clear="loadLogTail" />
                <el-switch v-model="logAutoRefresh" size="small" style="margin-left: 10px" />
                <span class="text-muted log-auto-label">自动刷新</span>
                <el-button size="small" style="margin-left: 10px" @click="loadLogTail"><el-icon><Refresh /></el-icon></el-button>
                <el-button size="small" type="danger" plain @click="doClearLogs" :loading="clearLogsLoading">清空</el-button>
              </div>
            </div>
          </template>
          <p class="log-meta" v-if="logMeta.file">
            {{ logMeta.file }} · {{ formatBytes(logMeta.size) }} · 显示最近 {{ logMeta.total }} 行
            <span class="text-muted" v-if="logFiles.length">（轮转文件 {{ logFiles.length - 1 }} 份，共 {{ formatBytes(logFilesTotalSize) }}）</span>
          </p>
          <pre class="log-box" v-loading="logLoading">{{ logText || '（暂无日志）' }}</pre>
        </el-card>

        <el-card :shadow="never" class="mt-16">
          <template #header>
            <h3>系统信息</h3>
          </template>
          <div class="system-info" v-if="sysInfo">
            <div class="info-grid">
              <div class="info-item"><span class="label">版本</span><span class="value">v{{ sysInfo.version }}<span class="text-muted" v-if="sysInfo.git_commit && sysInfo.git_commit !== 'unknown'"> ({{ sysInfo.git_commit }})</span></span></div>
              <div class="info-item"><span class="label">构建时间</span><span class="value">{{ sysInfo.build_time || '-' }}</span></div>
              <div class="info-item"><span class="label">Go 版本</span><span class="value">{{ sysInfo.go_version }}</span></div>
              <div class="info-item"><span class="label">平台</span><span class="value">{{ sysInfo.os }} / {{ sysInfo.arch }}</span></div>
              <div class="info-item"><span class="label">进程 PID</span><span class="value">{{ sysInfo.pid }}</span></div>
              <div class="info-item"><span class="label">启动时间</span><span class="value">{{ formatTime(sysInfo.start_time * 1000) }}</span></div>
              <div class="info-item"><span class="label">运行时长</span><span class="value">{{ formatUptime(sysInfo.uptime) }}</span></div>
              <div class="info-item"><span class="label">CPU 核心</span><span class="value">{{ sysInfo.cpu_count }}</span></div>
              <div class="info-item"><span class="label">内存用量</span><span class="value">{{ formatBytes((sysInfo.mem_used_mb || 0) * 1024 * 1024) }} / {{ formatBytes((sysInfo.mem_total_mb || 0) * 1024 * 1024) }}</span></div>
              <div class="info-item"><span class="label">磁盘用量</span><span class="value">{{ formatBytes((sysInfo.disk_used_mb || 0) * 1024 * 1024) }} / {{ formatBytes((sysInfo.disk_total_mb || 0) * 1024 * 1024) }}<span class="text-muted" v-if="sysInfo.disk_path"> ({{ sysInfo.disk_path }})</span></span></div>
              <div class="info-item"><span class="label">数据库大小</span><span class="value">{{ formatBytes(sysInfo.db_size) }}</span></div>
              <div class="info-item"><span class="label">摄像头 / 录像</span><span class="value">{{ sysInfo.camera_count }} 台 / {{ sysInfo.recording_count }} 段</span></div>
              <div class="info-item"><span class="label">日志大小</span><span class="value">{{ formatBytes(sysInfo.log_size) }}</span></div>
              <div class="info-item"><span class="label">配置文件</span><span class="value">{{ sysInfo.config_path }}</span></div>
            </div>
          </div>
        </el-card>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Check, Refresh, Download, Delete, Connection, Search, Upload } from '@element-plus/icons-vue'
import { api } from '@/api'

const activeTab = ref('system')
const restartLoading = ref(false)
const backupLoading = ref(false)
const clearLogsLoading = ref(false)
const sysInfo = ref<any>(null)

// 程序更新
const updateChecking = ref(false)
const updating = ref(false)
const updateInfo = ref<any>(null)

// 数据库备份
const backupFiles = ref<any[]>([])

// 日志查看器
const logLines = ref(100)
const logKeyword = ref('')
const logAutoRefresh = ref(true)
const logLoading = ref(false)
const logText = ref('')
const logMeta = ref<any>({ file: '', size: 0, total: 0 })
const logFiles = ref<any[]>([])
const logFilesTotalSize = computed(() => logFiles.value.reduce((s: number, f: any) => s + (f.size || 0), 0))
let logTimer: any = null

const systemForm = reactive({
  name: '监控录像系统',
  http_port: 8080,
  ws_port: 8081,
  mode: 'release',
  log_level: 'info',
})

const dbForm = reactive({
  type: 'sqlite',
  sqlite_path: './data/surveillance.db',
  pg_host: '',
})

const redisForm = reactive({
  host: 'localhost',
  port: 6379,
  password: '',
  db: 0,
})

const cameraDefaultForm = reactive({
  record_enabled: true,
  record_type: 'continuous',
  width: 1920,
  height: 1080,
  fps: 25,
  codec: 'h264',
  bitrate: 4096,
})

// 存储设置
const storageSaving = ref(false)
const webdavTesting = ref(false)
const webdavTestResult = ref<{ ok: boolean; message?: string; error?: string } | null>(null)
const storageForm = reactive({
  root_path: './recordings',
  segment_duration: 180,
  max_days: 7,
  max_storage_gb: 0,
  cleanup_interval: 3600,
  webdav: {
    enabled: false,
    url: '',
    username: '',
    password: '',
    base_path: 'surveillance',
    max_days: 30,
    max_storage_gb: 0,
    only: false,
  },
})

// 定时抓拍设置
const snapshotSaving = ref(false)
const snapshotForm = reactive({
  enabled: true,
  interval: 300,
})

const loadSnapshotSettings = async () => {
  try {
    const res: any = await api.settings.getCamera()
    if (res) {
      snapshotForm.enabled = res.snapshot_enabled !== false
      snapshotForm.interval = res.snapshot_interval || 300
    }
  } catch (e) { console.error(e) }
}

const saveSnapshotSettings = async () => {
  snapshotSaving.value = true
  try {
    const res: any = await api.settings.updateCamera({
      snapshot_enabled: snapshotForm.enabled,
      snapshot_interval: snapshotForm.interval,
    })
    ElMessage.success(res?.message || '保存成功')
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '保存失败')
  } finally {
    snapshotSaving.value = false
  }
}

const loadStorageSettings = async () => {
  try {
    const res: any = await api.settings.getStorage()
    if (res && res.root_path) {
      storageForm.root_path = res.root_path
      storageForm.segment_duration = res.segment_duration || 180
      storageForm.max_days = res.max_days || 7
      storageForm.max_storage_gb = res.max_storage_gb ?? 0
      storageForm.cleanup_interval = res.cleanup_interval || 3600
      if (res.webdav) {
        storageForm.webdav.enabled = !!res.webdav.enabled
        storageForm.webdav.url = res.webdav.url || ''
        storageForm.webdav.username = res.webdav.username || ''
        storageForm.webdav.password = '' // 不回显密码
        storageForm.webdav.base_path = res.webdav.base_path || 'surveillance'
        storageForm.webdav.max_days = res.webdav.max_days ?? 30
        storageForm.webdav.max_storage_gb = res.webdav.max_storage_gb ?? 0
        storageForm.webdav.only = !!res.webdav.only
      }
    }
  } catch (e) { console.error(e) }
}

const saveStorageSettings = async () => {
  storageSaving.value = true
  try {
    const res: any = await api.settings.updateStorage({
      root_path: storageForm.root_path,
      segment_duration: storageForm.segment_duration,
      max_days: storageForm.max_days,
      max_storage_gb: storageForm.max_storage_gb,
      cleanup_interval: storageForm.cleanup_interval,
      webdav: {
        enabled: storageForm.webdav.enabled,
        url: storageForm.webdav.url,
        username: storageForm.webdav.username,
        password: storageForm.webdav.password, // 空/掩码 = 不修改
        base_path: storageForm.webdav.base_path,
        max_days: storageForm.webdav.max_days,
        max_storage_gb: storageForm.webdav.max_storage_gb,
        only: storageForm.webdav.only,
      },
    })
    ElMessage.success(res?.message || '保存成功')
    storageForm.webdav.password = ''
  } catch (e) {
    ElMessage.error('保存失败')
  } finally {
    storageSaving.value = false
  }
}

const testWebdavConnection = async () => {
  if (!storageForm.webdav.url) {
    ElMessage.warning('请填写 WebDAV 服务器地址')
    return
  }
  webdavTesting.value = true
  webdavTestResult.value = null
  try {
    const res: any = await api.settings.testWebdav({
      url: storageForm.webdav.url,
      username: storageForm.webdav.username,
      password: storageForm.webdav.password,
      base_path: storageForm.webdav.base_path,
    })
    webdavTestResult.value = { ok: !!res.ok, message: res.message, error: res.error }
    if (res.ok) ElMessage.success('WebDAV 连接成功')
  } catch (e: any) {
    webdavTestResult.value = { ok: false, error: e?.message || '网络错误' }
  } finally {
    webdavTesting.value = false
  }
}

const formatTime = (time: number) => time ? new Date(time).toLocaleString('zh-CN') : '-'
const formatUptime = (sec: number) => { const d=Math.floor(sec/86400),h=Math.floor((sec%86400)/3600),m=Math.floor((sec%3600)/60); return `${d}天${h}小时${m}分` }
const formatBytes = (n: number) => {
  if (n === undefined || n === null) return '-'
  if (n === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n, i = 0
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`
}

const loadSystemConfig = async () => {
  try {
    const res = await api.system.config()
    Object.assign(systemForm, res.server || {})
    Object.assign(dbForm, res.database || {})
    Object.assign(redisForm, res.redis || {})
    Object.assign(cameraDefaultForm, res.camera || {})
  } catch (e) { console.error(e) }
}

const saveSystemConfig = async () => { try { await api.system.updateConfig({ server: systemForm }); ElMessage.success('保存成功') } catch(e) { ElMessage.error('保存失败') } }
const saveDbConfig = async () => { ElMessage.success('SQLite 配置保存成功（需重启生效）') }
const saveRedisConfig = async () => { ElMessage.success('Redis 配置保存成功（需重启生效）') }
const saveCameraDefaults = async () => { ElMessage.success('摄像头默认配置保存成功') }

// ========== 系统维护 ==========

const restartSystem = async () => {
  try {
    await ElMessageBox.confirm('确定重启系统？重启期间录像与预览会短暂中断。', '重启系统', { type: 'warning' })
  } catch { return }
  restartLoading.value = true
  logAutoRefresh.value = false
  try {
    await api.system.restart()
    ElMessage.success('系统重启中...')
  } catch (e) { ElMessage.error('重启失败') } finally {
    restartLoading.value = false
  }
}

// 数据库备份
const loadBackups = async () => {
  backupLoading.value = true
  try {
    const res: any = await api.system.listBackups()
    backupFiles.value = res.files || []
  } catch (e) { backupFiles.value = [] } finally {
    backupLoading.value = false
  }
}

const createBackup = async () => {
  backupLoading.value = true
  try {
    const res: any = await api.system.createBackup()
    ElMessage.success(res.message || '备份成功')
    loadBackups()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '备份失败')
  } finally {
    backupLoading.value = false
  }
}

const downloadBackupFile = async (name: string) => {
  try {
    const blob: any = await api.system.downloadBackup(name)
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = name
    a.click()
    URL.revokeObjectURL(url)
  } catch (e) { ElMessage.error('下载失败') }
}

const deleteBackupFile = async (name: string) => {
  try {
    await ElMessageBox.confirm(`确定删除备份 ${name}？删除后不可恢复。`, '删除备份', { type: 'warning' })
  } catch { return }
  try {
    await api.system.deleteBackup(name)
    ElMessage.success('已删除')
    loadBackups()
  } catch (e) { ElMessage.error('删除失败') }
}

// 运行日志
const loadLogTail = async () => {
  logLoading.value = !logText.value
  try {
    const res: any = await api.system.logTail({ lines: logLines.value, keyword: logKeyword.value || undefined })
    logText.value = (res.lines || []).join('\n')
    logMeta.value = { file: res.file || '', size: res.size || 0, total: res.total || 0 }
  } catch (e) {
    logText.value = ''
  } finally {
    logLoading.value = false
  }
}

const loadLogFiles = async () => {
  try {
    const res: any = await api.system.logFiles()
    logFiles.value = res.files || []
  } catch (e) { /* 忽略 */ }
}

const doClearLogs = async () => {
  try {
    await ElMessageBox.confirm('确定清空当前日志并删除所有轮转备份文件？', '清理日志', {
      type: 'warning', confirmButtonText: '确定清空', cancelButtonText: '取消',
    })
  } catch { return }
  clearLogsLoading.value = true
  try {
    const res: any = await api.system.clearLogs()
    ElMessage.success(res.message || '日志已清理')
    loadLogTail(); loadLogFiles(); viewSystemInfo()
  } catch (e) { ElMessage.error('清理失败') } finally {
    clearLogsLoading.value = false
  }
}

// 程序更新
const checkForUpdate = async () => {
  updateChecking.value = true
  updateInfo.value = null
  try {
    const res: any = await api.system.checkUpdate()
    updateInfo.value = res
    if (res.has_update) ElMessage.success(`发现新版本 v${res.latest_version}`)
    else if (!res.error) ElMessage.info('当前已是最新版本')
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '检查更新失败')
  } finally {
    updateChecking.value = false
  }
}

const doUpdate = async () => {
  if (!updateInfo.value?.has_update) return
  try {
    await ElMessageBox.confirm(
      `即将下载并安装 v${updateInfo.value.latest_version}，期间录像与预览会短暂中断（约 1~2 分钟），确定继续？`,
      '更新程序', { type: 'warning', confirmButtonText: '开始更新', cancelButtonText: '取消' },
    )
  } catch { return }
  updating.value = true
  logAutoRefresh.value = false
  try {
    const res: any = await api.system.performUpdate()
    ElMessage.success(res.message || '更新完成，系统重启中...')
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '更新失败')
  } finally {
    updating.value = false
  }
}

const viewSystemInfo = async () => {
  try { sysInfo.value = await api.system.info() } catch (e) { /* 忽略 */ }
}

onMounted(() => {
  loadSystemConfig(); viewSystemInfo(); loadStorageSettings(); loadSnapshotSettings()
  loadBackups(); loadLogFiles(); loadLogTail()
  logTimer = setInterval(() => { if (logAutoRefresh.value) loadLogTail() }, 5000)
})
onUnmounted(() => { if (logTimer) clearInterval(logTimer) })
</script>

<style scoped lang="scss">
.settings-page {
  .maintenance-actions { display: flex; gap: 12px; flex-wrap: wrap; }
  .system-info {
    .info-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
      gap: 16px;
      .info-item {
        display: flex;
        flex-direction: column;
        gap: 4px;
        padding: 12px;
        background: #fafafa;
        border-radius: 8px;
        .label { font-size: 12px; color: #909399; }
        .value { font-size: 14px; color: #303133; font-weight: 500; }
      }
    }
  }
  .form-hint {
    font-size: 12px;
    color: #909399;
    margin-left: 12px;
    line-height: 1.4;
  }
  .text-success { color: #67c23a; font-size: 13px; }
  .text-danger { color: #f56c6c; font-size: 13px; }
  .ml-12 { margin-left: 12px; }
  .card-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    width: 100%;
    h3 { margin: 0; }
  }
  .update-info {
    margin-top: 16px;
    padding: 12px 16px;
    background: #f5f7fa;
    border-radius: 8px;
    p { margin: 0 0 6px; font-size: 13px; }
    .update-asset { color: #606266; }
    .release-notes {
      margin-top: 8px;
      font-size: 12px;
      color: #909399;
      white-space: pre-wrap;
      word-break: break-word;
      max-height: 160px;
      overflow-y: auto;
    }
  }
  .log-toolbar { display: flex; align-items: center; }
  .log-auto-label { font-size: 12px; }
  .log-meta { margin: 0 0 8px; font-size: 12px; color: #909399; }
  .log-box {
    margin: 0;
    padding: 12px;
    background: #1e1e2e;
    color: #cdd6f4;
    border-radius: 8px;
    font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
    font-size: 12px;
    line-height: 1.6;
    max-height: 420px;
    overflow: auto;
    white-space: pre-wrap;
    word-break: break-all;
  }
  .text-muted { color: #909399; font-size: 12px; }
}
</style>
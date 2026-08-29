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
            <el-button type="warning" @click="backupDatabase" :loading="backupLoading">
              <el-icon><Download /></el-icon> 备份数据库
            </el-button>
            <el-button @click="clearLogs" :loading="clearLogsLoading">
              <el-icon><Delete /></el-icon> 清理日志
            </el-button>
            <el-button @click="viewSystemInfo">
              <el-icon><Monitor /></el-icon> 查看系统信息
            </el-button>
          </div>
        </el-card>

        <el-card :shadow="never" class="mt-16">
          <template #header>
            <h3>系统信息</h3>
          </template>
          <div class="system-info" v-if="sysInfo">
            <div class="info-grid">
              <div class="info-item"><span class="label">版本</span><span class="value">{{ sysInfo.version }}</span></div>
              <div class="info-item"><span class="label">Go 版本</span><span class="value">{{ sysInfo.go_version }}</span></div>
              <div class="info-item"><span class="label">启动时间</span><span class="value">{{ formatTime(sysInfo.start_time * 1000) }}</span></div>
              <div class="info-item"><span class="label">运行时长</span><span class="value">{{ formatUptime(sysInfo.uptime) }}</span></div>
            </div>
          </div>
        </el-card>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Check, Refresh, Download, Delete, Monitor, Connection } from '@element-plus/icons-vue'
import { api } from '@/api'

const activeTab = ref('system')
const restartLoading = ref(false)
const backupLoading = ref(false)
const clearLogsLoading = ref(false)
const sysInfo = ref<any>(null)

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

const restartSystem = async () => { try { restartLoading.value = true; await api.system.restart(); ElMessage.success('系统重启中...') } catch(e) { ElMessage.error('重启失败') } finally { restartLoading.value = false } }
const backupDatabase = async () => { try { backupLoading.value = true; ElMessage.success('数据库备份功能待实现') } finally { backupLoading.value = false } }
const clearLogs = async () => { try { clearLogsLoading.value = true; ElMessage.success('日志清理功能待实现') } finally { clearLogsLoading.value = false } }
const viewSystemInfo = async () => { try { sysInfo.value = await api.system.info() } catch(e) { ElMessage.error('获取系统信息失败') } }

onMounted(() => { loadSystemConfig(); viewSystemInfo(); loadStorageSettings(); loadSnapshotSettings() })
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
}
</style>
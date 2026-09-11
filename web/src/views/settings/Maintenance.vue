<template>
  <div class="settings-page">
    <el-card :shadow="never">
      <template #header>
        <h3>{{ t('settingsMaint.sysOpsTitle') }}</h3>
      </template>
      <div class="maintenance-actions">
        <el-button type="danger" @click="restartSystem" :loading="restartLoading">
          <el-icon><Refresh /></el-icon> {{ t('settingsMaint.restart') }}
        </el-button>
        <el-button type="success" @click="checkForUpdate" :loading="updateChecking">
          <el-icon><Search /></el-icon> {{ t('settingsMaint.checkUpdate') }}
        </el-button>
        <el-button type="primary" @click="doUpdate" :loading="updating" :disabled="!updateInfo?.has_update">
          <el-icon><Upload /></el-icon>
          {{ updateInfo?.has_update ? t('settingsMaint.updateBtn', { version: updateInfo.latest_version }) : t('settingsMaint.upToDate') }}
        </el-button>
        <el-button type="warning" @click="createBackup" :loading="backupLoading">
          <el-icon><Download /></el-icon> {{ t('settingsMaint.createBackup') }}
        </el-button>
      </div>
      <div class="update-proxy-row">
        <span class="update-proxy-label">{{ t('settingsMaint.updateProxyLabel') }}</span>
        <el-input v-model="updateCfg.proxy" :placeholder="t('settingsMaint.updateProxyPlaceholder')" style="width: 340px" clearable size="small" />
        <el-button size="small" @click="saveUpdateCfg" :loading="updateCfgSaving">{{ t('settingsMaint.saveProxy') }}</el-button>
        <span v-if="updateCfg.proxy" class="text-success">{{ t('settingsMaint.proxyActive') }}</span>
      </div>
      <div class="update-info" v-if="updateInfo">
        <template v-if="updateInfo.error">
          <el-alert :title="t('settingsMaint.updateError', { error: updateInfo.error })" type="warning" :closable="false" show-icon />
        </template>
        <template v-else>
          <p>{{ t('settingsMaint.currentVersion', { current: updateInfo.current_version, latest: updateInfo.latest_version }) }}
            <el-tag v-if="updateInfo.has_update" type="success" size="small">{{ t('settingsMaint.updateAvailable') }}</el-tag>
            <el-tag v-else type="info" size="small">{{ t('settingsMaint.upToDateTag') }}</el-tag>
          </p>
          <p v-if="updateInfo.asset_name" class="update-asset">
            {{ t('settingsMaint.assetInfo', { name: updateInfo.asset_name, size: formatBytes(updateInfo.asset_size) }) }}
            <span v-if="updateInfo.published_at" class="text-muted">{{ t('settingsMaint.publishedAt', { date: new Date(updateInfo.published_at).toLocaleString(locale.value === 'en' ? 'en-US' : 'zh-CN') }) }}</span>
          </p>
          <div class="release-notes" v-if="updateInfo.release_notes">{{ updateInfo.release_notes }}</div>
        </template>
      </div>
    </el-card>

    <el-card :shadow="never" class="mt-16">
      <template #header>
        <div class="card-header">
          <h3>{{ t('settingsMaint.envCheckTitle') }}</h3>
          <el-button size="small" :loading="envLoading" @click="loadEnv">
            <el-icon v-if="!envLoading"><Refresh /></el-icon> {{ t('settingsMaint.recheck') }}
          </el-button>
        </div>
      </template>
      <EnvCheckTable :report="envReport" :loading="envLoading" />
    </el-card>

    <el-card :shadow="never" class="mt-16">
      <template #header>
        <div class="card-header">
          <h3>{{ t('settingsMaint.backupTitle') }}</h3>
          <el-button size="small" @click="loadBackups"><el-icon><Refresh /></el-icon></el-button>
        </div>
      </template>
      <el-table :data="backupFiles" size="small" v-loading="backupLoading" style="width: 100%">
        <el-table-column prop="name" :label="t('settingsMaint.backupFile')" min-width="220" />
        <el-table-column :label="t('settingsMaint.backupSize')" width="110">
          <template #default="scope">{{ formatBytes(scope.row.size) }}</template>
        </el-table-column>
        <el-table-column :label="t('settingsMaint.backupTime')" width="180">
          <template #default="scope">{{ formatTime(scope.row.mod_time * 1000) }}</template>
        </el-table-column>
        <el-table-column :label="t('settingsMaint.actions')" width="160" fixed="right">
          <template #default="scope">
            <el-button size="small" @click="downloadBackupFile(scope.row.name)">
              <el-icon><Download /></el-icon> {{ t('settingsMaint.download') }}
            </el-button>
            <el-button size="small" type="danger" plain @click="deleteBackupFile(scope.row.name)">
              <el-icon><Delete /></el-icon> {{ t('settingsMaint.delete') }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!backupFiles.length && !backupLoading" :description="t('settingsMaint.noBackups')" :image-size="60" />
    </el-card>

    <el-card :shadow="never" class="mt-16">
      <template #header>
        <div class="card-header">
          <h3>{{ t('settingsMaint.logTitle') }}</h3>
          <div class="log-toolbar">
            <el-select v-model="logLines" size="small" style="width: 96px" @change="loadLogTail">
              <el-option v-for="n in [50, 100, 200, 500]" :key="n" :label="t('settingsMaint.logLines', { n })" :value="n" />
            </el-select>
            <el-input v-model="logKeyword" size="small" :placeholder="t('settingsMaint.logKeyword')" clearable style="width: 170px; margin-left: 8px" @keyup.enter="loadLogTail" @clear="loadLogTail" />
            <el-switch v-model="logAutoRefresh" size="small" style="margin-left: 10px" />
            <span class="text-muted log-auto-label">{{ t('settingsMaint.autoRefresh') }}</span>
            <el-button size="small" style="margin-left: 10px" @click="loadLogTail"><el-icon><Refresh /></el-icon></el-button>
            <el-button size="small" type="danger" plain @click="doClearLogs" :loading="clearLogsLoading">{{ t('settingsMaint.clearLogs') }}</el-button>
          </div>
        </div>
      </template>
      <p class="log-meta" v-if="logMeta.file">
        {{ t('settingsMaint.logMeta', { file: logMeta.file, size: formatBytes(logMeta.size), total: logMeta.total }) }}
        <span class="text-muted" v-if="logFiles.length">{{ t('settingsMaint.logMetaRotated', { count: logFiles.length - 1, size: formatBytes(logFilesTotalSize) }) }}</span>
      </p>
      <pre class="log-box" v-loading="logLoading">{{ logText || t('settingsMaint.noLog') }}</pre>
    </el-card>

    <el-card :shadow="never" class="mt-16">
      <template #header>
        <h3>{{ t('settingsMaint.sysInfoTitle') }}</h3>
      </template>
      <div class="system-info" v-if="sysInfo">
        <div class="info-grid">
          <div class="info-item"><span class="label">{{ t('settingsMaint.version') }}</span><span class="value">v{{ sysInfo.version }}<span class="text-muted" v-if="sysInfo.git_commit && sysInfo.git_commit !== 'unknown'"> ({{ sysInfo.git_commit }})</span></span></div>
          <div class="info-item"><span class="label">{{ t('settingsMaint.buildTime') }}</span><span class="value">{{ sysInfo.build_time || '-' }}</span></div>
          <div class="info-item"><span class="label">{{ t('settingsMaint.goVersion') }}</span><span class="value">{{ sysInfo.go_version }}</span></div>
          <div class="info-item"><span class="label">{{ t('settingsMaint.platform') }}</span><span class="value">{{ sysInfo.os }} / {{ sysInfo.arch }}</span></div>
          <div class="info-item"><span class="label">{{ t('settingsMaint.pid') }}</span><span class="value">{{ sysInfo.pid }}</span></div>
          <div class="info-item"><span class="label">{{ t('settingsMaint.startTime') }}</span><span class="value">{{ formatTime(sysInfo.start_time * 1000) }}</span></div>
          <div class="info-item"><span class="label">{{ t('settingsMaint.uptime') }}</span><span class="value">{{ formatUptime(sysInfo.uptime) }}</span></div>
          <div class="info-item"><span class="label">{{ t('settingsMaint.cpuCores') }}</span><span class="value">{{ sysInfo.cpu_count }}</span></div>
          <div class="info-item"><span class="label">{{ t('settingsMaint.memUsage') }}</span><span class="value">{{ formatBytes((sysInfo.mem_used_mb || 0) * 1024 * 1024) }} / {{ formatBytes((sysInfo.mem_total_mb || 0) * 1024 * 1024) }}</span></div>
          <div class="info-item"><span class="label">{{ t('settingsMaint.diskUsage') }}</span><span class="value">{{ formatBytes((sysInfo.disk_used_mb || 0) * 1024 * 1024) }} / {{ formatBytes((sysInfo.disk_total_mb || 0) * 1024 * 1024) }}<span class="text-muted" v-if="sysInfo.disk_path"> ({{ sysInfo.disk_path }})</span></span></div>
          <div class="info-item"><span class="label">{{ t('settingsMaint.dbSize') }}</span><span class="value">{{ formatBytes(sysInfo.db_size) }}</span></div>
          <div class="info-item"><span class="label">{{ t('settingsMaint.cameraRecording') }}</span><span class="value">{{ t('settingsMaint.cameraRecordingValue', { cams: sysInfo.camera_count, recs: sysInfo.recording_count }) }}</span></div>
          <div class="info-item"><span class="label">{{ t('settingsMaint.logSize') }}</span><span class="value">{{ formatBytes(sysInfo.log_size) }}</span></div>
          <div class="info-item"><span class="label">{{ t('settingsMaint.configPath') }}</span><span class="value">{{ sysInfo.config_path }}</span></div>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Download, Delete, Search, Upload } from '@element-plus/icons-vue'
import { api } from '@/api'
import EnvCheckTable from '@/components/EnvCheckTable.vue'
import { useI18n } from 'vue-i18n'

const { t, locale } = useI18n()

const restartLoading = ref(false)
const backupLoading = ref(false)
const clearLogsLoading = ref(false)
const sysInfo = ref<any>(null)

const envReport = ref<any>(null)
const envLoading = ref(false)
const loadEnv = async () => {
  envLoading.value = true
  try {
    envReport.value = await api.system.envCheck()
  } catch (e) {
    console.error(e)
  } finally {
    envLoading.value = false
  }
}

const updateChecking = ref(false)
const updating = ref(false)
const updateInfo = ref<any>(null)
const updateCfg = reactive({ proxy: '' })
const updateCfgSaving = ref(false)

const backupFiles = ref<any[]>([])

const logLines = ref(100)
const logKeyword = ref('')
const logAutoRefresh = ref(true)
const logLoading = ref(false)
const logText = ref('')
const logMeta = ref<any>({ file: '', size: 0, total: 0 })
const logFiles = ref<any[]>([])
const logFilesTotalSize = computed(() => logFiles.value.reduce((s: number, f: any) => s + (f.size || 0), 0))
let logTimer: any = null

const formatTime = (time: number) => time ? new Date(time).toLocaleString(locale.value === 'en' ? 'en-US' : 'zh-CN') : '-'
const formatUptime = (sec: number) => { const d=Math.floor(sec/86400),h=Math.floor((sec%86400)/3600),m=Math.floor((sec%3600)/60); return t('settingsMaint.uptimeFormat', { d, h, m }) }
const formatBytes = (n: number) => {
  if (n === undefined || n === null) return '-'
  if (n === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n, i = 0
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`
}

const restartSystem = async () => {
  try {
    await ElMessageBox.confirm(t('settingsMaint.restartConfirm'), t('settingsMaint.restart'), { type: 'warning' })
  } catch { return }
  restartLoading.value = true
  logAutoRefresh.value = false
  try {
    await api.system.restart()
    ElMessage.success(t('settingsMaint.restartSuccess'))
  } catch (e) { ElMessage.error(t('settingsMaint.restartFailed')) } finally {
    restartLoading.value = false
  }
}

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
    ElMessage.success(res.message || t('settingsMaint.backupSuccess'))
    loadBackups()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || t('settingsMaint.backupFailed'))
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
  } catch (e) { ElMessage.error(t('settingsMaint.downloadFailed')) }
}

const deleteBackupFile = async (name: string) => {
  try {
    await ElMessageBox.confirm(t('settingsMaint.deleteBackupConfirm', { name }), t('settingsMaint.delete'), { type: 'warning' })
  } catch { return }
  try {
    await api.system.deleteBackup(name)
    ElMessage.success(t('settingsMaint.deleteSuccess'))
    loadBackups()
  } catch (e) { ElMessage.error(t('settingsMaint.deleteFailed')) }
}

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
  } catch (e) { /* 拦截器已提示 */ }
}

const doClearLogs = async () => {
  try {
    await ElMessageBox.confirm(t('settingsMaint.confirmClearLogs'), t('settingsMaint.clearLogs'), {
      type: 'warning', confirmButtonText: t('settingsMaint.confirm'), cancelButtonText: t('settingsMaint.cancel'),
    })
  } catch { return }
  clearLogsLoading.value = true
  try {
    const res: any = await api.system.clearLogs()
    ElMessage.success(res.message || t('settingsMaint.clearLogsSuccess'))
    loadLogTail(); loadLogFiles(); viewSystemInfo()
  } catch (e) { ElMessage.error(t('settingsMaint.clearLogsFailed')) } finally {
    clearLogsLoading.value = false
  }
}

const checkForUpdate = async () => {
  updateChecking.value = true
  updateInfo.value = null
  try {
    const res: any = await api.system.checkUpdate()
    updateInfo.value = res
    if (res.has_update) ElMessage.success(t('settingsMaint.updateCheckSuccess', { version: res.latest_version }))
    else if (!res.error) ElMessage.info(t('settingsMaint.updateCheckUpToDate'))
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || t('settingsMaint.updateCheckFailed'))
  } finally {
    updateChecking.value = false
  }
}

const doUpdate = async () => {
  if (!updateInfo.value?.has_update) return
  try {
    await ElMessageBox.confirm(
      t('settingsMaint.updateConfirm', { version: updateInfo.value.latest_version }),
      t('settingsMaint.update'),
      { type: 'warning', confirmButtonText: t('settingsMaint.startUpdate'), cancelButtonText: t('settingsMaint.cancel') },
    )
  } catch { return }
  updating.value = true
  logAutoRefresh.value = false
  try {
    const res: any = await api.system.performUpdate()
    ElMessage.success(res.message || t('settingsMaint.updateSuccess'))
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || t('settingsMaint.updateFailed'))
  } finally {
    updating.value = false
  }
}

const viewSystemInfo = async () => {
  try { sysInfo.value = await api.system.info() } catch (e) { /* 拦截器已提示 */ }
}

const loadUpdateCfg = async () => {
  try {
    const res: any = await api.system.getUpdateConfig()
    updateCfg.proxy = res.proxy || ''
  } catch (e) { /* 拦截器已提示 */ }
}

const saveUpdateCfg = async () => {
  updateCfgSaving.value = true
  try {
    await api.system.saveUpdateConfig({ proxy: updateCfg.proxy.trim() })
    ElMessage.success(t('settingsMaint.proxySaved'))
    loadUpdateCfg()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || t('settingsMaint.proxySaveFailed'))
  } finally {
    updateCfgSaving.value = false
  }
}

onMounted(() => {
  viewSystemInfo()
  loadBackups(); loadLogFiles(); loadLogTail(); loadUpdateCfg()
  loadEnv()
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
  .text-success { color: #67c23a; font-size: 13px; }
  .card-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    width: 100%;
    h3 { margin: 0; }
  }
  .update-proxy-row {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-top: 16px;
    .update-proxy-label { font-size: 13px; color: #606266; white-space: nowrap; }
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
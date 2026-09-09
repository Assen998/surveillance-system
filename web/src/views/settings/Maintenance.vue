<template>
  <div class="settings-page">
    <el-card :shadow="never">
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
      <div class="update-proxy-row">
        <span class="update-proxy-label">更新代理（可选）</span>
        <el-input v-model="updateCfg.proxy" placeholder="直连 GitHub 不稳定时填写，如 http://192.168.1.5:7890"
          style="width: 340px" clearable size="small" />
        <el-button size="small" @click="saveUpdateCfg" :loading="updateCfgSaving">保存</el-button>
        <span v-if="updateCfg.proxy" class="text-success">使用中</span>
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
          <h3>运行环境检测</h3>
          <el-button size="small" :loading="envLoading" @click="loadEnv">
            <el-icon v-if="!envLoading"><Refresh /></el-icon> 运行环境检测
          </el-button>
        </div>
      </template>
      <EnvCheckTable :report="envReport" :loading="envLoading" />
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
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Download, Delete, Search, Upload } from '@element-plus/icons-vue'
import { api } from '@/api'
import EnvCheckTable from '@/components/EnvCheckTable.vue'

const restartLoading = ref(false)
const backupLoading = ref(false)
const clearLogsLoading = ref(false)
const sysInfo = ref<any>(null)

// 运行环境检测
const envReport = ref<any>(null)
const envLoading = ref(false)
const loadEnv = async () => {
  envLoading.value = true
  try {
    envReport.value = await api.system.envCheck()
  } catch (e) {
    // 错误提示由响应拦截器统一处理
    console.error(e)
  } finally {
    envLoading.value = false
  }
}

// 程序更新
const updateChecking = ref(false)
const updating = ref(false)
const updateInfo = ref<any>(null)
const updateCfg = reactive({ proxy: '' })
const updateCfgSaving = ref(false)

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

// 更新代理设置
const loadUpdateCfg = async () => {
  try {
    const res: any = await api.system.getUpdateConfig()
    updateCfg.proxy = res.proxy || ''
  } catch (e) { /* 忽略 */ }
}

const saveUpdateCfg = async () => {
  updateCfgSaving.value = true
  try {
    await api.system.saveUpdateConfig({ proxy: updateCfg.proxy.trim() })
    ElMessage.success('更新代理设置已保存')
    loadUpdateCfg()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '保存失败')
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

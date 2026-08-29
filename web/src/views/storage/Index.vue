<template>
  <div class="storage-page">
    <div class="page-header">
      <h2>存储管理</h2>
      <div class="header-actions">
        <el-button @click="fetchStats"><el-icon><Refresh /></el-icon> 刷新</el-button>
        <el-button type="primary" @click="manualCleanup"><el-icon><Delete /></el-icon> 立即清理</el-button>
      </div>
    </div>

    <!-- 存储概览卡片 -->
    <el-row :gutter="24" class="mb-24" v-if="stats">
      <el-col :xs="24" :sm="12" :lg="6">
        <el-card :shadow="never" class="stat-card">
          <div class="stat-header">
            <span class="stat-label">总容量</span>
            <el-tag type="info" size="small">{{ formatBytes(stats.total_space) }}</el-tag>
          </div>
          <div class="stat-bar">
            <div class="stat-bar-track">
              <div class="stat-bar-fill" :style="{ width: usagePercent + '%' }" :class="{ warning: usagePercent > 80, danger: usagePercent > 95 }" />
            </div>
            <div class="stat-value">{{ usagePercent.toFixed(1) }}%</div>
          </div>
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="12" :lg="6">
        <el-card :shadow="never" class="stat-card">
          <div class="stat-header">
            <span class="stat-label">已用空间</span>
            <el-tag type="danger" size="small">{{ formatBytes(stats.used_space) }}</el-tag>
          </div>
          <div class="stat-detail">
            <span v-if="maxStorageGb > 0">占用上限: {{ maxStorageGb }} GB</span>
            <span v-else>占用上限: 未设置</span>
          </div>
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="12" :lg="6">
        <el-card :shadow="never" class="stat-card">
          <div class="stat-header">
            <span class="stat-label">剩余空间</span>
            <el-tag type="success" size="small">{{ formatBytes(stats.free_space) }}</el-tag>
          </div>
          <div class="stat-detail">
            <span>可录制: {{ formatDuration(stats.free_space / (4 * 1024 * 1024 / 8)) }}</span>
          </div>
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="12" :lg="6">
        <el-card :shadow="never" class="stat-card">
          <div class="stat-header">
            <span class="stat-label">文件统计</span>
          </div>
          <div class="stat-grid">
            <div><span class="stat-num">{{ stats.recording_count }}</span><span class="stat-unit">录像片段</span></div>
            <div><span class="stat-num">{{ stats.snapshot_count }}</span><span class="stat-unit">抓拍图片</span></div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 按摄像头统计 -->
    <el-card :shadow="never" class="mb-16" v-if="stats?.camera_stats">
      <template #header>
        <h3>各摄像头存储占用</h3>
      </template>
      <el-table :data="cameraStatsArray" border stripe size="small" style="width: 100%">
        <el-table-column prop="camera_name" label="摄像头" width="200" />
        <el-table-column label="录像片段" width="120">
          <template #default="scope">{{ scope.row.recording_count }}</template>
        </el-table-column>
        <el-table-column label="占用空间" width="150">
          <template #default="scope">{{ formatBytes(scope.row.total_size) }}</template>
        </el-table-column>
        <el-table-column label="最早录像" width="180">
          <template #default="scope">{{ formatTime(scope.row.oldest_recording) }}</template>
        </el-table-column>
        <el-table-column label="最新录像" width="180">
          <template #default="scope">{{ formatTime(scope.row.latest_recording) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="150">
          <template #default="scope">
            <el-button size="small" link @click="goToCameraRecordings(scope.row.camera_id)">
              <el-icon><Film /></el-icon> 查看录像
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- MinIO 配置 -->
    <el-card :shadow="never" class="mt-16" v-if="false">
      <template #header>
        <h3>MinIO 对象存储</h3>
      </template>
      <el-form :model="minioConfig" label-width="120">
        <el-form-item label="启用 MinIO">
          <el-switch v-model="minioConfig.enabled" />
        </el-form-item>
        <el-form-item label="EndPoint">
          <el-input v-model="minioConfig.endpoint" placeholder="localhost:9000" style="width: 300px" />
        </el-form-item>
        <el-form-item label="Access Key">
          <el-input v-model="minioConfig.access_key" placeholder="minioadmin" style="width: 300px" />
        </el-form-item>
        <el-form-item label="Secret Key">
          <el-input v-model="minioConfig.secret_key" type="password" show-password placeholder="minioadmin" style="width: 300px" />
        </el-form-item>
        <el-form-item label="Bucket">
          <el-input v-model="minioConfig.bucket" placeholder="surveillance" style="width: 300px" />
        </el-form-item>
        <el-form-item label="使用 SSL">
          <el-switch v-model="minioConfig.use_ssl" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="testMinioConnection"><el-icon><Connection /></el-icon> 测试连接</el-button>
          <el-button @click="saveMinioConfig"><el-icon><Check /></el-icon> 保存</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, Delete, Check, Connection, Film } from '@element-plus/icons-vue'
import { api } from '@/api'
import { useStorageStore } from '@/stores'

const storageStore = useStorageStore()

const stats = ref<any>(null)
const maxStorageGb = ref(0)

const loadMaxStorage = async () => {
  try {
    const res: any = await api.settings.getStorage()
    maxStorageGb.value = res?.max_storage_gb ?? 0
  } catch (e) { console.error(e) }
}
const minioConfig = ref({
  enabled: false,
  endpoint: '',
  access_key: '',
  secret_key: '',
  bucket: '',
  use_ssl: false,
})

const usagePercent = computed(() => {
  if (!stats.value?.total_space) return 0
  return (stats.value.used_space / stats.value.total_space) * 100
})

const cameraStatsArray = computed(() => {
  if (!stats.value?.camera_stats) return []
  return Object.values(stats.value.camera_stats)
})

const formatBytes = (bytes: number) => {
  if (!bytes) return '0 B'
  const k = 1024, sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

const formatDuration = (seconds: number) => {
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  return d > 0 ? `${d}天${h}小时` : `${h}小时`
}

const formatTime = (time: string | Date | null) => {
  if (!time) return '-'
  return new Date(time).toLocaleString('zh-CN')
}

const fetchStats = async () => {
  await storageStore.fetchStats()
  stats.value = storageStore.stats
}

const manualCleanup = async () => {
  try {
    await api.storage.cleanup()
    ElMessage.success('清理完成')
    fetchStats()
  } catch (e) {
    ElMessage.error('清理失败')
  }
}

const testMinioConnection = async () => {
  ElMessage.info('测试连接功能待实现')
}

const saveMinioConfig = async () => {
  ElMessage.success('配置已保存')
}

const goToCameraRecordings = (cameraId: number) => {
  // router.push(`/recordings?camera_id=${cameraId}`)
}

onMounted(() => {
  fetchStats()
  loadMaxStorage()
})
</script>

<style scoped lang="scss">
.storage-page {
  .page-header { display:flex; justify-content:space-between; align-items:center; margin-bottom:24px; h2{margin:0;font-size:20px;font-weight:600;} .header-actions{display:flex;gap:12px;} }
  .stat-card { .stat-header{display:flex;justify-content:space-between;align-items:center;margin-bottom:12px;.stat-label{color:#909399;font-size:14px;}} .stat-bar{height:8px;background:#e6e9ed;border-radius:4px;overflow:hidden;position:relative;margin-bottom:8px;.stat-bar-track{height:100%;width:100%;background:#e6e9ed;border-radius:4px;}.stat-bar-fill{height:100%;background:linear-gradient(90deg,#409eff,#67c23a);border-radius:4px;transition:width 0.5s;&.warning{background:linear-gradient(90deg,#e6a23c,#f56c6c);}&.danger{background:#f56c6c;}} .stat-value{position:absolute;right:0;top:0;font-size:12px;color:#909399;} } .stat-detail{display:flex;gap:16px;font-size:12px;color:#909399;} .stat-grid{display:grid;grid-template-columns:1fr 1fr;gap:16px;text-align:center;.stat-num{display:block;font-size:24px;font-weight:700;color:#303133;}.stat-unit{display:block;font-size:12px;color:#909399;}} }
}
</style>
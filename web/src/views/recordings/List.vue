<template>
  <div class="recording-list-page">
    <div class="page-header">
      <h2>录像列表</h2>
      <div class="header-actions">
        <el-button @click="fetchRecordings"><el-icon><Refresh /></el-icon> 刷新</el-button>
        <el-button type="primary" @click="showCleanupDialog"><el-icon><Delete /></el-icon> 清理过期</el-button>
      </div>
    </div>

    <!-- 筛选 -->
    <el-card :shadow="never" class="mb-16">
      <el-form :inline="true" :model="searchForm" class="search-form">
        <el-form-item label="摄像头">
          <el-select v-model="searchForm.camera_id" placeholder="全部" style="width: 200px">
            <el-option v-for="cam in cameraStore.cameras" :key="cam.id" :label="cam.name" :value="cam.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="录像类型">
          <el-select v-model="searchForm.record_type" placeholder="全部" style="width: 140px">
            <el-option label="连续录像" value="continuous" />
            <el-option label="移动侦测" value="motion" />
            <el-option label="定时录像" value="schedule" />
            <el-option label="手动录像" value="manual" />
          </el-select>
        </el-form-item>
        <el-form-item label="时间范围">
          <el-date-picker v-model="searchForm.time_range" type="datetimerange" :range-separator="'-'" start-placeholder="开始" end-placeholder="结束" style="width: 360px" value-format="YYYY-MM-DD HH:mm:ss" />
        </el-form-item>
        <el-form-item>
          <el-button @click="handleSearch"><el-icon><Search /></el-icon> 搜索</el-button>
          <el-button @click="resetSearch">重置</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 列表 -->
    <el-card :shadow="never">
      <el-table :data="tableData" v-loading="loading" border stripe size="small" row-key="id" style="width: 100%">
        <el-table-column type="selection" width="55" />
        <el-table-column prop="camera_id" label="摄像头" width="180">
          <template #default="scope">
            <span>{{ getCameraName(scope.row.camera_id) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="时间范围" width="300">
          <template #default="scope">
            <div>{{ formatTime(scope.row.start_time) }}</div>
            <div class="text-muted">{{ formatTime(scope.row.end_time) }}</div>
          </template>
        </el-table-column>
        <el-table-column label="时长" width="100">
          <template #default="scope">{{ formatDuration(scope.row.duration) }}</template>
        </el-table-column>
        <el-table-column prop="record_type" label="类型" width="120">
          <template #default="scope">
            <el-tag :type="typeMap[scope.row.record_type] || 'info'" size="small">{{ typeLabels[scope.row.record_type] }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="大小" width="120">
          <template #default="scope">{{ formatBytes(scope.row.file_size) }}</template>
        </el-table-column>
        <el-table-column label="存储" width="100">
          <template #default="scope">
            <el-tag :type="scope.row.storage_type === 'local' ? 'success' : 'info'" size="small">{{ scope.row.storage_type }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="scope">
            <el-button-group size="small">
              <el-button link type="primary" @click.stop="playRecording(scope.row)">
                <el-icon><VideoPlay /></el-icon> 播放
              </el-button>
              <el-button link @click.stop="downloadRecording(scope.row)">
                <el-icon><Download /></el-icon> 下载
              </el-button>
              <el-button link type="danger" @click.stop="deleteRecording(scope.row.id)">
                <el-icon><Delete /></el-icon> 删除
              </el-button>
            </el-button-group>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination" v-if="total > pageSize">
        <el-pagination v-model:current-page="page" v-model:page-size="pageSize" :total="total" :page-sizes="[10,20,50,100]" layout="total, sizes, prev, pager, next" @size-change="fetchRecordings" @current-change="fetchRecordings" />
      </div>
    </el-card>

    <!-- 清理对话框 -->
    <el-dialog v-model="cleanupVisible" title="清理过期录像" width="400">
      <p>确定要删除 {{ cleanupDays }} 天前的所有录像吗？此操作不可恢复。</p>
      <el-form :model="cleanupForm" label-width="80">
        <el-form-item label="保留天数">
          <el-input-number v-model="cleanupForm.days" :min="1" :max="365" :controls="false" style="width: 100%" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="cleanupVisible = false">取消</el-button>
        <el-button type="danger" @click="confirmCleanup">确定清理</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Delete, Search, Download, VideoPlay } from '@element-plus/icons-vue'
import { api } from '@/api'
import { useCameraStore } from '@/stores'

const router = useRouter()

const cameraStore = useCameraStore()

const loading = ref(false)
const tableData = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

const searchForm = reactive({
  camera_id: '',
  record_type: '',
  time_range: <[string, string]>[],
})

const cleanupVisible = ref(false)
const cleanupDays = 7
const cleanupForm = reactive({ days: 7 })

const typeMap = { continuous: 'primary', motion: 'warning', schedule: 'success', manual: 'info' }
const typeLabels = { continuous: '连续', motion: '移动', schedule: '定时', manual: '手动' }

const formatTime = (time: string) => time ? new Date(time).toLocaleString('zh-CN') : '-'
const formatDuration = (sec: number) => { const h = Math.floor(sec/3600), m = Math.floor((sec%3600)/60), s = sec%60; return `${h}h${m}m${s}s` }
const formatBytes = (bytes: number) => { if(!bytes) return '0 B'; const k=1024,sizes=['B','KB','MB','GB']; const i=Math.floor(Math.log(bytes)/Math.log(k)); return parseFloat((bytes/Math.pow(k,i)).toFixed(1))+' '+sizes[i] }

const getCameraName = (id: number) => cameraStore.cameras.find(c => c.id === id)?.name || `ID:${id}`

const fetchRecordings = async () => {
  loading.value = true
  try {
    const params = { page: page.value, page_size: pageSize.value }
    if (searchForm.camera_id) params.camera_id = searchForm.camera_id
    if (searchForm.record_type) params.record_type = searchForm.record_type
    if (searchForm.time_range?.length === 2) { params.start = searchForm.time_range[0]; params.end = searchForm.time_range[1] }
    const res = await api.recordings.list(params)
    const all = res.data || res || []
    total.value = all.length
    const start = (page.value - 1) * pageSize.value
    tableData.value = all.slice(start, start + pageSize.value)
  } catch (e) { ElMessage.error('获取录像列表失败') }
  finally { loading.value = false }
}

const handleSearch = () => { page.value = 1; fetchRecordings() }
const resetSearch = () => { Object.keys(searchForm).forEach(k => searchForm[k] = ''); fetchRecordings() }

const playRecording = (rec: any) => { router.push(`/recordings/playback/${rec.camera_id}`) }
const downloadRecording = async (rec: any) => { try { const res = await api.recordings.download(rec.id); const url = window.URL.createObjectURL(new Blob([res])); const a = document.createElement('a'); a.href = url; a.download = `recording_${rec.id}.mp4`; a.click(); URL.revokeObjectURL(url) } catch(e) { ElMessage.error('下载失败') } }
const deleteRecording = (id: number) => { ElMessageBox.confirm('确定删除该录像？', '提示', {type:'warning'}).then(async()=>{try{await api.recordings.delete(id);ElMessage.success('删除成功');fetchRecordings()}catch(e){ElMessage.error('删除失败')}}).catch(()=>{}) }

const showCleanupDialog = () => { cleanupVisible.value = true }
const confirmCleanup = async () => { try { await api.storage.cleanup(); ElMessage.success('清理完成'); fetchRecordings(); cleanupVisible.value = false } catch(e) { ElMessage.error('清理失败') } }

onMounted(() => { fetchRecordings(); cameraStore.fetchCameras() })
</script>

<style scoped lang="scss">
.recording-list-page { .page-header { display:flex; justify-content:space-between; align-items:center; margin-bottom:24px; h2{margin:0;font-size:20px;font-weight:600;} .header-actions{display:flex;gap:12px;} } .search-form{:deep(.el-form-item){margin-bottom:0;}} .pagination{margin-top:16px;text-align:right;} .text-muted{color:#909399;font-size:12px;} }
</style>
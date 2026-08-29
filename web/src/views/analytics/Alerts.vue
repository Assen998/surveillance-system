<template>
  <div class="alert-list-page">
    <div class="page-header">
      <h2>报警记录</h2>
      <div class="header-actions">
        <el-button @click="fetchAlerts"><el-icon><Refresh /></el-icon> 刷新</el-button>
        <el-button type="primary" @click="ackAllUnread" :disabled="unreadCount === 0">
          <el-icon><Check /></el-icon> 全部标记已读
        </el-button>
        <el-button type="danger" @click="clearAllAlerts" :loading="clearLoading" :disabled="total === 0">
          <el-icon><Delete /></el-icon> 一键删除报警记录
        </el-button>
      </div>
    </div>

    <el-card :shadow="never" class="mb-16">
      <el-form :inline="true" :model="searchForm" class="search-form">
        <el-form-item label="摄像头">
          <el-select v-model="searchForm.camera_id" placeholder="全部" style="width: 200px">
            <el-option v-for="cam in cameraStore.cameras" :key="cam.id" :label="cam.name" :value="cam.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="报警类型">
          <el-select v-model="searchForm.type" placeholder="全部" style="width: 160px">
            <el-option label="运动检测" value="motion" />
            <el-option label="区域入侵" value="intrusion" />
            <el-option label="越界检测" value="line_cross" />
            <el-option label="目标检测" value="object_detect" />
            <el-option label="设备离线" value="offline" />
            <el-option label="存储满" value="storage_full" />
            <el-option label="系统错误" value="error" />
          </el-select>
        </el-form-item>
        <el-form-item label="报警等级">
          <el-select v-model="searchForm.level" placeholder="全部" style="width: 140px">
            <el-option label="低" value="low" />
            <el-option label="中" value="medium" />
            <el-option label="高" value="high" />
            <el-option label="严重" value="critical" />
          </el-select>
        </el-form-item>
        <el-form-item label="处理状态">
          <el-select v-model="searchForm.status" placeholder="全部" style="width: 140px">
            <el-option label="未处理" value="new" />
            <el-option label="已确认" value="acknowledged" />
            <el-option label="已解决" value="resolved" />
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

    <el-card :shadow="never">
      <el-table :data="tableData" v-loading="loading" border stripe size="small" row-key="id" style="width: 100%" highlight-current-row @current-change="handleCurrentChange">
        <el-table-column type="selection" width="55" />
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column label="摄像头" width="180">
          <template #default="scope">
            <span>{{ getCameraName(scope.row.camera_id) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="类型" width="130">
          <template #default="scope">
            <el-tag :type="typeType(scope.row.type)" size="small">{{ typeLabels[scope.row.type] }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="等级" width="100">
          <template #default="scope">
            <el-tag :type="levelType(scope.row.level)" size="small" effect="dark">{{ scope.row.level }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="message" label="内容" min-width="200" show-overflow-tooltip />
        <el-table-column label="状态" width="120">
          <template #default="scope">
            <el-tag :type="statusType(scope.row.status)" size="small">{{ statusLabels[scope.row.status] }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="报警时间" width="180">
          <template #default="scope">{{ formatTime(scope.row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="scope">
            <el-button-group size="small">
              <el-button link @click.stop="viewSnapshot(scope.row)" v-if="scope.row.snapshot_path">
                <el-icon><Picture /></el-icon> 查看图片
              </el-button>
              <el-button link type="primary" @click.stop="goToCamera(scope.row.camera_id)">
                <el-icon><VideoCamera /></el-icon> 定位摄像头
              </el-button>
              <el-button link @click.stop="ackAlert(scope.row)" v-if="scope.row.status === 'new'">
                <el-icon><Check /></el-icon> 确认
              </el-button>
              <el-button link @click.stop="resolveAlert(scope.row)" v-if="scope.row.status === 'acknowledged'">
                <el-icon><CircleCheck /></el-icon> 解决
              </el-button>
              <el-button link type="danger" @click.stop="deleteAlert(scope.row.id)">
                <el-icon><Delete /></el-icon> 删除
              </el-button>
            </el-button-group>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination" v-if="total > pageSize">
        <el-pagination v-model:current-page="page" v-model:page-size="pageSize" :total="total" :page-sizes="[10,20,50,100]" layout="total, sizes, prev, pager, next" @size-change="fetchAlerts" @current-change="fetchAlerts" />
      </div>
    </el-card>

    <!-- 报警详情对话框 -->
    <el-dialog v-model="detailVisible" title="报警详情" width="800" destroy-on-close>
      <div class="alert-detail" v-if="currentAlert">
        <div class="detail-header">
          <div class="alert-badges">
            <el-tag :type="typeType(currentAlert.type)" size="medium">{{ typeLabels[currentAlert.type] }}</el-tag>
            <el-tag :type="levelType(currentAlert.level)" size="medium" effect="dark">{{ currentAlert.level }}</el-tag>
            <el-tag :type="statusType(currentAlert.status)" size="medium">{{ statusLabels[currentAlert.status] }}</el-tag>
          </div>
          <div class="alert-time">{{ formatTime(currentAlert.created_at) }}</div>
        </div>
        <div class="detail-content">
          <p>{{ currentAlert.message }}</p>
          <div class="detail-json" v-if="currentAlert.details">
            <pre>{{ formatJson(currentAlert.details) }}</pre>
          </div>
        </div>
        <div class="detail-image" v-if="currentAlert.snapshot_path">
          <h4>现场抓拍</h4>
          <img :src="getImageUrl(currentAlert.snapshot_path)" alt="报警抓拍" @error="handleImageError" />
        </div>
        <div class="detail-actions">
          <el-button @click="ackAlert(currentAlert)" v-if="currentAlert.status === 'new'" type="primary">确认处理</el-button>
          <el-button @click="resolveAlert(currentAlert)" v-if="currentAlert.status === 'acknowledged'" type="success">标记解决</el-button>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Check, CircleCheck, Search, Picture, VideoCamera, Delete } from '@element-plus/icons-vue'
import { api } from '@/api'
import { useCameraStore } from '@/stores'

const cameraStore = useCameraStore()

const loading = ref(false)
const tableData = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const unreadCount = ref(0)

const searchForm = reactive({
  camera_id: '',
  type: '',
  level: '',
  status: '',
  time_range: <[string, string]>[],
})

const detailVisible = ref(false)
const currentAlert = ref<any>(null)

const typeLabels = { motion: '运动检测', intrusion: '区域入侵', line_cross: '越界检测', object_detect: '目标检测', offline: '设备离线', storage_full: '存储满', error: '系统错误' }
const statusLabels = { new: '未处理', acknowledged: '已确认', resolved: '已解决' }
const typeType = (t: string) => ({ motion: 'primary', intrusion: 'warning', line_cross: 'warning', object_detect: 'success', offline: 'danger', storage_full: 'warning', error: 'danger' }[t] || 'info')
const levelType = (l: string) => ({ low: 'success', medium: 'warning', high: 'danger', critical: 'danger' }[l] || 'info')
const statusType = (s: string) => ({ new: 'danger', acknowledged: 'warning', resolved: 'success' }[s] || 'info')

const formatTime = (time: string) => time ? new Date(time).toLocaleString('zh-CN') : '-'
const formatJson = (str: string) => { try { return JSON.stringify(JSON.parse(str), null, 2) } catch { return str } }
const getImageUrl = (path: string) => path.replace('./recordings', '/api/v1/stream').replace(/\\/g, '/')
const getCameraName = (id: number) => cameraStore.cameras.find(c => c.id === id)?.name || `ID:${id}`

const fetchAlerts = async () => {
  loading.value = true
  try {
    const params = { page: page.value, page_size: pageSize.value }
    if (searchForm.camera_id) params.camera_id = searchForm.camera_id
    if (searchForm.type) params.type = searchForm.type
    if (searchForm.level) params.level = searchForm.level
    if (searchForm.status) params.status = searchForm.status
    if (searchForm.time_range?.length === 2) { params.start = searchForm.time_range[0]; params.end = searchForm.time_range[1] }
    const res: any = await api.analytics.alerts(params)
    const all = res || []
    // 后端返回分页后的一页数据；total 挂在返回值上供分页器使用
    total.value = all._total ?? all.length
    unreadCount.value = all.filter((a: any) => a.status === 'new').length
    tableData.value = all
  } catch (e) { ElMessage.error('获取报警列表失败') }
  finally { loading.value = false }
}

const handleSearch = () => { page.value = 1; fetchAlerts() }
const resetSearch = () => { Object.keys(searchForm).forEach(k => searchForm[k] = ''); fetchAlerts() }

const handleCurrentChange = (row: any) => { currentAlert.value = row; detailVisible.value = true }

const viewSnapshot = (alert: any) => { currentAlert.value = alert; detailVisible.value = true }
const goToCamera = (id: number) => { /* router.push(`/cameras/${id}`) */ }

const ackAlert = async (alert: any) => {
  try { await api.analytics.acknowledge(alert.id); alert.status = 'acknowledged'; ElMessage.success('已确认'); fetchAlerts() } catch(e) { ElMessage.error('操作失败') }
}
const resolveAlert = async (alert: any) => {
  try { await api.analytics.resolve(alert.id); alert.status = 'resolved'; ElMessage.success('已解决'); fetchAlerts() } catch(e) { ElMessage.error('操作失败') }
}
const deleteAlert = (id: number) => { ElMessageBox.confirm('确定删除该报警记录？', '提示', {type:'warning'}).then(async()=>{try{await api.analytics.deleteAlert(id);ElMessage.success('删除成功');fetchAlerts()}catch(e){ElMessage.error('删除失败')}}).catch(()=>{}) }
const ackAllUnread = async () => { try { const unread = tableData.value.filter(a => a.status === 'new'); for (const a of unread) { await api.analytics.acknowledge(a.id) } ElMessage.success(`已确认 ${unread.length} 条报警`); fetchAlerts() } catch(e) { ElMessage.error('批量确认失败') } }

const clearLoading = ref(false)
const clearAllAlerts = async () => {
  try {
    await ElMessageBox.confirm(
      `确定要删除全部报警记录吗？共 ${total.value} 条，此操作不可恢复。`,
      '一键删除报警记录',
      { type: 'warning', confirmButtonText: '确定删除', cancelButtonText: '取消' }
    )
  } catch { return }
  clearLoading.value = true
  try {
    const res: any = await api.analytics.clearAlerts()
    tableData.value = []
    total.value = 0
    unreadCount.value = 0
    detailVisible.value = false
    ElMessage.success(res?.message || '已清空报警记录')
  } catch (e) {
    ElMessage.error('删除失败')
  } finally {
    clearLoading.value = false
  }
}

const handleImageError = (e: Event) => { (e.target as HTMLImageElement).style.display = 'none' }

onMounted(() => { fetchAlerts(); cameraStore.fetchCameras() })
</script>

<style scoped lang="scss">
.alert-list-page { .page-header{display:flex;justify-content:space-between;align-items:center;margin-bottom:24px;h2{margin:0;font-size:20px;font-weight:600;}.header-actions{display:flex;gap:12px;}} .search-form{:deep(.el-form-item){margin-bottom:0;}} .pagination{margin-top:16px;text-align:right;} .alert-detail{.detail-header{display:flex;justify-content:space-between;align-items:flex-start;margin-bottom:16px;padding-bottom:16px;border-bottom:1px solid #f0f0f0;.alert-badges{display:flex;gap:8px;flex-wrap:wrap;}}.detail-content{margin-bottom:16px;p{margin:0 0 12px;font-size:14px;}.detail-json{background:#f5f7fa;border-radius:8px;padding:12px;max-height:200px;overflow:auto;pre{margin:0;font-size:12px;font-family:monospace;color:#606266;}}} .detail-image{margin-bottom:16px;h4{margin:0 0 8px;font-size:14px;}img{max-width:100%;border-radius:8px;border:1px solid #e6e9ed;}}.detail-actions{display:flex;justify-content:flex-end;gap:12px;padding-top:16px;border-top:1px solid #f0f0f0;}}
}
</style>
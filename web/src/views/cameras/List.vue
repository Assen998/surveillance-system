<template>
  <div class="camera-list-page">
    <div class="page-header">
      <h2>摄像头列表</h2>
      <div class="header-actions">
        <el-button type="primary" @click="showAddDialog">
          <el-icon><Plus /></el-icon> 添加摄像头
        </el-button>
        <el-button @click="openLanScan">
          <el-icon><Search /></el-icon> 自动发现
        </el-button>
        <el-button @click="fetchCameras">
          <el-icon><Refresh /></el-icon> 刷新
        </el-button>
      </div>
    </div>

    <!-- 搜索筛选 -->
    <el-card :shadow="never" class="mb-16">
      <el-form :inline="true" :model="searchForm" class="search-form">
        <el-form-item label="关键词">
          <el-input v-model="searchForm.keyword" placeholder="名称/IP" clearable style="width: 200px" />
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="searchForm.status" placeholder="全部" style="width: 140px">
            <el-option label="在线" value="online" />
            <el-option label="离线" value="offline" />
            <el-option label="异常" value="error" />
          </el-select>
        </el-form-item>
        <el-form-item label="协议">
          <el-select v-model="searchForm.protocol" placeholder="全部" style="width: 140px">
            <el-option label="RTSP" value="rtsp" />
            <el-option label="ONVIF" value="onvif" />
            <el-option label="GB28181" value="gb28181" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button @click="handleSearch"><el-icon><Search /></el-icon> 搜索</el-button>
          <el-button @click="resetSearch">重置</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 列表 -->
    <el-card :shadow="never">
      <el-table
        :data="tableData"
        v-loading="loading"
        border
        stripe
        row-key="id"
        style="width: 100%"
        @row-click="handleRowClick"
      >
        <el-table-column type="selection" width="55" />
        <el-table-column prop="name" label="名称" min-width="160" />
        <el-table-column prop="ip" label="IP 地址" width="140" />
        <el-table-column prop="port" label="端口" width="80" />
        <el-table-column prop="protocol" label="协议" width="100">
          <template #default="scope">
            <el-tag :type="protocolType(scope.row.protocol)" size="small">{{ scope.row.protocol.toUpperCase() }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="分辨率" width="130">
          <template #default="scope">
            {{ scope.row.width }}×{{ scope.row.height }}
          </template>
        </el-table-column>
        <el-table-column label="编码/帧率" width="140">
          <template #default="scope">
            {{ scope.row.codec.toUpperCase() }} / {{ scope.row.fps }}fps
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="scope">
            <el-tag :class="['status-tag', scope.row.status]" size="small">
              {{ statusMap[scope.row.status] }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="录像" width="100">
          <template #default="scope">
            <el-switch
              v-model="scope.row.record_enabled"
              @change="toggleRecord(scope.row)"
              :active-value="true"
              :inactive-value="false"
            />
          </template>
        </el-table-column>
        <el-table-column label="录像模式" width="130">
          <template #default="scope">
            <el-tag :type="recordTypeTagType(scope.row.record_type)" size="small" effect="plain">
              {{ recordTypeLabel(scope.row.record_type) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="更新时间" width="160">
          <template #default="scope">
            {{ formatTime(scope.row.updated_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="scope">
            <el-button-group size="small">
              <el-button link type="primary" @click.stop="goToDetail(scope.row.id)">
                <el-icon><Monitor /></el-icon> 预览
              </el-button>
              <el-button link @click.stop="goToPlayback(scope.row.id)">
                <el-icon><Film /></el-icon> 回放
              </el-button>
              <el-button link @click.stop="editCamera(scope.row)">
                <el-icon><Edit /></el-icon> 编辑
              </el-button>
              <el-button link type="danger" @click.stop="deleteCamera(scope.row.id)">
                <el-icon><Delete /></el-icon> 删除
              </el-button>
            </el-button-group>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination" v-if="total > pageSize">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="total"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          @size-change="fetchCameras"
          @current-change="fetchCameras"
        />
      </div>
    </el-card>

    <!-- 局域网扫描对话框 -->
    <el-dialog v-model="lanScanDialogVisible" title="局域网 ONVIF 扫描" width="700" destroy-on-close>
      <div v-if="!lanScanning && lanScanResults.length === 0" style="text-align: center; padding: 40px;">
        <el-icon style="font-size: 48px; color: #909399;"><Search /></el-icon>
        <p class="mt-8" style="color: #909399;">点击下方按钮扫描局域网内的 ONVIF 设备</p>
        <p style="font-size: 12px; color: #c0c4cc;">优先 WS-Discovery 组播，无结果时自动回退到本机网段快速扫描（无需输入 IP 或密码）</p>
        <div class="mt-16" style="display: flex; align-items: center; justify-content: center; gap: 8px;">
          <span style="color: #606266; font-size: 13px;">扫描时长</span>
          <el-select v-model="scanTimeout" style="width: 120px">
            <el-option :value="10" label="10 秒" />
            <el-option :value="20" label="20 秒" />
            <el-option :value="30" label="30 秒" />
          </el-select>
        </div>
      </div>

      <div v-if="lanScanning" style="text-align: center; padding: 40px;">
        <el-icon class="loading-spinner" style="font-size: 32px;"><Loading /></el-icon>
        <p class="mt-8">正在扫描局域网... (约 {{ scanTimeout }} 秒)</p>
      </div>

      <div v-if="!lanScanning && lanScanResults.length > 0">
        <el-alert title="发现 {{ lanScanResults.length }} 个 ONVIF 设备，点击行选择一个添加" type="success" show-icon :closable="false" class="mb-8" />
        <el-table :data="lanScanResults" size="small" border max-height="300" @row-click="selectLanDevice" :highlight-current-row="true">
          <el-table-column prop="ip" label="IP" width="130" />
          <el-table-column prop="port" label="端口" width="70" />
          <el-table-column prop="manufacturer" label="厂商" min-width="90" />
          <el-table-column prop="model" label="型号" min-width="110" />
          <el-table-column prop="firmware" label="固件" min-width="90" />
          <el-table-column prop="auth_required" label="认证" width="90" align="center">
            <template #default="{ row }">
              <el-tag size="small" :type="row.auth_required ? 'warning' : 'success'">{{ row.auth_required ? '需登录' : '免登录' }}</el-tag>
            </template>
          </el-table-column>
        </el-table>
        <p class="mt-4" style="font-size: 12px; color: #909399;">点击行选择设备，然后点击下方「填入添加表单」；无需密码的设备会自动带出厂商信息</p>

        <div v-if="selectedLanDevice" class="mt-8 p-12" style="background: #f0f9eb; border-radius: 8px;">
          <h4 style="margin: 0 0 8px; color: #67c23a;">
            已选择: {{ selectedLanDevice.manufacturer || '未知厂商' }} {{ selectedLanDevice.model || '' }} ({{ selectedLanDevice.ip }}:{{ selectedLanDevice.port }})
          </h4>
          <p style="margin: 0; font-size: 12px; color: #909399;">
            <el-tag size="small" :type="selectedLanDevice.auth_required ? 'warning' : 'success'" style="margin-right: 8px;">{{ selectedLanDevice.auth_required ? '需在添加表单中填写用户名/密码' : '免登录设备' }}</el-tag>
          </p>
        </div>
      </div>

      <el-alert v-if="lanScanError" :title="lanScanError" type="error" show-icon :closable="false" class="mt-8" />

      <template #footer>
        <div style="width: 100%; display: flex; justify-content: space-between;">
          <el-button @click="lanScanDialogVisible = false">关闭</el-button>
          <el-button type="primary" :loading="lanScanning" @click="runLanScan" v-if="!lanScanning || lanScanResults.length === 0">
            <el-icon v-if="!lanScanning"><Search /></el-icon> {{ lanScanResults.length === 0 ? '开始扫描' : '重新扫描' }}
          </el-button>
          <el-button type="primary" :disabled="!selectedLanDevice" @click="fillFromLanScan" v-if="lanScanResults.length > 0 && !lanScanning">
            <el-icon><Plus /></el-icon> 填入添加摄像头表单
          </el-button>
        </div>
      </template>
    </el-dialog>

    <!-- 添加/编辑对话框 - 简化版 ONVIF 优先设计 -->
    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="640" destroy-on-close>
      <el-form :model="cameraForm" :rules="cameraRules" ref="cameraFormRef" label-width="120">
        
        <!-- 连接方式：默认 ONVIF，隐藏其他协议简化界面 -->
        <el-form-item label="连接方式">
          <el-radio-group v-model="cameraForm.protocol" style="display: flex; gap: 16px;">
            <el-radio value="onvif" :disabled="true">ONVIF Profile S (推荐)</el-radio>
          </el-radio-group>
          <p class="form-hint">默认使用 ONVIF 自动发现，仅需填写 IP、用户名、密码即可自动获取所有配置</p>
        </el-form-item>

        <!-- 核心输入：IP、用户名、密码 -->
        <el-form-item label="IP 地址" prop="ip">
          <el-input v-model="cameraForm.ip" placeholder="192.168.1.100" style="width: 300px" @blur="onIpBlur" />
        </el-form-item>

        <el-form-item label="用户名" prop="username">
          <el-input v-model="cameraForm.username" placeholder="admin" style="width: 300px" />
        </el-form-item>

        <el-form-item label="密码" prop="password">
          <el-input v-model="cameraForm.password" type="password" show-password placeholder="摄像头登录密码" style="width: 300px" />
        </el-form-item>

        <!-- 自动探测按钮 -->
        <el-form-item label="自动配置">
          <el-button 
            type="primary" 
            :loading="detectLoading" 
            :disabled="!cameraForm.ip || !cameraForm.username || !cameraForm.password"
            @click="autoDetectAndFill"
            style="width: 100%;"
          >
            <el-icon v-if="!detectLoading"><Search /></el-icon>
            <span v-if="!detectLoading">自动探测并填充配置</span>
            <span v-else>正在探测...</span>
          </el-button>
          <p class="form-hint" v-if="detectError" style="color: #f56c6c;">{{ detectError }}</p>
          <p class="form-hint" v-if="detectSuccess" style="color: #67c23a;">{{ detectSuccess }}</p>
        </el-form-item>

        <el-divider />

        <!-- 探测成功后显示：设备信息、Profile 选择、高级设置 -->
        <template v-if="detectedDevice">
          <el-form-item label="设备信息" class="section-title">
            <div class="section-divider" />
          </el-form-item>

          <el-form-item label="厂商/型号">
            <el-tag size="small">{{ detectedDevice.manufacturer }} {{ detectedDevice.model }}</el-tag>
            <el-tag size="small" style="margin-left: 8px;">固件: {{ detectedDevice.firmware }}</el-tag>
          </el-form-item>

          <el-form-item label="摄像头名称" prop="name">
            <el-input v-model="cameraForm.name" placeholder="自动填充：厂商+型号+IP后缀" style="width: 400px" maxlength="100" />
          </el-form-item>

          <el-form-item label="视频配置文件 (Profile)" prop="onvif_profile_token">
            <el-select v-model="cameraForm.onvif_profile_token" placeholder="请选择码流配置" style="width: 400px" clearable>
              <el-option 
                v-for="p in detectedDevice.profiles" 
                :key="p.token" 
                :label="formatProfileLabel(p)" 
                :value="p.token" 
              />
            </el-select>
            <p class="form-hint">主码流(高清)用于录像/回放，子码流(低清)用于多画面预览</p>
          </el-form-item>

          <!-- 自动填充的技术参数（只读展示，可手动修改） -->
          <el-form-item label="分辨率" v-if="selectedProfile">
            <el-row :gutter="12">
              <el-col :span="11">
                <el-input-number v-model="cameraForm.width" :disabled="true" style="width: 100%" /> px
              </el-col>
              <el-col :span="2"><span class="text-center">×</span></el-col>
              <el-col :span="11">
                <el-input-number v-model="cameraForm.height" :disabled="true" style="width: 100%" /> px
              </el-col>
            </el-row>
          </el-form-item>

          <el-form-item label="编码/帧率/码率" v-if="selectedProfile">
            <el-row :gutter="12">
              <el-col :span="8">
                <el-select v-model="cameraForm.codec" :disabled="true" style="width: 100%" placeholder="编码">
                  <el-option label="H.264" value="h264" />
                  <el-option label="H.265" value="h265" />
                </el-select>
              </el-col>
              <el-col :span="8">
                <el-input-number v-model="cameraForm.fps" :disabled="true" style="width: 100%" /> fps
              </el-col>
              <el-col :span="8">
                <el-input-number v-model="cameraForm.bitrate" :disabled="true" style="width: 100%" /> kbps
              </el-col>
            </el-row>
          </el-form-item>

          <el-form-item label="ONVIF 地址" prop="onvif_address">
            <el-input v-model="cameraForm.onvif_address" :disabled="true" style="width: 400px" />
          </el-form-item>

          <el-divider />

          <el-form-item label="高级选项" class="section-title">
            <div class="section-divider" />
          </el-form-item>

          <el-form-item label="启用 PTZ 控制" prop="ptz_enabled">
            <el-switch v-model="cameraForm.ptz_enabled" :disabled="!detectedDevice.ptzSupported" />
            <span v-if="!detectedDevice.ptzSupported" style="margin-left: 8px; color: #909399;">设备不支持 PTZ</span>
          </el-form-item>

          <el-form-item label="启用录像" prop="record_enabled">
            <el-switch v-model="cameraForm.record_enabled" />
          </el-form-item>

          <el-form-item label="录像类型" prop="record_type">
            <el-select v-model="cameraForm.record_type" placeholder="选择类型" style="width: 200px">
              <el-option label="连续录像 (7×24h)" value="continuous" />
              <el-option label="移动侦测录像" value="motion" />
              <el-option label="定时录像" value="schedule" />
            </el-select>
          </el-form-item>

          <el-form-item label="录像计划" prop="record_schedule" v-if="cameraForm.record_type === 'schedule'">
            <el-input v-model="cameraForm.record_schedule" placeholder="例如：0-23 (全天), 9-18 (工作时间), 22-6 (夜间)" style="width: 300px" />
            <p class="form-hint">格式：开始小时-结束小时，多个时段用逗号分隔，如：9-12,14-18,22-6</p>
          </el-form-item>
        </template>

        <div class="form-actions">
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" :loading="submitLoading" @click="submitCamera" :disabled="!detectedDevice">
            <el-icon><Check /></el-icon> 保存并启动
          </el-button>
        </div>
      </el-form>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Plus, Search, Refresh, Monitor, Film, Edit, Delete,
  SwitchButton, VideoCamera, Check, Loading
} from '@element-plus/icons-vue'
import { api } from '@/api'
import { useCameraStore } from '@/stores'

const router = useRouter()
const cameraStore = useCameraStore()

const loading = ref(false)
const tableData = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

// 新增：探测状态
const detectedDevice = ref<any>(null)
const detectLoading = ref(false)
const detectError = ref('')
const detectSuccess = ref('')

const onvifProfiles = ref<Array<{token: string, name: string, width: number, height: number, codec: string}>>([])
const fetchProfilesLoading = ref(false)
const detectOnvifLoading = ref(false)

const searchForm = reactive({
  keyword: '',
  status: '',
  protocol: '',
})

const dialogVisible = ref(false)
const dialogTitle = ref('添加摄像头')
const submitLoading = ref(false)
const cameraFormRef = ref()
const editingId = ref<number | null>(null)

const cameraForm = reactive({
  name: '',
  description: '',
  protocol: 'onvif',  // 默认 ONVIF
  ip: '',
  port: 80,
  path: '',
  onvif_address: '',
  discover_network: '',
  onvif_profile_token: '',
  username: '',
  password: '',
  width: 1920,
  height: 1080,
  fps: 25,
  codec: 'h264',
  bitrate: 4096,
  ptz_enabled: false,
  record_enabled: true,
  record_type: 'continuous',
  record_schedule: '0-23',
})

const cameraRules = {
  name: [{ required: true, message: '请输入摄像头名称', trigger: 'blur' }],
  ip: [{ required: true, message: '请输入 IP 地址', trigger: 'blur' }],
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
  onvif_profile_token: [{ required: true, message: '请选择视频配置文件', trigger: 'change' }],
}

const statusMap = { online: '在线', offline: '离线', error: '异常' }
const protocolType = (p: string) => ({ rtsp: 'primary', onvif: 'success', gb28181: 'warning' }[p] || 'info')
// 录像模式显示
const recordTypeLabel = (t: string) => ({ continuous: '连续录像', motion: '移动侦测录像', schedule: '定时录像' }[t] || t || '连续录像')
const recordTypeTagType = (t: string) => ({ continuous: 'success', motion: 'warning', schedule: 'primary' }[t] || 'info')

// ========== 列表加载与筛选 ==========
const allCameras = ref<any[]>([])

const applyFilters = () => {
  let list = allCameras.value
  const kw = searchForm.keyword.trim().toLowerCase()
  if (kw) {
    list = list.filter(c =>
      (c.name || '').toLowerCase().includes(kw) ||
      (c.ip || '').toLowerCase().includes(kw)
    )
  }
  if (searchForm.status) list = list.filter(c => c.status === searchForm.status)
  if (searchForm.protocol) list = list.filter(c => c.protocol === searchForm.protocol)
  return list
}

const fetchCameras = async () => {
  loading.value = true
  try {
    const res: any = await api.cameras.list()
    // 拦截器已解包 {code,data}，res 通常是数组本身
    allCameras.value = Array.isArray(res) ? res : (res?.data || [])
    const filtered = applyFilters()
    total.value = filtered.length
    const start = (page.value - 1) * pageSize.value
    tableData.value = filtered.slice(start, start + pageSize.value)
  } catch (e) {
    console.error('获取摄像头列表失败', e)
  } finally {
    loading.value = false
  }
}

const handleSearch = () => {
  page.value = 1
  fetchCameras()
}

const resetSearch = () => {
  searchForm.keyword = ''
  searchForm.status = ''
  searchForm.protocol = ''
  page.value = 1
  fetchCameras()
}

const handleRowClick = (row: any) => {
  goToDetail(row.id)
}

const formatTime = (time: string) => time ? new Date(time).toLocaleString('zh-CN') : '-'

// ========== 新增：探测相关辅助函数 (与 Add.vue 保持一致) ==========

// 当前选中的 Profile
const selectedProfile = computed(() => {
  if (!detectedDevice.value || !cameraForm.onvif_profile_token) return null
  return detectedDevice.value.profiles.find(p => p.token === cameraForm.onvif_profile_token) || null
})

// 格式化 Profile 标签
const formatProfileLabel = (p: any) => {
  const type = p.name.toLowerCase().includes('main') || p.width >= 1920 ? '主码流' : '子码流'
  return `${p.name} (${p.width}×${p.height}, ${p.codec.toUpperCase()}, ${p.bitrate}kbps) [${type}]`
}

// IP 失焦时自动填充默认名称
const onIpBlur = () => {
  if (cameraForm.ip && !cameraForm.name) {
    const suffix = cameraForm.ip.split('.').pop()
    cameraForm.name = `Camera_${suffix}`
  }
}

// 自动探测并填充所有配置 (与 Add.vue 完全一致)
const autoDetectAndFill = async () => {
  detectLoading.value = true
  detectError.value = ''
  detectSuccess.value = ''

  try {
    // axios interceptor unwraps response; probe returns { device, auth_required, error }
    const r: any = await api.cameras.probe(cameraForm.ip, cameraForm.username, cameraForm.password)
    const d = r?.device

    if (!d) {
      detectError.value = r?.error || '探测失败：无响应数据'
      return
    }

    detectedDevice.value = {
      ip: d.ip,
      port: d.port,
      name: d.name,
      manufacturer: d.manufacturer,
      model: d.model,
      firmware: d.firmware,
      serialNumber: d.serialNumber,
      hardwareId: d.hardwareId,
      mac: d.mac,
      profiles: d.profiles || [],
      xaddr: d.xaddr,
      ptzSupported: false, // 可后续扩展
    }

    // 自动填充表单
    cameraForm.onvif_address = d.xaddr
    cameraForm.port = d.port
    
    // 自动生成名称：厂商_型号_IP后缀
    if (!cameraForm.name && d.manufacturer && d.model) {
      const ipSuffix = d.ip.split('.').pop()
      cameraForm.name = `${d.manufacturer}_${d.model}_${ipSuffix}`
    }

    // 自动选择主码流（分辨率最高或名称含 main）
    if (d.profiles && d.profiles.length > 0) {
      let mainProfile = d.profiles.find(p => 
        p.name.toLowerCase().includes('main') || p.width >= 1920
      ) || d.profiles.reduce((max, p) => p.width * p.height > max.width * max.height ? p : max)
      cameraForm.onvif_profile_token = mainProfile.token
    }

    // 填充技术参数（从选中的 Profile）
    if (selectedProfile.value) {
      const p = selectedProfile.value
      cameraForm.width = p.width
      cameraForm.height = p.height
      cameraForm.fps = p.fps || 25
      cameraForm.codec = p.codec || 'h264'
      cameraForm.bitrate = p.bitrate || 4096
      if (p.rtspUri) {
        cameraForm.path = p.rtspUri
      }
    }

    detectSuccess.value = `探测成功：${d.manufacturer} ${d.model} (${d.profiles?.length || 0} 个配置文件)`
    ElMessage.success('自动配置完成，请确认后保存')

  } catch (e: any) {
    console.error(e)
    detectError.value = `探测失败：${e.message || '网络错误/认证失败/设备不支持 ONVIF'}`
    ElMessage.error(detectError.value)
  } finally {
    detectLoading.value = false
  }
}

// 监听 Profile 变化，自动更新技术参数
watch(() => cameraForm.onvif_profile_token, (newToken) => {
  if (!newToken || !detectedDevice.value) return
  const p = detectedDevice.value.profiles.find(pr => pr.token === newToken)
  if (p) {
    cameraForm.width = p.width
    cameraForm.height = p.height
    cameraForm.fps = p.fps || 25
    cameraForm.codec = p.codec || 'h264'
    cameraForm.bitrate = p.bitrate || 4096
    if (p.rtspUri) cameraForm.path = p.rtspUri
  }
})

const showAddDialog = () => {
  editingId.value = null
  dialogTitle.value = '添加摄像头'
  resetForm()
  dialogVisible.value = true
}

const editCamera = (row: any) => {
  // 跳转到独立编辑页面（与添加摄像头同一页面，预填现有参数与录像配置）
  router.push(`/cameras/edit/${row.id}`)
}

const resetForm = () => {
  Object.assign(cameraForm, {
    name: '', description: '', protocol: 'onvif', ip: '', port: 80, path: '',
    onvif_address: '', discover_network: '', onvif_profile_token: '',
    username: '', password: '', width: 1920, height: 1080, fps: 25, codec: 'h264',
    bitrate: 4096,
    ptz_enabled: false, record_enabled: true, record_type: 'continuous', record_schedule: '0-23'
  })
  
  // 清空探测状态
  detectedDevice.value = null
  detectError.value = ''
  detectSuccess.value = ''
  onIpBlur() // 确保默认 IP 触发名称生成（可选）
  cameraFormRef.value?.clearValidate()
}

const submitCamera = async () => {
  try {
    await cameraFormRef.value?.validate()
    submitLoading.value = true
    
    // 提交时只发送需要的字段
    const payload = {
      name: cameraForm.name,
      description: cameraForm.description,
      protocol: cameraForm.protocol,
      ip: cameraForm.ip,
      port: cameraForm.port,
      path: cameraForm.path,
      onvif_address: cameraForm.onvif_address,
      onvif_profile_token: cameraForm.onvif_profile_token,
      username: cameraForm.username,
      password: cameraForm.password,
      width: cameraForm.width,
      height: cameraForm.height,
      fps: cameraForm.fps,
      codec: cameraForm.codec,
      bitrate: cameraForm.bitrate,
      ptz_enabled: cameraForm.ptz_enabled,
      record_enabled: cameraForm.record_enabled,
      record_type: cameraForm.record_type,
      record_schedule: cameraForm.record_schedule,
    }

    if (editingId.value) {
      await api.cameras.update(editingId.value, payload)
      ElMessage.success('更新成功')
    } else {
      await api.cameras.create(payload)
      ElMessage.success('创建成功')
    }
    dialogVisible.value = false
    fetchCameras()
  } catch (e) {
    console.error(e)
  } finally {
    submitLoading.value = false
  }
}

const toggleRecord = async (row: any) => {
  try {
    await api.cameras.update(row.id, { record_enabled: row.record_enabled })
    ElMessage.success(row.record_enabled ? '已开启录像' : '已关闭录像')
  } catch (e) {
    row.record_enabled = !row.record_enabled
    ElMessage.error('操作失败')
  }
}

const deleteCamera = (id: number) => {
  ElMessageBox.confirm('确定要删除该摄像头吗？', '提示', { type: 'warning' })
    .then(async () => {
      try {
        await api.cameras.delete(id)
        ElMessage.success('删除成功')
        fetchCameras()
      } catch (e) { ElMessage.error('删除失败') }
    })
    .catch(() => {})
}

// ===== 局域网 WS-Discovery 扫描（真正的自动发现）=====
const lanScanDialogVisible = ref(false)
const lanScanResults = ref<any[]>([])
const lanScanning = ref(false)
const lanScanError = ref('')
const scanTimeout = ref(20)
const selectedLanDevice = ref<any>(null)

const openLanScan = () => {
  lanScanResults.value = []
  lanScanError.value = ''
  selectedLanDevice.value = null
  lanScanDialogVisible.value = true
}

const runLanScan = async () => {
  lanScanning.value = true
  lanScanError.value = ''
  lanScanResults.value = []
  selectedLanDevice.value = null
  try {
    const r: any = await api.cameras.discoverLAN(scanTimeout.value)
    if (r && Array.isArray(r)) {
      lanScanResults.value = r
      if (r.length === 0) {
        lanScanError.value = '未在局域网内发现 ONVIF 设备，请确认摄像头已开启 ONVIF 服务且与本机在同一网段'
        ElMessage.warning('未发现 ONVIF 设备')
      } else {
        ElMessage.success(`发现 ${r.length} 个 ONVIF 设备`)
      }
    } else {
      lanScanError.value = '扫描响应格式异常'
    }
  } catch (e: any) {
    console.error(e)
    lanScanError.value = '扫描失败：' + (e?.message || e?.response?.data?.error || '网络错误')
    ElMessage.error(lanScanError.value)
  } finally {
    lanScanning.value = false
  }
}

const selectLanDevice = (row: any) => {
  selectedLanDevice.value = row
}

const fillFromLanScan = () => {
  const d = selectedLanDevice.value
  if (!d) return
  showAddDialog()
  // 发现阶段没有凭据，只确定设备地址和 ONVIF 服务地址；
  // 完整设备信息/Profile 由表单内的“自动探测并填充配置”在用户填写账号密码后完成。
  cameraForm.ip = d.ip || ''
  cameraForm.port = d.port || 80
  cameraForm.onvif_address = d.xaddr || ''
  cameraForm.name = d.name || d.manufacturer || d.model || `Camera_${(d.ip || '').split('.').pop() || ''}`
  if (!cameraForm.name && d.ip) cameraForm.name = `Camera_${d.ip}`
  detectedDevice.value = null
  lanScanDialogVisible.value = false
  if (d.auth_required) {
    ElMessage.success(`已填入设备 IP：${cameraForm.ip}，请补充用户名/密码后点击「自动探测并填充配置」`)
  } else {
    ElMessage.info(`已填入设备 IP：${cameraForm.ip}，该设备无需登录（如需录像仍请确认账号权限）`)
  }
}

const goToDetail = (id: number) => router.push(`/cameras/${id}`)
const goToPlayback = (id: number) => router.push(`/recordings/playback/${id}`)

onMounted(() => fetchCameras())
</script>

<style scoped lang="scss">
.camera-list-page {
  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 24px;
    h2 { margin: 0; font-size: 20px; font-weight: 600; }
    .header-actions { display: flex; gap: 12px; }
  }

  .search-form {
    :deep(.el-form-item) { margin-bottom: 0; }
  }

  .pagination { margin-top: 16px; text-align: right; }
}
</style>
<template>
  <div class="camera-add-page">
    <div class="page-header">
      <el-button @click="goBack"><el-icon><ArrowLeft /></el-icon> {{ isEditMode ? '返回详情' : '返回列表' }}</el-button>
      <h2>{{ isEditMode ? '编辑摄像头' : '添加摄像头' }}</h2>
      <span v-if="isEditMode" class="form-hint">修改现有参数与录像配置，保存后自动重连生效</span>
    </div>

    <el-card :shadow="never">
      <el-form :model="cameraForm" :rules="cameraRules" ref="cameraFormRef" label-width="120">
        
        <!-- 协议选择（默认 ONVIF，隐藏其他协议简化界面） -->
        <el-form-item label="连接方式" prop="protocol">
          <el-radio-group v-model="cameraForm.protocol" style="display: flex; gap: 16px;">
            <el-radio value="onvif" :disabled="true">ONVIF Profile S (推荐)</el-radio>
            <!-- 如需支持其他协议可取消注释
            <el-radio value="rtsp">RTSP 手动配置</el-radio>
            <el-radio value="gb28181">GB/T 28181</el-radio>
            -->
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
          <el-input v-model="cameraForm.password" type="password" show-password :placeholder="isEditMode ? '留空表示不修改原密码' : '摄像头登录密码'" style="width: 300px" />
          <p class="form-hint" v-if="isEditMode">修改 IP/用户名/密码后，请重新「自动探测并填充配置」以刷新 Profile 列表</p>
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
            <p class="form-hint" v-if="isEditMode">仅显示当前已选 Profile；输入密码并「自动探测」可查看全部 Profile</p>
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

          <el-form-item label="RTSP 流地址" prop="path" v-if="selectedProfile?.rtspUri">
            <el-input v-model="cameraForm.path" :disabled="true" style="width: 500px" />
            <p class="form-hint">从 Profile 获取的 RTSP URI（保存后自动用于播放/录像）</p>
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
          <el-button @click="goBack">取消</el-button>
          <el-button type="primary" :loading="submitLoading" @click="submitForm" :disabled="!detectedDevice || editLoading">
            <el-icon><Check /></el-icon> {{ isEditMode ? '保存修改' : '保存并启动' }}
          </el-button>
        </div>
      </el-form>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { ArrowLeft, Check, Search } from '@element-plus/icons-vue'
import { api } from '@/api'

const router = useRouter()
const route = useRoute()

// 编辑模式：路由 /cameras/edit/:id（复用本页），预填现有参数与录像配置
const isEditMode = computed(() => route.name === 'CameraEdit')
const editId = computed(() => (route.params.id ? Number(route.params.id) : null))
const editLoading = ref(false)
const loadedRecordEnabled = ref(true) // 编辑加载时的录像开关（判断保存后是否需要重启流）

const submitLoading = ref(false)
const detectLoading = ref(false)
const cameraFormRef = ref()

const cameraForm = reactive({
  name: '',
  description: '',
  protocol: 'onvif', // 默认 ONVIF
  ip: '',
  port: 80,
  path: '',
  onvif_address: '',
  discover_network: '',
  onvif_profile_token: '',
  device_id: '',
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

// 探测到的设备信息
const detectedDevice = ref<{
  ip: string
  port: number
  name: string
  manufacturer: string
  model: string
  firmware: string
  serialNumber: string
  hardwareId: string
  mac: string
  profiles: Array<{
    token: string
    name: string
    width: number
    height: number
    fps: number
    bitrate: number
    codec: string
    rtspUri: string
    videoSourceTok: string
    videoEncoderTok: string
    ptzConfigurationToken: string
  }>
  xaddr: string
  ptzSupported: boolean
} | null>(null)

const detectError = ref('')
const detectSuccess = ref('')

// 编辑模式：加载现有摄像头，预填所有参数与录像配置
const loadCameraForEdit = async () => {
  if (!editId.value) return
  editLoading.value = true
  try {
    const res: any = await api.cameras.get(editId.value)
    const c = res?.data || res
    if (!c || !c.id) {
      ElMessage.error('摄像头不存在')
      goBack()
      return
    }
    // 预填表单（密码留空 = 保存时不修改）
    cameraForm.name = c.name || ''
    cameraForm.description = c.description || ''
    cameraForm.protocol = c.protocol || 'onvif'
    cameraForm.ip = c.ip || ''
    cameraForm.port = c.port || 80
    cameraForm.path = c.path || ''
    cameraForm.onvif_address = c.onvif_address || ''
    cameraForm.onvif_profile_token = c.onvif_profile_token || ''
    cameraForm.device_id = c.device_id || ''
    cameraForm.username = c.username || ''
    cameraForm.password = '' // 不回显密码；留空表示不修改
    cameraForm.width = c.width || 1920
    cameraForm.height = c.height || 1080
    cameraForm.fps = c.fps || 25
    cameraForm.codec = c.codec || 'h264'
    cameraForm.bitrate = c.bitrate || 4096
    cameraForm.ptz_enabled = !!c.ptz_enabled
    cameraForm.record_enabled = c.record_enabled !== false
    cameraForm.record_type = c.record_type || 'continuous'
    cameraForm.record_schedule = c.record_schedule || '0-23'
    loadedRecordEnabled.value = cameraForm.record_enabled

    // 用现有数据构造 detectedDevice，让 Profile/录像配置等区块直接可见，无需重新探测
    detectedDevice.value = {
      ip: c.ip || '',
      port: c.port || 80,
      name: c.name || '',
      manufacturer: '-',
      model: '-',
      firmware: '-',
      serialNumber: '',
      hardwareId: '',
      mac: '',
      profiles: c.onvif_profile_token ? [{
        token: c.onvif_profile_token,
        name: '当前配置',
        width: c.width || 1920,
        height: c.height || 1080,
        fps: c.fps || 25,
        bitrate: c.bitrate || 4096,
        codec: c.codec || 'h264',
        rtspUri: c.path || '',
        videoSourceTok: '',
        videoEncoderTok: '',
        ptzConfigurationToken: '',
      }] : [],
      xaddr: c.onvif_address || '',
      ptzSupported: true, // 编辑时允许切换 PTZ（真实支持情况以探测为准）
    }
  } catch (e) {
    console.error(e)
    ElMessage.error('加载摄像头信息失败')
  } finally {
    editLoading.value = false
  }
}

onMounted(() => {
  if (isEditMode.value) loadCameraForEdit()
})

const cameraRules = computed(() => ({
  name: [{ required: true, message: '请输入摄像头名称', trigger: 'blur' }],
  ip: [{ required: true, message: '请输入 IP 地址', trigger: 'blur' }],
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  // 编辑模式密码可留空（留空 = 不修改原密码）
  password: isEditMode.value
    ? [{ required: false, message: '', trigger: 'blur' }]
    : [{ required: true, message: '请输入密码', trigger: 'blur' }],
  onvif_profile_token: [{ required: true, message: '请选择视频配置文件', trigger: 'change' }],
}))

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

// 自动探测并填充所有配置
const autoDetectAndFill = async () => {
  detectLoading.value = true
  detectError.value = ''
  detectSuccess.value = ''

  try {
    const res: any = await api.cameras.probe(cameraForm.ip, cameraForm.username, cameraForm.password)
    // probe 返回 { device, auth_required, error }
    const d = res?.device

    if (!d) {
      detectError.value = res?.error || '探测失败：无响应数据'
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

const submitForm = async () => {
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
      password: cameraForm.password, // 编辑时留空 = 不修改原密码
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

    if (isEditMode.value && editId.value) {
      await api.cameras.update(editId.value, payload)
      // 录像开关未变化但流正在运行：录像模式/Profile 等变更需要重启流才生效
      // （开关变化时后端 UpdateCamera 已自动启停）
      let restarted = false
      if (cameraForm.record_enabled === loadedRecordEnabled.value) {
        try {
          const st: any = await api.cameras.status(editId.value)
          if (st?.is_streaming) {
            await api.cameras.restart(editId.value)
            restarted = true
          }
        } catch (e) { /* 重启失败不阻塞保存结果 */ }
      }
      ElMessage.success('摄像头已更新' + (restarted ? '，正在重连流生效...' : ''))
      router.push(`/cameras/${editId.value}`)
    } else {
      const res = await api.cameras.create(payload)
      ElMessage.success('摄像头添加成功，正在尝试连接...')
      router.push(`/cameras/${res.id}`)
    }
  } catch (e) {
    console.error(e)
  } finally {
    submitLoading.value = false
  }
}

const goBack = () => {
  if (isEditMode.value && editId.value) router.push(`/cameras/${editId.value}`)
  else router.push('/cameras')
}
</script>

<style scoped lang="scss">
.camera-add-page {
  .page-header { 
    display:flex; align-items:center; gap:16px; margin-bottom:24px; 
    h2{margin:0;font-size:20px;font-weight:600;} 
  }
  .section-title { margin: 24px 0 12px !important; font-size: 14px; font-weight: 600; color: #303133; }
  .section-divider { border-top: 1px solid #e6e9ed; margin-top: 8px; }
  .form-hint { margin: 4px 0 0; font-size: 12px; color: #909399; }
  .form-actions { 
    display:flex; justify-content:flex-end; gap:12px; margin-top:24px; padding-top:16px; border-top:1px solid #f0f0f0; 
  }
}
</style>
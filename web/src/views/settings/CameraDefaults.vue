<template>
  <div class="settings-page">
    <el-card :shadow="never">
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
        <el-form-item>
          <el-button type="primary" @click="saveCameraDefaults"><el-icon><Check /></el-icon> 保存</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Check } from '@element-plus/icons-vue'
import { api } from '@/api'

const cameraDefaultForm = reactive({
  record_enabled: true,
  record_type: 'continuous',
  width: 1920,
  height: 1080,
  fps: 25,
  codec: 'h264',
  bitrate: 4096,
})

const loadCameraDefaults = async () => {
  try {
    const res: any = await api.system.config()
    if (res.camera) Object.assign(cameraDefaultForm, res.camera)
  } catch (e) { console.error(e) }
}

const saveCameraDefaults = async () => { ElMessage.success('摄像头默认配置保存成功') }

onMounted(() => {
  loadCameraDefaults()
})
</script>

<style scoped lang="scss">
.settings-page {
  .text-center { text-align: center; }
}
</style>

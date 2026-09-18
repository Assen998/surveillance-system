<template>
  <div class="settings-page">
    <el-card :shadow="never">
      <template #header>
        <h3>{{ t('settingsDefaults.defaultRecordingSettings') }}</h3>
      </template>
      <el-form :model="cameraDefaultForm" label-width="160">
        <el-form-item :label="t('settingsDefaults.defaultRecordEnabled')">
          <el-switch v-model="cameraDefaultForm.record_enabled" />
        </el-form-item>
        <el-form-item :label="t('settingsDefaults.defaultRecordType')">
          <el-select v-model="cameraDefaultForm.record_type" :placeholder="t('settingsDefaults.selectType')" style="width: 200px">
            <el-option :label="t('settingsDefaults.continuous')" value="continuous" />
            <el-option :label="t('settingsDefaults.motion')" value="motion" />
            <el-option :label="t('settingsDefaults.schedule')" value="schedule" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="saveDefaults"><el-icon><Check /></el-icon> {{ t('settingsDefaults.save') }}</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card :shadow="never" class="mt-16">
      <template #header>
        <h3>{{ t('settingsDefaults.defaultVideoSettings') }}</h3>
      </template>
      <el-form :model="cameraDefaultForm" label-width="160">
        <el-form-item :label="t('settingsDefaults.defaultResolution')">
          <el-row :gutter="12">
            <el-col :span="11">
              <el-input-number v-model="cameraDefaultForm.width" :min="320" :max="8192" :controls="false" :placeholder="t('settingsDefaults.width')" style="width: 100%" />
            </el-col>
            <el-col :span="2"><span class="text-center">×</span></el-col>
            <el-col :span="11">
              <el-input-number v-model="cameraDefaultForm.height" :min="240" :max="8192" :controls="false" :placeholder="t('settingsDefaults.height')" style="width: 100%" />
            </el-col>
          </el-row>
        </el-form-item>
        <el-form-item :label="t('settingsDefaults.defaultFps')">
          <el-input-number v-model="cameraDefaultForm.fps" :min="1" :max="60" :controls="false" style="width: 120px" />
        </el-form-item>
        <el-form-item :label="t('settingsDefaults.defaultCodec')">
          <el-select v-model="cameraDefaultForm.codec" :placeholder="t('settingsDefaults.selectCodec')" style="width: 200px">
            <el-option label="H.264" value="h264" />
            <el-option label="H.265" value="h265" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('settingsDefaults.defaultBitrate')">
          <el-input-number v-model="cameraDefaultForm.bitrate" :min="512" :max="20480" :step="512" :controls="false" style="width: 120px" /> kbps
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="saveDefaults"><el-icon><Check /></el-icon> {{ t('settingsDefaults.save') }}</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card v-if="hwReport" :shadow="never" class="mt-16">
      <template #header>
        <h3>
          {{ t('settingsDefaults.hwAcceleration') }}
          <span class="hw-hint">{{ t('settingsDefaults.hwHint') }}</span>
        </h3>
      </template>
      <el-form label-width="160">
        <el-form-item :label="t('settingsDefaults.hwDecode')">
          <div class="hw-row">
            <el-switch v-model="hwForm.hw_decode" :disabled="!hwReport.decode.available" />
            <el-tag v-if="hwReport.decode.available" size="small" type="success" effect="plain" class="ml-8">
              {{ hwReport.decode.feature }} · {{ hwReport.decode.codecs.join('/') }}
            </el-tag>
            <el-tag v-else size="small" type="info" effect="plain" class="ml-8">{{ t('settingsDefaults.hwUnavailable') }}</el-tag>
          </div>
          <div class="hw-desc">
            {{ t('settingsDefaults.hwDecodeDesc') }}
            <template v-if="!hwReport.decode.available"> — {{ hwReason(hwReport.decode.reason) }}</template>
          </div>
        </el-form-item>
        <el-form-item :label="t('settingsDefaults.hwEncode')">
          <div class="hw-row">
            <el-switch v-model="hwForm.hw_encode" :disabled="!hwReport.encode.available" />
            <el-tag v-if="hwReport.encode.available" size="small" type="success" effect="plain" class="ml-8">
              {{ hwReport.encode.feature }} · {{ hwReport.encode.codecs.join('/') }}
            </el-tag>
            <el-tag v-else size="small" type="info" effect="plain" class="ml-8">{{ t('settingsDefaults.hwUnavailable') }}</el-tag>
          </div>
          <div class="hw-desc">
            {{ t('settingsDefaults.hwEncodeDesc') }}
            <template v-if="!hwReport.encode.available"> — {{ hwReason(hwReport.encode.reason) }}</template>
          </div>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="savingHw" @click="saveHW"><el-icon><Check /></el-icon> {{ t('settingsDefaults.save') }}</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Check } from '@element-plus/icons-vue'
import { api } from '@/api'

const { t } = useI18n()

const cameraDefaultForm = reactive({
  record_enabled: true,
  record_type: 'continuous',
  width: 1920,
  height: 1080,
  fps: 25,
  codec: 'h264',
  bitrate: 4096,
})

const hwForm = reactive({ hw_decode: false, hw_encode: false })
const hwReport = ref<any>(null)
const saving = ref(false)
const savingHw = ref(false)

const load = async () => {
  try {
    const [res, report] = (await Promise.all([api.system.settings(), api.system.hwCodec()])) as [any, any]
    if (res.camera_defaults) Object.assign(cameraDefaultForm, res.camera_defaults)
    hwForm.hw_decode = !!res.hw_decode
    hwForm.hw_encode = !!res.hw_encode
    hwReport.value = report
  } catch (e) {
    console.error(e)
  }
}

// 保存录像默认值（两张卡片共用一个完整对象，各卡片按钮均保存整体）
const saveDefaults = async () => {
  saving.value = true
  try {
    await api.system.updateSettings({ camera_defaults: { ...cameraDefaultForm } })
    ElMessage.success(t('settingsDefaults.saveSuccess'))
  } catch (e: any) {
    ElMessage.error(e?.message || String(e))
  } finally {
    saving.value = false
  }
}

const saveHW = async () => {
  savingHw.value = true
  try {
    await api.system.updateSettings({ hw_decode: hwForm.hw_decode, hw_encode: hwForm.hw_encode })
    ElMessage.success(t('settingsDefaults.saveSuccess'))
  } catch (e: any) {
    ElMessage.error(e?.message || String(e))
  } finally {
    savingHw.value = false
  }
}

const hwReason = (reason: string) => {
  const key = `settingsDefaults.hwReason.${reason}`
  const text = t(key)
  // i18n 未命中时回退到 no_device 文案
  return text === key ? t('settingsDefaults.hwReason.no_device') : text
}

onMounted(() => {
  load()
})
</script>

<style scoped lang="scss">
.settings-page {
  .text-center {
    text-align: center;
  }
  .hw-hint {
    font-size: 12px;
    font-weight: 400;
    color: #909399;
    margin-left: 10px;
  }
  .hw-row {
    display: flex;
    align-items: center;
  }
  .hw-desc {
    color: #909399;
    font-size: 12px;
    margin-top: 4px;
    line-height: 1.6;
  }
  .ml-8 {
    margin-left: 8px;
  }
}
</style>

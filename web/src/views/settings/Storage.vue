<template>
  <div class="settings-page">
    <el-card :shadow="never">
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
        <el-form-item label="仅存 WebDAV">
          <el-switch v-model="storageForm.webdav.only" :disabled="!storageForm.webdav.enabled" />
          <span class="form-hint">开启后录像上传成功即删除本地副本，本地仅作临时缓冲（上传失败则保留本地防丢失）；旧录像回放自动走 WebDAV 流式播放</span>
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

    <el-card :shadow="never" class="mt-16">
      <template #header>
        <h3>MinIO 对象存储（可选）</h3>
</template>
      <el-form :model="storageForm.minio" label-width="160">
        <el-form-item label="启用 MinIO">
          <el-switch v-model="storageForm.minio.enabled" />
          <span class="form-hint">开启后每个录像分段完成即上传到 MinIO（S3 兼容对象存储，本地仍保留）</span>
        </el-form-item>
        <el-form-item label="仅存 MinIO">
          <el-switch v-model="storageForm.minio.only" :disabled="!storageForm.minio.enabled" />
          <span class="form-hint">开启后录像上传成功即删除本地副本，本地仅作临时缓冲（上传失败则保留本地防丢失）；旧录像回放自动走 MinIO 流式播放</span>
        </el-form-item>
        <el-form-item label="服务地址">
          <el-input v-model="storageForm.minio.endpoint" placeholder="192.168.1.100:9000" style="width: 400px" />
          <span class="form-hint">host:port，不含 http:// 前缀</span>
        </el-form-item>
        <el-form-item label="Access Key">
          <el-input v-model="storageForm.minio.access_key" placeholder="访问密钥 ID" style="width: 300px" />
        </el-form-item>
        <el-form-item label="Secret Key">
          <el-input v-model="storageForm.minio.secret_key" type="password" show-password placeholder="留空表示不修改" style="width: 300px" />
        </el-form-item>
        <el-form-item label="Bucket">
          <el-input v-model="storageForm.minio.bucket" placeholder="surveillance" style="width: 300px" />
          <span class="form-hint">不存在时测试连接会自动创建</span>
        </el-form-item>
        <el-form-item label="使用 SSL">
          <el-switch v-model="storageForm.minio.use_ssl" />
          <span class="form-hint">局域网部署一般关闭</span>
        </el-form-item>
        <el-form-item label="远程根目录">
          <el-input v-model="storageForm.minio.base_path" placeholder="surveillance" style="width: 300px" />
          <span class="form-hint">录像将上传到 bucket 的 {远程根目录}/camera_{id}/ 前缀下</span>
        </el-form-item>
        <el-form-item label="远程保留天数">
          <el-input-number v-model="storageForm.minio.max_days" :min="0" :max="3650" :controls="false" style="width: 140px" />
          <span class="form-hint">独立于本地保留天数；0 = 不按时间自动删除（与本地清理周期同步执行）</span>
        </el-form-item>
        <el-form-item label="远程占用上限(GB)">
          <el-input-number v-model="storageForm.minio.max_storage_gb" :min="0" :max="100000" :step="0.5" :precision="1" :controls="false" style="width: 140px" />
          <span class="form-hint">0 = 不限制；超出后从最旧远程录像开始删除，直到回到上限以内</span>
        </el-form-item>
        <el-form-item>
          <el-button @click="testMinioConnection" :loading="minioTesting">
            <el-icon><Connection /></el-icon> 测试连接
          </el-button>
          <el-button type="primary" @click="saveStorageSettings" :loading="storageSaving">
            <el-icon><Check /></el-icon> 保存
          </el-button>
          <span v-if="minioTestResult" :class="['ml-12', minioTestResult.ok ? 'text-success' : 'text-danger']">
            {{ minioTestResult.message || minioTestResult.error }}
          </span>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Check, Connection } from '@element-plus/icons-vue'
import { api } from '@/api'


const storageSaving = ref(false)
const webdavTesting = ref(false)
const webdavTestResult = ref<{ ok: boolean; message?: string; error?: string } | null>(null)
const minioTesting = ref(false)
const minioTestResult = ref<{ ok: boolean; message?: string; error?: string } | null>(null)
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
    only: false,
  },
  minio: {
    enabled: false,
    endpoint: '',
    access_key: '',
    secret_key: '',
    bucket: 'surveillance',
    use_ssl: false,
    base_path: 'surveillance',
    max_days: 30,
    max_storage_gb: 0,
    only: false,
  },
})


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
        storageForm.webdav.password = ''
        storageForm.webdav.base_path = res.webdav.base_path || 'surveillance'
        storageForm.webdav.max_days = res.webdav.max_days ?? 30
        storageForm.webdav.max_storage_gb = res.webdav.max_storage_gb ?? 0
        storageForm.webdav.only = !!res.webdav.only
      }
      if (res.minio) {
        storageForm.minio.enabled = !!res.minio.enabled
        storageForm.minio.endpoint = res.minio.endpoint || ''
        storageForm.minio.access_key = res.minio.access_key || ''
        storageForm.minio.secret_key = ''
        storageForm.minio.bucket = res.minio.bucket || 'surveillance'
        storageForm.minio.use_ssl = !!res.minio.use_ssl
        storageForm.minio.base_path = res.minio.base_path || 'surveillance'
        storageForm.minio.max_days = res.minio.max_days ?? 30
        storageForm.minio.max_storage_gb = res.minio.max_storage_gb ?? 0
        storageForm.minio.only = !!res.minio.only
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
        password: storageForm.webdav.password,
        base_path: storageForm.webdav.base_path,
        max_days: storageForm.webdav.max_days,
        max_storage_gb: storageForm.webdav.max_storage_gb,
        only: storageForm.webdav.only,
      },
      minio: {
        enabled: storageForm.minio.enabled,
        endpoint: storageForm.minio.endpoint,
        access_key: storageForm.minio.access_key,
        secret_key: storageForm.minio.secret_key,
        bucket: storageForm.minio.bucket,
        use_ssl: storageForm.minio.use_ssl,
        base_path: storageForm.minio.base_path,
        max_days: storageForm.minio.max_days,
        max_storage_gb: storageForm.minio.max_storage_gb,
        only: storageForm.minio.only,
      },
    })
    ElMessage.success(res?.message || '保存成功')
    storageForm.webdav.password = ''
    storageForm.minio.secret_key = ''
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || '保存失败')
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

const testMinioConnection = async () => {
  if (!storageForm.minio.endpoint || !storageForm.minio.bucket) {
    ElMessage.warning('请填写 MinIO 服务地址与 Bucket')
    return
  }
  minioTesting.value = true
  minioTestResult.value = null
  try {
    const res: any = await api.settings.testMinio({
      endpoint: storageForm.minio.endpoint,
      access_key: storageForm.minio.access_key,
      secret_key: storageForm.minio.secret_key,
      bucket: storageForm.minio.bucket,
      use_ssl: storageForm.minio.use_ssl,
      base_path: storageForm.minio.base_path,
    })
    minioTestResult.value = { ok: !!res.ok, message: res.message, error: res.error }
    if (res.ok) ElMessage.success('MinIO 连接成功')
  } catch (e: any) {
    minioTestResult.value = { ok: false, error: e?.message || '网络错误' }
  } finally {
    minioTesting.value = false
  }
}

onMounted(() => {
  loadStorageSettings()
  loadSnapshotSettings()
})
</script>

<style scoped lang="scss">
.settings-page {
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

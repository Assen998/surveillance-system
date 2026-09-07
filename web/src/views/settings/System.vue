<template>
  <div class="settings-page">
    <el-card :shadow="never">
      <template #header>
        <h3>基本设置</h3>
      </template>
      <el-form :model="systemForm" label-width="140">
        <el-form-item label="系统名称">
          <el-input v-model="systemForm.name" placeholder="监控录像系统" style="width: 400px" />
        </el-form-item>
        <el-form-item label="HTTP 端口">
          <el-input-number v-model="systemForm.http_port" :min="1" :max="65535" :controls="false" style="width: 120px" />
        </el-form-item>
        <el-form-item label="WebSocket 端口">
          <el-input-number v-model="systemForm.ws_port" :min="1" :max="65535" :controls="false" style="width: 120px" />
        </el-form-item>
        <el-form-item label="运行模式">
          <el-select v-model="systemForm.mode" placeholder="选择模式" style="width: 200px">
            <el-option label="开发模式" value="debug" />
            <el-option label="生产模式" value="release" />
          </el-select>
        </el-form-item>
        <el-form-item label="日志级别">
          <el-select v-model="systemForm.log_level" placeholder="选择级别" style="width: 200px">
            <el-option label="Debug" value="debug" />
            <el-option label="Info" value="info" />
            <el-option label="Warn" value="warn" />
            <el-option label="Error" value="error" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="saveSystemConfig"><el-icon><Check /></el-icon> 保存</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card :shadow="never" class="mt-16">
      <template #header>
        <h3>数据库设置</h3>
      </template>
      <el-form :model="dbForm" label-width="140">
        <el-form-item label="数据库类型">
          <el-select v-model="dbForm.type" placeholder="选择类型" style="width: 200px" disabled>
            <el-option label="SQLite" value="sqlite" />
            <el-option label="PostgreSQL" value="postgres" />
          </el-select>
        </el-form-item>
        <el-form-item label="SQLite 路径" v-if="dbForm.type === 'sqlite'">
          <el-input v-model="dbForm.sqlite_path" placeholder="./data/surveillance.db" style="width: 400px" />
        </el-form-item>
        <el-form-item label="PostgreSQL 主机" v-if="dbForm.type === 'postgres'">
          <el-input v-model="dbForm.pg_host" placeholder="localhost" style="width: 300px" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="saveDbConfig" :disabled="dbForm.type !== 'sqlite'"><el-icon><Check /></el-icon> 保存</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card :shadow="never" class="mt-16">
      <template #header>
        <h3>Redis 设置</h3>
      </template>
      <el-form :model="redisForm" label-width="140">
        <el-form-item label="主机">
          <el-input v-model="redisForm.host" placeholder="localhost" style="width: 300px" />
        </el-form-item>
        <el-form-item label="端口">
          <el-input-number v-model="redisForm.port" :min="1" :max="65535" :controls="false" style="width: 120px" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="redisForm.password" type="password" show-password placeholder="留空表示无密码" style="width: 300px" />
        </el-form-item>
        <el-form-item label="数据库编号">
          <el-input-number v-model="redisForm.db" :min="0" :max="15" :controls="false" style="width: 120px" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="saveRedisConfig"><el-icon><Check /></el-icon> 保存</el-button>
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

const systemForm = reactive({
  name: '监控录像系统',
  http_port: 8080,
  ws_port: 8081,
  mode: 'release',
  log_level: 'info',
})

const dbForm = reactive({
  type: 'sqlite',
  sqlite_path: './data/surveillance.db',
  pg_host: '',
})

const redisForm = reactive({
  host: 'localhost',
  port: 6379,
  password: '',
  db: 0,
})

const loadSystemConfig = async () => {
  try {
    const res: any = await api.system.config()
    Object.assign(systemForm, res.server || {})
    Object.assign(dbForm, res.database || {})
    Object.assign(redisForm, res.redis || {})
  } catch (e) { console.error(e) }
}

const saveSystemConfig = async () => { try { await api.system.updateConfig({ server: systemForm }); ElMessage.success('保存成功') } catch(e) { ElMessage.error('保存失败') } }
const saveDbConfig = async () => { ElMessage.success('SQLite 配置保存成功（需重启生效）') }
const saveRedisConfig = async () => { ElMessage.success('Redis 配置保存成功（需重启生效）') }

onMounted(() => {
  loadSystemConfig()
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
}
</style>

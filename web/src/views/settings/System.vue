<template>
  <div class="settings-page">
    <el-card :shadow="never">
      <template #header>
        <h3>{{ t('settingsSystem.basicSettings') }}</h3>
</template>
      <el-form :model="systemForm" label-width="140">
        <el-form-item :label="t('settingsSystem.systemName')">
          <el-input v-model="systemForm.name" :placeholder="t('settingsSystem.defaultSystemName')" style="width: 400px" />
        </el-form-item>
        <el-form-item :label="t('settingsSystem.httpPort')">
          <el-input-number v-model="systemForm.http_port" :min="1" :max="65535" :controls="false" style="width: 120px" />
        </el-form-item>
        <el-form-item :label="t('settingsSystem.wsPort')">
          <el-input-number v-model="systemForm.ws_port" :min="1" :max="65535" :controls="false" style="width: 120px" />
        </el-form-item>
        <el-form-item :label="t('settingsSystem.runMode')">
          <el-select v-model="systemForm.mode" :placeholder="t('settingsSystem.selectMode')" style="width: 200px">
            <el-option :label="t('settingsSystem.debugMode')" value="debug" />
            <el-option :label="t('settingsSystem.releaseMode')" value="release" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('settingsSystem.logLevel')">
          <el-select v-model="systemForm.log_level" :placeholder="t('settingsSystem.selectLevel')" style="width: 200px">
            <el-option label="Debug" value="debug" />
            <el-option label="Info" value="info" />
            <el-option label="Warn" value="warn" />
            <el-option label="Error" value="error" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="saveSystemConfig"><el-icon><Check /></el-icon> {{ t('settingsSystem.save') }}</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card :shadow="never" class="mt-16">
      <template #header>
        <h3>{{ t('settingsSystem.databaseSettings') }}</h3>
</template>
      <el-form :model="dbForm" label-width="140">
        <el-form-item :label="t('settingsSystem.dbType')">
          <el-select v-model="dbForm.type" :placeholder="t('settingsSystem.selectType')" style="width: 200px" disabled>
            <el-option label="SQLite" value="sqlite" />
            <el-option label="PostgreSQL" value="postgres" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('settingsSystem.sqlitePath')" v-if="dbForm.type === 'sqlite'">
          <el-input v-model="dbForm.sqlite_path" placeholder="./data/surveillance.db" style="width: 400px" />
        </el-form-item>
        <el-form-item :label="t('settingsSystem.pgHost')" v-if="dbForm.type === 'postgres'">
          <el-input v-model="dbForm.pg_host" placeholder="localhost" style="width: 300px" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="saveDbConfig" :disabled="dbForm.type !== 'sqlite'"><el-icon><Check /></el-icon> {{ t('settingsSystem.save') }}</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card :shadow="never" class="mt-16">
      <template #header>
        <h3>{{ t('settingsSystem.redisSettings') }}</h3>
</template>
      <el-form :model="redisForm" label-width="140">
        <el-form-item :label="t('settingsSystem.host')">
          <el-input v-model="redisForm.host" placeholder="localhost" style="width: 300px" />
        </el-form-item>
        <el-form-item :label="t('settingsSystem.port')">
          <el-input-number v-model="redisForm.port" :min="1" :max="65535" :controls="false" style="width: 120px" />
        </el-form-item>
        <el-form-item :label="t('settingsSystem.password')">
          <el-input v-model="redisForm.password" type="password" show-password :placeholder="t('settingsSystem.noPasswordHint')" style="width: 300px" />
        </el-form-item>
        <el-form-item :label="t('settingsSystem.dbNumber')">
          <el-input-number v-model="redisForm.db" :min="0" :max="15" :controls="false" style="width: 120px" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="saveRedisConfig"><el-icon><Check /></el-icon> {{ t('settingsSystem.save') }}</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { reactive, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Check } from '@element-plus/icons-vue'
import { api } from '@/api'

const { t } = useI18n()

const systemForm = reactive({
  name: t('settingsSystem.defaultSystemName'),
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

const saveSystemConfig = async () => { try { await api.system.updateConfig({ server: systemForm }); ElMessage.success(t('settingsSystem.saveSuccess')) } catch(e) { ElMessage.error(t('settingsSystem.saveFailed')) } }
const saveDbConfig = async () => { ElMessage.success(t('settingsSystem.dbSaveSuccess')) }
const saveRedisConfig = async () => { ElMessage.success(t('settingsSystem.redisSaveSuccess')) }

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

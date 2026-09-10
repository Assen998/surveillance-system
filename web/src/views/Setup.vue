<template>
  <div class="setup-page">
    <div class="setup-card">
      <div class="setup-header">
        <div class="setup-title">{{ t("setup.appTitle") }}</div>
        <div class="setup-subtitle">{{ t("setup.subtitle") }}</div>
      </div>


      <el-result
        v-if="!needSetup"
        icon="success"
        :title="t('setup.doneTitle')"
        :sub-title="t('setup.doneSubtitle')"
      >
        <template #extra>
          <el-button type="primary" @click="router.replace('/login')">{{ t("setup.goLogin") }}</el-button>
</template>
      </el-result>

      <template v-else>

        <div class="setup-section">
          <div class="section-head">
            <span class="section-title">
              <el-tag size="small" type="primary" class="mr-8">1</el-tag>{{ t("setup.step1Title") }}
            </span>
            <el-button size="small" :loading="envLoading" @click="loadEnv">
              <el-icon v-if="!envLoading"><Refresh /></el-icon> {{ t("setup.recheck") }}
            </el-button>
          </div>
          <EnvCheckTable :report="envReport" :loading="envLoading" />
        </div>


        <div class="setup-section">
          <div class="section-head">
            <span class="section-title">
              <el-tag size="small" type="primary" class="mr-8">2</el-tag>{{ t("setup.step2Title") }}
            </span>
          </div>
          <el-alert
            v-if="envReport && !envReport.critical_ok"
            :title="t('setup.criticalWarn')"
            type="error" :closable="false" show-icon class="mb-16"
          />
          <el-form :model="form" :rules="rules" ref="formRef" label-width="110">
            <el-form-item :label="t('setup.username')" prop="username">
              <el-input v-model="form.username" :placeholder="t('setup.usernamePlaceholder')" style="width: 320px" maxlength="50" />
            </el-form-item>
            <el-form-item :label="t('setup.password')" prop="password">
              <el-input
                v-model="form.password"
                type="password"
                show-password
                :placeholder="t('setup.passwordPlaceholder')"
                style="width: 320px"
              />
            </el-form-item>
            <el-form-item :label="t('setup.confirm')" prop="confirm">
              <el-input
                v-model="form.confirm"
                type="password"
                show-password
                :placeholder="t('setup.confirmPlaceholder')"
                style="width: 320px"
              />
            </el-form-item>
            <el-form-item>
              <el-button
                type="primary"
                size="large"
                :loading="submitting"
                :disabled="envLoading || (envReport ? !envReport.critical_ok : true)"
                @click="submit"
              >
                {{ t("setup.submit") }}
              </el-button>
            </el-form-item>
          </el-form>
        </div>

        <p class="setup-note">
          {{ t("setup.note") }}
        </p>

        <div class="setup-footer">
          <el-dropdown trigger="click" @command="switchLang">
            <span class="lang-switch">
              <el-icon><Position /></el-icon>
              {{ currentLangLabel }}
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-for="l in SUPPORTED_LANGS" :key="l.value" :command="l.value" :disabled="l.value === lang">
                  {{ l.label }}
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
</template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import type { FormInstance } from 'element-plus'
import { Refresh, Position } from '@element-plus/icons-vue'
import { api } from '@/api'
import EnvCheckTable from '@/components/EnvCheckTable.vue'
import { useI18n } from 'vue-i18n'
import { setLang, SUPPORTED_LANGS } from '@/i18n'

const { t, locale } = useI18n()

const lang = computed(() => locale.value as string)
const currentLangLabel = computed(() => SUPPORTED_LANGS.find(l => l.value === lang.value)?.label || '中文')
const switchLang = (v: string) => setLang(v as 'zh' | 'en')

const router = useRouter()
const formRef = ref<FormInstance>()
const needSetup = ref(true)
const envReport = ref<any>(null)
const envLoading = ref(false)
const submitting = ref(false)

const form = reactive({
  username: 'admin',
  password: '',
  confirm: '',
})

const rules = computed(() => ({
  username: [
    { required: true, message: t('setup.usernameRequired'), trigger: 'blur' },
    { min: 3, max: 50, message: t('setup.usernameLength'), trigger: 'blur' },
  ],
  password: [
    { required: true, message: t('setup.passwordRequired'), trigger: 'blur' },
    { min: 6, message: t('setup.passwordMin'), trigger: 'blur' },
  ],
  confirm: [
    { required: true, message: t('setup.confirmRequired'), trigger: 'blur' },
    {
      validator: (_: any, value: string, cb: any) => {
        if (value !== form.password) cb(new Error(t('setup.confirmMismatch')))
        else cb()
      },
      trigger: 'blur',
    },
  ],
}))

const loadEnv = async () => {
  envLoading.value = true
  try {
    const res: any = await api.setup.status()
    needSetup.value = !!res?.need_setup
    envReport.value = res?.env || null
  } catch (e) {

    console.error(e)
  } finally {
    envLoading.value = false
  }
}

const submit = async () => {
  try {
    await formRef.value?.validate()
    submitting.value = true
    const res: any = await api.setup.create({
      username: form.username,
      password: form.password,
    })

    if (res?.token) {
      localStorage.setItem('token', res.token)
    }
    ElMessage.success(t('setup.success', { name: form.username }))
    router.replace('/dashboard')
  } catch (e) {

    console.error(e)
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  loadEnv()
})
</script>

<style scoped>
.setup-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #1f2d3d 0%, #2c3e50 55%, #34495e 100%);
  padding: 24px;
}
.setup-card {
  width: 100%;
  max-width: 780px;
  background: #fff;
  border-radius: 12px;
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.25);
  padding: 40px 44px;
}
.setup-header {
  text-align: center;
  margin-bottom: 28px;
}
.setup-title {
  font-size: 26px;
  font-weight: 700;
  color: #1f2d3d;
}
.setup-subtitle {
  margin-top: 6px;
  font-size: 14px;
  color: #909399;
}
.setup-section {
  margin-top: 24px;
  padding-top: 20px;
  border-top: 1px solid #ebeef5;
}
.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
}
.section-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
  display: inline-flex;
  align-items: center;
}
.mr-8 {
  margin-right: 8px;
}
.mb-16 {
  margin-bottom: 16px;
}
.setup-note {
  margin-top: 8px;
  text-align: center;
  font-size: 12px;
  color: #b1b3b8;
}
.setup-footer {
  margin-top: 24px;
  text-align: center;
  .lang-switch {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    cursor: pointer;
    outline: none;
  }
}
</style>

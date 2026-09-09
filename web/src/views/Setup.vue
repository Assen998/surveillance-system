<template>
  <div class="setup-page">
    <div class="setup-card">
      <div class="setup-header">
        <div class="setup-title">监控录像系统</div>
        <div class="setup-subtitle">首次运行初始化</div>
      </div>

      <!-- 已完成设置 -->
      <el-result
        v-if="!needSetup"
        icon="success"
        title="初始化设置已完成"
        sub-title="系统管理员账户已存在，请直接登录"
      >
        <template #extra>
          <el-button type="primary" @click="router.replace('/login')">前往登录</el-button>
        </template>
      </el-result>

      <template v-else>
        <!-- 第一步：环境检测 -->
        <div class="setup-section">
          <div class="section-head">
            <span class="section-title">
              <el-tag size="small" type="primary" class="mr-8">1</el-tag>运行环境检测
            </span>
            <el-button size="small" :loading="envLoading" @click="loadEnv">
              <el-icon v-if="!envLoading"><Refresh /></el-icon> 重新检测
            </el-button>
          </div>
          <EnvCheckTable :report="envReport" :loading="envLoading" />
        </div>

        <!-- 第二步：创建管理员账户 -->
        <div class="setup-section">
          <div class="section-head">
            <span class="section-title">
              <el-tag size="small" type="primary" class="mr-8">2</el-tag>创建管理员账户
            </span>
          </div>
          <el-alert
            v-if="envReport && !envReport.critical_ok"
            title="关键环境检测未通过，请先按上方提示修复（如安装 ffmpeg）后再创建账户"
            type="error" :closable="false" show-icon class="mb-16"
          />
          <el-form :model="form" :rules="rules" ref="formRef" label-width="110">
            <el-form-item label="管理员用户名" prop="username">
              <el-input v-model="form.username" placeholder="admin" style="width: 320px" maxlength="50" />
            </el-form-item>
            <el-form-item label="登录密码" prop="password">
              <el-input
                v-model="form.password"
                type="password"
                show-password
                placeholder="至少 6 位，建议 10 位以上"
                style="width: 320px"
              />
            </el-form-item>
            <el-form-item label="确认密码" prop="confirm">
              <el-input
                v-model="form.confirm"
                type="password"
                show-password
                placeholder="再次输入密码"
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
                完成设置并进入系统
              </el-button>
            </el-form-item>
          </el-form>
        </div>

        <p class="setup-note">
          完成设置后此页面不再可访问；账户信息仅保存在本机数据库中，请妥善保管。
        </p>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import type { FormInstance } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { api } from '@/api'
import EnvCheckTable from '@/components/EnvCheckTable.vue'

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

const rules = {
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { min: 3, max: 50, message: '用户名长度 3~50 个字符', trigger: 'blur' },
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 6, message: '密码至少 6 位', trigger: 'blur' },
  ],
  confirm: [
    { required: true, message: '请再次输入密码', trigger: 'blur' },
    {
      validator: (_: any, value: string, cb: any) => {
        if (value !== form.password) cb(new Error('两次输入的密码不一致'))
        else cb()
      },
      trigger: 'blur',
    },
  ],
}

const loadEnv = async () => {
  envLoading.value = true
  try {
    const res: any = await api.setup.status()
    needSetup.value = !!res?.need_setup
    envReport.value = res?.env || null
  } catch (e) {
    // 错误提示由响应拦截器统一处理
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
    // 直接使用返回的令牌登录
    if (res?.token) {
      localStorage.setItem('token', res.token)
    }
    ElMessage.success(`欢迎，${form.username}！初始化设置完成`)
    router.replace('/dashboard')
  } catch (e) {
    // 错误提示由响应拦截器统一处理（展示后端 {error} 文案）
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
</style>

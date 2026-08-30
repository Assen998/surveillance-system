<template>
  <div class="settings-page">
    <el-card :shadow="never" class="mb-16">
      <template #header>
        <h3>Webhook 配置 (钉钉/飞书/企业微信/Gotify)</h3>
      </template>
      <el-form :model="webhookForm" label-width="140">
        <el-form-item label="启用 Webhook">
          <el-switch v-model="webhookForm.enabled" />
        </el-form-item>
        <el-form-item label="推送类型">
          <el-select v-model="webhookForm.type" style="width: 240px">
            <el-option label="通用 JSON（钉钉/飞书/企微等）" value="generic" />
            <el-option label="Gotify" value="gotify" />
          </el-select>
          <span class="form-hint">Gotify 选此项后，URL 填 Gotify 的 /message?token=xxx 地址</span>
        </el-form-item>
        <el-form-item label="Webhook URL">
          <el-input
            v-model="webhookForm.url"
            :placeholder="webhookForm.type === 'gotify' ? 'https://gotify.example.com/message?token=xxx' : 'https://oapi.dingtalk.com/robot/send?access_token=xxx'"
            style="width: 500px"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="testWebhook"><el-icon><Connection /></el-icon> 测试连接</el-button>
          <el-button @click="saveWebhook"><el-icon><Check /></el-icon> 保存</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card :shadow="never" class="mb-16">
      <template #header>
        <h3>邮件通知配置</h3>
      </template>
      <el-form :model="emailForm" label-width="140">
        <el-form-item label="启用邮件通知">
          <el-switch v-model="emailForm.enabled" />
        </el-form-item>
        <el-form-item label="SMTP 服务器">
          <el-input v-model="emailForm.smtp_host" placeholder="smtp.example.com" style="width: 300px" />
        </el-form-item>
        <el-form-item label="SMTP 端口">
          <el-input-number v-model="emailForm.smtp_port" :min="1" :max="65535" :controls="false" style="width: 120px" />
        </el-form-item>
        <el-form-item label="用户名">
          <el-input v-model="emailForm.username" placeholder="your@email.com" style="width: 300px" />
        </el-form-item>
        <el-form-item label="密码/授权码">
          <el-input v-model="emailForm.password" type="password" show-password placeholder="授权码" style="width: 300px" />
        </el-form-item>
        <el-form-item label="发件人">
          <el-input v-model="emailForm.from" placeholder="Surveillance System <noreply@example.com>" style="width: 400px" />
        </el-form-item>
        <el-form-item label="收件人">
          <el-input v-model="emailForm.to" placeholder="输入邮箱，回车添加" style="width: 100%" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="testEmail"><el-icon><Connection /></el-icon> 测试发送</el-button>
          <el-button @click="saveEmail"><el-icon><Check /></el-icon> 保存</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card :shadow="never" class="mb-16">
      <template #header>
        <h3>短信通知配置</h3>
      </template>
      <el-form :model="smsForm" label-width="140">
        <el-form-item label="启用短信通知">
          <el-switch v-model="smsForm.enabled" />
        </el-form-item>
        <el-form-item label="服务商">
          <el-select v-model="smsForm.provider" placeholder="选择服务商" style="width: 200px">
            <el-option label="阿里云" value="aliyun" />
            <el-option label="腾讯云" value="tencent" />
          </el-select>
        </el-form-item>
        <el-form-item label="Access Key">
          <el-input v-model="smsForm.access_key" placeholder="AccessKey ID" style="width: 300px" />
        </el-form-item>
        <el-form-item label="Secret Key">
          <el-input v-model="smsForm.secret_key" type="password" show-password placeholder="AccessKey Secret" style="width: 300px" />
        </el-form-item>
        <el-form-item label="签名名称">
          <el-input v-model="smsForm.sign_name" placeholder="监控系统" style="width: 300px" />
        </el-form-item>
        <el-form-item label="模板代码">
          <el-input v-model="smsForm.template_code" placeholder="SMS_123456789" style="width: 300px" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="testSms"><el-icon><Connection /></el-icon> 测试发送</el-button>
          <el-button @click="saveSms"><el-icon><Check /></el-icon> 保存</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card :shadow="never">
      <template #header>
        <h3>报警推送测试</h3>
      </template>
      <div class="test-actions">
        <el-button type="primary" @click="sendTestAlert('webhook')" :loading="testLoading.webhook">
          <el-icon><Bell /></el-icon> 测试 Webhook
        </el-button>
        <el-button type="success" @click="sendTestAlert('email')" :loading="testLoading.email">
          <el-icon><Message /></el-icon> 测试邮件
        </el-button>
        <el-button type="warning" @click="sendTestAlert('sms')" :loading="testLoading.sms">
          <el-icon><Phone /></el-icon> 测试短信
        </el-button>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Connection, Check, Bell, Message, Phone } from '@element-plus/icons-vue'
import { api } from '@/api'

const testLoading = reactive({ webhook: false, email: false, sms: false })

const webhookForm = reactive({ enabled: false, url: '', type: 'generic' })
const emailForm = reactive({ enabled: false, smtp_host: '', smtp_port: 587, username: '', password: '', from: '', to: <string[]>[] })
const smsForm = reactive({ enabled: false, provider: 'aliyun', access_key: '', secret_key: '', sign_name: '', template_code: '' })

const loadConfig = async () => {
  try {
    const res = await api.alerts.config()
    Object.assign(webhookForm, res.channels?.webhook || {})
    if (!webhookForm.type) webhookForm.type = 'generic'
    Object.assign(emailForm, res.channels?.email || {})
    Object.assign(smsForm, res.channels?.sms || {})
  } catch (e) { console.error(e) }
}

const saveWebhook = async () => { try { await api.alerts.updateConfig({ channels: { webhook: webhookForm } }); ElMessage.success('保存成功') } catch(e) { ElMessage.error('保存失败') } }
const saveEmail = async () => { try { await api.alerts.updateConfig({ channels: { email: emailForm } }); ElMessage.success('保存成功') } catch(e) { ElMessage.error('保存失败') } }
const saveSms = async () => { try { await api.alerts.updateConfig({ channels: { sms: smsForm } }); ElMessage.success('保存成功') } catch(e) { ElMessage.error('保存失败') } }

const testWebhook = async () => { testLoading.webhook = true; try { await api.alerts.test('webhook', { webhook: webhookForm }); ElMessage.success('测试消息已发送') } catch(e) { ElMessage.error('发送失败') } finally { testLoading.webhook = false } }
const testEmail = async () => { testLoading.email = true; try { await api.alerts.test('email', { email: emailForm }); ElMessage.success('测试邮件已发送') } catch(e) { ElMessage.error('发送失败') } finally { testLoading.email = false } }
const testSms = async () => { testLoading.sms = true; try { await api.alerts.test('sms', { sms: smsForm }); ElMessage.success('测试短信已发送') } catch(e) { ElMessage.error('发送失败') } finally { testLoading.sms = false } }

const sendTestAlert = async (channel: string) => { const formMap: Record<string, any> = { webhook: webhookForm, email: emailForm, sms: smsForm }; testLoading[channel] = true; try { await api.alerts.test(channel, { [channel]: formMap[channel] }); ElMessage.success(`${channel} 测试报警已发送`) } catch(e) { ElMessage.error('发送失败') } finally { testLoading[channel] = false } }

onMounted(() => loadConfig())
</script>

<style scoped lang="scss">
.settings-page { .test-actions{display:flex;gap:12px;flex-wrap:wrap;} }
</style>
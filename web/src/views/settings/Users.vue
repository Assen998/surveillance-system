<template>
  <div class="settings-page">
    <el-card :shadow="never" class="mb-16">
      <template #header>
        <div class="card-header">
          <h3>{{ t('users.title') }}</h3>
          <el-button type="primary" @click="showAddUserDialog">
            <el-icon><UserFilled /></el-icon> {{ t('users.addUser') }}
          </el-button>
        </div>
      </template>

      <el-table :data="users" border stripe size="small" style="width: 100%">
        <el-table-column prop="username" :label="t('users.colUsername')" width="150" />
        <el-table-column prop="email" :label="t('users.colEmail')" width="220" />
        <el-table-column prop="phone" :label="t('users.colPhone')" width="150" />
        <el-table-column :label="t('users.colRole')" width="120">
          <template #default="scope">
            <el-tag :type="roleType(scope.row.role)" size="small">{{ t('users.role' + scope.row.role) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('users.colStatus')" width="100">
          <template #default="scope">
            <el-tag :type="scope.row.status === 'active' ? 'success' : 'danger'" size="small">
              {{ t('users.status' + scope.row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="last_login" :label="t('users.colLastLogin')" width="180">
          <template #default="scope">{{ formatTime(scope.row.last_login) }}</template>
        </el-table-column>
        <el-table-column :label="t('users.colActions')" width="240" fixed="right">
          <template #default="scope">
            <el-button-group size="small">
              <el-button link @click.stop="editUser(scope.row)">
                <el-icon><Edit /></el-icon> {{ t('users.edit') }}
              </el-button>
              <el-button link :type="scope.row.status === 'active' ? 'warning' : 'success'" @click.stop="toggleUserStatus(scope.row)">
                <el-icon><UserFilled v-if="scope.row.status === 'active'" /><User /></el-icon>
                {{ t('users.' + (scope.row.status === 'active' ? 'disable' : 'enable')) }}
              </el-button>
              <el-button link @click.stop="resetPassword(scope.row)">
                <el-icon><Lock /></el-icon> {{ t('users.resetPassword') }}
              </el-button>
              <el-button link @click.stop="assignPermissions(scope.row)">
                <el-icon><VideoCamera /></el-icon> {{ t('users.assignPerm') }}
              </el-button>
              <el-button link type="danger" @click.stop="deleteUser(scope.row.id)" v-if="scope.row.username !== 'admin'">
                <el-icon><Delete /></el-icon> {{ t('users.delete') }}
              </el-button>
            </el-button-group>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 添加/编辑用户对话框 -->
    <el-dialog v-model="userDialogVisible" :title="userDialogTitle" width="500" destroy-on-close>
      <el-form :model="userForm" :rules="userRules" ref="userFormRef" label-width="100">
        <el-form-item :label="t('users.username')" prop="username">
          <el-input v-model="userForm.username" :placeholder="t('users.placeholderUsername')" :disabled="editingUserId !== null" />
        </el-form-item>
        <el-form-item :label="t('users.email')" prop="email">
          <el-input v-model="userForm.email" :placeholder="t('users.placeholderEmail')" />
        </el-form-item>
        <el-form-item :label="t('users.phone')" prop="phone">
          <el-input v-model="userForm.phone" :placeholder="t('users.placeholderPhone')" />
        </el-form-item>
        <el-form-item :label="t('users.role')" prop="role">
          <el-select v-model="userForm.role" :placeholder="t('users.placeholderRole')" style="width: 100%">
            <el-option :label="t('users.roleAdmin')" value="admin" />
            <el-option :label="t('users.roleOperator')" value="operator" />
            <el-option :label="t('users.roleViewer')" value="viewer" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('users.status')" prop="status">
          <el-select v-model="userForm.status" :placeholder="t('users.placeholderStatus')" style="width: 100%">
            <el-option :label="t('users.statusActive')" value="active" />
            <el-option :label="t('users.statusDisabled')" value="disabled" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('users.password')" prop="password" v-if="editingUserId === null">
          <el-input v-model="userForm.password" type="password" show-password :placeholder="t('users.placeholderPassword')" />
        </el-form-item>
        <el-form-item :label="t('users.confirmPassword')" prop="confirmPassword" v-if="editingUserId === null">
          <el-input v-model="userForm.confirmPassword" type="password" show-password :placeholder="t('users.placeholderConfirmPassword')" />
        </el-form-item>
        <el-form-item :label="t('users.newPassword')" prop="newPassword" v-if="editingUserId !== null">
          <el-input v-model="userForm.newPassword" type="password" show-password :placeholder="t('users.placeholderNewPassword')" />
        </el-form-item>
        <el-form-item :label="t('users.confirmNewPassword')" prop="confirmNewPassword" v-if="editingUserId !== null">
          <el-input v-model="userForm.confirmNewPassword" type="password" show-password :placeholder="t('users.placeholderConfirmNewPassword')" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="userDialogVisible = false">{{ t('users.cancel') }}</el-button>
        <el-button type="primary" :loading="userSubmitLoading" @click="submitUser">{{ t('users.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- 权限分配对话框 -->
    <el-dialog v-model="permDialogVisible" :title="t('users.permTitle')" width="600" destroy-on-close>
      <p class="mb-16">{{ t('users.permUser', { name: permUserName }) }}</p>
      <el-table :data="cameraPermissions" border size="small" style="width: 100%">
        <el-table-column prop="camera_name" :label="t('users.permColCamera')" width="200" />
        <el-table-column :label="t('users.permColCurrent')" width="150">
          <template #default="scope">
            <el-tag :type="permType(scope.row.permission)" size="small">{{ t('users.perm' + scope.row.permission) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('users.permColSet')" width="200">
          <template #default="scope">
            <el-select v-model="scope.row.permission" :placeholder="t('users.placeholderPermission')" style="width: 100%" @change="updateUserPermission(permUserId, scope.row.camera_id, scope.row.permission)">
              <el-option :label="t('users.permNone')" value="none" />
              <el-option :label="t('users.permView')" value="view" />
              <el-option :label="t('users.permControl')" value="control" />
              <el-option :label="t('users.permConfig')" value="config" />
              <el-option :label="t('users.permAdmin')" value="admin" />
            </el-select>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { UserFilled, Edit, Lock, Delete, User, VideoCamera } from '@element-plus/icons-vue'
import { api } from '@/api'
import { useI18n } from 'vue-i18n'

const { t, locale } = useI18n()

const users = ref<any[]>([])
const userDialogVisible = ref(false)
const userDialogTitle = computed(() => editingUserId.value ? t('users.editUserDialog') : t('users.addUserDialog'))
const userSubmitLoading = ref(false)
const userFormRef = ref()
const editingUserId = ref<number | null>(null)

const userForm = reactive({
  username: '', email: '', phone: '', role: 'viewer', status: 'active',
  password: '', confirmPassword: '', newPassword: '', confirmNewPassword: '',
})

const userRules = computed(() => ({
  username: [{ required: true, message: t('users.usernameRequired'), trigger: 'blur' }],
  email: [
    { required: true, message: t('users.emailRequired'), trigger: 'blur' },
    { type: 'email', message: t('users.emailInvalid'), trigger: 'blur' },
  ],
  role: [{ required: true, message: t('users.roleRequired'), trigger: 'change' }],
  status: [{ required: true, message: t('users.statusRequired'), trigger: 'change' }],
  password: [
    { required: true, message: t('users.passwordRequired'), trigger: 'blur' },
    { min: 6, message: t('users.passwordMin'), trigger: 'blur' },
  ],
  confirmPassword: [
    { required: true, message: t('users.confirmPasswordRequired'), trigger: 'blur' },
    { validator: (_: any, value: string) => value === userForm.password ? Promise.resolve() : Promise.reject(t('users.passwordMismatch')) },
  ],
  newPassword: [
    { validator: (_: any, value: string) => value ? (value.length >= 6 ? Promise.resolve() : Promise.reject(t('users.newPasswordMin'))) : Promise.resolve() },
  ],
  confirmNewPassword: [
    { validator: (_: any, value: string) => value === userForm.newPassword ? Promise.resolve() : Promise.reject(t('users.confirmNewPasswordMismatch')) },
  ],
}))

const permDialogVisible = ref(false)
const permUserId = ref<number>(0)
const permUserName = ref('')
const cameraPermissions = ref<any[]>([])

const roleType = (r: string) => ({ admin: 'danger', operator: 'warning', viewer: 'info' }[r] || 'info')
const permType = (p: string) => ({ none: 'info', view: 'success', control: 'primary', config: 'warning', admin: 'danger' }[p] || 'info')

const formatTime = (time: string | null) => time ? new Date(time).toLocaleString(locale.value === 'en' ? 'en-US' : 'zh-CN') : t('users.neverLoggedIn')

const fetchUsers = async () => {
  try {
    const res: any = await api.users.list()
    users.value = res.data || res || []
  } catch (e) { /* 拦截器已提示 */ }
}

const showAddUserDialog = () => {
  editingUserId.value = null
  resetUserForm()
  userDialogVisible.value = true
}

const editUser = (row: any) => {
  editingUserId.value = row.id
  userForm.username = row.username
  userForm.email = row.email || ''
  userForm.phone = row.phone || ''
  userForm.role = row.role
  userForm.status = row.status
  userForm.password = ''
  userForm.confirmPassword = ''
  userForm.newPassword = ''
  userForm.confirmNewPassword = ''
  userDialogVisible.value = true
}

const resetUserForm = () => {
  Object.assign(userForm, { username: '', email: '', phone: '', role: 'viewer', status: 'active', password: '', confirmPassword: '', newPassword: '', confirmNewPassword: '' })
  userFormRef.value?.clearValidate()
}

const submitUser = async () => {
  await userFormRef.value?.validate()
  userSubmitLoading.value = true
  try {
    if (editingUserId.value) {
      const data: any = {
        email: userForm.email,
        phone: userForm.phone,
        role: userForm.role,
        status: userForm.status,
      }
      if (userForm.newPassword) data.new_password = userForm.newPassword
      await api.users.update(editingUserId.value, data)
      ElMessage.success(t('users.updateSuccess'))
    } else {
      await api.users.create({
        username: userForm.username,
        password: userForm.password,
        email: userForm.email,
        phone: userForm.phone,
        role: userForm.role,
        status: userForm.status,
      })
      ElMessage.success(t('users.createSuccess'))
    }
    userDialogVisible.value = false
    fetchUsers()
  } catch (e) { /* 拦截器已提示 */ }
  finally { userSubmitLoading.value = false }
}

const toggleUserStatus = async (row: any) => {
  const newStatus = row.status === 'active' ? 'disabled' : 'active'
  try {
    await api.users.update(row.id, { status: newStatus })
    ElMessage.success(t('users.toggle' + (newStatus === 'active' ? 'Enabled' : 'Disabled')))
    fetchUsers()
  } catch (e) { /* 拦截器已提示 */ }
}

const resetPassword = (row: any) => {
  ElMessageBox.prompt(t('users.resetPasswordPrompt'), t('users.resetPasswordTitle', { username: row.username }), {
    confirmButtonText: t('users.confirm'),
    cancelButtonText: t('users.cancel'),
    inputType: 'password',
    inputPattern: /^.{6,}$/,
    inputErrorMessage: t('users.resetPasswordError'),
  }).then(async ({ value }) => {
    await api.users.resetPassword(row.id, value)
    ElMessage.success(t('users.resetSuccess', { username: row.username }))
  }).catch(() => { /* 取消 */ })
}

const deleteUser = (id: number) => {
  ElMessageBox.confirm(t('users.deleteConfirm'), t('users.tip'), { type: 'warning' }).then(async () => {
    await api.users.remove(id)
    ElMessage.success(t('users.deleteSuccess'))
    fetchUsers()
  }).catch(() => {})
}

const assignPermissions = async (user: any) => {
  permUserId.value = user.id
  permUserName.value = user.username
  permDialogVisible.value = true
  cameraPermissions.value = []
  try {
    const [camRes, permRes]: any[] = await Promise.all([
      api.cameras.list(),
      api.users.listPermissions(user.id),
    ])
    const cams = (camRes.data || camRes || []) as any[]
    const perms: any[] = permRes.data || permRes || []
    cameraPermissions.value = cams.map((cam) => ({
      camera_id: cam.id,
      camera_name: cam.name,
      permission: perms.find((p) => p.camera_id === cam.id)?.permission || 'none',
    }))
  } catch (e) { /* 拦截器已提示 */ }
}

const updateUserPermission = async (userId: number, cameraId: number, permission: string) => {
  try {
    await api.users.setPermissions(userId, cameraPermissions.value.map((p) => ({
      camera_id: p.camera_id,
      permission: p.camera_id === cameraId ? permission : p.permission,
    })))
  } catch (e) { /* 拦截器已提示 */ }
}

onMounted(() => fetchUsers())
</script>

<style scoped lang="scss">
.settings-page {}
</style>
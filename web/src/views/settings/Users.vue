<template>
  <div class="settings-page">
    <el-card :shadow="never" class="mb-16">
      <template #header>
        <div class="card-header">
          <h3>用户列表</h3>
          <el-button type="primary" @click="showAddUserDialog">
            <el-icon><UserFilled /></el-icon> 添加用户
          </el-button>
        </div>
      </template>

      <el-table :data="users" border stripe size="small" style="width: 100%">
        <el-table-column prop="username" label="用户名" width="150" />
        <el-table-column prop="email" label="邮箱" width="220" />
        <el-table-column prop="phone" label="手机" width="150" />
        <el-table-column label="角色" width="120">
          <template #default="scope">
            <el-tag :type="roleType(scope.row.role)" size="small">{{ roleLabels[scope.row.role] }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="scope">
            <el-tag :type="scope.row.status === 'active' ? 'success' : 'danger'" size="small">
              {{ scope.row.status === 'active' ? '启用' : '禁用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="last_login" label="最后登录" width="180">
          <template #default="scope">{{ formatTime(scope.row.last_login) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="240" fixed="right">
          <template #default="scope">
            <el-button-group size="small">
              <el-button link @click.stop="editUser(scope.row)">
                <el-icon><Edit /></el-icon> 编辑
              </el-button>
              <el-button link :type="scope.row.status === 'active' ? 'warning' : 'success'" @click.stop="toggleUserStatus(scope.row)">
                <el-icon><UserFilled v-if="scope.row.status === 'active'" /><User /></el-icon>
                {{ scope.row.status === 'active' ? '禁用' : '启用' }}
              </el-button>
              <el-button link @click.stop="resetPassword(scope.row)">
                <el-icon><Lock /></el-icon> 重置密码
              </el-button>
              <el-button link @click.stop="assignPermissions(scope.row)">
                <el-icon><VideoCamera /></el-icon> 权限
              </el-button>
              <el-button link type="danger" @click.stop="deleteUser(scope.row.id)" v-if="scope.row.username !== 'admin'">
                <el-icon><Delete /></el-icon> 删除
              </el-button>
            </el-button-group>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 添加/编辑用户对话框 -->
    <el-dialog v-model="userDialogVisible" :title="userDialogTitle" width="500" destroy-on-close>
      <el-form :model="userForm" :rules="userRules" ref="userFormRef" label-width="100">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="userForm.username" placeholder="请输入用户名" :disabled="editingUserId !== null" />
        </el-form-item>
        <el-form-item label="邮箱" prop="email">
          <el-input v-model="userForm.email" placeholder="请输入邮箱" />
        </el-form-item>
        <el-form-item label="手机" prop="phone">
          <el-input v-model="userForm.phone" placeholder="请输入手机号" />
        </el-form-item>
        <el-form-item label="角色" prop="role">
          <el-select v-model="userForm.role" placeholder="选择角色" style="width: 100%">
            <el-option label="管理员" value="admin" />
            <el-option label="操作员" value="operator" />
            <el-option label="只读用户" value="viewer" />
          </el-select>
        </el-form-item>
        <el-form-item label="状态" prop="status">
          <el-select v-model="userForm.status" placeholder="选择状态" style="width: 100%">
            <el-option label="启用" value="active" />
            <el-option label="禁用" value="disabled" />
          </el-select>
        </el-form-item>
        <el-form-item label="密码" prop="password" v-if="editingUserId === null">
          <el-input v-model="userForm.password" type="password" show-password placeholder="请输入密码" />
        </el-form-item>
        <el-form-item label="确认密码" prop="confirmPassword" v-if="editingUserId === null">
          <el-input v-model="userForm.confirmPassword" type="password" show-password placeholder="请再次输入密码" />
        </el-form-item>
        <el-form-item label="新密码" prop="newPassword" v-if="editingUserId !== null">
          <el-input v-model="userForm.newPassword" type="password" show-password placeholder="留空则不修改" />
        </el-form-item>
        <el-form-item label="确认新密码" prop="confirmNewPassword" v-if="editingUserId !== null">
          <el-input v-model="userForm.confirmNewPassword" type="password" show-password placeholder="请再次输入新密码" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="userDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="userSubmitLoading" @click="submitUser">保存</el-button>
      </template>
    </el-dialog>

    <!-- 权限分配对话框 -->
    <el-dialog v-model="permDialogVisible" title="分配摄像头权限" width="600" destroy-on-close>
      <p class="mb-16">为用户 <strong>{{ permUserName }}</strong> 分配摄像头访问权限</p>
      <el-table :data="cameraPermissions" border size="small" style="width: 100%">
        <el-table-column prop="camera_name" label="摄像头" width="200" />
        <el-table-column label="当前权限" width="150">
          <template #default="scope">
            <el-tag :type="permType(scope.row.permission)" size="small">{{ permLabels[scope.row.permission] }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="设置权限" width="200">
          <template #default="scope">
            <el-select v-model="scope.row.permission" placeholder="选择权限" style="width: 100%" @change="updateUserPermission(permUserId, scope.row.camera_id, scope.row.permission)">
              <el-option label="无权限" value="none" />
              <el-option label="仅查看" value="view" />
              <el-option label="查看+控制" value="control" />
              <el-option label="查看+控制+配置" value="config" />
              <el-option label="完全管理" value="admin" />
            </el-select>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { UserFilled, Edit, Lock, Delete, User, VideoCamera } from '@element-plus/icons-vue'
import { api } from '@/api'

const users = ref<any[]>([])
const userDialogVisible = ref(false)
const userDialogTitle = ref('添加用户')
const userSubmitLoading = ref(false)
const userFormRef = ref()
const editingUserId = ref<number | null>(null)

const userForm = reactive({
  username: '', email: '', phone: '', role: 'viewer', status: 'active',
  password: '', confirmPassword: '', newPassword: '', confirmNewPassword: '',
})

const userRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  email: [{ required: true, message: '请输入邮箱', trigger: 'blur' }, { type: 'email', message: '邮箱格式不正确', trigger: 'blur' }],
  role: [{ required: true, message: '请选择角色', trigger: 'change' }],
  status: [{ required: true, message: '请选择状态', trigger: 'change' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }, { min: 6, message: '密码至少6位', trigger: 'blur' }],
  confirmPassword: [{ required: true, message: '请确认密码', trigger: 'blur' }, { validator: (rule: any, value: string) => value === userForm.password ? Promise.resolve() : Promise.reject('两次密码不一致') }],
  newPassword: [{ validator: (rule: any, value: string) => value ? (value.length >= 6 ? Promise.resolve() : Promise.reject('密码至少6位')) : Promise.resolve() }],
  confirmNewPassword: [{ validator: (rule: any, value: string) => value === userForm.newPassword ? Promise.resolve() : Promise.reject('两次密码不一致') }],
}

const permDialogVisible = ref(false)
const permUserId = ref<number>(0)
const permUserName = ref('')
const cameraPermissions = ref<any[]>([])

const roleLabels = { admin: '管理员', operator: '操作员', viewer: '只读用户' }
const roleType = (r: string) => ({ admin: 'danger', operator: 'warning', viewer: 'info' }[r] || 'info')
const permLabels = { none: '无权限', view: '仅查看', control: '查看+控制', config: '查看+控制+配置', admin: '完全管理' }
const permType = (p: string) => ({ none: 'info', view: 'success', control: 'primary', config: 'warning', admin: 'danger' }[p] || 'info')

const formatTime = (time: string | null) => time ? new Date(time).toLocaleString('zh-CN') : '从未登录'

const fetchUsers = async () => {
  try {
    const res: any = await api.users.list()
    users.value = res.data || res || []
  } catch (e) { /* 拦截器已提示 */ }
}

const showAddUserDialog = () => {
  editingUserId.value = null
  userDialogTitle.value = '添加用户'
  resetUserForm()
  userDialogVisible.value = true
}

const editUser = (row: any) => {
  editingUserId.value = row.id
  userDialogTitle.value = '编辑用户'
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
      ElMessage.success('更新成功')
    } else {
      await api.users.create({
        username: userForm.username,
        password: userForm.password,
        email: userForm.email,
        phone: userForm.phone,
        role: userForm.role,
        status: userForm.status,
      })
      ElMessage.success('创建成功')
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
    ElMessage.success(`用户已${newStatus === 'active' ? '启用' : '禁用'}`)
    fetchUsers()
  } catch (e) { /* 拦截器已提示 */ }
}

const resetPassword = (row: any) => {
  ElMessageBox.prompt('请输入新密码', `重置用户 ${row.username} 的密码`, {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    inputType: 'password',
    inputPattern: /^.{6,}$/,
    inputErrorMessage: '密码至少6位',
  }).then(async ({ value }) => {
    await api.users.resetPassword(row.id, value)
    ElMessage.success(`用户 ${row.username} 密码已重置`)
  }).catch(() => { /* 取消或失败（拦截器已提示后端错误） */ })
}

const deleteUser = (id: number) => {
  ElMessageBox.confirm('确定删除该用户？删除后其登录凭证立即失效。', '提示', { type: 'warning' }).then(async () => {
    await api.users.remove(id)
    ElMessage.success('删除成功')
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
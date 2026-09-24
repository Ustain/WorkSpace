(function (AMS) {
  'use strict';
  const { ref, reactive, onMounted } = Vue;
  const { ElMessage, ElMessageBox } = ElementPlus;

  async function confirm(msg, title) {
    try {
      await ElMessageBox.confirm(msg, title || '操作确认', { type: 'warning' });
      return true;
    } catch (e) {
      return false;
    }
  }

  // ================= 用户管理 =================
  AMS.pages.users = {
    name: 'UsersPage',
    props: { user: Object },
    setup() {
      const loading = ref(false);
      const list = ref([]);
      const total = ref(0);
      const roles = ref([]);
      const depts = ref([]);
      const filters = reactive({ keyword: '', roleId: '', deptId: '', enabled: '' });
      const page = reactive({ page: 1, pageSize: 20 });

      async function load() {
        loading.value = true;
        try {
          const data = await AMS.api.get('/users', Object.assign({}, filters, page));
          list.value = data.list || [];
          total.value = data.total || 0;
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          loading.value = false;
        }
      }

      onMounted(async () => {
        try {
          roles.value = await AMS.api.get('/roles');
          depts.value = await AMS.api.get('/departments');
        } catch (e) { /* 忽略 */ }
        load();
      });

      const dlg = reactive({ show: false, editing: false, saving: false });
      const blank = () => ({
        id: null, username: '', password: '', realName: '', email: '', phone: '',
        departmentId: null, roleId: null, enabled: true,
      });
      const form = reactive(blank());

      function openCreate() { Object.assign(form, blank()); dlg.editing = false; dlg.show = true; }

      function openEdit(row) {
        Object.assign(form, {
          id: row.id, username: row.username, password: '', realName: row.realName,
          email: row.email, phone: row.phone, departmentId: row.departmentId || null,
          roleId: row.roleId, enabled: row.enabled,
        });
        dlg.editing = true;
        dlg.show = true;
      }

      async function save() {
        if (!form.realName) return ElMessage.warning('请填写真实姓名');
        if (!form.roleId) return ElMessage.warning('请选择角色');
        if (!dlg.editing && (!form.username || form.password.length < 6)) {
          return ElMessage.warning('请填写登录名，且初始密码不少于 6 位');
        }
        dlg.saving = true;
        try {
          const body = Object.assign({}, form);
          delete body.id;
          if (dlg.editing) await AMS.api.put('/users/' + form.id, body);
          else await AMS.api.post('/users', body);
          ElMessage.success(dlg.editing ? '用户信息已更新' : '用户已创建');
          dlg.show = false;
          load();
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          dlg.saving = false;
        }
      }

      async function toggle(row) {
        const next = !row.enabled;
        if (!(await confirm(`确定${next ? '启用' : '停用'}用户「${row.realName}」吗？`))) return;
        try {
          await AMS.api.put('/users/' + row.id, {
            realName: row.realName, email: row.email, phone: row.phone,
            departmentId: row.departmentId, roleId: row.roleId, enabled: next,
          });
          ElMessage.success(next ? '用户已启用' : '用户已停用');
          load();
        } catch (e) {
          ElMessage.error(e.message);
        }
      }

      async function resetPwd(row) {
        if (!(await confirm(`确定重置「${row.realName}」的密码吗？重置后为 ams@123456`))) return;
        try {
          const res = await AMS.api.post(`/users/${row.id}/reset-password`, {});
          ElMessageBox.alert(`新密码为：${res.password}，请通知用户尽快修改。`, '密码已重置');
        } catch (e) {
          ElMessage.error(e.message);
        }
      }

      async function remove(row) {
        if (!(await confirm(`确定删除用户「${row.realName}」吗？该操作不可恢复。`))) return;
        try {
          await AMS.api.del('/users/' + row.id);
          ElMessage.success('用户已删除');
          load();
        } catch (e) {
          ElMessage.error(e.message);
        }
      }

      return {
        loading, list, total, roles, depts, filters, page, load,
        dlg, form, openCreate, openEdit, save, toggle, resetPwd, remove,
        fmtDateTime: AMS.fmtDateTime,
        search() { page.page = 1; load(); },
      };
    },
    template: `
      <div>
        <div class="panel">
          <div class="filter-bar">
            <el-input v-model="filters.keyword" placeholder="登录名 / 姓名 / 邮箱 / 手机号" clearable
                      style="width:250px" @keyup.enter="search">
              <template #prefix><el-icon><Search /></el-icon></template>
            </el-input>
            <el-select v-model="filters.roleId" placeholder="角色" clearable style="width:160px" @change="search">
              <el-option v-for="r in roles" :key="r.id" :label="r.name" :value="String(r.id)" />
            </el-select>
            <el-select v-model="filters.deptId" placeholder="部门" clearable style="width:190px" @change="search">
              <el-option v-for="d in depts" :key="d.id" :label="d.name" :value="String(d.id)" />
            </el-select>
            <el-select v-model="filters.enabled" placeholder="状态" clearable style="width:130px" @change="search">
              <el-option label="已启用" value="true" />
              <el-option label="已停用" value="false" />
            </el-select>
            <el-button type="primary" @click="search">查询</el-button>
            <span style="flex:1"></span>
            <el-button type="primary" @click="openCreate"><el-icon><Plus /></el-icon>新增用户</el-button>
          </div>
        </div>

        <div class="panel">
          <el-table :data="list" v-loading="loading" size="small" border stripe>
            <el-table-column prop="username" label="登录名" width="130" />
            <el-table-column prop="realName" label="姓名" width="100" />
            <el-table-column prop="roleName" label="角色" width="120">
              <template #default="{ row }">
                <el-tag size="small" :type="row.roleCode === 'admin' ? 'danger' : (row.roleCode === 'user' ? 'info' : 'warning')">
                  {{ row.roleName }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="departmentName" label="部门" min-width="150" show-overflow-tooltip />
            <el-table-column prop="email" label="邮箱" min-width="190" show-overflow-tooltip />
            <el-table-column prop="phone" label="手机号" width="125" />
            <el-table-column label="在用设备" width="95" align="center">
              <template #default="{ row }">
                <el-tag v-if="row.holdingCount" size="small" type="primary" effect="plain">{{ row.holdingCount }} 台</el-tag>
                <span v-else class="text-muted">—</span>
              </template>
            </el-table-column>
            <el-table-column label="状态" width="80">
              <template #default="{ row }">
                <el-tag size="small" :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '已启用' : '已停用' }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="上次登录" width="145">
              <template #default="{ row }">{{ fmtDateTime(row.lastLoginAt) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="215" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" size="small" @click="openEdit(row)">编辑</el-button>
                <el-button link type="warning" size="small" @click="resetPwd(row)">重置密码</el-button>
                <el-button link :type="row.enabled ? 'info' : 'success'" size="small" @click="toggle(row)">
                  {{ row.enabled ? '停用' : '启用' }}
                </el-button>
                <el-button link type="danger" size="small" @click="remove(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>

          <div class="pager">
            <el-pagination background layout="total, prev, pager, next" :total="total"
                           v-model:current-page="page.page" :page-size="page.pageSize" @current-change="load" />
          </div>
        </div>

        <el-dialog v-model="dlg.show" :title="dlg.editing ? '编辑用户' : '新增用户'" width="540px">
          <el-form label-width="90px">
            <el-form-item label="登录名" required>
              <el-input v-model="form.username" :disabled="dlg.editing" placeholder="用于登录系统，创建后不可修改" />
            </el-form-item>
            <el-form-item v-if="!dlg.editing" label="初始密码" required>
              <el-input v-model="form.password" placeholder="至少 6 位，用户登录后可自行修改" />
            </el-form-item>
            <el-form-item label="真实姓名" required><el-input v-model="form.realName" /></el-form-item>
            <el-form-item label="邮箱">
              <el-input v-model="form.email" placeholder="用于接收设备到期提醒邮件" />
            </el-form-item>
            <el-form-item label="手机号"><el-input v-model="form.phone" /></el-form-item>
            <el-form-item label="所属部门">
              <el-select v-model="form.departmentId" style="width:100%" clearable>
                <el-option v-for="d in depts" :key="d.id" :label="d.name" :value="d.id" />
              </el-select>
            </el-form-item>
            <el-form-item label="角色" required>
              <el-select v-model="form.roleId" style="width:100%">
                <el-option v-for="r in roles" :key="r.id" :label="r.name" :value="r.id">
                  <span>{{ r.name }}</span>
                  <span class="text-muted" style="float:right;font-size:12px">{{ r.description }}</span>
                </el-option>
              </el-select>
            </el-form-item>
            <el-form-item label="账号状态">
              <el-switch v-model="form.enabled" active-text="启用" inactive-text="停用" />
            </el-form-item>
          </el-form>
          <template #footer>
            <el-button @click="dlg.show = false">取消</el-button>
            <el-button type="primary" :loading="dlg.saving" @click="save">保存</el-button>
          </template>
        </el-dialog>
      </div>`,
  };

  // ================= 角色管理 =================
  AMS.pages.roles = {
    name: 'RolesPage',
    setup() {
      const loading = ref(false);
      const list = ref([]);
      const perms = ref([]);

      async function load() {
        loading.value = true;
        try {
          list.value = await AMS.api.get('/roles');
          perms.value = await AMS.api.get('/roles/permissions');
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          loading.value = false;
        }
      }
      onMounted(load);

      // 权限点按 group 分组渲染成多个勾选块。
      function grouped() {
        const g = {};
        perms.value.forEach((p) => {
          (g[p.group] = g[p.group] || []).push(p);
        });
        return g;
      }

      const dlg = reactive({ show: false, editing: false, saving: false });
      const form = reactive({ id: null, code: '', name: '', description: '', permissions: [], isSystem: false });

      function openCreate() {
        Object.assign(form, { id: null, code: '', name: '', description: '', permissions: [], isSystem: false });
        dlg.editing = false;
        dlg.show = true;
      }

      function openEdit(row) {
        Object.assign(form, {
          id: row.id, code: row.code, name: row.name, description: row.description,
          permissions: (row.permissions || []).slice(), isSystem: row.isSystem,
        });
        dlg.editing = true;
        dlg.show = true;
      }

      async function save() {
        if (!form.name) return ElMessage.warning('请填写角色名称');
        if (!dlg.editing && !form.code) return ElMessage.warning('请填写角色编码');
        dlg.saving = true;
        try {
          const body = {
            code: form.code, name: form.name,
            description: form.description, permissions: form.permissions,
          };
          if (dlg.editing) await AMS.api.put('/roles/' + form.id, body);
          else await AMS.api.post('/roles', body);
          ElMessage.success('角色已保存');
          dlg.show = false;
          load();
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          dlg.saving = false;
        }
      }

      async function remove(row) {
        if (!(await confirm(`确定删除角色「${row.name}」吗？`))) return;
        try {
          await AMS.api.del('/roles/' + row.id);
          ElMessage.success('角色已删除');
          load();
        } catch (e) {
          ElMessage.error(e.message);
        }
      }

      function toggleGroup(items, checked) {
        items.forEach((p) => {
          const i = form.permissions.indexOf(p.code);
          if (checked && i < 0) form.permissions.push(p.code);
          if (!checked && i >= 0) form.permissions.splice(i, 1);
        });
      }

      function permName(code) {
        const p = perms.value.find((x) => x.code === code);
        return p ? p.name : code;
      }

      return { loading, list, perms, grouped, dlg, form, openCreate, openEdit, save, remove, toggleGroup, permName };
    },
    template: `
      <div>
        <div class="panel">
          <div class="panel-title">
            <span>角色与权限</span>
            <span class="sub">通过勾选权限点控制每个角色能看到哪些菜单、能执行哪些操作</span>
            <span class="spacer"></span>
            <el-button type="primary" size="small" @click="openCreate"><el-icon><Plus /></el-icon>新增角色</el-button>
          </div>

          <el-table :data="list" v-loading="loading" size="small" border>
            <el-table-column prop="name" label="角色名称" width="140">
              <template #default="{ row }">
                {{ row.name }}
                <el-tag v-if="row.isSystem" size="small" type="info" effect="plain" style="margin-left:6px">内置</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="code" label="编码" width="130" />
            <el-table-column prop="description" label="说明" min-width="230" show-overflow-tooltip />
            <el-table-column label="用户数" width="80" align="center">
              <template #default="{ row }">{{ row.userCount || 0 }}</template>
            </el-table-column>
            <el-table-column label="权限点" min-width="320">
              <template #default="{ row }">
                <el-space wrap :size="4">
                  <el-tag v-for="p in (row.permissions || []).slice(0, 8)" :key="p" size="small" effect="plain">
                    {{ permName(p) }}
                  </el-tag>
                  <el-tag v-if="(row.permissions || []).length > 8" size="small" type="info">
                    +{{ row.permissions.length - 8 }}
                  </el-tag>
                </el-space>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="120" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" size="small" @click="openEdit(row)">编辑</el-button>
                <el-button link type="danger" size="small" :disabled="row.isSystem" @click="remove(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>

        <el-dialog v-model="dlg.show" :title="dlg.editing ? '编辑角色' : '新增角色'" width="620px">
          <el-form label-width="84px">
            <el-form-item label="角色名称" required><el-input v-model="form.name" style="max-width:280px" /></el-form-item>
            <el-form-item label="角色编码" required>
              <el-input v-model="form.code" :disabled="form.isSystem" style="max-width:280px"
                        placeholder="英文标识，如 lab_manager" />
            </el-form-item>
            <el-form-item label="说明"><el-input v-model="form.description" type="textarea" :rows="2" /></el-form-item>
            <el-form-item label="权限点">
              <div style="width:100%">
                <div v-for="(items, g) in grouped()" :key="g" style="margin-bottom:12px">
                  <div style="font-size:12.5px;color:#8a93a6;margin-bottom:6px">
                    {{ g }}
                    <el-button link type="primary" size="small" @click="toggleGroup(items, true)">全选</el-button>
                    <el-button link size="small" @click="toggleGroup(items, false)">清空</el-button>
                  </div>
                  <el-checkbox-group v-model="form.permissions">
                    <el-checkbox v-for="p in items" :key="p.code" :value="p.code" :label="p.name" />
                  </el-checkbox-group>
                </div>
              </div>
            </el-form-item>
          </el-form>
          <template #footer>
            <el-button @click="dlg.show = false">取消</el-button>
            <el-button type="primary" :loading="dlg.saving" @click="save">保存</el-button>
          </template>
        </el-dialog>
      </div>`,
  };

  // ================= 部门管理 =================
  AMS.pages.departments = {
    name: 'DepartmentsPage',
    setup() {
      const loading = ref(false);
      const list = ref([]);

      async function load() {
        loading.value = true;
        try { list.value = await AMS.api.get('/departments'); }
        catch (e) { ElMessage.error(e.message); }
        finally { loading.value = false; }
      }
      onMounted(load);

      const dlg = reactive({ show: false, editing: false, saving: false });
      const form = reactive({ id: null, name: '', code: '', leader: '', remark: '' });

      function openCreate() {
        Object.assign(form, { id: null, name: '', code: '', leader: '', remark: '' });
        dlg.editing = false; dlg.show = true;
      }
      function openEdit(row) {
        Object.assign(form, { id: row.id, name: row.name, code: row.code, leader: row.leader, remark: row.remark });
        dlg.editing = true; dlg.show = true;
      }

      async function save() {
        if (!form.name) return ElMessage.warning('请填写部门名称');
        dlg.saving = true;
        try {
          const body = { name: form.name, code: form.code, leader: form.leader, remark: form.remark };
          if (dlg.editing) await AMS.api.put('/departments/' + form.id, body);
          else await AMS.api.post('/departments', body);
          ElMessage.success('部门已保存');
          dlg.show = false;
          load();
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          dlg.saving = false;
        }
      }

      async function remove(row) {
        if (!(await confirm(`确定删除部门「${row.name}」吗？`))) return;
        try {
          await AMS.api.del('/departments/' + row.id);
          ElMessage.success('部门已删除');
          load();
        } catch (e) {
          ElMessage.error(e.message);
        }
      }

      return { loading, list, dlg, form, openCreate, openEdit, save, remove };
    },
    template: `
      <div>
        <div class="panel">
          <div class="panel-title">
            <span>部门管理</span>
            <span class="sub">部门用于统计各科室的设备占用情况</span>
            <span class="spacer"></span>
            <el-button type="primary" size="small" @click="openCreate"><el-icon><Plus /></el-icon>新增部门</el-button>
          </div>
          <el-table :data="list" v-loading="loading" size="small" border stripe>
            <el-table-column prop="name" label="部门名称" min-width="180" />
            <el-table-column prop="code" label="编码" width="110" />
            <el-table-column prop="leader" label="负责人" width="110" />
            <el-table-column label="人数" width="90" align="center">
              <template #default="{ row }">{{ row.userCount || 0 }}</template>
            </el-table-column>
            <el-table-column label="在用设备" width="100" align="center">
              <template #default="{ row }">
                <el-tag v-if="row.assetCount" size="small" type="primary" effect="plain">{{ row.assetCount }} 台</el-tag>
                <span v-else class="text-muted">—</span>
              </template>
            </el-table-column>
            <el-table-column prop="remark" label="备注" min-width="230" show-overflow-tooltip />
            <el-table-column label="操作" width="120" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" size="small" @click="openEdit(row)">编辑</el-button>
                <el-button link type="danger" size="small" @click="remove(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>

        <el-dialog v-model="dlg.show" :title="dlg.editing ? '编辑部门' : '新增部门'" width="480px">
          <el-form label-width="84px">
            <el-form-item label="部门名称" required><el-input v-model="form.name" /></el-form-item>
            <el-form-item label="编码"><el-input v-model="form.code" placeholder="如 HPC" /></el-form-item>
            <el-form-item label="负责人"><el-input v-model="form.leader" /></el-form-item>
            <el-form-item label="备注"><el-input v-model="form.remark" type="textarea" :rows="2" /></el-form-item>
          </el-form>
          <template #footer>
            <el-button @click="dlg.show = false">取消</el-button>
            <el-button type="primary" :loading="dlg.saving" @click="save">保存</el-button>
          </template>
        </el-dialog>
      </div>`,
  };

  // ================= 设备类型管理 =================
  AMS.pages.categories = {
    name: 'CategoriesPage',
    setup() {
      const loading = ref(false);
      const list = ref([]);

      async function load() {
        loading.value = true;
        try { list.value = await AMS.api.get('/categories'); }
        catch (e) { ElMessage.error(e.message); }
        finally { loading.value = false; }
      }
      onMounted(load);

      const dlg = reactive({ show: false, editing: false, saving: false });
      const form = reactive({ id: null, name: '', code: '', defaultLoanDays: 180, sort: 0 });

      function openCreate() {
        Object.assign(form, { id: null, name: '', code: '', defaultLoanDays: 180, sort: list.value.length + 1 });
        dlg.editing = false; dlg.show = true;
      }
      function openEdit(row) {
        Object.assign(form, {
          id: row.id, name: row.name, code: row.code,
          defaultLoanDays: row.defaultLoanDays, sort: row.sort,
        });
        dlg.editing = true; dlg.show = true;
      }

      async function save() {
        if (!form.name) return ElMessage.warning('请填写类型名称');
        dlg.saving = true;
        try {
          const body = {
            name: form.name, code: form.code,
            defaultLoanDays: Number(form.defaultLoanDays), sort: Number(form.sort),
          };
          if (dlg.editing) await AMS.api.put('/categories/' + form.id, body);
          else await AMS.api.post('/categories', body);
          ElMessage.success('类型已保存');
          dlg.show = false;
          load();
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          dlg.saving = false;
        }
      }

      async function remove(row) {
        if (!(await confirm(`确定删除设备类型「${row.name}」吗？`))) return;
        try {
          await AMS.api.del('/categories/' + row.id);
          ElMessage.success('类型已删除');
          load();
        } catch (e) {
          ElMessage.error(e.message);
        }
      }

      return { loading, list, dlg, form, openCreate, openEdit, save, remove };
    },
    template: `
      <div>
        <div class="panel">
          <div class="panel-title">
            <span>设备类型</span>
            <span class="sub">类型编码用于生成资产编号（如 NB-2026-0001）；默认借用天数会作为申请表单的初始值</span>
            <span class="spacer"></span>
            <el-button type="primary" size="small" @click="openCreate"><el-icon><Plus /></el-icon>新增类型</el-button>
          </div>
          <el-table :data="list" v-loading="loading" size="small" border stripe>
            <el-table-column prop="sort" label="排序" width="70" align="center" />
            <el-table-column prop="name" label="类型名称" min-width="160" />
            <el-table-column prop="code" label="编码" width="100" />
            <el-table-column label="默认借用天数" width="130" align="center">
              <template #default="{ row }">{{ row.defaultLoanDays }} 天</template>
            </el-table-column>
            <el-table-column label="设备数量" width="110" align="center">
              <template #default="{ row }">
                <el-tag v-if="row.assetCount" size="small" effect="plain">{{ row.assetCount }} 台</el-tag>
                <span v-else class="text-muted">—</span>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="120" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" size="small" @click="openEdit(row)">编辑</el-button>
                <el-button link type="danger" size="small" @click="remove(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>

        <el-dialog v-model="dlg.show" :title="dlg.editing ? '编辑类型' : '新增类型'" width="460px">
          <el-form label-width="106px">
            <el-form-item label="类型名称" required><el-input v-model="form.name" placeholder="如 笔记本电脑" /></el-form-item>
            <el-form-item label="类型编码"><el-input v-model="form.code" placeholder="如 NB，用于资产编号前缀" /></el-form-item>
            <el-form-item label="默认借用天数">
              <el-input-number v-model="form.defaultLoanDays" :min="1" :max="1095" style="width:100%" />
            </el-form-item>
            <el-form-item label="排序">
              <el-input-number v-model="form.sort" :min="0" style="width:100%" />
            </el-form-item>
          </el-form>
          <template #footer>
            <el-button @click="dlg.show = false">取消</el-button>
            <el-button type="primary" :loading="dlg.saving" @click="save">保存</el-button>
          </template>
        </el-dialog>
      </div>`,
  };
})(window.AMS);

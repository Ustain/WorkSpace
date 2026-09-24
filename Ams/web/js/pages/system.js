(function (AMS) {
  'use strict';
  const { ref, reactive, onMounted } = Vue;
  const { ElMessage, ElMessageBox } = ElementPlus;

  // ================= 系统设置 =================
  AMS.pages.settings = {
    name: 'SettingsPage',
    setup() {
      const loading = ref(false);
      const saving = ref(false);
      const running = ref(false);
      const info = ref({});
      const form = reactive({
        system_name: '', org_name: '', remind_before_days: 7,
        max_loan_days: 365, email_notify: true,
      });
      const lastRun = ref(null);

      async function load() {
        loading.value = true;
        try {
          const data = await AMS.api.get('/settings');
          info.value = data;
          const v = data.values || {};
          Object.assign(form, {
            system_name: v.system_name || '',
            org_name: v.org_name || '',
            remind_before_days: Number(v.remind_before_days || 7),
            max_loan_days: Number(v.max_loan_days || 365),
            email_notify: v.email_notify !== 'false',
          });
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          loading.value = false;
        }
      }
      onMounted(load);

      async function save() {
        saving.value = true;
        try {
          await AMS.api.put('/settings', {
            system_name: form.system_name,
            org_name: form.org_name,
            remind_before_days: String(form.remind_before_days),
            max_loan_days: String(form.max_loan_days),
            email_notify: form.email_notify ? 'true' : 'false',
          });
          ElMessage.success('系统设置已保存，提醒策略立即生效');
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          saving.value = false;
        }
      }

      // 演示时不必等定时器，手动触发一次扫描立刻产生提醒。
      async function runReminder() {
        running.value = true;
        try {
          const res = await AMS.api.post('/jobs/reminder/run', {});
          lastRun.value = res.result;
          ElMessageBox.alert(
            `本次扫描：临期 ${res.result.dueSoon} 台，逾期 ${res.result.overdue} 台，` +
            `新推送提醒 ${res.result.notified} 条。\n\n` +
            `临期与逾期通知已发给设备领用人（站内信 + 邮件），管理员收到的是汇总消息。`,
            '到期扫描完成');
          window.dispatchEvent(new CustomEvent('ams:refresh-badge'));
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          running.value = false;
        }
      }

      return { loading, saving, running, info, form, lastRun, save, runReminder };
    },
    template: `
      <div style="max-width:860px" v-loading="loading">
        <div class="panel">
          <div class="panel-title">基础信息</div>
          <el-form label-width="130px">
            <el-form-item label="系统名称">
              <el-input v-model="form.system_name" style="max-width:380px" />
            </el-form-item>
            <el-form-item label="单位名称">
              <el-input v-model="form.org_name" style="max-width:380px" />
            </el-form-item>
          </el-form>
        </div>

        <div class="panel">
          <div class="panel-title">
            <span>领用与提醒策略</span>
            <span class="sub">影响到期提醒的触发时机与申请表单校验</span>
          </div>
          <el-form label-width="130px">
            <el-form-item label="提前提醒天数">
              <el-input-number v-model="form.remind_before_days" :min="1" :max="60" style="width:180px" />
              <span class="text-muted" style="margin-left:12px;font-size:12.5px">
                设备到期前 {{ form.remind_before_days }} 天开始推送提醒
              </span>
            </el-form-item>
            <el-form-item label="单次最长借用">
              <el-input-number v-model="form.max_loan_days" :min="1" :max="1095" style="width:180px" />
              <span class="text-muted" style="margin-left:12px;font-size:12.5px">超过该天数的领用申请会被拒绝提交</span>
            </el-form-item>
            <el-form-item label="邮件通知">
              <el-switch v-model="form.email_notify" active-text="开启" inactive-text="关闭" />
              <span class="text-muted" style="margin-left:12px;font-size:12.5px">
                开启后普通用户收到站内信的同时会额外收到邮件；管理员仅收站内信
              </span>
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="saving" @click="save">保存设置</el-button>
            </el-form-item>
          </el-form>
        </div>

        <div class="panel">
          <div class="panel-title">
            <span>到期提醒任务</span>
            <span class="sub">后台每 {{ info.reminderInterval }} 自动扫描一次</span>
          </div>
          <el-alert type="info" :closable="false" show-icon style="margin-bottom:14px"
                    title="提醒规则：临期与逾期两类事件对每台设备各只推送一次；设备归还或续借后标记重置，下个借用周期会重新提醒。" />
          <el-button type="primary" :loading="running" @click="runReminder">
            <el-icon><Refresh /></el-icon>立即执行一次扫描
          </el-button>
          <el-descriptions v-if="lastRun" :column="3" border size="small" style="margin-top:14px">
            <el-descriptions-item label="临期设备">{{ lastRun.dueSoon }} 台</el-descriptions-item>
            <el-descriptions-item label="逾期设备">{{ lastRun.overdue }} 台</el-descriptions-item>
            <el-descriptions-item label="本次推送">{{ lastRun.notified }} 条</el-descriptions-item>
          </el-descriptions>
        </div>

        <div class="panel">
          <div class="panel-title">邮件通道</div>
          <el-alert v-if="info.smtpEnabled" type="success" :closable="false" show-icon
                    :title="'已配置 SMTP（' + info.smtpHost + '），邮件将真实投递。'" />
          <el-alert v-else type="warning" :closable="false" show-icon
                    title="未配置 SMTP，邮件进入模拟投递模式：不会真正外发，但内容完整落库，可在「邮件记录」中查看。" />
          <div class="text-muted" style="font-size:12.5px;margin-top:12px;line-height:1.9">
            如需真实发信，启动服务前设置环境变量：<br>
            <code>AMS_SMTP_HOST</code> / <code>AMS_SMTP_PORT</code> /
            <code>AMS_SMTP_USER</code> / <code>AMS_SMTP_PASS</code> / <code>AMS_SMTP_FROM</code>
          </div>
        </div>
      </div>`,
  };

  // ================= 审计日志 =================
  AMS.pages.audit = {
    name: 'AuditPage',
    setup() {
      const loading = ref(false);
      const list = ref([]);
      const total = ref(0);
      const filters = reactive({ username: '', module: '', range: [] });
      const page = reactive({ page: 1, pageSize: 20 });

      async function load() {
        loading.value = true;
        try {
          const p = {
            username: filters.username, module: filters.module,
            page: page.page, pageSize: page.pageSize,
          };
          AMS.applyRange(p, filters.range, 'from', 'to');
          const data = await AMS.api.get('/audit-logs', p);
          list.value = data.list || [];
          total.value = data.total || 0;
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          loading.value = false;
        }
      }
      onMounted(load);

      const modules = [
        { value: '', label: '全部模块' },
        { value: 'assets', label: '设备' },
        { value: 'applications', label: '申请审批' },
        { value: 'repairs', label: '报修' },
        { value: 'users', label: '用户' },
        { value: 'roles', label: '角色' },
        { value: 'departments', label: '部门' },
        { value: 'categories', label: '设备类型' },
        { value: 'settings', label: '系统设置' },
        { value: 'auth', label: '登录' },
      ];

      return {
        loading, list, total, filters, page, load, modules,
        fmtDateTime: AMS.fmtDateTime,
        search() { page.page = 1; load(); },
      };
    },
    template: `
      <div>
        <div class="panel">
          <div class="filter-bar">
            <el-input v-model="filters.username" placeholder="操作人登录名" clearable style="width:190px" @keyup.enter="search" />
            <el-select v-model="filters.module" placeholder="模块" clearable style="width:160px" @change="search">
              <el-option v-for="m in modules" :key="m.value" :label="m.label" :value="m.value" />
            </el-select>
            <el-date-picker v-model="filters.range" type="daterange" value-format="YYYY-MM-DD"
                            start-placeholder="开始日期" end-placeholder="结束日期" range-separator="~" style="width:250px" />
            <el-button type="primary" @click="search"><el-icon><Search /></el-icon>查询</el-button>
          </div>
        </div>

        <div class="panel">
          <div class="panel-title">
            <span>操作审计</span>
            <span class="sub">记录所有写操作的执行人、时间、入参与来源 IP；密码类字段已脱敏</span>
          </div>
          <el-table :data="list" v-loading="loading" size="small" border stripe>
            <el-table-column label="时间" width="150">
              <template #default="{ row }">{{ fmtDateTime(row.createdAt) }}</template>
            </el-table-column>
            <el-table-column prop="username" label="操作人" width="120" />
            <el-table-column prop="module" label="模块" width="120" />
            <el-table-column prop="action" label="动作" width="140" />
            <el-table-column prop="method" label="方法" width="76" />
            <el-table-column prop="path" label="接口路径" min-width="190" show-overflow-tooltip />
            <el-table-column prop="detail" label="请求内容" min-width="270" show-overflow-tooltip />
            <el-table-column prop="ip" label="来源 IP" width="130" />
            <template #empty><empty-tip text="暂无审计记录" /></template>
          </el-table>

          <div class="pager">
            <el-pagination background layout="total, prev, pager, next" :total="total"
                           v-model:current-page="page.page" :page-size="page.pageSize" @current-change="load" />
          </div>
        </div>
      </div>`,
  };

  // ================= 邮件记录 =================
  AMS.pages.emails = {
    name: 'EmailsPage',
    setup() {
      const loading = ref(false);
      const list = ref([]);
      const total = ref(0);
      const filters = reactive({ to: '' });
      const page = reactive({ page: 1, pageSize: 20 });
      const preview = reactive({ show: false, row: null });

      async function load() {
        loading.value = true;
        try {
          const data = await AMS.api.get('/emails', { to: filters.to, page: page.page, pageSize: page.pageSize });
          list.value = data.list || [];
          total.value = data.total || 0;
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          loading.value = false;
        }
      }
      onMounted(load);

      const statusMap = {
        sent: { label: '已发送', type: 'success' },
        mocked: { label: '模拟投递', type: 'info' },
        failed: { label: '发送失败', type: 'danger' },
      };

      return {
        loading, list, total, filters, page, load, preview, statusMap,
        fmtDateTime: AMS.fmtDateTime, pick: AMS.pick,
        search() { page.page = 1; load(); },
        open(row) { preview.row = row; preview.show = true; },
      };
    },
    template: `
      <div>
        <div class="panel">
          <div class="filter-bar">
            <el-input v-model="filters.to" placeholder="收件邮箱" clearable style="width:240px" @keyup.enter="search" />
            <el-button type="primary" @click="search"><el-icon><Search /></el-icon>查询</el-button>
            <span style="flex:1"></span>
            <span class="text-muted" style="font-size:12.5px">
              未配置 SMTP 时邮件为「模拟投递」，内容仍完整记录在此，可点击查看渲染效果
            </span>
          </div>
        </div>

        <div class="panel">
          <el-table :data="list" v-loading="loading" size="small" border stripe>
            <el-table-column label="发送时间" width="150">
              <template #default="{ row }">{{ fmtDateTime(row.createdAt) }}</template>
            </el-table-column>
            <el-table-column prop="to" label="收件人" min-width="210" />
            <el-table-column prop="subject" label="主题" min-width="260" show-overflow-tooltip />
            <el-table-column label="状态" width="110">
              <template #default="{ row }">
                <el-tag size="small" :type="pick(statusMap, row.status).type">{{ pick(statusMap, row.status).label }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="error" label="错误信息" min-width="160" show-overflow-tooltip />
            <el-table-column label="操作" width="90" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" size="small" @click="open(row)">查看正文</el-button>
              </template>
            </el-table-column>
            <template #empty><empty-tip text="暂无邮件记录。可在「系统设置」中手动触发一次到期扫描来生成提醒邮件" /></template>
          </el-table>

          <div class="pager">
            <el-pagination background layout="total, prev, pager, next" :total="total"
                           v-model:current-page="page.page" :page-size="page.pageSize" @current-change="load" />
          </div>
        </div>

        <el-dialog v-model="preview.show" title="邮件正文预览" width="700px">
          <template v-if="preview.row">
            <el-descriptions :column="1" border size="small" style="margin-bottom:14px">
              <el-descriptions-item label="收件人">{{ preview.row.to }}</el-descriptions-item>
              <el-descriptions-item label="主题">{{ preview.row.subject }}</el-descriptions-item>
              <el-descriptions-item label="时间">{{ fmtDateTime(preview.row.createdAt) }}</el-descriptions-item>
            </el-descriptions>
            <div class="mail-preview" v-html="preview.row.body"></div>
          </template>
        </el-dialog>
      </div>`,
  };
})(window.AMS);

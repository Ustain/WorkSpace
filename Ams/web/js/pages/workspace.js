(function (AMS) {
  'use strict';
  const { ref, reactive, computed, onMounted } = Vue;
  const { ElMessage, ElMessageBox } = ElementPlus;

  // ================= 我的工作台（普通用户首页） =================
  AMS.pages.home = {
    name: 'HomePage',
    props: { user: Object },
    setup() {
      const loading = ref(true);
      const st = ref({});

      async function load() {
        loading.value = true;
        try { st.value = await AMS.api.get('/stats/my'); }
        catch (e) { ElMessage.error(e.message); }
        finally { loading.value = false; }
      }
      onMounted(load);

      const go = (h) => { window.location.hash = h; };

      const pieOption = computed(() => AMS.chartOption({
        title: { text: '我持有的设备类型', left: 'center', top: 6, textStyle: { fontSize: 13.5, color: '#5a6376' } },
        tooltip: { trigger: 'item', formatter: '{b}: {c} 台 ({d}%)' },
        legend: { bottom: 0, itemWidth: 9, itemHeight: 9, textStyle: { fontSize: 11.5 } },
        series: [{
          type: 'pie', radius: ['40%', '64%'], center: ['50%', '50%'],
          itemStyle: { borderRadius: 5, borderColor: '#fff', borderWidth: 2 },
          label: { show: false }, labelLine: { show: false },
          data: st.value.byCategory || [],
        }],
      }));

      return {
        loading, st, go, pieOption,
        moneyShort: AMS.moneyShort, fmtDate: AMS.fmtDate,
      };
    },
    template: `
      <div v-loading="loading">
        <div class="stat-grid">
          <stat-card label="我领用的设备" :value="st.myAssets || 0" unit="台" icon="💼" tone="primary"
                     :foot="'资产价值 ' + moneyShort(st.myValue) + ' 元'" @click="go('#/my-assets')" />
          <stat-card label="即将到期" :value="st.dueSoon || 0" unit="台" icon="⏰" tone="warning"
                     :foot="(st.remindDays || 7) + ' 天内需归还或续借'" @click="go('#/my-assets')" />
          <stat-card label="已逾期" :value="st.overdue || 0" unit="台" icon="🚨" tone="danger"
                     foot="请尽快归还" @click="go('#/my-assets')" />
          <stat-card label="审批中的申请" :value="st.pending || 0" unit="单" icon="📝" tone="info"
                     :foot="'历史通过 ' + (st.approved || 0) + ' 单'" @click="go('#/my-applications')" />
          <stat-card label="进行中的报修" :value="st.myRepair || 0" unit="单" icon="🔧" tone="warning"
                     foot="等待管理员处理" @click="go('#/my-repairs')" />
          <stat-card label="未读消息" :value="st.unread || 0" unit="条" icon="🔔" tone="primary"
                     foot="到期提醒会在这里推送" @click="go('#/notifications')" />
        </div>

        <div class="chart-grid">
          <div class="panel mb0">
            <div class="panel-title">
              <span>最近需要归还的设备</span>
              <span class="spacer"></span>
              <el-button link type="primary" @click="go('#/my-assets')">查看全部</el-button>
            </div>
            <empty-tip v-if="!st.upcoming || !st.upcoming.length" text="您当前没有在用设备，可前往「可领用设备」发起申请" />
            <el-table v-else :data="st.upcoming" size="small">
              <el-table-column label="设备" min-width="180" show-overflow-tooltip>
                <template #default="{ row }">
                  <div>{{ row.name }}</div>
                  <div class="text-muted" style="font-size:11.5px">{{ row.assetNo }}</div>
                </template>
              </el-table-column>
              <el-table-column label="类型" width="100">
                <template #default="{ row }">{{ row.category ? row.category.name : '—' }}</template>
              </el-table-column>
              <el-table-column label="应归还" width="190">
                <template #default="{ row }"><due-tag :due-at="row.dueAt" :status="row.status" /></template>
              </el-table-column>
            </el-table>
          </div>

          <div class="panel mb0">
            <e-chart v-if="st.byCategory && st.byCategory.length" :option="pieOption" />
            <empty-tip v-else text="暂无持有设备" />
          </div>
        </div>

        <div class="panel">
          <div class="panel-title">快捷操作</div>
          <el-space wrap :size="12">
            <el-button type="primary" @click="go('#/available')"><el-icon><Plus /></el-icon>申请领用设备</el-button>
            <el-button @click="go('#/my-assets')"><el-icon><Refresh /></el-icon>归还 / 续借</el-button>
            <el-button @click="go('#/my-assets')"><el-icon><Tools /></el-icon>设备报修</el-button>
            <el-button @click="go('#/my-applications')"><el-icon><Document /></el-icon>查看申请进度</el-button>
            <el-button @click="go('#/notifications')"><el-icon><Bell /></el-icon>消息中心</el-button>
          </el-space>
        </div>
      </div>`,
  };

  // ================= 可领用设备 =================
  AMS.pages.available = {
    name: 'AvailablePage',
    props: { user: Object },
    setup() {
      const loading = ref(false);
      const list = ref([]);
      const total = ref(0);
      const cats = ref([]);
      const filters = reactive({ keyword: '', categoryId: [] });
      const page = reactive({ page: 1, pageSize: 12 });
      const view = ref('card');

      async function load() {
        loading.value = true;
        try {
          const data = await AMS.api.get('/assets', {
            quick: 'available', keyword: filters.keyword, categoryId: filters.categoryId,
            page: page.page, pageSize: page.pageSize, sortBy: 'name', sortOrder: 'asc',
          });
          list.value = data.list || [];
          total.value = data.total || 0;
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          loading.value = false;
        }
      }

      onMounted(async () => {
        try { cats.value = (await AMS.api.get('/assets/options')).categories || []; } catch (e) { /* 忽略 */ }
        load();
      });

      const drawer = reactive({ show: false, id: null });
      const apply = reactive({ show: false, asset: null, reason: '', loanDays: 180, saving: false });

      const reasonPresets = [
        '新入职配备办公设备', '课题组数据处理需要', '学术汇报 / 会议使用',
        '原设备故障返修，临时替换', '外出参会携带使用', '多屏调试需要额外显示器',
      ];

      function openApply(row) {
        const cat = cats.value.find((c) => c.id === row.categoryId);
        Object.assign(apply, {
          show: true, asset: row, reason: '', saving: false,
          // 借用天数按设备类型给默认值：转接头 90 天、笔记本 365 天。
          loanDays: (cat && cat.defaultLoanDays) || 180,
        });
      }

      async function submitApply() {
        if (!apply.reason.trim()) return ElMessage.warning('请填写申请事由');
        apply.saving = true;
        try {
          const res = await AMS.api.post('/applications', {
            type: 'borrow', assetId: apply.asset.id,
            reason: apply.reason, loanDays: Number(apply.loanDays),
          });
          ElMessage.success(res.message);
          apply.show = false;
          load();
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          apply.saving = false;
        }
      }

      return {
        loading, list, total, cats, filters, page, view, load, drawer, apply,
        openApply, submitApply, reasonPresets,
        money: AMS.money, fmtDate: AMS.fmtDate,
        search() { page.page = 1; load(); },
        openDetail(row) { drawer.id = row.id; drawer.show = true; },
      };
    },
    template: `
      <div>
        <div class="panel">
          <div class="filter-bar">
            <el-input v-model="filters.keyword" placeholder="搜索设备名称 / 品牌 / 型号" clearable
                      style="width:260px" @keyup.enter="search">
              <template #prefix><el-icon><Search /></el-icon></template>
            </el-input>
            <el-select v-model="filters.categoryId" multiple collapse-tags placeholder="设备类型"
                       clearable style="width:240px" @change="search">
              <el-option v-for="c in cats" :key="c.id" :label="c.name" :value="String(c.id)" />
            </el-select>
            <el-button type="primary" @click="search">查询</el-button>
            <span style="flex:1"></span>
            <el-radio-group v-model="view" size="small">
              <el-radio-button value="card">卡片</el-radio-button>
              <el-radio-button value="table">列表</el-radio-button>
            </el-radio-group>
          </div>
        </div>

        <div class="panel" v-loading="loading">
          <div class="panel-title">
            <span>当前可领用设备</span>
            <span class="sub">共 {{ total }} 台在库设备可申请</span>
          </div>

          <empty-tip v-if="!list.length" text="暂无可领用设备，请稍后再来或联系资产管理员" />

          <div v-else-if="view === 'card'" class="asset-card-grid">
            <div v-for="a in list" :key="a.id" class="asset-card">
              <div class="name">{{ a.name }}</div>
              <div class="meta">
                {{ a.category ? a.category.name : '' }} · {{ a.brand }} {{ a.model }}<br>
                <span v-if="a.specification">{{ a.specification }}<br></span>
                编号 {{ a.assetNo }}<br>
                存放于 {{ a.location || '未标注' }}
              </div>
              <div class="foot">
                <span class="price">{{ money(a.price) }}</span>
                <span style="flex:1"></span>
                <el-button size="small" @click="openDetail(a)">详情</el-button>
                <el-button size="small" type="primary" @click="openApply(a)">申请领用</el-button>
              </div>
            </div>
          </div>

          <el-table v-else :data="list" size="small" border stripe>
            <el-table-column prop="assetNo" label="资产编号" width="132" />
            <el-table-column prop="name" label="设备名称" min-width="180" show-overflow-tooltip />
            <el-table-column label="类型" width="105">
              <template #default="{ row }">{{ row.category ? row.category.name : '—' }}</template>
            </el-table-column>
            <el-table-column label="品牌型号" min-width="150" show-overflow-tooltip>
              <template #default="{ row }">{{ row.brand }} {{ row.model }}</template>
            </el-table-column>
            <el-table-column prop="specification" label="规格配置" min-width="200" show-overflow-tooltip />
            <el-table-column prop="location" label="存放位置" width="140" show-overflow-tooltip />
            <el-table-column label="操作" width="150" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" size="small" @click="openDetail(row)">详情</el-button>
                <el-button link type="primary" size="small" @click="openApply(row)">申请领用</el-button>
              </template>
            </el-table-column>
          </el-table>

          <div class="pager">
            <el-pagination background layout="total, sizes, prev, pager, next" :total="total"
                           v-model:current-page="page.page" v-model:page-size="page.pageSize"
                           :page-sizes="[12, 24, 48]" @current-change="load" @size-change="load" />
          </div>
        </div>

        <asset-drawer v-model="drawer.show" :asset-id="drawer.id" />

        <el-dialog v-model="apply.show" title="提交领用申请" width="520px">
          <template v-if="apply.asset">
            <el-descriptions :column="2" border size="small" style="margin-bottom:14px">
              <el-descriptions-item label="设备名称">{{ apply.asset.name }}</el-descriptions-item>
              <el-descriptions-item label="资产编号">{{ apply.asset.assetNo }}</el-descriptions-item>
              <el-descriptions-item label="品牌型号">{{ apply.asset.brand }} {{ apply.asset.model }}</el-descriptions-item>
              <el-descriptions-item label="存放位置">{{ apply.asset.location || '—' }}</el-descriptions-item>
              <el-descriptions-item label="规格配置" :span="2">{{ apply.asset.specification || '—' }}</el-descriptions-item>
            </el-descriptions>

            <el-form label-width="84px">
              <el-form-item label="借用天数" required>
                <el-input-number v-model="apply.loanDays" :min="1" :max="365" style="width:170px" />
                <span class="text-muted" style="margin-left:10px;font-size:12.5px">
                  预计归还 {{ fmtDate(new Date(Date.now() + apply.loanDays * 86400000)) }}
                </span>
              </el-form-item>
              <el-form-item label="申请事由" required>
                <el-input v-model="apply.reason" type="textarea" :rows="3" placeholder="请说明用途，便于管理员审批" />
              </el-form-item>
              <el-form-item label="常用事由">
                <el-space wrap>
                  <el-tag v-for="r in reasonPresets" :key="r" size="small" effect="plain"
                          style="cursor:pointer" @click="apply.reason = r">{{ r }}</el-tag>
                </el-space>
              </el-form-item>
            </el-form>
            <el-alert type="info" :closable="false" show-icon
                      title="提交后管理员会收到待办提醒。审批结果将通过站内信与邮件通知您，到期前系统也会自动提醒归还。" />
          </template>
          <template #footer>
            <el-button @click="apply.show = false">取消</el-button>
            <el-button type="primary" :loading="apply.saving" @click="submitApply">提交申请</el-button>
          </template>
        </el-dialog>
      </div>`,
  };

  // ================= 我的设备 =================
  AMS.pages['my-assets'] = {
    name: 'MyAssetsPage',
    props: { user: Object },
    setup() {
      const loading = ref(false);
      const list = ref([]);
      const total = ref(0);
      const page = reactive({ page: 1, pageSize: 20 });

      async function load() {
        loading.value = true;
        try {
          const data = await AMS.api.get('/assets', {
            mine: 1, status: 'in_use', sortBy: 'dueAt', sortOrder: 'asc',
            page: page.page, pageSize: page.pageSize,
          });
          list.value = data.list || [];
          total.value = data.total || 0;
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          loading.value = false;
        }
      }
      onMounted(load);

      const drawer = reactive({ show: false, id: null });

      // 归还 / 续借共用一个弹窗，只是提交的 type 不同。
      const dlg = reactive({ show: false, type: 'return', asset: null, reason: '', loanDays: 90, saving: false });

      function open(row, type) {
        Object.assign(dlg, {
          show: true, type, asset: row, saving: false, loanDays: 90,
          reason: type === 'return' ? '工作调整，设备不再需要，申请归还。' : '',
        });
      }

      async function submit() {
        if (!dlg.reason.trim()) return ElMessage.warning('请填写申请说明');
        dlg.saving = true;
        try {
          const res = await AMS.api.post('/applications', {
            type: dlg.type, assetId: dlg.asset.id,
            reason: dlg.reason, loanDays: dlg.type === 'renew' ? Number(dlg.loanDays) : 0,
          });
          ElMessage.success(res.message);
          dlg.show = false;
          load();
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          dlg.saving = false;
        }
      }

      const repair = reactive({ show: false, asset: null, fault: '', saving: false });
      const faultPresets = [
        '无法开机，电源指示灯不亮', '屏幕出现竖条纹 / 闪烁', '键盘部分按键失灵',
        '风扇异响且温度明显偏高', '电池续航骤降', 'USB 接口接触不良',
      ];

      function openRepair(row) {
        Object.assign(repair, { show: true, asset: row, fault: '', saving: false });
      }

      async function submitRepair() {
        if (!repair.fault.trim()) return ElMessage.warning('请描述故障现象');
        repair.saving = true;
        try {
          const res = await AMS.api.post('/repairs', { assetId: repair.asset.id, fault: repair.fault });
          ElMessage.success(res.message);
          repair.show = false;
          load();
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          repair.saving = false;
        }
      }

      return {
        loading, list, total, page, load, drawer, dlg, repair, faultPresets,
        open, submit, openRepair, submitRepair,
        fmtDate: AMS.fmtDate, money: AMS.money,
        openDetail(row) { drawer.id = row.id; drawer.show = true; },
      };
    },
    template: `
      <div>
        <div class="panel">
          <div class="panel-title">
            <span>我当前领用的设备</span>
            <span class="sub">共 {{ total }} 台，按到期时间从近到远排列</span>
            <span class="spacer"></span>
            <el-button size="small" @click="load"><el-icon><Refresh /></el-icon>刷新</el-button>
          </div>

          <el-table :data="list" v-loading="loading" size="small" border stripe>
            <el-table-column prop="assetNo" label="资产编号" width="132" />
            <el-table-column prop="name" label="设备名称" min-width="180" show-overflow-tooltip>
              <template #default="{ row }">
                <el-link type="primary" :underline="false" @click="openDetail(row)">{{ row.name }}</el-link>
              </template>
            </el-table-column>
            <el-table-column label="类型" width="105">
              <template #default="{ row }">{{ row.category ? row.category.name : '—' }}</template>
            </el-table-column>
            <el-table-column label="品牌型号" min-width="145" show-overflow-tooltip>
              <template #default="{ row }">{{ row.brand }} {{ row.model }}</template>
            </el-table-column>
            <el-table-column label="领用时间" width="110">
              <template #default="{ row }">{{ fmtDate(row.borrowedAt) }}</template>
            </el-table-column>
            <el-table-column label="应归还" width="195">
              <template #default="{ row }"><due-tag :due-at="row.dueAt" :status="row.status" /></template>
            </el-table-column>
            <el-table-column prop="location" label="存放位置" width="135" show-overflow-tooltip />
            <el-table-column label="操作" width="190" fixed="right">
              <template #default="{ row }">
                <el-button link type="success" size="small" @click="open(row, 'return')">申请归还</el-button>
                <el-button link type="warning" size="small" @click="open(row, 'renew')">续借</el-button>
                <el-button link type="danger" size="small" @click="openRepair(row)">报修</el-button>
              </template>
            </el-table-column>
            <template #empty><empty-tip text="您当前没有领用中的设备" /></template>
          </el-table>

          <div class="pager">
            <el-pagination background layout="total, prev, pager, next" :total="total"
                           v-model:current-page="page.page" :page-size="page.pageSize" @current-change="load" />
          </div>
        </div>

        <asset-drawer v-model="drawer.show" :asset-id="drawer.id" />

        <el-dialog v-model="dlg.show" :title="dlg.type === 'return' ? '申请归还设备' : '申请续借设备'" width="500px">
          <template v-if="dlg.asset">
            <el-descriptions :column="1" border size="small" style="margin-bottom:14px">
              <el-descriptions-item label="设备">{{ dlg.asset.name }}（{{ dlg.asset.assetNo }}）</el-descriptions-item>
              <el-descriptions-item label="当前到期">{{ fmtDate(dlg.asset.dueAt) }}</el-descriptions-item>
            </el-descriptions>
            <el-form label-width="84px">
              <el-form-item v-if="dlg.type === 'renew'" label="续借天数" required>
                <el-input-number v-model="dlg.loanDays" :min="1" :max="365" style="width:170px" />
                <span class="text-muted" style="margin-left:10px;font-size:12.5px">在原到期日基础上顺延</span>
              </el-form-item>
              <el-form-item :label="dlg.type === 'return' ? '归还说明' : '续借事由'" required>
                <el-input v-model="dlg.reason" type="textarea" :rows="3"
                          :placeholder="dlg.type === 'return' ? '如：工作调整，设备不再需要' : '如：课题尚未结项，需继续使用'" />
              </el-form-item>
            </el-form>
            <el-alert type="info" :closable="false" show-icon
                      :title="dlg.type === 'return'
                        ? '归还申请经管理员核验设备后生效，通过后设备将回到可领用状态。'
                        : '续借通过后到期时间会在原到期日基础上顺延，系统会按新日期重新提醒。'" />
          </template>
          <template #footer>
            <el-button @click="dlg.show = false">取消</el-button>
            <el-button type="primary" :loading="dlg.saving" @click="submit">提交申请</el-button>
          </template>
        </el-dialog>

        <el-dialog v-model="repair.show" title="提交设备报修" width="500px">
          <template v-if="repair.asset">
            <el-descriptions :column="1" border size="small" style="margin-bottom:14px">
              <el-descriptions-item label="设备">{{ repair.asset.name }}（{{ repair.asset.assetNo }}）</el-descriptions-item>
            </el-descriptions>
            <el-form label-width="84px">
              <el-form-item label="故障现象" required>
                <el-input v-model="repair.fault" type="textarea" :rows="3" placeholder="请尽量描述具体现象与复现方式" />
              </el-form-item>
              <el-form-item label="常见故障">
                <el-space wrap>
                  <el-tag v-for="f in faultPresets" :key="f" size="small" effect="plain"
                          style="cursor:pointer" @click="repair.fault = f">{{ f }}</el-tag>
                </el-space>
              </el-form-item>
            </el-form>
            <el-alert type="warning" :closable="false" show-icon
                      title="提交后设备将转入「维修中」状态，管理员会收到工单提醒，处理进度会通知您。" />
          </template>
          <template #footer>
            <el-button @click="repair.show = false">取消</el-button>
            <el-button type="primary" :loading="repair.saving" @click="submitRepair">提交报修</el-button>
          </template>
        </el-dialog>
      </div>`,
  };

  // ================= 我的申请 =================
  AMS.pages['my-applications'] = {
    name: 'MyApplicationsPage',
    props: { user: Object },
    setup() {
      const loading = ref(false);
      const list = ref([]);
      const total = ref(0);
      const filters = reactive({ status: [], type: [] });
      const page = reactive({ page: 1, pageSize: 20 });

      async function load() {
        loading.value = true;
        try {
          const data = await AMS.api.get('/applications', {
            mine: 1, status: filters.status, type: filters.type,
            page: page.page, pageSize: page.pageSize,
          });
          list.value = data.list || [];
          total.value = data.total || 0;
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          loading.value = false;
        }
      }
      onMounted(load);

      async function cancel(row) {
        try {
          await ElMessageBox.confirm(`确定撤销申请 ${row.applyNo} 吗？`, '撤销确认', { type: 'warning' });
        } catch (e) { return; }
        try {
          await AMS.api.post(`/applications/${row.id}/cancel`);
          ElMessage.success('申请已撤销');
          load();
        } catch (e) {
          ElMessage.error(e.message);
        }
      }

      return {
        loading, list, total, filters, page, load, cancel,
        fmtDate: AMS.fmtDate, fmtDateTime: AMS.fmtDateTime, pick: AMS.pick,
        applyType: AMS.applyType, applyStatus: AMS.applyStatus,
        search() { page.page = 1; load(); },
      };
    },
    template: `
      <div>
        <div class="panel">
          <div class="filter-bar">
            <el-select v-model="filters.status" multiple collapse-tags placeholder="审批状态" clearable
                       style="width:230px" @change="search">
              <el-option v-for="(v, k) in applyStatus" :key="k" :label="v.label" :value="k" />
            </el-select>
            <el-select v-model="filters.type" multiple collapse-tags placeholder="申请类型" clearable
                       style="width:210px" @change="search">
              <el-option v-for="(v, k) in applyType" :key="k" :label="v.label" :value="k" />
            </el-select>
            <span style="flex:1"></span>
            <el-button size="small" @click="load"><el-icon><Refresh /></el-icon>刷新</el-button>
          </div>
        </div>

        <div class="panel">
          <el-table :data="list" v-loading="loading" size="small" border stripe>
            <el-table-column prop="applyNo" label="申请单号" width="152" />
            <el-table-column label="类型" width="78">
              <template #default="{ row }">
                <el-tag size="small" :type="pick(applyType, row.type).type">{{ pick(applyType, row.type).label }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="设备" min-width="190" show-overflow-tooltip>
              <template #default="{ row }">
                <div v-if="row.asset">
                  <div>{{ row.asset.name }}</div>
                  <div class="text-muted" style="font-size:11.5px">{{ row.asset.assetNo }}</div>
                </div>
              </template>
            </el-table-column>
            <el-table-column prop="reason" label="申请事由" min-width="200" show-overflow-tooltip />
            <el-table-column label="提交时间" width="145">
              <template #default="{ row }">{{ fmtDateTime(row.createdAt) }}</template>
            </el-table-column>
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <el-tag size="small" :type="pick(applyStatus, row.status).type">{{ pick(applyStatus, row.status).label }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="审批反馈" min-width="220" show-overflow-tooltip>
              <template #default="{ row }">
                <div v-if="row.status !== 'pending' && row.approver">
                  <div style="font-size:12px">{{ row.approver.realName }} · {{ fmtDateTime(row.approvedAt) }}</div>
                  <div class="text-muted" style="font-size:11.5px">{{ row.approveComment }}</div>
                </div>
                <span v-else class="text-muted">等待管理员审批</span>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="82" fixed="right">
              <template #default="{ row }">
                <el-button v-if="row.status === 'pending'" link type="danger" size="small" @click="cancel(row)">撤销</el-button>
                <span v-else class="text-muted">—</span>
              </template>
            </el-table-column>
            <template #empty><empty-tip text="您还没有提交过申请" /></template>
          </el-table>

          <div class="pager">
            <el-pagination background layout="total, prev, pager, next" :total="total"
                           v-model:current-page="page.page" :page-size="page.pageSize" @current-change="load" />
          </div>
        </div>
      </div>`,
  };

  // ================= 个人中心 =================
  AMS.pages.profile = {
    name: 'ProfilePage',
    props: { user: Object },
    emits: ['refresh-user'],
    setup(props, { emit }) {
      const form = reactive({
        realName: props.user.realName, email: props.user.email, phone: props.user.phone,
      });
      const pwd = reactive({ oldPassword: '', newPassword: '', confirm: '' });
      const saving = ref(false);
      const changing = ref(false);

      async function save() {
        saving.value = true;
        try {
          await AMS.api.put('/auth/profile', form);
          ElMessage.success('个人信息已更新');
          emit('refresh-user');
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          saving.value = false;
        }
      }

      async function changePwd() {
        if (pwd.newPassword.length < 6) return ElMessage.warning('新密码长度至少 6 位');
        if (pwd.newPassword !== pwd.confirm) return ElMessage.warning('两次输入的新密码不一致');
        changing.value = true;
        try {
          await AMS.api.put('/auth/password', { oldPassword: pwd.oldPassword, newPassword: pwd.newPassword });
          ElMessage.success('密码已修改，请重新登录');
          setTimeout(() => window.dispatchEvent(new CustomEvent('ams:unauthorized')), 900);
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          changing.value = false;
        }
      }

      return { form, pwd, saving, changing, save, changePwd, fmtDateTime: AMS.fmtDateTime };
    },
    template: `
      <div style="max-width:720px">
        <div class="panel">
          <div class="panel-title">账号信息</div>
          <el-descriptions :column="2" border size="small">
            <el-descriptions-item label="登录名">{{ user.username }}</el-descriptions-item>
            <el-descriptions-item label="角色">{{ user.roleName }}</el-descriptions-item>
            <el-descriptions-item label="所属部门">{{ user.departmentName || '—' }}</el-descriptions-item>
            <el-descriptions-item label="上次登录">{{ fmtDateTime(user.lastLoginAt) }}</el-descriptions-item>
            <el-descriptions-item label="权限点" :span="2">
              <el-space wrap>
                <el-tag v-for="p in user.permissions" :key="p" size="small" effect="plain">{{ p }}</el-tag>
              </el-space>
            </el-descriptions-item>
          </el-descriptions>
        </div>

        <div class="panel">
          <div class="panel-title">编辑资料</div>
          <el-form label-width="90px">
            <el-form-item label="真实姓名"><el-input v-model="form.realName" style="max-width:320px" /></el-form-item>
            <el-form-item label="邮箱">
              <el-input v-model="form.email" style="max-width:320px" placeholder="用于接收到期提醒邮件" />
            </el-form-item>
            <el-form-item label="手机号"><el-input v-model="form.phone" style="max-width:320px" /></el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="saving" @click="save">保存</el-button>
            </el-form-item>
          </el-form>
        </div>

        <div class="panel">
          <div class="panel-title">修改密码</div>
          <el-form label-width="90px">
            <el-form-item label="原密码">
              <el-input v-model="pwd.oldPassword" type="password" show-password style="max-width:320px" />
            </el-form-item>
            <el-form-item label="新密码">
              <el-input v-model="pwd.newPassword" type="password" show-password style="max-width:320px" />
            </el-form-item>
            <el-form-item label="确认新密码">
              <el-input v-model="pwd.confirm" type="password" show-password style="max-width:320px" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="changing" @click="changePwd">修改密码</el-button>
            </el-form-item>
          </el-form>
        </div>
      </div>`,
  };
})(window.AMS);

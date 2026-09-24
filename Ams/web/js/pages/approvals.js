(function (AMS) {
  'use strict';
  const { ref, reactive, onMounted } = Vue;
  const { ElMessage } = ElementPlus;

  // 审批中心（管理员）：处理领用 / 归还 / 续借 / 报废四类申请。
  AMS.pages.approvals = {
    name: 'ApprovalsPage',
    props: { user: Object },
    setup() {
      const loading = ref(false);
      const list = ref([]);
      const total = ref(0);
      const depts = ref([]);

      const filters = reactive({ status: ['pending'], type: [], keyword: '', deptId: '', range: [] });
      const page = reactive({ page: 1, pageSize: 20 });

      async function load() {
        loading.value = true;
        try {
          const p = {
            status: filters.status, type: filters.type,
            keyword: filters.keyword, deptId: filters.deptId,
            page: page.page, pageSize: page.pageSize,
          };
          AMS.applyRange(p, filters.range, 'createdFrom', 'createdTo');
          const data = await AMS.api.get('/applications', p);
          list.value = data.list || [];
          total.value = data.total || 0;
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          loading.value = false;
        }
      }

      function search() { page.page = 1; load(); }

      onMounted(async () => {
        try {
          const opt = await AMS.api.get('/assets/options');
          depts.value = opt.departments || [];
        } catch (e) { /* 忽略 */ }
        load();
      });

      // ---------- 审批弹窗 ----------
      const dlg = reactive({ show: false, row: null, approve: true, comment: '', saving: false });

      const presets = {
        approve: ['同意领用，请按期归还并妥善保管。', '同意，已核对库存与使用需求。', '同意续借，请于新到期日前归还。'],
        reject: ['库存紧张，建议先复用本组闲置设备。', '申请事由不充分，请补充说明用途后重新提交。', '该设备已有其他优先级更高的使用安排。'],
      };

      function open(row, approve) {
        Object.assign(dlg, {
          show: true, row, approve, saving: false,
          comment: approve ? presets.approve[0] : '',
        });
      }

      async function submit() {
        if (!dlg.approve && !dlg.comment.trim()) {
          return ElMessage.warning('驳回时请填写理由，便于申请人了解原因');
        }
        dlg.saving = true;
        try {
          const path = `/applications/${dlg.row.id}/${dlg.approve ? 'approve' : 'reject'}`;
          const res = await AMS.api.post(path, { comment: dlg.comment });
          ElMessage.success(res.message);
          dlg.show = false;
          load();
          window.dispatchEvent(new CustomEvent('ams:refresh-badge'));
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          dlg.saving = false;
        }
      }

      return {
        loading, list, total, depts, filters, page, load, search,
        dlg, open, submit, presets,
        fmtDate: AMS.fmtDate, fmtDateTime: AMS.fmtDateTime, pick: AMS.pick,
        applyType: AMS.applyType, applyStatus: AMS.applyStatus,
      };
    },
    template: `
      <div>
        <div class="panel">
          <div class="filter-bar">
            <el-select v-model="filters.status" multiple collapse-tags placeholder="审批状态" clearable style="width:220px">
              <el-option v-for="(v, k) in applyStatus" :key="k" :label="v.label" :value="k" />
            </el-select>
            <el-select v-model="filters.type" multiple collapse-tags placeholder="申请类型" clearable style="width:200px">
              <el-option v-for="(v, k) in applyType" :key="k" :label="v.label" :value="k" />
            </el-select>
            <el-select v-model="filters.deptId" placeholder="申请部门" clearable style="width:190px">
              <el-option v-for="d in depts" :key="d.id" :label="d.name" :value="String(d.id)" />
            </el-select>
            <el-date-picker v-model="filters.range" type="daterange" value-format="YYYY-MM-DD"
                            start-placeholder="提交起" end-placeholder="提交止" range-separator="~" style="width:250px" />
            <el-input v-model="filters.keyword" placeholder="单号 / 事由" clearable style="width:200px" @keyup.enter="search" />
            <el-button type="primary" @click="search"><el-icon><Search /></el-icon>查询</el-button>
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
            <el-table-column label="设备" min-width="200" show-overflow-tooltip>
              <template #default="{ row }">
                <div v-if="row.asset">
                  <div>{{ row.asset.name }}</div>
                  <div class="text-muted" style="font-size:11.5px">{{ row.asset.assetNo }} · {{ row.asset.category ? row.asset.category.name : '' }}</div>
                </div>
                <span v-else class="text-muted">设备已删除</span>
              </template>
            </el-table-column>
            <el-table-column label="申请人" width="130">
              <template #default="{ row }">
                <div v-if="row.applicant">
                  <div>{{ row.applicant.realName }}</div>
                  <div class="text-muted" style="font-size:11.5px">{{ row.applicant.department ? row.applicant.department.name : '' }}</div>
                </div>
              </template>
            </el-table-column>
            <el-table-column prop="reason" label="申请事由" min-width="190" show-overflow-tooltip />
            <el-table-column label="借用期限" width="150">
              <template #default="{ row }">
                <span v-if="row.loanDays">{{ row.loanDays }} 天</span>
                <span v-else class="text-muted">—</span>
                <div v-if="row.expectDueAt" class="text-muted" style="font-size:11.5px">至 {{ fmtDate(row.expectDueAt) }}</div>
              </template>
            </el-table-column>
            <el-table-column label="提交时间" width="145">
              <template #default="{ row }">{{ fmtDateTime(row.createdAt) }}</template>
            </el-table-column>
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <el-tag size="small" :type="pick(applyStatus, row.status).type">{{ pick(applyStatus, row.status).label }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="审批信息" min-width="190" show-overflow-tooltip>
              <template #default="{ row }">
                <div v-if="row.status !== 'pending' && row.approver">
                  <div style="font-size:12px">{{ row.approver.realName }} · {{ fmtDateTime(row.approvedAt) }}</div>
                  <div class="text-muted" style="font-size:11.5px">{{ row.approveComment }}</div>
                </div>
                <span v-else class="text-muted">—</span>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="132" fixed="right">
              <template #default="{ row }">
                <template v-if="row.status === 'pending'">
                  <el-button link type="success" size="small" @click="open(row, true)">通过</el-button>
                  <el-button link type="danger" size="small" @click="open(row, false)">驳回</el-button>
                </template>
                <span v-else class="text-muted">已处理</span>
              </template>
            </el-table-column>
            <template #empty><empty-tip text="当前没有符合条件的申请" /></template>
          </el-table>

          <div class="pager">
            <el-pagination background layout="total, prev, pager, next"
                           :total="total" v-model:current-page="page.page" :page-size="page.pageSize"
                           @current-change="load" />
          </div>
        </div>

        <el-dialog v-model="dlg.show" :title="dlg.approve ? '审批通过' : '驳回申请'" width="540px">
          <template v-if="dlg.row">
            <el-descriptions :column="2" border size="small" style="margin-bottom:14px">
              <el-descriptions-item label="申请单号">{{ dlg.row.applyNo }}</el-descriptions-item>
              <el-descriptions-item label="类型">{{ pick(applyType, dlg.row.type).label }}</el-descriptions-item>
              <el-descriptions-item label="设备">{{ dlg.row.asset ? dlg.row.asset.name : '—' }}</el-descriptions-item>
              <el-descriptions-item label="申请人">{{ dlg.row.applicant ? dlg.row.applicant.realName : '—' }}</el-descriptions-item>
              <el-descriptions-item label="申请事由" :span="2">{{ dlg.row.reason }}</el-descriptions-item>
              <el-descriptions-item v-if="dlg.row.expectDueAt" label="预计到期" :span="2">
                {{ fmtDate(dlg.row.expectDueAt) }}（{{ dlg.row.loanDays }} 天）
              </el-descriptions-item>
            </el-descriptions>

            <el-form label-width="76px">
              <el-form-item :label="dlg.approve ? '审批意见' : '驳回理由'" :required="!dlg.approve">
                <el-input v-model="dlg.comment" type="textarea" :rows="3"
                          :placeholder="dlg.approve ? '可填写补充说明' : '请说明驳回原因，申请人会收到通知'" />
              </el-form-item>
              <el-form-item label="常用语">
                <el-space wrap>
                  <el-tag v-for="p in (dlg.approve ? presets.approve : presets.reject)" :key="p"
                          size="small" effect="plain" style="cursor:pointer" @click="dlg.comment = p">
                    {{ p.length > 16 ? p.slice(0, 16) + '…' : p }}
                  </el-tag>
                </el-space>
              </el-form-item>
            </el-form>

            <el-alert v-if="dlg.approve" type="success" :closable="false" show-icon
                      title="通过后系统会自动变更设备状态，并向申请人推送站内信与邮件通知。" />
            <el-alert v-else type="warning" :closable="false" show-icon
                      title="驳回不会改变设备状态，申请人会收到含驳回理由的通知。" />
          </template>
          <template #footer>
            <el-button @click="dlg.show = false">取消</el-button>
            <el-button :type="dlg.approve ? 'success' : 'danger'" :loading="dlg.saving" @click="submit">
              {{ dlg.approve ? '确认通过' : '确认驳回' }}
            </el-button>
          </template>
        </el-dialog>
      </div>`,
  };
})(window.AMS);

(function (AMS) {
  'use strict';
  const { ref, reactive, onMounted } = Vue;
  const { ElMessage } = ElementPlus;

  // 报修工单。管理员看全部并处理，普通用户在「我的报修」里复用同一组件（mine 模式）。
  AMS.pages.repairs = {
    name: 'RepairsPage',
    props: { user: Object, mine: Boolean },
    setup(props) {
      const loading = ref(false);
      const list = ref([]);
      const total = ref(0);
      const filters = reactive({ status: [] });
      const page = reactive({ page: 1, pageSize: 20 });

      const canHandle = () => !props.mine && (props.user.permissions || []).includes('repair:handle');

      async function load() {
        loading.value = true;
        try {
          const data = await AMS.api.get('/repairs', {
            status: filters.status, mine: props.mine ? 1 : '',
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

      const dlg = reactive({
        show: false, row: null, saving: false,
        status: 'resolved', solution: '', cost: 0, backToStock: true,
      });

      function open(row) {
        Object.assign(dlg, {
          show: true, row, saving: false,
          status: row.status === 'open' ? 'processing' : 'resolved',
          solution: '', cost: 0,
          // 原本有领用人的设备，修完默认退还本人而不是收回库房。
          backToStock: !(row.asset && row.asset.holderId),
        });
      }

      async function submit() {
        dlg.saving = true;
        try {
          const res = await AMS.api.post(`/repairs/${dlg.row.id}/handle`, {
            status: dlg.status, solution: dlg.solution,
            cost: Number(dlg.cost) || 0, backToStock: dlg.backToStock,
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

      return {
        loading, list, total, filters, page, load, canHandle,
        dlg, open, submit,
        fmtDateTime: AMS.fmtDateTime, money: AMS.money, pick: AMS.pick,
        repairStatus: AMS.repairStatus,
        search() { page.page = 1; load(); },
      };
    },
    template: `
      <div>
        <div class="panel">
          <div class="filter-bar">
            <el-select v-model="filters.status" multiple collapse-tags placeholder="工单状态" clearable style="width:240px">
              <el-option v-for="(v, k) in repairStatus" :key="k" :label="v.label" :value="k" />
            </el-select>
            <el-button type="primary" @click="search"><el-icon><Search /></el-icon>查询</el-button>
            <span class="spacer" style="flex:1"></span>
            <span class="text-muted" style="font-size:12.5px">
              {{ mine ? '这里是您提交的全部报修工单' : '待处理工单会排在最前面' }}
            </span>
          </div>
        </div>

        <div class="panel">
          <el-table :data="list" v-loading="loading" size="small" border stripe>
            <el-table-column prop="ticketNo" label="工单号" width="145" />
            <el-table-column label="设备" min-width="200" show-overflow-tooltip>
              <template #default="{ row }">
                <div v-if="row.asset">
                  <div>{{ row.asset.name }}</div>
                  <div class="text-muted" style="font-size:11.5px">{{ row.asset.assetNo }}</div>
                </div>
                <span v-else class="text-muted">—</span>
              </template>
            </el-table-column>
            <el-table-column prop="fault" label="故障现象" min-width="230" show-overflow-tooltip />
            <el-table-column label="报修人" width="95">
              <template #default="{ row }">{{ row.reporter ? row.reporter.realName : '—' }}</template>
            </el-table-column>
            <el-table-column label="报修时间" width="145">
              <template #default="{ row }">{{ fmtDateTime(row.createdAt) }}</template>
            </el-table-column>
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <el-tag size="small" :type="pick(repairStatus, row.status).type">{{ pick(repairStatus, row.status).label }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="处理结果" min-width="200" show-overflow-tooltip>
              <template #default="{ row }">
                <div v-if="row.solution">
                  <div style="font-size:12px">{{ row.solution }}</div>
                  <div class="text-muted" style="font-size:11.5px">
                    {{ row.handler ? row.handler.realName : '' }}
                    <span v-if="row.cost"> · 费用 {{ money(row.cost) }}</span>
                  </div>
                </div>
                <span v-else class="text-muted">—</span>
              </template>
            </el-table-column>
            <el-table-column v-if="canHandle()" label="操作" width="92" fixed="right">
              <template #default="{ row }">
                <el-button v-if="row.status === 'open' || row.status === 'processing'"
                           link type="primary" size="small" @click="open(row)">处理</el-button>
                <span v-else class="text-muted">已完结</span>
              </template>
            </el-table-column>
            <template #empty><empty-tip text="暂无报修工单" /></template>
          </el-table>

          <div class="pager">
            <el-pagination background layout="total, prev, pager, next" :total="total"
                           v-model:current-page="page.page" :page-size="page.pageSize" @current-change="load" />
          </div>
        </div>

        <el-dialog v-model="dlg.show" title="处理报修工单" width="520px">
          <template v-if="dlg.row">
            <el-descriptions :column="1" border size="small" style="margin-bottom:14px">
              <el-descriptions-item label="工单号">{{ dlg.row.ticketNo }}</el-descriptions-item>
              <el-descriptions-item label="设备">{{ dlg.row.asset ? dlg.row.asset.name + '（' + dlg.row.asset.assetNo + '）' : '—' }}</el-descriptions-item>
              <el-descriptions-item label="故障现象">{{ dlg.row.fault }}</el-descriptions-item>
            </el-descriptions>

            <el-form label-width="96px">
              <el-form-item label="处理进度">
                <el-radio-group v-model="dlg.status">
                  <el-radio value="processing">处理中</el-radio>
                  <el-radio value="resolved">已修复</el-radio>
                  <el-radio value="closed">关闭工单</el-radio>
                </el-radio-group>
              </el-form-item>
              <el-form-item label="处理说明">
                <el-input v-model="dlg.solution" type="textarea" :rows="3"
                          placeholder="如：更换主板排线，已通过 2 小时压力测试" />
              </el-form-item>
              <el-form-item label="维修费用">
                <el-input-number v-model="dlg.cost" :min="0" :precision="2" :step="50" style="width:100%" />
              </el-form-item>
              <el-form-item v-if="dlg.status === 'resolved'" label="修复后归属">
                <el-radio-group v-model="dlg.backToStock">
                  <el-radio :value="false">退还原领用人</el-radio>
                  <el-radio :value="true">收回库房（在库）</el-radio>
                </el-radio-group>
              </el-form-item>
            </el-form>
          </template>
          <template #footer>
            <el-button @click="dlg.show = false">取消</el-button>
            <el-button type="primary" :loading="dlg.saving" @click="submit">提交</el-button>
          </template>
        </el-dialog>
      </div>`,
  };
})(window.AMS);

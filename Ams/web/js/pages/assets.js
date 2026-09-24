(function (AMS) {
  'use strict';
  const { ref, reactive, computed, onMounted, watch } = Vue;
  const { ElMessage, ElMessageBox } = ElementPlus;

  // 设备台账：需求 4 的全部过滤维度 + 批量操作 + 导入导出 + 资产履历。
  AMS.pages.assets = {
    name: 'AssetsPage',
    props: { user: Object, query: Object },
    setup(props) {
      const loading = ref(false);
      const list = ref([]);
      const total = ref(0);
      const selected = ref([]);
      const options = ref({ categories: [], statuses: [], locations: [], brands: [], departments: [] });

      const filters = reactive({
        keyword: '', categoryId: [], status: [], location: '', deptId: '',
        brand: '', quick: '', borrowRange: [], dueRange: [], purchaseRange: [],
        priceMin: '', priceMax: '',
      });
      const sort = reactive({ sortBy: 'createdAt', sortOrder: 'desc' });
      const page = reactive({ page: 1, pageSize: 20 });

      const can = (p) => (props.user.permissions || []).includes(p);

      function buildParams() {
        const p = {
          keyword: filters.keyword, categoryId: filters.categoryId, status: filters.status,
          location: filters.location, deptId: filters.deptId, brand: filters.brand,
          quick: filters.quick, priceMin: filters.priceMin, priceMax: filters.priceMax,
          sortBy: sort.sortBy, sortOrder: sort.sortOrder,
        };
        AMS.applyRange(p, filters.borrowRange, 'borrowedFrom', 'borrowedTo');
        AMS.applyRange(p, filters.dueRange, 'dueFrom', 'dueTo');
        AMS.applyRange(p, filters.purchaseRange, 'purchaseFrom', 'purchaseTo');
        return p;
      }

      async function load() {
        loading.value = true;
        try {
          const data = await AMS.api.get('/assets', Object.assign(buildParams(), page));
          list.value = data.list || [];
          total.value = data.total || 0;
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          loading.value = false;
        }
      }

      function search() { page.page = 1; load(); }

      function reset() {
        Object.assign(filters, {
          keyword: '', categoryId: [], status: [], location: '', deptId: '',
          brand: '', quick: '', borrowRange: [], dueRange: [], purchaseRange: [],
          priceMin: '', priceMax: '',
        });
        search();
      }

      // 首页卡片跳转过来时带了筛选条件，这里接住。
      function applyRouteQuery() {
        const q = props.query || {};
        if (q.quick) filters.quick = q.quick;
        if (q.status) filters.status = String(q.status).split(',');
        if (q.keyword) filters.keyword = q.keyword;
      }

      onMounted(async () => {
        applyRouteQuery();
        try { options.value = await AMS.api.get('/assets/options'); } catch (e) { /* 下拉数据失败不阻塞主列表 */ }
        load();
      });
      watch(() => props.query, () => { applyRouteQuery(); search(); }, { deep: true });

      function onSortChange({ prop, order }) {
        if (!order) { sort.sortBy = 'createdAt'; sort.sortOrder = 'desc'; }
        else { sort.sortBy = prop; sort.sortOrder = order === 'ascending' ? 'asc' : 'desc'; }
        load();
      }

      // ---------- 详情抽屉 ----------
      const drawer = reactive({ show: false, id: null });
      const openDetail = (row) => { drawer.id = row.id; drawer.show = true; };

      // ---------- 新增 / 编辑 ----------
      const dialog = reactive({ show: false, editing: false, saving: false });
      const formRef = ref(null);
      const blank = () => ({
        id: null, assetNo: '', name: '', categoryId: null, brand: '', model: '',
        serialNo: '', specification: '', status: 'in_stock', location: '',
        price: 0, purchaseDate: '', warrantyUntil: '', remark: '',
      });
      const form = reactive(blank());
      const rules = {
        name: [{ required: true, message: '请填写设备名称', trigger: 'blur' }],
        categoryId: [{ required: true, message: '请选择设备类型', trigger: 'change' }],
      };

      function openCreate() {
        Object.assign(form, blank());
        dialog.editing = false;
        dialog.show = true;
      }

      function openEdit(row) {
        Object.assign(form, {
          id: row.id, assetNo: row.assetNo, name: row.name, categoryId: row.categoryId,
          brand: row.brand, model: row.model, serialNo: row.serialNo,
          specification: row.specification, status: row.status, location: row.location,
          price: row.price, purchaseDate: row.purchaseDate ? row.purchaseDate.slice(0, 10) : '',
          warrantyUntil: row.warrantyUntil ? row.warrantyUntil.slice(0, 10) : '',
          remark: row.remark,
        });
        dialog.editing = true;
        dialog.show = true;
      }

      async function save() {
        await formRef.value.validate();
        dialog.saving = true;
        try {
          const body = Object.assign({}, form);
          delete body.id;
          if (dialog.editing) await AMS.api.put('/assets/' + form.id, body);
          else await AMS.api.post('/assets', body);
          ElMessage.success(dialog.editing ? '设备信息已更新' : '设备已新增');
          dialog.show = false;
          load();
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          dialog.saving = false;
        }
      }

      async function remove(row) {
        try {
          await ElMessageBox.confirm(
            `确定删除设备「${row.name}（${row.assetNo}）」吗？该操作会同时清除其资产履历，且不可恢复。`,
            '删除确认', { type: 'warning', confirmButtonText: '确定删除', cancelButtonText: '取消' });
        } catch (e) { return; }
        try {
          await AMS.api.del('/assets/' + row.id);
          ElMessage.success('设备已删除');
          load();
        } catch (e) {
          ElMessage.error(e.message);
        }
      }

      // ---------- 批量操作 ----------
      const batch = reactive({ show: false, action: 'scrap', location: '', remark: '', saving: false });
      const batchLabels = {
        scrap: '批量报废', repair: '批量转维修', instock: '批量回库',
        transfer: '批量调拨', delete: '批量删除',
      };

      function openBatch() {
        if (!selected.value.length) return ElMessage.warning('请先勾选要操作的设备');
        Object.assign(batch, { show: true, action: 'scrap', location: '', remark: '', saving: false });
      }

      async function runBatch() {
        batch.saving = true;
        try {
          const res = await AMS.api.post('/assets/batch', {
            ids: selected.value.map((x) => x.id),
            action: batch.action, location: batch.location, remark: batch.remark,
          });
          ElMessage.success(res.message);
          batch.show = false;
          selected.value = [];
          load();
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          batch.saving = false;
        }
      }

      // ---------- 导入 / 导出 ----------
      function exportFile(format) {
        AMS.api.download('/assets/export', Object.assign(buildParams(), { format }));
        ElMessage.success('已按当前筛选条件导出，请查看浏览器下载');
      }

      const importer = reactive({ show: false, uploading: false, result: null });

      async function doImport(file) {
        importer.uploading = true;
        importer.result = null;
        try {
          importer.result = await AMS.api.upload('/assets/import', file);
          const r = importer.result;
          if (r.failed === 0) ElMessage.success(`导入完成：新增 ${r.created} 条，更新 ${r.updated} 条`);
          else ElMessage.warning(`导入完成，但有 ${r.failed} 行未通过校验`);
          load();
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          importer.uploading = false;
        }
        return false; // 阻止 el-upload 自己发请求
      }

      const quickTabs = [
        { value: '', label: '全部' },
        { value: 'available', label: '可领用' },
        { value: 'dueSoon', label: '即将到期' },
        { value: 'overdue', label: '已逾期' },
        { value: 'warrantyExpiring', label: '保修将过期' },
      ];

      const selectedCount = computed(() => selected.value.length);

      return {
        loading, list, total, selected, options, filters, sort, page, quickTabs, selectedCount,
        load, search, reset, onSortChange, can,
        drawer, openDetail,
        dialog, form, formRef, rules, openCreate, openEdit, save, remove,
        batch, batchLabels, openBatch, runBatch,
        exportFile, importer, doImport,
        downloadTemplate: () => AMS.api.download('/assets/import/template'),
        fmtDate: AMS.fmtDate, money: AMS.money,
        statusOptions: AMS.assetStatus,
      };
    },
    template: `
      <div>
        <div class="panel">
          <div class="filter-bar">
            <el-input v-model="filters.keyword" placeholder="编号 / 名称 / 序列号 / 品牌" clearable
                      @keyup.enter="search" style="width:240px">
              <template #prefix><el-icon><Search /></el-icon></template>
            </el-input>
            <el-select v-model="filters.categoryId" multiple collapse-tags collapse-tags-tooltip
                       placeholder="设备类型" clearable style="width:210px">
              <el-option v-for="c in options.categories" :key="c.id" :label="c.name" :value="String(c.id)" />
            </el-select>
            <el-select v-model="filters.status" multiple collapse-tags collapse-tags-tooltip
                       placeholder="设备状态" clearable style="width:200px">
              <el-option v-for="s in options.statuses" :key="s.value" :label="s.label" :value="s.value" />
            </el-select>
            <el-select v-model="filters.location" placeholder="存放位置" clearable filterable style="width:180px">
              <el-option v-for="l in options.locations" :key="l" :label="l" :value="l" />
            </el-select>
            <el-select v-model="filters.deptId" placeholder="领用部门" clearable style="width:180px">
              <el-option v-for="d in options.departments" :key="d.id" :label="d.name" :value="String(d.id)" />
            </el-select>
          </div>

          <div class="filter-bar" style="margin-top:10px">
            <el-date-picker v-model="filters.borrowRange" type="daterange" value-format="YYYY-MM-DD"
                            start-placeholder="领用起" end-placeholder="领用止" range-separator="~"
                            style="width:250px" />
            <el-date-picker v-model="filters.dueRange" type="daterange" value-format="YYYY-MM-DD"
                            start-placeholder="到期起" end-placeholder="到期止" range-separator="~"
                            style="width:250px" />
            <el-date-picker v-model="filters.purchaseRange" type="daterange" value-format="YYYY-MM-DD"
                            start-placeholder="采购起" end-placeholder="采购止" range-separator="~"
                            style="width:250px" />
            <el-input v-model="filters.priceMin" placeholder="价格下限" style="width:112px" clearable />
            <span class="text-muted">—</span>
            <el-input v-model="filters.priceMax" placeholder="价格上限" style="width:112px" clearable />
            <el-button type="primary" @click="search"><el-icon><Search /></el-icon>查询</el-button>
            <el-button @click="reset">重置</el-button>
          </div>
        </div>

        <div class="panel">
          <div class="table-toolbar">
            <el-radio-group v-model="filters.quick" size="small" @change="search">
              <el-radio-button v-for="t in quickTabs" :key="t.value" :value="t.value">{{ t.label }}</el-radio-button>
            </el-radio-group>
            <span class="spacer"></span>
            <el-tag v-if="selectedCount" type="primary" effect="plain">已选 {{ selectedCount }} 项</el-tag>
            <el-button v-if="can('asset:edit')" size="small" :disabled="!selectedCount" @click="openBatch">
              <el-icon><Operation /></el-icon>批量操作
            </el-button>
            <el-dropdown v-if="can('asset:export')" @command="exportFile">
              <el-button size="small"><el-icon><Download /></el-icon>导出<el-icon><ArrowDown /></el-icon></el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="xlsx">导出为 Excel (.xlsx)</el-dropdown-item>
                  <el-dropdown-item command="csv">导出为 CSV</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
            <el-button v-if="can('asset:import')" size="small" @click="importer.show = true">
              <el-icon><Upload /></el-icon>导入
            </el-button>
            <el-button v-if="can('asset:edit')" type="primary" size="small" @click="openCreate">
              <el-icon><Plus /></el-icon>新增设备
            </el-button>
          </div>

          <el-table :data="list" v-loading="loading" size="small" border stripe
                    @selection-change="v => selected = v" @sort-change="onSortChange"
                    :row-key="r => r.id">
            <!-- 列表只保留台账核对必需的字段；品牌型号、存放位置、领用部门等在详情抽屉里看，
                 否则列宽会撑出横向滚动条，反而看不到「到期时间」这类关键信息。 -->
            <el-table-column type="selection" width="42" :selectable="() => can('asset:edit')" />
            <el-table-column prop="assetNo" label="资产编号" width="122" sortable="custom" />
            <el-table-column prop="name" label="设备名称" min-width="150" sortable="custom" show-overflow-tooltip>
              <template #default="{ row }">
                <el-link type="primary" :underline="false" @click="openDetail(row)">{{ row.name }}</el-link>
                <div class="text-muted" style="font-size:11.5px">{{ row.brand }} {{ row.model }}</div>
              </template>
            </el-table-column>
            <el-table-column label="类型" width="92">
              <template #default="{ row }">{{ row.category ? row.category.name : '—' }}</template>
            </el-table-column>
            <el-table-column prop="status" label="状态" width="86" sortable="custom">
              <template #default="{ row }"><status-tag :status="row.status" /></template>
            </el-table-column>
            <el-table-column label="领用人" width="118" show-overflow-tooltip>
              <template #default="{ row }">
                <template v-if="row.holder">
                  <div>{{ row.holder.realName }}</div>
                  <div class="text-muted" style="font-size:11.5px">{{ row.holderDept ? row.holderDept.name : '' }}</div>
                </template>
                <span v-else class="text-muted">—</span>
              </template>
            </el-table-column>
            <el-table-column prop="borrowedAt" label="领用时间" width="104" sortable="custom">
              <template #default="{ row }">{{ fmtDate(row.borrowedAt) }}</template>
            </el-table-column>
            <el-table-column prop="dueAt" label="到期时间" width="176" sortable="custom">
              <template #default="{ row }"><due-tag :due-at="row.dueAt" :status="row.status" /></template>
            </el-table-column>
            <el-table-column prop="price" label="采购价" width="104" align="right" sortable="custom">
              <template #default="{ row }">{{ money(row.price) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="142" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" size="small" @click="openDetail(row)">详情</el-button>
                <el-button v-if="can('asset:edit')" link type="primary" size="small" @click="openEdit(row)">编辑</el-button>
                <el-button v-if="can('asset:delete')" link type="danger" size="small" @click="remove(row)">删除</el-button>
              </template>
            </el-table-column>
            <template #empty><empty-tip text="没有符合条件的设备" /></template>
          </el-table>

          <div class="pager">
            <el-pagination background layout="total, sizes, prev, pager, next, jumper"
                           :total="total" v-model:current-page="page.page" v-model:page-size="page.pageSize"
                           :page-sizes="[10, 20, 50, 100]" @current-change="load" @size-change="load" />
          </div>
        </div>

        <asset-drawer v-model="drawer.show" :asset-id="drawer.id" />

        <!-- 新增 / 编辑 -->
        <el-dialog v-model="dialog.show" :title="dialog.editing ? '编辑设备' : '新增设备'" width="720px" destroy-on-close>
          <el-form ref="formRef" :model="form" :rules="rules" label-width="96px">
            <el-row :gutter="16">
              <el-col :span="12"><el-form-item label="资产编号">
                <el-input v-model="form.assetNo" placeholder="留空则按类型自动生成" />
              </el-form-item></el-col>
              <el-col :span="12"><el-form-item label="设备名称" prop="name">
                <el-input v-model="form.name" placeholder="如 ThinkPad X1 Carbon" />
              </el-form-item></el-col>
              <el-col :span="12"><el-form-item label="设备类型" prop="categoryId">
                <el-select v-model="form.categoryId" placeholder="请选择" style="width:100%">
                  <el-option v-for="c in options.categories" :key="c.id" :label="c.name" :value="c.id" />
                </el-select>
              </el-form-item></el-col>
              <el-col :span="12"><el-form-item label="设备状态">
                <el-select v-model="form.status" style="width:100%">
                  <el-option v-for="s in options.statuses" :key="s.value" :label="s.label" :value="s.value" />
                </el-select>
              </el-form-item></el-col>
              <el-col :span="12"><el-form-item label="品牌"><el-input v-model="form.brand" /></el-form-item></el-col>
              <el-col :span="12"><el-form-item label="型号"><el-input v-model="form.model" /></el-form-item></el-col>
              <el-col :span="12"><el-form-item label="序列号"><el-input v-model="form.serialNo" /></el-form-item></el-col>
              <el-col :span="12"><el-form-item label="存放位置">
                <el-select v-model="form.location" filterable allow-create default-first-option style="width:100%" placeholder="可直接输入新位置">
                  <el-option v-for="l in options.locations" :key="l" :label="l" :value="l" />
                </el-select>
              </el-form-item></el-col>
              <el-col :span="12"><el-form-item label="采购价格">
                <el-input-number v-model="form.price" :min="0" :precision="2" :step="100" style="width:100%" />
              </el-form-item></el-col>
              <el-col :span="12"><el-form-item label="采购日期">
                <el-date-picker v-model="form.purchaseDate" type="date" value-format="YYYY-MM-DD" style="width:100%" />
              </el-form-item></el-col>
              <el-col :span="12"><el-form-item label="保修到期">
                <el-date-picker v-model="form.warrantyUntil" type="date" value-format="YYYY-MM-DD" style="width:100%" />
              </el-form-item></el-col>
              <el-col :span="24"><el-form-item label="规格配置">
                <el-input v-model="form.specification" placeholder="如 i7-1360P / 32G / 1T SSD / 14英寸" />
              </el-form-item></el-col>
              <el-col :span="24"><el-form-item label="备注">
                <el-input v-model="form.remark" type="textarea" :rows="2" />
              </el-form-item></el-col>
            </el-row>
          </el-form>
          <template #footer>
            <el-button @click="dialog.show = false">取消</el-button>
            <el-button type="primary" :loading="dialog.saving" @click="save">保存</el-button>
          </template>
        </el-dialog>

        <!-- 批量操作 -->
        <el-dialog v-model="batch.show" title="批量操作" width="480px">
          <el-alert type="info" :closable="false" show-icon style="margin-bottom:16px"
                    :title="'已选中 ' + selectedCount + ' 台设备。领用中的设备不会被改状态或删除，系统会自动跳过。'" />
          <el-form label-width="90px">
            <el-form-item label="操作类型">
              <el-select v-model="batch.action" style="width:100%">
                <el-option v-for="(v, k) in batchLabels" :key="k" :label="v" :value="k" />
              </el-select>
            </el-form-item>
            <el-form-item v-if="batch.action === 'transfer'" label="目标位置">
              <el-select v-model="batch.location" filterable allow-create default-first-option style="width:100%" placeholder="选择或输入新位置">
                <el-option v-for="l in options.locations" :key="l" :label="l" :value="l" />
              </el-select>
            </el-form-item>
            <el-form-item v-if="batch.action === 'scrap'" label="报废原因">
              <el-input v-model="batch.remark" type="textarea" :rows="2" placeholder="如：超过使用年限，经鉴定无维修价值" />
            </el-form-item>
          </el-form>
          <template #footer>
            <el-button @click="batch.show = false">取消</el-button>
            <el-button type="primary" :loading="batch.saving" @click="runBatch">确定执行</el-button>
          </template>
        </el-dialog>

        <!-- 导入 -->
        <el-dialog v-model="importer.show" title="批量导入设备" width="640px">
          <el-alert type="info" :closable="false" show-icon style="margin-bottom:14px"
                    title="支持 .xlsx 与 .csv。资产编号已存在的行会执行更新，留空则自动生成编号。建议先下载模板。" />
          <el-button size="small" @click="downloadTemplate" style="margin-bottom:14px">
            <el-icon><Download /></el-icon>下载导入模板
          </el-button>

          <el-upload drag :auto-upload="false" :show-file-list="false" accept=".xlsx,.csv"
                     :on-change="(f) => doImport(f.raw)">
            <el-icon class="el-icon--upload"><UploadFilled /></el-icon>
            <div class="el-upload__text">将文件拖到此处，或<em>点击选择文件</em></div>
          </el-upload>

          <div v-loading="importer.uploading" style="min-height:8px"></div>

          <div v-if="importer.result" class="mt10">
            <el-descriptions :column="4" border size="small">
              <el-descriptions-item label="总行数">{{ importer.result.total }}</el-descriptions-item>
              <el-descriptions-item label="新增">
                <span style="color:#52ac5e;font-weight:600">{{ importer.result.created }}</span>
              </el-descriptions-item>
              <el-descriptions-item label="更新">
                <span style="color:#3a7bd5;font-weight:600">{{ importer.result.updated }}</span>
              </el-descriptions-item>
              <el-descriptions-item label="失败">
                <span :style="{ color: importer.result.failed ? '#e05c5c' : '#98a1b3', fontWeight: 600 }">{{ importer.result.failed }}</span>
              </el-descriptions-item>
            </el-descriptions>

            <div v-if="importer.result.errors && importer.result.errors.length" class="mt10">
              <div class="panel-title" style="margin-bottom:8px">未通过校验的行</div>
              <el-table :data="importer.result.errors" size="small" border max-height="240" class="import-result-errors">
                <el-table-column prop="row" label="行号" width="70" />
                <el-table-column prop="assetNo" label="资产编号" width="130" />
                <el-table-column prop="reason" label="原因" show-overflow-tooltip />
              </el-table>
            </div>
          </div>

          <template #footer>
            <el-button type="primary" @click="importer.show = false">完成</el-button>
          </template>
        </el-dialog>
      </div>`,
  };
})(window.AMS);

(function (AMS) {
  'use strict';
  const { ref, computed, onMounted } = Vue;

  // 管理员首页：数字概览 + 六张图表 + 待办快捷入口。
  AMS.pages.dashboard = {
    name: 'DashboardPage',
    setup() {
      const loading = ref(true);
      const ov = ref({});
      const charts = ref({});
      const pendingApps = ref([]);

      async function load() {
        loading.value = true;
        try {
          const [o, c, p] = await Promise.all([
            AMS.api.get('/stats/overview'),
            AMS.api.get('/stats/charts'),
            AMS.api.get('/applications', { status: 'pending', pageSize: 6 }),
          ]);
          ov.value = o;
          charts.value = c;
          pendingApps.value = p.list || [];
        } catch (e) {
          ElementPlus.ElMessage.error(e.message);
        } finally {
          loading.value = false;
        }
      }
      onMounted(load);

      const go = (hash) => { window.location.hash = hash; };

      // ----- 图表 option -----
      const pieOf = (data, title) => AMS.chartOption({
        title: { text: title, left: 'center', top: 6, textStyle: { fontSize: 13.5, color: '#5a6376' } },
        tooltip: { trigger: 'item', formatter: '{b}: {c} 台 ({d}%)' },
        legend: { type: 'scroll', orient: 'vertical', right: 6, top: 'middle', itemWidth: 9, itemHeight: 9, textStyle: { fontSize: 11.5 } },
        series: [{
          type: 'pie', radius: ['42%', '66%'], center: ['38%', '55%'],
          avoidLabelOverlap: true, itemStyle: { borderRadius: 5, borderColor: '#fff', borderWidth: 2 },
          label: { show: false }, labelLine: { show: false },
          data: data || [],
        }],
      });

      const categoryPie = computed(() => pieOf(charts.value.byCategory, '设备类型分布'));
      const statusPie = computed(() => pieOf(charts.value.byStatus, '设备状态分布'));

      const trendOption = computed(() => {
        const t = charts.value.monthlyTrend || { labels: [], borrow: [], returned: [], rejected: [] };
        return AMS.chartOption({
          title: { text: '近 12 个月流转趋势', left: 'center', top: 6, textStyle: { fontSize: 13.5, color: '#5a6376' } },
          legend: { bottom: 0, itemWidth: 12, itemHeight: 8, textStyle: { fontSize: 11.5 } },
          grid: { left: 12, right: 22, top: 46, bottom: 30, containLabel: true },
          xAxis: { type: 'category', data: t.labels, axisLabel: { fontSize: 11, rotate: 30 }, axisTick: { show: false } },
          yAxis: { type: 'value', minInterval: 1, splitLine: { lineStyle: { type: 'dashed', color: '#eef1f6' } } },
          series: [
            { name: '领用', type: 'line', smooth: true, symbolSize: 6, data: t.borrow, areaStyle: { opacity: .14 } },
            { name: '归还', type: 'line', smooth: true, symbolSize: 6, data: t.returned, areaStyle: { opacity: .14 } },
            { name: '驳回', type: 'line', smooth: true, symbolSize: 6, data: t.rejected },
          ],
        });
      });

      const deptOption = computed(() => {
        const d = (charts.value.byDept || []).slice(0, 10);
        return AMS.chartOption({
          title: { text: '各部门在用设备数', left: 'center', top: 6, textStyle: { fontSize: 13.5, color: '#5a6376' } },
          grid: { left: 12, right: 30, top: 46, bottom: 8, containLabel: true },
          xAxis: { type: 'value', minInterval: 1, splitLine: { lineStyle: { type: 'dashed', color: '#eef1f6' } } },
          yAxis: { type: 'category', data: d.map((x) => x.name).reverse(), axisTick: { show: false }, axisLabel: { fontSize: 11.5 } },
          series: [{
            type: 'bar', barMaxWidth: 18, data: d.map((x) => x.value).reverse(),
            itemStyle: { borderRadius: [0, 4, 4, 0], color: '#4a90e2' },
            label: { show: true, position: 'right', fontSize: 11, color: '#8a93a6' },
          }],
        });
      });

      const valueOption = computed(() => {
        const d = charts.value.valueByCategory || [];
        return AMS.chartOption({
          title: { text: '资产价值分布（元）', left: 'center', top: 6, textStyle: { fontSize: 13.5, color: '#5a6376' } },
          tooltip: { trigger: 'axis', valueFormatter: (v) => AMS.money(v) },
          grid: { left: 12, right: 18, top: 46, bottom: 30, containLabel: true },
          xAxis: { type: 'category', data: d.map((x) => x.name), axisLabel: { fontSize: 10.5, rotate: 34 }, axisTick: { show: false } },
          yAxis: { type: 'value', axisLabel: { formatter: (v) => v >= 10000 ? (v / 10000) + '万' : v }, splitLine: { lineStyle: { type: 'dashed', color: '#eef1f6' } } },
          series: [{ type: 'bar', barMaxWidth: 26, data: d.map((x) => x.value), itemStyle: { borderRadius: [4, 4, 0, 0], color: '#e09a2d' } }],
        });
      });

      const purchaseOption = computed(() => {
        const p = charts.value.purchaseByYear || { labels: [], counts: [], values: [] };
        return AMS.chartOption({
          title: { text: '历年采购数量与金额', left: 'center', top: 6, textStyle: { fontSize: 13.5, color: '#5a6376' } },
          legend: { bottom: 0, itemWidth: 12, itemHeight: 8, textStyle: { fontSize: 11.5 } },
          grid: { left: 12, right: 18, top: 46, bottom: 30, containLabel: true },
          xAxis: { type: 'category', data: p.labels, axisTick: { show: false } },
          yAxis: [
            { type: 'value', name: '台数', nameTextStyle: { fontSize: 11, color: '#a6adbb' }, minInterval: 1, splitLine: { lineStyle: { type: 'dashed', color: '#eef1f6' } } },
            { type: 'value', name: '金额(万)', nameTextStyle: { fontSize: 11, color: '#a6adbb' }, splitLine: { show: false } },
          ],
          series: [
            { name: '采购数量', type: 'bar', barMaxWidth: 24, data: p.counts, itemStyle: { borderRadius: [4, 4, 0, 0], color: '#52ac5e' } },
            { name: '采购金额', type: 'line', yAxisIndex: 1, smooth: true, symbolSize: 6, data: (p.values || []).map((v) => +(v / 10000).toFixed(1)), itemStyle: { color: '#e05c5c' } },
          ],
        });
      });

      const forecastOption = computed(() => {
        const f = charts.value.dueForecast || { labels: [], counts: [] };
        return AMS.chartOption({
          title: { text: '未来 8 周到期分布', left: 'center', top: 6, textStyle: { fontSize: 13.5, color: '#5a6376' } },
          grid: { left: 12, right: 18, top: 46, bottom: 28, containLabel: true },
          xAxis: { type: 'category', data: f.labels, axisLabel: { fontSize: 10.5, rotate: 30 }, axisTick: { show: false } },
          yAxis: { type: 'value', minInterval: 1, splitLine: { lineStyle: { type: 'dashed', color: '#eef1f6' } } },
          series: [{
            type: 'bar', barMaxWidth: 26, data: f.counts,
            // 首个柱子是"已逾期"，单独标红以示区别。
            itemStyle: {
              borderRadius: [4, 4, 0, 0],
              color: (p) => (p.dataIndex === 0 ? '#e05c5c' : '#4a90e2'),
            },
            label: { show: true, position: 'top', fontSize: 11, color: '#8a93a6' },
          }],
        });
      });

      return {
        loading, ov, charts, pendingApps, go, load,
        categoryPie, statusPie, trendOption, deptOption, valueOption, purchaseOption, forecastOption,
        moneyShort: AMS.moneyShort, fmtDateTime: AMS.fmtDateTime, pick: AMS.pick,
        applyType: AMS.applyType,
      };
    },
    template: `
      <div v-loading="loading">
        <div class="stat-grid">
          <stat-card label="设备总数" :value="ov.totalAssets || 0" unit="台" icon="📦" tone="primary"
                     :foot="'在用率 ' + (ov.usageRate || 0) + '%'" @click="go('#/assets')" />
          <stat-card label="领用中" :value="ov.inUse || 0" unit="台" icon="🙋" tone="success"
                     :foot="'占用资产 ' + moneyShort(ov.inUseValue) + ' 元'" @click="go('#/assets?quick=&status=in_use')" />
          <stat-card label="可领用库存" :value="ov.inStock || 0" unit="台" icon="🏷️" tone="info"
                     foot="当前在库可申请" @click="go('#/assets?status=in_stock')" />
          <stat-card label="即将到期" :value="ov.dueSoon || 0" unit="台" icon="⏰" tone="warning"
                     :foot="(ov.remindDays || 7) + ' 天内需归还'" @click="go('#/assets?quick=dueSoon')" />
          <stat-card label="已逾期" :value="ov.overdue || 0" unit="台" icon="🚨" tone="danger"
                     foot="需要重点跟进" @click="go('#/assets?quick=overdue')" />
          <stat-card label="待审批" :value="ov.pendingApply || 0" unit="单" icon="📝" tone="warning"
                     foot="等待您处理" @click="go('#/approvals')" />
          <stat-card label="维修中" :value="ov.repairing || 0" unit="台" icon="🔧" tone="warning"
                     :foot="'未完结工单 ' + (ov.openRepair || 0) + ' 个'" @click="go('#/repairs')" />
          <stat-card label="资产总值" :value="moneyShort(ov.totalValue)" unit="元" icon="💰" tone="primary"
                     foot="不含已报废设备" />
        </div>

        <div class="panel" v-if="pendingApps.length">
          <div class="panel-title">
            <span>待办事项</span>
            <span class="sub">{{ pendingApps.length }} 条申请等待审批</span>
            <span class="spacer"></span>
            <el-button link type="primary" @click="go('#/approvals')">前往审批中心</el-button>
          </div>
          <el-table :data="pendingApps" size="small" @row-click="() => go('#/approvals')" style="cursor:pointer">
            <el-table-column prop="applyNo" label="申请单号" width="150" />
            <el-table-column label="类型" width="80">
              <template #default="{ row }">
                <el-tag size="small" :type="pick(applyType, row.type).type">{{ pick(applyType, row.type).label }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="设备" min-width="180">
              <template #default="{ row }">{{ row.asset ? row.asset.name : '—' }}</template>
            </el-table-column>
            <el-table-column label="申请人" width="110">
              <template #default="{ row }">{{ row.applicant ? row.applicant.realName : '—' }}</template>
            </el-table-column>
            <el-table-column prop="reason" label="事由" min-width="200" show-overflow-tooltip />
            <el-table-column label="提交时间" width="145">
              <template #default="{ row }">{{ fmtDateTime(row.createdAt) }}</template>
            </el-table-column>
          </el-table>
        </div>

        <div class="chart-grid">
          <div class="panel mb0"><e-chart :option="categoryPie" /></div>
          <div class="panel mb0"><e-chart :option="statusPie" /></div>
          <div class="panel mb0" style="grid-column:1/-1"><e-chart :option="trendOption" height="320px" /></div>
          <div class="panel mb0"><e-chart :option="deptOption" /></div>
          <div class="panel mb0"><e-chart :option="forecastOption" /></div>
          <div class="panel mb0"><e-chart :option="valueOption" /></div>
          <div class="panel mb0"><e-chart :option="purchaseOption" /></div>
        </div>
      </div>`,
  };
})(window.AMS);

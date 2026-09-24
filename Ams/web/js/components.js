(function (AMS) {
  'use strict';
  const { ref, onMounted, onBeforeUnmount, watch, computed, nextTick } = Vue;

  // ---------- ECharts 包装组件 ----------
  // 负责实例生命周期、option 变化重绘、容器尺寸自适应（含侧边栏折叠）。
  AMS.EChart = {
    name: 'EChart',
    props: {
      option: { type: Object, required: true },
      height: { type: String, default: '300px' },
      loading: { type: Boolean, default: false },
    },
    setup(props) {
      const el = ref(null);
      let chart = null;
      let observer = null;

      const render = () => {
        if (!chart || !props.option) return;
        chart.setOption(props.option, true);
      };

      onMounted(() => {
        chart = echarts.init(el.value);
        render();
        // ResizeObserver 比 window.resize 更可靠：侧边栏折叠时容器宽度变化也能捕获。
        observer = new ResizeObserver(() => chart && chart.resize());
        observer.observe(el.value);
      });

      onBeforeUnmount(() => {
        if (observer) observer.disconnect();
        if (chart) chart.dispose();
        chart = null;
      });

      watch(() => props.option, render, { deep: true });
      watch(() => props.loading, (v) => {
        if (!chart) return;
        v ? chart.showLoading({ text: '加载中', color: '#4a90e2', textColor: '#8a93a6', maskColor: 'rgba(255,255,255,.7)' })
          : chart.hideLoading();
      });

      return { el };
    },
    template: `<div ref="el" class="chart-box" :style="{ height }"></div>`,
  };

  // ---------- 数字卡片 ----------
  AMS.StatCard = {
    name: 'StatCard',
    props: {
      label: String,
      value: [Number, String],
      unit: String,
      foot: String,
      icon: { type: String, default: '📦' },
      tone: { type: String, default: 'primary' },
    },
    template: `
      <div class="stat-card" :class="tone">
        <div class="deco">{{ icon }}</div>
        <div class="label">{{ label }}</div>
        <div class="value">{{ value }}<small v-if="unit">{{ unit }}</small></div>
        <div class="foot" v-if="foot">{{ foot }}</div>
      </div>`,
  };

  // ---------- 资产状态标签 ----------
  AMS.StatusTag = {
    name: 'StatusTag',
    props: { status: String, map: { type: String, default: 'assetStatus' } },
    setup(props) {
      const info = computed(() => AMS.pick(AMS[props.map] || {}, props.status));
      return { info };
    },
    template: `<el-tag :type="info.type" size="small" effect="light">{{ info.label }}</el-tag>`,
  };

  // ---------- 到期倒计时 ----------
  // 同一个字段在不同页面反复出现，抽成组件保证"剩 3 天 / 逾期 5 天"的口径一致。
  AMS.DueTag = {
    name: 'DueTag',
    props: { dueAt: String, status: String },
    setup(props) {
      const info = computed(() => {
        if (props.status !== 'in_use' || !props.dueAt) return null;
        const days = Math.ceil((new Date(props.dueAt) - Date.now()) / 86400000);
        if (days < 0) return { text: `已逾期 ${-days} 天`, type: 'danger' };
        if (days === 0) return { text: '今天到期', type: 'danger' };
        if (days <= 7) return { text: `剩 ${days} 天`, type: 'warning' };
        if (days <= 30) return { text: `剩 ${days} 天`, type: 'primary' };
        return { text: `剩 ${days} 天`, type: 'info' };
      });
      return { info, fmtDate: AMS.fmtDate };
    },
    template: `
      <span v-if="info" class="nowrap">
        {{ fmtDate(dueAt) }}
        <el-tag :type="info.type" size="small" effect="plain" style="margin-left:6px">{{ info.text }}</el-tag>
      </span>
      <span v-else class="text-muted">—</span>`,
  };

  // ---------- 空状态 ----------
  AMS.EmptyTip = {
    name: 'EmptyTip',
    props: { text: { type: String, default: '暂无数据' } },
    template: `<div class="empty-tip">{{ text }}</div>`,
  };

  // ---------- 资产详情抽屉 ----------
  // 设备基础信息 + 全生命周期履历 + 资产标签二维码，三合一。
  AMS.AssetDrawer = {
    name: 'AssetDrawer',
    components: { StatusTag: AMS.StatusTag, DueTag: AMS.DueTag, EmptyTip: AMS.EmptyTip },
    props: { modelValue: Boolean, assetId: Number },
    emits: ['update:modelValue'],
    setup(props, { emit }) {
      const asset = ref(null);
      const records = ref([]);
      const loading = ref(false);
      const tab = ref('info');

      const visible = computed({
        get: () => props.modelValue,
        set: (v) => emit('update:modelValue', v),
      });

      async function load() {
        if (!props.assetId) return;
        loading.value = true;
        try {
          const [a, r] = await Promise.all([
            AMS.api.get('/assets/' + props.assetId),
            AMS.api.get(`/assets/${props.assetId}/records`),
          ]);
          asset.value = a;
          records.value = r || [];
        } catch (e) {
          ElementPlus.ElMessage.error(e.message);
        } finally {
          loading.value = false;
        }
      }

      watch(() => [props.modelValue, props.assetId], ([open]) => {
        if (open) { tab.value = 'info'; load(); }
      });

      const actionLabel = {
        created: '入库登记', apply: '提交申请', borrow: '领用', return: '归还',
        renew: '续借', scrap: '报废', repair: '维修', update: '信息变更',
        import: '批量导入', reject: '审批驳回', cancel: '撤销申请',
        transfer: '调拨', instock: '回库',
      };
      const actionColor = {
        created: '#5a8fd6', borrow: '#4a90e2', return: '#52ac5e', renew: '#e09a2d',
        scrap: '#6c7a94', repair: '#e8744f', reject: '#e05c5c', update: '#8f6ed5',
      };

      return {
        visible, asset, records, loading, tab, actionLabel, actionColor,
        fmtDate: AMS.fmtDate, fmtDateTime: AMS.fmtDateTime, money: AMS.money,
        qrURL: AMS.api.qrURL,
        printQR() { window.open(AMS.api.qrURL(props.assetId), '_blank'); },
      };
    },
    template: `
      <el-drawer v-model="visible" size="620px" :title="asset ? asset.name : '设备详情'">
        <div v-loading="loading">
          <template v-if="asset">
            <div style="display:flex;align-items:center;gap:10px;margin-bottom:12px">
              <span style="font-size:17px;font-weight:600">{{ asset.name }}</span>
              <status-tag :status="asset.status" />
              <el-tag size="small" type="info" effect="plain">{{ asset.assetNo }}</el-tag>
            </div>

            <el-tabs v-model="tab">
              <el-tab-pane label="基础信息" name="info">
                <div class="detail-grid">
                  <div class="detail-item"><span class="k">设备类型</span><span class="v">{{ asset.category ? asset.category.name : '—' }}</span></div>
                  <div class="detail-item"><span class="k">品牌型号</span><span class="v">{{ asset.brand }} {{ asset.model }}</span></div>
                  <div class="detail-item"><span class="k">序列号</span><span class="v">{{ asset.serialNo || '—' }}</span></div>
                  <div class="detail-item"><span class="k">存放位置</span><span class="v">{{ asset.location || '—' }}</span></div>
                  <div class="detail-item"><span class="k">采购价格</span><span class="v">{{ money(asset.price) }}</span></div>
                  <div class="detail-item"><span class="k">采购日期</span><span class="v">{{ fmtDate(asset.purchaseDate) }}</span></div>
                  <div class="detail-item"><span class="k">保修到期</span><span class="v">{{ fmtDate(asset.warrantyUntil) }}</span></div>
                  <div class="detail-item"><span class="k">当前领用人</span><span class="v">{{ asset.holder ? asset.holder.realName : '—' }}</span></div>
                  <div class="detail-item"><span class="k">领用部门</span><span class="v">{{ asset.holderDept ? asset.holderDept.name : '—' }}</span></div>
                  <div class="detail-item"><span class="k">领用时间</span><span class="v">{{ fmtDate(asset.borrowedAt) }}</span></div>
                  <div class="detail-item" style="grid-column:1/-1">
                    <span class="k">应归还</span><span class="v"><due-tag :due-at="asset.dueAt" :status="asset.status" /></span>
                  </div>
                  <div class="detail-item" style="grid-column:1/-1"><span class="k">规格配置</span><span class="v">{{ asset.specification || '—' }}</span></div>
                  <div class="detail-item" style="grid-column:1/-1"><span class="k">备注</span><span class="v">{{ asset.remark || '—' }}</span></div>
                </div>
              </el-tab-pane>

              <el-tab-pane :label="'资产履历 (' + records.length + ')'" name="records">
                <empty-tip v-if="!records.length" text="暂无履历记录" />
                <el-timeline v-else style="padding-left:4px">
                  <el-timeline-item
                    v-for="r in records" :key="r.id"
                    :timestamp="fmtDateTime(r.createdAt)"
                    :color="actionColor[r.action] || '#909399'"
                    placement="top">
                    <div style="font-weight:600;font-size:13px">
                      {{ actionLabel[r.action] || r.action }}
                      <span class="text-muted" style="font-weight:400;margin-left:8px">{{ r.operator }}</span>
                    </div>
                    <div class="timeline-detail">{{ r.detail }}</div>
                  </el-timeline-item>
                </el-timeline>
              </el-tab-pane>

              <el-tab-pane label="资产标签" name="qr">
                <div class="qr-box">
                  <img :src="qrURL(asset.id)" alt="资产二维码">
                  <div class="cap">{{ asset.assetNo }} · {{ asset.name }}</div>
                  <div class="cap">打印后贴于设备表面，盘点时扫码即可定位该设备</div>
                  <el-button size="small" style="margin-top:12px" @click="printQR">在新窗口打开 / 打印</el-button>
                </div>
              </el-tab-pane>
            </el-tabs>
          </template>
        </div>
      </el-drawer>`,
  };

  // 统一注册到 Vue 应用。
  AMS.registerComponents = function (app) {
    app.component('EChart', AMS.EChart);
    app.component('StatCard', AMS.StatCard);
    app.component('StatusTag', AMS.StatusTag);
    app.component('DueTag', AMS.DueTag);
    app.component('EmptyTip', AMS.EmptyTip);
    app.component('AssetDrawer', AMS.AssetDrawer);
  };
})(window.AMS);

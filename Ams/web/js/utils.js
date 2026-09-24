// 全局命名空间。前端不走打包，各脚本按顺序往这里挂东西。
window.AMS = window.AMS || { pages: {} };

(function (AMS) {
  'use strict';

  const pad = (n) => String(n).padStart(2, '0');

  // 后端返回的是 RFC3339 时间串，这里统一成中文习惯的展示格式。
  AMS.fmtDate = function (v) {
    if (!v) return '—';
    const d = new Date(v);
    if (isNaN(d)) return '—';
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  };

  AMS.fmtDateTime = function (v) {
    if (!v) return '—';
    const d = new Date(v);
    if (isNaN(d)) return '—';
    return `${AMS.fmtDate(v)} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
  };

  // 距今多久，用于消息列表这种"看个大概"的场景。
  AMS.fromNow = function (v) {
    if (!v) return '';
    const diff = Date.now() - new Date(v).getTime();
    const min = Math.floor(diff / 60000);
    if (min < 1) return '刚刚';
    if (min < 60) return `${min} 分钟前`;
    const h = Math.floor(min / 60);
    if (h < 24) return `${h} 小时前`;
    const d = Math.floor(h / 24);
    if (d < 30) return `${d} 天前`;
    return AMS.fmtDate(v);
  };

  AMS.money = function (n) {
    if (n == null) return '—';
    return '¥' + Number(n).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  };

  // 金额超过一万时折算成"万元"，看板卡片上不至于挤成一行数字。
  AMS.moneyShort = function (n) {
    const v = Number(n || 0);
    if (v >= 10000) return (v / 10000).toFixed(1) + ' 万';
    return v.toFixed(0);
  };

  AMS.assetStatus = {
    in_stock: { label: '在库', type: 'success' },
    in_use: { label: '领用中', type: 'primary' },
    repairing: { label: '维修中', type: 'warning' },
    scrapped: { label: '已报废', type: 'info' },
    lost: { label: '遗失', type: 'danger' },
  };

  AMS.applyType = {
    borrow: { label: '领用', type: 'primary' },
    return: { label: '归还', type: 'success' },
    renew: { label: '续借', type: 'warning' },
    scrap: { label: '报废', type: 'info' },
  };

  AMS.applyStatus = {
    pending: { label: '待审批', type: 'warning' },
    approved: { label: '已通过', type: 'success' },
    rejected: { label: '已驳回', type: 'danger' },
    cancelled: { label: '已撤销', type: 'info' },
  };

  AMS.repairStatus = {
    open: { label: '待处理', type: 'danger' },
    processing: { label: '处理中', type: 'warning' },
    resolved: { label: '已修复', type: 'success' },
    closed: { label: '已关闭', type: 'info' },
  };

  AMS.noticeStyle = {
    approval: { icon: '📝', bg: '#e8f1ff', color: '#3a7bd5' },
    due: { icon: '⏰', bg: '#fff6e6', color: '#e09a2d' },
    overdue: { icon: '🚨', bg: '#ffeded', color: '#e05c5c' },
    repair: { icon: '🔧', bg: '#fff6e6', color: '#e09a2d' },
    system: { icon: '📢', bg: '#eef1f6', color: '#6c7a94' },
  };

  AMS.pick = function (map, key, fallback) {
    return map[key] || fallback || { label: key || '—', type: 'info' };
  };

  // 把 el-date-picker 的区间值拆成两个查询参数。
  AMS.applyRange = function (params, range, fromKey, toKey) {
    if (Array.isArray(range) && range.length === 2 && range[0] && range[1]) {
      params[fromKey] = range[0];
      params[toKey] = range[1];
    }
  };

  // ECharts 统一配色，保证各图表观感一致。
  AMS.palette = ['#4a90e2', '#52ac5e', '#e09a2d', '#e05c5c', '#8f6ed5', '#3fbfc0',
    '#e8744f', '#5a8fd6', '#9bbf3f', '#c264a8', '#6c7a94', '#d4a03f'];

  AMS.chartBase = {
    color: AMS.palette,
    textStyle: { fontFamily: '-apple-system, "Microsoft YaHei", sans-serif' },
    grid: { left: 12, right: 18, top: 42, bottom: 8, containLabel: true },
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
  };

  // 深拷贝后合并，避免多个图表共享同一个 option 对象。
  AMS.chartOption = function (extra) {
    return Object.assign(JSON.parse(JSON.stringify(AMS.chartBase)), extra);
  };
})(window.AMS);

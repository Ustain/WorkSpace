(function (AMS) {
  'use strict';
  const { ref, reactive, onMounted } = Vue;
  const { ElMessage } = ElementPlus;

  // 消息中心：管理员与普通用户共用，区别只在于收到的消息类型不同。
  AMS.pages.notifications = {
    name: 'NotificationsPage',
    setup() {
      const loading = ref(false);
      const list = ref([]);
      const total = ref(0);
      const filters = reactive({ unread: '', type: [] });
      const page = reactive({ page: 1, pageSize: 20 });

      async function load() {
        loading.value = true;
        try {
          const data = await AMS.api.get('/notifications', {
            unread: filters.unread, type: filters.type,
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

      function refreshBadge() {
        window.dispatchEvent(new CustomEvent('ams:refresh-badge'));
      }

      async function markRead(n) {
        if (n.isRead) return;
        try {
          await AMS.api.post(`/notifications/${n.id}/read`);
          n.isRead = true;
          refreshBadge();
        } catch (e) {
          ElMessage.error(e.message);
        }
      }

      async function markAll() {
        try {
          await AMS.api.post('/notifications/read-all');
          ElMessage.success('全部消息已标记为已读');
          load();
          refreshBadge();
        } catch (e) {
          ElMessage.error(e.message);
        }
      }

      async function remove(n) {
        try {
          await AMS.api.del('/notifications/' + n.id);
          load();
          refreshBadge();
        } catch (e) {
          ElMessage.error(e.message);
        }
      }

      const typeLabels = {
        approval: '审批通知', due: '到期提醒', overdue: '逾期告警',
        repair: '报修工单', system: '系统消息',
      };

      return {
        loading, list, total, filters, page, load, markRead, markAll, remove, typeLabels,
        style: AMS.noticeStyle, fromNow: AMS.fromNow, fmtDateTime: AMS.fmtDateTime,
        search() { page.page = 1; load(); },
      };
    },
    template: `
      <div>
        <div class="panel">
          <div class="filter-bar">
            <el-radio-group v-model="filters.unread" size="small" @change="search">
              <el-radio-button value="">全部消息</el-radio-button>
              <el-radio-button value="1">仅未读</el-radio-button>
            </el-radio-group>
            <el-select v-model="filters.type" multiple collapse-tags placeholder="消息类型" clearable
                       style="width:250px" @change="search">
              <el-option v-for="(v, k) in typeLabels" :key="k" :label="v" :value="k" />
            </el-select>
            <span style="flex:1"></span>
            <el-button size="small" @click="markAll"><el-icon><Check /></el-icon>全部标记为已读</el-button>
          </div>
        </div>

        <div class="panel" v-loading="loading">
          <empty-tip v-if="!list.length" text="暂无消息" />
          <div v-for="n in list" :key="n.id" class="notice-item" :class="{ unread: !n.isRead }" @click="markRead(n)">
            <div class="icon" :style="{ background: (style[n.type] || style.system).bg }">
              {{ (style[n.type] || style.system).icon }}
            </div>
            <div class="body">
              <div class="t">
                {{ n.title }}
                <el-tag size="small" effect="plain" type="info">{{ typeLabels[n.type] || n.type }}</el-tag>
                <el-badge v-if="!n.isRead" is-dot />
              </div>
              <div class="c">{{ n.content }}</div>
              <div class="time">{{ fromNow(n.createdAt) }} · {{ fmtDateTime(n.createdAt) }}</div>
            </div>
            <div>
              <el-button link type="danger" size="small" @click.stop="remove(n)">删除</el-button>
            </div>
          </div>

          <div class="pager" v-if="total > page.pageSize">
            <el-pagination background layout="total, prev, pager, next" :total="total"
                           v-model:current-page="page.page" :page-size="page.pageSize" @current-change="load" />
          </div>
        </div>
      </div>`,
  };
})(window.AMS);

(function (AMS) {
  'use strict';
  const { createApp, ref, reactive, computed, onMounted, onBeforeUnmount, watch, h } = Vue;
  const { ElMessage } = ElementPlus;

  // ---------- 极简 hash 路由 ----------
  // 只有十几个页面，用不上 vue-router；解析 "#/assets?quick=overdue" 即可。
  function parseHash() {
    const raw = (window.location.hash || '').replace(/^#\/?/, '');
    const [path, search] = raw.split('?');
    const query = {};
    new URLSearchParams(search || '').forEach((v, k) => { query[k] = v; });
    return { path: path || '', query };
  }

  // 菜单定义。perm 为空表示登录即可见；adminOnly 用来把"管理视角"的项藏起来。
  const MENUS = [
    {
      group: '管理视角', perm: 'apply:approve',
      items: [
        { path: 'dashboard', label: '统计看板', icon: 'DataLine', perm: 'stat:view' },
        { path: 'assets', label: '设备台账', icon: 'Box', perm: 'asset:view' },
        { path: 'approvals', label: '审批中心', icon: 'Stamp', perm: 'apply:approve', badge: 'pendingApply' },
        { path: 'repairs', label: '报修工单', icon: 'Tools', perm: 'repair:handle', badge: 'openRepair' },
      ],
    },
    {
      group: '我的工作台',
      items: [
        { path: 'home', label: '工作台', icon: 'HomeFilled', hideForAdmin: true },
        { path: 'available', label: '可领用设备', icon: 'Goods', perm: 'apply:create' },
        { path: 'my-assets', label: '我的设备', icon: 'Briefcase' },
        { path: 'my-applications', label: '我的申请', icon: 'Document' },
        { path: 'my-repairs', label: '我的报修', icon: 'Warning', hideForAdmin: true },
        { path: 'notifications', label: '消息中心', icon: 'Bell', badge: 'unread' },
      ],
    },
    {
      group: '基础数据', perm: 'asset:edit',
      items: [
        { path: 'categories', label: '设备类型', icon: 'Files', perm: 'asset:edit' },
        { path: 'departments', label: '部门管理', icon: 'OfficeBuilding', perm: 'dept:manage' },
      ],
    },
    {
      group: '系统管理', perm: 'user:manage',
      items: [
        { path: 'users', label: '用户管理', icon: 'User', perm: 'user:manage' },
        { path: 'roles', label: '角色权限', icon: 'Key', perm: 'role:manage' },
        { path: 'settings', label: '系统设置', icon: 'Setting', perm: 'setting:manage' },
        { path: 'emails', label: '邮件记录', icon: 'Message', perm: 'setting:manage' },
        { path: 'audit', label: '审计日志', icon: 'Tickets', perm: 'audit:view' },
      ],
    },
  ];

  const TITLES = {
    dashboard: '统计看板', assets: '设备台账', approvals: '审批中心', repairs: '报修工单',
    home: '我的工作台', available: '可领用设备', 'my-assets': '我的设备',
    'my-applications': '我的申请', 'my-repairs': '我的报修', notifications: '消息中心',
    categories: '设备类型', departments: '部门管理', users: '用户管理', roles: '角色权限',
    settings: '系统设置', emails: '邮件记录', audit: '审计日志', profile: '个人中心',
  };

  // ---------- 登录页 ----------
  const LoginPage = {
    name: 'LoginPage',
    emits: ['logged-in'],
    setup(props, { emit }) {
      const meta = ref({ systemName: 'IT 资产管理系统', orgName: '', demoAccounts: [] });
      const form = reactive({ username: '', password: '' });
      const loading = ref(false);

      onMounted(async () => {
        try { meta.value = await AMS.api.get('/meta'); } catch (e) { /* 用默认文案兜底 */ }
      });

      async function submit() {
        if (!form.username || !form.password) return ElMessage.warning('请输入用户名和密码');
        loading.value = true;
        try {
          const data = await AMS.api.post('/auth/login', form);
          AMS.api.store.token = data.token;
          ElMessage.success(`欢迎回来，${data.user.realName || data.user.username}`);
          emit('logged-in', data.user);
        } catch (e) {
          ElMessage.error(e.message);
        } finally {
          loading.value = false;
        }
      }

      function fill(acc) {
        form.username = acc.username;
        form.password = acc.password;
        submit();
      }

      const features = [
        ['📦', '设备全生命周期台账：入库、领用、归还、续借、报修、报废，每一步都留痕可追溯'],
        ['📝', '领用审批闭环：申请、审批、驳回、撤销，审批结果自动变更设备状态'],
        ['⏰', '到期自动提醒：管理员站内信汇总，领用人站内信 + 邮件双通道'],
        ['📊', '数据看板：类型 / 状态 / 部门 / 价值 / 趋势 / 到期预测多维图表'],
        ['📤', '批量导入导出：Excel 与 CSV 双格式，导入逐行校验并给出错误报告'],
      ];

      return { meta, form, loading, submit, fill, features };
    },
    template: `
      <div class="login-page">
        <div class="login-card">
          <div class="login-intro">
            <h1>{{ meta.systemName }}</h1>
            <div class="sub">{{ meta.orgName }}</div>
            <div v-for="f in features" :key="f[1]" class="login-feature">
              <div class="dot">{{ f[0] }}</div>
              <div class="txt">{{ f[1] }}</div>
            </div>
          </div>

          <div class="login-form">
            <h2>账号登录</h2>
            <div class="tip">请使用单位分配的账号登录系统</div>
            <el-form @submit.prevent="submit">
              <el-form-item>
                <el-input v-model="form.username" size="large" placeholder="用户名" clearable>
                  <template #prefix><el-icon><User /></el-icon></template>
                </el-input>
              </el-form-item>
              <el-form-item>
                <el-input v-model="form.password" size="large" type="password" placeholder="密码"
                          show-password @keyup.enter="submit">
                  <template #prefix><el-icon><Lock /></el-icon></template>
                </el-input>
              </el-form-item>
              <el-button type="primary" size="large" style="width:100%" :loading="loading" @click="submit">
                登 录
              </el-button>
            </el-form>

            <div class="demo-accounts" v-if="meta.demoAccounts && meta.demoAccounts.length">
              <div class="label">演示账号（点击即可直接登录体验不同角色视图）</div>
              <div v-for="a in meta.demoAccounts" :key="a.username" class="demo-account" @click="fill(a)">
                <el-tag size="small" :type="a.username === 'admin' ? 'danger' : (a.username === 'assetmgr' ? 'warning' : 'info')">
                  {{ a.role }}
                </el-tag>
                <div style="flex:1;min-width:0">
                  <div class="who">{{ a.username }} / {{ a.password }}</div>
                  <div class="desc">{{ a.desc }}</div>
                </div>
                <el-icon><ArrowRight /></el-icon>
              </div>
            </div>
          </div>
        </div>
      </div>`,
  };

  // ---------- 主布局 ----------
  const Layout = {
    name: 'AppLayout',
    props: { user: Object },
    emits: ['logout', 'refresh-user'],
    setup(props, { emit }) {
      const route = reactive(parseHash());
      const collapsed = ref(false);
      const badges = reactive({ unread: 0, pendingApply: 0, openRepair: 0 });
      let timer = null;

      const isAdmin = computed(() => (props.user.permissions || []).includes('apply:approve'));
      const has = (perm) => !perm || (props.user.permissions || []).includes(perm);

      // 可见菜单 = 权限过滤 + 管理员/普通用户视图差异。
      const menus = computed(() => MENUS
        .filter((g) => has(g.perm))
        .map((g) => ({
          group: g.group,
          items: g.items.filter((i) => has(i.perm) && !(i.hideForAdmin && isAdmin.value)),
        }))
        .filter((g) => g.items.length));

      const defaultPath = computed(() => (isAdmin.value ? 'dashboard' : 'home'));

      function syncRoute() {
        const r = parseHash();
        if (!r.path) {
          window.location.hash = '#/' + defaultPath.value;
          return;
        }
        route.path = r.path;
        route.query = r.query;
      }

      // 当前路由对应的页面组件；'my-repairs' 复用报修页的个人模式。
      const currentPage = computed(() => {
        if (route.path === 'my-repairs') return AMS.pages.repairs;
        return AMS.pages[route.path] || null;
      });

      const pageProps = computed(() => {
        const p = { user: props.user, query: route.query };
        if (route.path === 'my-repairs') p.mine = true;
        return p;
      });

      const title = computed(() => TITLES[route.path] || '资产管理');

      async function loadBadges() {
        try {
          const n = await AMS.api.get('/notifications/unread-count');
          badges.unread = n.total || 0;
        } catch (e) { /* 忽略轮询失败 */ }
        if (!isAdmin.value) return;
        try {
          const ov = await AMS.api.get('/stats/overview');
          badges.pendingApply = ov.pendingApply || 0;
          badges.openRepair = ov.openRepair || 0;
        } catch (e) { /* 忽略 */ }
      }

      onMounted(() => {
        syncRoute();
        loadBadges();
        // 轮询未读数，让站内信红点自己动起来（演示定时提醒效果时很直观）。
        timer = setInterval(loadBadges, 30000);
        window.addEventListener('hashchange', syncRoute);
        window.addEventListener('ams:refresh-badge', loadBadges);
      });

      onBeforeUnmount(() => {
        clearInterval(timer);
        window.removeEventListener('hashchange', syncRoute);
        window.removeEventListener('ams:refresh-badge', loadBadges);
      });

      // 切页面时刷新一次角标，保证审批完待办数立刻变化。
      watch(() => route.path, loadBadges);

      function nav(path) {
        if (route.path === path) return;
        window.location.hash = '#/' + path;
      }

      async function logout() {
        try { await AMS.api.post('/auth/logout', {}); } catch (e) { /* 忽略 */ }
        emit('logout');
      }

      return {
        route, collapsed, badges, menus, isAdmin, currentPage, pageProps, title, nav, logout,
        onCommand(cmd) {
          if (cmd === 'logout') logout();
          else nav(cmd);
        },
      };
    },
    template: `
      <div class="ams-layout">
        <aside class="ams-sidebar" :class="{ collapsed }">
          <div class="ams-brand">
            <span class="logo">📦</span>
            <span v-show="!collapsed">资产管理系统</span>
          </div>
          <nav class="ams-menu">
            <template v-for="g in menus" :key="g.group">
              <div class="menu-group" v-show="!collapsed">{{ g.group }}</div>
              <div v-for="i in g.items" :key="i.path"
                   class="menu-item" :class="{ active: route.path === i.path }"
                   :title="i.label" @click="nav(i.path)">
                <el-icon><component :is="i.icon" /></el-icon>
                <span v-show="!collapsed">{{ i.label }}</span>
                <span v-if="i.badge && badges[i.badge] && !collapsed" class="badge">{{ badges[i.badge] }}</span>
              </div>
            </template>
          </nav>
        </aside>

        <div class="ams-main">
          <header class="ams-header">
            <div class="header-action" @click="collapsed = !collapsed">
              <el-icon size="18"><component :is="collapsed ? 'Expand' : 'Fold'" /></el-icon>
            </div>
            <div class="title">{{ title }}</div>
            <div class="spacer"></div>

            <el-tag v-if="isAdmin" size="small" type="danger" effect="plain">管理员视图</el-tag>
            <el-tag v-else size="small" type="info" effect="plain">普通用户视图</el-tag>

            <el-badge :value="badges.unread" :hidden="!badges.unread" :max="99">
              <div class="header-action" @click="nav('notifications')" title="消息中心">
                <el-icon size="18"><Bell /></el-icon>
              </div>
            </el-badge>

            <el-dropdown @command="onCommand">
              <div class="header-action" style="gap:7px">
                <el-avatar :size="28" style="background:#4a90e2;font-size:13px">
                  {{ (user.realName || user.username).slice(0, 1) }}
                </el-avatar>
                <span style="font-size:13.5px;color:#2c3348">{{ user.realName || user.username }}</span>
                <el-icon><ArrowDown /></el-icon>
              </div>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item disabled>
                    {{ user.roleName }} · {{ user.departmentName || '未分配部门' }}
                  </el-dropdown-item>
                  <el-dropdown-item command="profile" divided>
                    <el-icon><User /></el-icon>个人中心
                  </el-dropdown-item>
                  <el-dropdown-item command="notifications">
                    <el-icon><Bell /></el-icon>消息中心
                  </el-dropdown-item>
                  <el-dropdown-item command="logout" divided>
                    <el-icon><SwitchButton /></el-icon>退出登录
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </header>

          <main class="ams-content">
            <component v-if="currentPage" :is="currentPage" v-bind="pageProps"
                       :key="route.path" @refresh-user="$emit('refresh-user')" />
            <div v-else class="panel">
              <empty-tip text="页面不存在，请从左侧菜单选择功能" />
            </div>
          </main>
        </div>
      </div>`,
  };

  // ---------- 根组件 ----------
  const Root = {
    name: 'AmsApp',
    components: { LoginPage, AppLayout: Layout },
    setup() {
      const user = ref(null);
      const booting = ref(true);

      async function fetchProfile() {
        if (!AMS.api.store.token) { booting.value = false; return; }
        try {
          user.value = await AMS.api.get('/auth/profile');
        } catch (e) {
          AMS.api.store.token = '';
          user.value = null;
        } finally {
          booting.value = false;
        }
      }

      function onLoggedIn(u) {
        user.value = u;
        window.location.hash = '#/' + (u.permissions.includes('apply:approve') ? 'dashboard' : 'home');
      }

      function logout() {
        AMS.api.store.token = '';
        user.value = null;
        window.location.hash = '';
      }

      onMounted(() => {
        fetchProfile();
        // token 失效时由 api 层广播，这里统一回到登录页。
        window.addEventListener('ams:unauthorized', logout);
      });

      return { user, booting, onLoggedIn, logout, fetchProfile };
    },
    template: `
      <div style="height:100%">
        <div v-if="booting" class="boot-screen">
          <div class="boot-logo">📦</div>
          <div class="boot-text">资产管理系统加载中…</div>
        </div>
        <login-page v-else-if="!user" @logged-in="onLoggedIn" />
        <app-layout v-else :user="user" @logout="logout" @refresh-user="fetchProfile" />
      </div>`,
  };

  const app = createApp(Root);
  app.use(ElementPlus, { locale: ElementPlusLocaleZhCn });
  // 图标以全局组件形式注册，模板里可以直接 <el-icon><Search /></el-icon>。
  Object.entries(ElementPlusIconsVue).forEach(([name, comp]) => app.component(name, comp));
  AMS.registerComponents(app);
  app.mount('#app');
})(window.AMS);

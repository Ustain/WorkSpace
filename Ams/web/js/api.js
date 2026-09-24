(function (AMS) {
  'use strict';

  const TOKEN_KEY = 'ams.token';

  const store = {
    get token() { return localStorage.getItem(TOKEN_KEY) || ''; },
    set token(v) { v ? localStorage.setItem(TOKEN_KEY, v) : localStorage.removeItem(TOKEN_KEY); },
  };

  function qs(params) {
    if (!params) return '';
    const sp = new URLSearchParams();
    Object.entries(params).forEach(([k, v]) => {
      if (v === undefined || v === null || v === '') return;
      // 数组参数展开成重复 key，后端用 QueryArray 接收。
      Array.isArray(v) ? v.forEach((x) => x !== '' && sp.append(k, x)) : sp.append(k, v);
    });
    const s = sp.toString();
    return s ? '?' + s : '';
  }

  async function request(method, url, { body, params, raw } = {}) {
    const headers = {};
    if (store.token) headers.Authorization = 'Bearer ' + store.token;

    const init = { method, headers };
    if (body instanceof FormData) {
      init.body = body;
    } else if (body !== undefined) {
      headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(body);
    }

    const resp = await fetch('/api' + url + qs(params), init);

    // 401 统一踢回登录页，由 app.js 监听该事件。
    if (resp.status === 401) {
      store.token = '';
      window.dispatchEvent(new CustomEvent('ams:unauthorized'));
      throw new Error('登录已失效，请重新登录');
    }
    if (raw) {
      if (!resp.ok) throw new Error('请求失败');
      return resp.blob();
    }

    let payload;
    try {
      payload = await resp.json();
    } catch (e) {
      throw new Error('服务返回异常');
    }
    if (!resp.ok || payload.code !== 0) {
      throw new Error(payload.message || '请求失败');
    }
    return payload.data;
  }

  AMS.api = {
    store,
    get: (url, params) => request('GET', url, { params }),
    post: (url, body, params) => request('POST', url, { body, params }),
    put: (url, body) => request('PUT', url, { body }),
    del: (url) => request('DELETE', url),

    // 下载类接口走浏览器原生下载：拼上 token 后直接跳转，省去 blob 中转。
    download(url, params) {
      const p = Object.assign({}, params, { token: store.token });
      const a = document.createElement('a');
      a.href = '/api' + url + qs(p);
      a.rel = 'noopener';
      document.body.appendChild(a);
      a.click();
      a.remove();
    },

    upload(url, file) {
      const fd = new FormData();
      fd.append('file', file);
      return request('POST', url, { body: fd });
    },

    qrURL(id) {
      return `/api/assets/${id}/qrcode?token=${encodeURIComponent(store.token)}`;
    },
  };
})(window.AMS);

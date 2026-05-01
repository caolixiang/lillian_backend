package httpapi

import "net/http"

func (s *Server) handleAdminPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte(adminPageHTML()))
}

func (s *Server) handleIcon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte(lillianIconSVG))
}

const lillianIconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="16" fill="#f8f5ff"/><path d="M18 44c8-2 13-8 14-20 1 12 6 18 14 20-7 5-21 5-28 0Z" fill="#8f7af4"/><path d="M32 12c4 6 5 13 0 21-5-8-4-15 0-21Z" fill="#5849bf"/><path d="M21 24c7 0 11 4 11 11-8-1-12-5-11-11Z" fill="#d9b7f5"/><path d="M43 24c-7 0-11 4-11 11 8-1 12-5 11-11Z" fill="#b7d7f5"/><circle cx="32" cy="38" r="4" fill="#26233f"/></svg>`

func adminPageHTML() string {
	return `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>莉莉安的后台 | Lillian's Canvas Admin</title>
  <link rel="icon" href="/lillian-icon.svg" type="image/svg+xml">
  <link rel="apple-touch-icon" href="/lillian-icon.svg">
  <style>
    :root {
      color-scheme: light;
      --bg: #f6f5f2;
      --panel: #fff;
      --soft: #faf9f6;
      --line: #dddbd3;
      --text: #202124;
      --muted: #6f706c;
      --strong: #111;
      --ok: #166534;
      --warn: #92400e;
      --bad: #b91c1c;
      --focus: #806ce8;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-width: 320px;
      background: var(--bg);
      color: var(--text);
      font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      font-size: 14px;
      line-height: 1.5;
    }
    button, input, select { font: inherit; }
    button {
      min-height: 34px;
      border: 1px solid var(--line);
      border-radius: 7px;
      background: #fff;
      color: var(--strong);
      padding: 0 12px;
      cursor: pointer;
      white-space: nowrap;
    }
    button:hover { border-color: #b9b7ad; background: #f8f7f3; }
    button:disabled { opacity: .55; cursor: not-allowed; }
    button.primary { background: #222; border-color: #222; color: #fff; }
    button.primary:hover { background: #000; }
    button.danger { color: var(--bad); }
    button.small { min-height: 28px; padding: 0 9px; font-size: 12px; }
    input, select {
      width: 100%;
      min-height: 34px;
      border: 1px solid var(--line);
      border-radius: 7px;
      background: #fff;
      color: var(--text);
      padding: 0 10px;
    }
    button:focus-visible, input:focus-visible, select:focus-visible {
      outline: 2px solid var(--focus);
      outline-offset: 1px;
    }
    label { display: grid; gap: 5px; color: var(--muted); font-size: 12px; font-weight: 650; }
    .shell { max-width: 1260px; margin: 0 auto; padding: 24px; }
    header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      margin-bottom: 18px;
    }
    .brand { display: flex; align-items: center; gap: 12px; min-width: 0; }
    .mark {
      width: 42px;
      height: 42px;
      border: 1px solid var(--line);
      border-radius: 10px;
      background: #fff;
      display: block;
      object-fit: cover;
      box-shadow: 0 6px 18px rgba(35, 35, 30, .07);
    }
    h1 { margin: 0; font-size: 20px; line-height: 1.1; letter-spacing: 0; }
    h2 { margin: 0; font-size: 14px; letter-spacing: 0; }
    .subtitle { margin-top: 3px; color: var(--muted); font-size: 12px; }
    .header-actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; justify-content: flex-end; }
    .status {
      display: inline-flex;
      align-items: center;
      gap: 7px;
      min-height: 30px;
      border: 1px solid var(--line);
      border-radius: 999px;
      background: #fff;
      color: var(--muted);
      padding: 0 10px;
      white-space: nowrap;
    }
    .dot { width: 8px; height: 8px; border-radius: 50%; background: #a3a3a3; }
    .status.ok .dot { background: var(--ok); }
    .status.bad .dot { background: var(--bad); }
    section {
      overflow: hidden;
      border: 1px solid var(--line);
      border-radius: 8px;
      background: var(--panel);
    }
    .section-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      border-bottom: 1px solid var(--line);
      background: var(--soft);
      padding: 13px 14px;
    }
    .body { padding: 14px; }
    .form { display: grid; gap: 10px; }
    .columns { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
    .actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
    .section-actions { display: flex; align-items: center; justify-content: flex-end; gap: 8px; flex-wrap: wrap; }
    .form-delete { margin-left: auto; }
    .hint { margin: 0; color: var(--muted); font-size: 12px; }
    .message { min-height: 20px; color: var(--muted); font-size: 12px; white-space: pre-wrap; }
    .message:empty { display: none; }
    .message.ok { color: var(--ok); }
    .message.bad { color: var(--bad); }
    .login-panel { max-width: 420px; margin: 72px auto 0; }
    .login-card { box-shadow: 0 18px 45px rgba(35, 35, 30, .08); }
    .grid { display: grid; grid-template-columns: 360px minmax(0, 1fr); gap: 16px; align-items: start; }
    .stack { display: grid; gap: 16px; }
    .tabs { display: flex; gap: 6px; flex-wrap: wrap; }
    .tab { background: transparent; border-color: transparent; color: var(--muted); }
    .tab.active { background: #fff; border-color: var(--line); color: var(--strong); }
    .tabpanel { display: none; }
    .tabpanel.active { display: block; }
    [hidden] { display: none !important; }
    .table-wrap { width: 100%; overflow: auto; }
    table { width: 100%; min-width: 780px; border-collapse: collapse; }
    th, td {
      border-bottom: 1px solid var(--line);
      padding: 9px 10px;
      text-align: left;
      vertical-align: top;
      white-space: nowrap;
    }
    th { background: var(--soft); color: var(--muted); font-size: 12px; font-weight: 700; }
    tr:last-child td { border-bottom: 0; }
    code { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 12px; color: #262626; }
    .code-cell { display: flex; align-items: center; gap: 7px; }
    .note-cell { min-width: 210px; max-width: 300px; white-space: normal; }
    .note-view { display: flex; align-items: center; gap: 8px; min-height: 28px; }
    .note-text { min-width: 0; max-width: 205px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text); }
    .note-empty { color: var(--muted); }
    .note-edit { display: grid; grid-template-columns: minmax(150px, 1fr) auto; align-items: center; gap: 7px; }
    .note-edit input { min-height: 30px; }
    .note-edit-actions { display: flex; align-items: center; gap: 6px; }
    .select-cell { width: 34px; text-align: center; }
    .select-cell input { width: auto; min-height: auto; padding: 0; }
    .pill {
      display: inline-flex;
      align-items: center;
      min-height: 22px;
      border: 1px solid var(--line);
      border-radius: 999px;
      background: #fff;
      color: var(--muted);
      padding: 0 8px;
      font-size: 12px;
    }
    .pill.ok { color: var(--ok); border-color: #bbd7c2; background: #f0f8f2; }
    .pill.bad { color: var(--bad); border-color: #efc7c7; background: #fff5f5; }
    .pill.warn { color: var(--warn); border-color: #f0d4a8; background: #fff8ed; }
    .icon-status {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 24px;
      height: 24px;
      border-radius: 999px;
      border: 1px solid var(--line);
      font-weight: 800;
      line-height: 1;
    }
    .icon-status.ok { color: var(--ok); border-color: #bbd7c2; background: #f0f8f2; }
    .icon-status.bad { color: var(--bad); border-color: #efc7c7; background: #fff5f5; }
    .created-list { display: grid; gap: 8px; margin-top: 10px; }
    .created-item { display: grid; gap: 8px; border: 1px solid var(--line); border-radius: 7px; background: #fbfbfa; padding: 10px; }
    .created-item-row { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
    .provider-form-wrap { border-top: 1px solid var(--line); background: #fff; }
    @media (max-width: 900px) {
      .shell { padding: 16px; }
      header { align-items: flex-start; flex-direction: column; }
      .header-actions { justify-content: flex-start; }
      .grid { grid-template-columns: 1fr; }
      .columns { grid-template-columns: 1fr; }
      table { min-width: 900px; }
    }
  </style>
</head>
<body>
  <div class="shell">
    <header>
      <div class="brand">
        <img class="mark" src="/lillian-icon.svg" alt="">
        <div>
          <h1>莉莉安的后台</h1>
          <div class="subtitle">Lillian's Canvas Admin</div>
        </div>
      </div>
      <div class="header-actions">
        <div id="authStatus" class="status"><span class="dot"></span><span>未登录</span></div>
        <button id="headerLogoutButton" type="button" hidden>退出登录</button>
      </div>
    </header>

    <div id="loginPanel" class="login-panel">
      <section class="login-card">
        <div class="section-head"><h2>管理员登录</h2></div>
        <div class="body">
          <form id="loginForm" class="form">
            <label>管理员密码<input id="adminPassword" type="password" autocomplete="current-password" placeholder="输入管理员密码"></label>
            <div class="actions"><button class="primary" type="submit">进入后台</button></div>
            <div id="loginMessage" class="message"></div>
          </form>
        </div>
      </section>
    </div>

    <div id="adminGrid" class="grid" hidden>
      <div class="stack">
        <section>
          <div class="section-head"><h2>生成兑换码</h2></div>
          <div class="body">
            <form id="licenseForm" class="form">
              <div class="columns">
                <label>类型<select name="tier"><option value="basic">普通 1K</option><option value="hd">高清 2K/4K</option></select></label>
                <label>数量<input name="count" type="number" min="1" max="100" value="1"></label>
              </div>
              <div class="columns">
                <label>每个密匙次数<input name="totalCredits" type="number" min="1" value="5"></label>
                <label>最大并发<input name="maxConcurrent" type="number" min="1" value="6"></label>
              </div>
              <label>备注 / 发放对象<input name="note" placeholder="例如：Alice / 订单号 / 渠道"></label>
              <label>有效天数<input name="expiresInDays" type="number" min="1" value="30"></label>
              <div class="actions">
                <button class="primary" type="submit">生成兑换码</button>
                <button id="copyCreatedKeys" type="button" hidden>复制本次生成</button>
              </div>
              <div id="licenseMessage" class="message"></div>
              <div id="createdKeys" class="created-list" hidden></div>
            </form>
          </div>
        </section>
      </div>

      <div class="stack">
        <section>
          <div class="section-head">
            <h2>后台数据</h2>
            <div class="tabs">
              <button class="tab active" type="button" data-tab="licenses">兑换码</button>
              <button class="tab" type="button" data-tab="profiles">服务商</button>
            </div>
          </div>
          <div id="licensesPanel" class="tabpanel active">
            <div class="section-head">
              <h2>兑换码列表</h2>
              <div class="section-actions">
                <button id="deleteSelectedLicenses" class="danger" type="button" disabled>删除选中</button>
                <button id="refreshLicenses" type="button">刷新</button>
              </div>
            </div>
            <div class="table-wrap">
              <table>
                <thead><tr><th class="select-cell"><input id="selectAllLicenses" type="checkbox" aria-label="全选兑换码"></th><th>兑换码</th><th>类型</th><th>剩余/总数</th><th>并发</th><th>有效</th><th>备注 / 发放对象</th><th>操作</th></tr></thead>
                <tbody id="licensesTable"></tbody>
              </table>
            </div>
          </div>
          <div id="profilesPanel" class="tabpanel">
            <div class="section-head">
              <h2>服务商列表</h2>
              <div class="section-actions">
                <button id="newProfile" type="button">新增服务商</button>
                <button id="refreshProfiles" type="button">刷新</button>
              </div>
            </div>
            <div class="table-wrap">
              <table>
                <thead><tr><th>名称</th><th>桶</th><th>模型</th><th>优先级</th><th>状态</th><th>密钥</th><th>操作</th></tr></thead>
                <tbody id="profilesTable"></tbody>
              </table>
            </div>
            <div id="providerFormWrap" class="provider-form-wrap" hidden>
              <div class="section-head"><h2>服务商配置</h2></div>
              <div class="body">
                <form id="profileForm" class="form">
                  <input name="id" type="hidden">
                  <div class="columns">
                    <label>名称<input name="label" placeholder="BLTCY 1K"></label>
                    <label>桶<select name="tierBucket"><option value="1k">1K</option><option value="hd">HD</option></select></label>
                  </div>
                  <label>Base URL<input name="apiBaseUrl" placeholder="https://api.example.com"></label>
                  <label>API Key<input name="apiKey" type="password" autocomplete="off" placeholder="更新时留空表示沿用原密钥"></label>
                  <div class="columns">
                    <label>优先级<input name="priority" type="number" min="1" value="100"></label>
                    <label>状态<select name="status"><option value="active">启用</option><option value="disabled">停用</option></select></label>
                  </div>
                  <label>API 模式<select name="apiMode"><option value="images">OpenAI Images</option><option value="ohmytoken">OhMyToken</option><option value="responses">Responses</option></select></label>
                  <p class="hint">模型固定为 gpt-image-2。OpenAI Images/Responses 会发送质量参数；OhMyToken 模式只发送 size、aspect_ratio、response_format。</p>
                  <div class="actions">
                    <button class="primary" type="submit">保存服务商</button>
                    <button id="resetProfileForm" type="button">清空</button>
                    <button id="deleteProfileButton" class="danger form-delete" type="button" hidden>删除服务商</button>
                  </div>
                  <div id="profileMessage" class="message"></div>
                </form>
              </div>
            </div>
          </div>
        </section>
      </div>
    </div>
  </div>

  <script>
    (function () {
      var state = {
        token: localStorage.getItem('lillian-admin-token') || '',
        profiles: [],
        licenses: [],
        createdKeys: [],
        editingProfileId: '',
        editingLicenseNoteId: '',
        lastSelectedLicenseId: '',
        selectedLicenseIds: []
      };
      var els = {
        loginPanel: document.getElementById('loginPanel'),
        adminGrid: document.getElementById('adminGrid'),
        loginForm: document.getElementById('loginForm'),
        adminPassword: document.getElementById('adminPassword'),
        loginMessage: document.getElementById('loginMessage'),
        headerLogoutButton: document.getElementById('headerLogoutButton'),
        authStatus: document.getElementById('authStatus'),
        licenseForm: document.getElementById('licenseForm'),
        licenseMessage: document.getElementById('licenseMessage'),
        copyCreatedKeys: document.getElementById('copyCreatedKeys'),
        createdKeys: document.getElementById('createdKeys'),
        selectAllLicenses: document.getElementById('selectAllLicenses'),
        deleteSelectedLicenses: document.getElementById('deleteSelectedLicenses'),
        profileForm: document.getElementById('profileForm'),
        profileMessage: document.getElementById('profileMessage'),
        resetProfileForm: document.getElementById('resetProfileForm'),
        newProfile: document.getElementById('newProfile'),
        deleteProfileButton: document.getElementById('deleteProfileButton'),
        providerFormWrap: document.getElementById('providerFormWrap'),
        profilesTable: document.getElementById('profilesTable'),
        licensesTable: document.getElementById('licensesTable')
      };

      setAdminVisible(Boolean(state.token));
      updateAuthStatus(false);

      function h(value) {
        return String(value == null ? '' : value)
          .replace(/&/g, '&amp;')
          .replace(/</g, '&lt;')
          .replace(/>/g, '&gt;')
          .replace(/"/g, '&quot;')
          .replace(/'/g, '&#39;');
      }
      function message(el, text, kind) {
        el.textContent = text || '';
        el.className = 'message' + (kind ? ' ' + kind : '');
      }
      function setAdminVisible(visible) {
        els.loginPanel.hidden = visible;
        els.adminGrid.hidden = !visible;
        els.headerLogoutButton.hidden = !visible;
      }
      function updateAuthStatus(ok) {
        var text = state.token ? (ok ? '已登录' : '正在验证') : '未登录';
        els.authStatus.className = 'status' + (ok ? ' ok' : state.token ? '' : ' bad');
        els.authStatus.querySelector('span:last-child').textContent = text;
      }
      function authHeaders() {
        if (!state.token) throw new Error('请先登录管理员账号');
        return { Authorization: 'Bearer ' + state.token };
      }
      async function api(path, options) {
        options = options || {};
        var headers = Object.assign({}, options.headers || {}, authHeaders());
        if (options.body && !headers['Content-Type']) headers['Content-Type'] = 'application/json';
        var response = await fetch(path, Object.assign({}, options, { headers: headers }));
        var contentType = response.headers.get('Content-Type') || '';
        var body = contentType.indexOf('application/json') >= 0 ? await response.json() : await response.text();
        if (!response.ok) {
          var text = body && body.error && body.error.message ? body.error.message : String(body || response.statusText);
          throw new Error(text);
        }
        return body;
      }
      async function copyText(text) {
        await navigator.clipboard.writeText(text);
      }
      function statusPill(value) {
        var kind = value === 'active' || value === 'done' ? 'ok' : value === 'queued' || value === 'running' ? 'warn' : 'bad';
        return '<span class="pill ' + kind + '">' + h(value || '-') + '</span>';
      }
      function isExpired(value) {
        if (!value) return false;
        var ts = Date.parse(value);
        return Number.isFinite(ts) && ts <= Date.now();
      }
      function expiryStatus(value) {
        var expired = isExpired(value);
        var label = expired ? '已过期' : '有效';
        return '<span class="icon-status ' + (expired ? 'bad' : 'ok') + '" title="' + h(label) + '" aria-label="' + h(label) + '">' + (expired ? '×' : '✓') + '</span>';
      }
      function renderCreatedKeys() {
        els.copyCreatedKeys.hidden = state.createdKeys.length === 0;
        els.createdKeys.hidden = state.createdKeys.length === 0;
        els.createdKeys.innerHTML = state.createdKeys.map(function (item) {
          return '<div class="created-item"><div class="created-item-row"><code>' + h(item.key) + '</code><button class="small" type="button" data-copy-created="' + h(item.key) + '">复制</button></div><div class="hint">' + h(item.tier) + ' · ' + h(item.totalCredits) + ' 次' + (item.note ? ' · ' + h(item.note) : '') + '</div></div>';
        }).join('');
      }
      function renderLicenses() {
        els.licensesTable.innerHTML = state.licenses.map(function (license) {
          var key = license.key || '';
          var checked = state.selectedLicenseIds.indexOf(license.id) >= 0 ? ' checked' : '';
          var note = license.note || '';
          var editingNote = state.editingLicenseNoteId === license.id;
          var noteHtml = editingNote
            ? '<div class="note-edit"><input data-license-note-input="' + h(license.id) + '" value="' + h(note) + '" placeholder="备注 / 发放对象"><div class="note-edit-actions"><button class="small primary" type="button" data-save-license-note="' + h(license.id) + '">确定</button><button class="small" type="button" data-cancel-license-note="' + h(license.id) + '">取消</button></div></div>'
            : '<div class="note-view"><span class="note-text' + (note ? '' : ' note-empty') + '">' + h(note || '未填写') + '</span><button class="small" type="button" data-edit-license-note="' + h(license.id) + '">编辑</button></div>';
          return '<tr><td class="select-cell"><input type="checkbox" data-license-select="' + h(license.id) + '"' + checked + ' aria-label="选择兑换码"></td><td><div class="code-cell">' +
            (key ? '<code>' + h(key) + '</code><button class="small" type="button" data-copy-license="' + h(key) + '">复制</button>' : '<span class="pill warn">旧数据不可查看</span>') +
            '</div></td><td>' + h(license.tier) + '</td><td>' + h(license.remaining_credits) + ' / ' + h(license.total_credits) + '</td><td>' + h(license.max_concurrent) + '</td><td>' + expiryStatus(license.expires_at) + '</td><td class="note-cell">' + noteHtml + '</td><td><button class="small danger" type="button" data-delete-license="' + h(license.id) + '">删除</button></td></tr>';
        }).join('') || '<tr><td colspan="8">暂无兑换码</td></tr>';
        var selectedCount = state.selectedLicenseIds.length;
        els.deleteSelectedLicenses.disabled = selectedCount === 0;
        els.deleteSelectedLicenses.textContent = selectedCount ? '删除选中 ' + selectedCount : '删除选中';
        els.selectAllLicenses.checked = state.licenses.length > 0 && selectedCount === state.licenses.length;
        els.selectAllLicenses.indeterminate = selectedCount > 0 && selectedCount < state.licenses.length;
      }
      function renderProfiles() {
        els.profilesTable.innerHTML = state.profiles.map(function (profile) {
          return '<tr><td>' + h(profile.label) + '</td><td><span class="pill">' + h(profile.tierBucket) + '</span></td><td>' + h(profile.model) + '</td><td>' + h(profile.priority) + '</td><td>' + statusPill(profile.status) + '</td><td>' + (profile.hasApiKey ? '<span class="pill ok">已保存</span>' : '<span class="pill bad">缺失</span>') + '</td><td><div class="actions"><button class="small" type="button" data-edit-profile="' + h(profile.id) + '">修改</button><button class="small danger" type="button" data-delete-profile="' + h(profile.id) + '">删除</button></div></td></tr>';
        }).join('') || '<tr><td colspan="7">暂无服务商</td></tr>';
      }
      async function loadLicenses() {
        var data = await api('/admin/licenses?limit=100');
        state.licenses = data.licenses || [];
        var licenseIds = state.licenses.map(function (item) { return item.id; });
        state.selectedLicenseIds = state.selectedLicenseIds.filter(function (id) { return licenseIds.indexOf(id) >= 0; });
        if (state.editingLicenseNoteId && licenseIds.indexOf(state.editingLicenseNoteId) < 0) state.editingLicenseNoteId = '';
        if (state.lastSelectedLicenseId && licenseIds.indexOf(state.lastSelectedLicenseId) < 0) state.lastSelectedLicenseId = '';
        renderLicenses();
      }
      async function loadProfiles() {
        var data = await api('/admin/service-profiles');
        state.profiles = data.serviceProfiles || [];
        renderProfiles();
      }
      async function refreshAll() {
        await Promise.all([loadLicenses(), loadProfiles()]);
        setAdminVisible(true);
        updateAuthStatus(true);
      }
      function formValue(form, name) {
        return new FormData(form).get(name);
      }
      function setProfileFormVisible(visible) {
        els.providerFormWrap.hidden = !visible;
      }
      function resetProfileForm(messageText, visible) {
        state.editingProfileId = '';
        els.profileForm.reset();
        els.profileForm.elements.id.value = '';
        els.profileForm.elements.tierBucket.value = '1k';
        els.profileForm.elements.priority.value = 100;
        els.profileForm.elements.apiMode.value = 'images';
        els.profileForm.elements.status.value = 'active';
        els.deleteProfileButton.hidden = true;
        setProfileFormVisible(Boolean(visible));
        message(els.profileMessage, messageText || '', '');
      }
      function loadProfileForm(profile) {
        setProfileFormVisible(true);
        state.editingProfileId = profile.id || '';
        els.profileForm.elements.id.value = profile.id || '';
        els.profileForm.elements.label.value = profile.label || '';
        els.profileForm.elements.tierBucket.value = profile.tierBucket || '1k';
        els.profileForm.elements.apiBaseUrl.value = profile.apiBaseUrl || '';
        els.profileForm.elements.apiKey.value = '';
        els.profileForm.elements.apiMode.value = profile.apiMode || 'images';
        els.profileForm.elements.priority.value = profile.priority || 100;
        els.profileForm.elements.status.value = profile.status || 'active';
        els.deleteProfileButton.hidden = !state.editingProfileId;
        message(els.profileMessage, '已载入服务商，保存时 API Key 留空会沿用原密钥。', '');
      }
      async function deleteLicenseIds(ids) {
        ids = Array.from(new Set((ids || []).filter(Boolean)));
        if (!ids.length) return;
        if (!confirm('确定删除选中的 ' + ids.length + ' 个兑换码？删除后列表中不再显示。')) return;
        await api('/admin/licenses/delete', { method: 'POST', body: JSON.stringify({ ids: ids }) });
        state.selectedLicenseIds = state.selectedLicenseIds.filter(function (id) { return ids.indexOf(id) < 0; });
        message(els.licenseMessage, '已删除 ' + ids.length + ' 个兑换码', 'ok');
        await loadLicenses();
      }
      function logout() {
        state.token = '';
        state.createdKeys = [];
        localStorage.removeItem('lillian-admin-token');
        els.adminPassword.value = '';
        setAdminVisible(false);
        updateAuthStatus(false);
        renderCreatedKeys();
        message(els.loginMessage, '', '');
      }
      els.loginForm.addEventListener('submit', async function (event) {
        event.preventDefault();
        var password = els.adminPassword.value.trim();
        if (!password) {
          message(els.loginMessage, '请输入管理员密码', 'bad');
          return;
        }
        state.token = password;
        setAdminVisible(true);
        updateAuthStatus(false);
        try {
          await refreshAll();
          localStorage.setItem('lillian-admin-token', state.token);
          els.adminPassword.value = '';
          message(els.loginMessage, '', '');
        } catch (error) {
          state.token = '';
          localStorage.removeItem('lillian-admin-token');
          setAdminVisible(false);
          updateAuthStatus(false);
          message(els.loginMessage, error.message, 'bad');
        }
      });
      els.headerLogoutButton.addEventListener('click', logout);
      document.getElementById('refreshLicenses').addEventListener('click', function () {
        loadLicenses().then(function () { message(els.licenseMessage, '兑换码已刷新', 'ok'); }).catch(function (error) { message(els.licenseMessage, error.message, 'bad'); });
      });
      els.selectAllLicenses.addEventListener('change', function () {
        state.selectedLicenseIds = els.selectAllLicenses.checked ? state.licenses.map(function (license) { return license.id; }) : [];
        state.lastSelectedLicenseId = '';
        renderLicenses();
      });
      els.deleteSelectedLicenses.addEventListener('click', function () {
        deleteLicenseIds(state.selectedLicenseIds).catch(function (error) { message(els.licenseMessage, error.message, 'bad'); });
      });
      document.getElementById('refreshProfiles').addEventListener('click', function () {
        loadProfiles().then(function () { message(els.profileMessage, '服务商已刷新', 'ok'); }).catch(function (error) { message(els.profileMessage, error.message, 'bad'); });
      });
      document.querySelectorAll('.tab').forEach(function (button) {
        button.addEventListener('click', function () {
          document.querySelectorAll('.tab').forEach(function (item) { item.classList.remove('active'); });
          document.querySelectorAll('.tabpanel').forEach(function (panel) { panel.classList.remove('active'); });
          button.classList.add('active');
          document.getElementById(button.dataset.tab + 'Panel').classList.add('active');
        });
      });
      els.licenseForm.addEventListener('submit', async function (event) {
        event.preventDefault();
        message(els.licenseMessage, '正在生成...', '');
        try {
          var payload = {
            tier: formValue(els.licenseForm, 'tier'),
            count: Number(formValue(els.licenseForm, 'count') || 1),
            totalCredits: Number(formValue(els.licenseForm, 'totalCredits') || 5),
            maxConcurrent: Number(formValue(els.licenseForm, 'maxConcurrent') || 6),
            expiresInDays: Number(formValue(els.licenseForm, 'expiresInDays') || 30),
            note: String(formValue(els.licenseForm, 'note') || '').trim()
          };
          var data = await api('/admin/licenses', { method: 'POST', body: JSON.stringify(payload) });
          state.createdKeys = data.keys || [];
          renderCreatedKeys();
          message(els.licenseMessage, '已生成 ' + state.createdKeys.length + ' 个兑换码。', 'ok');
          await loadLicenses();
        } catch (error) {
          message(els.licenseMessage, error.message, 'bad');
        }
      });
      els.copyCreatedKeys.addEventListener('click', function () {
        var text = state.createdKeys.map(function (item) { return item.key; }).join('\n');
        copyText(text).then(function () { message(els.licenseMessage, '已复制本次生成的兑换码', 'ok'); }).catch(function (error) { message(els.licenseMessage, error.message, 'bad'); });
      });
      els.profileForm.addEventListener('submit', async function (event) {
        event.preventDefault();
        message(els.profileMessage, '正在保存...', '');
        try {
          var payload = {
            id: String(formValue(els.profileForm, 'id') || '').trim(),
            label: String(formValue(els.profileForm, 'label') || '').trim(),
            tierBucket: formValue(els.profileForm, 'tierBucket'),
            apiBaseUrl: String(formValue(els.profileForm, 'apiBaseUrl') || '').trim(),
            apiKey: String(formValue(els.profileForm, 'apiKey') || '').trim(),
            apiMode: formValue(els.profileForm, 'apiMode'),
            priority: Number(formValue(els.profileForm, 'priority') || 100),
            status: formValue(els.profileForm, 'status')
          };
          var profile = await api('/admin/service-profiles', { method: 'POST', body: JSON.stringify(payload) });
          els.profileForm.elements.apiKey.value = '';
          state.editingProfileId = profile.id || payload.id || '';
          els.profileForm.elements.id.value = state.editingProfileId;
          els.deleteProfileButton.hidden = !state.editingProfileId;
          message(els.profileMessage, '服务商已保存', 'ok');
          await loadProfiles();
        } catch (error) {
          message(els.profileMessage, error.message, 'bad');
        }
      });
      els.resetProfileForm.addEventListener('click', function () { resetProfileForm('', false); });
      els.newProfile.addEventListener('click', function () { resetProfileForm('正在新增服务商，保存后会自动生成内部 ID。', true); });
      els.deleteProfileButton.addEventListener('click', async function () {
        var id = state.editingProfileId || String(formValue(els.profileForm, 'id') || '').trim();
        if (!id) return;
        if (!confirm('确定删除这个服务商？删除后不会再参与路由。')) return;
        els.deleteProfileButton.disabled = true;
        try {
          await api('/admin/service-profiles/' + encodeURIComponent(id), { method: 'DELETE' });
          message(els.profileMessage, '服务商已删除', 'ok');
          resetProfileForm('服务商已删除');
          await loadProfiles();
        } catch (error) {
          message(els.profileMessage, error.message, 'bad');
        } finally {
          els.deleteProfileButton.disabled = false;
        }
      });
      document.addEventListener('click', async function (event) {
        var target = event.target;
        if (!target || !target.dataset) return;
        if (target.dataset.copyCreated) copyText(target.dataset.copyCreated).then(function () { message(els.licenseMessage, '已复制兑换码', 'ok'); }).catch(function (error) { message(els.licenseMessage, error.message, 'bad'); });
        if (target.dataset.copyLicense) copyText(target.dataset.copyLicense).then(function () { message(els.licenseMessage, '已复制兑换码', 'ok'); }).catch(function (error) { message(els.licenseMessage, error.message, 'bad'); });
        if (target.dataset.licenseSelect) {
          var selectedId = target.dataset.licenseSelect;
          var orderedLicenseIds = state.licenses.map(function (license) { return license.id; });
          var selectedSet = new Set(state.selectedLicenseIds);
          var shouldSelect = Boolean(target.checked);
          var anchorIndex = orderedLicenseIds.indexOf(state.lastSelectedLicenseId);
          var currentIndex = orderedLicenseIds.indexOf(selectedId);
          var rangeIds = [selectedId];
          if (event.shiftKey && anchorIndex >= 0 && currentIndex >= 0) {
            var start = Math.min(anchorIndex, currentIndex);
            var end = Math.max(anchorIndex, currentIndex);
            rangeIds = orderedLicenseIds.slice(start, end + 1);
          }
          if (shouldSelect) rangeIds.forEach(function (id) { selectedSet.add(id); });
          else rangeIds.forEach(function (id) { selectedSet.delete(id); });
          state.selectedLicenseIds = orderedLicenseIds.filter(function (id) { return selectedSet.has(id); });
          state.lastSelectedLicenseId = selectedId;
          renderLicenses();
        }
        if (target.dataset.editLicenseNote) {
          state.editingLicenseNoteId = target.dataset.editLicenseNote;
          renderLicenses();
          setTimeout(function () {
            var input = document.querySelector('[data-license-note-input="' + CSS.escape(state.editingLicenseNoteId) + '"]');
            if (input) { input.focus(); input.select(); }
          }, 0);
        }
        if (target.dataset.cancelLicenseNote) {
          state.editingLicenseNoteId = '';
          renderLicenses();
        }
        if (target.dataset.saveLicenseNote) {
          var noteInput = document.querySelector('[data-license-note-input="' + CSS.escape(target.dataset.saveLicenseNote) + '"]');
          var note = noteInput ? noteInput.value : '';
          target.disabled = true;
          try {
            await api('/admin/licenses/' + encodeURIComponent(target.dataset.saveLicenseNote), { method: 'PATCH', body: JSON.stringify({ note: note }) });
            state.editingLicenseNoteId = '';
            message(els.licenseMessage, '备注已保存', 'ok');
            await loadLicenses();
          } catch (error) {
            message(els.licenseMessage, error.message, 'bad');
          } finally {
            target.disabled = false;
          }
        }
        if (target.dataset.deleteLicense) deleteLicenseIds([target.dataset.deleteLicense]).catch(function (error) { message(els.licenseMessage, error.message, 'bad'); });
        if (target.dataset.editProfile) {
          var profile = state.profiles.find(function (item) { return item.id === target.dataset.editProfile; });
          if (profile) loadProfileForm(profile);
        }
        if (target.dataset.deleteProfile) {
          var deleteProfile = state.profiles.find(function (item) { return item.id === target.dataset.deleteProfile; });
          var name = deleteProfile ? deleteProfile.label : target.dataset.deleteProfile;
          if (!confirm('确定删除服务商「' + name + '」？删除后不会再参与路由。')) return;
          target.disabled = true;
          try {
            await api('/admin/service-profiles/' + encodeURIComponent(target.dataset.deleteProfile), { method: 'DELETE' });
            if (state.editingProfileId === target.dataset.deleteProfile) resetProfileForm('服务商已删除');
            await loadProfiles();
          } catch (error) {
            message(els.profileMessage, error.message, 'bad');
          } finally {
            target.disabled = false;
          }
        }
      });
      renderCreatedKeys();
      renderLicenses();
      renderProfiles();
      if (state.token) refreshAll().catch(function () { logout(); message(els.loginMessage, '登录已失效，请重新输入管理员密码', 'bad'); });
    })();
  </script>
</body>
</html>`
}

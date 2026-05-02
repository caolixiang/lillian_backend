import './styles.css'

type MessageKind = '' | 'ok' | 'bad'
type AdminView = 'main' | 'settings'

interface LicenseRecord {
  id: string
  key?: string
  serviceCode: string
  credits: number
  max_concurrent: number
  status?: string
  expires_at?: string | null
  redeemed_at?: string | null
  redeemedWalletAddress?: string | null
  redeemedWalletId?: string | null
  note?: string
}

interface ServiceProfile {
  id: string
  label: string
  tierBucket: '1k' | 'hd' | string
  apiBaseUrl: string
  model: string
  apiMode: string
  codexCli?: boolean
  priority: number
  maxConcurrent?: number
  status: string
  hasApiKey: boolean
}

interface RuntimeSettings {
  imageGlobalConcurrency: number
  imageProviderDefaultConcurrency: number
  upstreamTimeoutSeconds: number
}

interface WalletEntitlement {
  serviceCode: string
  label?: string
  remaining: number
  maxConcurrent: number
}

interface AdminWalletRecord {
  address: string
  entitlements: WalletEntitlement[]
}

interface AdminWalletRedemption {
  id: string
  licenseKeyId: string
  serviceCode: string
  creditsAdded: number
  licenseNote?: string
  licenseStatus?: string
  createdAt: string
}

interface AdminWalletTask {
  id: string
  serviceCode: string
  status: string
  requestedSize: string
  serviceProfile: string
  serviceProfileLabel?: string
  creditReserved: boolean
  creditCharged: boolean
  error?: string | null
  createdAt: string
  updatedAt: string
  finishedAt?: string | null
}

interface AdminWalletLookupResponse {
  wallet: AdminWalletRecord
  redemptions: AdminWalletRedemption[]
  tasks: AdminWalletTask[]
}

interface CreateLicenseResponse {
  keys: Array<{
    id: string
    key: string
    serviceCode: string
    credits: number
    maxConcurrent: number
    note?: string
  }>
}

interface LicenseListResponse {
  licenses: LicenseRecord[]
  page?: {
    limit?: number
    offset?: number
    hasMore?: boolean
    search?: string
  }
}

interface ServiceProfileListResponse {
  serviceProfiles: ServiceProfile[]
}

interface RuntimeSettingsResponse {
  settings: RuntimeSettings
}

interface AdminState {
  token: string
  profiles: ServiceProfile[]
  licenses: LicenseRecord[]
  createdKeys: CreateLicenseResponse['keys']
  editingProfileId: string
  editingLicenseNoteId: string
  lastSelectedLicenseId: string
  selectedLicenseIds: string[]
  licensePageSize: number
  licenseOffset: number
  licenseHasMore: boolean
  licenseSearch: string
  walletLookupAddress: string
  walletLookup: AdminWalletLookupResponse | null
  runtimeSettings: RuntimeSettings
  view: AdminView
}

const state: AdminState = {
  token: localStorage.getItem('lillian-admin-token') || '',
  profiles: [],
  licenses: [],
  createdKeys: [],
  editingProfileId: '',
  editingLicenseNoteId: '',
  lastSelectedLicenseId: '',
  selectedLicenseIds: [],
  licensePageSize: initialLicensePageSize(),
  licenseOffset: 0,
  licenseHasMore: false,
  licenseSearch: '',
  walletLookupAddress: '',
  walletLookup: null,
  runtimeSettings: {
    imageGlobalConcurrency: 6,
    imageProviderDefaultConcurrency: 2,
    upstreamTimeoutSeconds: 600,
  },
  view: 'main',
}

const app = document.querySelector<HTMLDivElement>('#app')

if (!app) {
  throw new Error('Admin root element is missing')
}

function initialLicensePageSize(): number {
  return localStorage.getItem('lillian-admin-license-page-size') === '50' ? 50 : 20
}

app.innerHTML = `
  <div class="shell">
    <header class="topbar">
      <div class="brand">
        <img class="mark" src="/lillian-icon.svg?v=cloudflare-backend-icon" alt="">
        <div>
          <h1>莉莉安的后台</h1>
          <div class="subtitle">Lillian's Canvas Admin</div>
        </div>
      </div>
      <div class="header-actions">
        <button id="settingsButton" type="button" hidden>设置</button>
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
                <label>服务<select name="serviceCode"><option value="image-2-sd">标清 image-2-sd</option><option value="image-2-hd">HD image-2-hd</option></select></label>
                <label>数量<input name="count" type="number" min="1" max="100" value="1"></label>
              </div>
              <div class="columns">
                <label>每个密匙次数<input name="credits" type="number" min="1" value="5"></label>
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
              <button class="tab" type="button" data-tab="wallets">钱包</button>
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
            <form id="licenseSearchForm" class="list-toolbar">
              <label class="search-field">搜索兑换码<input id="licenseSearch" name="q" type="search" autocomplete="off" placeholder="输入完整兑换码"></label>
              <div class="toolbar-actions">
                <button type="submit">搜索</button>
                <button id="clearLicenseSearch" type="button">清空</button>
                <label class="page-size">每页<select id="licensePageSize"><option value="20">20</option><option value="50">50</option></select></label>
              </div>
            </form>
            <div class="table-wrap">
              <table>
                <thead><tr><th class="select-cell"><input id="selectAllLicenses" type="checkbox" aria-label="全选兑换码"></th><th>兑换码</th><th>服务</th><th>次数</th><th>并发</th><th>状态</th><th>兑换钱包</th><th>备注 / 发放对象</th><th>操作</th></tr></thead>
                <tbody id="licensesTable"></tbody>
              </table>
            </div>
            <div class="pager">
              <span id="licensePageInfo" class="page-info"></span>
              <button id="prevLicensePage" type="button">上一页</button>
              <button id="nextLicensePage" type="button">下一页</button>
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
                <thead><tr><th>名称</th><th>桶</th><th>模型</th><th>优先级</th><th>并发</th><th>状态</th><th>密钥</th><th>操作</th></tr></thead>
                <tbody id="profilesTable"></tbody>
              </table>
            </div>
            <div id="providerFormWrap" class="provider-form-wrap" hidden>
              <div class="section-head"><h2>服务商配置</h2></div>
              <div class="body">
                <form id="profileForm" class="form">
                  <input name="id" type="hidden">
                  <div class="columns">
                    <label>名称<input name="label" placeholder="OpenAI Compatible HD"></label>
                    <label>桶<select name="tierBucket"><option value="1k">1K</option><option value="hd">HD</option></select></label>
                  </div>
                  <label>Base URL<input name="apiBaseUrl" placeholder="https://api.openai.com/v1"></label>
                  <label>API Key<input name="apiKey" type="password" autocomplete="off" placeholder="更新时留空表示沿用原密钥"></label>
                  <div class="columns">
                    <label>优先级<input name="priority" type="number" min="1" value="100"></label>
                    <label>最大并发<input name="maxConcurrent" type="number" min="0" max="100" value="0"></label>
                  </div>
                  <div class="columns">
                    <label>状态<select name="status"><option value="active">启用</option><option value="disabled">停用</option></select></label>
                    <label>API 模式<select name="apiMode"><option value="images">OpenAI Images</option><option value="ohmytoken">OhMyToken</option><option value="responses">Responses</option></select></label>
                  </div>
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
          <div id="walletsPanel" class="tabpanel">
            <div class="section-head">
              <h2>钱包查询</h2>
            </div>
            <form id="walletLookupForm" class="list-toolbar">
              <label class="search-field">钱包地址<input id="walletLookupAddress" name="address" type="search" autocomplete="off" placeholder="0x..."></label>
              <div class="toolbar-actions">
                <button class="primary" type="submit">查询</button>
                <button id="clearWalletLookup" type="button">清空</button>
              </div>
            </form>
            <div class="body wallet-summary" id="walletSummary" hidden></div>
            <div class="section-head compact-head"><h2>权益</h2></div>
            <div class="table-wrap">
              <table class="wallet-table">
                <thead><tr><th>服务</th><th>剩余次数</th><th>最大并发</th></tr></thead>
                <tbody id="walletEntitlementsTable"></tbody>
              </table>
            </div>
            <div class="section-head compact-head"><h2>兑换记录</h2></div>
            <div class="table-wrap">
              <table class="wallet-table">
                <thead><tr><th>时间</th><th>服务</th><th>次数</th><th>兑换码</th><th>备注</th></tr></thead>
                <tbody id="walletRedemptionsTable"></tbody>
              </table>
            </div>
            <div class="section-head compact-head"><h2>最近任务</h2></div>
            <div class="table-wrap">
              <table class="wallet-table">
                <thead><tr><th>时间</th><th>任务</th><th>服务</th><th>尺寸</th><th>状态</th><th>扣费</th><th>服务商</th><th>错误</th></tr></thead>
                <tbody id="walletTasksTable"></tbody>
              </table>
            </div>
            <div class="body"><div id="walletMessage" class="message"></div></div>
          </div>
        </section>
      </div>
    </div>

    <div id="settingsView" class="settings-view" hidden>
      <section>
        <div class="section-head">
          <h2>设置</h2>
          <div class="section-actions">
            <button id="settingsBackButton" type="button">返回</button>
            <button id="refreshRuntimeSettings" type="button">刷新</button>
          </div>
        </div>
        <div class="body">
          <form id="runtimeSettingsForm" class="form">
            <label>全局生图并发<input name="imageGlobalConcurrency" type="number" min="1" max="100" value="6"></label>
            <label>默认服务商并发<input name="imageProviderDefaultConcurrency" type="number" min="1" max="100" value="2"></label>
            <label>上游超时秒数<input name="upstreamTimeoutSeconds" type="number" min="60" max="1800" value="600"></label>
            <div class="actions">
              <button class="primary" type="submit">保存运行设置</button>
            </div>
            <div id="runtimeSettingsMessage" class="message"></div>
          </form>
        </div>
      </section>
    </div>
  </div>
`

const els = {
  loginPanel: mustGet<HTMLDivElement>('loginPanel'),
  adminGrid: mustGet<HTMLDivElement>('adminGrid'),
  settingsView: mustGet<HTMLDivElement>('settingsView'),
  loginForm: mustGet<HTMLFormElement>('loginForm'),
  adminPassword: mustGet<HTMLInputElement>('adminPassword'),
  loginMessage: mustGet<HTMLDivElement>('loginMessage'),
  settingsButton: mustGet<HTMLButtonElement>('settingsButton'),
  settingsBackButton: mustGet<HTMLButtonElement>('settingsBackButton'),
  headerLogoutButton: mustGet<HTMLButtonElement>('headerLogoutButton'),
  licenseForm: mustGet<HTMLFormElement>('licenseForm'),
  licenseMessage: mustGet<HTMLDivElement>('licenseMessage'),
  copyCreatedKeys: mustGet<HTMLButtonElement>('copyCreatedKeys'),
  createdKeys: mustGet<HTMLDivElement>('createdKeys'),
  selectAllLicenses: mustGet<HTMLInputElement>('selectAllLicenses'),
  deleteSelectedLicenses: mustGet<HTMLButtonElement>('deleteSelectedLicenses'),
  licenseSearchForm: mustGet<HTMLFormElement>('licenseSearchForm'),
  licenseSearch: mustGet<HTMLInputElement>('licenseSearch'),
  clearLicenseSearch: mustGet<HTMLButtonElement>('clearLicenseSearch'),
  licensePageSize: mustGet<HTMLSelectElement>('licensePageSize'),
  licensePageInfo: mustGet<HTMLSpanElement>('licensePageInfo'),
  prevLicensePage: mustGet<HTMLButtonElement>('prevLicensePage'),
  nextLicensePage: mustGet<HTMLButtonElement>('nextLicensePage'),
  walletLookupForm: mustGet<HTMLFormElement>('walletLookupForm'),
  walletLookupAddress: mustGet<HTMLInputElement>('walletLookupAddress'),
  clearWalletLookup: mustGet<HTMLButtonElement>('clearWalletLookup'),
  walletSummary: mustGet<HTMLDivElement>('walletSummary'),
  walletEntitlementsTable: mustGet<HTMLTableSectionElement>('walletEntitlementsTable'),
  walletRedemptionsTable: mustGet<HTMLTableSectionElement>('walletRedemptionsTable'),
  walletTasksTable: mustGet<HTMLTableSectionElement>('walletTasksTable'),
  walletMessage: mustGet<HTMLDivElement>('walletMessage'),
  runtimeSettingsForm: mustGet<HTMLFormElement>('runtimeSettingsForm'),
  runtimeSettingsMessage: mustGet<HTMLDivElement>('runtimeSettingsMessage'),
  refreshRuntimeSettings: mustGet<HTMLButtonElement>('refreshRuntimeSettings'),
  profileForm: mustGet<HTMLFormElement>('profileForm'),
  profileMessage: mustGet<HTMLDivElement>('profileMessage'),
  resetProfileForm: mustGet<HTMLButtonElement>('resetProfileForm'),
  newProfile: mustGet<HTMLButtonElement>('newProfile'),
  deleteProfileButton: mustGet<HTMLButtonElement>('deleteProfileButton'),
  providerFormWrap: mustGet<HTMLDivElement>('providerFormWrap'),
  profilesTable: mustGet<HTMLTableSectionElement>('profilesTable'),
  licensesTable: mustGet<HTMLTableSectionElement>('licensesTable'),
  refreshLicenses: mustGet<HTMLButtonElement>('refreshLicenses'),
  refreshProfiles: mustGet<HTMLButtonElement>('refreshProfiles'),
}

setAdminVisible(Boolean(state.token))
renderCreatedKeys()
renderLicenses()
renderProfiles()
renderWalletLookup()
renderRuntimeSettings()
bindEvents()

if (state.token) {
  refreshAll().catch(() => {
    logout()
    message(els.loginMessage, '登录已失效，请重新输入管理员密码', 'bad')
  })
}

function mustGet<T extends HTMLElement>(id: string): T {
  const element = document.getElementById(id)
  if (!element) {
    throw new Error(`Missing element: ${id}`)
  }
  return element as T
}

function field<T extends HTMLInputElement | HTMLSelectElement>(form: HTMLFormElement, name: string): T {
  const element = form.elements.namedItem(name)
  if (!(element instanceof HTMLInputElement || element instanceof HTMLSelectElement)) {
    throw new Error(`Missing form field: ${name}`)
  }
  return element as T
}

function formValue(form: HTMLFormElement, name: string): string {
  const value = new FormData(form).get(name)
  return typeof value === 'string' ? value : ''
}

function h(value: unknown): string {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function message(el: HTMLElement, text: string, kind: MessageKind = ''): void {
  el.textContent = text
  el.className = `message${kind ? ` ${kind}` : ''}`
}

function setAdminVisible(visible: boolean): void {
  els.loginPanel.hidden = visible
  els.settingsButton.hidden = !visible
  els.headerLogoutButton.hidden = !visible
  if (visible) {
    showAdminView(state.view)
  } else {
    els.adminGrid.hidden = true
    els.settingsView.hidden = true
  }
}

function showAdminView(view: AdminView): void {
  state.view = view
  const visible = Boolean(state.token)
  els.adminGrid.hidden = !visible || view !== 'main'
  els.settingsView.hidden = !visible || view !== 'settings'
  els.settingsButton.classList.toggle('active', visible && view === 'settings')
}

function authHeaders(): Record<string, string> {
  if (!state.token) {
    throw new Error('请先登录管理员账号')
  }
  return { Authorization: `Bearer ${state.token}` }
}

async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers)
  for (const [key, value] of Object.entries(authHeaders())) {
    headers.set(key, value)
  }
  if (options.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  const response = await fetch(path, { ...options, headers })
  const contentType = response.headers.get('Content-Type') || ''
  const body = contentType.includes('application/json') ? await response.json() : await response.text()
  if (!response.ok) {
    const errorBody = body as { error?: { message?: string } }
    const text = errorBody?.error?.message || String(body || response.statusText)
    throw new Error(text)
  }
  return body as T
}

async function copyText(text: string): Promise<void> {
  await navigator.clipboard.writeText(text)
}

function statusPill(value: string): string {
  const kind = value === 'active' || value === 'done' ? 'ok' : value === 'queued' || value === 'running' ? 'warn' : 'bad'
  return `<span class="pill ${kind}">${h(value || '-')}</span>`
}

function compactID(value: string): string {
  const text = String(value || '')
  if (text.length <= 16) return text
  return `${text.slice(0, 8)}…${text.slice(-6)}`
}

function formatDateTime(value?: string | null): string {
  if (!value) return '-'
  const ts = Date.parse(value)
  if (!Number.isFinite(ts)) return value
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(ts))
}

function isExpired(value?: string | null): boolean {
  if (!value) return false
  const ts = Date.parse(value)
  return Number.isFinite(ts) && ts <= Date.now()
}

function availabilityStatus(license: LicenseRecord): string {
  let label = '可用'
  if (license.status && license.status !== 'active') label = '不可用'
  else if (isExpired(license.expires_at)) label = '已过期'
  const available = label === '可用'
  return `<span class="icon-status ${available ? 'ok' : 'bad'}" title="${h(label)}" aria-label="${h(label)}">${available ? '✓' : '×'}</span>`
}

function renderCreatedKeys(): void {
  els.copyCreatedKeys.hidden = state.createdKeys.length === 0
  els.createdKeys.hidden = state.createdKeys.length === 0
  els.createdKeys.innerHTML = state.createdKeys
    .map(
      (item) =>
        `<div class="created-item"><div class="created-item-row"><code>${h(item.key)}</code><button class="small" type="button" data-copy-created="${h(item.key)}">复制</button></div><div class="hint">${h(serviceLabel(item.serviceCode))} · ${h(item.credits)} 次${item.note ? ` · ${h(item.note)}` : ''}</div></div>`,
    )
    .join('')
}

function renderLicenses(): void {
  els.licensesTable.innerHTML =
    state.licenses
      .map((license) => {
        const key = license.key || ''
        const checked = state.selectedLicenseIds.includes(license.id) ? ' checked' : ''
        const note = license.note || ''
        const redeemedWallet = license.redeemedWalletAddress || license.redeemedWalletId || ''
        const editingNote = state.editingLicenseNoteId === license.id
        const noteHtml = editingNote
          ? `<div class="note-edit"><input data-license-note-input="${h(license.id)}" value="${h(note)}" placeholder="备注 / 发放对象"><div class="note-edit-actions"><button class="small primary" type="button" data-save-license-note="${h(license.id)}">确定</button><button class="small" type="button" data-cancel-license-note="${h(license.id)}">取消</button></div></div>`
          : `<div class="note-view"><span class="note-text${note ? '' : ' note-empty'}">${h(note || '未填写')}</span><button class="small" type="button" data-edit-license-note="${h(license.id)}">编辑</button></div>`
        return `<tr><td class="select-cell"><input type="checkbox" data-license-select="${h(license.id)}"${checked} aria-label="选择兑换码"></td><td><div class="code-cell">${
          key
            ? `<code>${h(key)}</code><button class="small" type="button" data-copy-license="${h(key)}">复制</button>`
            : '<span class="pill warn">旧数据不可查看</span>'
        }</div></td><td>${h(serviceLabel(license.serviceCode))}</td><td>${h(license.credits)}</td><td>${h(license.max_concurrent)}</td><td>${availabilityStatus(license)}</td><td>${redeemedWallet ? `<code>${h(redeemedWallet)}</code>` : '<span class="pill">未兑换</span>'}</td><td class="note-cell">${noteHtml}</td><td><button class="small danger" type="button" data-delete-license="${h(license.id)}">删除</button></td></tr>`
      })
      .join('') || '<tr><td colspan="9">暂无兑换码</td></tr>'

  const selectedCount = state.selectedLicenseIds.length
  els.deleteSelectedLicenses.disabled = selectedCount === 0
  els.deleteSelectedLicenses.textContent = selectedCount ? `删除选中 ${selectedCount}` : '删除选中'
  els.selectAllLicenses.checked = state.licenses.length > 0 && selectedCount === state.licenses.length
  els.selectAllLicenses.indeterminate = selectedCount > 0 && selectedCount < state.licenses.length
  renderLicensePager()
}

function renderLicensePager(): void {
  const page = Math.floor(state.licenseOffset / state.licensePageSize) + 1
  const start = state.licenses.length ? state.licenseOffset + 1 : 0
  const end = state.licenseOffset + state.licenses.length
  const searchSuffix = state.licenseSearch ? ' · 搜索结果' : ''
  els.licensePageSize.value = String(state.licensePageSize)
  if (document.activeElement !== els.licenseSearch) {
    els.licenseSearch.value = state.licenseSearch
  }
  els.prevLicensePage.disabled = state.licenseOffset <= 0
  els.nextLicensePage.disabled = !state.licenseHasMore
  els.licensePageInfo.textContent = state.licenses.length ? `第 ${page} 页 · ${start}-${end}${searchSuffix}` : `暂无结果${searchSuffix}`
}

function renderProfiles(): void {
  els.profilesTable.innerHTML =
    state.profiles
      .map((profile) => {
        const enabled = profile.status === 'active'
        const providerConcurrency = Number(profile.maxConcurrent || 0) > 0 ? String(profile.maxConcurrent) : '默认'
        return `<tr><td>${h(profile.label)}</td><td><span class="pill">${h(profile.tierBucket)}</span></td><td>${h(profile.model)}</td><td>${h(profile.priority)}</td><td>${h(providerConcurrency)}</td><td>${statusPill(profile.status)}</td><td>${profile.hasApiKey ? '<span class="pill ok">已保存</span>' : '<span class="pill bad">缺失</span>'}</td><td><div class="actions"><button class="small" type="button" data-edit-profile="${h(profile.id)}">修改</button><button class="small" type="button" data-toggle-profile="${h(profile.id)}">${enabled ? '关闭' : '启用'}</button><button class="small danger" type="button" data-delete-profile="${h(profile.id)}">删除</button></div></td></tr>`
      })
      .join('') || '<tr><td colspan="8">暂无服务商</td></tr>'
}

function renderWalletLookup(): void {
  if (document.activeElement !== els.walletLookupAddress) {
    els.walletLookupAddress.value = state.walletLookupAddress
  }
  const data = state.walletLookup
  els.walletSummary.hidden = !data
  if (!data) {
    els.walletSummary.innerHTML = ''
    els.walletEntitlementsTable.innerHTML = '<tr><td colspan="3">输入钱包地址后查询</td></tr>'
    els.walletRedemptionsTable.innerHTML = '<tr><td colspan="5">输入钱包地址后查询</td></tr>'
    els.walletTasksTable.innerHTML = '<tr><td colspan="8">输入钱包地址后查询</td></tr>'
    return
  }

  const totalRemaining = data.wallet.entitlements.reduce((sum, item) => sum + Number(item.remaining || 0), 0)
  els.walletSummary.innerHTML = `<div class="metric-row"><div><span>钱包</span><code>${h(data.wallet.address)}</code></div><div><span>剩余总次数</span><strong>${h(totalRemaining)}</strong></div><div><span>服务数</span><strong>${h(data.wallet.entitlements.length)}</strong></div><div><span>最近任务</span><strong>${h(data.tasks.length)}</strong></div></div>`
  els.walletEntitlementsTable.innerHTML =
    data.wallet.entitlements
      .map(
        (item) =>
          `<tr><td>${h(item.label || serviceLabel(item.serviceCode))}<div class="hint">${h(item.serviceCode)}</div></td><td>${h(item.remaining)}</td><td>${h(item.maxConcurrent)}</td></tr>`,
      )
      .join('') || '<tr><td colspan="3">暂无权益</td></tr>'
  els.walletRedemptionsTable.innerHTML =
    data.redemptions
      .map((item) => {
        const note = item.licenseNote || ''
        return `<tr><td>${h(formatDateTime(item.createdAt))}</td><td>${h(serviceLabel(item.serviceCode))}<div class="hint">${h(item.serviceCode)}</div></td><td>${h(item.creditsAdded)}</td><td><code title="${h(item.licenseKeyId)}">${h(compactID(item.licenseKeyId))}</code></td><td>${note ? h(note) : '<span class="pill">未填写</span>'}</td></tr>`
      })
      .join('') || '<tr><td colspan="5">暂无兑换记录</td></tr>'
  els.walletTasksTable.innerHTML =
    data.tasks
      .map((task) => {
        const charged = task.creditCharged ? '<span class="pill ok">已扣费</span>' : task.creditReserved ? '<span class="pill warn">已预占</span>' : '<span class="pill">未扣费</span>'
        const error = task.error ? `<span class="task-error" title="${h(task.error)}">${h(task.error)}</span>` : '-'
        const provider = task.serviceProfileLabel || task.serviceProfile || '-'
        const providerTitle = task.serviceProfile && task.serviceProfileLabel ? task.serviceProfile : provider
        return `<tr><td>${h(formatDateTime(task.createdAt))}</td><td><code title="${h(task.id)}">${h(compactID(task.id))}</code></td><td>${h(serviceLabel(task.serviceCode))}</td><td>${h(task.requestedSize || '-')}</td><td>${statusPill(task.status)}</td><td>${charged}</td><td title="${h(providerTitle)}">${h(provider)}</td><td>${error}</td></tr>`
      })
      .join('') || '<tr><td colspan="8">暂无生成任务</td></tr>'
}

function renderRuntimeSettings(): void {
  field<HTMLInputElement>(els.runtimeSettingsForm, 'imageGlobalConcurrency').value = String(state.runtimeSettings.imageGlobalConcurrency)
  field<HTMLInputElement>(els.runtimeSettingsForm, 'imageProviderDefaultConcurrency').value = String(state.runtimeSettings.imageProviderDefaultConcurrency)
  field<HTMLInputElement>(els.runtimeSettingsForm, 'upstreamTimeoutSeconds').value = String(state.runtimeSettings.upstreamTimeoutSeconds)
}

async function loadLicenses(): Promise<void> {
  const params = new URLSearchParams({
    limit: String(state.licensePageSize),
    offset: String(state.licenseOffset),
  })
  if (state.licenseSearch) {
    params.set('q', state.licenseSearch)
  }
  const data = await api<LicenseListResponse>(`/admin/licenses?${params.toString()}`)
  if ((data.licenses || []).length === 0 && state.licenseOffset > 0) {
    state.licenseOffset = 0
    await loadLicenses()
    return
  }
  state.licenses = data.licenses || []
  state.licenseHasMore = Boolean(data.page?.hasMore)
  const licenseIds = state.licenses.map((item) => item.id)
  state.selectedLicenseIds = state.selectedLicenseIds.filter((id) => licenseIds.includes(id))
  if (state.editingLicenseNoteId && !licenseIds.includes(state.editingLicenseNoteId)) state.editingLicenseNoteId = ''
  if (state.lastSelectedLicenseId && !licenseIds.includes(state.lastSelectedLicenseId)) state.lastSelectedLicenseId = ''
  renderLicenses()
}

async function loadProfiles(): Promise<void> {
  const data = await api<ServiceProfileListResponse>('/admin/service-profiles')
  state.profiles = data.serviceProfiles || []
  renderProfiles()
}

async function loadRuntimeSettings(): Promise<void> {
  const data = await api<RuntimeSettingsResponse>('/admin/runtime-settings')
  state.runtimeSettings = data.settings || state.runtimeSettings
  renderRuntimeSettings()
}

async function loadWalletLookup(address: string): Promise<void> {
  const normalized = address.trim().toLowerCase()
  if (!/^0x[0-9a-f]{40}$/.test(normalized)) {
    throw new Error('钱包地址格式无效')
  }
  const data = await api<AdminWalletLookupResponse>(`/admin/wallets/${encodeURIComponent(normalized)}`)
  state.walletLookupAddress = normalized
  state.walletLookup = data
  renderWalletLookup()
}

async function refreshAll(): Promise<void> {
  await Promise.all([loadLicenses(), loadProfiles()])
  setAdminVisible(true)
}

function setProfileFormVisible(visible: boolean): void {
  els.providerFormWrap.hidden = !visible
}

function resetProfileForm(messageText = '', visible = false): void {
  state.editingProfileId = ''
  els.profileForm.reset()
  field<HTMLInputElement>(els.profileForm, 'id').value = ''
  field<HTMLSelectElement>(els.profileForm, 'tierBucket').value = '1k'
  field<HTMLInputElement>(els.profileForm, 'priority').value = '100'
  field<HTMLInputElement>(els.profileForm, 'maxConcurrent').value = '0'
  field<HTMLSelectElement>(els.profileForm, 'apiMode').value = 'images'
  field<HTMLSelectElement>(els.profileForm, 'status').value = 'active'
  els.deleteProfileButton.hidden = true
  setProfileFormVisible(visible)
  message(els.profileMessage, messageText)
}

function loadProfileForm(profile: ServiceProfile): void {
  setProfileFormVisible(true)
  state.editingProfileId = profile.id || ''
  field<HTMLInputElement>(els.profileForm, 'id').value = profile.id || ''
  field<HTMLInputElement>(els.profileForm, 'label').value = profile.label || ''
  field<HTMLSelectElement>(els.profileForm, 'tierBucket').value = profile.tierBucket || '1k'
  field<HTMLInputElement>(els.profileForm, 'apiBaseUrl').value = profile.apiBaseUrl || ''
  field<HTMLInputElement>(els.profileForm, 'apiKey').value = ''
  field<HTMLSelectElement>(els.profileForm, 'apiMode').value = profile.apiMode || 'images'
  field<HTMLInputElement>(els.profileForm, 'priority').value = String(profile.priority || 100)
  field<HTMLInputElement>(els.profileForm, 'maxConcurrent').value = String(profile.maxConcurrent || 0)
  field<HTMLSelectElement>(els.profileForm, 'status').value = profile.status || 'active'
  els.deleteProfileButton.hidden = !state.editingProfileId
  message(els.profileMessage, '已载入服务商，保存时 API Key 留空会沿用原密钥。')
}

async function deleteLicenseIds(ids: string[]): Promise<void> {
  const uniqueIds = Array.from(new Set(ids.filter(Boolean)))
  if (!uniqueIds.length) return
  if (!confirm(`确定删除选中的 ${uniqueIds.length} 个兑换码？删除后列表中不再显示。`)) return
  await api('/admin/licenses/delete', { method: 'POST', body: JSON.stringify({ ids: uniqueIds }) })
  state.selectedLicenseIds = state.selectedLicenseIds.filter((id) => !uniqueIds.includes(id))
  message(els.licenseMessage, `已删除 ${uniqueIds.length} 个兑换码`, 'ok')
  await loadLicenses()
}

function logout(): void {
  state.token = ''
  state.createdKeys = []
  localStorage.removeItem('lillian-admin-token')
  els.adminPassword.value = ''
  state.view = 'main'
  setAdminVisible(false)
  renderCreatedKeys()
  message(els.loginMessage, '')
}

function bindEvents(): void {
  els.loginForm.addEventListener('submit', async (event) => {
    event.preventDefault()
    const password = els.adminPassword.value.trim()
    if (!password) {
      message(els.loginMessage, '请输入管理员密码', 'bad')
      return
    }
    state.token = password
    state.view = 'main'
    setAdminVisible(true)
    try {
      await refreshAll()
      localStorage.setItem('lillian-admin-token', state.token)
      els.adminPassword.value = ''
      message(els.loginMessage, '')
    } catch (error) {
      state.token = ''
      localStorage.removeItem('lillian-admin-token')
      setAdminVisible(false)
      message(els.loginMessage, (error as Error).message, 'bad')
    }
  })

  els.settingsButton.addEventListener('click', () => {
    showAdminView('settings')
    loadRuntimeSettings().catch((error: Error) => message(els.runtimeSettingsMessage, error.message, 'bad'))
  })

  els.settingsBackButton.addEventListener('click', () => {
    showAdminView('main')
  })

  els.headerLogoutButton.addEventListener('click', logout)

  els.refreshLicenses.addEventListener('click', () => {
    loadLicenses()
      .then(() => message(els.licenseMessage, '兑换码已刷新', 'ok'))
      .catch((error: Error) => message(els.licenseMessage, error.message, 'bad'))
  })

  els.licenseSearchForm.addEventListener('submit', (event) => {
    event.preventDefault()
    state.licenseSearch = els.licenseSearch.value.trim()
    state.licenseOffset = 0
    state.selectedLicenseIds = []
    state.lastSelectedLicenseId = ''
    loadLicenses()
      .then(() => message(els.licenseMessage, state.licenseSearch ? '搜索完成' : '兑换码已刷新', 'ok'))
      .catch((error: Error) => message(els.licenseMessage, error.message, 'bad'))
  })

  els.clearLicenseSearch.addEventListener('click', () => {
    if (!state.licenseSearch && !els.licenseSearch.value.trim()) return
    els.licenseSearch.value = ''
    state.licenseSearch = ''
    state.licenseOffset = 0
    state.selectedLicenseIds = []
    state.lastSelectedLicenseId = ''
    loadLicenses()
      .then(() => message(els.licenseMessage, '搜索已清空', 'ok'))
      .catch((error: Error) => message(els.licenseMessage, error.message, 'bad'))
  })

  els.licensePageSize.addEventListener('change', () => {
    state.licensePageSize = els.licensePageSize.value === '50' ? 50 : 20
    localStorage.setItem('lillian-admin-license-page-size', String(state.licensePageSize))
    state.licenseOffset = 0
    state.selectedLicenseIds = []
    state.lastSelectedLicenseId = ''
    loadLicenses().catch((error: Error) => message(els.licenseMessage, error.message, 'bad'))
  })

  els.prevLicensePage.addEventListener('click', () => {
    if (state.licenseOffset <= 0) return
    state.licenseOffset = Math.max(0, state.licenseOffset - state.licensePageSize)
    state.selectedLicenseIds = []
    state.lastSelectedLicenseId = ''
    loadLicenses().catch((error: Error) => message(els.licenseMessage, error.message, 'bad'))
  })

  els.nextLicensePage.addEventListener('click', () => {
    if (!state.licenseHasMore) return
    state.licenseOffset += state.licensePageSize
    state.selectedLicenseIds = []
    state.lastSelectedLicenseId = ''
    loadLicenses().catch((error: Error) => message(els.licenseMessage, error.message, 'bad'))
  })

  els.selectAllLicenses.addEventListener('change', () => {
    state.selectedLicenseIds = els.selectAllLicenses.checked ? state.licenses.map((license) => license.id) : []
    state.lastSelectedLicenseId = ''
    renderLicenses()
  })

  els.deleteSelectedLicenses.addEventListener('click', () => {
    deleteLicenseIds(state.selectedLicenseIds).catch((error: Error) => message(els.licenseMessage, error.message, 'bad'))
  })

  els.walletLookupForm.addEventListener('submit', (event) => {
    event.preventDefault()
    const address = els.walletLookupAddress.value.trim()
    message(els.walletMessage, '正在查询...')
    loadWalletLookup(address)
      .then(() => message(els.walletMessage, '钱包数据已载入', 'ok'))
      .catch((error: Error) => {
        state.walletLookup = null
        renderWalletLookup()
        message(els.walletMessage, error.message, 'bad')
      })
  })

  els.clearWalletLookup.addEventListener('click', () => {
    state.walletLookupAddress = ''
    state.walletLookup = null
    els.walletLookupAddress.value = ''
    renderWalletLookup()
    message(els.walletMessage, '')
  })

  els.refreshProfiles.addEventListener('click', () => {
    loadProfiles()
      .then(() => message(els.profileMessage, '服务商已刷新', 'ok'))
      .catch((error: Error) => message(els.profileMessage, error.message, 'bad'))
  })

  els.refreshRuntimeSettings.addEventListener('click', () => {
    loadRuntimeSettings()
      .then(() => message(els.runtimeSettingsMessage, '运行设置已刷新', 'ok'))
      .catch((error: Error) => message(els.runtimeSettingsMessage, error.message, 'bad'))
  })

  els.runtimeSettingsForm.addEventListener('submit', async (event) => {
    event.preventDefault()
    message(els.runtimeSettingsMessage, '正在保存...')
    try {
      const payload = {
        imageGlobalConcurrency: Number(formValue(els.runtimeSettingsForm, 'imageGlobalConcurrency') || 6),
        imageProviderDefaultConcurrency: Number(formValue(els.runtimeSettingsForm, 'imageProviderDefaultConcurrency') || 2),
        upstreamTimeoutSeconds: Number(formValue(els.runtimeSettingsForm, 'upstreamTimeoutSeconds') || 600),
      }
      const data = await api<RuntimeSettingsResponse>('/admin/runtime-settings', { method: 'POST', body: JSON.stringify(payload) })
      state.runtimeSettings = data.settings || payload
      renderRuntimeSettings()
      message(els.runtimeSettingsMessage, '运行设置已保存', 'ok')
    } catch (error) {
      message(els.runtimeSettingsMessage, (error as Error).message, 'bad')
    }
  })

  document.querySelectorAll<HTMLButtonElement>('.tab').forEach((button) => {
    button.addEventListener('click', () => {
      document.querySelectorAll('.tab').forEach((item) => item.classList.remove('active'))
      document.querySelectorAll('.tabpanel').forEach((panel) => panel.classList.remove('active'))
      button.classList.add('active')
      document.getElementById(`${button.dataset.tab}Panel`)?.classList.add('active')
    })
  })

  els.licenseForm.addEventListener('submit', async (event) => {
    event.preventDefault()
    message(els.licenseMessage, '正在生成...')
    try {
      const payload = {
        serviceCode: formValue(els.licenseForm, 'serviceCode'),
        count: Number(formValue(els.licenseForm, 'count') || 1),
        credits: Number(formValue(els.licenseForm, 'credits') || 5),
        maxConcurrent: Number(formValue(els.licenseForm, 'maxConcurrent') || 6),
        expiresInDays: Number(formValue(els.licenseForm, 'expiresInDays') || 30),
        note: formValue(els.licenseForm, 'note').trim(),
      }
      const data = await api<CreateLicenseResponse>('/admin/licenses', { method: 'POST', body: JSON.stringify(payload) })
      state.createdKeys = data.keys || []
      state.licenseSearch = ''
      state.licenseOffset = 0
      state.selectedLicenseIds = []
      state.lastSelectedLicenseId = ''
      renderCreatedKeys()
      message(els.licenseMessage, `已生成 ${state.createdKeys.length} 个兑换码。`, 'ok')
      await loadLicenses()
    } catch (error) {
      message(els.licenseMessage, (error as Error).message, 'bad')
    }
  })

  els.copyCreatedKeys.addEventListener('click', () => {
    const text = state.createdKeys.map((item) => item.key).join('\n')
    copyText(text)
      .then(() => message(els.licenseMessage, '已复制本次生成的兑换码', 'ok'))
      .catch((error: Error) => message(els.licenseMessage, error.message, 'bad'))
  })

  els.profileForm.addEventListener('submit', async (event) => {
    event.preventDefault()
    message(els.profileMessage, '正在保存...')
    try {
      const payload = {
        id: formValue(els.profileForm, 'id').trim(),
        label: formValue(els.profileForm, 'label').trim(),
        tierBucket: formValue(els.profileForm, 'tierBucket'),
        apiBaseUrl: formValue(els.profileForm, 'apiBaseUrl').trim(),
        apiKey: formValue(els.profileForm, 'apiKey').trim(),
        apiMode: formValue(els.profileForm, 'apiMode'),
        priority: Number(formValue(els.profileForm, 'priority') || 100),
        maxConcurrent: Number(formValue(els.profileForm, 'maxConcurrent') || 0),
        status: formValue(els.profileForm, 'status'),
      }
      const profile = await api<ServiceProfile>('/admin/service-profiles', { method: 'POST', body: JSON.stringify(payload) })
      field<HTMLInputElement>(els.profileForm, 'apiKey').value = ''
      state.editingProfileId = profile.id || payload.id || ''
      field<HTMLInputElement>(els.profileForm, 'id').value = state.editingProfileId
      els.deleteProfileButton.hidden = !state.editingProfileId
      message(els.profileMessage, '服务商已保存', 'ok')
      await loadProfiles()
    } catch (error) {
      message(els.profileMessage, (error as Error).message, 'bad')
    }
  })

  els.resetProfileForm.addEventListener('click', () => resetProfileForm('', false))
  els.newProfile.addEventListener('click', () => resetProfileForm('正在新增服务商，保存后会自动生成内部 ID。', true))

  els.deleteProfileButton.addEventListener('click', async () => {
    const id = state.editingProfileId || formValue(els.profileForm, 'id').trim()
    if (!id) return
    if (!confirm('确定删除这个服务商？删除后不会再参与路由。')) return
    els.deleteProfileButton.disabled = true
    try {
      await api(`/admin/service-profiles/${encodeURIComponent(id)}`, { method: 'DELETE' })
      message(els.profileMessage, '服务商已删除', 'ok')
      resetProfileForm('服务商已删除')
      await loadProfiles()
    } catch (error) {
      message(els.profileMessage, (error as Error).message, 'bad')
    } finally {
      els.deleteProfileButton.disabled = false
    }
  })

  document.addEventListener('click', (event) => {
    void handleDocumentClick(event)
  })
}

function serviceLabel(value: string): string {
  if (value === 'image-2-hd' || value === 'hd') return 'HD 2K/4K'
  return '标清 1K'
}

async function handleDocumentClick(event: MouseEvent): Promise<void> {
  const target = event.target
  if (!(target instanceof HTMLElement)) return

  if (target.dataset.copyCreated) {
    await copyText(target.dataset.copyCreated)
      .then(() => message(els.licenseMessage, '已复制兑换码', 'ok'))
      .catch((error: Error) => message(els.licenseMessage, error.message, 'bad'))
  }

  if (target.dataset.copyLicense) {
    await copyText(target.dataset.copyLicense)
      .then(() => message(els.licenseMessage, '已复制兑换码', 'ok'))
      .catch((error: Error) => message(els.licenseMessage, error.message, 'bad'))
  }

  if (target instanceof HTMLInputElement && target.dataset.licenseSelect) {
    selectLicense(target.dataset.licenseSelect, target.checked, event.shiftKey)
  }

  if (target.dataset.editLicenseNote) {
    state.editingLicenseNoteId = target.dataset.editLicenseNote
    renderLicenses()
    window.setTimeout(() => {
      const input = document.querySelector<HTMLInputElement>(`[data-license-note-input="${CSS.escape(state.editingLicenseNoteId)}"]`)
      input?.focus()
      input?.select()
    }, 0)
  }

  if (target.dataset.cancelLicenseNote) {
    state.editingLicenseNoteId = ''
    renderLicenses()
  }

  if (target.dataset.saveLicenseNote && target instanceof HTMLButtonElement) {
    const id = target.dataset.saveLicenseNote
    const noteInput = document.querySelector<HTMLInputElement>(`[data-license-note-input="${CSS.escape(id)}"]`)
    target.disabled = true
    try {
      await api(`/admin/licenses/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify({ note: noteInput?.value || '' }) })
      state.editingLicenseNoteId = ''
      message(els.licenseMessage, '备注已保存', 'ok')
      await loadLicenses()
    } catch (error) {
      message(els.licenseMessage, (error as Error).message, 'bad')
    } finally {
      target.disabled = false
    }
  }

  if (target.dataset.deleteLicense) {
    await deleteLicenseIds([target.dataset.deleteLicense]).catch((error: Error) => message(els.licenseMessage, error.message, 'bad'))
  }

  if (target.dataset.editProfile) {
    const profile = state.profiles.find((item) => item.id === target.dataset.editProfile)
    if (profile) loadProfileForm(profile)
  }

  if (target.dataset.toggleProfile && target instanceof HTMLButtonElement) {
    const profile = state.profiles.find((item) => item.id === target.dataset.toggleProfile)
    if (!profile) return
    const nextStatus = profile.status === 'active' ? 'disabled' : 'active'
    target.disabled = true
    try {
      await api<ServiceProfile>('/admin/service-profiles', {
        method: 'POST',
        body: JSON.stringify({
          id: profile.id,
          label: profile.label,
          tierBucket: profile.tierBucket,
          apiBaseUrl: profile.apiBaseUrl,
          apiMode: profile.apiMode,
          codexCli: Boolean(profile.codexCli),
          priority: profile.priority,
          maxConcurrent: Number(profile.maxConcurrent || 0),
          status: nextStatus,
        }),
      })
      message(els.profileMessage, nextStatus === 'active' ? '服务商已启用' : '服务商已关闭', 'ok')
      await loadProfiles()
    } catch (error) {
      message(els.profileMessage, (error as Error).message, 'bad')
    } finally {
      target.disabled = false
    }
  }

  if (target.dataset.deleteProfile && target instanceof HTMLButtonElement) {
    const profile = state.profiles.find((item) => item.id === target.dataset.deleteProfile)
    const name = profile ? profile.label : target.dataset.deleteProfile
    if (!confirm(`确定删除服务商「${name}」？删除后不会再参与路由。`)) return
    target.disabled = true
    try {
      await api(`/admin/service-profiles/${encodeURIComponent(target.dataset.deleteProfile)}`, { method: 'DELETE' })
      if (state.editingProfileId === target.dataset.deleteProfile) resetProfileForm('服务商已删除')
      await loadProfiles()
    } catch (error) {
      message(els.profileMessage, (error as Error).message, 'bad')
    } finally {
      target.disabled = false
    }
  }
}

function selectLicense(selectedId: string, shouldSelect: boolean, shiftKey: boolean): void {
  const orderedLicenseIds = state.licenses.map((license) => license.id)
  const selectedSet = new Set(state.selectedLicenseIds)
  const anchorIndex = orderedLicenseIds.indexOf(state.lastSelectedLicenseId)
  const currentIndex = orderedLicenseIds.indexOf(selectedId)
  let rangeIds = [selectedId]

  if (shiftKey && anchorIndex >= 0 && currentIndex >= 0) {
    const start = Math.min(anchorIndex, currentIndex)
    const end = Math.max(anchorIndex, currentIndex)
    rangeIds = orderedLicenseIds.slice(start, end + 1)
  }

  if (shouldSelect) rangeIds.forEach((id) => selectedSet.add(id))
  else rangeIds.forEach((id) => selectedSet.delete(id))

  state.selectedLicenseIds = orderedLicenseIds.filter((id) => selectedSet.has(id))
  state.lastSelectedLicenseId = selectedId
  renderLicenses()
}

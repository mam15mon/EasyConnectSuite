import './style.css';
import { matchesResource } from './resource-filter.mjs';

const PREVIEW = new URLSearchParams(location.search).has('preview');

const app = document.querySelector('#app');

app.innerHTML = `
  <header class="top">
    <div class="brand"><span class="brand-symbol" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M12 3 4 6v6c0 5 8 9 8 9s8-4 8-9V6z"/><path d="m8 12 3 3 5-6"/></svg></span><span class="brand-name">VPN 工作台</span><span class="brand-caption">EasyConnect Suite</span></div>
    <div class="top-status" aria-live="polite"><span class="summary-online"><i></i><strong id="global-connected">0</strong> 已连接</span><span id="global-pending">0 连接中</span><span id="global-errors" hidden></span></div>
  </header>
  <main class="workspace">
    <aside class="connections-panel" aria-labelledby="connections-heading">
      <div class="section-head"><h1 id="connections-heading">我的连接 <span id="profile-count" class="panel-count-badge">0</span></h1><button class="add-connection" id="btn-new-profile" aria-label="新建 VPN" title="新建 VPN">＋</button></div>
      <div class="view-filters" aria-label="连接筛选"><button class="active" data-connection-filter="all" aria-pressed="true">全部</button><button data-connection-filter="running" aria-pressed="false">运行中</button></div>
      <div class="connection-list" id="connection-list" aria-label="VPN 连接列表"></div>
      <div class="sidebar-footer"><button class="btn-plain danger" id="btn-stop-all" disabled>断开全部连接</button><button class="log-directory" id="log-path-hint" title="复制日志目录">日志目录</button><span id="preview-indicator" hidden>演示模式 · 模拟数据</span></div>
    </aside>
    <section class="detail-workspace" aria-labelledby="detail-name">
      <div class="detail-header"><div><h2 id="detail-name">选择一条 VPN</h2><span id="detail-gateway" class="subtle"></span></div><div class="detail-actions"><button class="btn-plain" id="btn-edit-profile">编辑</button><button class="btn-icon danger" id="btn-del-profile" aria-label="删除当前配置" title="删除当前配置"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13M10 11v5M14 11v5"/></svg></button><button class="master-switch" id="btn-toggle"><span id="btn-label">连接</span></button></div></div>
      <select class="profile-select" id="profile-select" aria-label="切换查看的 VPN" hidden></select>
      <div class="status-strip"><span class="connection-state" id="state-word">未连接</span><span class="detail-duration">连接时长 <span id="uptime-display">00:00:00</span></span></div>
      <p class="state-note" id="state-note">选择或新建配置后连接。</p>
      <div class="telemetry-grid">
        <button class="telemetry-item" id="item-client-ip" title="复制虚拟 IP"><span class="telemetry-label">虚拟 IP</span><span class="telemetry-val pending" id="client-ip-val">—</span></button>
        <button class="telemetry-item" id="item-dns" title="复制内网 DNS"><span class="telemetry-label">内网 DNS</span><span class="telemetry-val pending" id="dns-val">—</span></button>
        <button class="telemetry-item" id="item-socks" title="复制 SOCKS5 地址"><span class="telemetry-label">SOCKS5</span><span class="telemetry-val" id="socks-port-val">—</span></button>
        <button class="telemetry-item" id="item-http" title="复制 HTTP 代理地址"><span class="telemetry-label">HTTP 代理</span><span class="telemetry-val" id="http-port-val">—</span></button>
      </div>
      <nav class="workspace-tabs" role="tablist" aria-label="连接详情">
        <button role="tab" id="tab-resources" data-workspace-page="resources" aria-controls="page-resources" aria-selected="true">授权资源</button>
        <button role="tab" id="tab-settings" data-workspace-page="settings" aria-controls="page-settings" aria-selected="false" tabindex="-1">连接设置</button>
        <button role="tab" id="tab-logs" data-workspace-page="logs" aria-controls="page-logs" aria-selected="false" tabindex="-1">运行记录</button>
      </nav>
      <section class="workspace-page resources-panel" id="page-resources" role="tabpanel" aria-labelledby="tab-resources">
        <div class="resource-heading"><span>服务器授权的访问范围 <span class="panel-count-badge" id="route-count"></span></span><button class="btn-plain" id="btn-refresh-resources" title="刷新当前连接的资源视图">刷新</button><p id="resource-owner" hidden></p></div>
        <div class="routes-search-wrap"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="10" cy="10" r="6"/><path d="m15 15 5 5"/></svg><input class="routes-filter" id="routes-filter" type="search" placeholder="搜索 IP、域名或端口" aria-label="筛选当前 VPN 资源" /></div>
        <div class="route-table" id="route-table"><div class="route-empty">连接后显示服务器授权的资源。</div></div>
      </section>
      <section class="workspace-page" id="page-settings" role="tabpanel" aria-labelledby="tab-settings" hidden>
        <p class="page-description">访问方式分别对当前配置生效，断开后可修改。</p>
        <div class="mode-controls">
          <label class="tun-mode-control" id="tun-mode-note" title="开启时需要管理员授权"><div><span class="tun-mode-title">TUN 分流</span><small>系统流量按服务器授权资源进入 VPN。首次开启会提示申请管理员权限。</small></div><input type="checkbox" id="tun-mode-toggle" role="switch" aria-label="TUN 分流" disabled /></label>
          <label class="tun-mode-control" id="proxy-mode-note"><div><span class="tun-mode-title">本地代理</span><small>为当前 VPN 提供独立的 SOCKS5 和 HTTP 端口，可与 TUN 同时使用。</small></div><input type="checkbox" id="proxy-mode-toggle" role="switch" aria-label="本地代理" disabled /></label>
        </div>
      </section>
      <section class="workspace-page logs-panel" id="page-logs" role="tabpanel" aria-labelledby="tab-logs" hidden>
        <div class="resource-heading"><span>当前配置的最近 200 条记录</span><button class="btn-plain" id="btn-refresh-logs">刷新记录</button></div><p class="page-description">记录阶段、停止原因、重连与收发统计。完整日志保存在日志目录。</p>
        <pre id="profile-log-output" class="profile-log-output" tabindex="0" aria-label="当前 VPN 运行日志">连接后显示运行记录。</pre>
      </section>
      <footer class="workspace-footer">关闭窗口后，连接继续在托盘运行。</footer>
    </section>
  </main>
  <!-- Profile 编辑弹窗 -->
  <div class="modal-overlay" id="modal-overlay">
    <div class="modal-box" role="dialog" aria-modal="true" aria-labelledby="modal-title">
      <div class="modal-head">
        <div class="modal-title" id="modal-title">编辑服务器配置</div>
        <button class="modal-close" id="modal-close-btn" aria-label="关闭配置编辑">&times;</button>
      </div>
      <div class="modal-body">
        <input type="hidden" id="form-id" />
        <div class="form-group">
          <label class="form-label" for="form-protocol">VPN 类型</label>
          <select class="form-select" id="form-protocol"><option value="easyconnect">深信服 EasyConnect</option><option value="univpn">华为／联软 UniVPN</option><option value="topsec">天融信 TopSAP（标准模式）</option></select>
        </div>
        <div class="form-group" id="form-domain-container" hidden>
          <label class="form-label" for="form-domain">UniVPN 认证域（留空使用服务器地址）</label>
          <input class="form-input" id="form-domain" placeholder="网关配置的认证域" />
          <small>当前支持账号密码与 IPv4；内网域名需系统已有 DNS 配置。</small>
        </div>
        <div id="form-cert-container" hidden>
          <label class="toggle-row">
            <input type="checkbox" id="form-insecure-skip-verify" />
            <span>忽略服务器证书校验（不安全）</span>
          </label>
          <small>仅对当前配置生效；开启后无法验证网关身份，账号密码可能被冒充的网关获取。</small>
        </div>
        <div class="form-group">
          <label class="form-label" for="form-name">节点名称（备注）</label>
          <input type="text" class="form-input" id="form-name" placeholder="例如：公司生产环境、学校内网" />
        </div>
        <div class="form-row">
          <div class="form-group">
            <label class="form-label" for="form-server">VPN 服务器地址</label>
            <input type="text" class="form-input" id="form-server" placeholder="vpn.example.com 或公网 IP" />
          </div>
          <div class="form-group">
            <label class="form-label" for="form-port">服务端口</label>
            <input type="number" class="form-input" id="form-port" value="443" />
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label class="form-label" for="form-username">登录账号</label>
            <input type="text" class="form-input" id="form-username" placeholder="用户名" />
          </div>
          <div class="form-group">
            <label class="form-label" for="form-password">登录密码</label>
            <input type="password" class="form-input" id="form-password" placeholder="••••••••" />
          </div>
        </div>

        <div class="form-row">
          <div class="form-group"><label class="form-label" for="form-socks-port">本地 SOCKS5 端口</label><input class="form-input" id="form-socks-port" type="number" min="1" max="65535" placeholder="自动分配" /></div>
          <div class="form-group"><label class="form-label" for="form-http-port">本地 HTTP 端口</label><input class="form-input" id="form-http-port" type="number" min="1" max="65535" placeholder="自动分配" /></div>
        </div>
        <label class="toggle-row">
          <input type="checkbox" id="form-enable-2fa" />
          <span>二次认证（登录时自动提交 TOTP 动态码）</span>
        </label>
        <div class="form-group" id="form-totp-container" style="display:none">
          <label class="form-label" for="form-totp">TOTP 动态秘钥 (Base32)</label>
          <input type="text" class="form-input" id="form-totp" placeholder="例如 JBSWY3DPEHPK3PXP" />
        </div>

        <label class="toggle-row">
          <input type="checkbox" id="form-enable-keepalive" />
          <span>自定义会话保活地址（默认使用内置轻量探测）</span>
        </label>
        <div class="form-group" id="form-keepalive-container" style="display:none">
          <label class="form-label" for="form-keepalive-url">保活目标 URL（连接期间定期心跳探测）</label>
          <input type="text" class="form-input" id="form-keepalive-url" placeholder="http://172.31.112.1" />
        </div>

        <label class="toggle-row">
          <input type="checkbox" id="form-disable-keepalive" />
          <span>完全禁用会话保活心跳（不推荐）</span>
        </label>


        <div class="form-group">
          <label class="form-label" for="form-nic">出口物理网卡</label>
          <select class="form-select" id="form-nic">
            <option value="以太网">以太网</option>
            <option value="WLAN">WLAN</option>
          </select>
        </div>
      </div>
      <div class="modal-actions">
        <button class="btn-plain" id="btn-modal-cancel">取消</button>
        <button class="btn-solid" id="btn-modal-save">保存配置</button>
      </div>
    </div>
  </div>

  <div class="toast" id="toast"></div>
`;

// ---------- 工具 ----------

function showToast(msg) {
  const toast = document.getElementById('toast');
  toast.innerText = msg;
  toast.classList.add('show');
  setTimeout(() => toast.classList.remove('show'), 2200);
}

function copyToClipboard(text, label) {
  if (!text || text === '—') return;
  navigator.clipboard.writeText(text).then(() => {
    showToast(`已复制 ${label}: ${text}`);
  }).catch(() => {
    showToast(`复制失败: ${text}`);
  });
}

function formatSeconds(sec) {
  const h = Math.floor(sec / 3600).toString().padStart(2, '0');
  const m = Math.floor((sec % 3600) / 60).toString().padStart(2, '0');
  const s = Math.floor(sec % 60).toString().padStart(2, '0');
  return `${h}:${m}:${s}`;
}

// ---------- 元素 ----------

const profileSelect = document.getElementById('profile-select');
const btnToggle = document.getElementById('btn-toggle');
const tunModeToggle = document.getElementById('tun-mode-toggle');
const tunModeNote = document.getElementById('tun-mode-note');
const proxyModeToggle = document.getElementById('proxy-mode-toggle');
const proxyModeNote = document.getElementById('proxy-mode-note');
const btnLabel = document.getElementById('btn-label');
const uptimeDisplay = document.getElementById('uptime-display');
const stateWord = document.getElementById('state-word');
const stateNote = document.getElementById('state-note');
const clientIpVal = document.getElementById('client-ip-val');
const dnsVal = document.getElementById('dns-val');
const socksPortVal = document.getElementById('socks-port-val');
const httpPortVal = document.getElementById('http-port-val');
const routeTable = document.getElementById('route-table');
const routeCount = document.getElementById('route-count');
const routesFilter = document.getElementById('routes-filter');

// 遥测点击复制
document.getElementById('item-client-ip').addEventListener('click', () => {
  copyToClipboard(clientIpVal.innerText, '虚拟 IP');
});
document.getElementById('item-dns').addEventListener('click', () => {
  copyToClipboard(dnsVal.innerText, '内网 DNS');
});
document.getElementById('item-socks').addEventListener('click', () => {
  if (socksPortVal.innerText !== '已关闭') copyToClipboard(socksPortVal.innerText, 'SOCKS5 代理');
});
document.getElementById('item-http').addEventListener('click', () => {
  if (httpPortVal.innerText !== '已关闭') copyToClipboard(httpPortVal.innerText, 'HTTP 代理');
});
document.getElementById('log-path-hint').addEventListener('click', () => {
  copyToClipboard('%APPDATA%\\EasyConnectVPN\\logs', '日志目录');
});

// ---------- 弹窗元素 ----------

const modalOverlay = document.getElementById('modal-overlay');
const modalTitle = document.getElementById('modal-title');
const formId = document.getElementById('form-id');
const formProtocol = document.getElementById('form-protocol');
const formDomain = document.getElementById('form-domain');
const formInsecureSkipVerify = document.getElementById('form-insecure-skip-verify');
const formSocksPort = document.getElementById('form-socks-port');
const formHTTPPort = document.getElementById('form-http-port');
const formName = document.getElementById('form-name');
const formServer = document.getElementById('form-server');
const formPort = document.getElementById('form-port');
const formUsername = document.getElementById('form-username');
const formPassword = document.getElementById('form-password');
const formEnable2FA = document.getElementById('form-enable-2fa');
const formTotpContainer = document.getElementById('form-totp-container');
const formTotp = document.getElementById('form-totp');
const formNic = document.getElementById('form-nic');
const formEnableKeepAlive = document.getElementById('form-enable-keepalive');
const formKeepAliveContainer = document.getElementById('form-keepalive-container');
const formKeepAliveUrl = document.getElementById('form-keepalive-url');
const formDisableKeepAlive = document.getElementById('form-disable-keepalive');

let cachedProfiles = [];
let connectionStatuses = [];
let activeProfile = null;
let modeSaving = false;
let currentStatus = { running: false, state: 'disconnected' };
let detectedInterfaces = ['以太网', 'WLAN'];
let lastResourceRows = [];
let connectionFilter = 'all';
let selectionVersion = 0;
let selectionPending = false;
let selectionQueue = Promise.resolve();
let resourceRequest = 0;
const pendingActions = new Set();
const actionVersions = new Map();
const profileResources = new Map();
let workspacePage = 'resources';
let logRequest = 0;
let modalReturnFocus = null;
const protocolNames = { easyconnect: 'EasyConnect', univpn: 'UniVPN', topsec: 'TopSAP' };

function receiveStatus(status) {
  const index = connectionStatuses.findIndex(s => s.profileId === status.profileId);
  const previous = connectionStatuses[index];
  const changed = !previous || previous.state !== status.state || previous.stage !== status.stage || previous.message !== status.message;
  if (index >= 0) connectionStatuses[index] = status;
  else if (cachedProfiles.some(p => p.id === status.profileId)) connectionStatuses.push(status);
  if (!status.running) {
    profileResources.delete(status.profileId);
    if (status.profileId === activeProfile?.id) { resourceRequest++; renderResources([]); }
  }
  updateStatusUI(status);
  renderConnections();
  if (changed && workspacePage === 'logs' && status.profileId === activeProfile?.id) refreshProfileLogs();
}

function escapeHTML(value) {
  return String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
}

// ---------- 表单联动 ----------

formEnable2FA.addEventListener('change', () => {
  formTotpContainer.style.display = formEnable2FA.checked ? 'block' : 'none';
});
formEnableKeepAlive.addEventListener('change', () => {
  formKeepAliveContainer.style.display = formEnableKeepAlive.checked ? 'block' : 'none';
});
formDisableKeepAlive.addEventListener('change', () => {
  if (formDisableKeepAlive.checked) {
    formEnableKeepAlive.checked = false;
    formKeepAliveContainer.style.display = 'none';
  }
});

// ---------- 状态 ----------

const STATE_WORDS = {
  connected: '已连接',
  connecting: '连接中',
  disconnected: '未连接',
  error: '连接出错',
};

function updateInfoFields(status) {
  if (!status) return;

  const proxyDisabled = status.running ? status.disableProxy : activeProfile?.disableProxy;
  socksPortVal.innerText = proxyDisabled ? '已关闭' : `127.0.0.1:${status.socksPort || activeProfile?.socksPort || 1080}`;
  httpPortVal.innerText = proxyDisabled ? '已关闭' : `127.0.0.1:${status.httpPort || activeProfile?.httpPort || 1081}`;
  socksPortVal.classList.toggle('pending', !!proxyDisabled);
  httpPortVal.classList.toggle('pending', !!proxyDisabled);

  clientIpVal.innerText = status.running && status.clientIp ? status.clientIp : '—';
  dnsVal.innerText = status.running && status.dns ? status.dns : '—';
  clientIpVal.classList.toggle('pending', clientIpVal.innerText === '—');
  dnsVal.classList.toggle('pending', dnsVal.innerText === '—');

}

function updateStatusUI(status) {
  if (status.profileId && status.profileId !== activeProfile?.id) return;
  currentStatus = status;
  uptimeDisplay.innerText = formatSeconds(status.duration || 0);
  document.body.dataset.state = status.state;
  const word = status.state === 'connecting' ? (status.stage || '连接中') : (STATE_WORDS[status.state] || '未连接');
  stateWord.innerText = word;
  stateWord.className = `connection-state ${status.state}`;
  document.getElementById('detail-name').textContent = activeProfile?.name || '选择一条 VPN';
  document.getElementById('detail-gateway').textContent = activeProfile ? `${protocolNames[activeProfile.protocol || 'easyconnect']} / ${activeProfile.server}:${activeProfile.port}` : '';
  document.getElementById('resource-owner').textContent = activeProfile ? `${activeProfile.name} 的授权资源` : '选择连接后查看资源';
  document.getElementById('btn-edit-profile').disabled = !activeProfile || !!status.running || modeSaving;
  document.getElementById('btn-del-profile').disabled = !activeProfile || !!status.running || modeSaving;

  if (status.state === 'connected') {
    btnLabel.innerText = '断开连接';
    btnToggle.disabled = false;
    stateNote.innerText = status.tunMode
      ? status.disableProxy
        ? '按授权资源分流，本地代理已关闭。'
        : '按授权资源分流，本地代理可用。'
      : status.disableProxy
        ? 'VPN 已连接；TUN 和本地代理均已关闭'
        : '会话已建立，可通过本地代理访问授权资源';
  } else if (status.state === 'connecting') {
    const reconnecting = status.stage === '重连中';
    btnLabel.innerText = reconnecting ? '停止重连' : '取消连接';
    btnToggle.disabled = false;
    stateNote.innerText = status.message;
  } else if (status.state === 'error') {
    btnLabel.innerText = '重试连接';
    btnToggle.disabled = false;
    stateNote.innerText = status.lastError || '连接建立失败，可检查配置后重试';
  } else {
    btnLabel.innerText = '连接';
    btnToggle.disabled = false;
    stateNote.innerText = activeProfile
      ? `准备连接节点「${activeProfile.name}」（${activeProfile.server}:${activeProfile.port}）`
      : '通道未建立';
    uptimeDisplay.innerText = '00:00:00';
  }

  stateNote.title = stateNote.innerText;
  tunModeToggle.checked = !!(status.running ? status.tunMode : activeProfile?.tunMode);
  tunModeToggle.disabled = modeSaving || selectionPending || status.running || !activeProfile;
  proxyModeToggle.checked = !(status.running ? status.disableProxy : activeProfile?.disableProxy);
  proxyModeToggle.disabled = modeSaving || selectionPending || status.running || !activeProfile;
  proxyModeNote.title = status.running
    ? '断开连接后可切换本地代理'
    : 'SOCKS5 / HTTP，可与 TUN 同时开启';
  profileSelect.disabled = modeSaving;
  btnToggle.disabled = modeSaving || !activeProfile || (pendingActions.has(activeProfile?.id) && status.state !== 'connecting');
  tunModeNote.title = modeSaving
    ? '正在保存设置或等待管理员授权…'
    : status.running
      ? '断开连接后可切换模式'
      : '开启时需要管理员授权';
  updateInfoFields(status);
  renderConnections();
}

// ---------- Profile ----------

async function refreshProfiles() {
  const version = selectionVersion;
  const appConfig = await window.go.main.App.GetAppConfig();
  if (version !== selectionVersion) return;
  cachedProfiles = appConfig.profiles || [];
  profileSelect.replaceChildren();
  for (const p of cachedProfiles) {
    const opt = document.createElement('option');
    opt.value = p.id;
    opt.textContent = p.name;
    profileSelect.appendChild(opt);
  }
  const previousProfileId = activeProfile?.id;
  activeProfile = cachedProfiles.find(p => p.id === appConfig.activeProfileId) || cachedProfiles[0];
  if (workspacePage === 'logs' && activeProfile?.id !== previousProfileId) {
    document.getElementById('profile-log-output').textContent = '正在读取运行记录…';
    refreshProfileLogs();
  }
  profileSelect.value = activeProfile?.id || '';
  await refreshConnections();
  if (version !== selectionVersion) return;
  const status = await window.go.main.App.GetStatus();
  if (version === selectionVersion) updateStatusUI(status);
  await refreshResources(false);
}

async function selectProfile(id) {
  if (modeSaving) return;
  const profile = cachedProfiles.find(p => p.id === id);
  if (!profile) return;
  const version = ++selectionVersion;
  selectionPending = true;
  activeProfile = profile;
  if (workspacePage === 'logs') { document.getElementById('profile-log-output').textContent = '正在读取运行记录…'; refreshProfileLogs(); }
  profileSelect.value = id;
  routesFilter.value = '';
  updateStatusUI(connectionStatuses.find(s => s.profileId === id) || {profileId:id, running:false, state:'disconnected'});
  renderResources(profileResources.get(id) || []);
  const selection = selectionQueue.then(() => window.go.main.App.SetActiveProfile(id));
  selectionQueue = selection.catch(() => {});
  try {
    if (!await selection) throw new Error('服务器配置不存在');
    if (version !== selectionVersion) return;
    const status = await window.go.main.App.GetStatus();
    if (version !== selectionVersion) return;
    updateStatusUI(status);
    await refreshResources(false);
  } catch (err) { showToast(String(err)); }
  finally { if (version === selectionVersion) { selectionPending = false; updateStatusUI(currentStatus); } }
}

profileSelect.addEventListener('change', e => selectProfile(e.target.value));

function openEditModal(profile) {
  if (profile && connectionStatuses.some(s => s.profileId === profile.id && s.running)) { showToast('请先断开该配置再编辑'); return; }
  modalReturnFocus = document.activeElement;
  modalTitle.innerText = profile ? '编辑服务器配置' : '新建服务器配置';
  formId.value = profile ? profile.id : '';
  formProtocol.value = profile?.protocol || 'easyconnect';
  formDomain.value = profile?.domain || '';
  formSocksPort.value = profile?.socksPort || '';
  formHTTPPort.value = profile?.httpPort || '';
  formName.value = profile ? profile.name : '新配置';
  formServer.value = profile ? profile.server : '';
  formPort.value = profile ? profile.port : 443;
  formUsername.value = profile ? profile.username : '';
  formPassword.value = profile ? profile.password : '';
  formInsecureSkipVerify.checked = !!profile?.insecureSkipVerify;
  formEnable2FA.checked = profile ? !!profile.enable2FA : false;
  formTotp.value = (profile && profile.totpSecret) ? profile.totpSecret : '';
  formTotpContainer.style.display = formEnable2FA.checked ? 'block' : 'none';

  formEnableKeepAlive.checked = !!(profile && profile.keepAliveUrl);
  formKeepAliveUrl.value = (profile && profile.keepAliveUrl) ? profile.keepAliveUrl : '';
  formKeepAliveContainer.style.display = formEnableKeepAlive.checked ? 'block' : 'none';
  formDisableKeepAlive.checked = profile ? !!profile.disableKeepAlive : false;

  formNic.innerHTML = '';
  const autoOption = document.createElement('option');
  autoOption.value = '';
  autoOption.innerText = '自动选择出口';
  formNic.appendChild(autoOption);
  detectedInterfaces.forEach(name => {
    const opt = document.createElement('option');
    opt.value = name;
    opt.innerText = name;
    if (profile && profile.bindInterface === name) opt.selected = true;
    formNic.appendChild(opt);
  });

  updateProtocolFields();
  modalOverlay.classList.add('show');
  formName.focus();
}

function closeModal() {
  modalOverlay.classList.remove('show');
  if (modalReturnFocus?.isConnected) modalReturnFocus.focus();
}

document.getElementById('btn-edit-profile').addEventListener('click', () => {
  if (activeProfile) openEditModal(activeProfile);
});

document.getElementById('btn-new-profile').addEventListener('click', () => {
  openEditModal(null);
});

document.getElementById('btn-del-profile').addEventListener('click', async () => {
  if (!activeProfile) return;
  if (cachedProfiles.length <= 1) {
    showToast('至少保留一个配置，无法删除');
    return;
  }
  if (confirm(`确定要删除配置「${activeProfile.name}」吗？`)) {
    if (!await window.go.main.App.DeleteProfile(activeProfile.id)) { showToast('请先断开该配置再删除'); return; }
    showToast('配置已删除');
    await refreshProfiles();
  }
});

document.getElementById('modal-close-btn').addEventListener('click', closeModal);
document.getElementById('btn-modal-cancel').addEventListener('click', closeModal);

document.getElementById('btn-modal-save').addEventListener('click', async () => {
  const p = {
    id: formId.value.trim(),
    protocol: formProtocol.value,
    domain: formProtocol.value === "univpn" ? formDomain.value.trim() : "",
    insecureSkipVerify: formProtocol.value !== "easyconnect" && formInsecureSkipVerify.checked,
    socksPort: Number(formSocksPort.value) || 0,
    httpPort: Number(formHTTPPort.value) || 0,
    name: formName.value.trim() || '未命名配置',
    server: formServer.value.trim(),
    port: parseInt(formPort.value) || 443,
    username: formUsername.value.trim(),
    password: formPassword.value,
    enable2FA: formProtocol.value === "easyconnect" && formEnable2FA.checked,
    totpSecret: formTotp.value.trim(),
    bindInterface: formNic.value,
    tunMode: !!cachedProfiles.find(profile => profile.id === formId.value.trim())?.tunMode,
    disableProxy: !!cachedProfiles.find(profile => profile.id === formId.value.trim())?.disableProxy,
    disableZjuConfig: true,
    disableKeepAlive: formDisableKeepAlive.checked,
    keepAliveUrl: formEnableKeepAlive.checked ? formKeepAliveUrl.value.trim() : '',
  };

  if (!p.server || !p.username) {
    showToast('服务器地址和账号不能为空');
    return;
  }

  try { await window.go.main.App.SaveProfile(p); } catch (err) { showToast(String(err)); return; }
  closeModal();
  showToast('配置已保存');
  await refreshProfiles();
});

// ---------- 连接 ----------

async function saveTUNMode(enabled) {
  modeSaving = true;
  updateStatusUI(currentStatus);
  let restarting = false;
  try {
    restarting = await window.go.main.App.SetProfileTUNMode(activeProfile.id, enabled);
    if (restarting) {
      tunModeToggle.checked = enabled;
      tunModeNote.title = '授权成功，正在以管理员身份重新启动…';
      return true;
    }
    await refreshProfiles();
    return false;
  } finally {
    if (!restarting) {
      modeSaving = false;
      updateStatusUI(currentStatus);
    }
  }
}

tunModeToggle.addEventListener('change', async () => {
  const enabled = tunModeToggle.checked;
  try {
    await saveTUNMode(enabled);
  } catch (err) {
    showToast(String(err));
  }
});

proxyModeToggle.addEventListener('change', async () => {
  const enabled = proxyModeToggle.checked;
  modeSaving = true;
  updateStatusUI(currentStatus);
  try {
    await window.go.main.App.SetProxyEnabled(enabled);
    await refreshProfiles();
  } catch (err) {
    showToast(String(err));
  } finally {
    modeSaving = false;
    updateStatusUI(currentStatus);
  }
});

async function toggleConnection(id) {
  if (!id || modeSaving) return;
  const profile = cachedProfiles.find(p => p.id === id);
  if (!profile) return;
  const status = connectionStatuses.find(s => s.profileId === id);
  const stopping = !!status?.running;
  if (pendingActions.has(id) && (!stopping || status.state !== 'connecting')) return;
  const actionVersion = (actionVersions.get(id) || 0) + 1;
  actionVersions.set(id, actionVersion);
  pendingActions.add(id);
  renderConnections();
  if (id === activeProfile?.id) updateStatusUI(currentStatus);
  try {
    if (stopping) {
      const res = await window.go.main.App.StopProfileVPN(id);
      if (res !== 'OK') throw new Error(res);
    } else {
      if (profile.tunMode) {
        modeSaving = true;
        updateStatusUI(currentStatus);
        try {
          if (await window.go.main.App.SetProfileTUNMode(id, true)) return;
        } finally { modeSaving = false; }
      }
      receiveStatus({profileId:id, activeProfile:profile.name, protocol:profile.protocol || 'easyconnect', running:true, state:'connecting', stage:'准备中', message:'正在准备连接…', tunMode:!!profile.tunMode, disableProxy:!!profile.disableProxy});
      const res = await window.go.main.App.StartProfileVPN(id);
      if (actionVersions.get(id) !== actionVersion) return;
      if (res !== 'OK') {
        receiveStatus({profileId:id, running:false, state:'error', lastError:res});
        throw new Error(res);
      }
    }
    await refreshConnections();
    if (id === activeProfile?.id) {
      const version = selectionVersion;
      const updated = await window.go.main.App.GetStatus();
      if (version === selectionVersion) updateStatusUI(updated);
      await refreshResources(false);
    }
  } catch (err) {
    showToast(`${profile.name}：${err}`);
  } finally {
    if (actionVersions.get(id) === actionVersion) pendingActions.delete(id);
    renderConnections();
    updateStatusUI(currentStatus);
  }
}
btnToggle.addEventListener('click', () => toggleConnection(activeProfile?.id));

// ---------- 可达资源 ----------

function renderResources(rows) {
  lastResourceRows = rows || [];
  const keyword = routesFilter.value.trim().toLowerCase();
  const rows2 = keyword
    ? lastResourceRows.filter(r => matchesResource(r, keyword))
    : lastResourceRows;

  if (!lastResourceRows.length) {
    routeTable.innerHTML = `
      <div class="route-empty">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <circle cx="12" cy="12" r="10"/>
          <line x1="2" y1="12" x2="22" y2="12"/>
          <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>
        </svg>
        <span>建立隧道后，在此展示服务端授权访问的内网网段与域名映射</span>
      </div>`;
    routeCount.innerText = '';
    return;
  }

  const kindOf = (r) => String(r.kind || r.Kind || '').toLowerCase();
  const shown = rows2.length;
  routeCount.innerText = keyword ? `${shown}/${lastResourceRows.length}` : `共 ${lastResourceRows.length} 条`;

  if (!shown) {
    routeTable.innerHTML = `
      <div class="route-empty">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <circle cx="11" cy="11" r="8"/>
          <line x1="21" y1="21" x2="16.65" y2="16.65"/>
        </svg>
        <span>未搜索到匹配「${escapeHTML(keyword)}」的资源</span>
      </div>`;
    return;
  }

  const ipRows = rows2.filter(r => kindOf(r) === 'ip');
  const domainRows = rows2.filter(r => kindOf(r) === 'domain');

  const makeRow = (r, isIp) => `
    <div class="route-row">
      <div class="route-target-wrap">
        <svg class="route-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          ${isIp
            ? '<rect x="2" y="2" width="20" height="8" rx="2" ry="2"/><rect x="2" y="14" width="20" height="8" rx="2" ry="2"/>'
            : '<circle cx="12" cy="12" r="10"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>'}
        </svg>
        <span class="route-target" title="${escapeHTML(r.target || r.Target || '')}">${escapeHTML(r.target || r.Target || '')}</span>
      </div>
      <span class="route-badge">${escapeHTML(r.ports || r.Ports || '')} · ${escapeHTML(r.protocol || r.Protocol || '')}</span>
    </div>`;

  const makeGroup = (title, list, isIp) => list.length
    ? `<div class="route-group"><span>${title}</span><span class="route-group-badge">${list.length}</span></div>` +
      list.map(item => makeRow(item, isIp)).join('')
    : '';

  routeTable.innerHTML =
    makeGroup('内网 IP 段', ipRows, true) +
    makeGroup('域名映射解析', domainRows, false);
}

async function refreshResources(notify = true) {
  if (!activeProfile || !window.go?.main?.App) return;
  const id = activeProfile.id;
  const version = selectionVersion;
  const request = ++resourceRequest;
  const refreshBtn = document.getElementById('btn-refresh-resources');
  refreshBtn.classList.add('spinning');
  try {
    const rows = await window.go.main.App.GetVPNResources();
    if (id !== activeProfile?.id || version !== selectionVersion || request !== resourceRequest) return;
    profileResources.set(id, rows || []);
    renderResources(rows);
    if (notify) showToast('资源视图已刷新');
  } catch (err) { showToast(String(err)); }
  finally { if (request === resourceRequest) refreshBtn.classList.remove('spinning'); }
}

routesFilter.addEventListener('input', () => renderResources(lastResourceRows));

document.getElementById('btn-refresh-resources').addEventListener('click', () => {
  refreshResources();
});

// ---------- 预览模式（仅浏览器 ?preview，用于视觉检查） ----------

async function loadPreviewData() {
  const { installPreview } = await import('./preview.mjs');
  installPreview(receiveStatus);
  document.getElementById('preview-indicator').hidden = false;
  detectedInterfaces = await window.go.main.App.GetInterfaces();
  await refreshProfiles();
}

// ---------- 启动 ----------

async function initApp() {
  for (let i = 0; i < 40; i++) {
    if (window.go && window.go.main && window.go.main.App && window.runtime) break;
    await new Promise(r => setTimeout(r, 50));
  }

  if (window.runtime) {
    window.runtime.EventsOn('vpn-status', (status) => {
      receiveStatus(status);
      if (!status.running && status.profileId === activeProfile?.id) refreshResources();
    });

    window.runtime.EventsOn('vpn-resources', (payload) => {
      profileResources.set(payload.profileId, payload.entries || []);
      if (payload.profileId === activeProfile?.id) { resourceRequest++; renderResources(payload.entries); }
    });

    window.runtime.EventsOn('status-tick', status => receiveStatus(status));

    window.runtime.EventsOn('vpn-connections', (statuses) => { connectionStatuses = statuses || []; renderConnections(); });

    window.runtime.EventsOn('profile-switched', async () => {
      await refreshProfiles();
    });
  }

  try {
    if (window.go && window.go.main && window.go.main.App) {
      detectedInterfaces = await window.go.main.App.GetInterfaces();
      await refreshProfiles();

    } else if (PREVIEW) {
      await loadPreviewData();
    }
  } catch (err) {
    console.error('Init error:', err);
    showToast('[初始化异常] ' + err);
  }
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', initApp);
} else {
  initApp();
}

function updateProtocolFields() {
  const uni = formProtocol.value === 'univpn';
  const easy = formProtocol.value === 'easyconnect';
  document.getElementById('form-domain-container').hidden = !uni;
  document.getElementById('form-cert-container').hidden = easy;
  for (const input of [formEnable2FA, formEnableKeepAlive, formDisableKeepAlive]) {
    input.closest('.toggle-row').style.display = easy ? '' : 'none';
  }
  formTotpContainer.style.display = easy && formEnable2FA.checked ? 'block' : 'none';
  formKeepAliveContainer.style.display = easy && formEnableKeepAlive.checked ? 'block' : 'none';
}
formProtocol.addEventListener('change', updateProtocolFields);
async function refreshConnections() {
  connectionStatuses = await window.go.main.App.GetConnections();
  renderConnections();
}
function renderConnections() {
  const list = document.getElementById('connection-list');
  const focused = document.activeElement?.closest('[data-profile-action]');
  const focusID = focused?.dataset.profileId;
  const focusAction = focused?.dataset.profileAction;
  const rows = new Map(Array.from(list.querySelectorAll('.connection-row')).map(row => [row.dataset.profileId, row]));
  const visibleIDs = new Set();
  document.getElementById('profile-count').textContent = cachedProfiles.length;
  document.getElementById('global-connected').textContent = connectionStatuses.filter(s => s.state === 'connected').length;
  document.getElementById('global-pending').textContent = `${connectionStatuses.filter(s => s.state === 'connecting').length} 连接中`;
  const errors = connectionStatuses.filter(s => s.state === 'error').length;
  const errorSummary = document.getElementById('global-errors');
  errorSummary.hidden = !errors;
  errorSummary.textContent = `${errors} 条异常`;
  document.getElementById('btn-stop-all').disabled = modeSaving || !connectionStatuses.some(s => s.running);
  for (const profile of cachedProfiles) {
    const status = connectionStatuses.find(s => s.profileId === profile.id) || {state:'disconnected', running:false};
    if (connectionFilter === 'running' && !status.running) continue;
    let row = rows.get(profile.id);
    if (!row) {
      row = document.createElement('div');
      row.className = 'connection-row';
      row.dataset.profileId = profile.id;
      row.innerHTML = '<div class="connection-identity"><button class="connection-name" data-profile-action="select"></button><div class="connection-meta"><span class="protocol-tag"></span><span class="connection-gateway"></span></div></div><button class="connection-action" data-profile-action="toggle"></button><div class="connection-status-line"><span class="connection-state"></span><span class="connection-time"></span></div>';
    }
    row.classList.toggle('selected', profile.id === activeProfile?.id);
    const state = status.state === 'connecting' ? (status.stage || '连接中') : STATE_WORDS[status.state] || '未连接';
    const name = row.querySelector('.connection-name');
    name.textContent = profile.name;
    name.setAttribute('aria-label', `查看 ${profile.name}`);
    name.setAttribute('aria-pressed', String(profile.id === activeProfile?.id));
    row.querySelector('.protocol-tag').textContent = protocolNames[profile.protocol || 'easyconnect'];
    const gateway = row.querySelector('.connection-gateway');
    gateway.textContent = gateway.title = `${profile.server}:${profile.port}`;
    const action = row.querySelector('.connection-action');
    const actionWord = status.running ? status.state === 'connecting' ? '取消' : '断开' : status.state === 'error' ? '重试' : '连接';
    action.textContent = pendingActions.has(profile.id) && status.state !== 'connecting' ? '处理中' : actionWord;
    action.setAttribute('aria-label', `${actionWord} ${profile.name}`);
    action.classList.toggle('stop', !!status.running);
    const stateLabel = row.querySelector('.connection-state');
    stateLabel.textContent = state;
    stateLabel.className = `connection-state ${status.state}`;
    stateLabel.title = status.lastError || status.message || state;
    row.querySelector('.connection-time').textContent = status.running ? formatSeconds(status.duration || 0) : status.state === 'error' ? '查看详情' : '';
    for (const button of row.querySelectorAll('[data-profile-action]')) {
      button.dataset.profileId = profile.id;
      button.disabled = modeSaving || (button.dataset.profileAction === 'toggle' && pendingActions.has(profile.id) && status.state !== 'connecting');
    }
    const position = visibleIDs.size;
    visibleIDs.add(profile.id);
    if (list.children[position] !== row) list.insertBefore(row, list.children[position] || null);
  }
  for (const [id, row] of rows) if (!visibleIDs.has(id)) row.remove();
  let empty = list.querySelector('.list-empty');
  if (!visibleIDs.size) {
    if (!empty) {
      empty = document.createElement('div');
      empty.className = 'list-empty';
      list.appendChild(empty);
    }
    empty.textContent = cachedProfiles.length ? '没有运行中的连接，可切回“全部”选择 VPN。' : '还没有 VPN 配置，点击“新建 VPN”添加。';
  } else {
    empty?.remove();
  }
  if (focusID) list.querySelector(`[data-profile-id="${CSS.escape(focusID)}"] [data-profile-action="${focusAction}"]`)?.focus({preventScroll:true});
}

document.getElementById('connection-list').addEventListener('click', event => {
  const row = event.target.closest('.connection-row');
  if (!row || modeSaving) return;
  const action = event.target.closest('[data-profile-action="toggle"]');
  if (action) {
    if (!action.disabled) toggleConnection(row.dataset.profileId);
  } else {
    selectProfile(row.dataset.profileId);
  }
});

for (const button of document.querySelectorAll('[data-connection-filter]')) {
  button.addEventListener('click', () => {
    connectionFilter = button.dataset.connectionFilter;
    for (const b of document.querySelectorAll('[data-connection-filter]')) {
      b.classList.toggle('active', b === button);
      b.setAttribute('aria-pressed', String(b === button));
    }
    renderConnections();
  });
}
document.getElementById('btn-stop-all').addEventListener('click', async () => {
  if (!confirm('断开所有正在运行的 VPN？')) return;
  try { await window.go.main.App.StopAllVPN(); await refreshProfiles(); }
  catch (err) { showToast(String(err)); }
});

function activateWorkspacePage(page) {
 workspacePage = page;
 for (const button of document.querySelectorAll('[data-workspace-page]')) {
  const selected = button.dataset.workspacePage === page;
  button.setAttribute('aria-selected',String(selected)); button.tabIndex = selected ? 0 : -1;
 }
 for (const panel of document.querySelectorAll('.workspace-page')) panel.hidden = panel.id !== `page-${page}`;
 if (page === 'logs') refreshProfileLogs();
}
const workspaceTabs = Array.from(document.querySelectorAll('[data-workspace-page]'));
for (const button of workspaceTabs) {
 button.addEventListener('click',()=>activateWorkspacePage(button.dataset.workspacePage));
 button.addEventListener('keydown',event=>{
  if (!['ArrowLeft','ArrowRight','Home','End'].includes(event.key)) return;
  event.preventDefault();
  const index = event.key === 'Home' ? 0 : event.key === 'End' ? workspaceTabs.length-1 : (workspaceTabs.indexOf(button)+(event.key === 'ArrowRight'?1:-1)+workspaceTabs.length)%workspaceTabs.length;
  workspaceTabs[index].focus(); workspaceTabs[index].click();
 });
}
async function refreshProfileLogs() {
 if (!activeProfile || !window.go?.main?.App) return;
 const id = activeProfile.id;
 const request = ++logRequest;
 const output = document.getElementById('profile-log-output');
 const refresh = document.getElementById('btn-refresh-logs');
 refresh.disabled = true;
 try {
  const lines = await window.go.main.App.GetProfileLogs(id);
  if (request !== logRequest || id !== activeProfile?.id) return;
  const followLatest = output.scrollHeight-output.scrollTop-output.clientHeight < 24;
  output.textContent = lines.length ? lines.join('\n') : '暂无当前配置的运行记录。连接新版程序后，阶段、停止原因和收发统计会记录在这里。';
  if (followLatest) output.scrollTop = output.scrollHeight;
 } catch (err) {
  if (request === logRequest && id === activeProfile?.id) output.textContent = `读取记录失败：${err}`;
 } finally { if (request === logRequest) refresh.disabled = false; }
}
document.getElementById('btn-refresh-logs').addEventListener('click',refreshProfileLogs);
setInterval(()=>{ if (workspacePage === 'logs') refreshProfileLogs(); },15000);

document.addEventListener('keydown',event=>{
 if (!modalOverlay.classList.contains('show')) return;
 if (event.key === 'Escape') {event.preventDefault();closeModal();return;}
 if (event.key !== 'Tab') return;
 const controls = Array.from(modalOverlay.querySelectorAll('button,input:not([type="hidden"]),select')).filter(element=>!element.disabled && element.offsetParent !== null);
 const first = controls[0], last = controls.at(-1);
 if (event.shiftKey && document.activeElement === first) {event.preventDefault();last.focus();}
 else if (!event.shiftKey && document.activeElement === last) {event.preventDefault();first.focus();}
});

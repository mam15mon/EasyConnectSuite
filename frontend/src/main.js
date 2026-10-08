import './style.css';

const PREVIEW = new URLSearchParams(location.search).has('preview');

const app = document.querySelector('#app');

app.innerHTML = `
  <header class="top">
    <div class="brand">
      <div class="brand-icon-wrap">
        <svg class="brand-mark" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
          <path d="M9 12l2 2 4-4"/>
        </svg>
      </div>
      <div>
        <span class="brand-name">EasyConnect Suite</span>
      </div>
      <span class="brand-badge">内置网关</span>
    </div>

    <div class="top-status">
      <div class="uptime-badge">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10"/>
          <polyline points="12 6 12 12 16 14"/>
        </svg>
        <span class="uptime" id="uptime-display">00:00:00</span>
      </div>
      <div class="status-pill">
        <span class="lamp" id="state-lamp"></span>
        <span class="top-state-word" id="top-state-word">未连接</span>
      </div>
    </div>
  </header>

  <div class="deck">
    <!-- 左侧控制栏 -->
    <div class="deck-col-left">
      <!-- 隧道核心状态卡片 -->
      <section class="card hero-card">
        <div class="hero-state-header">
          <div class="hero-state-title-wrap">
            <div class="state-radar">
              <span class="state-radar-dot"></span>
              <span class="state-radar-ring"></span>
            </div>
            <span class="state-word" id="state-word">未连接</span>
          </div>
        </div>
        <div class="state-note" id="state-note">通道未建立</div>

        <label class="tun-mode-control">
          <span class="tun-mode-copy">
            <span class="tun-mode-title">TUN 模式</span>
            <span class="tun-mode-note" id="tun-mode-note">自动分流内网流量，保留本地代理</span>
          </span>
          <input type="checkbox" id="tun-mode-toggle" role="switch" aria-label="TUN 模式" disabled />
        </label>

        <div class="master-btn-wrap">
          <button class="master-switch" id="btn-toggle">
            <svg id="btn-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
              <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/>
            </svg>
            <span id="btn-label">建立隧道</span>
          </button>
        </div>

        <div class="telemetry-grid">
          <div class="telemetry-item" id="item-client-ip" title="点击复制虚拟内网 IP">
            <div class="telemetry-label-row">
              <span class="telemetry-label">虚拟内网 IP</span>
              <svg class="copy-hint-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2"/>
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
              </svg>
            </div>
            <span class="telemetry-val pending" id="client-ip-val">—</span>
          </div>

          <div class="telemetry-item" id="item-dns" title="点击复制内网 DNS">
            <div class="telemetry-label-row">
              <span class="telemetry-label">内网 DNS</span>
              <svg class="copy-hint-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2"/>
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
              </svg>
            </div>
            <span class="telemetry-val pending" id="dns-val">—</span>
          </div>

          <div class="telemetry-item" id="item-socks" title="点击复制 SOCKS5 地址">
            <div class="telemetry-label-row">
              <span class="telemetry-label">SOCKS5 代理</span>
              <svg class="copy-hint-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2"/>
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
              </svg>
            </div>
            <span class="telemetry-val" id="socks-port-val">127.0.0.1:1080</span>
          </div>

          <div class="telemetry-item" id="item-http" title="点击复制 HTTP 代理地址">
            <div class="telemetry-label-row">
              <span class="telemetry-label">HTTP 代理</span>
              <svg class="copy-hint-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2"/>
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
              </svg>
            </div>
            <span class="telemetry-val" id="http-port-val">127.0.0.1:1081</span>
          </div>
        </div>
      </section>

      <!-- 服务器节点配置卡片 -->
      <section class="card profiles-card">
        <div class="card-head">
          <div class="card-title-group">
            <svg class="card-title-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="2" y="2" width="20" height="8" rx="2" ry="2"/>
              <rect x="2" y="14" width="20" height="8" rx="2" ry="2"/>
              <line x1="6" y1="6" x2="6.01" y2="6"/>
              <line x1="6" y1="18" x2="6.01" y2="18"/>
            </svg>
            <span class="card-title">节点配置</span>
          </div>
        </div>
        <div class="card-body">
          <div class="profile-select-wrap">
            <select class="profile-select" id="profile-select"></select>
            <svg class="profile-select-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="6 9 12 15 18 9"/>
            </svg>
          </div>
          <div class="profile-actions-row">
            <button class="btn-chip" id="btn-new-profile">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="12" y1="5" x2="12" y2="19"/>
                <line x1="5" y1="12" x2="19" y2="12"/>
              </svg>
              <span>新建</span>
            </button>
            <button class="btn-chip" id="btn-edit-profile">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
                <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
              </svg>
              <span>编辑</span>
            </button>
            <button class="btn-chip danger" id="btn-del-profile" title="删除当前配置">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <polyline points="3 6 5 6 21 6"/>
                <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
              </svg>
              <span>删除</span>
            </button>
          </div>
        </div>
      </section>
    </div>

    <!-- 右侧可达资源网格 -->
    <div class="deck-col-right">
      <section class="card routes-card">
        <div class="card-head">
          <div class="card-title-group">
            <svg class="card-title-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10"/>
              <line x1="2" y1="12" x2="22" y2="12"/>
              <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>
            </svg>
            <span class="card-title">可达内网资源</span>
            <span class="panel-count-badge" id="route-count"></span>
          </div>

          <div class="routes-toolbar">
            <div class="routes-search-wrap">
              <svg class="routes-search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="11" cy="11" r="8"/>
                <line x1="21" y1="21" x2="16.65" y2="16.65"/>
              </svg>
              <input class="routes-filter" id="routes-filter" type="text" placeholder="快速筛选 IP / 域名..." />
            </div>
            <button class="btn-icon-action" id="btn-refresh-resources" title="重新从网关拉取资源列表">
              <svg id="refresh-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <polyline points="23 4 23 10 17 10"/>
                <polyline points="1 20 1 14 7 14"/>
                <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
              </svg>
              <span>刷新</span>
            </button>
          </div>
        </div>

        <div class="route-table" id="route-table">
          <div class="route-empty">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M5 12h14"/>
              <path d="M12 5l7 7-7 7"/>
            </svg>
            <span>建立隧道后，在此展示服务端授权访问的内网网段与域名映射</span>
          </div>
        </div>
      </section>
    </div>
  </div>

  <footer class="hint-bar">
    <div class="hint-left">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <rect x="2" y="3" width="20" height="14" rx="2" ry="2"/>
        <line x1="8" y1="21" x2="16" y2="21"/>
        <line x1="12" y1="17" x2="12" y2="21"/>
      </svg>
      <span>关闭窗口将最小化到系统托盘，网络通道与代理服务保持常驻</span>
    </div>
    <div>
      <span>日志目录：</span>
      <code id="log-path-hint" title="点击复制日志路径">%APPDATA%\\EasyConnectVPN\\logs</code>
    </div>
  </footer>

  <!-- Profile 编辑弹窗 -->
  <div class="modal-overlay" id="modal-overlay">
    <div class="modal-box">
      <div class="modal-head">
        <div class="modal-title" id="modal-title">编辑服务器配置</div>
        <button class="modal-close" id="modal-close-btn">&times;</button>
      </div>
      <div class="modal-body">
        <input type="hidden" id="form-id" />
        <div class="form-group">
          <label class="form-label">节点名称（备注）</label>
          <input type="text" class="form-input" id="form-name" placeholder="例如：公司生产环境、学校内网" />
        </div>
        <div class="form-row">
          <div class="form-group">
            <label class="form-label">VPN 服务器地址</label>
            <input type="text" class="form-input" id="form-server" placeholder="vpn.example.com 或公网 IP" />
          </div>
          <div class="form-group">
            <label class="form-label">服务端口</label>
            <input type="number" class="form-input" id="form-port" value="443" />
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label class="form-label">登录账号</label>
            <input type="text" class="form-input" id="form-username" placeholder="用户名" />
          </div>
          <div class="form-group">
            <label class="form-label">登录密码</label>
            <input type="password" class="form-input" id="form-password" placeholder="••••••••" />
          </div>
        </div>

        <label class="toggle-row">
          <input type="checkbox" id="form-enable-2fa" />
          <span>二次认证（登录时自动提交 TOTP 动态码）</span>
        </label>
        <div class="form-group" id="form-totp-container" style="display:none">
          <label class="form-label">TOTP 动态秘钥 (Base32)</label>
          <input type="text" class="form-input" id="form-totp" placeholder="例如 JBSWY3DPEHPK3PXP" />
        </div>

        <label class="toggle-row">
          <input type="checkbox" id="form-enable-keepalive" />
          <span>自定义会话保活地址（默认使用内置轻量探测）</span>
        </label>
        <div class="form-group" id="form-keepalive-container" style="display:none">
          <label class="form-label">保活目标 URL（连接期间定期心跳探测）</label>
          <input type="text" class="form-input" id="form-keepalive-url" placeholder="http://172.31.112.1" />
        </div>

        <label class="toggle-row">
          <input type="checkbox" id="form-disable-keepalive" />
          <span>完全禁用会话保活心跳（不推荐）</span>
        </label>


        <div class="form-group">
          <label class="form-label">出口物理网卡</label>
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
const btnLabel = document.getElementById('btn-label');
const uptimeDisplay = document.getElementById('uptime-display');
const stateWord = document.getElementById('state-word');
const stateNote = document.getElementById('state-note');
const topStateWord = document.getElementById('top-state-word');
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
  copyToClipboard(socksPortVal.innerText, 'SOCKS5 代理');
});
document.getElementById('item-http').addEventListener('click', () => {
  copyToClipboard(httpPortVal.innerText, 'HTTP 代理');
});
document.getElementById('log-path-hint').addEventListener('click', () => {
  copyToClipboard('%APPDATA%\\EasyConnectVPN\\logs', '日志目录');
});

// ---------- 弹窗元素 ----------

const modalOverlay = document.getElementById('modal-overlay');
const modalTitle = document.getElementById('modal-title');
const formId = document.getElementById('form-id');
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
let activeProfile = null;
let modeSaving = false;
let currentStatus = { running: false, state: 'disconnected' };
let detectedInterfaces = ['以太网', 'WLAN'];
let lastResourceRows = [];

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

  if (status.socksPort) socksPortVal.innerText = '127.0.0.1:' + status.socksPort;
  if (status.httpPort) httpPortVal.innerText = '127.0.0.1:' + status.httpPort;

  if (!status.running || status.state === 'disconnected') {
    clientIpVal.innerText = '—';
    clientIpVal.classList.add('pending');
    dnsVal.innerText = '—';
    dnsVal.classList.add('pending');
  } else {
    if (status.clientIp) {
      clientIpVal.innerText = status.clientIp;
      clientIpVal.classList.remove('pending');
    }
    if (status.dns) {
      dnsVal.innerText = status.dns;
      dnsVal.classList.remove('pending');
    }
  }
}

function updateStatusUI(status) {
  currentStatus = status;
  document.body.dataset.state = status.state;
  const word = status.state === 'connecting' ? status.stage : STATE_WORDS[status.state];
  stateWord.innerText = word;
  topStateWord.innerText = word;

  if (status.state === 'connected') {
    btnLabel.innerText = '断开隧道';
    btnToggle.disabled = false;
    stateNote.innerText = status.tunMode
      ? 'TUN 已连接，按 VPN 资源分流；本地 SOCKS5 / HTTP 代理仍可使用'
      : 'VPN 通道工作正常，流量经本地代理端口直接转发';
  } else if (status.state === 'connecting') {
    btnLabel.innerText = status.stage + '…';
    btnToggle.disabled = true;
    stateNote.innerText = status.message;
  } else if (status.state === 'error') {
    btnLabel.innerText = '重新建立';
    btnToggle.disabled = false;
    stateNote.innerText = status.lastError || '连接建立失败，可检查配置后重试';
  } else {
    btnLabel.innerText = '建立隧道';
    btnToggle.disabled = false;
    stateNote.innerText = activeProfile
      ? `准备连接节点「${activeProfile.name}」（${activeProfile.server}:${activeProfile.port}）`
      : '通道未建立';
    uptimeDisplay.innerText = '00:00:00';
  }

  tunModeToggle.checked = !!(status.running ? status.tunMode : activeProfile?.tunMode);
  tunModeToggle.disabled = modeSaving || status.running || !activeProfile;
  profileSelect.disabled = modeSaving;
  if (modeSaving) btnToggle.disabled = true;
  tunModeNote.innerText = modeSaving
    ? '正在保存设置或等待管理员授权…'
    : status.running
      ? '断开连接后可切换模式'
      : '开启时需要管理员授权；本地代理仍可用';
  updateInfoFields(status);
}

// ---------- Profile ----------

async function refreshProfiles() {
  const appConfig = await window.go.main.App.GetAppConfig();
  cachedProfiles = appConfig.profiles || [];
  profileSelect.innerHTML = '';

  cachedProfiles.forEach(p => {
    const opt = document.createElement('option');
    opt.value = p.id;
    opt.innerText = p.name + (p.enable2FA ? ' [2FA]' : '');
    if (p.id === appConfig.activeProfileId) opt.selected = true;
    profileSelect.appendChild(opt);
  });

  activeProfile = cachedProfiles.find(p => p.id === appConfig.activeProfileId) || cachedProfiles[0];
  if (!currentStatus.running) updateStatusUI(currentStatus);
}

function updateProfileView(profile) {
  if (!profile) return;
  activeProfile = profile;
  if (!currentStatus.running) updateStatusUI(currentStatus);
}

profileSelect.addEventListener('change', async (e) => {
  const id = e.target.value;
  await window.go.main.App.SetActiveProfile(id);
  updateProfileView(cachedProfiles.find(item => item.id === id));
});

function openEditModal(profile) {
  modalTitle.innerText = profile ? '编辑服务器配置' : '新建服务器配置';
  formId.value = profile ? profile.id : '';
  formName.value = profile ? profile.name : '新配置';
  formServer.value = profile ? profile.server : '';
  formPort.value = profile ? profile.port : 443;
  formUsername.value = profile ? profile.username : '';
  formPassword.value = profile ? profile.password : '';
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

  modalOverlay.classList.add('show');
}

function closeModal() {
  modalOverlay.classList.remove('show');
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
    await window.go.main.App.DeleteProfile(activeProfile.id);
    showToast('配置已删除');
    await refreshProfiles();
  }
});

document.getElementById('modal-close-btn').addEventListener('click', closeModal);
document.getElementById('btn-modal-cancel').addEventListener('click', closeModal);

document.getElementById('btn-modal-save').addEventListener('click', async () => {
  const p = {
    id: formId.value.trim(),
    name: formName.value.trim() || '未命名配置',
    server: formServer.value.trim(),
    port: parseInt(formPort.value) || 443,
    username: formUsername.value.trim(),
    password: formPassword.value.trim(),
    enable2FA: formEnable2FA.checked,
    totpSecret: formTotp.value.trim(),
    bindInterface: formNic.value,
    tunMode: !!cachedProfiles.find(profile => profile.id === formId.value.trim())?.tunMode,
    disableZjuConfig: true,
    disableKeepAlive: formDisableKeepAlive.checked,
    keepAliveUrl: formEnableKeepAlive.checked ? formKeepAliveUrl.value.trim() : '',
  };

  if (!p.server || !p.username) {
    showToast('服务器地址和账号不能为空');
    return;
  }

  await window.go.main.App.SaveProfile(p);
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
    restarting = await window.go.main.App.SetTUNMode(enabled);
    if (restarting) {
      tunModeToggle.checked = enabled;
      tunModeNote.innerText = '授权成功，正在以管理员身份重新启动…';
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

btnToggle.addEventListener('click', async () => {
  if (currentStatus.running) {
    await window.go.main.App.StopVPN();
  } else {
    if (activeProfile?.tunMode) {
      try {
        if (await saveTUNMode(true)) return;
      } catch (err) {
        showToast(String(err));
        return;
      }
    }
    updateStatusUI({ running: true, state: 'connecting', stage: '准备中', message: '正在准备连接…', tunMode: !!activeProfile?.tunMode });
    const res = await window.go.main.App.StartVPN();
    if (res !== 'OK') {
      showToast('[连接失败] ' + res);
      updateStatusUI({ running: false, state: 'error', lastError: res });
    }
  }
});

// ---------- 可达资源 ----------

function renderResources(rows) {
  lastResourceRows = rows || [];
  const keyword = routesFilter.value.trim().toLowerCase();
  const rows2 = keyword
    ? lastResourceRows.filter(r => String(r.target || r.Target || '').toLowerCase().includes(keyword))
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
        <span>未搜索到匹配「${keyword}」的资源</span>
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
        <span class="route-target" title="${r.target || r.Target || ''}">${r.target || r.Target || ''}</span>
      </div>
      <span class="route-badge">${(r.ports || r.Ports || '')} · ${r.protocol || r.Protocol || ''}</span>
    </div>`;

  const makeGroup = (title, list, isIp) => list.length
    ? `<div class="route-group"><span>${title}</span><span class="route-group-badge">${list.length}</span></div>` +
      list.map(item => makeRow(item, isIp)).join('')
    : '';

  routeTable.innerHTML =
    makeGroup('内网 IP 段', ipRows, true) +
    makeGroup('域名映射解析', domainRows, false);
}

async function refreshResources() {
  const refreshBtn = document.getElementById('btn-refresh-resources');
  if (refreshBtn) refreshBtn.classList.add('spinning');
  try {
    if (window.go && window.go.main && window.go.main.App) {
      renderResources(await window.go.main.App.GetVPNResources());
      showToast('资源列表已刷新');
    }
  } catch (err) {
    console.error('GetVPNResources error:', err);
  } finally {
    setTimeout(() => {
      if (refreshBtn) refreshBtn.classList.remove('spinning');
    }, 500);
  }
}

routesFilter.addEventListener('input', () => renderResources(lastResourceRows));

document.getElementById('btn-refresh-resources').addEventListener('click', () => {
  refreshResources();
});

// ---------- 预览模式（仅浏览器 ?preview，用于视觉检查） ----------

function loadPreviewData() {
  const demoProfiles = [
    { id: 'p1', name: 'IDC 生产节点 (带2FA)', server: '111.205.145.240', port: 58443, enable2FA: true },
    { id: 'p2', name: '测试开发集群', server: '10.0.0.8', port: 443, enable2FA: false },
  ];
  cachedProfiles = demoProfiles;
  activeProfile = demoProfiles[0];
  profileSelect.innerHTML = demoProfiles.map(p =>
    `<option value="${p.id}">${p.name}${p.enable2FA ? ' [2FA]' : ''}</option>`).join('');

  updateStatusUI({
    running: true, state: 'connected',
    clientIp: '192.168.0.2', dns: '172.31.112.1',
    socksPort: '1080', httpPort: '1081',
  });
  uptimeDisplay.innerText = '08:20:31';

  const demo = [];
  for (let i = 0; i < 14; i++) {
    demo.push({ kind: 'ip', target: `172.31.10${i}.0 ~ 172.31.10${i}.255`, ports: '全部端口', protocol: '全部协议' });
  }
  demo.push({ kind: 'domain', target: 'ops.internal.example.com → 172.31.128.12', ports: '固定解析', protocol: '-' });
  demo.push({ kind: 'domain', target: 'git.internal.example.com', ports: '443', protocol: 'TCP' });
  renderResources(demo);
}

// ---------- 启动 ----------

async function initApp() {
  for (let i = 0; i < 40; i++) {
    if (window.go && window.go.main && window.go.main.App && window.runtime) break;
    await new Promise(r => setTimeout(r, 50));
  }

  if (window.runtime) {
    window.runtime.EventsOn('vpn-status', (status) => {
      updateStatusUI(status);
      if (!status.running) refreshResources();
    });

    window.runtime.EventsOn('vpn-resources', (rows) => {
      renderResources(rows);
    });

    window.runtime.EventsOn('status-tick', (status) => {
      uptimeDisplay.innerText = formatSeconds(status.duration);
      updateInfoFields(status);
    });

    window.runtime.EventsOn('profile-switched', async () => {
      await refreshProfiles();
    });
  }

  try {
    if (window.go && window.go.main && window.go.main.App) {
      detectedInterfaces = await window.go.main.App.GetInterfaces();
      await refreshProfiles();
      updateStatusUI(await window.go.main.App.GetStatus());
      refreshResources();
    } else if (PREVIEW) {
      loadPreviewData();
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

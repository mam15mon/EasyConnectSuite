import './style.css';

const app = document.querySelector('#app');

app.innerHTML = `
  <header class="header">
    <div class="brand">
      <div class="brand-icon">
        <svg viewBox="0 0 24 24"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"/></svg>
      </div>
      <div>
        <div class="brand-title">EasyConnect VPN</div>
        <div class="brand-subtitle">多节点管理 · 用户态隔离架构</div>
      </div>
    </div>
    <div class="header-status">
      <div class="uptime-badge" id="uptime-display">00:00:00</div>
      <div class="status-pill" id="status-pill">
        <span class="status-dot"></span>
        <span id="status-text">未连接</span>
      </div>
    </div>
  </header>

  <div class="main-grid">
    <div class="left-panel">
      <!-- Profile Switcher Card -->
      <div class="card profile-card">
        <div class="card-header-bar">
          <span class="card-title">服务器配置 (Profile)</span>
          <div class="card-header-actions">
            <button class="btn-sm" id="btn-new-profile" title="新建服务器配置">➕ 新建</button>
            <button class="btn-sm" id="btn-edit-profile" title="修改当前配置">✏️ 编辑</button>
            <button class="btn-sm danger" id="btn-del-profile" title="删除当前配置">🗑️</button>
          </div>
        </div>
        <div class="profile-select-wrap">
          <select class="profile-select" id="profile-select"></select>
        </div>
      </div>

      <!-- Action Card -->
      <div class="card action-card">
        <button class="btn-toggle" id="btn-toggle">
          <span id="btn-icon">⚡</span>
          <span id="btn-label">立即连接 VPN</span>
        </button>

        <div class="proxy-info">
          <div class="proxy-pill">
            <div class="proxy-label">SOCKS5 代理</div>
            <div class="proxy-val" id="socks-port-val">127.0.0.1:1080</div>
          </div>
          <div class="proxy-pill">
            <div class="proxy-label">HTTP 代理</div>
            <div class="proxy-val" id="http-port-val">127.0.0.1:1081</div>
          </div>
        </div>

        <div class="proxy-info">
          <div class="proxy-pill">
            <div class="proxy-label">虚拟内网 IP</div>
            <div class="proxy-val" id="client-ip-val">等待握手...</div>
          </div>
          <div class="proxy-pill">
            <div class="proxy-label">内网 DNS</div>
            <div class="proxy-val" id="dns-val">--</div>
          </div>
        </div>
      </div>
    </div>

    <!-- Right Panel: Authorized Resources -->
    <div class="card terminal-card resource-card">
      <div class="terminal-header">
        <div class="terminal-title">
          <div class="terminal-dots">
            <span class="terminal-dot dot-red"></span>
            <span class="terminal-dot dot-yellow"></span>
            <span class="terminal-dot dot-green"></span>
          </div>
          <span>可用内网资源 (服务器授权)</span>
        </div>
        <div class="terminal-actions">
          <button class="btn-terminal" id="btn-refresh-resources" title="刷新资源列表">🔄 刷新</button>
        </div>
      </div>
      <div class="terminal-body" id="resource-body">
        <div class="log-line log-system">连接 VPN 后将在此显示服务器授权访问的内网资源。</div>
      </div>
    </div>
  </div>

  <!-- Profile Edit / Create Modal -->
  <div class="modal-overlay" id="modal-overlay">
    <div class="modal-box">
      <div class="modal-header">
        <div class="modal-title" id="modal-title">编辑服务器配置</div>
        <button class="modal-close" id="modal-close-btn">&times;</button>
      </div>
      <div class="modal-body">
        <input type="hidden" id="form-id" />
        <div class="form-group">
          <label class="form-label">配置名称 (备注标签)</label>
          <input type="text" class="form-input" id="form-name" placeholder="例如：公司生产环境 / 测试环境" />
        </div>
        <div class="form-row">
          <div class="form-group">
            <label class="form-label">VPN 服务器地址 (Host)</label>
            <input type="text" class="form-input" id="form-server" placeholder="例如：vpn.example.com 或 203.0.113.10" />
          </div>
          <div class="form-group">
            <label class="form-label">端口 (Port)</label>
            <input type="number" class="form-input" id="form-port" value="443" />
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label class="form-label">账号 (Username)</label>
            <input type="text" class="form-input" id="form-username" />
          </div>
          <div class="form-group">
            <label class="form-label">密码 (Password)</label>
            <input type="password" class="form-input" id="form-password" />
          </div>
        </div>

        <!-- 2FA Checkbox Toggle -->
        <label class="toggle-row">
          <input type="checkbox" id="form-enable-2fa" />
          <span style="font-size:12px;font-weight:500">启用二次认证 (连接时自动提交 TOTP)</span>
        </label>

        <div class="form-group" id="form-totp-container" style="display:none">
          <label class="form-label">TOTP Secret 密钥 (Base32，连接时自动提交)</label>
          <input type="text" class="form-input" id="form-totp" placeholder="例如：JBSWY3DPEHPK3PXP" />
        </div>

        <!-- KeepAlive Controls -->
        <label class="toggle-row">
          <input type="checkbox" id="form-enable-keepalive" />
          <span style="font-size:12px;font-weight:500">自定义会话保活地址 (默认用引擎内置探测，一般无需修改)</span>
        </label>

        <div class="form-group" id="form-keepalive-container" style="display:none">
          <label class="form-label">保活 URL (连接期间定期通过隧道访问该地址)</label>
          <input type="text" class="form-input" id="form-keepalive-url" placeholder="例如：http://172.31.112.1" />
        </div>

        <label class="toggle-row">
          <input type="checkbox" id="form-disable-keepalive" />
          <span style="font-size:12px;font-weight:500">完全禁用会话保活 (不推荐，长时间空闲可能被服务端断开)</span>
        </label>

        <div class="form-group">
          <label class="form-label">绑定物理网卡 (推荐以太网，防 TUN 冲突)</label>
          <select class="form-select" id="form-nic">
            <option value="以太网">以太网</option>
            <option value="WLAN">WLAN</option>
          </select>
        </div>
      </div>
      <div class="modal-actions">
        <button class="btn-secondary" id="btn-modal-cancel">取消</button>
        <button class="btn-primary" id="btn-modal-save">保存此配置</button>
      </div>
    </div>
  </div>

  <div class="toast" id="toast">已复制到剪贴板</div>
`;

// Helper: Show toast
function showToast(msg) {
  const toast = document.getElementById('toast');
  toast.innerText = msg;
  toast.classList.add('show');
  setTimeout(() => toast.classList.remove('show'), 1800);
}

function formatSeconds(sec) {
  const h = Math.floor(sec / 3600).toString().padStart(2, '0');
  const m = Math.floor((sec % 3600) / 60).toString().padStart(2, '0');
  const s = (sec % 60).toString().padStart(2, '0');
  return `${h}:${m}:${s}`;
}

const resourceBody = document.getElementById('resource-body');

// Render the server-authorized resource list (IP segments / domains).
function renderResources(rows) {
  rows = rows || [];
  if (!rows.length) {
    resourceBody.innerHTML = '<div class="log-line log-system">连接 VPN 后将在此显示服务器授权访问的内网资源。</div>';
    return;
  }

  const ipRows = rows.filter(r => r.kind === 'ip');
  const domainRows = rows.filter(r => r.kind === 'domain');

  const makeRow = (r) => `
    <div class="resource-row">
      <span class="resource-icon">${r.kind === 'ip' ? '🔢' : '🌐'}</span>
      <span class="resource-target">${r.target}</span>
      <span class="resource-meta">${r.ports} · ${r.protocol}</span>
    </div>`;

  const makeGroup = (title, list) => list.length
    ? `<div class="resource-group">${title} (${list.length})</div>` + list.map(makeRow).join('')
    : '';

  resourceBody.innerHTML =
    makeGroup('IP 资源', ipRows) +
    makeGroup('域名资源', domainRows);
}

async function refreshResources() {
  if (!window.go || !window.go.main || !window.go.main.App) return;
  try {
    renderResources(await window.go.main.App.GetVPNResources());
  } catch (err) {
    console.error('GetVPNResources error:', err);
  }
}

document.getElementById('btn-refresh-resources').addEventListener('click', () => {
  refreshResources();
  showToast('资源列表已刷新');
});

// Elements
const profileSelect = document.getElementById('profile-select');

const statusPill = document.getElementById('status-pill');
const statusText = document.getElementById('status-text');
const btnToggle = document.getElementById('btn-toggle');
const btnLabel = document.getElementById('btn-label');
const btnIcon = document.getElementById('btn-icon');
const uptimeDisplay = document.getElementById('uptime-display');
const clientIpVal = document.getElementById('client-ip-val');
const dnsVal = document.getElementById('dns-val');
const socksPortVal = document.getElementById('socks-port-val');
const httpPortVal = document.getElementById('http-port-val');

// Modal Elements
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
let currentStatus = { running: false, state: 'disconnected' };
let detectedInterfaces = ['以太网', 'WLAN'];

// 2FA Checkbox change
formEnable2FA.addEventListener('change', () => {
  formTotpContainer.style.display = formEnable2FA.checked ? 'block' : 'none';
});

// KeepAlive controls: disable wins over custom URL
formEnableKeepAlive.addEventListener('change', () => {
  formKeepAliveContainer.style.display = formEnableKeepAlive.checked ? 'block' : 'none';
});
formDisableKeepAlive.addEventListener('change', () => {
  if (formDisableKeepAlive.checked) {
    formEnableKeepAlive.checked = false;
    formKeepAliveContainer.style.display = 'none';
  }
});

// Sync info pills (proxy ports / IP / DNS) from backend status.
// Called from every vpn-status AND status-tick event so any field that
// arrives late (e.g. DNS parsed after "VPN client started") still lands.
function updateInfoFields(status) {
  if (!status) return;

  if (status.socksPort) socksPortVal.innerText = '127.0.0.1:' + status.socksPort;
  if (status.httpPort) httpPortVal.innerText = '127.0.0.1:' + status.httpPort;

  if (!status.running || status.state === 'disconnected') {
    clientIpVal.innerText = '等待握手...';
    dnsVal.innerText = '--';
  } else {
    if (status.clientIp) clientIpVal.innerText = status.clientIp;
    if (status.dns) dnsVal.innerText = status.dns;
  }
}

// Update Status UI
function updateStatusUI(status) {
  currentStatus = status;
  statusPill.className = 'status-pill ' + status.state;
  
  if (status.state === 'connected') {
    statusText.innerText = '已连接';
    btnToggle.className = 'btn-toggle connected';
    btnLabel.innerText = '断开连接';
    btnIcon.innerText = '⏹';
    btnToggle.disabled = false;
  } else if (status.state === 'connecting') {
    statusText.innerText = '握手中...';
    btnToggle.className = 'btn-toggle';
    btnLabel.innerText = '正在握手协商...';
    btnIcon.innerText = '⏳';
    btnToggle.disabled = true;
  } else {
    statusText.innerText = '未连接';
    btnToggle.className = 'btn-toggle';
    btnLabel.innerText = activeProfile ? `连接 ${activeProfile.name}` : '立即连接 VPN';
    btnIcon.innerText = '⚡';
    btnToggle.disabled = false;
    uptimeDisplay.innerText = '00:00:00';
  }

  updateInfoFields(status);
}

// Refresh Profile Select & UI
async function refreshProfiles() {
  const appConfig = await window.go.main.App.GetAppConfig();
  cachedProfiles = appConfig.profiles || [];
  profileSelect.innerHTML = '';

  cachedProfiles.forEach(p => {
    const opt = document.createElement('option');
    opt.value = p.id;
    opt.innerText = p.enable2FA ? `${p.name} [2FA]` : `${p.name} [直连]`;
    if (p.id === appConfig.activeProfileId) opt.selected = true;
    profileSelect.appendChild(opt);
  });

  activeProfile = cachedProfiles.find(p => p.id === appConfig.activeProfileId) || cachedProfiles[0];
  updateProfileView(activeProfile);
}

// Update Active Profile Display
function updateProfileView(profile) {
  if (!profile) return;
  activeProfile = profile;

  if (!currentStatus.running) {
    btnLabel.innerText = `连接 ${profile.name}`;
  }
}

// Select Profile change
profileSelect.addEventListener('change', async (e) => {
  const id = e.target.value;
  await window.go.main.App.SetActiveProfile(id);
  const p = cachedProfiles.find(item => item.id === id);
  updateProfileView(p);
});

// Open Modal for Edit
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

  // Interfaces
  formNic.innerHTML = '';
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

// Modal Buttons
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
  if (confirm(`确定要删除配置 [${activeProfile.name}] 吗？`)) {
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
    disableZjuConfig: true,
    disableKeepAlive: formDisableKeepAlive.checked,
    keepAliveUrl: formEnableKeepAlive.checked ? formKeepAliveUrl.value.trim() : ''
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

// Toggle VPN Connection
btnToggle.addEventListener('click', async () => {
  if (currentStatus.running) {
    await window.go.main.App.StopVPN();
  } else {
    updateStatusUI({ running: true, state: 'connecting' });
    const res = await window.go.main.App.StartVPN();
    if (res !== 'OK') {
      showToast('[连接失败] ' + res);
      updateStatusUI({ running: false, state: 'disconnected' });
    }
  }
});

async function initApp() {
  // Wait for Wails runtime bridge
  for (let i = 0; i < 40; i++) {
    if (window.go && window.go.main && window.go.main.App && window.runtime) {
      break;
    }
    await new Promise(r => setTimeout(r, 50));
  }

  // Setup event listeners
  if (window.runtime) {
    window.runtime.EventsOn('vpn-status', (status) => {
      updateStatusUI(status);
      if (!status.running) {
        refreshResources();
      }
    });

    // The backend pushes the authorized resource list right after login
    // completes (the "connected" status event fires too early to fetch).
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
      const status = await window.go.main.App.GetStatus();
      updateStatusUI(status);
      refreshResources();
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

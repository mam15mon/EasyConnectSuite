import './style.css';

const PREVIEW = new URLSearchParams(location.search).has('preview');

const app = document.querySelector('#app');

app.innerHTML = `
  <header class="top">
    <div class="brand">
      <svg class="brand-mark" viewBox="0 0 24 24" fill="none" stroke="#1f2328" stroke-width="1.8">
        <circle cx="12" cy="12" r="9"/>
        <circle cx="12" cy="12" r="3.2" fill="#047857" stroke="none"/>
        <path d="M12 3v5.8M12 15.2V21M3 12h5.8M15.2 12H21"/>
      </svg>
      <span class="brand-name">隧道值班台</span>
      <span class="brand-sub">EasyConnect 内网通道</span>
    </div>
    <div class="top-status">
      <span class="uptime" id="uptime-display">00:00:00</span>
      <span class="lamp" id="state-lamp"></span>
      <span class="top-state-word" id="top-state-word">未连接</span>
    </div>
  </header>

  <div class="deck">
    <div class="deck-col-left">
      <!-- 隧道主机板 -->
      <section class="panel master">
        <div class="state-line"><span class="state-word" id="state-word">未连接</span></div>
        <div class="state-note" id="state-note">通道未建立</div>

        <button class="master-switch" id="btn-toggle">
          <span id="btn-label">建立隧道</span>
        </button>

        <dl class="readouts">
          <div class="readout"><dt>虚拟内网 IP</dt><dd class="pending" id="client-ip-val">—</dd></div>
          <div class="readout"><dt>内网 DNS</dt><dd class="pending" id="dns-val">—</dd></div>
          <div class="readout"><dt>SOCKS5 代理</dt><dd id="socks-port-val">127.0.0.1:1080</dd></div>
          <div class="readout"><dt>HTTP 代理</dt><dd id="http-port-val">127.0.0.1:1081</dd></div>
        </dl>
      </section>

      <!-- 服务器配置 -->
      <section class="panel">
        <div class="panel-head">
          <span class="panel-title">服务器配置</span>
          <div style="display:flex;gap:6px">
            <button class="btn-chip" id="btn-new-profile">新建</button>
            <button class="btn-chip" id="btn-edit-profile">编辑</button>
            <button class="btn-chip danger" id="btn-del-profile" title="删除当前配置">删除</button>
          </div>
        </div>
        <div class="profiles-body">
          <div class="profile-row">
            <select class="profile-select" id="profile-select"></select>
          </div>
        </div>
      </section>
    </div>

    <!-- 可达资源 -->
    <div class="deck-col-right">
      <section class="panel routes">
        <div class="panel-head">
          <div>
            <span class="panel-title">可达资源</span>
            <span class="panel-count" id="route-count"></span>
          </div>
          <div style="display:flex;gap:6px;align-items:center">
            <input class="routes-filter" id="routes-filter" type="text" placeholder="筛选 IP / 域名" />
            <button class="btn-icon" id="btn-refresh-resources" title="刷新资源列表">刷新</button>
          </div>
        </div>
        <div class="route-table" id="route-table">
          <div class="route-empty">连接后在此列出服务器授权的内网资源</div>
        </div>
      </section>
    </div>
  </div>

  <footer class="hint-bar">
    <span>关闭窗口即最小化到托盘，隧道保持运行</span>
    <span>日志存于 <code>%APPDATA%\\EasyConnectVPN\\logs</code></span>
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
          <label class="form-label">配置名称（备注）</label>
          <input type="text" class="form-input" id="form-name" placeholder="例如：公司生产环境" />
        </div>
        <div class="form-row">
          <div class="form-group">
            <label class="form-label">VPN 服务器地址</label>
            <input type="text" class="form-input" id="form-server" placeholder="vpn.example.com 或 IP" />
          </div>
          <div class="form-group">
            <label class="form-label">端口</label>
            <input type="number" class="form-input" id="form-port" value="443" />
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label class="form-label">账号</label>
            <input type="text" class="form-input" id="form-username" />
          </div>
          <div class="form-group">
            <label class="form-label">密码</label>
            <input type="password" class="form-input" id="form-password" />
          </div>
        </div>

        <label class="toggle-row">
          <input type="checkbox" id="form-enable-2fa" />
          <span>二次认证（连接时自动提交 TOTP）</span>
        </label>
        <div class="form-group" id="form-totp-container" style="display:none">
          <label class="form-label">TOTP Secret（Base32）</label>
          <input type="text" class="form-input" id="form-totp" placeholder="例如 JBSWY3DPEHPK3PXP" />
        </div>

        <label class="toggle-row">
          <input type="checkbox" id="form-enable-keepalive" />
          <span>自定义会话保活地址（默认用引擎内置探测）</span>
        </label>
        <div class="form-group" id="form-keepalive-container" style="display:none">
          <label class="form-label">保活 URL（连接期间定期经隧道访问）</label>
          <input type="text" class="form-input" id="form-keepalive-url" placeholder="http://172.31.112.1" />
        </div>
        <label class="toggle-row">
          <input type="checkbox" id="form-disable-keepalive" />
          <span>完全禁用会话保活（不推荐）</span>
        </label>

        <div class="form-group">
          <label class="form-label">绑定物理网卡</label>
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

function formatSeconds(sec) {
  const h = Math.floor(sec / 3600).toString().padStart(2, '0');
  const m = Math.floor((sec % 3600) / 60).toString().padStart(2, '0');
  const s = Math.floor(sec % 60).toString().padStart(2, '0');
  return `${h}:${m}:${s}`;
}

// ---------- 元素 ----------

const profileSelect = document.getElementById('profile-select');
const btnToggle = document.getElementById('btn-toggle');
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
  connecting: '握手中',
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
  const word = STATE_WORDS[status.state] || '未连接';
  stateWord.innerText = word;
  topStateWord.innerText = word;

  if (status.state === 'connected') {
    btnLabel.innerText = '断开隧道';
    btnToggle.disabled = false;
    stateNote.innerText = '通道工作正常，流量经代理端口进出';
  } else if (status.state === 'connecting') {
    btnLabel.innerText = '正在握手';
    btnToggle.disabled = true;
    stateNote.innerText = status.lastError || '正在登录并建立隧道…';
  } else if (status.state === 'error') {
    btnLabel.innerText = '重新建立';
    btnToggle.disabled = false;
    stateNote.innerText = status.lastError || '连接失败，可重试';
  } else {
    btnLabel.innerText = '建立隧道';
    btnToggle.disabled = false;
    stateNote.innerText = activeProfile
      ? `将连接 ${activeProfile.name}（${activeProfile.server}:${activeProfile.port}）`
      : '通道未建立';
    uptimeDisplay.innerText = '00:00:00';
  }

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

btnToggle.addEventListener('click', async () => {
  if (currentStatus.running) {
    await window.go.main.App.StopVPN();
  } else {
    updateStatusUI({ running: true, state: 'connecting' });
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
    routeTable.innerHTML = '<div class="route-empty">连接后在此列出服务器授权的内网资源</div>';
    routeCount.innerText = '';
    return;
  }

  const kindOf = (r) => String(r.kind || r.Kind || '').toLowerCase();
  const shown = rows2.length;
  routeCount.innerText = keyword ? `${shown}/${lastResourceRows.length}` : `共 ${lastResourceRows.length} 条`;

  if (!shown) {
    routeTable.innerHTML = `<div class="route-empty">没有匹配「${keyword}」的资源</div>`;
    return;
  }

  const ipRows = rows2.filter(r => kindOf(r) === 'ip');
  const domainRows = rows2.filter(r => kindOf(r) === 'domain');

  const makeRow = (r) => `
    <div class="route-row">
      <span class="route-target">${r.target || r.Target || ''}</span>
      <span class="route-badge">${(r.ports || r.Ports || '')} · ${r.protocol || r.Protocol || ''}</span>
    </div>`;

  const makeGroup = (title, list) => list.length
    ? `<div class="route-group">${title} · ${list.length}</div>` + list.map(makeRow).join('')
    : '';

  routeTable.innerHTML =
    makeGroup('IP 段', ipRows) +
    makeGroup('域名', domainRows);
}

async function refreshResources() {
  if (!window.go || !window.go.main || !window.go.main.App) return;
  try {
    renderResources(await window.go.main.App.GetVPNResources());
  } catch (err) {
    console.error('GetVPNResources error:', err);
  }
}

routesFilter.addEventListener('input', () => renderResources(lastResourceRows));

document.getElementById('btn-refresh-resources').addEventListener('click', () => {
  refreshResources();
});

// ---------- 预览模式（仅浏览器 ?preview，用于视觉检查） ----------

function loadPreviewData() {
  const demoProfiles = [
    { id: 'p1', name: 'IDC 生产 (带2FA)', server: '111.205.145.240', port: 58443, enable2FA: true },
    { id: 'p2', name: '测试环境', server: '10.0.0.8', port: 443, enable2FA: false },
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

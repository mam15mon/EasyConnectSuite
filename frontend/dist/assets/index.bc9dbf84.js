(function(){const t=document.createElement("link").relList;if(t&&t.supports&&t.supports("modulepreload"))return;for(const s of document.querySelectorAll('link[rel="modulepreload"]'))a(s);new MutationObserver(s=>{for(const l of s)if(l.type==="childList")for(const f of l.addedNodes)f.tagName==="LINK"&&f.rel==="modulepreload"&&a(f)}).observe(document,{childList:!0,subtree:!0});function o(s){const l={};return s.integrity&&(l.integrity=s.integrity),s.referrerpolicy&&(l.referrerPolicy=s.referrerpolicy),s.crossorigin==="use-credentials"?l.credentials="include":s.crossorigin==="anonymous"?l.credentials="omit":l.credentials="same-origin",l}function a(s){if(s.ep)return;s.ep=!0;const l=o(s);fetch(s.href,l)}})();const ae=new URLSearchParams(location.search).has("preview"),le=document.querySelector("#app");le.innerHTML=`
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
      <span class="brand-badge">\u5185\u7F6E\u7F51\u5173</span>
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
        <span class="top-state-word" id="top-state-word">\u672A\u8FDE\u63A5</span>
      </div>
    </div>
  </header>

  <div class="deck">
    <!-- \u5DE6\u4FA7\u63A7\u5236\u680F -->
    <div class="deck-col-left">
      <!-- \u96A7\u9053\u6838\u5FC3\u72B6\u6001\u5361\u7247 -->
      <section class="card hero-card">
        <div class="hero-state-header">
          <div class="hero-state-title-wrap">
            <div class="state-radar">
              <span class="state-radar-dot"></span>
              <span class="state-radar-ring"></span>
            </div>
            <span class="state-word" id="state-word">\u672A\u8FDE\u63A5</span>
          </div>
        </div>
        <div class="state-note" id="state-note">\u901A\u9053\u672A\u5EFA\u7ACB</div>

        <label class="tun-mode-control">
          <span class="tun-mode-copy">
            <span class="tun-mode-title">TUN \u6A21\u5F0F</span>
            <span class="tun-mode-note" id="tun-mode-note">\u81EA\u52A8\u5206\u6D41 VPN \u5185\u7F51\u6D41\u91CF</span>
          </span>
          <input type="checkbox" id="tun-mode-toggle" role="switch" aria-label="TUN \u6A21\u5F0F" disabled />
        </label>

        <label class="tun-mode-control">
          <span class="tun-mode-copy">
            <span class="tun-mode-title">\u672C\u5730\u4EE3\u7406</span>
            <span class="tun-mode-note" id="proxy-mode-note">SOCKS5 / HTTP\uFF0C\u53EF\u4E0E TUN \u540C\u65F6\u5F00\u542F</span>
          </span>
          <input type="checkbox" id="proxy-mode-toggle" role="switch" aria-label="\u672C\u5730\u4EE3\u7406" disabled />
        </label>

        <div class="master-btn-wrap">
          <button class="master-switch" id="btn-toggle">
            <svg id="btn-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
              <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/>
            </svg>
            <span id="btn-label">\u5EFA\u7ACB\u96A7\u9053</span>
          </button>
        </div>

        <div class="telemetry-grid">
          <div class="telemetry-item" id="item-client-ip" title="\u70B9\u51FB\u590D\u5236\u865A\u62DF\u5185\u7F51 IP">
            <div class="telemetry-label-row">
              <span class="telemetry-label">\u865A\u62DF\u5185\u7F51 IP</span>
              <svg class="copy-hint-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2"/>
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
              </svg>
            </div>
            <span class="telemetry-val pending" id="client-ip-val">\u2014</span>
          </div>

          <div class="telemetry-item" id="item-dns" title="\u70B9\u51FB\u590D\u5236\u5185\u7F51 DNS">
            <div class="telemetry-label-row">
              <span class="telemetry-label">\u5185\u7F51 DNS</span>
              <svg class="copy-hint-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2"/>
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
              </svg>
            </div>
            <span class="telemetry-val pending" id="dns-val">\u2014</span>
          </div>

          <div class="telemetry-item" id="item-socks" title="\u70B9\u51FB\u590D\u5236 SOCKS5 \u5730\u5740">
            <div class="telemetry-label-row">
              <span class="telemetry-label">SOCKS5 \u4EE3\u7406</span>
              <svg class="copy-hint-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2"/>
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
              </svg>
            </div>
            <span class="telemetry-val" id="socks-port-val">127.0.0.1:1080</span>
          </div>

          <div class="telemetry-item" id="item-http" title="\u70B9\u51FB\u590D\u5236 HTTP \u4EE3\u7406\u5730\u5740">
            <div class="telemetry-label-row">
              <span class="telemetry-label">HTTP \u4EE3\u7406</span>
              <svg class="copy-hint-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2"/>
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>
              </svg>
            </div>
            <span class="telemetry-val" id="http-port-val">127.0.0.1:1081</span>
          </div>
        </div>
      </section>

      <!-- \u670D\u52A1\u5668\u8282\u70B9\u914D\u7F6E\u5361\u7247 -->
      <section class="card profiles-card">
        <div class="card-head">
          <div class="card-title-group">
            <svg class="card-title-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="2" y="2" width="20" height="8" rx="2" ry="2"/>
              <rect x="2" y="14" width="20" height="8" rx="2" ry="2"/>
              <line x1="6" y1="6" x2="6.01" y2="6"/>
              <line x1="6" y1="18" x2="6.01" y2="18"/>
            </svg>
            <span class="card-title">\u8282\u70B9\u914D\u7F6E</span>
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
              <span>\u65B0\u5EFA</span>
            </button>
            <button class="btn-chip" id="btn-edit-profile">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
                <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
              </svg>
              <span>\u7F16\u8F91</span>
            </button>
            <button class="btn-chip danger" id="btn-del-profile" title="\u5220\u9664\u5F53\u524D\u914D\u7F6E">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <polyline points="3 6 5 6 21 6"/>
                <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
              </svg>
              <span>\u5220\u9664</span>
            </button>
          </div>
        </div>
      </section>
    </div>

    <!-- \u53F3\u4FA7\u53EF\u8FBE\u8D44\u6E90\u7F51\u683C -->
    <div class="deck-col-right">
      <section class="card routes-card">
        <div class="card-head">
          <div class="card-title-group">
            <svg class="card-title-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10"/>
              <line x1="2" y1="12" x2="22" y2="12"/>
              <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>
            </svg>
            <span class="card-title">\u53EF\u8FBE\u5185\u7F51\u8D44\u6E90</span>
            <span class="panel-count-badge" id="route-count"></span>
          </div>

          <div class="routes-toolbar">
            <div class="routes-search-wrap">
              <svg class="routes-search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="11" cy="11" r="8"/>
                <line x1="21" y1="21" x2="16.65" y2="16.65"/>
              </svg>
              <input class="routes-filter" id="routes-filter" type="text" placeholder="\u5FEB\u901F\u7B5B\u9009 IP / \u57DF\u540D..." />
            </div>
            <button class="btn-icon-action" id="btn-refresh-resources" title="\u91CD\u65B0\u4ECE\u7F51\u5173\u62C9\u53D6\u8D44\u6E90\u5217\u8868">
              <svg id="refresh-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <polyline points="23 4 23 10 17 10"/>
                <polyline points="1 20 1 14 7 14"/>
                <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>
              </svg>
              <span>\u5237\u65B0</span>
            </button>
          </div>
        </div>

        <div class="route-table" id="route-table">
          <div class="route-empty">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M5 12h14"/>
              <path d="M12 5l7 7-7 7"/>
            </svg>
            <span>\u5EFA\u7ACB\u96A7\u9053\u540E\uFF0C\u5728\u6B64\u5C55\u793A\u670D\u52A1\u7AEF\u6388\u6743\u8BBF\u95EE\u7684\u5185\u7F51\u7F51\u6BB5\u4E0E\u57DF\u540D\u6620\u5C04</span>
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
      <span>\u5173\u95ED\u7A97\u53E3\u5C06\u6700\u5C0F\u5316\u5230\u7CFB\u7EDF\u6258\u76D8\uFF0C\u7F51\u7EDC\u901A\u9053\u4E0E\u4EE3\u7406\u670D\u52A1\u4FDD\u6301\u5E38\u9A7B</span>
    </div>
    <div>
      <span>\u65E5\u5FD7\u76EE\u5F55\uFF1A</span>
      <code id="log-path-hint" title="\u70B9\u51FB\u590D\u5236\u65E5\u5FD7\u8DEF\u5F84">%APPDATA%\\EasyConnectVPN\\logs</code>
    </div>
  </footer>

  <!-- Profile \u7F16\u8F91\u5F39\u7A97 -->
  <div class="modal-overlay" id="modal-overlay">
    <div class="modal-box">
      <div class="modal-head">
        <div class="modal-title" id="modal-title">\u7F16\u8F91\u670D\u52A1\u5668\u914D\u7F6E</div>
        <button class="modal-close" id="modal-close-btn">&times;</button>
      </div>
      <div class="modal-body">
        <input type="hidden" id="form-id" />
        <div class="form-group">
          <label class="form-label">\u8282\u70B9\u540D\u79F0\uFF08\u5907\u6CE8\uFF09</label>
          <input type="text" class="form-input" id="form-name" placeholder="\u4F8B\u5982\uFF1A\u516C\u53F8\u751F\u4EA7\u73AF\u5883\u3001\u5B66\u6821\u5185\u7F51" />
        </div>
        <div class="form-row">
          <div class="form-group">
            <label class="form-label">VPN \u670D\u52A1\u5668\u5730\u5740</label>
            <input type="text" class="form-input" id="form-server" placeholder="vpn.example.com \u6216\u516C\u7F51 IP" />
          </div>
          <div class="form-group">
            <label class="form-label">\u670D\u52A1\u7AEF\u53E3</label>
            <input type="number" class="form-input" id="form-port" value="443" />
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label class="form-label">\u767B\u5F55\u8D26\u53F7</label>
            <input type="text" class="form-input" id="form-username" placeholder="\u7528\u6237\u540D" />
          </div>
          <div class="form-group">
            <label class="form-label">\u767B\u5F55\u5BC6\u7801</label>
            <input type="password" class="form-input" id="form-password" placeholder="\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022" />
          </div>
        </div>

        <label class="toggle-row">
          <input type="checkbox" id="form-enable-2fa" />
          <span>\u4E8C\u6B21\u8BA4\u8BC1\uFF08\u767B\u5F55\u65F6\u81EA\u52A8\u63D0\u4EA4 TOTP \u52A8\u6001\u7801\uFF09</span>
        </label>
        <div class="form-group" id="form-totp-container" style="display:none">
          <label class="form-label">TOTP \u52A8\u6001\u79D8\u94A5 (Base32)</label>
          <input type="text" class="form-input" id="form-totp" placeholder="\u4F8B\u5982 JBSWY3DPEHPK3PXP" />
        </div>

        <label class="toggle-row">
          <input type="checkbox" id="form-enable-keepalive" />
          <span>\u81EA\u5B9A\u4E49\u4F1A\u8BDD\u4FDD\u6D3B\u5730\u5740\uFF08\u9ED8\u8BA4\u4F7F\u7528\u5185\u7F6E\u8F7B\u91CF\u63A2\u6D4B\uFF09</span>
        </label>
        <div class="form-group" id="form-keepalive-container" style="display:none">
          <label class="form-label">\u4FDD\u6D3B\u76EE\u6807 URL\uFF08\u8FDE\u63A5\u671F\u95F4\u5B9A\u671F\u5FC3\u8DF3\u63A2\u6D4B\uFF09</label>
          <input type="text" class="form-input" id="form-keepalive-url" placeholder="http://172.31.112.1" />
        </div>

        <label class="toggle-row">
          <input type="checkbox" id="form-disable-keepalive" />
          <span>\u5B8C\u5168\u7981\u7528\u4F1A\u8BDD\u4FDD\u6D3B\u5FC3\u8DF3\uFF08\u4E0D\u63A8\u8350\uFF09</span>
        </label>


        <div class="form-group">
          <label class="form-label">\u51FA\u53E3\u7269\u7406\u7F51\u5361</label>
          <select class="form-select" id="form-nic">
            <option value="\u4EE5\u592A\u7F51">\u4EE5\u592A\u7F51</option>
            <option value="WLAN">WLAN</option>
          </select>
        </div>
      </div>
      <div class="modal-actions">
        <button class="btn-plain" id="btn-modal-cancel">\u53D6\u6D88</button>
        <button class="btn-solid" id="btn-modal-save">\u4FDD\u5B58\u914D\u7F6E</button>
      </div>
    </div>
  </div>

  <div class="toast" id="toast"></div>
`;function r(e){const t=document.getElementById("toast");t.innerText=e,t.classList.add("show"),setTimeout(()=>t.classList.remove("show"),2200)}function T(e,t){!e||e==="\u2014"||navigator.clipboard.writeText(e).then(()=>{r(`\u5DF2\u590D\u5236 ${t}: ${e}`)}).catch(()=>{r(`\u590D\u5236\u5931\u8D25: ${e}`)})}function re(e){const t=Math.floor(e/3600).toString().padStart(2,"0"),o=Math.floor(e%3600/60).toString().padStart(2,"0"),a=Math.floor(e%60).toString().padStart(2,"0");return`${t}:${o}:${a}`}const x=document.getElementById("profile-select"),v=document.getElementById("btn-toggle"),k=document.getElementById("tun-mode-toggle"),R=document.getElementById("tun-mode-note"),S=document.getElementById("proxy-mode-toggle"),de=document.getElementById("proxy-mode-note"),I=document.getElementById("btn-label"),O=document.getElementById("uptime-display"),ce=document.getElementById("state-word"),B=document.getElementById("state-note"),pe=document.getElementById("top-state-word"),w=document.getElementById("client-ip-val"),b=document.getElementById("dns-val"),A=document.getElementById("socks-port-val"),C=document.getElementById("http-port-val"),$=document.getElementById("route-table"),D=document.getElementById("route-count"),W=document.getElementById("routes-filter");document.getElementById("item-client-ip").addEventListener("click",()=>{T(w.innerText,"\u865A\u62DF IP")});document.getElementById("item-dns").addEventListener("click",()=>{T(b.innerText,"\u5185\u7F51 DNS")});document.getElementById("item-socks").addEventListener("click",()=>{A.innerText!=="\u5DF2\u5173\u95ED"&&T(A.innerText,"SOCKS5 \u4EE3\u7406")});document.getElementById("item-http").addEventListener("click",()=>{C.innerText!=="\u5DF2\u5173\u95ED"&&T(C.innerText,"HTTP \u4EE3\u7406")});document.getElementById("log-path-hint").addEventListener("click",()=>{T("%APPDATA%\\EasyConnectVPN\\logs","\u65E5\u5FD7\u76EE\u5F55")});const G=document.getElementById("modal-overlay"),me=document.getElementById("modal-title"),P=document.getElementById("form-id"),j=document.getElementById("form-name"),z=document.getElementById("form-server"),q=document.getElementById("form-port"),J=document.getElementById("form-username"),X=document.getElementById("form-password"),E=document.getElementById("form-enable-2fa"),Y=document.getElementById("form-totp-container"),Z=document.getElementById("form-totp"),L=document.getElementById("form-nic"),g=document.getElementById("form-enable-keepalive"),H=document.getElementById("form-keepalive-container"),_=document.getElementById("form-keepalive-url"),M=document.getElementById("form-disable-keepalive");let m=[],n=null,p=!1,c={running:!1,state:"disconnected"},Q=["\u4EE5\u592A\u7F51","WLAN"],u=[];E.addEventListener("change",()=>{Y.style.display=E.checked?"block":"none"});g.addEventListener("change",()=>{H.style.display=g.checked?"block":"none"});M.addEventListener("change",()=>{M.checked&&(g.checked=!1,H.style.display="none")});const ue={connected:"\u5DF2\u8FDE\u63A5",connecting:"\u8FDE\u63A5\u4E2D",disconnected:"\u672A\u8FDE\u63A5",error:"\u8FDE\u63A5\u51FA\u9519"};function ee(e){if(!e)return;const t=e.running?e.disableProxy:n==null?void 0:n.disableProxy;A.innerText=t?"\u5DF2\u5173\u95ED":"127.0.0.1:1080",C.innerText=t?"\u5DF2\u5173\u95ED":"127.0.0.1:1081",A.classList.toggle("pending",!!t),C.classList.toggle("pending",!!t),!e.running||e.state==="disconnected"?(w.innerText="\u2014",w.classList.add("pending"),b.innerText="\u2014",b.classList.add("pending")):(e.clientIp&&(w.innerText=e.clientIp,w.classList.remove("pending")),e.dns&&(b.innerText=e.dns,b.classList.remove("pending")))}function d(e){c=e,document.body.dataset.state=e.state;const t=e.state==="connecting"?e.stage:ue[e.state];ce.innerText=t,pe.innerText=t,e.state==="connected"?(I.innerText="\u65AD\u5F00\u96A7\u9053",v.disabled=!1,B.innerText=e.tunMode?e.disableProxy?"TUN \u5DF2\u8FDE\u63A5\uFF0C\u6309 VPN \u8D44\u6E90\u5206\u6D41\uFF1B\u672C\u5730\u4EE3\u7406\u5DF2\u5173\u95ED":"TUN \u5DF2\u8FDE\u63A5\uFF0C\u6309 VPN \u8D44\u6E90\u5206\u6D41\uFF1B\u672C\u5730 SOCKS5 / HTTP \u4EE3\u7406\u53EF\u4F7F\u7528":e.disableProxy?"VPN \u5DF2\u8FDE\u63A5\uFF1BTUN \u548C\u672C\u5730\u4EE3\u7406\u5747\u5DF2\u5173\u95ED":"VPN \u901A\u9053\u5DE5\u4F5C\u6B63\u5E38\uFF0C\u6D41\u91CF\u7ECF\u672C\u5730\u4EE3\u7406\u7AEF\u53E3\u76F4\u63A5\u8F6C\u53D1"):e.state==="connecting"?(I.innerText=e.stage+"\u2026",v.disabled=!0,B.innerText=e.message):e.state==="error"?(I.innerText="\u91CD\u65B0\u5EFA\u7ACB",v.disabled=!1,B.innerText=e.lastError||"\u8FDE\u63A5\u5EFA\u7ACB\u5931\u8D25\uFF0C\u53EF\u68C0\u67E5\u914D\u7F6E\u540E\u91CD\u8BD5"):(I.innerText="\u5EFA\u7ACB\u96A7\u9053",v.disabled=!1,B.innerText=n?`\u51C6\u5907\u8FDE\u63A5\u8282\u70B9\u300C${n.name}\u300D\uFF08${n.server}:${n.port}\uFF09`:"\u901A\u9053\u672A\u5EFA\u7ACB",O.innerText="00:00:00"),k.checked=!!(e.running?e.tunMode:n==null?void 0:n.tunMode),k.disabled=p||e.running||!n,S.checked=!(e.running?e.disableProxy:n==null?void 0:n.disableProxy),S.disabled=p||e.running||!n,de.innerText=e.running?"\u65AD\u5F00\u8FDE\u63A5\u540E\u53EF\u5207\u6362\u672C\u5730\u4EE3\u7406":"SOCKS5 / HTTP\uFF0C\u53EF\u4E0E TUN \u540C\u65F6\u5F00\u542F",x.disabled=p,p&&(v.disabled=!0),R.innerText=p?"\u6B63\u5728\u4FDD\u5B58\u8BBE\u7F6E\u6216\u7B49\u5F85\u7BA1\u7406\u5458\u6388\u6743\u2026":e.running?"\u65AD\u5F00\u8FDE\u63A5\u540E\u53EF\u5207\u6362\u6A21\u5F0F":"\u5F00\u542F\u65F6\u9700\u8981\u7BA1\u7406\u5458\u6388\u6743",ee(e)}async function y(){const e=await window.go.main.App.GetAppConfig();m=e.profiles||[],x.innerHTML="",m.forEach(t=>{const o=document.createElement("option");o.value=t.id,o.innerText=t.name+(t.enable2FA?" [2FA]":""),t.id===e.activeProfileId&&(o.selected=!0),x.appendChild(o)}),n=m.find(t=>t.id===e.activeProfileId)||m[0],c.running||d(c)}function ve(e){!e||(n=e,c.running||d(c))}x.addEventListener("change",async e=>{const t=e.target.value;await window.go.main.App.SetActiveProfile(t),ve(m.find(o=>o.id===t))});function te(e){me.innerText=e?"\u7F16\u8F91\u670D\u52A1\u5668\u914D\u7F6E":"\u65B0\u5EFA\u670D\u52A1\u5668\u914D\u7F6E",P.value=e?e.id:"",j.value=e?e.name:"\u65B0\u914D\u7F6E",z.value=e?e.server:"",q.value=e?e.port:443,J.value=e?e.username:"",X.value=e?e.password:"",E.checked=e?!!e.enable2FA:!1,Z.value=e&&e.totpSecret?e.totpSecret:"",Y.style.display=E.checked?"block":"none",g.checked=!!(e&&e.keepAliveUrl),_.value=e&&e.keepAliveUrl?e.keepAliveUrl:"",H.style.display=g.checked?"block":"none",M.checked=e?!!e.disableKeepAlive:!1,L.innerHTML="";const t=document.createElement("option");t.value="",t.innerText="\u81EA\u52A8\u9009\u62E9\u51FA\u53E3",L.appendChild(t),Q.forEach(o=>{const a=document.createElement("option");a.value=o,a.innerText=o,e&&e.bindInterface===o&&(a.selected=!0),L.appendChild(a)}),G.classList.add("show")}function U(){G.classList.remove("show")}document.getElementById("btn-edit-profile").addEventListener("click",()=>{n&&te(n)});document.getElementById("btn-new-profile").addEventListener("click",()=>{te(null)});document.getElementById("btn-del-profile").addEventListener("click",async()=>{if(!!n){if(m.length<=1){r("\u81F3\u5C11\u4FDD\u7559\u4E00\u4E2A\u914D\u7F6E\uFF0C\u65E0\u6CD5\u5220\u9664");return}confirm(`\u786E\u5B9A\u8981\u5220\u9664\u914D\u7F6E\u300C${n.name}\u300D\u5417\uFF1F`)&&(await window.go.main.App.DeleteProfile(n.id),r("\u914D\u7F6E\u5DF2\u5220\u9664"),await y())}});document.getElementById("modal-close-btn").addEventListener("click",U);document.getElementById("btn-modal-cancel").addEventListener("click",U);document.getElementById("btn-modal-save").addEventListener("click",async()=>{var t,o;const e={id:P.value.trim(),name:j.value.trim()||"\u672A\u547D\u540D\u914D\u7F6E",server:z.value.trim(),port:parseInt(q.value)||443,username:J.value.trim(),password:X.value.trim(),enable2FA:E.checked,totpSecret:Z.value.trim(),bindInterface:L.value,tunMode:!!((t=m.find(a=>a.id===P.value.trim()))!=null&&t.tunMode),disableProxy:!!((o=m.find(a=>a.id===P.value.trim()))!=null&&o.disableProxy),disableZjuConfig:!0,disableKeepAlive:M.checked,keepAliveUrl:g.checked?_.value.trim():""};if(!e.server||!e.username){r("\u670D\u52A1\u5668\u5730\u5740\u548C\u8D26\u53F7\u4E0D\u80FD\u4E3A\u7A7A");return}await window.go.main.App.SaveProfile(e),U(),r("\u914D\u7F6E\u5DF2\u4FDD\u5B58"),await y()});async function ne(e){p=!0,d(c);let t=!1;try{return t=await window.go.main.App.SetTUNMode(e),t?(k.checked=e,R.innerText="\u6388\u6743\u6210\u529F\uFF0C\u6B63\u5728\u4EE5\u7BA1\u7406\u5458\u8EAB\u4EFD\u91CD\u65B0\u542F\u52A8\u2026",!0):(await y(),!1)}finally{t||(p=!1,d(c))}}k.addEventListener("change",async()=>{const e=k.checked;try{await ne(e)}catch(t){r(String(t))}});S.addEventListener("change",async()=>{const e=S.checked;p=!0,d(c);try{await window.go.main.App.SetProxyEnabled(e),await y()}catch(t){r(String(t))}finally{p=!1,d(c)}});v.addEventListener("click",async()=>{if(c.running)await window.go.main.App.StopVPN();else{if(n!=null&&n.tunMode)try{if(await ne(!0))return}catch(t){r(String(t));return}d({running:!0,state:"connecting",stage:"\u51C6\u5907\u4E2D",message:"\u6B63\u5728\u51C6\u5907\u8FDE\u63A5\u2026",tunMode:!!(n!=null&&n.tunMode),disableProxy:!!(n!=null&&n.disableProxy)});const e=await window.go.main.App.StartVPN();e!=="OK"&&(r("[\u8FDE\u63A5\u5931\u8D25] "+e),d({running:!1,state:"error",lastError:e}))}});function N(e){u=e||[];const t=W.value.trim().toLowerCase(),o=t?u.filter(i=>String(i.target||i.Target||"").toLowerCase().includes(t)):u;if(!u.length){$.innerHTML=`
      <div class="route-empty">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <circle cx="12" cy="12" r="10"/>
          <line x1="2" y1="12" x2="22" y2="12"/>
          <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>
        </svg>
        <span>\u5EFA\u7ACB\u96A7\u9053\u540E\uFF0C\u5728\u6B64\u5C55\u793A\u670D\u52A1\u7AEF\u6388\u6743\u8BBF\u95EE\u7684\u5185\u7F51\u7F51\u6BB5\u4E0E\u57DF\u540D\u6620\u5C04</span>
      </div>`,D.innerText="";return}const a=i=>String(i.kind||i.Kind||"").toLowerCase(),s=o.length;if(D.innerText=t?`${s}/${u.length}`:`\u5171 ${u.length} \u6761`,!s){$.innerHTML=`
      <div class="route-empty">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <circle cx="11" cy="11" r="8"/>
          <line x1="21" y1="21" x2="16.65" y2="16.65"/>
        </svg>
        <span>\u672A\u641C\u7D22\u5230\u5339\u914D\u300C${t}\u300D\u7684\u8D44\u6E90</span>
      </div>`;return}const l=o.filter(i=>a(i)==="ip"),f=o.filter(i=>a(i)==="domain"),oe=(i,h)=>`
    <div class="route-row">
      <div class="route-target-wrap">
        <svg class="route-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          ${h?'<rect x="2" y="2" width="20" height="8" rx="2" ry="2"/><rect x="2" y="14" width="20" height="8" rx="2" ry="2"/>':'<circle cx="12" cy="12" r="10"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>'}
        </svg>
        <span class="route-target" title="${i.target||i.Target||""}">${i.target||i.Target||""}</span>
      </div>
      <span class="route-badge">${i.ports||i.Ports||""} \xB7 ${i.protocol||i.Protocol||""}</span>
    </div>`,K=(i,h,ie)=>h.length?`<div class="route-group"><span>${i}</span><span class="route-group-badge">${h.length}</span></div>`+h.map(se=>oe(se,ie)).join(""):"";$.innerHTML=K("\u5185\u7F51 IP \u6BB5",l,!0)+K("\u57DF\u540D\u6620\u5C04\u89E3\u6790",f,!1)}async function V(){const e=document.getElementById("btn-refresh-resources");e&&e.classList.add("spinning");try{window.go&&window.go.main&&window.go.main.App&&(N(await window.go.main.App.GetVPNResources()),r("\u8D44\u6E90\u5217\u8868\u5DF2\u5237\u65B0"))}catch(t){console.error("GetVPNResources error:",t)}finally{setTimeout(()=>{e&&e.classList.remove("spinning")},500)}}W.addEventListener("input",()=>N(u));document.getElementById("btn-refresh-resources").addEventListener("click",()=>{V()});function ge(){const e=[{id:"p1",name:"IDC \u751F\u4EA7\u8282\u70B9 (\u5E262FA)",server:"111.205.145.240",port:58443,enable2FA:!0},{id:"p2",name:"\u6D4B\u8BD5\u5F00\u53D1\u96C6\u7FA4",server:"10.0.0.8",port:443,enable2FA:!1}];m=e,n=e[0],x.innerHTML=e.map(o=>`<option value="${o.id}">${o.name}${o.enable2FA?" [2FA]":""}</option>`).join(""),d({running:!0,state:"connected",clientIp:"192.168.0.2",dns:"172.31.112.1",socksPort:"1080",httpPort:"1081"}),O.innerText="08:20:31";const t=[];for(let o=0;o<14;o++)t.push({kind:"ip",target:`172.31.10${o}.0 ~ 172.31.10${o}.255`,ports:"\u5168\u90E8\u7AEF\u53E3",protocol:"\u5168\u90E8\u534F\u8BAE"});t.push({kind:"domain",target:"ops.internal.example.com \u2192 172.31.128.12",ports:"\u56FA\u5B9A\u89E3\u6790",protocol:"-"}),t.push({kind:"domain",target:"git.internal.example.com",ports:"443",protocol:"TCP"}),N(t)}async function F(){for(let e=0;e<40&&!(window.go&&window.go.main&&window.go.main.App&&window.runtime);e++)await new Promise(t=>setTimeout(t,50));window.runtime&&(window.runtime.EventsOn("vpn-status",e=>{d(e),e.running||V()}),window.runtime.EventsOn("vpn-resources",e=>{N(e)}),window.runtime.EventsOn("status-tick",e=>{O.innerText=re(e.duration),ee(e)}),window.runtime.EventsOn("profile-switched",async()=>{await y()}));try{window.go&&window.go.main&&window.go.main.App?(Q=await window.go.main.App.GetInterfaces(),await y(),d(await window.go.main.App.GetStatus()),V()):ae&&ge()}catch(e){console.error("Init error:",e),r("[\u521D\u59CB\u5316\u5F02\u5E38] "+e)}}document.readyState==="loading"?document.addEventListener("DOMContentLoaded",F):F();

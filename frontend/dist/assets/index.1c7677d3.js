(function(){const t=document.createElement("link").relList;if(t&&t.supports&&t.supports("modulepreload"))return;for(const s of document.querySelectorAll('link[rel="modulepreload"]'))i(s);new MutationObserver(s=>{for(const a of s)if(a.type==="childList")for(const c of a.addedNodes)c.tagName==="LINK"&&c.rel==="modulepreload"&&i(c)}).observe(document,{childList:!0,subtree:!0});function n(s){const a={};return s.integrity&&(a.integrity=s.integrity),s.referrerpolicy&&(a.referrerPolicy=s.referrerpolicy),s.crossorigin==="use-credentials"?a.credentials="include":s.crossorigin==="anonymous"?a.credentials="omit":a.credentials="same-origin",a}function i(s){if(s.ep)return;s.ep=!0;const a=n(s);fetch(s.href,a)}})();function M(e){if(!/^\d{1,3}(\.\d{1,3}){3}$/.test(e))return null;const t=e.split(".").map(Number);return t.some(n=>n>255)?null:t.reduce((n,i)=>n*256+i,0)}function he(e,t){const n=String(e.target||e.Target||"").toLowerCase(),i=String(e.kind||e.Kind||"").toLowerCase(),s=M(t);if(s===null||i!=="ip")return n.includes(t);const a=n.split("~").map(N=>N.trim());if(a.length===1)return M(a[0])===s;if(a.length!==2)return!1;const c=M(a[0]),C=M(a[1]);return c!==null&&C!==null&&c<=s&&s<=C}const we=new URLSearchParams(location.search).has("preview"),be=document.querySelector("#app");be.innerHTML=`
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
      <span class="brand-badge">\u591A VPN</span>
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

        <div class="mode-controls">
          <label class="tun-mode-control" id="tun-mode-note" title="\u5F00\u542F\u65F6\u9700\u8981\u7BA1\u7406\u5458\u6388\u6743">
            <span class="tun-mode-title">TUN \u6A21\u5F0F</span>
            <input type="checkbox" id="tun-mode-toggle" role="switch" aria-label="TUN \u6A21\u5F0F" disabled />
          </label>

          <label class="tun-mode-control" id="proxy-mode-note" title="SOCKS5 / HTTP\uFF0C\u53EF\u4E0E TUN \u540C\u65F6\u5F00\u542F">
            <span class="tun-mode-title">\u672C\u5730\u4EE3\u7406</span>
            <input type="checkbox" id="proxy-mode-toggle" role="switch" aria-label="\u672C\u5730\u4EE3\u7406" disabled />
          </label>
        </div>

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
          <div class="connection-list" id="connection-list" aria-label="VPN \u8FDE\u63A5\u5217\u8868"></div>
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
          <label class="form-label" for="form-protocol">VPN \u7C7B\u578B</label>
          <select class="form-select" id="form-protocol"><option value="easyconnect">\u6DF1\u4FE1\u670D EasyConnect</option><option value="univpn">\u534E\u4E3A\uFF0F\u8054\u8F6F UniVPN</option></select>
        </div>
        <div class="form-group" id="form-domain-container" hidden>
          <label class="form-label" for="form-domain">UniVPN \u8BA4\u8BC1\u57DF\uFF08\u7559\u7A7A\u4F7F\u7528\u670D\u52A1\u5668\u5730\u5740\uFF09</label>
          <input class="form-input" id="form-domain" placeholder="\u7F51\u5173\u914D\u7F6E\u7684\u8BA4\u8BC1\u57DF" />
          <small>\u5F53\u524D\u652F\u6301\u8D26\u53F7\u5BC6\u7801\u4E0E IPv4\uFF1B\u5185\u7F51\u57DF\u540D\u9700\u7CFB\u7EDF\u5DF2\u6709 DNS \u914D\u7F6E\u3002</small>
        </div>
        <div id="form-cert-container" hidden>
          <label class="toggle-row">
            <input type="checkbox" id="form-insecure-skip-verify" />
            <span>\u5FFD\u7565\u670D\u52A1\u5668\u8BC1\u4E66\u6821\u9A8C\uFF08\u4E0D\u5B89\u5168\uFF09</span>
          </label>
          <small>\u4EC5\u5BF9\u5F53\u524D UniVPN \u914D\u7F6E\u751F\u6548\uFF1B\u5F00\u542F\u540E\u65E0\u6CD5\u9A8C\u8BC1\u7F51\u5173\u8EAB\u4EFD\uFF0C\u8D26\u53F7\u5BC6\u7801\u53EF\u80FD\u88AB\u5192\u5145\u7684\u7F51\u5173\u83B7\u53D6\u3002</small>
        </div>
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

        <div class="form-row">
          <div class="form-group"><label class="form-label" for="form-socks-port">\u672C\u5730 SOCKS5 \u7AEF\u53E3</label><input class="form-input" id="form-socks-port" type="number" min="1" max="65535" placeholder="\u81EA\u52A8\u5206\u914D" /></div>
          <div class="form-group"><label class="form-label" for="form-http-port">\u672C\u5730 HTTP \u7AEF\u53E3</label><input class="form-input" id="form-http-port" type="number" min="1" max="65535" placeholder="\u81EA\u52A8\u5206\u914D" /></div>
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
`;function r(e){const t=document.getElementById("toast");t.innerText=e,t.classList.add("show"),setTimeout(()=>t.classList.remove("show"),2200)}function L(e,t){!e||e==="\u2014"||navigator.clipboard.writeText(e).then(()=>{r(`\u5DF2\u590D\u5236 ${t}: ${e}`)}).catch(()=>{r(`\u590D\u5236\u5931\u8D25: ${e}`)})}function J(e){const t=Math.floor(e/3600).toString().padStart(2,"0"),n=Math.floor(e%3600/60).toString().padStart(2,"0"),i=Math.floor(e%60).toString().padStart(2,"0");return`${t}:${n}:${i}`}const w=document.getElementById("profile-select"),h=document.getElementById("btn-toggle"),B=document.getElementById("tun-mode-toggle"),X=document.getElementById("tun-mode-note"),U=document.getElementById("proxy-mode-toggle"),ke=document.getElementById("proxy-mode-note"),V=document.getElementById("btn-label"),H=document.getElementById("uptime-display"),xe=document.getElementById("state-word"),y=document.getElementById("state-note"),Ee=document.getElementById("top-state-word"),I=document.getElementById("client-ip-val"),T=document.getElementById("dns-val"),K=document.getElementById("socks-port-val"),D=document.getElementById("http-port-val"),G=document.getElementById("route-table"),z=document.getElementById("route-count"),Y=document.getElementById("routes-filter");document.getElementById("item-client-ip").addEventListener("click",()=>{L(I.innerText,"\u865A\u62DF IP")});document.getElementById("item-dns").addEventListener("click",()=>{L(T.innerText,"\u5185\u7F51 DNS")});document.getElementById("item-socks").addEventListener("click",()=>{K.innerText!=="\u5DF2\u5173\u95ED"&&L(K.innerText,"SOCKS5 \u4EE3\u7406")});document.getElementById("item-http").addEventListener("click",()=>{D.innerText!=="\u5DF2\u5173\u95ED"&&L(D.innerText,"HTTP \u4EE3\u7406")});document.getElementById("log-path-hint").addEventListener("click",()=>{L("%APPDATA%\\EasyConnectVPN\\logs","\u65E5\u5FD7\u76EE\u5F55")});const Z=document.getElementById("modal-overlay"),Ie=document.getElementById("modal-title"),$=document.getElementById("form-id"),g=document.getElementById("form-protocol"),_=document.getElementById("form-domain"),Q=document.getElementById("form-insecure-skip-verify"),ee=document.getElementById("form-socks-port"),te=document.getElementById("form-http-port"),ne=document.getElementById("form-name"),oe=document.getElementById("form-server"),ie=document.getElementById("form-port"),se=document.getElementById("form-username"),ae=document.getElementById("form-password"),f=document.getElementById("form-enable-2fa"),W=document.getElementById("form-totp-container"),le=document.getElementById("form-totp"),O=document.getElementById("form-nic"),u=document.getElementById("form-enable-keepalive"),F=document.getElementById("form-keepalive-container"),re=document.getElementById("form-keepalive-url"),P=document.getElementById("form-disable-keepalive");let p=[],b=[],o=null,m=!1,k={running:!1,state:"disconnected"},de=["\u4EE5\u592A\u7F51","WLAN"],v=[];f.addEventListener("change",()=>{W.style.display=f.checked?"block":"none"});u.addEventListener("change",()=>{F.style.display=u.checked?"block":"none"});P.addEventListener("change",()=>{P.checked&&(u.checked=!1,F.style.display="none")});const ce={connected:"\u5DF2\u8FDE\u63A5",connecting:"\u8FDE\u63A5\u4E2D",disconnected:"\u672A\u8FDE\u63A5",error:"\u8FDE\u63A5\u51FA\u9519"};function pe(e){if(!e)return;const t=e.running?e.disableProxy:o==null?void 0:o.disableProxy;K.innerText=t?"\u5DF2\u5173\u95ED":`127.0.0.1:${e.socksPort||(o==null?void 0:o.socksPort)||1080}`,D.innerText=t?"\u5DF2\u5173\u95ED":`127.0.0.1:${e.httpPort||(o==null?void 0:o.httpPort)||1081}`,K.classList.toggle("pending",!!t),D.classList.toggle("pending",!!t),!e.running||e.state==="disconnected"?(I.innerText="\u2014",I.classList.add("pending"),T.innerText="\u2014",T.classList.add("pending")):(e.clientIp&&(I.innerText=e.clientIp,I.classList.remove("pending")),e.dns&&(T.innerText=e.dns,T.classList.remove("pending")))}function d(e){if(e.profileId&&e.profileId!==(o==null?void 0:o.id))return;k=e,H.innerText=J(e.duration||0),document.body.dataset.state=e.state;const t=e.state==="connecting"?e.stage:ce[e.state];if(xe.innerText=t,Ee.innerText=t,e.state==="connected")V.innerText="\u65AD\u5F00\u96A7\u9053",h.disabled=!1,y.innerText=e.tunMode?e.disableProxy?"TUN \u5DF2\u8FDE\u63A5\uFF0C\u6309 VPN \u8D44\u6E90\u5206\u6D41\uFF1B\u672C\u5730\u4EE3\u7406\u5DF2\u5173\u95ED":"TUN \u5DF2\u8FDE\u63A5\uFF0C\u6309 VPN \u8D44\u6E90\u5206\u6D41\uFF1B\u672C\u5730 SOCKS5 / HTTP \u4EE3\u7406\u53EF\u4F7F\u7528":e.disableProxy?"VPN \u5DF2\u8FDE\u63A5\uFF1BTUN \u548C\u672C\u5730\u4EE3\u7406\u5747\u5DF2\u5173\u95ED":"VPN \u901A\u9053\u5DE5\u4F5C\u6B63\u5E38\uFF0C\u6D41\u91CF\u7ECF\u672C\u5730\u4EE3\u7406\u7AEF\u53E3\u76F4\u63A5\u8F6C\u53D1";else if(e.state==="connecting"){const n=e.stage==="\u91CD\u8FDE\u4E2D";V.innerText=n?"\u505C\u6B62\u91CD\u8FDE":"\u53D6\u6D88\u8FDE\u63A5",h.disabled=!1,y.innerText=e.message}else e.state==="error"?(V.innerText="\u91CD\u65B0\u5EFA\u7ACB",h.disabled=!1,y.innerText=e.lastError||"\u8FDE\u63A5\u5EFA\u7ACB\u5931\u8D25\uFF0C\u53EF\u68C0\u67E5\u914D\u7F6E\u540E\u91CD\u8BD5"):(V.innerText="\u5EFA\u7ACB\u96A7\u9053",h.disabled=!1,y.innerText=o?`\u51C6\u5907\u8FDE\u63A5\u8282\u70B9\u300C${o.name}\u300D\uFF08${o.server}:${o.port}\uFF09`:"\u901A\u9053\u672A\u5EFA\u7ACB",H.innerText="00:00:00");y.title=y.innerText,B.checked=!!(e.running?e.tunMode:o==null?void 0:o.tunMode),B.disabled=m||e.running||!o,U.checked=!(e.running?e.disableProxy:o==null?void 0:o.disableProxy),U.disabled=m||e.running||!o,ke.title=e.running?"\u65AD\u5F00\u8FDE\u63A5\u540E\u53EF\u5207\u6362\u672C\u5730\u4EE3\u7406":"SOCKS5 / HTTP\uFF0C\u53EF\u4E0E TUN \u540C\u65F6\u5F00\u542F",w.disabled=m,m&&(h.disabled=!0),X.title=m?"\u6B63\u5728\u4FDD\u5B58\u8BBE\u7F6E\u6216\u7B49\u5F85\u7BA1\u7406\u5458\u6388\u6743\u2026":e.running?"\u65AD\u5F00\u8FDE\u63A5\u540E\u53EF\u5207\u6362\u6A21\u5F0F":"\u5F00\u542F\u65F6\u9700\u8981\u7BA1\u7406\u5458\u6388\u6743",pe(e)}async function x(){const e=await window.go.main.App.GetAppConfig();p=e.profiles||[],w.innerHTML="",p.forEach(t=>{const n=document.createElement("option");n.value=t.id,n.innerText=t.name+(t.enable2FA?" [2FA]":""),t.id===e.activeProfileId&&(n.selected=!0),w.appendChild(n)}),o=p.find(t=>t.id===e.activeProfileId)||p[0],await Be(),d(await window.go.main.App.GetStatus())}function me(e){!e||(o=e)}w.addEventListener("change",async e=>{const t=e.target.value;await window.go.main.App.SetActiveProfile(t),me(p.find(n=>n.id===t)),d(await window.go.main.App.GetStatus()),S(),A()});function ue(e){if(e&&b.some(n=>n.profileId===e.id&&n.running)){r("\u8BF7\u5148\u65AD\u5F00\u8BE5\u914D\u7F6E\u518D\u7F16\u8F91");return}Ie.innerText=e?"\u7F16\u8F91\u670D\u52A1\u5668\u914D\u7F6E":"\u65B0\u5EFA\u670D\u52A1\u5668\u914D\u7F6E",$.value=e?e.id:"",g.value=(e==null?void 0:e.protocol)||"easyconnect",_.value=(e==null?void 0:e.domain)||"",ee.value=(e==null?void 0:e.socksPort)||"",te.value=(e==null?void 0:e.httpPort)||"",ne.value=e?e.name:"\u65B0\u914D\u7F6E",oe.value=e?e.server:"",ie.value=e?e.port:443,se.value=e?e.username:"",ae.value=e?e.password:"",Q.checked=!!(e!=null&&e.insecureSkipVerify),f.checked=e?!!e.enable2FA:!1,le.value=e&&e.totpSecret?e.totpSecret:"",W.style.display=f.checked?"block":"none",u.checked=!!(e&&e.keepAliveUrl),re.value=e&&e.keepAliveUrl?e.keepAliveUrl:"",F.style.display=u.checked?"block":"none",P.checked=e?!!e.disableKeepAlive:!1,O.innerHTML="";const t=document.createElement("option");t.value="",t.innerText="\u81EA\u52A8\u9009\u62E9\u51FA\u53E3",O.appendChild(t),de.forEach(n=>{const i=document.createElement("option");i.value=n,i.innerText=n,e&&e.bindInterface===n&&(i.selected=!0),O.appendChild(i)}),ge(),Z.classList.add("show")}function j(){Z.classList.remove("show")}document.getElementById("btn-edit-profile").addEventListener("click",()=>{o&&ue(o)});document.getElementById("btn-new-profile").addEventListener("click",()=>{ue(null)});document.getElementById("btn-del-profile").addEventListener("click",async()=>{if(!!o){if(p.length<=1){r("\u81F3\u5C11\u4FDD\u7559\u4E00\u4E2A\u914D\u7F6E\uFF0C\u65E0\u6CD5\u5220\u9664");return}if(confirm(`\u786E\u5B9A\u8981\u5220\u9664\u914D\u7F6E\u300C${o.name}\u300D\u5417\uFF1F`)){if(!await window.go.main.App.DeleteProfile(o.id)){r("\u8BF7\u5148\u65AD\u5F00\u8BE5\u914D\u7F6E\u518D\u5220\u9664");return}r("\u914D\u7F6E\u5DF2\u5220\u9664"),await x()}}});document.getElementById("modal-close-btn").addEventListener("click",j);document.getElementById("btn-modal-cancel").addEventListener("click",j);document.getElementById("btn-modal-save").addEventListener("click",async()=>{var t,n;const e={id:$.value.trim(),protocol:g.value,domain:g.value==="univpn"?_.value.trim():"",insecureSkipVerify:g.value==="univpn"&&Q.checked,socksPort:Number(ee.value)||0,httpPort:Number(te.value)||0,name:ne.value.trim()||"\u672A\u547D\u540D\u914D\u7F6E",server:oe.value.trim(),port:parseInt(ie.value)||443,username:se.value.trim(),password:ae.value,enable2FA:g.value==="easyconnect"&&f.checked,totpSecret:le.value.trim(),bindInterface:O.value,tunMode:!!((t=p.find(i=>i.id===$.value.trim()))!=null&&t.tunMode),disableProxy:!!((n=p.find(i=>i.id===$.value.trim()))!=null&&n.disableProxy),disableZjuConfig:!0,disableKeepAlive:P.checked,keepAliveUrl:u.checked?re.value.trim():""};if(!e.server||!e.username){r("\u670D\u52A1\u5668\u5730\u5740\u548C\u8D26\u53F7\u4E0D\u80FD\u4E3A\u7A7A");return}try{await window.go.main.App.SaveProfile(e)}catch(i){r(String(i));return}j(),r("\u914D\u7F6E\u5DF2\u4FDD\u5B58"),await x()});async function ve(e){m=!0,d(k);let t=!1;try{return t=await window.go.main.App.SetTUNMode(e),t?(B.checked=e,X.title="\u6388\u6743\u6210\u529F\uFF0C\u6B63\u5728\u4EE5\u7BA1\u7406\u5458\u8EAB\u4EFD\u91CD\u65B0\u542F\u52A8\u2026",!0):(await x(),!1)}finally{t||(m=!1,d(k))}}B.addEventListener("change",async()=>{const e=B.checked;try{await ve(e)}catch(t){r(String(t))}});U.addEventListener("change",async()=>{const e=U.checked;m=!0,d(k);try{await window.go.main.App.SetProxyEnabled(e),await x()}catch(t){r(String(t))}finally{m=!1,d(k)}});h.addEventListener("click",async()=>{if(k.running)await window.go.main.App.StopProfileVPN(o.id);else{if(o!=null&&o.tunMode)try{if(await ve(!0))return}catch(n){r(String(n));return}d({running:!0,state:"connecting",stage:"\u51C6\u5907\u4E2D",message:"\u6B63\u5728\u51C6\u5907\u8FDE\u63A5\u2026",tunMode:!!(o!=null&&o.tunMode),disableProxy:!!(o!=null&&o.disableProxy)});const e=o==null?void 0:o.id,t=await window.go.main.App.StartProfileVPN(e);t!=="OK"&&e===(o==null?void 0:o.id)&&(r("[\u8FDE\u63A5\u5931\u8D25] "+t),d({running:!1,state:"error",lastError:t}))}});function R(e){v=e||[];const t=Y.value.trim().toLowerCase(),n=t?v.filter(l=>he(l,t)):v;if(!v.length){G.innerHTML=`
      <div class="route-empty">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <circle cx="12" cy="12" r="10"/>
          <line x1="2" y1="12" x2="22" y2="12"/>
          <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>
        </svg>
        <span>\u5EFA\u7ACB\u96A7\u9053\u540E\uFF0C\u5728\u6B64\u5C55\u793A\u670D\u52A1\u7AEF\u6388\u6743\u8BBF\u95EE\u7684\u5185\u7F51\u7F51\u6BB5\u4E0E\u57DF\u540D\u6620\u5C04</span>
      </div>`,z.innerText="";return}const i=l=>String(l.kind||l.Kind||"").toLowerCase(),s=n.length;if(z.innerText=t?`${s}/${v.length}`:`\u5171 ${v.length} \u6761`,!s){G.innerHTML=`
      <div class="route-empty">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <circle cx="11" cy="11" r="8"/>
          <line x1="21" y1="21" x2="16.65" y2="16.65"/>
        </svg>
        <span>\u672A\u641C\u7D22\u5230\u5339\u914D\u300C${t}\u300D\u7684\u8D44\u6E90</span>
      </div>`;return}const a=n.filter(l=>i(l)==="ip"),c=n.filter(l=>i(l)==="domain"),C=(l,E)=>`
    <div class="route-row">
      <div class="route-target-wrap">
        <svg class="route-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          ${E?'<rect x="2" y="2" width="20" height="8" rx="2" ry="2"/><rect x="2" y="14" width="20" height="8" rx="2" ry="2"/>':'<circle cx="12" cy="12" r="10"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>'}
        </svg>
        <span class="route-target" title="${l.target||l.Target||""}">${l.target||l.Target||""}</span>
      </div>
      <span class="route-badge">${l.ports||l.Ports||""} \xB7 ${l.protocol||l.Protocol||""}</span>
    </div>`,N=(l,E,fe)=>E.length?`<div class="route-group"><span>${l}</span><span class="route-group-badge">${E.length}</span></div>`+E.map(ye=>C(ye,fe)).join(""):"";G.innerHTML=N("\u5185\u7F51 IP \u6BB5",a,!0)+N("\u57DF\u540D\u6620\u5C04\u89E3\u6790",c,!1)}async function S(){const e=document.getElementById("btn-refresh-resources");e&&e.classList.add("spinning");try{window.go&&window.go.main&&window.go.main.App&&(R(await window.go.main.App.GetVPNResources()),r("\u8D44\u6E90\u5217\u8868\u5DF2\u5237\u65B0"))}catch(t){console.error("GetVPNResources error:",t)}finally{setTimeout(()=>{e&&e.classList.remove("spinning")},500)}}Y.addEventListener("input",()=>R(v));document.getElementById("btn-refresh-resources").addEventListener("click",()=>{S()});function Te(){const e=[{id:"p1",name:"IDC \u751F\u4EA7\u8282\u70B9 (\u5E262FA)",server:"111.205.145.240",port:58443,enable2FA:!0},{id:"p2",name:"UniVPN \u5F00\u53D1\u96C6\u7FA4",protocol:"univpn",server:"10.0.0.8",port:443,enable2FA:!1,socksPort:1082,httpPort:1083}];p=e,o=e[0],b=e.map(n=>({profileId:n.id,activeProfile:n.name,protocol:n.protocol||"easyconnect",running:!0,state:"connected"})),A(),w.innerHTML=e.map(n=>`<option value="${n.id}">${n.name}${n.enable2FA?" [2FA]":""}</option>`).join(""),d({running:!0,state:"connected",clientIp:"192.168.0.2",dns:"172.31.112.1",socksPort:"1080",httpPort:"1081"}),H.innerText="08:20:31";const t=[];for(let n=0;n<14;n++)t.push({kind:"ip",target:`172.31.10${n}.0 ~ 172.31.10${n}.255`,ports:"\u5168\u90E8\u7AEF\u53E3",protocol:"\u5168\u90E8\u534F\u8BAE"});t.push({kind:"domain",target:"ops.internal.example.com \u2192 172.31.128.12",ports:"\u56FA\u5B9A\u89E3\u6790",protocol:"-"}),t.push({kind:"domain",target:"git.internal.example.com",ports:"443",protocol:"TCP"}),R(t)}async function q(){for(let e=0;e<40&&!(window.go&&window.go.main&&window.go.main.App&&window.runtime);e++)await new Promise(t=>setTimeout(t,50));window.runtime&&(window.runtime.EventsOn("vpn-status",e=>{d(e),!e.running&&e.profileId===(o==null?void 0:o.id)&&S()}),window.runtime.EventsOn("vpn-resources",e=>{e.profileId===(o==null?void 0:o.id)&&R(e.entries)}),window.runtime.EventsOn("status-tick",e=>{e.profileId===(o==null?void 0:o.id)&&(H.innerText=J(e.duration),pe(e))}),window.runtime.EventsOn("vpn-connections",e=>{b=e||[],A()}),window.runtime.EventsOn("profile-switched",async()=>{await x()}));try{window.go&&window.go.main&&window.go.main.App?(de=await window.go.main.App.GetInterfaces(),await x(),d(await window.go.main.App.GetStatus()),S()):we&&Te()}catch(e){console.error("Init error:",e),r("[\u521D\u59CB\u5316\u5F02\u5E38] "+e)}}document.readyState==="loading"?document.addEventListener("DOMContentLoaded",q):q();function ge(){const e=g.value==="univpn";document.getElementById("form-domain-container").hidden=!e,document.getElementById("form-cert-container").hidden=!e;for(const t of[f,u,P])t.closest(".toggle-row").style.display=e?"none":"";W.style.display=!e&&f.checked?"block":"none",F.style.display=!e&&u.checked?"block":"none"}g.addEventListener("change",ge);async function Be(){b=await window.go.main.App.GetConnections(),A()}function A(){const e=document.getElementById("connection-list");e.replaceChildren();for(const n of b){const i=document.createElement("button");i.className="connection-row",i.classList.toggle("selected",n.profileId===(o==null?void 0:o.id));const s=document.createElement("span");s.textContent=`${n.activeProfile} \xB7 ${n.protocol==="univpn"?"UniVPN":"\u6DF1\u4FE1\u670D"}`;const a=document.createElement("span");a.className=`connection-state ${n.state}`,a.textContent=ce[n.state]||n.state,i.append(s,a),i.addEventListener("click",async()=>{await window.go.main.App.SetActiveProfile(n.profileId),w.value=n.profileId,me(p.find(c=>c.id===n.profileId)),d(await window.go.main.App.GetStatus()),S(),A()}),e.appendChild(i)}const t=b.filter(n=>n.running).length;if(t){const n=document.createElement("button");n.className="btn-chip danger",n.textContent=`\u65AD\u5F00\u5168\u90E8\uFF08${t}\uFF09`,n.addEventListener("click",()=>window.go.main.App.StopAllVPN()),e.appendChild(n)}}

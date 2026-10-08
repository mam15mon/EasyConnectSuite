(function(){const t=document.createElement("link").relList;if(t&&t.supports&&t.supports("modulepreload"))return;for(const i of document.querySelectorAll('link[rel="modulepreload"]'))l(i);new MutationObserver(i=>{for(const s of i)if(s.type==="childList")for(const u of s.addedNodes)u.tagName==="LINK"&&u.rel==="modulepreload"&&l(u)}).observe(document,{childList:!0,subtree:!0});function n(i){const s={};return i.integrity&&(s.integrity=i.integrity),i.referrerpolicy&&(s.referrerPolicy=i.referrerpolicy),i.crossorigin==="use-credentials"?s.credentials="include":i.crossorigin==="anonymous"?s.credentials="omit":s.credentials="same-origin",s}function l(i){if(i.ep)return;i.ep=!0;const s=n(i);fetch(i.href,s)}})();const te=new URLSearchParams(location.search).has("preview"),ne=document.querySelector("#app");ne.innerHTML=`
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
`;function r(e){const t=document.getElementById("toast");t.innerText=e,t.classList.add("show"),setTimeout(()=>t.classList.remove("show"),2200)}function b(e,t){!e||e==="\u2014"||navigator.clipboard.writeText(e).then(()=>{r(`\u5DF2\u590D\u5236 ${t}: ${e}`)}).catch(()=>{r(`\u590D\u5236\u5931\u8D25: ${e}`)})}function oe(e){const t=Math.floor(e/3600).toString().padStart(2,"0"),n=Math.floor(e%3600/60).toString().padStart(2,"0"),l=Math.floor(e%60).toString().padStart(2,"0");return`${t}:${n}:${l}`}const E=document.getElementById("profile-select"),f=document.getElementById("btn-toggle"),k=document.getElementById("btn-label"),C=document.getElementById("uptime-display"),ie=document.getElementById("state-word"),x=document.getElementById("state-note"),se=document.getElementById("top-state-word"),y=document.getElementById("client-ip-val"),h=document.getElementById("dns-val"),O=document.getElementById("socks-port-val"),H=document.getElementById("http-port-val"),P=document.getElementById("route-table"),V=document.getElementById("route-count"),D=document.getElementById("routes-filter");document.getElementById("item-client-ip").addEventListener("click",()=>{b(y.innerText,"\u865A\u62DF IP")});document.getElementById("item-dns").addEventListener("click",()=>{b(h.innerText,"\u5185\u7F51 DNS")});document.getElementById("item-socks").addEventListener("click",()=>{b(O.innerText,"SOCKS5 \u4EE3\u7406")});document.getElementById("item-http").addEventListener("click",()=>{b(H.innerText,"HTTP \u4EE3\u7406")});document.getElementById("log-path-hint").addEventListener("click",()=>{b("%APPDATA%\\EasyConnectVPN\\logs","\u65E5\u5FD7\u76EE\u5F55")});const F=document.getElementById("modal-overlay"),ae=document.getElementById("modal-title"),K=document.getElementById("form-id"),R=document.getElementById("form-name"),U=document.getElementById("form-server"),W=document.getElementById("form-port"),G=document.getElementById("form-username"),j=document.getElementById("form-password"),w=document.getElementById("form-enable-2fa"),z=document.getElementById("form-totp-container"),q=document.getElementById("form-totp"),L=document.getElementById("form-nic"),m=document.getElementById("form-enable-keepalive"),S=document.getElementById("form-keepalive-container"),J=document.getElementById("form-keepalive-url"),I=document.getElementById("form-disable-keepalive");let c=[],a=null,v={running:!1,state:"disconnected"},X=["\u4EE5\u592A\u7F51","WLAN"],d=[];w.addEventListener("change",()=>{z.style.display=w.checked?"block":"none"});m.addEventListener("change",()=>{S.style.display=m.checked?"block":"none"});I.addEventListener("change",()=>{I.checked&&(m.checked=!1,S.style.display="none")});const re={connected:"\u5DF2\u8FDE\u63A5",connecting:"\u63E1\u624B\u4E2D",disconnected:"\u672A\u8FDE\u63A5",error:"\u8FDE\u63A5\u51FA\u9519"};function Y(e){!e||(e.socksPort&&(O.innerText="127.0.0.1:"+e.socksPort),e.httpPort&&(H.innerText="127.0.0.1:"+e.httpPort),!e.running||e.state==="disconnected"?(y.innerText="\u2014",y.classList.add("pending"),h.innerText="\u2014",h.classList.add("pending")):(e.clientIp&&(y.innerText=e.clientIp,y.classList.remove("pending")),e.dns&&(h.innerText=e.dns,h.classList.remove("pending"))))}function p(e){v=e,document.body.dataset.state=e.state;const t=re[e.state]||"\u672A\u8FDE\u63A5";ie.innerText=t,se.innerText=t,e.state==="connected"?(k.innerText="\u65AD\u5F00\u96A7\u9053",f.disabled=!1,x.innerText="VPN \u901A\u9053\u5DE5\u4F5C\u6B63\u5E38\uFF0C\u6D41\u91CF\u7ECF\u672C\u5730\u4EE3\u7406\u7AEF\u53E3\u76F4\u63A5\u8F6C\u53D1"):e.state==="connecting"?(k.innerText="\u6B63\u5728\u63E1\u624B\u2026",f.disabled=!0,x.innerText=e.lastError||"\u6B63\u5728\u53D1\u8D77\u8BA4\u8BC1\u5E76\u534F\u5546\u52A0\u5BC6\u96A7\u9053\uFF0C\u8BF7\u7A0D\u5019\u2026"):e.state==="error"?(k.innerText="\u91CD\u65B0\u5EFA\u7ACB",f.disabled=!1,x.innerText=e.lastError||"\u8FDE\u63A5\u5EFA\u7ACB\u5931\u8D25\uFF0C\u53EF\u68C0\u67E5\u914D\u7F6E\u540E\u91CD\u8BD5"):(k.innerText="\u5EFA\u7ACB\u96A7\u9053",f.disabled=!1,x.innerText=a?`\u51C6\u5907\u8FDE\u63A5\u8282\u70B9\u300C${a.name}\u300D\uFF08${a.server}:${a.port}\uFF09`:"\u901A\u9053\u672A\u5EFA\u7ACB",C.innerText="00:00:00"),Y(e)}async function B(){const e=await window.go.main.App.GetAppConfig();c=e.profiles||[],E.innerHTML="",c.forEach(t=>{const n=document.createElement("option");n.value=t.id,n.innerText=t.name+(t.enable2FA?" [2FA]":""),t.id===e.activeProfileId&&(n.selected=!0),E.appendChild(n)}),a=c.find(t=>t.id===e.activeProfileId)||c[0],v.running||p(v)}function le(e){!e||(a=e,v.running||p(v))}E.addEventListener("change",async e=>{const t=e.target.value;await window.go.main.App.SetActiveProfile(t),le(c.find(n=>n.id===t))});function Z(e){ae.innerText=e?"\u7F16\u8F91\u670D\u52A1\u5668\u914D\u7F6E":"\u65B0\u5EFA\u670D\u52A1\u5668\u914D\u7F6E",K.value=e?e.id:"",R.value=e?e.name:"\u65B0\u914D\u7F6E",U.value=e?e.server:"",W.value=e?e.port:443,G.value=e?e.username:"",j.value=e?e.password:"",w.checked=e?!!e.enable2FA:!1,q.value=e&&e.totpSecret?e.totpSecret:"",z.style.display=w.checked?"block":"none",m.checked=!!(e&&e.keepAliveUrl),J.value=e&&e.keepAliveUrl?e.keepAliveUrl:"",S.style.display=m.checked?"block":"none",I.checked=e?!!e.disableKeepAlive:!1,L.innerHTML="",X.forEach(t=>{const n=document.createElement("option");n.value=t,n.innerText=t,e&&e.bindInterface===t&&(n.selected=!0),L.appendChild(n)}),F.classList.add("show")}function M(){F.classList.remove("show")}document.getElementById("btn-edit-profile").addEventListener("click",()=>{a&&Z(a)});document.getElementById("btn-new-profile").addEventListener("click",()=>{Z(null)});document.getElementById("btn-del-profile").addEventListener("click",async()=>{if(!!a){if(c.length<=1){r("\u81F3\u5C11\u4FDD\u7559\u4E00\u4E2A\u914D\u7F6E\uFF0C\u65E0\u6CD5\u5220\u9664");return}confirm(`\u786E\u5B9A\u8981\u5220\u9664\u914D\u7F6E\u300C${a.name}\u300D\u5417\uFF1F`)&&(await window.go.main.App.DeleteProfile(a.id),r("\u914D\u7F6E\u5DF2\u5220\u9664"),await B())}});document.getElementById("modal-close-btn").addEventListener("click",M);document.getElementById("btn-modal-cancel").addEventListener("click",M);document.getElementById("btn-modal-save").addEventListener("click",async()=>{const e={id:K.value.trim(),name:R.value.trim()||"\u672A\u547D\u540D\u914D\u7F6E",server:U.value.trim(),port:parseInt(W.value)||443,username:G.value.trim(),password:j.value.trim(),enable2FA:w.checked,totpSecret:q.value.trim(),bindInterface:L.value,disableZjuConfig:!0,disableKeepAlive:I.checked,keepAliveUrl:m.checked?J.value.trim():""};if(!e.server||!e.username){r("\u670D\u52A1\u5668\u5730\u5740\u548C\u8D26\u53F7\u4E0D\u80FD\u4E3A\u7A7A");return}await window.go.main.App.SaveProfile(e),M(),r("\u914D\u7F6E\u5DF2\u4FDD\u5B58"),await B()});f.addEventListener("click",async()=>{if(v.running)await window.go.main.App.StopVPN();else{p({running:!0,state:"connecting"});const e=await window.go.main.App.StartVPN();e!=="OK"&&(r("[\u8FDE\u63A5\u5931\u8D25] "+e),p({running:!1,state:"error",lastError:e}))}});function T(e){d=e||[];const t=D.value.trim().toLowerCase(),n=t?d.filter(o=>String(o.target||o.Target||"").toLowerCase().includes(t)):d;if(!d.length){P.innerHTML=`
      <div class="route-empty">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <circle cx="12" cy="12" r="10"/>
          <line x1="2" y1="12" x2="22" y2="12"/>
          <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>
        </svg>
        <span>\u5EFA\u7ACB\u96A7\u9053\u540E\uFF0C\u5728\u6B64\u5C55\u793A\u670D\u52A1\u7AEF\u6388\u6743\u8BBF\u95EE\u7684\u5185\u7F51\u7F51\u6BB5\u4E0E\u57DF\u540D\u6620\u5C04</span>
      </div>`,V.innerText="";return}const l=o=>String(o.kind||o.Kind||"").toLowerCase(),i=n.length;if(V.innerText=t?`${i}/${d.length}`:`\u5171 ${d.length} \u6761`,!i){P.innerHTML=`
      <div class="route-empty">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <circle cx="11" cy="11" r="8"/>
          <line x1="21" y1="21" x2="16.65" y2="16.65"/>
        </svg>
        <span>\u672A\u641C\u7D22\u5230\u5339\u914D\u300C${t}\u300D\u7684\u8D44\u6E90</span>
      </div>`;return}const s=n.filter(o=>l(o)==="ip"),u=n.filter(o=>l(o)==="domain"),_=(o,g)=>`
    <div class="route-row">
      <div class="route-target-wrap">
        <svg class="route-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          ${g?'<rect x="2" y="2" width="20" height="8" rx="2" ry="2"/><rect x="2" y="14" width="20" height="8" rx="2" ry="2"/>':'<circle cx="12" cy="12" r="10"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>'}
        </svg>
        <span class="route-target" title="${o.target||o.Target||""}">${o.target||o.Target||""}</span>
      </div>
      <span class="route-badge">${o.ports||o.Ports||""} \xB7 ${o.protocol||o.Protocol||""}</span>
    </div>`,$=(o,g,Q)=>g.length?`<div class="route-group"><span>${o}</span><span class="route-group-badge">${g.length}</span></div>`+g.map(ee=>_(ee,Q)).join(""):"";P.innerHTML=$("\u5185\u7F51 IP \u6BB5",s,!0)+$("\u57DF\u540D\u6620\u5C04\u89E3\u6790",u,!1)}async function A(){const e=document.getElementById("btn-refresh-resources");e&&e.classList.add("spinning");try{window.go&&window.go.main&&window.go.main.App&&(T(await window.go.main.App.GetVPNResources()),r("\u8D44\u6E90\u5217\u8868\u5DF2\u5237\u65B0"))}catch(t){console.error("GetVPNResources error:",t)}finally{setTimeout(()=>{e&&e.classList.remove("spinning")},500)}}D.addEventListener("input",()=>T(d));document.getElementById("btn-refresh-resources").addEventListener("click",()=>{A()});function de(){const e=[{id:"p1",name:"IDC \u751F\u4EA7\u8282\u70B9 (\u5E262FA)",server:"111.205.145.240",port:58443,enable2FA:!0},{id:"p2",name:"\u6D4B\u8BD5\u5F00\u53D1\u96C6\u7FA4",server:"10.0.0.8",port:443,enable2FA:!1}];c=e,a=e[0],E.innerHTML=e.map(n=>`<option value="${n.id}">${n.name}${n.enable2FA?" [2FA]":""}</option>`).join(""),p({running:!0,state:"connected",clientIp:"192.168.0.2",dns:"172.31.112.1",socksPort:"1080",httpPort:"1081"}),C.innerText="08:20:31";const t=[];for(let n=0;n<14;n++)t.push({kind:"ip",target:`172.31.10${n}.0 ~ 172.31.10${n}.255`,ports:"\u5168\u90E8\u7AEF\u53E3",protocol:"\u5168\u90E8\u534F\u8BAE"});t.push({kind:"domain",target:"ops.internal.example.com \u2192 172.31.128.12",ports:"\u56FA\u5B9A\u89E3\u6790",protocol:"-"}),t.push({kind:"domain",target:"git.internal.example.com",ports:"443",protocol:"TCP"}),T(t)}async function N(){for(let e=0;e<40&&!(window.go&&window.go.main&&window.go.main.App&&window.runtime);e++)await new Promise(t=>setTimeout(t,50));window.runtime&&(window.runtime.EventsOn("vpn-status",e=>{p(e),e.running||A()}),window.runtime.EventsOn("vpn-resources",e=>{T(e)}),window.runtime.EventsOn("status-tick",e=>{C.innerText=oe(e.duration),Y(e)}),window.runtime.EventsOn("profile-switched",async()=>{await B()}));try{window.go&&window.go.main&&window.go.main.App?(X=await window.go.main.App.GetInterfaces(),await B(),p(await window.go.main.App.GetStatus()),A()):te&&de()}catch(e){console.error("Init error:",e),r("[\u521D\u59CB\u5316\u5F02\u5E38] "+e)}}document.readyState==="loading"?document.addEventListener("DOMContentLoaded",N):N();

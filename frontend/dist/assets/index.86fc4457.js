(function(){const t=document.createElement("link").relList;if(t&&t.supports&&t.supports("modulepreload"))return;for(const o of document.querySelectorAll('link[rel="modulepreload"]'))l(o);new MutationObserver(o=>{for(const s of o)if(s.type==="childList")for(const n of s.addedNodes)n.tagName==="LINK"&&n.rel==="modulepreload"&&l(n)}).observe(document,{childList:!0,subtree:!0});function i(o){const s={};return o.integrity&&(s.integrity=o.integrity),o.referrerpolicy&&(s.referrerPolicy=o.referrerpolicy),o.crossorigin==="use-credentials"?s.credentials="include":o.crossorigin==="anonymous"?s.credentials="omit":s.credentials="same-origin",s}function l(o){if(o.ep)return;o.ep=!0;const s=i(o);fetch(o.href,s)}})();const j=document.querySelector("#app");j.innerHTML=`
  <header class="header">
    <div class="brand">
      <div class="brand-icon">
        <svg viewBox="0 0 24 24"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"/></svg>
      </div>
      <div>
        <div class="brand-title">EasyConnect VPN</div>
        <div class="brand-subtitle">\u591A\u8282\u70B9\u7BA1\u7406 \xB7 \u7528\u6237\u6001\u9694\u79BB\u67B6\u6784</div>
      </div>
    </div>
    <div class="header-status">
      <div class="uptime-badge" id="uptime-display">00:00:00</div>
      <div class="status-pill" id="status-pill">
        <span class="status-dot"></span>
        <span id="status-text">\u672A\u8FDE\u63A5</span>
      </div>
    </div>
  </header>

  <div class="main-grid">
    <div class="left-panel">
      <!-- Profile Switcher Card -->
      <div class="card profile-card">
        <div class="card-header-bar">
          <span class="card-title">\u670D\u52A1\u5668\u914D\u7F6E (Profile)</span>
          <div class="card-header-actions">
            <button class="btn-sm" id="btn-new-profile" title="\u65B0\u5EFA\u670D\u52A1\u5668\u914D\u7F6E">\u2795 \u65B0\u5EFA</button>
            <button class="btn-sm" id="btn-edit-profile" title="\u4FEE\u6539\u5F53\u524D\u914D\u7F6E">\u270F\uFE0F \u7F16\u8F91</button>
            <button class="btn-sm danger" id="btn-del-profile" title="\u5220\u9664\u5F53\u524D\u914D\u7F6E">\u{1F5D1}\uFE0F</button>
          </div>
        </div>
        <div class="profile-select-wrap">
          <select class="profile-select" id="profile-select"></select>
        </div>
      </div>

      <!-- Action Card -->
      <div class="card action-card">
        <button class="btn-toggle" id="btn-toggle">
          <span id="btn-icon">\u26A1</span>
          <span id="btn-label">\u7ACB\u5373\u8FDE\u63A5 VPN</span>
        </button>

        <div class="proxy-info">
          <div class="proxy-pill">
            <div class="proxy-label">SOCKS5 \u4EE3\u7406</div>
            <div class="proxy-val" id="socks-port-val">127.0.0.1:1080</div>
          </div>
          <div class="proxy-pill">
            <div class="proxy-label">HTTP \u4EE3\u7406</div>
            <div class="proxy-val" id="http-port-val">127.0.0.1:1081</div>
          </div>
        </div>

        <div class="proxy-info">
          <div class="proxy-pill">
            <div class="proxy-label">\u865A\u62DF\u5185\u7F51 IP</div>
            <div class="proxy-val" id="client-ip-val">\u7B49\u5F85\u63E1\u624B...</div>
          </div>
          <div class="proxy-pill">
            <div class="proxy-label">\u5185\u7F51 DNS</div>
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
          <span>\u53EF\u7528\u5185\u7F51\u8D44\u6E90 (\u670D\u52A1\u5668\u6388\u6743)</span>
        </div>
        <div class="terminal-actions">
          <button class="btn-terminal" id="btn-refresh-resources" title="\u5237\u65B0\u8D44\u6E90\u5217\u8868">\u{1F504} \u5237\u65B0</button>
        </div>
      </div>
      <div class="terminal-body" id="resource-body">
        <div class="log-line log-system">\u8FDE\u63A5 VPN \u540E\u5C06\u5728\u6B64\u663E\u793A\u670D\u52A1\u5668\u6388\u6743\u8BBF\u95EE\u7684\u5185\u7F51\u8D44\u6E90\u3002</div>
      </div>
    </div>
  </div>

  <!-- Profile Edit / Create Modal -->
  <div class="modal-overlay" id="modal-overlay">
    <div class="modal-box">
      <div class="modal-header">
        <div class="modal-title" id="modal-title">\u7F16\u8F91\u670D\u52A1\u5668\u914D\u7F6E</div>
        <button class="modal-close" id="modal-close-btn">&times;</button>
      </div>
      <div class="modal-body">
        <input type="hidden" id="form-id" />
        <div class="form-group">
          <label class="form-label">\u914D\u7F6E\u540D\u79F0 (\u5907\u6CE8\u6807\u7B7E)</label>
          <input type="text" class="form-input" id="form-name" placeholder="\u4F8B\u5982\uFF1A\u516C\u53F8\u751F\u4EA7\u73AF\u5883 / \u6D4B\u8BD5\u73AF\u5883" />
        </div>
        <div class="form-row">
          <div class="form-group">
            <label class="form-label">VPN \u670D\u52A1\u5668\u5730\u5740 (Host)</label>
            <input type="text" class="form-input" id="form-server" placeholder="\u4F8B\u5982\uFF1Avpn.example.com \u6216 203.0.113.10" />
          </div>
          <div class="form-group">
            <label class="form-label">\u7AEF\u53E3 (Port)</label>
            <input type="number" class="form-input" id="form-port" value="443" />
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label class="form-label">\u8D26\u53F7 (Username)</label>
            <input type="text" class="form-input" id="form-username" />
          </div>
          <div class="form-group">
            <label class="form-label">\u5BC6\u7801 (Password)</label>
            <input type="password" class="form-input" id="form-password" />
          </div>
        </div>

        <!-- 2FA Checkbox Toggle -->
        <label class="toggle-row">
          <input type="checkbox" id="form-enable-2fa" />
          <span style="font-size:12px;font-weight:500">\u542F\u7528\u4E8C\u6B21\u8BA4\u8BC1 (\u8FDE\u63A5\u65F6\u81EA\u52A8\u63D0\u4EA4 TOTP)</span>
        </label>

        <div class="form-group" id="form-totp-container" style="display:none">
          <label class="form-label">TOTP Secret \u5BC6\u94A5 (Base32\uFF0C\u8FDE\u63A5\u65F6\u81EA\u52A8\u63D0\u4EA4)</label>
          <input type="text" class="form-input" id="form-totp" placeholder="\u4F8B\u5982\uFF1AJBSWY3DPEHPK3PXP" />
        </div>

        <!-- KeepAlive Controls -->
        <label class="toggle-row">
          <input type="checkbox" id="form-enable-keepalive" />
          <span style="font-size:12px;font-weight:500">\u81EA\u5B9A\u4E49\u4F1A\u8BDD\u4FDD\u6D3B\u5730\u5740 (\u9ED8\u8BA4\u7528\u5F15\u64CE\u5185\u7F6E\u63A2\u6D4B\uFF0C\u4E00\u822C\u65E0\u9700\u4FEE\u6539)</span>
        </label>

        <div class="form-group" id="form-keepalive-container" style="display:none">
          <label class="form-label">\u4FDD\u6D3B URL (\u8FDE\u63A5\u671F\u95F4\u5B9A\u671F\u901A\u8FC7\u96A7\u9053\u8BBF\u95EE\u8BE5\u5730\u5740)</label>
          <input type="text" class="form-input" id="form-keepalive-url" placeholder="\u4F8B\u5982\uFF1Ahttp://172.31.112.1" />
        </div>

        <label class="toggle-row">
          <input type="checkbox" id="form-disable-keepalive" />
          <span style="font-size:12px;font-weight:500">\u5B8C\u5168\u7981\u7528\u4F1A\u8BDD\u4FDD\u6D3B (\u4E0D\u63A8\u8350\uFF0C\u957F\u65F6\u95F4\u7A7A\u95F2\u53EF\u80FD\u88AB\u670D\u52A1\u7AEF\u65AD\u5F00)</span>
        </label>

        <div class="form-group">
          <label class="form-label">\u7ED1\u5B9A\u7269\u7406\u7F51\u5361 (\u63A8\u8350\u4EE5\u592A\u7F51\uFF0C\u9632 TUN \u51B2\u7A81)</label>
          <select class="form-select" id="form-nic">
            <option value="\u4EE5\u592A\u7F51">\u4EE5\u592A\u7F51</option>
            <option value="WLAN">WLAN</option>
          </select>
        </div>
      </div>
      <div class="modal-actions">
        <button class="btn-secondary" id="btn-modal-cancel">\u53D6\u6D88</button>
        <button class="btn-primary" id="btn-modal-save">\u4FDD\u5B58\u6B64\u914D\u7F6E</button>
      </div>
    </div>
  </div>

  <div class="toast" id="toast">\u5DF2\u590D\u5236\u5230\u526A\u8D34\u677F</div>
`;function r(e){const t=document.getElementById("toast");t.innerText=e,t.classList.add("show"),setTimeout(()=>t.classList.remove("show"),1800)}function q(e){const t=Math.floor(e/3600).toString().padStart(2,"0"),i=Math.floor(e%3600/60).toString().padStart(2,"0"),l=(e%60).toString().padStart(2,"0");return`${t}:${i}:${l}`}const T=document.getElementById("resource-body");function S(e){if(e=e||[],!e.length){T.innerHTML='<div class="log-line log-system">\u8FDE\u63A5 VPN \u540E\u5C06\u5728\u6B64\u663E\u793A\u670D\u52A1\u5668\u6388\u6743\u8BBF\u95EE\u7684\u5185\u7F51\u8D44\u6E90\u3002</div>';return}const t=n=>String(n.kind||n.Kind||"").toLowerCase(),i=e.filter(n=>t(n)==="ip"),l=e.filter(n=>t(n)==="domain"),o=n=>`
    <div class="resource-row">
      <span class="resource-icon">${t(n)==="ip"?"\u{1F522}":"\u{1F310}"}</span>
      <span class="resource-target">${n.target||n.Target||""}</span>
      <span class="resource-meta">${(n.ports||n.Ports||"")+" \xB7 "+(n.protocol||n.Protocol||"")}</span>
    </div>`,s=(n,y)=>y.length?`<div class="resource-group">${n} (${y.length})</div>`+y.map(o).join(""):"";T.innerHTML=s("IP \u8D44\u6E90",i)+s("\u57DF\u540D\u8D44\u6E90",l)}async function v(){if(!(!window.go||!window.go.main||!window.go.main.App))try{S(await window.go.main.App.GetVPNResources())}catch(e){console.error("GetVPNResources error:",e)}}document.getElementById("btn-refresh-resources").addEventListener("click",()=>{v(),r("\u8D44\u6E90\u5217\u8868\u5DF2\u5237\u65B0")});const E=document.getElementById("profile-select"),J=document.getElementById("status-pill"),w=document.getElementById("status-text"),d=document.getElementById("btn-toggle"),u=document.getElementById("btn-label"),h=document.getElementById("btn-icon"),N=document.getElementById("uptime-display"),B=document.getElementById("client-ip-val"),A=document.getElementById("dns-val"),X=document.getElementById("socks-port-val"),Y=document.getElementById("http-port-val"),C=document.getElementById("modal-overlay"),Z=document.getElementById("modal-title"),V=document.getElementById("form-id"),O=document.getElementById("form-name"),M=document.getElementById("form-server"),$=document.getElementById("form-port"),K=document.getElementById("form-username"),R=document.getElementById("form-password"),p=document.getElementById("form-enable-2fa"),U=document.getElementById("form-totp-container"),H=document.getElementById("form-totp"),x=document.getElementById("form-nic"),m=document.getElementById("form-enable-keepalive"),I=document.getElementById("form-keepalive-container"),F=document.getElementById("form-keepalive-url"),f=document.getElementById("form-disable-keepalive");let c=[],a=null,k={running:!1,state:"disconnected"},z=["\u4EE5\u592A\u7F51","WLAN"];p.addEventListener("change",()=>{U.style.display=p.checked?"block":"none"});m.addEventListener("change",()=>{I.style.display=m.checked?"block":"none"});f.addEventListener("change",()=>{f.checked&&(m.checked=!1,I.style.display="none")});function D(e){!e||(e.socksPort&&(X.innerText="127.0.0.1:"+e.socksPort),e.httpPort&&(Y.innerText="127.0.0.1:"+e.httpPort),!e.running||e.state==="disconnected"?(B.innerText="\u7B49\u5F85\u63E1\u624B...",A.innerText="--"):(e.clientIp&&(B.innerText=e.clientIp),e.dns&&(A.innerText=e.dns)))}function g(e){k=e,J.className="status-pill "+e.state,e.state==="connected"?(w.innerText="\u5DF2\u8FDE\u63A5",d.className="btn-toggle connected",u.innerText="\u65AD\u5F00\u8FDE\u63A5",h.innerText="\u23F9",d.disabled=!1):e.state==="connecting"?(w.innerText="\u63E1\u624B\u4E2D...",d.className="btn-toggle",u.innerText="\u6B63\u5728\u63E1\u624B\u534F\u5546...",h.innerText="\u23F3",d.disabled=!0):(w.innerText="\u672A\u8FDE\u63A5",d.className="btn-toggle",u.innerText=a?`\u8FDE\u63A5 ${a.name}`:"\u7ACB\u5373\u8FDE\u63A5 VPN",h.innerText="\u26A1",d.disabled=!1,N.innerText="00:00:00"),D(e)}async function b(){const e=await window.go.main.App.GetAppConfig();c=e.profiles||[],E.innerHTML="",c.forEach(t=>{const i=document.createElement("option");i.value=t.id,i.innerText=t.enable2FA?`${t.name} [2FA]`:`${t.name} [\u76F4\u8FDE]`,t.id===e.activeProfileId&&(i.selected=!0),E.appendChild(i)}),a=c.find(t=>t.id===e.activeProfileId)||c[0],G(a)}function G(e){!e||(a=e,k.running||(u.innerText=`\u8FDE\u63A5 ${e.name}`))}E.addEventListener("change",async e=>{const t=e.target.value;await window.go.main.App.SetActiveProfile(t);const i=c.find(l=>l.id===t);G(i)});function W(e){Z.innerText=e?"\u7F16\u8F91\u670D\u52A1\u5668\u914D\u7F6E":"\u65B0\u5EFA\u670D\u52A1\u5668\u914D\u7F6E",V.value=e?e.id:"",O.value=e?e.name:"\u65B0\u914D\u7F6E",M.value=e?e.server:"",$.value=e?e.port:443,K.value=e?e.username:"",R.value=e?e.password:"",p.checked=e?!!e.enable2FA:!1,H.value=e&&e.totpSecret?e.totpSecret:"",U.style.display=p.checked?"block":"none",m.checked=!!(e&&e.keepAliveUrl),F.value=e&&e.keepAliveUrl?e.keepAliveUrl:"",I.style.display=m.checked?"block":"none",f.checked=e?!!e.disableKeepAlive:!1,x.innerHTML="",z.forEach(t=>{const i=document.createElement("option");i.value=t,i.innerText=t,e&&e.bindInterface===t&&(i.selected=!0),x.appendChild(i)}),C.classList.add("show")}function P(){C.classList.remove("show")}document.getElementById("btn-edit-profile").addEventListener("click",()=>{a&&W(a)});document.getElementById("btn-new-profile").addEventListener("click",()=>{W(null)});document.getElementById("btn-del-profile").addEventListener("click",async()=>{if(!!a){if(c.length<=1){r("\u81F3\u5C11\u4FDD\u7559\u4E00\u4E2A\u914D\u7F6E\uFF0C\u65E0\u6CD5\u5220\u9664");return}confirm(`\u786E\u5B9A\u8981\u5220\u9664\u914D\u7F6E [${a.name}] \u5417\uFF1F`)&&(await window.go.main.App.DeleteProfile(a.id),r("\u914D\u7F6E\u5DF2\u5220\u9664"),await b())}});document.getElementById("modal-close-btn").addEventListener("click",P);document.getElementById("btn-modal-cancel").addEventListener("click",P);document.getElementById("btn-modal-save").addEventListener("click",async()=>{const e={id:V.value.trim(),name:O.value.trim()||"\u672A\u547D\u540D\u914D\u7F6E",server:M.value.trim(),port:parseInt($.value)||443,username:K.value.trim(),password:R.value.trim(),enable2FA:p.checked,totpSecret:H.value.trim(),bindInterface:x.value,disableZjuConfig:!0,disableKeepAlive:f.checked,keepAliveUrl:m.checked?F.value.trim():""};if(!e.server||!e.username){r("\u670D\u52A1\u5668\u5730\u5740\u548C\u8D26\u53F7\u4E0D\u80FD\u4E3A\u7A7A");return}await window.go.main.App.SaveProfile(e),P(),r("\u914D\u7F6E\u5DF2\u4FDD\u5B58"),await b()});d.addEventListener("click",async()=>{if(k.running)await window.go.main.App.StopVPN();else{g({running:!0,state:"connecting"});const e=await window.go.main.App.StartVPN();e!=="OK"&&(r("[\u8FDE\u63A5\u5931\u8D25] "+e),g({running:!1,state:"disconnected"}))}});async function L(){for(let e=0;e<40&&!(window.go&&window.go.main&&window.go.main.App&&window.runtime);e++)await new Promise(t=>setTimeout(t,50));window.runtime&&(window.runtime.EventsOn("vpn-status",e=>{g(e),e.state==="connected"?v():e.running||v()}),window.runtime.EventsOn("vpn-resources",e=>{S(e)}),window.runtime.EventsOn("status-tick",e=>{N.innerText=q(e.duration),D(e)}),window.runtime.EventsOn("profile-switched",async()=>{await b()}));try{if(window.go&&window.go.main&&window.go.main.App){z=await window.go.main.App.GetInterfaces(),await b();const e=await window.go.main.App.GetStatus();g(e),v()}}catch(e){console.error("Init error:",e),r("[\u521D\u59CB\u5316\u5F02\u5E38] "+e)}}document.readyState==="loading"?document.addEventListener("DOMContentLoaded",L):L();

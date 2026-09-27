(function(){const t=document.createElement("link").relList;if(t&&t.supports&&t.supports("modulepreload"))return;for(const i of document.querySelectorAll('link[rel="modulepreload"]'))a(i);new MutationObserver(i=>{for(const n of i)if(n.type==="childList")for(const l of n.addedNodes)l.tagName==="LINK"&&l.rel==="modulepreload"&&a(l)}).observe(document,{childList:!0,subtree:!0});function o(i){const n={};return i.integrity&&(n.integrity=i.integrity),i.referrerpolicy&&(n.referrerPolicy=i.referrerpolicy),i.crossorigin==="use-credentials"?n.credentials="include":i.crossorigin==="anonymous"?n.credentials="omit":n.credentials="same-origin",n}function a(i){if(i.ep)return;i.ep=!0;const n=o(i);fetch(i.href,n)}})();const G=document.querySelector("#app");G.innerHTML=`
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
`;function r(e){const t=document.getElementById("toast");t.innerText=e,t.classList.add("show"),setTimeout(()=>t.classList.remove("show"),1800)}function W(e){const t=Math.floor(e/3600).toString().padStart(2,"0"),o=Math.floor(e%3600/60).toString().padStart(2,"0"),a=(e%60).toString().padStart(2,"0");return`${t}:${o}:${a}`}const P=document.getElementById("resource-body");async function v(){if(!window.go||!window.go.main||!window.go.main.App)return;let e=[];try{e=await window.go.main.App.GetVPNResources()||[]}catch(n){console.error("GetVPNResources error:",n)}if(!e.length){P.innerHTML='<div class="log-line log-system">\u8FDE\u63A5 VPN \u540E\u5C06\u5728\u6B64\u663E\u793A\u670D\u52A1\u5668\u6388\u6743\u8BBF\u95EE\u7684\u5185\u7F51\u8D44\u6E90\u3002</div>';return}const t=e.filter(n=>n.kind==="ip"),o=e.filter(n=>n.kind==="domain"),a=n=>`
    <div class="resource-row">
      <span class="resource-icon">${n.kind==="ip"?"\u{1F522}":"\u{1F310}"}</span>
      <span class="resource-target">${n.target}</span>
      <span class="resource-meta">${n.ports} \xB7 ${n.protocol}</span>
    </div>`,i=(n,l)=>l.length?`<div class="resource-group">${n} (${l.length})</div>`+l.map(a).join(""):"";P.innerHTML=i("IP \u8D44\u6E90",t)+i("\u57DF\u540D\u8D44\u6E90",o)}document.getElementById("btn-refresh-resources").addEventListener("click",()=>{v(),r("\u8D44\u6E90\u5217\u8868\u5DF2\u5237\u65B0")});const h=document.getElementById("profile-select"),j=document.getElementById("status-pill"),y=document.getElementById("status-text"),d=document.getElementById("btn-toggle"),u=document.getElementById("btn-label"),w=document.getElementById("btn-icon"),L=document.getElementById("uptime-display"),T=document.getElementById("client-ip-val"),B=document.getElementById("dns-val"),q=document.getElementById("socks-port-val"),J=document.getElementById("http-port-val"),S=document.getElementById("modal-overlay"),X=document.getElementById("modal-title"),N=document.getElementById("form-id"),C=document.getElementById("form-name"),V=document.getElementById("form-server"),$=document.getElementById("form-port"),M=document.getElementById("form-username"),O=document.getElementById("form-password"),p=document.getElementById("form-enable-2fa"),K=document.getElementById("form-totp-container"),U=document.getElementById("form-totp"),E=document.getElementById("form-nic"),m=document.getElementById("form-enable-keepalive"),x=document.getElementById("form-keepalive-container"),H=document.getElementById("form-keepalive-url"),f=document.getElementById("form-disable-keepalive");let c=[],s=null,I={running:!1,state:"disconnected"},R=["\u4EE5\u592A\u7F51","WLAN"];p.addEventListener("change",()=>{K.style.display=p.checked?"block":"none"});m.addEventListener("change",()=>{x.style.display=m.checked?"block":"none"});f.addEventListener("change",()=>{f.checked&&(m.checked=!1,x.style.display="none")});function F(e){!e||(e.socksPort&&(q.innerText="127.0.0.1:"+e.socksPort),e.httpPort&&(J.innerText="127.0.0.1:"+e.httpPort),!e.running||e.state==="disconnected"?(T.innerText="\u7B49\u5F85\u63E1\u624B...",B.innerText="--"):(e.clientIp&&(T.innerText=e.clientIp),e.dns&&(B.innerText=e.dns)))}function g(e){I=e,j.className="status-pill "+e.state,e.state==="connected"?(y.innerText="\u5DF2\u8FDE\u63A5",d.className="btn-toggle connected",u.innerText="\u65AD\u5F00\u8FDE\u63A5",w.innerText="\u23F9",d.disabled=!1):e.state==="connecting"?(y.innerText="\u63E1\u624B\u4E2D...",d.className="btn-toggle",u.innerText="\u6B63\u5728\u63E1\u624B\u534F\u5546...",w.innerText="\u23F3",d.disabled=!0):(y.innerText="\u672A\u8FDE\u63A5",d.className="btn-toggle",u.innerText=s?`\u8FDE\u63A5 ${s.name}`:"\u7ACB\u5373\u8FDE\u63A5 VPN",w.innerText="\u26A1",d.disabled=!1,L.innerText="00:00:00"),F(e)}async function b(){const e=await window.go.main.App.GetAppConfig();c=e.profiles||[],h.innerHTML="",c.forEach(t=>{const o=document.createElement("option");o.value=t.id,o.innerText=t.enable2FA?`${t.name} [2FA]`:`${t.name} [\u76F4\u8FDE]`,t.id===e.activeProfileId&&(o.selected=!0),h.appendChild(o)}),s=c.find(t=>t.id===e.activeProfileId)||c[0],z(s)}function z(e){!e||(s=e,I.running||(u.innerText=`\u8FDE\u63A5 ${e.name}`))}h.addEventListener("change",async e=>{const t=e.target.value;await window.go.main.App.SetActiveProfile(t);const o=c.find(a=>a.id===t);z(o)});function D(e){X.innerText=e?"\u7F16\u8F91\u670D\u52A1\u5668\u914D\u7F6E":"\u65B0\u5EFA\u670D\u52A1\u5668\u914D\u7F6E",N.value=e?e.id:"",C.value=e?e.name:"\u65B0\u914D\u7F6E",V.value=e?e.server:"",$.value=e?e.port:443,M.value=e?e.username:"",O.value=e?e.password:"",p.checked=e?!!e.enable2FA:!1,U.value=e&&e.totpSecret?e.totpSecret:"",K.style.display=p.checked?"block":"none",m.checked=!!(e&&e.keepAliveUrl),H.value=e&&e.keepAliveUrl?e.keepAliveUrl:"",x.style.display=m.checked?"block":"none",f.checked=e?!!e.disableKeepAlive:!1,E.innerHTML="",R.forEach(t=>{const o=document.createElement("option");o.value=t,o.innerText=t,e&&e.bindInterface===t&&(o.selected=!0),E.appendChild(o)}),S.classList.add("show")}function k(){S.classList.remove("show")}document.getElementById("btn-edit-profile").addEventListener("click",()=>{s&&D(s)});document.getElementById("btn-new-profile").addEventListener("click",()=>{D(null)});document.getElementById("btn-del-profile").addEventListener("click",async()=>{if(!!s){if(c.length<=1){r("\u81F3\u5C11\u4FDD\u7559\u4E00\u4E2A\u914D\u7F6E\uFF0C\u65E0\u6CD5\u5220\u9664");return}confirm(`\u786E\u5B9A\u8981\u5220\u9664\u914D\u7F6E [${s.name}] \u5417\uFF1F`)&&(await window.go.main.App.DeleteProfile(s.id),r("\u914D\u7F6E\u5DF2\u5220\u9664"),await b())}});document.getElementById("modal-close-btn").addEventListener("click",k);document.getElementById("btn-modal-cancel").addEventListener("click",k);document.getElementById("btn-modal-save").addEventListener("click",async()=>{const e={id:N.value.trim(),name:C.value.trim()||"\u672A\u547D\u540D\u914D\u7F6E",server:V.value.trim(),port:parseInt($.value)||443,username:M.value.trim(),password:O.value.trim(),enable2FA:p.checked,totpSecret:U.value.trim(),bindInterface:E.value,disableZjuConfig:!0,disableKeepAlive:f.checked,keepAliveUrl:m.checked?H.value.trim():""};if(!e.server||!e.username){r("\u670D\u52A1\u5668\u5730\u5740\u548C\u8D26\u53F7\u4E0D\u80FD\u4E3A\u7A7A");return}await window.go.main.App.SaveProfile(e),k(),r("\u914D\u7F6E\u5DF2\u4FDD\u5B58"),await b()});d.addEventListener("click",async()=>{if(I.running)await window.go.main.App.StopVPN();else{g({running:!0,state:"connecting"});const e=await window.go.main.App.StartVPN();e!=="OK"&&(r("[\u8FDE\u63A5\u5931\u8D25] "+e),g({running:!1,state:"disconnected"}))}});async function A(){for(let e=0;e<40&&!(window.go&&window.go.main&&window.go.main.App&&window.runtime);e++)await new Promise(t=>setTimeout(t,50));window.runtime&&(window.runtime.EventsOn("vpn-status",e=>{g(e),e.state==="connected"?v():e.running||v()}),window.runtime.EventsOn("status-tick",e=>{L.innerText=W(e.duration),F(e)}),window.runtime.EventsOn("profile-switched",async()=>{await b()}));try{if(window.go&&window.go.main&&window.go.main.App){R=await window.go.main.App.GetInterfaces(),await b();const e=await window.go.main.App.GetStatus();g(e),v()}}catch(e){console.error("Init error:",e),r("[\u521D\u59CB\u5316\u5F02\u5E38] "+e)}}document.readyState==="loading"?document.addEventListener("DOMContentLoaded",A):A();

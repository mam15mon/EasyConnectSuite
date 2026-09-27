(function(){const t=document.createElement("link").relList;if(t&&t.supports&&t.supports("modulepreload"))return;for(const o of document.querySelectorAll('link[rel="modulepreload"]'))l(o);new MutationObserver(o=>{for(const s of o)if(s.type==="childList")for(const v of s.addedNodes)v.tagName==="LINK"&&v.rel==="modulepreload"&&l(v)}).observe(document,{childList:!0,subtree:!0});function n(o){const s={};return o.integrity&&(s.integrity=o.integrity),o.referrerpolicy&&(s.referrerPolicy=o.referrerpolicy),o.crossorigin==="use-credentials"?s.credentials="include":o.crossorigin==="anonymous"?s.credentials="omit":s.credentials="same-origin",s}function l(o){if(o.ep)return;o.ep=!0;const s=n(o);fetch(o.href,s)}})();const Z=new URLSearchParams(location.search).has("preview"),_=document.querySelector("#app");_.innerHTML=`
  <header class="top">
    <div class="brand">
      <svg class="brand-mark" viewBox="0 0 24 24" fill="none" stroke="#1f2328" stroke-width="1.8">
        <circle cx="12" cy="12" r="9"/>
        <circle cx="12" cy="12" r="3.2" fill="#047857" stroke="none"/>
        <path d="M12 3v5.8M12 15.2V21M3 12h5.8M15.2 12H21"/>
      </svg>
      <span class="brand-name">\u96A7\u9053\u503C\u73ED\u53F0</span>
      <span class="brand-sub">EasyConnect \u5185\u7F51\u901A\u9053</span>
    </div>
    <div class="top-status">
      <span class="uptime" id="uptime-display">00:00:00</span>
      <span class="lamp" id="state-lamp"></span>
      <span class="top-state-word" id="top-state-word">\u672A\u8FDE\u63A5</span>
    </div>
  </header>

  <div class="deck">
    <div class="deck-col-left">
      <!-- \u96A7\u9053\u4E3B\u673A\u677F -->
      <section class="panel master">
        <div class="state-line"><span class="state-word" id="state-word">\u672A\u8FDE\u63A5</span></div>
        <div class="state-note" id="state-note">\u901A\u9053\u672A\u5EFA\u7ACB</div>

        <button class="master-switch" id="btn-toggle">
          <span id="btn-label">\u5EFA\u7ACB\u96A7\u9053</span>
        </button>

        <dl class="readouts">
          <div class="readout"><dt>\u865A\u62DF\u5185\u7F51 IP</dt><dd class="pending" id="client-ip-val">\u2014</dd></div>
          <div class="readout"><dt>\u5185\u7F51 DNS</dt><dd class="pending" id="dns-val">\u2014</dd></div>
          <div class="readout"><dt>SOCKS5 \u4EE3\u7406</dt><dd id="socks-port-val">127.0.0.1:1080</dd></div>
          <div class="readout"><dt>HTTP \u4EE3\u7406</dt><dd id="http-port-val">127.0.0.1:1081</dd></div>
        </dl>
      </section>

      <!-- \u670D\u52A1\u5668\u914D\u7F6E -->
      <section class="panel">
        <div class="panel-head">
          <span class="panel-title">\u670D\u52A1\u5668\u914D\u7F6E</span>
          <div style="display:flex;gap:6px">
            <button class="btn-chip" id="btn-new-profile">\u65B0\u5EFA</button>
            <button class="btn-chip" id="btn-edit-profile">\u7F16\u8F91</button>
            <button class="btn-chip danger" id="btn-del-profile" title="\u5220\u9664\u5F53\u524D\u914D\u7F6E">\u5220\u9664</button>
          </div>
        </div>
        <div class="profiles-body">
          <div class="profile-row">
            <select class="profile-select" id="profile-select"></select>
          </div>
        </div>
      </section>
    </div>

    <!-- \u53EF\u8FBE\u8D44\u6E90 -->
    <div class="deck-col-right">
      <section class="panel routes">
        <div class="panel-head">
          <div>
            <span class="panel-title">\u53EF\u8FBE\u8D44\u6E90</span>
            <span class="panel-count" id="route-count"></span>
          </div>
          <div style="display:flex;gap:6px;align-items:center">
            <input class="routes-filter" id="routes-filter" type="text" placeholder="\u7B5B\u9009 IP / \u57DF\u540D" />
            <button class="btn-icon" id="btn-refresh-resources" title="\u5237\u65B0\u8D44\u6E90\u5217\u8868">\u5237\u65B0</button>
          </div>
        </div>
        <div class="route-table" id="route-table">
          <div class="route-empty">\u8FDE\u63A5\u540E\u5728\u6B64\u5217\u51FA\u670D\u52A1\u5668\u6388\u6743\u7684\u5185\u7F51\u8D44\u6E90</div>
        </div>
      </section>
    </div>
  </div>

  <footer class="hint-bar">
    <span>\u5173\u95ED\u7A97\u53E3\u5373\u6700\u5C0F\u5316\u5230\u6258\u76D8\uFF0C\u96A7\u9053\u4FDD\u6301\u8FD0\u884C</span>
    <span>\u65E5\u5FD7\u5B58\u4E8E <code>%APPDATA%\\EasyConnectVPN\\logs</code></span>
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
          <label class="form-label">\u914D\u7F6E\u540D\u79F0\uFF08\u5907\u6CE8\uFF09</label>
          <input type="text" class="form-input" id="form-name" placeholder="\u4F8B\u5982\uFF1A\u516C\u53F8\u751F\u4EA7\u73AF\u5883" />
        </div>
        <div class="form-row">
          <div class="form-group">
            <label class="form-label">VPN \u670D\u52A1\u5668\u5730\u5740</label>
            <input type="text" class="form-input" id="form-server" placeholder="vpn.example.com \u6216 IP" />
          </div>
          <div class="form-group">
            <label class="form-label">\u7AEF\u53E3</label>
            <input type="number" class="form-input" id="form-port" value="443" />
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label class="form-label">\u8D26\u53F7</label>
            <input type="text" class="form-input" id="form-username" />
          </div>
          <div class="form-group">
            <label class="form-label">\u5BC6\u7801</label>
            <input type="password" class="form-input" id="form-password" />
          </div>
        </div>

        <label class="toggle-row">
          <input type="checkbox" id="form-enable-2fa" />
          <span>\u4E8C\u6B21\u8BA4\u8BC1\uFF08\u8FDE\u63A5\u65F6\u81EA\u52A8\u63D0\u4EA4 TOTP\uFF09</span>
        </label>
        <div class="form-group" id="form-totp-container" style="display:none">
          <label class="form-label">TOTP Secret\uFF08Base32\uFF09</label>
          <input type="text" class="form-input" id="form-totp" placeholder="\u4F8B\u5982 JBSWY3DPEHPK3PXP" />
        </div>

        <label class="toggle-row">
          <input type="checkbox" id="form-enable-keepalive" />
          <span>\u81EA\u5B9A\u4E49\u4F1A\u8BDD\u4FDD\u6D3B\u5730\u5740\uFF08\u9ED8\u8BA4\u7528\u5F15\u64CE\u5185\u7F6E\u63A2\u6D4B\uFF09</span>
        </label>
        <div class="form-group" id="form-keepalive-container" style="display:none">
          <label class="form-label">\u4FDD\u6D3B URL\uFF08\u8FDE\u63A5\u671F\u95F4\u5B9A\u671F\u7ECF\u96A7\u9053\u8BBF\u95EE\uFF09</label>
          <input type="text" class="form-input" id="form-keepalive-url" placeholder="http://172.31.112.1" />
        </div>
        <label class="toggle-row">
          <input type="checkbox" id="form-disable-keepalive" />
          <span>\u5B8C\u5168\u7981\u7528\u4F1A\u8BDD\u4FDD\u6D3B\uFF08\u4E0D\u63A8\u8350\uFF09</span>
        </label>

        <div class="form-group">
          <label class="form-label">\u7ED1\u5B9A\u7269\u7406\u7F51\u5361</label>
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
`;function m(e){const t=document.getElementById("toast");t.innerText=e,t.classList.add("show"),setTimeout(()=>t.classList.remove("show"),2200)}function z(e){const t=Math.floor(e/3600).toString().padStart(2,"0"),n=Math.floor(e%3600/60).toString().padStart(2,"0"),l=Math.floor(e%60).toString().padStart(2,"0");return`${t}:${n}:${l}`}const E=document.getElementById("profile-select"),f=document.getElementById("btn-toggle"),b=document.getElementById("btn-label"),B=document.getElementById("uptime-display"),Q=document.getElementById("state-word"),w=document.getElementById("state-note"),ee=document.getElementById("top-state-word"),y=document.getElementById("client-ip-val"),h=document.getElementById("dns-val"),te=document.getElementById("socks-port-val"),ne=document.getElementById("http-port-val"),x=document.getElementById("route-table"),C=document.getElementById("route-count"),O=document.getElementById("routes-filter"),F=document.getElementById("modal-overlay"),oe=document.getElementById("modal-title"),R=document.getElementById("form-id"),V=document.getElementById("form-name"),K=document.getElementById("form-server"),D=document.getElementById("form-port"),H=document.getElementById("form-username"),U=document.getElementById("form-password"),g=document.getElementById("form-enable-2fa"),W=document.getElementById("form-totp-container"),G=document.getElementById("form-totp"),L=document.getElementById("form-nic"),p=document.getElementById("form-enable-keepalive"),S=document.getElementById("form-keepalive-container"),j=document.getElementById("form-keepalive-url"),k=document.getElementById("form-disable-keepalive");let r=[],a=null,u={running:!1,state:"disconnected"},q=["\u4EE5\u592A\u7F51","WLAN"],d=[];g.addEventListener("change",()=>{W.style.display=g.checked?"block":"none"});p.addEventListener("change",()=>{S.style.display=p.checked?"block":"none"});k.addEventListener("change",()=>{k.checked&&(p.checked=!1,S.style.display="none")});const ie={connected:"\u5DF2\u8FDE\u63A5",connecting:"\u63E1\u624B\u4E2D",disconnected:"\u672A\u8FDE\u63A5",error:"\u8FDE\u63A5\u51FA\u9519"};function J(e){!e||(e.socksPort&&(te.innerText="127.0.0.1:"+e.socksPort),e.httpPort&&(ne.innerText="127.0.0.1:"+e.httpPort),!e.running||e.state==="disconnected"?(y.innerText="\u2014",y.classList.add("pending"),h.innerText="\u2014",h.classList.add("pending")):(e.clientIp&&(y.innerText=e.clientIp,y.classList.remove("pending")),e.dns&&(h.innerText=e.dns,h.classList.remove("pending"))))}function c(e){u=e,document.body.dataset.state=e.state;const t=ie[e.state]||"\u672A\u8FDE\u63A5";Q.innerText=t,ee.innerText=t,e.state==="connected"?(b.innerText="\u65AD\u5F00\u96A7\u9053",f.disabled=!1,w.innerText="\u901A\u9053\u5DE5\u4F5C\u6B63\u5E38\uFF0C\u6D41\u91CF\u7ECF\u4EE3\u7406\u7AEF\u53E3\u8FDB\u51FA"):e.state==="connecting"?(b.innerText="\u6B63\u5728\u63E1\u624B",f.disabled=!0,w.innerText=e.lastError||"\u6B63\u5728\u767B\u5F55\u5E76\u5EFA\u7ACB\u96A7\u9053\u2026"):e.state==="error"?(b.innerText="\u91CD\u65B0\u5EFA\u7ACB",f.disabled=!1,w.innerText=e.lastError||"\u8FDE\u63A5\u5931\u8D25\uFF0C\u53EF\u91CD\u8BD5"):(b.innerText="\u5EFA\u7ACB\u96A7\u9053",f.disabled=!1,w.innerText=a?`\u5C06\u8FDE\u63A5 ${a.name}\uFF08${a.server}:${a.port}\uFF09`:"\u901A\u9053\u672A\u5EFA\u7ACB",B.innerText="00:00:00"),J(e)}async function I(){const e=await window.go.main.App.GetAppConfig();r=e.profiles||[],E.innerHTML="",r.forEach(t=>{const n=document.createElement("option");n.value=t.id,n.innerText=t.name+(t.enable2FA?" [2FA]":""),t.id===e.activeProfileId&&(n.selected=!0),E.appendChild(n)}),a=r.find(t=>t.id===e.activeProfileId)||r[0],u.running||c(u)}function se(e){!e||(a=e,u.running||c(u))}E.addEventListener("change",async e=>{const t=e.target.value;await window.go.main.App.SetActiveProfile(t),se(r.find(n=>n.id===t))});function X(e){oe.innerText=e?"\u7F16\u8F91\u670D\u52A1\u5668\u914D\u7F6E":"\u65B0\u5EFA\u670D\u52A1\u5668\u914D\u7F6E",R.value=e?e.id:"",V.value=e?e.name:"\u65B0\u914D\u7F6E",K.value=e?e.server:"",D.value=e?e.port:443,H.value=e?e.username:"",U.value=e?e.password:"",g.checked=e?!!e.enable2FA:!1,G.value=e&&e.totpSecret?e.totpSecret:"",W.style.display=g.checked?"block":"none",p.checked=!!(e&&e.keepAliveUrl),j.value=e&&e.keepAliveUrl?e.keepAliveUrl:"",S.style.display=p.checked?"block":"none",k.checked=e?!!e.disableKeepAlive:!1,L.innerHTML="",q.forEach(t=>{const n=document.createElement("option");n.value=t,n.innerText=t,e&&e.bindInterface===t&&(n.selected=!0),L.appendChild(n)}),F.classList.add("show")}function $(){F.classList.remove("show")}document.getElementById("btn-edit-profile").addEventListener("click",()=>{a&&X(a)});document.getElementById("btn-new-profile").addEventListener("click",()=>{X(null)});document.getElementById("btn-del-profile").addEventListener("click",async()=>{if(!!a){if(r.length<=1){m("\u81F3\u5C11\u4FDD\u7559\u4E00\u4E2A\u914D\u7F6E\uFF0C\u65E0\u6CD5\u5220\u9664");return}confirm(`\u786E\u5B9A\u8981\u5220\u9664\u914D\u7F6E\u300C${a.name}\u300D\u5417\uFF1F`)&&(await window.go.main.App.DeleteProfile(a.id),m("\u914D\u7F6E\u5DF2\u5220\u9664"),await I())}});document.getElementById("modal-close-btn").addEventListener("click",$);document.getElementById("btn-modal-cancel").addEventListener("click",$);document.getElementById("btn-modal-save").addEventListener("click",async()=>{const e={id:R.value.trim(),name:V.value.trim()||"\u672A\u547D\u540D\u914D\u7F6E",server:K.value.trim(),port:parseInt(D.value)||443,username:H.value.trim(),password:U.value.trim(),enable2FA:g.checked,totpSecret:G.value.trim(),bindInterface:L.value,disableZjuConfig:!0,disableKeepAlive:k.checked,keepAliveUrl:p.checked?j.value.trim():""};if(!e.server||!e.username){m("\u670D\u52A1\u5668\u5730\u5740\u548C\u8D26\u53F7\u4E0D\u80FD\u4E3A\u7A7A");return}await window.go.main.App.SaveProfile(e),$(),m("\u914D\u7F6E\u5DF2\u4FDD\u5B58"),await I()});f.addEventListener("click",async()=>{if(u.running)await window.go.main.App.StopVPN();else{c({running:!0,state:"connecting"});const e=await window.go.main.App.StartVPN();e!=="OK"&&(m("[\u8FDE\u63A5\u5931\u8D25] "+e),c({running:!1,state:"error",lastError:e}))}});function T(e){d=e||[];const t=O.value.trim().toLowerCase(),n=t?d.filter(i=>String(i.target||i.Target||"").toLowerCase().includes(t)):d;if(!d.length){x.innerHTML='<div class="route-empty">\u8FDE\u63A5\u540E\u5728\u6B64\u5217\u51FA\u670D\u52A1\u5668\u6388\u6743\u7684\u5185\u7F51\u8D44\u6E90</div>',C.innerText="";return}const l=i=>String(i.kind||i.Kind||"").toLowerCase(),o=n.length;if(C.innerText=t?`${o}/${d.length}`:`\u5171 ${d.length} \u6761`,!o){x.innerHTML=`<div class="route-empty">\u6CA1\u6709\u5339\u914D\u300C${t}\u300D\u7684\u8D44\u6E90</div>`;return}const s=n.filter(i=>l(i)==="ip"),v=n.filter(i=>l(i)==="domain"),Y=i=>`
    <div class="route-row">
      <span class="route-target">${i.target||i.Target||""}</span>
      <span class="route-badge">${i.ports||i.Ports||""} \xB7 ${i.protocol||i.Protocol||""}</span>
    </div>`,M=(i,P)=>P.length?`<div class="route-group">${i} \xB7 ${P.length}</div>`+P.map(Y).join(""):"";x.innerHTML=M("IP \u6BB5",s)+M("\u57DF\u540D",v)}async function A(){if(!(!window.go||!window.go.main||!window.go.main.App))try{T(await window.go.main.App.GetVPNResources())}catch(e){console.error("GetVPNResources error:",e)}}O.addEventListener("input",()=>T(d));document.getElementById("btn-refresh-resources").addEventListener("click",()=>{A()});function ae(){const e=[{id:"p1",name:"IDC \u751F\u4EA7 (\u5E262FA)",server:"111.205.145.240",port:58443,enable2FA:!0},{id:"p2",name:"\u6D4B\u8BD5\u73AF\u5883",server:"10.0.0.8",port:443,enable2FA:!1}];r=e,a=e[0],E.innerHTML=e.map(n=>`<option value="${n.id}">${n.name}${n.enable2FA?" [2FA]":""}</option>`).join(""),c({running:!0,state:"connected",clientIp:"192.168.0.2",dns:"172.31.112.1",socksPort:"1080",httpPort:"1081"}),B.innerText="08:20:31";const t=[];for(let n=0;n<14;n++)t.push({kind:"ip",target:`172.31.10${n}.0 ~ 172.31.10${n}.255`,ports:"\u5168\u90E8\u7AEF\u53E3",protocol:"\u5168\u90E8\u534F\u8BAE"});t.push({kind:"domain",target:"ops.internal.example.com \u2192 172.31.128.12",ports:"\u56FA\u5B9A\u89E3\u6790",protocol:"-"}),t.push({kind:"domain",target:"git.internal.example.com",ports:"443",protocol:"TCP"}),T(t)}async function N(){for(let e=0;e<40&&!(window.go&&window.go.main&&window.go.main.App&&window.runtime);e++)await new Promise(t=>setTimeout(t,50));window.runtime&&(window.runtime.EventsOn("vpn-status",e=>{c(e),e.running||A()}),window.runtime.EventsOn("vpn-resources",e=>{T(e)}),window.runtime.EventsOn("status-tick",e=>{B.innerText=z(e.duration),J(e)}),window.runtime.EventsOn("profile-switched",async()=>{await I()}));try{window.go&&window.go.main&&window.go.main.App?(q=await window.go.main.App.GetInterfaces(),await I(),c(await window.go.main.App.GetStatus()),A()):Z&&ae()}catch(e){console.error("Init error:",e),m("[\u521D\u59CB\u5316\u5F02\u5E38] "+e)}}document.readyState==="loading"?document.addEventListener("DOMContentLoaded",N):N();

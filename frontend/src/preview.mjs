// Explicit ?preview only: no credentials, network requests or persisted profiles.
export function installPreview(onStatus) {
  let selected = 'topsec';
  let nextID = 5;
  const profiles = [
    {id:'topsec', name:'研发内网', protocol:'topsec', server:'vpn.example.com', port:58443, tunMode:true, socksPort:1080, httpPort:1081},
    {id:'uni', name:'办公网络', protocol:'univpn', server:'office.example.com', port:7443, tunMode:true, socksPort:1082, httpPort:1083},
    {id:'easy', name:'测试环境', protocol:'easyconnect', server:'lab.example.com', port:443, tunMode:false, socksPort:1084, httpPort:1085},
    {id:'error', name:'备用网络', protocol:'easyconnect', server:'backup.example.com', port:443, tunMode:false, socksPort:1086, httpPort:1087},
  ];
  const statuses = new Map(profiles.map((p,i) => [p.id, {profileId:p.id, activeProfile:p.name, protocol:p.protocol, tunMode:p.tunMode, disableProxy:false, socksPort:String(p.socksPort), httpPort:String(p.httpPort), running:i<2, state:i<2?'connected':i===3?'error':'disconnected', duration:i<2?3182-i*900:0, clientIp:i<2?`192.0.2.${10+i}`:'', dns:i===1?'198.51.100.53':'', lastError:i===3?'模拟错误：连接网关超时，请检查网络后重试。':''}]));
  const resources = new Map([
    ['topsec', [{kind:'ip', target:'198.51.100.0 ~ 198.51.100.255', ports:'全部端口', protocol:'全部协议'}, {kind:'ip', target:'203.0.113.0 ~ 203.0.113.255', ports:'全部端口', protocol:'全部协议'}]],
    ['uni', [{kind:'ip', target:'192.0.2.0 ~ 192.0.2.255', ports:'443', protocol:'TCP'}, {kind:'domain', target:'intranet.example.com', ports:'443', protocol:'TCP'}]],
    ['easy', [{kind:'domain', target:'test.example.com', ports:'443', protocol:'TCP'}]],
  ]);
  const generation = new Map();
  const clone = value => structuredClone(value);
  const emit = id => onStatus(clone(statuses.get(id)));
  const api = {
    GetAppConfig:async()=>clone({profiles, activeProfileId:selected}),
    GetConnections:async()=>clone([...statuses.values()]),
    GetStatus:async()=>clone(statuses.get(selected)),
    GetVPNResources:async()=>clone(statuses.get(selected)?.state==='connected' ? resources.get(selected)||[] : []),
    GetProfileVPNResources:async id=>{
      if(!profiles.some(p=>p.id===id))throw new Error('服务器配置不存在');
      return clone(statuses.get(id)?.state==='connected' ? resources.get(id)||[] : []);
    },
    GetProfileLogs:async id=>[`${new Date().toLocaleTimeString()} [VPN profile="${id}"] status phase=${statuses.get(id)?.state||'disconnected'}`,`${new Date().toLocaleTimeString()} 演示记录：实际阶段、停止原因和流量统计由新版引擎写入。`],
    GetInterfaces:async()=>['WLAN','以太网'],
    SetActiveProfile:async id=>{ if(!profiles.some(p=>p.id===id)) return false; selected=id; return true; },
    SetProfileTUNMode:async(id,enabled)=>{ const p=profiles.find(p=>p.id===id); if(!p)throw new Error('配置不存在'); p.tunMode=enabled; const status=statuses.get(id); if(status.running){status.tunState=enabled?"attaching":"detaching";emit(id);await new Promise(r=>setTimeout(r,300));status.tunMode=enabled;status.tunState=enabled?"on":"off";status.tunError="";emit(id);} },
    SetProxyEnabled:async enabled=>{ profiles.find(p=>p.id===selected).disableProxy=!enabled; },
    StartProfileVPN:async id=>{
      const status=statuses.get(id);
      const profile=profiles.find(p=>p.id===id);
      const version=(generation.get(id)||0)+1;
      generation.set(id,version);
      Object.assign(status,{tunMode:profile.tunMode,disableProxy:!!profile.disableProxy,socksPort:String(profile.socksPort),httpPort:String(profile.httpPort),running:true,state:'connecting',stage:'认证中',message:'演示：正在连接网关…',lastError:'',duration:0}); emit(id);
      await new Promise(resolve=>setTimeout(resolve,650));
      if(generation.get(id)!==version)return 'OK';
      Object.assign(status,{state:'connected',stage:'',clientIp:'192.0.2.20'});emit(id);return 'OK';
    },
    StopProfileVPN:async id=>{generation.set(id,(generation.get(id)||0)+1);Object.assign(statuses.get(id),{running:false,state:'disconnected',stage:'',message:'',duration:0,clientIp:'',dns:''});emit(id);return 'OK';},
    StopAllVPN:async()=>{ for(const s of statuses.values())if(s.running)await api.StopProfileVPN(s.profileId); return 'OK'; },
    SaveProfile:async p=>{
      if(!p.id)p.id=`preview-${nextID++}`;
      const index=profiles.findIndex(v=>v.id===p.id);
      if(index>=0)profiles[index]=clone(p);else profiles.push(clone(p));
      statuses.set(p.id,{profileId:p.id, activeProfile:p.name, protocol:p.protocol, running:false,state:'disconnected'});
      selected=p.id;
    },
    DeleteProfile:async id=>{ if(statuses.get(id)?.running)return false;const i=profiles.findIndex(p=>p.id===id);if(i<0)return false;profiles.splice(i,1);statuses.delete(id);if(selected===id)selected=profiles[0]?.id;return true;},
  };
  window.go={main:{App:api}};
  setInterval(()=>{for(const s of statuses.values())if(s.running){s.duration++;emit(s.profileId);}},1000);
}

import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import test from 'node:test';
import assert from 'node:assert/strict';

// Execute the production functions with controlled Wails responses; no timers or live VPNs.
const source = readFileSync(new URL('./main.js', import.meta.url), 'utf8');
function productionFunction(name) {
  let start = source.indexOf(`function ${name}(`);
  if (source.slice(start - 6, start) === 'async ') start -= 6;
  assert.ok(start >= 0, `production function ${name} exists`);
  return source.slice(start, source.indexOf('\n}', start) + 2);
}
function deferred() {
  let resolve;
  const promise = new Promise(done => { resolve = done; });
  return { promise, resolve };
}
function workspace(api = {}) {
  const state = {
    modeSaving:false, cachedProfiles:[{id:'a',name:'A'},{id:'b',name:'B'}],
    selectionVersion:0, selectionPending:false, activeProfile:{id:'a'}, workspacePage:'resources',
    profileSelect:{}, routesFilter:{}, currentStatus:{profileId:'a',running:false,state:'disconnected'},
    connectionStatuses:[], selectionQueue:Promise.resolve(), profileResources:new Map(), resourceRequest:0,
    pendingActions:new Set(), actionVersions:new Map(), rendered:[], notifications:[],
    document:{getElementById:()=>({classList:{add(){},remove(){}}})},
    window:{go:{main:{App:api}}}, renderConnections(){},
    updateStatusUI(status) { if (!status.profileId || status.profileId === state.activeProfile?.id) state.currentStatus = status; },
    receiveStatus(status) {
      const index = state.connectionStatuses.findIndex(s => s.profileId === status.profileId);
      if (index >= 0) state.connectionStatuses[index] = status; else state.connectionStatuses.push(status);
      state.updateStatusUI(status);
    },
    renderResources(rows) { state.rendered = rows; }, showToast(message) { state.notifications.push(message); },
  };
  vm.createContext(state);
  vm.runInContext(['selectProfile','refreshResources','toggleConnection','refreshConnections','receiveStatus'].map(productionFunction).join('\n'), state);
  return state;
}

test('TUN mode request keeps captured profile and releases busy state after rejected UAC', async () => {
  const entered=deferred(), result=deferred();
  const calls=[];
  const state=workspace({SetProfileTUNMode:async(id,enabled)=>{calls.push([id,enabled]);entered.resolve();return result.promise;},GetConnections:async()=>[{profileId:'b',running:true,state:'connected'}]});
  state.refreshProfiles=async()=>{};
  vm.runInContext(productionFunction('saveTUNMode'),state);
  const operation=state.saveTUNMode(true);
  await entered.promise;
  assert.equal(state.modeSaving,true);
  state.activeProfile={id:'b'};
  result.resolve(Promise.reject(new Error('UAC canceled')));
  await assert.rejects(operation,/UAC canceled/);
  assert.deepEqual(calls,[['a',true]]);
  assert.equal(state.modeSaving,false);
  assert.equal(state.currentStatus.profileId,'b');
});

test('resource refresh targets B while backend selection is still A', async () => {
  const held = deferred(), entered = deferred();
  let backend = 'a';
  const rows = id => [{target:`${id}-resources`}];
  const state = workspace({
    SetActiveProfile:async id => { backend=id; if(id==='a') { entered.resolve(); await held.promise; } return true; },
    GetStatus:async()=>({profileId:backend}), GetVPNResources:async()=>rows(backend),
    GetProfileVPNResources:async id=>rows(id),
  });
  const first = state.selectProfile('a');
  await entered.promise;
  const second = state.selectProfile('b');
  try {
    await state.refreshResources(false);
    assert.equal(state.rendered[0].target,'b-resources');
    assert.equal(state.profileResources.get('b')[0].target,'b-resources');
  } finally { held.resolve(); await Promise.all([first,second]); }
});

test('late startup error cannot replace a newer successful tray connection', async () => {
  const result=deferred(), entered=deferred();
  const connected={profileId:'a',running:true,state:'connected',clientIp:'192.0.2.22'};
  const state=workspace({
    StartProfileVPN:async()=>{entered.resolve(); return result.promise;},
    GetConnections:async()=>[connected], GetStatus:async()=>connected,
    GetVPNResources:async()=>[], GetProfileVPNResources:async()=>[],
  });
  const action=state.toggleConnection('a');
  await entered.promise;
  state.receiveStatus(connected);
  result.resolve('old startup returned late');
  await action;
  assert.equal(state.currentStatus.state,'connected');
  assert.equal(state.currentStatus.clientIp,'192.0.2.22');
  assert.equal(state.pendingActions.size,0);
});

test('newest resource response wins when requests return in reverse order', async () => {
  const first=deferred(), second=deferred();
  let requests=0;
  const state=workspace({GetProfileVPNResources:()=>++requests===1?first.promise:second.promise});
  const old=state.refreshResources(false), latest=state.refreshResources(false);
  second.resolve([{target:'new'}]); await latest;
  first.resolve([{target:'old'}]); await old;
  assert.equal(state.rendered[0].target,'new');
  assert.equal(state.profileResources.get('a')[0].target,'new');
});

test('A to B to A selection rejects the first A response', async () => {
  const first=deferred();
  const state=workspace({GetProfileVPNResources:()=>first.promise});
  const old=state.refreshResources(false);
  state.activeProfile={id:'b'}; state.selectionVersion++;
  state.activeProfile={id:'a'}; state.selectionVersion++;
  state.renderResources([{target:'current-A'}]);
  first.resolve([{target:'old-A'}]); await old;
  assert.equal(state.rendered[0].target,'current-A');
  assert.equal(state.profileResources.has('a'),false);
});

test('disconnect event invalidates pending resource response', async () => {
  const pending=deferred();
  const state=workspace({GetProfileVPNResources:()=>pending.promise});
  const refresh=state.refreshResources(false);
  state.receiveStatus({profileId:'a',running:false,state:'disconnected'});
  pending.resolve([{target:'obsolete'}]); await refresh;
  assert.equal(state.rendered.length,0);
  assert.equal(state.profileResources.has('a'),false);
});

test('resource update event takes precedence over a pending read', async () => {
  const pending=deferred();
  const state=workspace({GetProfileVPNResources:()=>pending.promise});
  const handlers=new Map();
  state.window.runtime={EventsOn:(event,handler)=>handlers.set(event,handler)};
  const start=source.indexOf("window.runtime.EventsOn('vpn-resources'");
  const end=source.indexOf("window.runtime.EventsOn('profile-switched'",start);
  vm.runInContext(source.slice(start,end),state);
  const refresh=state.refreshResources(false);
  handlers.get('vpn-resources')({profileId:'a',entries:[{target:'event-new'}]});
  pending.resolve([{target:'read-old'}]); await refresh;
  assert.equal(state.rendered[0].target,'event-new');
  assert.equal(state.profileResources.get('a')[0].target,'event-new');
});

test('actual current startup failure remains visible and releases pending action', async () => {
  const failed={profileId:'a',running:false,state:'error',lastError:'authentication rejected'};
  const state=workspace({StartProfileVPN:async()=>failed.lastError,GetConnections:async()=>[failed]});
  await state.toggleConnection('a');
  assert.equal(state.currentStatus.state,'error');
  assert.equal(state.currentStatus.lastError,'authentication rejected');
  assert.equal(state.pendingActions.size,0);
  assert.ok(state.notifications.some(message=>message.includes('authentication rejected')));
});

test('unknown resource ID reports failure without querying selected-profile API', async () => {
  let legacyCalls=0;
  const state=workspace({
    GetProfileVPNResources:async()=>{throw new Error('服务器配置不存在');},
    GetVPNResources:async()=>{legacyCalls++;return [{target:'wrong'}];},
  });
  state.activeProfile={id:'unknown'};
  await state.refreshResources(false);
  assert.equal(legacyCalls,0);
  assert.equal(state.profileResources.size,0);
  assert.ok(state.notifications.some(message=>message.includes('服务器配置不存在')));
});

test('preview resource API uses explicit ID and rejects unknown profiles', async () => {
  const preview=readFileSync(new URL('./preview.mjs',import.meta.url),'utf8');
  const scope={window:{},structuredClone,setInterval(){}};
  vm.createContext(scope);
  vm.runInContext(preview.replace('export function installPreview','function installPreview'),scope);
  scope.installPreview(()=>{});
  const api=scope.window.go.main.App;
  const selected=(await api.GetAppConfig()).activeProfileId;
  assert.equal(selected,'topsec');
  const rows=await api.GetProfileVPNResources('uni');
  assert.equal(rows[0].target,'192.0.2.0 ~ 192.0.2.255');
  assert.equal((await api.GetAppConfig()).activeProfileId,selected);
  assert.equal((await api.GetProfileVPNResources('easy')).length,0);
  await assert.rejects(api.GetProfileVPNResources('unknown'),/服务器配置不存在/);
});

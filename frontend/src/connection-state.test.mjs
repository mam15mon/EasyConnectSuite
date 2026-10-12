import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import test from 'node:test';
import assert from 'node:assert/strict';

// Execute the production functions with controlled Wails responses; no timers or live VPNs.
const source = readFileSync(new URL('./main.js', import.meta.url), 'utf8');
function productionFunction(name) {
  const start = source.indexOf(`async function ${name}(`);
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
  vm.runInContext(['selectProfile','refreshResources','toggleConnection','refreshConnections'].map(productionFunction).join('\n'), state);
  return state;
}

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

import test from 'node:test';
import assert from 'node:assert/strict';
import { connectionFeedback, buildDiagnostics } from './connection-feedback.mjs';

test('stage clocks and reconnect labels do not confuse uptime with waiting', () => {
 const status={state:'connecting',stage:'重连中',stageDetail:'认证中',stageStartedAt:10000,duration:3600};
 assert.deepEqual(connectionFeedback(status,0,15000),{label:'重连 · 认证中',waiting:true,seconds:5,timeLabel:'本阶段',note:undefined});
 assert.equal(connectionFeedback({...status,stageStartedAt:0},0,15000).seconds,null);
 assert.equal(connectionFeedback(status,0,9000).seconds,0);
 assert.equal(connectionFeedback({state:'connected',duration:3600},0,15000).seconds,3600);
 const preparing=connectionFeedback({state:'disconnected'},12000,15000);
 assert.equal(preparing.seconds,3);
 assert.match(preparing.label,/系统授权/);
 assert.equal(connectionFeedback({state:'error',lastError:'连接已断开且自动重连失败: test'}).label,'重连失败');
});

test('diagnostic export keeps typed evidence but excludes raw credentials and arbitrary labels', () => {
 const secret='PRIVATE_CANARY';
 const status={protocol:'easyconnect',state:'connecting',stage:'重连中',stageDetail:'认证中',stageStartedAt:Date.now(),lastError:secret,profileId:secret,clientIp:'192.0.2.9',tunError:secret};
 const report=buildDiagnostics(status,[
  `[VPN profile="${secret}"] operation=12 name=tun_update goroutine=40 phase=end outcome=failed stage=worker_update_reply elapsed=1m2.3s stage_elapsed=2s canceled=false timeout=true error_type=*net.OpError cause=https://user:${secret}@vpn.private.test:443/?token=${secret}`,
  `operation=13 name=x goroutine=40 phase=end stage=${secret} error_type=${secret} cause=${secret}`,
  `password=${secret} cookie=${secret} TWFID=${secret} username=${secret} 192.0.2.9 [2001:db8::1] C:\Users\${secret}\engine.log`,
  `operation=12 name=x phase=stack_payload snapshot=1 stack=${secret}`,
 ]);
 for(const forbidden of [secret,'vpn.private.test','192.0.2.9','2001:db8','profile=','cause=','stack=','password=','token=']) assert.ok(!report.includes(forbidden),forbidden);
 for(const field of ['operation=12','stage=worker_update_reply','elapsed=1m2.3s','timeout=true','error_type=*net.OpError']) assert.ok(report.includes(field),field);
 assert.match(buildDiagnostics({state:'disconnected'},[]),/暂无可导出的阶段记录/);
});

test('diagnostic export explains target connection failure without exporting its endpoint', () => {
 const lastError='VPN 目标连接失败，请检查服务器地址、端口及网络连通性: TopSAP 获取认证参数: Post "https://PRIVATE_CANARY@vpn.private.test:7443/": dial tcp 192.0.2.9:7443: connect: connection refused';
 const report=buildDiagnostics({protocol:'topsec',state:'error',lastError},[]);
 assert.match(report,/失败原因: VPN 目标连接失败，请检查服务器地址、端口及网络连通性/);
 for(const secret of ['PRIVATE_CANARY','vpn.private.test','192.0.2.9','7443','Post','dial tcp']) assert.ok(!report.includes(secret),secret);
 for(const status of [
  {state:'disconnected',lastError},
  {state:'error',lastError:'VPN 登录失败: TopSAP 认证被拒绝 Ret=-1001'},
  {state:'error',lastError:'VPN 登录失败: tls: failed to verify certificate'},
  {state:'error',lastError:'VPN 登录失败: context canceled'},
 ]) assert.ok(!buildDiagnostics(status,[]).includes('失败原因:'),status.lastError);
});

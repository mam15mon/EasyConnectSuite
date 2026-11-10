const states = {connected:'已连接', connecting:'连接中', disconnected:'未连接', error:'连接失败'};

export function connectionFeedback(status, preparingAt = 0, now = Date.now()) {
  const connecting = status.state === 'connecting';
  const started = preparingAt || (connecting ? status.stageStartedAt : 0);
  let label = states[status.state] || '未连接';
  if (preparingAt) label = '准备 TUN / 系统授权';
  else if (connecting) label = status.stage === '重连中'
    ? `重连 · ${status.stageDetail || '等待恢复'}` : status.stage || '连接中';
  else if (status.state === 'error' && status.lastError?.includes('自动重连失败')) label = '重连失败';
  return {
    label, waiting:!!preparingAt || connecting,
    seconds:started ? Math.max(0, Math.floor((now-started)/1000)) : connecting ? null : status.duration || 0,
    timeLabel:preparingAt || connecting ? '本阶段' : '本次运行',
    note:preparingAt ? '正在准备 TUN；如弹出系统授权窗口，请确认后继续。' : status.message,
  };
}

// Only known local labels and typed counters leave the raw log. Never copy
// free-form cause/message, profile identifiers, endpoints or stack source paths.
const stages = new Set([
  'authentication','underlay_setup','resource_load','tun_update','policy_publish',
  'previous_resolver_close','resources_callback','status_callback','proxy_start',
  'wait_tun_operation_lock','resource_view','wait_backend_policy_lock','build_worker_spec',
  'dns_namespace_discovery','validate_discovery','worker_update_reply','failed_update_cleanup',
  'cancel_engine','tun_detach','terminal_callback','begin',
]);
const uiStages = new Set(['准备中','认证中','建立隧道','加载资源','启动代理','重连中','更新 TUN','更新资源','重建隧道','配置网卡','识别 DNS','配置路由','配置 DNS']);
const phases = new Set(['begin','advance','end','pending','cancel_requested','stack_begin','stack_end','stack_rate_limited']);
const outcomes = new Set(['updated','returned','discarded','not_attached','failed','interrupted','skipped']);
const errorTypes = new Set(['<nil>','*errors.errorString','*fmt.wrapError','*net.OpError','*net.DNSError','*url.Error','context.deadlineExceededError']);

export function buildDiagnostics(status, lines) {
  const report = ['VPN 诊断摘要（不含原始日志、地址和凭据）'];
  if (['easyconnect','univpn','topsec'].includes(status.protocol)) report.push(`协议: ${status.protocol}`);
  if (states[status.state]) report.push(`状态: ${states[status.state]}`);
  if (status.state === 'error' && status.lastError?.startsWith('VPN 目标连接失败，')) {
    report.push('失败原因: VPN 目标连接失败，请检查服务器地址、端口及网络连通性');
  }
  for (const key of ['stage','stageDetail']) if (uiStages.has(status[key])) report.push(`${key}: ${status[key]}`);
  const feedback = connectionFeedback(status);
  if (feedback.waiting && feedback.seconds !== null) report.push(`本阶段秒数: ${feedback.seconds}`);
  if (['off','on','attaching','detaching','error'].includes(status.tunState)) report.push(`TUN: ${status.tunState}`);
  const evidence = [];
  for (const line of lines) {
    if (!/\boperation=\d+\s+name=/.test(line)) continue;
    const fields = [];
    for (const match of line.matchAll(/\b(operation|phase|stage|outcome|elapsed|stage_elapsed|canceled|timeout|error_type)=([^\s]+)/g)) {
      const [,key,value] = match;
      const allowed = key === 'operation' ? /^\d+$/.test(value)
        : key === 'phase' ? phases.has(value)
        : key === 'stage' ? stages.has(value)
        : key === 'outcome' ? outcomes.has(value)
        : key === 'error_type' ? errorTypes.has(value)
        : key === 'elapsed' || key === 'stage_elapsed' ? /^(?:\d+(?:\.\d+)?(?:ns|µs|ms|s|m|h))+$/.test(value)
        : value === 'true' || value === 'false';
      if (allowed) fields.push(`${key}=${value}`);
    }
    // Stack payloads and unrecognised lines have no allowed phase field.
    if (fields.some(field=>field.startsWith('phase='))) evidence.push(fields.join(' '));
  }
  report.push(evidence.length ? evidence.join('\n') : '暂无可导出的阶段记录，请在本机运行记录中查看详情。');
  return report.join('\n');
}

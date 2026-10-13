# Logging

The zju-connect engine uses its log package and the standard Go logger. engine.startLogCapture registers with the process-wide engineLogs hub, which owns shared rotating files and sends lines to OnLog callbacks. Removing one engine must not restore logger output or close files still used by another. Never call standard log.SetOutput while holding the hub lock. GUI connections share APPDATA/EasyConnectVPN/logs/engine.log; it is a combined process log.

engine.rotatingWriter caps engine.log at 8 MiB and retains one .old generation (about 16 MiB total). Reuse this pattern for engine integration rather than adding a logging service.

Log connection phases, protocol selection, reconnect decisions and cleanup failures. UI status travels through vpn-status with profileId/protocol; vpn-resources carries {profileId, entries}; vpn-connections carries all profile statuses. Callbacks must check connection identity and cancellation before changing state, so stale callbacks cannot overwrite a restarted profile or another selected profile.

Never print passwords, TOTP secrets, tokens, full authentication payloads or decrypted packets. Native command lines must not contain VPN passwords. Research and tests use local mock gateways or redacted metadata.

UniVPN setup rejection must persist `UniVPN authentication rejected: code=N (0xXXXXXXXX)` through the existing engine logger before returning the terminal error. Startup failure messages returned to the UI are not automatically written into engine.log. Log only the typed numeric code; validate persistence with the shared log capture and reject credential/token output in the test.

TopSAP must persist setup failures before returning to the GUI, including protocol stage and wrapped cause. Standard-logger messages carry a process-local session number, PID and quoted profile ID so concurrent sessions and restarted processes are distinguishable. Record gateway/certificate policy, auth/config HTTP endpoint/status/timing, numeric Ret or missing Ret, resource presence/count, TLS/ACK progress, first IPv4 send/receive size, aggregate byte counts on close and logout results. Do not log request forms, username/password, S_CODE, cookies, session token or packet bodies. Test with the existing engineLogs capture, including Ret=0 with missing resources and terminal rejection; an error visible only in the UI does not satisfy this contract.

Engine lifecycle and gVisor reconnect logs carry PID, engine/stack identity and quoted profile ID. Log progress/state before publishing it to Wails, startup failure with its cause, cleanup stage start/completion/failure and elapsed time. Capture the stop request before cancelling: user_disconnect, user_disconnect_all or app_shutdown. A terminal transport failure records tunnel_unrecoverable and the cause. Treat context prefixes as literal data, never as printf format strings.

TopSAP samples its existing transport once per minute: uptime, actual sample gap, RX/TX byte totals and deltas, last RX/TX age or never. Record idle samples with zero deltas; zero traffic alone is not a tunnel error. Sampling sends no probe or speculative keepalive. Cancellation stops the sampler; concurrent counters are atomic. gVisor records keepalive timeout/relogin reasons through the same capture; unwrap HTTP URL errors so their embedded URLs are not logged.

The tray window logs received WM_POWERBROADCAST suspend/resume notifications, with the source and current running-profile states. These events do not distinguish sleep from hibernation and do not prove reconnection. No sleep prevention or new recovery mechanism is implied. Microsoft defines the message and PBT_APMRESUMEAUTOMATIC in https://learn.microsoft.com/en-us/windows/win32/power/wm-powerbroadcast and https://learn.microsoft.com/en-us/windows/win32/power/pbt-apmresumeautomatic.

GetProfileLogs accepts an existing profile ID, reads only the fixed .old/current files and returns the latest 200 matching quoted-profile entries. Unknown profiles and read failures are explicit errors; absent files are an empty fresh-log state. Rendering uses textContent and async results must still match the selected profile. The full combined file remains the investigation source.


EasyConnect authentication errors expose only local stage labels and bounded numeric ErrorCode/pwpErrorCode values. Password rejection retains ErrLoginRejected. No raw password/SMS/TOTP/certificate/keepalive responses, config/resource XML dumps, or token-bearing binary handshake dumps may reach logs/errors; resource responses are no longer automatically persisted as raw XML. Unwrap HTTP URL errors before logging keepalive failures because the query contains TWFID. Regression tests enable debug logging and place a private canary in failed replies for every auth stage.

## Operation evidence contract

### 1. Scope / trigger

The user requires future failures to leave evidence sufficient to locate the failing or blocked operation. Apply this to every new or changed authentication, startup, reconnect, TUN, IPC control and cleanup path. An unexplained last line is insufficient. Record facts and explicit unknowns; missing progress alone does not establish deadlock, Wi-Fi loss, gateway refusal or a driver fault.

### 2. Signatures

Use the existing nested module's log sink, not a separate logging service:

```go
func log.BeginOperation(prefix, name string, cancelled <-chan struct{}) *log.Operation
func (o *log.Operation) Step(stage string)
func (o *log.Operation) Finish(outcome string, err error)
```

Call `Step` before a potentially blocking action. Always finish the operation on return or panic unwinding, before closing its log capture. Names/stages are local code labels. Never pass profiles, credentials, replies or packets as stage names. UniVPN `Options.OnStage func(string)` reports only fixed transport/protocol stage labels; it must not carry authentication fields.

### 3. Contracts

- Log PID, engine/profile/protocol or stack identity, monotonically increasing process-local operation ID, initiating goroutine ID, operation name, current/previous stage, stage/total duration, outcome, cancellation, timeout, error type and safe wrapped cause. Argument-free stack headers let the initiating goroutine be matched to its operation. Keep `%` in identities literal.
- Operation observation checks every five seconds. A stage with no progress for at least 30 seconds emits `phase=pending evidence=no_stage_progress root_cause=undetermined`, then one bounded all-goroutine snapshot. This is a diagnostic threshold, not a new timeout or recovery action. Cancellation emits a request marker and does not stop observation until the operation actually returns.
- At most one snapshot attempt per operation; at most one process snapshot per minute. Other pending operations reference its snapshot ID. Capture is capped at 256 KiB and reports `truncated`; preserve goroutine states/functions/source lines, strip function argument words/addresses. Every snapshot line retains the triggering operation identity and snapshot ID.
- TUN policy update names operation-lock wait, backend-policy-lock wait, spec construction, DNS namespace discovery, Worker reply wait and failure cleanup. Preserve `session/attachment/policy` and resource counts without resource bodies.
- TUN IPC control requests log both `direction=send` and `direction=handle`, PID, profile/session/attachment/policy, request ID, pending/active counts and stages for request transmission, reply wait, handler and reply transmission. Do not trace each ping, dial or DNS packet; never log Message.Data, auth data or full envelopes. Existing IPC schema and deadlines remain unchanged.
- Worker diagnostic operation lines go both through existing IPC and to its existing stderr crash log, so control-pipe failure does not erase its local evidence. GUI remains the engine.log writer. Diagnostic stack bounds/frequency apply independently in each process.
- Unwrap token-bearing HTTP URL errors before writing their underlying cause. Existing credential restrictions apply to diagnostics. Evidence does not prove live gateway compatibility or identify remote/system faults without corresponding measurements.

### 4. Validation / error matrix

| Observed event | Required evidence |
| --- | --- |
| Normal completion | Same operation ID, stages/durations, end outcome; observer stops |
| Error / timeout | Failing stage, error type/cause and timeout flag; never report success |
| Cancellation | Request marker when observed, canceled flag on end; if ignored, pending snapshot still runs |
| Step blocks before acquiring a lock | Named wait stage plus actual goroutine wait stack |
| Concurrent operations stall | Correlated operation/goroutine IDs and bounded shared snapshot, without a dump storm |
| Worker request fails | Matching sender/handler request identities when delivered; explicit cancellation, pipe or remote error |
| Snapshot reaches capacity | `truncated=true`; never imply the omitted threads were checked |
| External cause is unproven | State the observed error/wait and retain `root_cause=undetermined` |

### 5. Good / base / bad cases

Good: `tun_update` remains at `wait_backend_policy_lock`, its goroutine ID matches a `[sync.RWMutex.Lock]` stack, and related send/relogin stacks are available for investigation. Base: a TLS deadline returns with stage, elapsed time, timeout and safe error cause. Bad: claim a confirmed deadlock or Wi-Fi outage solely because one finish log is missing, or silently stop observing when cancellation was requested but never completed.

### 6. Tests required

`log/operation_test.go` captures a real blocked goroutine after cancellation and checks caller correlation, wait states, one snapshot, process rate limiting, idempotent Finish, stopped observer, timeout cause, URL canary removal and stack argument stripping. `engine/diagnostic_test.go` asserts durable TUN wait/cleanup stages with connection identity. `internal/tunipc/diagnostic_test.go` checks matching request IDs/directions and cancellation, with a private payload canary absent from logs. UniVPN mock tests verify the exact safe stage sequence. Run root and affected nested tests/vet/race, then build the Windows executable; keep live network recovery separately recorded.

### 7. Wrong vs correct

Wrong: only log `updating TUN`, and on a later hang conclude that the gateway failed. Correct: enter `wait_tun_operation_lock`, advance to `wait_backend_policy_lock`, then `worker_update_reply`; on a stall capture the matching caller's stack and record only what it establishes. A test must validate emitted evidence, not just search source for logging calls.

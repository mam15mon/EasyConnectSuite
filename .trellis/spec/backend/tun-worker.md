# Independent Windows TUN attachment

## 1. Scope / Trigger

Applies to Windows GUI TUN toggles, reconnect resource publication, process entry, named pipes, DNS and route teardown. GUI owns authentication, gVisor, resolver/fake-IP registry and SOCKS/HTTP listeners. Only VPN.exe --tun-worker owns Wintun/system stack/routes/NRPT. Never restart or elevate the GUI to attach TUN. Worker transport loss may trigger bounded attachment recovery after confirmed cleanup; never compensate by reauthenticating the VPN.

## 2. Signatures

```go
func (a *App) SetProfileTUNMode(id string, enabled bool) error
func (a *App) SetTUNMode(enabled bool) error
func (e *Engine) SetTUN(ctx context.Context, enabled bool) error
func (e *Engine) TUNState() TUNStatus
type TUNController interface { Attach(context.Context, TUNSpec, TUNBackend) (TUNAttachment, error) }
type TUNBackend interface {
    Dial(context.Context, string, string) (net.Conn, error)
    DNS(context.Context, []byte) ([]byte, []string, error)
}
type TUNAttachment interface {
    Update(context.Context, TUNSpec) error
    Close(context.Context) error
    Done() <-chan struct{}
}
```

## 3. Contracts

- Early CLI: VPN.exe --tun-worker <random-local-pipe> <parent-pid>, before Wails/config/tray. runas uses SW_HIDE/NO_UI/NOCLOSEPROCESS. Manifest asInvoker. Worker requires elevation and checks retained parent handle/image and actual pipe server PID; GUI validates launched Worker handle/PID. DACL permits GUI user/Admins/SYSTEM, first instance only, reject remote clients. No credentials, config paths or shell instructions in protocol.
- Control: uint32 big endian length, UTF-8 JSON version=1, closed envelope via DisallowUnknownFields. Message has id/kind/op/ref and explicit spec/network/address/pipe/data/routes/error/text fields. ref has profile/session/attachment/policy. Maximum 1 MiB, 64 pending and active requests. hello precedes prepare/release/attach/update/detach/shutdown; Worker may request dial/dns. ping and cancellation handled by transport. Heartbeat 3 seconds, inbound deadline 15 seconds, writes 5 seconds; dial 15 seconds, DNS 10 seconds, system control 45 seconds.
- Data pipes use GUI-generated random names and verified PID. Up to 1024 flows. TCP length frames <=65535 plus 0xffffffff directional EOF; zero-length UDP remains a datagram. UDP is framed per packet, never io.Copy. Duplicate pipe handles give independent read/write os.File state because Go 1.25 pipe offset updates race on one File. Listener Close cancels and drains pending ConnectNamedPipe before closing its handle.
- Spec passes ref/slot/prefixes/names. Slots 0..63; canonical IPv4 prefixes with no default route. Worker rejects duplicate profile/slot and non-increasing policy versions. GUI validates actual IPv4 TCP/UDP destination/port and attachment ref before Dial/DNS. GUI owns domain ACL bindings, never accepts Worker-supplied authorization.
- loadPolicy saves resourceIPs/resourceDomains before automatic ZJU additions and clones maps. TUN tunView shares resolver/stack but has strict resource dialer. App builds ordinary proxy policy independently of saved TUN intent; Options.TUNEnabled requests startup attachment after proxy setup. Hot toggles never close/replace proxy policy or resolver. DNS returns approved dynamic /32s; Worker commits routes before replying, and never creates /32 where original server prefix covers the address.
- VPNStatus adds tunState=off/attaching/on/detaching/error and tunError. Running tunMode reports actual attachment; disconnected UI uses saved preference. TUN failure preserves VPN State/Running/IP/duration/resources. Connected hot toggles allowed; initial connecting/stopping blocked; reconnecting can detach only. Save intent first, perform UAC/IPC outside App.mu, rollback only target preference on failure. Delete/failed login/cancel/exit release reservations. Last attachment and reservation releases Worker.
- Engine retains runtime TUN intent independently of the attachment. Explicit disable and Stop clear intent; policy refresh alone never reenables a detached TUN. Worker loss allows one immediate recovery attempt with a one-minute minimum interval between automatic attempts, with no timer/retry loop. Later independent losses may recover again. Explicit enable resets this limit. Failed elevation remains an error. Automatic attachment reuses the last approved DNS namespaces without tunnel probes: this path may run inside the VPN relogin callback. The existing post-tunnel recovery callback performs DNS discovery/resource refresh after the transport is installed.
- GUI retains the old Worker process handle and waits up to 45 seconds for exit before notifying attachment loss. Worker cleanup failures propagate as a nonzero exit code. Unconfirmed exit/cleanup blocks relaunch and remains an explicit error; do not overwrite residue with a new attachment.
- GUI is sole engine.log writer. Worker progress/log events are bounded text sent over IPC; Engine records TUN state and errors with profile identity. Worker stderr uses tun-worker-crash.log. Close-to-tray retains connection/heartbeat; real shutdown/EOF/heartbeat loss/parent death cleans ingress. Startup probes only; authorized Worker cleans confirmed own residue.

## 4. Validation & Error Matrix

| Input / event | Required result |
| --- | --- |
| UAC canceled or preference save fails | Original preference and VPN remain; no reservation or hidden attachment |
| Invalid envelope/version/oversized frame | Reject and close transport |
| Wrong PID/image/pipe namespace | No system operation |
| Old session/attachment/policy | Reject backend request; never publish old route or flow |
| Missing resources or default route | TUN error, no unrestricted VPN forwarding |
| Exact route or overlapping DNS ownership | TUN conflict, other ingress preserved |
| Policy update fails | Close only failed ingress before returning error; recovered VPN stays recovered |
| Worker loss | Close TUN flows, preserve VPN/proxy, report interruption; if still desired, attempt one replacement after old Worker exit and successful cleanup, at most once per minute |
| Cleanup fails | Surface TUN error/residue; do not reattach to conceal failure |

## 5. Good / Base / Bad Cases

Good: already connected SOCKS/HTTP sessions continue sending during on/off TUN operations, same resolver/router/listeners. Base: disconnected enable prepares Worker without an adapter, then connect attaches it. Bad: migrating whole Engine into Worker, GUI Quit/relaunch, copying GUI credentials, accepting DNS-provided ZJU proxy ACL as TUN authorization, or attributing TUN failure to VPN login.

## 6. Tests Required

engine/tun_session_test.go checks owned lifecycle/cancellation/resource view; tun_proxy_windows_test.go establishes real SOCKS5 and HTTP CONNECT streams across toggles. tun_windows_test.go checks domain port ACL/dynamic /32/longest prefix; tun_identity_windows_test.go checks ownership and claim teardown. internal/tunipc tests malformed frames, cancellation, TCP EOF, empty UDP, actual named pipe identity/close. Root tun_worker_windows_test.go exercises real pipe TCP/UDP and obsolete policy. tun_mode_windows_test.go checks save-before-UAC, unlocked App and rollback.

Opt-in ECS_TUN_SYSTEM_TEST=accept/cancel/crash runs real elevation and reserved slots 60/61 with documentation addresses. Must use an idle VPN app; never store credentials. Tests independently assert parent privileges, Worker identity, two real route/NRPT sets, TCP/DNS, individual teardown, actual cancellation and parent crash cleanup. Mock/system success is not real gateway compatibility. Run root test/vet/race, nested affected compatibility packages, Node checks/tests, Wails build and 840x660 redacted preview.

## 7. Wrong vs Correct

Wrong: hot toggle overwrites conf.TUNMode, reloads policy/resolver and calls Stop/Setup. Correct: retain policy and use Engine.SetTUN with a strict view and independent attachment lifecycle.

Wrong: one os.File object drives both pipe directions, ignoring race detector failures. Correct: DuplicateHandle to separate directional File state, preserve overlapped deadlines and test concurrent full-duplex traffic with race.

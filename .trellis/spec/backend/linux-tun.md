# Linux GUI and headless TUN

## 1. Scope / Trigger

Applies to Linux GUI mode changes, headless `-tun`, policy replacement, system DNS and cleanup. Ubuntu 24.04/x86-64 is the release target. Reuse the existing three desktop protocol subsets, resource policy and IPC; keep Android's EasyConnect prototype scope separate.

## 2. Signatures

```go
func NewLinuxTUNController() *LinuxTUNController
func (c *LinuxTUNController) Attach(context.Context, TUNSpec, TUNBackend) (TUNAttachment, error)
func (c *LinuxTUNController) Close() error
func StartLinuxTUNIngress(context.Context, TUNSpec, TUNBackend) (*tunIngress, error)
func startWorkerIngress(context.Context, engine.TUNSpec, engine.TUNBackend, func(string, string)) (workerIngress, error)
// workerIngress also exposes UpdateSpec(context.Context, engine.TUNSpec) error.
```

GUI uses `SetProfileTUNMode` and the shared `tunControl`/Worker contracts in [tun-worker.md](tun-worker.md). Headless accepts `-tun` and `-no-proxy`; `-no-proxy` requires `-tun`. Existing defaults stay loopback SOCKS5/HTTP only. Validate root privileges before reading credentials when `-tun` is set. Root is supplied by the caller; the headless TUN controller does not launch an authorization dialog.

## 3. Contracts

- GUI stays unprivileged and launches the same executable with `pkexec --tun-worker` through the existing authenticated Unix-socket protocol. Actual command arguments are `pkexec <executable> --tun-worker <socket> <parent-pid>`. Fresh GUI profiles enable TUN and disable proxies; saved values remain unchanged. Hot toggle saves intent, rolls back on failure and keeps VPN/proxy sessions intact. Linux window close disconnects and exits.
- Each attachment owns an exclusive, non-persistent `ecs%d` interface and routes. Policy replacement closes the old interface before creating a new one. Never delete another owner's matching route or reconfigure an external interface.
- Interface addresses use `198.19.128.(slot*4+1)/30`; DNS endpoints use `198.19.129.(slot*4+2)/32`. Keep DNS outside the interface prefix: sing-tun's system stack rewrites TCP destinations within `Inet4Address` to loopback before calling the handler. Using the peer address as DNS silently diverts TCP DNS while UDP still works. Validate both transports against the real system stack.
- With `IFA_F_NOPREFIXROUTE`, install the system TCP NAT peer `198.19.128.(slot*4+2)/32` on the owned interface via `linuxRoutes.Apply` before creating the stack. This return route is independent of DNS and resource routes; without it TCP SYN replies are unreachable although UDP and split DNS can succeed. Preserve conflict checks and interface-owned cleanup; never restore a broad automatic prefix route to mask the dependency.
- Only resource namespaces require running `systemd-resolved` and root. IP-only TUN has no DNS service dependency. Each attachment opens a private D-Bus connection, captures the service's unique owner and sends `SetLinkDefaultRoute(false)`, `SetLinkDNS`, then route-only `SetLinkDomains` to that owner.
- Cancellation before sending prevents a request. Once sent, retain the exclusive interface until an actual reply with the matching sender and request serial. Cancellation prevents subsequent writes; local timeout, disconnect or name-owner loss does not prove completion. Confirmed service errors and outcome-unknown errors remain distinct and reply bodies never enter diagnostics.
- Cleanup deletes the owned interface and verifies its ifindex is absent via netlink; resolved observes `RTM_DELLINK`. Never call `RevertLink` or edit global `resolv.conf`. An unknown DNS outcome returns `TUNCleanupError`; retain process-local claims and block automatic replacement. This is not a guarantee against a privileged external process forcing interface-index reuse or replacing owned interfaces.
- Headless uses a separate `linuxTUNGeneration` for each policy. Dial/DNS execute outside controller locks, validate generation/cancellation before and after the backend call, and discard stale results. Upstream VPN connection Close runs asynchronously with bounded operation evidence; TCP `CloseWrite` remains available. System interface/route/DNS cleanup is synchronous and checked.
- An already-canceled update returns before retiring the old generation. During creation, stop the temporary cancellation hook before committing the new policy; if cancellation has won, clean the late ingress. Never bind a successfully published attachment to a completed IPC request's context.
- TUN failure is separate from VPN login failure. With proxies enabled, headless reports the TUN error and keeps the proxy session; TUN-only startup failure exits. Stop the engine before closing its controller, and return cleanup failures from the command. No alternative DNS manager, identity or protocol fallback is added.
- Use the existing operation logger for wait stages, DNS requests, system attach/release and asynchronous flow retirement. Preserve profile/session/attachment/policy identity and cancellation evidence, without resource bodies or credentials. GUI Worker diagnostics use `/var/log/EasyConnectSuite/tun-worker-crash.log`; headless writes to its captured output.

## 4. Validation & Error Matrix

| Event | Required behavior |
| --- | --- |
| `-no-proxy` without `-tun` | Reject before reading credentials |
| Non-root headless TUN | Reject before reading credentials |
| Missing resolved with resource namespaces | Explicit TUN error; existing proxies remain |
| Service replies after cancellation | Keep interface until reply, send no next mutation, clean owned state |
| Bus disappears after sending | Unknown outcome/cleanup error; block automatic replacement |
| Old backend Dial/DNS returns after update | Discard reply; close retired flow without reentering its endpoint lock |
| Cleanup fails | Retain failure and reject new attachments; never report clean success |
| TERM/INT or process crash | Owned non-persistent interfaces/routes disappear; resolved clears the deleted link |

## 5. Good / Base / Bad Cases

Good: one open SOCKS stream survives TUN on/off/on with no extra authentication. Base: headless root `-tun -no-proxy` routes only authorized IPv4 TCP/UDP resources. Bad: infer completion from `ctx.Done`, delete the interface and allow a delayed service mutation to target a later interface.

## 6. Tests Required

- `TestLinuxTUNIngressTCPUDP`: real TCP/UDP round trips with DNS outside the interface prefix; assert the exact TCP peer route belongs to that interface and disappears on close.
- `engine/tun_controller_linux_test.go`: actual synchronous Dial-to-Update reentry, blocked upstream Close, late DNS/Dial, canceled late creation, preserved already-canceled state and cleanup failure gate. Run with `-race`.
- `cmd/vpn-headless/main_tun_test.go`: permission/flag checks before input, independent TUN status, retained proxy sessions, no-proxy failure and engine/controller cleanup order.
- Private-bus tests in `engine/dns_resolved_linux_test.go`: sender/serial identity, unique-owner pinning, cancel-and-wait, explicit rejection and outcome-unknown disconnection.
- `build/linux/resolved-isolated-test.sh`: separate network/mount namespaces and private system bus; actual split DNS, UDP/TCP DNS, two attachments, policy replacement, SIGSTOP/cancel/SIGCONT and crash link cleanup. `ECS_WORKER_TEST_BIN` adds `TestLinuxWorkerResolvedCancellation` for attach/update and `TestLinuxWorkerRequestLifetime` for post-reply traffic. These tests must not connect to the host bus or modify host DNS. Actual resolved queries use `.test` or `.internal`; systemd v255 rejects `.invalid` before routing to DNS links, so `.invalid` is unsuitable for this positive-path fixture.
- `build/linux/headless-smoke-test.sh --tun-isolated`: client and synthetic VPN gateway in separate network namespaces so direct kernel routes cannot bypass TUN. Check TCP/half-close, UDP, DNS, proxies, no-proxy and TERM/INT/KILL cleanup. The GUI backend test retains SOCKS/HTTP streams through real Worker TUN toggles and checks authentication count plus cache-disabled resolved queries.
- `build/linux/gui-interaction-test.sh`: actual window saves fresh defaults, then sets proxy mode through accessibility controls, connects, disconnects and closes. Shared mode tests cover persistence failure and authorization refusal rollback. The privileged test harness is not evidence of real polkit-dialog interaction or live-gateway support.

## 7. Wrong vs Correct

Wrong: wait for gVisor `Close` while a synchronous reconnect callback owns its endpoint lock, or retain a request cancellation hook after publishing its replacement policy. Correct: retire the upstream connection asynchronously, and complete cancellation ownership handoff before returning success. Keep separate evidence for compile, mock, isolated system and live gateway results in the release record.

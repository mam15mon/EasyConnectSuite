# Error handling

Return Go errors from engine setup. Preserve causes with %w when adding context, as engine.Start does for underlay, authentication and TUN failures. Wails-facing messages follow the Chinese UI style.

engine.Start uses deferred cleanup on failed startup. Newly opened connections, routes and DNS rules need a clear owner and cleanup path. Stop must cancel reconnect work as well as close the tunnel.

An explicit bad-password response is terminal on its first occurrence: zju-connect/client/easyconnect/password.go exposes ErrLoginRejected, and stack/gvisor/reconnect.go stops that reconnect sequence. Temporary network dial failures are a different class. Do not turn authentication refusal into unlimited network retries.

UniVPN AUTH exposes a numeric status without a verified per-firmware code table. Every nonzero result is an authentication rejection, not proof of a wrong password. protocol.go returns AuthenticationError{Code} unwrapping univpn.ErrLoginRejected, with decimal/hex in the UI string. The adapter wraps both this cause and easyconnect.ErrLoginRejected to retain typed details and stop-on-first-rejection. Do not guess status meanings, discard the code, or print raw AUTH replies/tokens. A successful official-client login with the same credentials requires investigating protocol/policy compatibility rather than insisting the password is wrong. Domain or MAC changes need observed evidence; mock acceptance does not establish real-gateway compatibility.

Windows cleanup removes only this program's routes and NRPT markers. Preserve more-specific routes owned by other VPN clients.

Observed successful official UniVPN 10781.16.0.0729 authentication uses a 14-character uppercase `XXXX-XXXX-XXXX` MAC TLV. The pinned working Python prototype uses SHA256(username + "|" + gateway host), first six bytes, first byte `(b | 2) & 254`, then that format. Keep Go aligned with this reference using independent literal MAC and complete AUTH vectors. The C patch's colon-separated format differs; a mock that merely accepts nonempty fields does not catch it. Observed rejection code 2 has no confirmed semantic mapping; do not claim this format change proves live login success.

Do not add fallback methods, alternate protocols, insecure TLS behavior or speculative exception handling without a request. Unsupported authentication methods should return an explicit error.

## VPN target connection failures

1. Scope: startup `managedClient.Setup()` errors, including TopSAP control/data TCP connections. The operation stage `authentication` covers setup and does not prove authentication rejection.
2. Signatures: preserve `Start(Options) (*Engine, error)` and the original error chain. No new status fields or protocol retries.
3. Contract: when `errors.As` finds `*net.OpError` with `Op == "dial"` and the cause is not `context.Canceled`, return the local prefix `VPN 目标连接失败，请检查服务器地址、端口及网络连通性: ` with `%w`. Local error details retain the actual target and underlying error. This establishes a failed connection attempt, not a proven incorrect port, firewall fault or wrong password.
4. Error matrix: TCP dial failure gets the target explanation; authentication rejection, certificate verification failure and cancellation retain their existing classification. Preserve startup operation identity, stage, timing, cancellation/timeout, error type/cause and cleanup observation.
5. Cases: a refused local TCP connection is a target failure; a nonzero authentication result remains a login refusal; successful TCP followed by a certificate error must not become a target-port failure.
6. Tests: `engine/startup_error_test.go` exercises real local TCP refusal through direct UniVPN and HTTP EasyConnect setup, checks endpoint/cause and persistent operation evidence, and distinguishes authentication, certificate and cancellation failures. `frontend/src/connection-feedback.test.mjs` checks the fixed diagnostic explanation without endpoints/credentials and rejects stale/non-target classifications.
7. Wrong vs correct: do not infer a password problem from `stage=authentication` or copy arbitrary error text into diagnostics. `buildDiagnostics` exports only the fixed target explanation when an error status starts with the local target-failure prefix; preserve the existing address/credential exclusion.

## EasyConnect session recovery

`easyconnect.Client.Setup() error` may reuse `Options.SessionID` only on its first attempt. Every subsequent setup attempt clears the old TWFID, tunnel token and line list, closes the retained IP connection, and executes the existing full authentication flow before requesting a new token and IP. This also applies after a failed password flow, because its intermediate TWFID is not an authorized session. A nonempty local ID alone does not establish server-side validity after `ErrSangforShutdown`.

`setupMu` serializes setup, HTTP session keepalive and send/receive handshakes so they cannot use a partially updated session. `Close()` cancels the lifecycle context before acquiring this mutex, allowing active network setup and queued operations to stop. Preserve the existing one-time keepalive loops and typed `ErrLoginRejected` behavior; the gVisor reconnect controller still stops on the first explicit rejection and retains its existing network retry policy.

`requestConfig()` rejects non-200 HTTP responses and closes the response body on all paths. `parseLineListFromConfig()` returns an error for missing `Conf`/`Mline`, missing or disabled `enable`, and missing/empty `list`; never chain `SelectElement` through an unchecked parent. Optional multiline failures retain the established logging/continuation behavior. Normal authentication/reconnect logs must not contain TWFID, CSRF values, TLS session IDs or tunnel tokens.

Regression coverage lives in `client/easyconnect/relogin_test.go`, `relogin_lifecycle_test.go` and `parse_config_test.go`: a local TLS gateway terminates the old session with SHUTDOWN, requires fresh authentication, checks new cookies/token, assigns an IP and accepts both rebuilt tunnel channels; rejection retries cannot reuse a partial login; a supplied initial session remains compatible; cancellation stops authentication/keepalive/handshakes; invalid config returns errors without panics. Compilation and these mock results do not establish live gateway or system TUN recovery. No real profile, gateway address, credential or private log belongs in these fixtures or specs.


EasyConnect implements `client.IPUpdateHandlerSetter`. Setup validates the allocated IPv4 address and calls the registered stack update before publishing the new client IP/retained connection; a failed callback aborts Setup and closes the new connection. IP reads and handler registration share setupMu. A mock gateway changes addresses between logins; gVisor regression traffic checks actual outgoing source IP and UDP echo after an address update.

Authentication requires nonempty TwfID, RSA_ENCRYPT_KEY and RndImg before indexing/using fields, validates RSA exponent/modulus and captcha state, and rejects missing TOTP session IDs. Preserve the existing absent-exponent compatibility default. Token construction validates TLS session-ID and TWFID lengths; IP replies require at least eight bytes (including fragmented reads). Resource parent elements and required type/proto attributes are checked before dereferencing; empty DNS cannot retain a previous server. Existing optional multiline/resource continuation and supported auth success markers remain unchanged.

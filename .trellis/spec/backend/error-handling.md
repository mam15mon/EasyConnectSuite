# Error handling

Return Go errors from engine setup. Preserve causes with %w when adding context, as engine.Start does for underlay, authentication and TUN failures. Wails-facing messages follow the Chinese UI style.

engine.Start uses deferred cleanup on failed startup. Newly opened connections, routes and DNS rules need a clear owner and cleanup path. Stop must cancel reconnect work as well as close the tunnel.

An explicit bad-password response is terminal on its first occurrence: zju-connect/client/easyconnect/password.go exposes ErrLoginRejected, and stack/gvisor/reconnect.go stops that reconnect sequence. Temporary network dial failures are a different class. Do not turn authentication refusal into unlimited network retries.

UniVPN AUTH exposes a numeric status without a verified per-firmware code table. Every nonzero result is an authentication rejection, not proof of a wrong password. protocol.go returns AuthenticationError{Code} unwrapping univpn.ErrLoginRejected, with decimal/hex in the UI string. The adapter wraps both this cause and easyconnect.ErrLoginRejected to retain typed details and stop-on-first-rejection. Do not guess status meanings, discard the code, or print raw AUTH replies/tokens. A successful official-client login with the same credentials requires investigating protocol/policy compatibility rather than insisting the password is wrong. Domain or MAC changes need observed evidence; mock acceptance does not establish real-gateway compatibility.

Windows cleanup removes only this program's routes and NRPT markers. Preserve more-specific routes owned by other VPN clients.

Do not add fallback methods, alternate protocols, insecure TLS behavior or speculative exception handling without a request. Unsupported authentication methods should return an explicit error.

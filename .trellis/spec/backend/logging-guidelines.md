# Logging

The zju-connect engine uses its log package and the standard Go logger. engine.startLogCapture registers with the process-wide engineLogs hub, which owns shared rotating files and sends lines to OnLog callbacks. Removing one engine must not restore logger output or close files still used by another. Never call standard log.SetOutput while holding the hub lock. GUI connections share APPDATA/EasyConnectVPN/logs/engine.log; it is a combined process log.

engine.rotatingWriter caps engine.log at 1 MiB and retains one .old generation. Reuse this pattern for engine integration rather than adding a logging service.

Log connection phases, protocol selection, reconnect decisions and cleanup failures. UI status travels through vpn-status with profileId/protocol; vpn-resources carries {profileId, entries}; vpn-connections carries all profile statuses. Callbacks must check connection identity and cancellation before changing state, so stale callbacks cannot overwrite a restarted profile or another selected profile.

Never print passwords, TOTP secrets, tokens, full authentication payloads or decrypted packets. Native command lines must not contain VPN passwords. Research and tests use local mock gateways or redacted metadata.

UniVPN setup rejection must persist `UniVPN authentication rejected: code=N (0xXXXXXXXX)` through the existing engine logger before returning the terminal error. Startup failure messages returned to the UI are not automatically written into engine.log. Log only the typed numeric code; validate persistence with the shared log capture and reject credential/token output in the test.

TopSAP must persist setup failures before returning to the GUI, including protocol stage and wrapped cause. Standard-logger messages carry a process-local session number so concurrent sessions are distinguishable. Record gateway/certificate policy, auth/config HTTP endpoint/status/timing, numeric Ret or missing Ret, resource presence/count, TLS/ACK progress, first IPv4 send/receive size, aggregate byte counts on close and logout results. Do not log request forms, username/password, S_CODE, cookies, session token or packet bodies. Test with the existing engineLogs capture, including Ret=0 with missing resources and terminal rejection; an error visible only in the UI does not satisfy this contract.

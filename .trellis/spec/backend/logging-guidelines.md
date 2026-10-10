# Logging

The zju-connect engine uses its log package and the standard Go logger. engine.startLogCapture registers with the process-wide engineLogs hub, which owns shared rotating files and sends lines to OnLog callbacks. Removing one engine must not restore logger output or close files still used by another. Never call standard log.SetOutput while holding the hub lock. GUI connections share APPDATA/EasyConnectVPN/logs/engine.log; it is a combined process log.

engine.rotatingWriter caps engine.log at 1 MiB and retains one .old generation. Reuse this pattern for engine integration rather than adding a logging service.

Log connection phases, protocol selection, reconnect decisions and cleanup failures. UI status travels through vpn-status with profileId/protocol; vpn-resources carries {profileId, entries}; vpn-connections carries all profile statuses. Callbacks must check connection identity and cancellation before changing state, so stale callbacks cannot overwrite a restarted profile or another selected profile.

Never print passwords, TOTP secrets, tokens, full authentication payloads or decrypted packets. Native command lines must not contain VPN passwords. Research and tests use local mock gateways or redacted metadata.

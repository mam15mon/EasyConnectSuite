# Logging

The zju-connect engine uses its log package and the standard Go logger. engine.startLogCapture sends lines to an OnLog callback and optional persistent LogFile. app.StartVPN selects APPDATA/EasyConnectVPN/logs/engine.log.

engine.rotatingWriter caps engine.log at 1 MiB and retains one .old generation. Reuse this pattern for engine integration rather than adding a logging service.

Log connection phases, protocol selection, reconnect decisions and cleanup failures. UI status travels through vpn-status; resource updates through vpn-resources.

Never print passwords, TOTP secrets, tokens, full authentication payloads or decrypted packets. Native command lines must not contain VPN passwords. Research and tests use local mock gateways or redacted metadata.

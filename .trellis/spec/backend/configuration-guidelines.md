# Configuration

There is no database or ORM. app.go stores AppConfig and Profile as JSON in vpn_profiles.json. NewApp selects an existing file beside the executable or the existing APPDATA/EasyConnectVPN location. Do not add another storage mechanism during protocol work.

JSON field names are frontend contracts: Profile.TUNMode uses json:"tunMode" and ResourceEntry.Target uses json:"target". Keep generated frontend/wailsjs/go bindings aligned with exported type changes.

Preserve saved EasyConnect profiles when adding a protocol selector. Document any default needed solely to read pre-existing profile files. Never silently substitute protocols after an error.

Profile.protocol defaults to easyconnect when absent; univpn uses Profile.domain for its optional authentication domain. socksPort/httpPort are numeric per-profile ports. Old profiles without ports receive 1080+2*i/1081+2*i; new profiles choose an unused configured pair. Bind errors remain explicit. SaveProfile returns an error and rejects editing a running/stopping connection; DeleteProfile applies the same guard.

connections.go owns sessions by profile ID. Selecting a profile changes displayed status/resources only. Wails StartProfileVPN/StopProfileVPN use explicit IDs; StopAllVPN and shutdown stop every session. Never replace an explicit missing ID with the selected profile. Each engine gets its own context, cleanup callbacks, listeners and TUN slot. GUI-owned EasyConnect/gVisor paths must not execute process-global CLI terminal hooks.

Current profiles include password and TOTP fields in saved JSON; this is not an OS credential vault. Do not copy real profiles, credentials or APPDATA logs into tasks or Git. The root .gitignore excludes vpn_profiles.json, captures and logs.

SetProxyEnabled and SetTUNEnabled illustrate restoring the previous in-memory value when configuration saving fails.

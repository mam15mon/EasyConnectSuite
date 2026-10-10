# Configuration

There is no database or ORM. app.go stores AppConfig and Profile as JSON in vpn_profiles.json. NewApp selects an existing file beside the executable or the existing APPDATA/EasyConnectVPN location. Do not add another storage mechanism during protocol work.

JSON field names are frontend contracts: Profile.TUNMode uses json:"tunMode" and ResourceEntry.Target uses json:"target". Keep generated frontend/wailsjs/go bindings aligned with exported type changes.

Preserve saved EasyConnect profiles when adding a protocol selector. Document any default needed solely to read pre-existing profile files. Never silently substitute protocols after an error.

Current profiles include password and TOTP fields in saved JSON; this is not an OS credential vault. Do not copy real profiles, credentials or APPDATA logs into tasks or Git. The root .gitignore excludes vpn_profiles.json, captures and logs.

SetProxyEnabled and SetTUNEnabled illustrate restoring the previous in-memory value when configuration saving fails.

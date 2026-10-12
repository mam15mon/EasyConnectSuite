# TopSAP protocol contract

## Scope / trigger

Apply when changing `engine/topsec`, its managedClient adapter, or profile.protocol=topsec. Implement only the observed standard password/uncompressed IPv4 mode. Credentials, capture data, device IDs and native binaries stay outside Git. Use original Go protocol code, not official-client subprocesses.

## Signatures

`topsec.Dial(context.Context, Options) (*Session, error)` owns HTTP/cookies, data TLS and cancellation. `Options` contains Server, Username, Password, TLSConfig and underlay DialContext. Session implements complete-packet Read/Write/Close plus IP/Routes/DNSServers. `newTopSAPClient(ctx, conf, insecureSkipVerify, underlay)` adapts existing managedClient and IP update hooks. No exported Wails model field is added; reuse Profile.protocol and insecureSkipVerify.

## Request and response contracts

POST auth_param, then one user_pswd form: UTF-8 Base64 U/P, HASH empty for fresh resources, S_CODE device identity. get_config_info uses HASH empty and TYPE=all; require res_list, never infer a default route. TYPE=phone requests contact information and returns Ret/phone/mail, not the complete resource configuration. Native libTopSecVPN.dll constructs HASH + &TYPE=all at 0x1015172e..0x10151752 and uses the same get_config_info request line at 0x101517dc. A capture from a cached session containing TYPE=phone must not become the resource request contract. logout has an empty body. auth_param uses unquoted identifier keys: quote object keys only, preserve strings/arrays and reject executable text; responses have a 4 MiB bound. Missing Ret is a protocol error, nonzero Ret retains its numeric authentication refusal.

user_param.change_pswd=on is the portal permission to modify a password, not a mandatory password-change condition. The successful official login returns it with Ret=0; installed sv_portal.js stores it in userSetting[0] and checks it when the user invokes changePassword. Do not reject a successful login based on this flag. The mock successful response must include it so full session/TCP/UDP and failure-cleanup tests cover this regression. Actual authentication refusals still follow Ret; do not invent another force-change field or retry variant.

S_CODE's Windows NIC path enumerates GetAdaptersInfo, queries registry PnPInstanceId, and concatenates each accepted PCI MAC as six `%03X` groups separated by hyphens, then Base64(MD5(text)). Preserve adapter order and the PCI/six-byte filter. Read each registry string independently with GetStringValue: carrying a previous adapter's byte count can skip a longer PCI identifier after a short virtual-adapter identifier. This intentionally departs from the previously observed native bounded-read behavior. The digest remains unchanged when the accepted adapter set is unchanged; previously skipped PCI adapters can change identity and require live gateway verification. No alternate hardware identity or installed TopSAP service/cache dependency.

Device enumeration diagnostics use the shared logger with PID/profile, adapter ordinal/type, PNP presence, PCI status, MAC length, selected status, registry error stage and final counts. Never log raw MAC, PNP instance IDs or S_CODE. Regression tests cover a short virtual PNP followed by a long PCI PNP, multiple accepted adapters in order, registry errors, no eligible adapter and identity redaction.

TLS >=1.2 verifies by default. Per-profile manual certificate opt-in follows the user's earlier ignore-certificate request; retain authentication/data SPKI equality. CONNECT is the verified 130-byte constant. Setup command 1 body is 328 bytes, byte 3=4, token at 72 (<256 bytes). ACK is (0,2,0,1); send command 7 once with eight zero bytes. Client data header (0,15,len,1), server (0xAABB,15,len,0xEEFF0800); payload is exactly one IPv4 packet. Do not invent heartbeat timers or unknown frame semantics.

## Validation and error matrix

| Input / condition | Required behavior |
| --- | --- |
| Missing Ret / damaged frame / invalid mask | Explicit protocol failure |
| Ret nonzero or HTTP 401/403 | AuthenticationError{Code}, adapter wraps easyconnect.ErrLoginRejected, first refusal terminal |
| Missing res_list | Fail startup; revoke authenticated session |
| Nonstandard mode / security rules / unverified DNS schema | Explicit unsupported error, no variant retry |
| Untrusted certificate, opt-in false | Fail before sending credentials |
| Different control/data key even with opt-in | Reject and revoke session |
| Cancel or Close | Interrupt blocked I/O, bounded logout, own transport cleanup only |

## Good / base / bad cases

Good: `na`/`ip`, 198.51.100.19 with 255.255.255.0 yields 198.51.100.0/24. Base: empty DNS/resource-domain configuration creates no NRPT zones. Bad: missing resources cannot become unrestricted routing; a new Ret must not be labeled wrong password without a verified meaning.

## Tests required

Independent literal device/auth/header vectors; two MACs; preservation of quoted strings and array booleans; missing Ret/resources; corrupt/truncated frames; first rejection; cancellation; authenticated setup-failure logout; certificate defaults, explicit opt-in and key mismatch. Test actual gVisor TCP/UDP round trips, three protocol session isolation and existing route/DNS ownership regression. Local private overlay tests may compare current native identity and actual schemas without committing raw evidence. Record mock, Windows build and live gateway success separately.

Successful mock configuration requests must assert TYPE=all. TYPE=phone must return a contacts-only response to reproduce the old bug. Assert persistent session-numbered stage/HTTP/Ret/resource-presence logs on success and resource failure, logout after authenticated failure, and absence of credentials/tokens/forms/packets. Core logging uses the existing standard logger captured by engineLogs; no separate files or sinks.

## Wrong vs correct

Wrong: infer byte field semantics or hardware choice from one successful capture, then test against an encoder reusing that assumption. Correct: corroborate native call boundaries, add independent vectors and compare current native/Go outputs with only match status exposed. A private local overlay caught last-MAC and unbounded-registry differences; leave those regression contracts explicit. The product does not consult the native service used by that research check.

## Complete configuration owns the active virtual address

TYPE=all can assign a different net_param.vip_v4 from user_pswd. The native na_client.exe ProcessLoginServerAckPacket_NEW (0x52d820, VIP parse at 0x52e7af..0x52e820) configures the network stack from the complete config. Use that response's IPv4 and DNS before creating the data stack; reject missing, unspecified or non-IPv4 current addresses without reverting to the login address. Synthetic gateways must use differing login/config VIP and reject obsolete-source packets. Actual fix4 restored receive traffic and user-confirmed SSH access on the observed gateway.

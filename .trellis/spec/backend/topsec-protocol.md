# TopSAP protocol contract

## Scope / trigger

Apply when changing `engine/topsec`, its managedClient adapter, or profile.protocol=topsec. Implement only the observed standard password/uncompressed IPv4 mode. Credentials, capture data, device IDs and native binaries stay outside Git. Use original Go protocol code, not official-client subprocesses.

## Signatures

`topsec.Dial(context.Context, Options) (*Session, error)` owns HTTP/cookies, data TLS and cancellation. `Options` contains Server, Username, Password, TLSConfig and underlay DialContext. Session implements complete-packet Read/Write/Close plus IP/Routes/DNSServers. `newTopSAPClient(ctx, conf, insecureSkipVerify, underlay)` adapts existing managedClient and IP update hooks. No exported Wails model field is added; reuse Profile.protocol and insecureSkipVerify.

## Request and response contracts

POST auth_param, then one user_pswd form: UTF-8 Base64 U/P, HASH empty for fresh resources, S_CODE device identity. get_config_info uses HASH empty and TYPE=phone; require res_list, never infer a default route. logout has an empty body. auth_param uses unquoted identifier keys: quote object keys only, preserve strings/arrays and reject executable text; responses have a 4 MiB bound. Missing Ret is a protocol error, nonzero Ret retains its numeric authentication refusal.

user_param.change_pswd=on is the portal permission to modify a password, not a mandatory password-change condition. The successful official login returns it with Ret=0; installed sv_portal.js stores it in userSetting[0] and checks it when the user invokes changePassword. Do not reject a successful login based on this flag. The mock successful response must include it so full session/TCP/UDP and failure-cleanup tests cover this regression. Actual authentication refusals still follow Ret; do not invent another force-change field or retry variant.

S_CODE's verified Windows NIC path enumerates GetAdaptersInfo, queries registry PnPInstanceId, and concatenates each accepted PCI MAC as six `%03X` groups separated by hyphens, then Base64(MD5(text)). Native RegQueryValueEx carries its byte count (initially 256) across ALL queried adapters, including non-PCI and ERROR_MORE_DATA. GetStringValue's resizing would change which adapters qualify. Mirror bounded Key.GetValue reads and carry the returned count; do not select only the first/last MAC or include adapters that native skips. No installed TopSAP service/cache dependency in the product.

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

## Wrong vs correct

Wrong: infer byte field semantics or hardware choice from one successful capture, then test against an encoder reusing that assumption. Correct: corroborate native call boundaries, add independent vectors and compare current native/Go outputs with only match status exposed. A private local overlay caught last-MAC and unbounded-registry differences; leave those regression contracts explicit. The product does not consult the native service used by that research check.

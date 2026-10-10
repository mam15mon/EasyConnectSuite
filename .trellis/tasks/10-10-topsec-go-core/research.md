# TopSAP evidence (2026-10-10)

## Public repositories inspected

| Source and pinned revision | Verified boundary |
| --- | --- |
| [Sajor-X/TopSAP_Client](https://github.com/Sajor-X/TopSAP_Client/tree/91c9d2ed5c2e6968c4280c99a5a83e219d6a9cd3), Apache-2.0 | top.py calls localhost:7443 API for login_by_pwd, get_gid and query_statistics, plus an external OCR service; it does not implement the remote VPN transport. Its ten-login retry behavior is unsuitable for this project. |
| [inclyc/containerized-topsap](https://github.com/inclyc/containerized-topsap/tree/2cb38126fcf54b87ac1cd634da8bc2a7125a3002), no detected repository license | Containerfile extracts official sv_websrv, libvpn_client.so and topvpn binaries. Wrapper scripts do not supply a portable protocol core. |
| [libra146/docker-topsap](https://github.com/libra146/docker-topsap), AGPL-3.0 repository | README and start.sh run official TopSAP binaries with expect and danted. Not a Go protocol implementation; none of its code copied. |
| [cankevinlaurent/vpn_script](https://github.com/cankevinlaurent/vpn_script/tree/3e52ec89911c2a4e732d9b469b339cff9b7d321a), GPL-3.0 | Python handles device administration CGI and user management. It is not an IP tunnel client and is unsuitable as a data-channel reference. |

These searches do not prove no independent implementation exists. They establish only that the inspected candidates lack one.

## Installed official program: read-only observations

Uninstall metadata identifies TopSAP 3.6.6.18.64.1 under NGVONE/Client. Electron UI uses TopSAP.node and native libTopSecVPN.dll; the DLL inspected is PE i386 with SendRequestInfo exported. na_client.exe and sv_service.exe are present. No vendor executable, DLL or JS was modified, loaded into a new process, or copied into this repository.

Bundled sv_init.js sends login_by_pwd JSON through Electron IPC, with gateway, username/password representation, optional gid, vpn_version, auth/data protocol and ports, and certificate type. Those parameters are UI-to-native API contracts, not proof of remote wire encoding. The inspected Python also targets the local API, so directly porting its requests would still depend on the official core.

Static native string markers identify remote paths such as /vone/login/auth_param, /vone/login/user_pswd, /vone/login/gid, /vone/portal/res_list and /vone/portal/client_get_res_list. These are research leads, not sufficient descriptions of request fields, response framing, cipher negotiation or data transport. No real server, user, device ID, password or token is recorded here.

## Capture readiness

tshark and dumpcap are already installed and on PATH. dumpcap -D successfully enumerates WLAN, loopback and the vendor SV-Connection interface; built-in PktMon is available. This agent shell is not elevated. Interface discovery is verified; capture rights and packet collection have not yet been tested. PCAP/PCAPNG are already excluded by .gitignore. No capture or real login was started in this research pass.

## Next evidence collection

1. Confirm graphic CAPTCHA behavior; inspect only relevant sanitized diagnostic metadata for authentication mode, TLS and tunnel capabilities.
2. Take one bounded, target-filtered official-client connect/disconnect sample while the user performs normal authentication. Avoid unrelated device traffic and keep original capture local.
3. Determine whether documented TLS session-key export or useful application diagnostics provide the missing plaintext. Do not assume SSLKEYLOGFILE works with the native SDK, and do not globally disable certificate verification to acquire a sample.
4. Establish remote authentication sequence, resource replies, data framing, heartbeat and teardown with independent sanitized vectors before implementing engine/topsec.
5. Implement and test against those vectors, then integrate existing lifecycle/TUN/proxy surfaces and validate authorized live behavior.

An encrypted packet trace alone does not establish a complete protocol. If no plaintext diagnostics or packet semantics are available, report the exact missing evidence instead of inventing a transport or shipping a nonworking protocol selector.

## Follow-up: authentication and capture preflight

User confirms no graphic CAPTCHA; password authentication needs no SMS or browser step. Do not add CAPTCHA UI or OCR to the initial scope.

The unelevated dumpcap WLAN preflight uses filter `less 0`, duration one second, and a temporary file outside the repository. It exits successfully with zero packets, confirming capture permission without collecting any network traffic; the temporary file was removed. Thus capture need not elevate Codex or change the official installation.

The initial process filter omitted TopCOM.exe, so its absence of matched sockets was insufficient evidence that the official client was disconnected. User subsequently supplied a connected screenshot; endpoint ownership and the active vendor adapter were verified. Future connection checks must include TopCOM as well as na_client and the Electron UI.

Compiled na_client string markers include `before add private header, enc data len:%d` and `private header len:%d, send msg len:%d`; this establishes a private framing lead but not the header structure, command IDs or algorithm. Native strings contain OpenSSL symbols. The SDK's presence of `nss_keylog_int` does not prove application-level key export is enabled. [OpenSSL callback documentation](https://docs.openssl.org/1.1.1/man3/SSL_CTX_set_keylog_callback/) describes an application-installed callback; setting an environment variable alone is not established for this SDK. No key material or process-memory dump was collected.

Next concrete dependency: an active user-created official connection, then a bounded capture of only its gateway. Research remains incomplete until authentication/resource replies and private data framing are independently verified.

## Successful official reconnect capture and diagnostic evidence

A bounded capture was started before the user manually disconnected and logged in again. The valid PCAPNG has 261 packets over approximately 64 seconds, with target-filtered Mihomo and WLAN interfaces; duplicate interface observations must be deduplicated when counting logical connections. Authentication connections negotiated TLS 1.0 with ECDHE-ECDSA-AES128-SHA; the persistent data connection negotiated TLS 1.2 with ECDHE-ECDSA-AES128-GCM-SHA256. These are observed properties of this sample, not a claim about every supported gateway mode. One-byte TCP probes must not be interpreted as application heartbeat commands.

The native diagnostic log already contains plaintext authentication HTTP. The successful request uses /vone/login/user_pswd with form fields U, P, HASH and S_CODE. The response reports HTTP 200 and X-sv-ret 0 and supplies user/net parameters. An auth_param response is JavaScript-style object text with unquoted keys; it must not be treated as strict JSON without verifying parsing. Configured auth/data ports in the official request differ from ports in the capabilities response. HASH is associated with a native sha256Str diagnostic, but its input has not been established. Stop speculative digest combinations; derive the algorithm from evidence. Keep diagnostic contents, identifiers, credentials, cookies, raw replies and packet captures outside Git.

ECDHE handshake capture and the public server certificate do not provide the ephemeral secrets needed for passive TLS decryption. No key-export callback has been verified in the native SDK. The data-channel private frame structure remains unverified.

## Local TLS interception experiment

User explicitly requests client HTTPS -> MITM proxy with generated certificate -> server HTTPS. The official login UI has proxy types 0 (none), 1 (SOCKS), and 2 (HTTPS), with address, port, optional user/password/domain. The proxy is a research tool; it is not a replacement protocol core and is not added to the shipped EXE.

Installed mitmproxy 12.2.3 in an isolated local virtual environment outside the repository. Initial cryptography 48/OpenSSL 4 configuration failed during TLS 1.0 setup. Fixed this experimental environment to cryptography 46.0.5, pyOpenSSL 25.3.0 and service-identity 24.2.0; pip check passes, and mitmdump reports OpenSSL 3.5.5.

The proxy listens only on loopback port 18080, intercepts only the selected gateway and port, uses generic TCP interception for that gateway, and shuts down after 20 minutes. Gateway certificate verification is disabled only in this experimental proxy's upstream connection; no system proxy, Windows trust store or shipped client policy was changed. Experimental CA, private key, TLS secrets and flow files remain local outside Git. Note: allow_hosts matches host:port, while tcp_hosts matches hostname only; this was verified from the installed next_layer.py, corrected and retested.

Two credential-free POST probes were sent through the proxy, one using TLS 1.0 and the other TLS 1.2 with the experimental CA explicitly trusted only in those Python contexts. Both established TLS and returned HTTP 404. Offline FlowReader inspection confirmed two TCP flows with plaintext request and response bytes and upstream TLS 1.2. This verifies TLS termination and plaintext recording, not TopSAP login or the meaning of the 404 response. The user has been asked to configure the official proxy and reconnect once. Its acceptance of the generated certificate and whether both authentication/data channels use the proxy remain pending actual evidence.

## Clash TUN routing into MITM

User requests interception through the already-running Clash instead of changing TopSAP's proxy settings. Verified the running Mihomo v1.19.32 production controller through its observed Windows named pipe, with rule mode and TUN enabled. Active controller metadata shows na_client.exe traffic to the gateway currently using DIRECT. The controller pipe in generated YAML differs from the live production pipe; discovery is required, and API credentials must never be printed.

Applied an in-memory experimental HTTP proxy node pointing to loopback 18080 and two rules before the original 25 rules: target gateway TCP/port to MITM, and the MITM Python process to the same gateway/port via DIRECT to prevent recursion. Original subscriptions and persistent profile files were not edited. The local helper preserves Selector choices during reload and keeps an original configuration snapshot outside Git. Verified the runtime has the experimental node and 27 rules.

A credential-free curl request, with no explicit HTTP proxy option, traversed Clash TUN and reached MITM. Offline flow inspection confirms plaintext POST /vone/login/auth_param and a 2373-byte server stream; curl received HTTP 200. Curl's first attempt to validate the intercepted certificate against the experimental CA failed with verification result 53 (unsupported or invalid name syntax). A second, credential-free probe disabled verification only for that invocation. This proves routing and interception; it does not prove the official native client's certificate acceptance. No Windows CA installation or native client modification was made.

Restarted the bounded MITM process with a shutdown hook that removes the experimental rules/node via controller reload from the current generated base configuration, only if the marker rule is still present. A local restore command is also available. The original physical config remains unchanged. Started a new five-minute capture with verified interface GUIDs (loopback, Mihomo and WLAN); use GUIDs rather than assuming numeric capture indexes stay fixed. User is asked only to manually reconnect the official client, with no TopSAP proxy setting required. Official authentication and data-channel interception results remain pending.

## Official MITM authentication confirmed; data sample not yet sealed

The first reconnect recorded logout only: Clash regenerated its base config during the attempt and discarded the runtime-only experimental node/rules. This is an observed ownership issue, not a certificate failure. Moved the temporary insertion into the active subscription's otherwise-empty extension script, with an expiry, and used Clash's observed Reactivate Subscription button. Verified the generated config and running controller both retained 27 rules. Cleanup subsequently restored the extension byte-for-byte and the running rule count to 25.

The next official reconnect was intercepted successfully. Saved plaintext auth_param, user_pswd and get_config_info HTTP, all status 200; login reports X-sv-ret 0. Native TLS 1.0 accepted the generated certificate in this sample without installing a Windows CA. Controller confirms na_client.exe -> MITM and python.exe -> DIRECT for the established TCP data connection. This does not establish certificate behavior for other SDKs/gateways.

The successful login form has U/P/HASH/S_CODE. Its strict JSON reply has user_param, net_param, ErrMsg and Ret; no resource array in this response. acl_permit is an integer, not a resource list. Compression is off and alg is standard. DNS/VIP strings exist in net_param, but must not be confused with resource-scoped routing authorization.

After the user reported access/disconnect completion, the data flow was still open at the controller check. The operator stopped MITM before its standard flow writer sealed that active TCP flow; saved .mitm contains four completed authentication/control flows only. The loopback capture started after the data TLS handshake, so it is not established as a standalone decryptable data sample despite available TLS key logs. Do not claim a verified private tunnel frame from these files.

Corrected capture lifecycle: append each tcp_message's raw plaintext, direction and timestamp immediately to a local JSONL journal; add a local stop flag that calls master.shutdown gracefully. Credential-free curl test confirms request/response journal persistence and HTTP 200. Graceful stop exits 0, removes temporary runtime rules, and reports cleanup completion. This replaces reliance on closing TCP flows or sending Ctrl-C through a piped tool session.

A fresh collection was prepared, but the user stopped Computer Use with Escape before configuration reactivation. No further Windows UI input was issued. Stopped capture and proxy, removed the pending temporary extension block, and verified original script/runtime rules restored. Only completed authentication evidence is confirmed; a complete live-recorded data sample is still needed for the native Go tunnel implementation.

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

At the follow-up check there are no official-client remote TCP connections; the only observed vendor UDP endpoint is a local service listener. No target is guessed from older connection snapshots or unrelated VPN profiles. User was asked to connect the official TopSAP and keep it connected so gateway targeting can be derived from its process-owned sockets.

Compiled na_client string markers include `before add private header, enc data len:%d` and `private header len:%d, send msg len:%d`; this establishes a private framing lead but not the header structure, command IDs or algorithm. Native strings contain OpenSSL symbols. The SDK's presence of `nss_keylog_int` does not prove application-level key export is enabled. [OpenSSL callback documentation](https://docs.openssl.org/1.1.1/man3/SSL_CTX_set_keylog_callback/) describes an application-installed callback; setting an environment variable alone is not established for this SDK. No key material or process-memory dump was collected.

Next concrete dependency: an active user-created official connection, then a bounded capture of only its gateway. Research remains incomplete until authentication/resource replies and private data framing are independently verified.

# TopSAP protocol implementation

This package is an original Go implementation based on locally observed protocol
traffic and examination of the installed TopSAP 3.6.6.18.64.1 Windows client,
macOS V3.5.0.73.3, and Linux 3.6.3.32.2.22375 amd64.
No official binary, DLL, source code, credentials, device identifier, session
token, capture, or private log is redistributed. It does not invoke TopSAP.

Observed scope: standard TLS, username/password (UTF-8 Base64), platform-specific
device identity, uncompressed IPv4 L3 framing, and `na`/`ip` resource prefixes.
S_CODE is a device identifier, not an emulated security check. Browser, captcha,
two-factor challenges, national cryptography, client certificates, endpoint
security policy, other resource/DNS schemas, and unobserved control commands
are explicitly unsupported. No guessed protocol retries or default routes.

Capture establishes official-client IPv4 TCP traffic; TCP/UDP synthetic tests
establish framing only. A Go EXE requires separate live gateway validation.
Linux uses the observed disk/DMI order and excludes the native random
registration-file branch. The pinned native comparison uses only synthetic
inputs; the product neither loads nor redistributes that library. Linux/macOS
live gateway acceptance remains separately unverified. Reproducible hashes,
addresses and test boundaries are documented in ../../docs/topsec-go.md.

Prior research references (no implementation copied):
https://github.com/Sajor/TopSAP_Client (official-client automation);
https://github.com/inclyc/TopSAP (official-client container tooling).

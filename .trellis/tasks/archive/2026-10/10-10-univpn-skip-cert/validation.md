# Validation — Windows, 2026-10-10

- `go test -race ./... -count=1 -timeout=75s` and `go vet ./...` passed, including previous multi-session TCP/UDP tests.
- New mock TLS regressions verify that the production UniVPN adapter rejects an untrusted certificate by default, connects with explicit skip, and obtains the assigned IP. Core transport still rejects mismatched control/data keys with skip enabled.
- Configuration tests verify old profiles default false, JSON round-trip retains opt-in, and the connection manager sends only the selected profile's option to its engine.
- Wails production build passed, including Vite frontend build and binding generation: `wails build -m -nosyncgomod -trimpath -o EasyConnectGUI-multi-vpn-cert.exe`.
- Uncompressed EXE: `build/bin/EasyConnectGUI-multi-vpn-cert.exe`, 20,516,864 bytes, SHA256 `0CB58FB424DA1F0DA4C17BACA8DD7609D1D790813169C07B0086FA9E2EE552AA`. No UPX run, per user's latest instruction. Previous artifacts and running clients preserved.
- Browser preview: EasyConnect hides the option; UniVPN shows default unchecked with warning; checking works. Screenshot `build/bin/univpn-cert-option-preview.png` contains example background states, no real credentials and no saved real profile.
- No OS certificate-store changes and no live gateway login. A successful mock certificate bypass does not prove other firmware/authentication behavior.

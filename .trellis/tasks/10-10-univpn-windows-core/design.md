# Design

## Boundary
The missing behavior is a verified Windows UniVPN engine. It lives in the patched OpenConnect core, separate from our existing zju-connect engine. This phase does not replace engine.Start or change the GUI.

## Inputs
- Bruce-lhq/univpn-connect revision 037acad1b5101c756f5583a3aa8effaa260a81db.
- OpenConnect revision 70d1e79d1e55849dfc71dcc199b1edb535b547e4.
- Upstream archive SHA-256 c193f6f0f197bf4cdcc15d74c8803f9073406ddcd2f4d4ce8455ee1f76dcb72f.
- Four public patches in the source project's native/openconnect directory.

## Build and evidence
Use MSYS2 MINGW64 with GCC, autotools, OpenSSL, libxml2 and zlib. Keep downloaded sources and compiled binaries under ignored build/bin. Use the upstream preparation/build scripts after inspecting them. Preserve source and dependency license records. Write our verification report under docs/.

## Compatibility
The implementation uses password authentication and dual TLS IPv4 channels. MFA, browser SSO, IPv6, DTLS and universal firmware support are outside the observed subset. Real TUN cleanup and live gateway connectivity remain separate from CLI and local codec tests.

## Future integration boundary
If live compatibility succeeds, adapt the core to the existing application's lifecycle and resource policy. Protocol capabilities must determine which settings appear; server routes and DNS cannot be assumed to match EasyConnect. This phase introduces no protocol fallback.

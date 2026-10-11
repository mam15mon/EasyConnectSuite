# Sources and observations

- https://github.com/Bruce-lhq/univpn-connect — independent experimental client; management MIT, modified core LGPL-2.1.
- https://github.com/Bruce-lhq/univpn-connect/blob/main/docs/validation.md — one real gateway/macOS tested upstream; Windows builds remain pending there.
- https://github.com/Bruce-lhq/univpn-connect/blob/main/docs/protocol.md — authentication, assigned IPv4, control/data TLS framing and heartbeat observations.
- https://github.com/Bruce-lhq/univpn-connect/blob/main/docs/building.md — Windows MINGW64 build and Wintun staging.
- https://www.infradead.org/openconnect/ — upstream's supported protocol list does not include UniVPN; the additional patches are required.
- https://github.com/mindfold-ai/Trellis/blob/main/README_CN.md — user's selected workflow tool.

The user confirmed Huawei/Leagsoft UniVPN and username/password-only authentication. Client version and the authorized test gateway remain unknown. The user prefers a Go protocol implementation; no official-client container or C binary fallback is introduced.

Source revision: 037acad1b5101c756f5583a3aa8effaa260a81db. The Go port follows the observed protocol and C patches and retains LGPL-2.1-or-later notices. See docs/univpn-go.md for reproducible checks and the separate live gateway/GUI gates.

# Sources and observations

- https://github.com/Bruce-lhq/univpn-connect — independent experimental client; management MIT, modified core LGPL-2.1.
- https://github.com/Bruce-lhq/univpn-connect/blob/main/docs/validation.md — one real gateway/macOS tested upstream; Windows builds remain pending there.
- https://github.com/Bruce-lhq/univpn-connect/blob/main/docs/protocol.md — authentication, assigned IPv4, control/data TLS framing and heartbeat observations.
- https://github.com/Bruce-lhq/univpn-connect/blob/main/docs/building.md — Windows MINGW64 build and Wintun staging.
- https://www.infradead.org/openconnect/ — upstream's supported protocol list does not include UniVPN; the additional patches are required.
- https://github.com/mindfold-ai/Trellis/blob/main/README_CN.md — user's selected workflow tool.

The user confirmed Huawei/Leagsoft UniVPN. Client version and authentication method are requested and still unknown. A container running the official client is not used as a fallback.

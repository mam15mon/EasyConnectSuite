# Directory structure

- main.go configures Wails and embeds frontend/dist.
- app.go owns profiles and Wails methods; connections.go manages independent sessions and profile-tagged state/resource events.
- engine/engine.go adapts zju-connect and owns startup/shutdown callbacks.
- engine/policy.go and routes.go derive policy from VPN server resources.
- engine/univpn/ contains the LGPL-2.1-or-later Go UniVPN packet transport, connected to engine.Start through engine/univpn_client.go. The adapter consumes profile authentication and TLS options; docs/univpn-go.md records scope and validation.
- engine/tun_windows.go handles TCP/UDP TUN ingress; routes_windows.go owns Windows routes.
- tun_mode_windows.go handles elevation and stale-rule cleanup; tray_windows.go owns the tray.
- zju-connect/ contains the local Go module `EasyConnectSuite/vpncore`, selected by root `replace EasyConnectSuite/vpncore => ./zju-connect`. All application, core, test and platform imports use `EasyConnectSuite/vpncore/...`. Keep the original license and attribution; the retained directory name identifies source provenance. It contains protocol clients, gVisor networking and proxy services.
- frontend/ contains the plain JavaScript/Vite UI and generated Wails bindings.
- android-app/ and build/android/ are separate Android work; desktop tasks do not require changing them.

Keep Windows code in *_windows.go with matching tests. Put regression tests beside the changed package. Do not restructure existing packages for a single protocol. New protocols must expose their actual capabilities rather than claiming existing EasyConnect resource behavior.

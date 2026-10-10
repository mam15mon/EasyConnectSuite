# Directory structure

- main.go configures Wails and embeds frontend/dist.
- app.go owns profiles, Wails methods, status events and the active engine.
- engine/engine.go adapts zju-connect and owns startup/shutdown callbacks.
- engine/policy.go and routes.go derive policy from VPN server resources.
- engine/univpn/ contains the standalone LGPL-2.1-or-later Go UniVPN packet transport. It is not yet wired into engine.Start; docs/univpn-go.md records its scope and validation.
- engine/tun_windows.go handles TCP/UDP TUN ingress; routes_windows.go owns Windows routes.
- tun_mode_windows.go handles elevation and stale-rule cleanup; tray_windows.go owns the tray.
- zju-connect/ is a local Go module selected by the root go.mod replace directive. It contains protocol clients, gVisor networking and proxy services.
- frontend/ contains the plain JavaScript/Vite UI and generated Wails bindings.
- android-app/ and build/android/ are separate Android work; desktop tasks do not require changing them.

Keep Windows code in *_windows.go with matching tests. Put regression tests beside the changed package. Do not restructure existing packages for a single protocol. New protocols must expose their actual capabilities rather than claiming existing EasyConnect resource behavior.

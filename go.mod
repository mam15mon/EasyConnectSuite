module EasyConnectSuite

go 1.25.6

require (
	github.com/miekg/dns v1.1.72
	github.com/mythologyli/sing-tun v0.0.0-20260201144630-c04d9db95dc7
	github.com/mythologyli/zju-connect v0.0.0-00010101000000-000000000000
	github.com/sagernet/sing v0.7.18
	github.com/wailsapp/wails/v2 v2.12.0
	golang.org/x/sys v0.45.0
	inet.af/netaddr v0.0.0-20230525184311-b8eac61e914a
)

require (
	git.sr.ht/~jackmordaunt/go-toast/v2 v2.0.3 // indirect
	github.com/andybalholm/brotli v1.2.1 // indirect
	github.com/beevik/etree v1.6.0 // indirect
	github.com/bep/debounce v1.2.1 // indirect
	github.com/boombuler/barcode v1.1.0 // indirect
	github.com/ebitengine/purego v0.10.0 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/jchv/go-winloader v0.0.0-20210711035445-715c2860da7e // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/labstack/echo/v4 v4.13.3 // indirect
	github.com/labstack/gommon v0.4.2 // indirect
	github.com/leaanthony/go-ansi-parser v1.6.1 // indirect
	github.com/leaanthony/gosod v1.0.4 // indirect
	github.com/leaanthony/slicer v1.6.0 // indirect
	github.com/leaanthony/u v1.1.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/metacubex/gvisor v0.0.0-20251227095601-261ec1326fe8 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/pquerna/otp v1.5.0 // indirect
	github.com/refraction-networking/utls v1.8.2 // indirect
	github.com/riobard/go-bloom v0.0.0-20200614022211-cdc8013cb5b3 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/sagernet/go-tun2socks v1.16.12-0.20220818015926-16cb67876a61 // indirect
	github.com/sagernet/netlink v0.0.0-20240916134442-83396419aa8b // indirect
	github.com/samber/lo v1.49.1 // indirect
	github.com/scjalliance/comshim v0.0.0-20251021001035-b69f3cdad6f3 // indirect
	github.com/shadowsocks/go-shadowsocks2 v0.1.5 // indirect
	github.com/shirou/gopsutil/v4 v4.26.4 // indirect
	github.com/things-go/go-socks5 v0.1.1 // indirect
	github.com/tkrajina/go-reflector v0.5.8 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	github.com/wailsapp/go-webview2 v1.0.22 // indirect
	github.com/wailsapp/mimetype v1.4.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go4.org/intern v0.0.0-20230525184215-6c62f75575cb // indirect
	go4.org/unsafe/assume-no-moving-gc v0.0.0-20231121144256-b99613f794b6 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/exp v0.0.0-20260508232706-74f9aab9d74a // indirect
	golang.org/x/mod v0.36.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools v0.45.0 // indirect
	gvisor.dev/gvisor v0.0.0-20260129214308-cb856800aa1c // indirect
)

// The VPN engine runs in-process from the vendored fork in ./zju-connect.
replace github.com/mythologyli/zju-connect => ./zju-connect

// Carried over from upstream zju-connect: replaces in dependency modules
// are ignored by the Go toolchain, so this must live here too.
replace inet.af/netaddr => github.com/inetaf/netaddr v0.0.0-20230525184311-b8eac61e914a

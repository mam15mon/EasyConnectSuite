# Validation — Windows, 2026-10-10

- Root `go test -race ./... -count=1 -timeout=75s` and `go vet ./...` passed with temporary CGO/MinGW environment. After adding the final TUN close regression, `go test -race ./engine -count=1 -timeout=45s` and `go vet ./engine` passed.
- Independent local TLS mock gateway exercised actual gVisor TCP and UDP payloads. Two separate UniVPN sessions operated concurrently; closing the first did not interrupt the second's UDP transport. No real gateway was contacted.
- Application tests cover profile migration, separate connection state, stale callbacks, proxy ports/TUN slots, startup failure isolation, cancellation during startup, stop-one/stop-all, edit-after-stop and elevation guards.
- Proxy tests cover independent SOCKS5/HTTP listeners, port conflicts and closing owned active connections without stopping the other listener.
- TUN tests use in-memory backends: unique identities, equal-prefix and overlapping-DNS conflicts, more-specific routes, own-claim refresh, all-slot stale cleanup and wrong-address/other-client preservation. The final shutdown regression checks pending handlers can take the policy lock while closed and route claims stay reserved until teardown completes.
- Modified nested EasyConnect/service/gVisor packages passed tests, race and vet. Nested full `go test ./...` has two unrelated existing Windows failures: keylog and PCAP assert Unix file mode 0600 but Windows reports 0666. All other packages passed. No unrelated fixes added.
- Vite production build and all four resource-filter tests passed. Browser preview verified two example connections, connection switching display and the UniVPN configuration fields. Screenshot: ignored `build/bin/multi-vpn-preview.png`, containing example states only.
- Final production Wails build passed: `wails build -m -nosyncgomod -trimpath -o EasyConnectGUI-multi-vpn.exe`. File size: 20,516,352 bytes. Bindings/bundle are current and Git clean after build.
- Existing desktop client remains running, untouched. Actual EXE launch is deferred to the user exiting the old single-instance client. Real gateway/firmware, elevated TUN and coexistence remain unverified.
- UPX ultra-brute/LZMA compression and integrity result will be recorded after completion.

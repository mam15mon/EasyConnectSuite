# Implementation

- [x] Align MAC with observed format and working prototype algorithm.
- [x] Persist safe numeric AUTH rejection metadata.
- [x] Independent vectors, full-frame TLS mock validation and log persistence test.
- [x] Root race/vet, uncompressed EXE build, spec/docs update and delivery.

Verification (Windows, 2026-10-10): root `go test -race ./... -count=1 -timeout=75s` passes for app, engine and protocol; `go vet ./...` passes. Production `wails build -m -nosyncgomod -trimpath -o EasyConnectGUI-multi-vpn-authfix.exe` passes with Compress=false and no UPX. Artifact size 20,519,424 bytes; SHA256 030142FD02B0EDCF5D53EC689134938FCB66D8F32B8709D96CC2E9171BCF586D. No actual gateway login attempted; user retry remains necessary to establish live compatibility.

Root cause of the missed wire difference: Go followed the native C patch's MAC while the working Python prototype and observed official logs use another encoding. Prior mocks accepted nonempty AUTH fields, so they could not expose that difference. Prevent recurrence by independently verifying the complete AUTH payload and literal MAC values. Missing failure-code persistence was a separate adapter boundary gap: returned UI errors bypass phase-only log output. The log capture test verifies durable numeric metadata without credentials.

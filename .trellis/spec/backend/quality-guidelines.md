# Quality and verification

Make surgical changes directly implementing the request. Match existing style. Do not add fallback mechanisms, speculative abstractions or unrelated cleanup. State assumptions and distinguish source claims from verified behavior.

After a command changes project files, stage only those files and commit with a summary. Read-only commands do not need empty commits. User-requested commits take precedence over Trellis defaults that defer commits.

For Go behavior changes:
- Format touched files with gofmt.
- Run go test ./... and go vet ./... from the root.
- Run relevant nested zju-connect package tests when changing that module; root ./... does not traverse it.
- Use meaningful regression tests for lifecycle, routing, DNS and authentication. engine/tun_cleanup_windows_test.go and reconnect_test.go are examples.
- Build the Windows desktop after exported Wails model or UI changes.
- Distinguish compilation, mock integration, real TUN/routing tests and live gateway compatibility.

Frontend checks are in ../frontend/index.md. build/bin outputs are ignored. Record source revisions, licenses and reproducible native build steps. Do not claim a live connection without testing an authorized gateway.

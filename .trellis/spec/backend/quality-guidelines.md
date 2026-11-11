# Quality and verification

Make surgical changes directly implementing the request. Match existing style. Do not add fallback mechanisms, speculative abstractions or unrelated cleanup. State assumptions and distinguish source claims from verified behavior.

After a command changes project files, stage only those files and commit with a summary. Read-only commands do not need empty commits. User-requested commits take precedence over Trellis defaults that defer commits.

For Go behavior changes:
- Format touched files with gofmt.
- Run go test ./... and go vet ./... from the root.
- Run relevant nested zju-connect package tests when changing that module; root ./... does not traverse it. Release CI must run the complete nested module tests and vet separately on Windows, macOS and Linux.
- POSIX file-mode tests assert 0600 on Unix. On Windows, Go only uses the owner-write bit to control the read-only attribute; test that supported property while retaining content/data checks. FileMode does not verify Windows ACL isolation.
- Use meaningful regression tests for lifecycle, routing, DNS and authentication. engine/tun_cleanup_windows_test.go and reconnect_test.go are examples.
- Build the Windows desktop after exported Wails model or UI changes.
- Distinguish compilation, mock integration, real TUN/routing tests and live gateway compatibility.

Delivery covers Windows, macOS Intel/Apple Silicon, Linux GUI/headless and the current Android EasyConnect prototype. Linux user-facing TUN remains disabled; internal isolated system tests do not establish product support. Push one source revision, wait for its GitHub Actions workflows to pass, and publish those workflow artifacts together in a GitHub Release with source revision, download links and validation limits. Windows CI uses `.github/workflows/windows.yml`. Its hosted VM can lack the required physical PCI device identity: `setupTopSAPIntegrationClient` skips only that exact hardware-unavailable error in adapter integration tests. All other errors still fail; synthetic TopSAP session and Windows device-enumeration tests remain required. Report these skipped hardware-dependent adapter cases rather than claiming they passed. Do not add a product identity override or alternate device algorithm to accommodate CI.

Frontend checks are in ../frontend/index.md. build/bin outputs are ignored. Record source revisions, licenses and reproducible native build steps. Do not claim a live connection without testing an authorized gateway.

## Windows workspace reentry

Window show/restore calls may synchronously dispatch another tray callback on the same OS thread. `showTrayWorkspace` must snapshot the Wails context and shutdown state under `App.mu`, then release it before calling either native window action. Holding the mutex through `WindowUnminimise` can reenter the same function and deadlock the tray, status ticker and reconnect callbacks. A second EXE launch uses this same workspace entry point.

`tray_workspace` operations use the existing bounded observer with `wait_app_lock`, `window_show` and `window_restore` stages and a final shown/skipped outcome. Tests in `tray_workspace_windows_test.go` simulate synchronous reentry from both window actions, verify tray/status reads remain available and check completed diagnostic evidence. Preserve the no-context and shutdown guards. Mock callbacks establish lock safety; real tray clicks and gateway reconnects require separate verification.

## Linux startup and early shutdown

Wails v2.12 registers signal handling before entering GTK's main loop. A window can be visible before `gtk_main`, so immediately calling `gtk_main_quit` can return without stopping the later loop. Linux `OnBeforeClose` waits for an idle callback that observes an active GTK main loop; `linux_close` records the `wait_gtk_main_loop` stage with the bounded operation observer. Do not replace that condition with a fixed sleep or a DOM-ready dependency. Signals received before Wails registers its handler can terminate the process directly, before the application startup callback.

The Linux Actions workflow runs repeated window/startup signal checks and `build/linux/gtk-early-shutdown-test.sh`, which uses a test-only GTK interposer to hold the process before the main loop, checks that the shutdown operation is waiting, then allows the real loop and requires clean completion. These checks exercise the tested Wails/GTK environment, not every Linux desktop or a live VPN session.

Linux Wails starts `OnStartup` on a goroutine and invokes `OnShutdown` after the GTK loop ends. Linux platform options wrap both callbacks with a completion channel: shutdown waits for startup to finish before invoking cleanup, including when shutdown begins before startup runs. `linux_shutdown` records `wait_startup` through the bounded operation observer. `TestLinuxShutdownWaitsForStartup` and `TestLinuxShutdownWaitsWhenStartupHasNotRunYet` observe the wait stage before asserting cleanup has not run; the Linux workflow runs them under the race detector. This guarantee is specific to the Linux callback wiring.

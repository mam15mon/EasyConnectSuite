# Configuration

There is no database or ORM. app.go stores AppConfig and Profile as JSON in vpn_profiles.json. NewApp selects an existing file beside the executable or APPDATA/VPN/vpn_profiles.json. storage.go owns applicationDataDir/applicationLogDir. Before crash logging and Wails startup, migrateLegacyStorage atomically renames APPDATA/EasyConnectVPN to APPDATA/VPN on the same volume, preserving the entire tree. An absent legacy directory is a no-op; existing old and new stores are an explicit conflict and neither is overwritten. Migration errors are visible before startup. WebView uses APPDATA/VPN/webview. TestStorageMigration* validates byte preservation, legacy removal, repeated startup, fresh installs and conflict preservation. Root TestMain isolates APPDATA so mock startup cannot create a real user store. Do not add another storage mechanism during protocol work.

JSON field names are frontend contracts: Profile.TUNMode uses json:"tunMode" and ResourceEntry.Target uses json:"target". Keep generated frontend/wailsjs/go bindings aligned with exported type changes.

Preserve saved EasyConnect profiles when adding a protocol selector. Document any default needed solely to read pre-existing profile files. Never silently substitute protocols after an error.

Profile.protocol defaults to easyconnect when absent; univpn uses Profile.domain for its optional authentication domain. socksPort/httpPort are numeric per-profile ports. Old profiles without ports receive 1080+2*i/1081+2*i; new profiles choose an unused configured pair. Bind errors remain explicit. SaveProfile returns an error and rejects editing a running/stopping connection; DeleteProfile applies the same guard.

Fresh application defaults and profiles created by the GUI set `tunMode=true` and `disableProxy=true` (TUN enabled, local SOCKS5/HTTP proxies disabled). Loading or editing existing profiles preserves their saved mode values, including explicit false values and legacy zero-value behavior; do not migrate all profiles to the new defaults.

Profile.insecureSkipVerify defaults to false. The user requested optional certificate skipping; it now applies separately to UniVPN and TopSAP profiles. The checkbox must explain that gateway identity is unverified; engine.Options passes the selected profile's value into that protocol's TLS config. Never enable it automatically after a verification error, apply it to another profile, or remove the control/data public-key equality check. Reconnect preserves the same profile option. This option does not change Windows certificate stores or EasyConnect behavior. TopSAP uses protocol=topsec, hides UniVPN domain and EasyConnect 2FA/keepalive fields, disables ZJU additions, and gets resources from res_list; see topsec-protocol.md.

connections.go owns sessions by profile ID. Selecting a profile changes displayed status/resources only. Wails StartProfileVPN/StopProfileVPN use explicit IDs; StopAllVPN and shutdown stop every session. Never replace an explicit missing ID with the selected profile. Each engine gets its own context, cleanup callbacks, listeners and TUN slot. GUI-owned EasyConnect/gVisor paths must not execute process-global CLI terminal hooks.

Tray menus use a locked snapshot of all connection statuses, with a tray-only stopping marker. Store profile IDs in the command map when the menu is built; do not resolve the action through current selection or a later profile-array index. Connect/disconnect actions preserve selection; only details calls SetActiveProfile and emits profile-switched. Count connected separately from connecting/reconnecting/stopping/error. Cancellation classification comes from this startup's context before startConnectionID cancels a failed connection, not from whichever session now occupies the profile ID. The icon data is protected by trayIconMu; shutdown posts the destroy/quit message to the owning tray thread. Tooltip text fits 127 UTF-16 units plus NUL without splitting a surrogate pair. Native menu regression tests may create/query/destroy menus without showing them or accessing real accounts.

Windows product metadata, executable and window title are VPN. Keep the single-instance identity stable across this rename: a second launch activates the existing live instance. HideWindowOnClose remains true: window X hides to the tray without disconnecting. Tray exit sets shuttingDown and requests runtime.Quit; only Wails OnShutdown invokes App.shutdown, guarded by shutdownOnce. Keep the tray until connection and Worker cleanup returns. Stop UI events/ticks and reject workspace activation during shutdown because the Wails message loop is ending. TestShutdownRetainsTrayUntilCleanupAndRunsOnce blocks a reconnecting mock engine, verifies cancellation and retained tray, rejects new startup, joins concurrent shutdown calls and confirms one cleanup and final tray removal.

Current profiles include password and TOTP fields in saved JSON; this is not an OS credential vault. Do not copy real profiles, credentials or APPDATA logs into tasks or Git. The root .gitignore excludes vpn_profiles.json, captures and logs.

SetProxyEnabled and SetTUNEnabled illustrate restoring the previous in-memory value when configuration saving fails.

## Scenario: Session identity and explicit profile resources

### 1. Scope / Trigger

Multi-VPN startup, retry, profile selection and resource refresh cross the Go/Wails/JavaScript boundary. An old startup can finish after a replacement session; local UI selection can precede the serialized backend selection.

### 2. Signatures

- `GetProfileVPNResources(id string) ([]engine.ResourceEntry, error)` is the explicit resource query; generated Wails bindings expose `GetProfileVPNResources(arg1: string): Promise<Array<engine.ResourceEntry>>`.
- Existing `GetVPNResources() []engine.ResourceEntry` remains compatible. New UI refreshes use the explicit API without falling back to the selection-based API.
- `startConnectionID(id string, start func(engine.Options) (vpnEngine, error)) string` returns `OK` or an error string. That string describes the request that finished, not necessarily the session currently occupying the ID.

### 3. Contracts

`id` must identify a saved profile; never resolve an unknown/empty ID through `ActiveProfileID`. Resource entries retain string fields `kind`, `target`, `ports`, `protocol`. Query under `a.mu`, return a slice copy, and preserve selection/status/sessions. A nil Go slice is an empty resource result at the UI boundary (Wails may serialize it as null).

Publish a startup result only while `a.connections[p.ID] == c && !c.stopping`. Even when superseded, assign that startup's engine, cancel its context on error, close its `ready` channel and return its own error. A log-directory failure removes only that attempted session and emits its profile-scoped status; update global details and clear global resources only when that profile is selected. `APPDATA` determines the existing log location; a directory error remains explicit, with no alternate directory.

UI resource reads capture profile ID, selection version and request version; stale responses and responses invalidated by status/resource events cannot update the cache or view. On a startup error, refresh `GetConnections()` and use its latest status for that ID rather than synthesizing an error status from the return string. Keep the error notification for the failed request.

### 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Known profile, current running/non-stopping session | Independent snapshot of its resources |
| Known profile, absent/disconnected/error/stopping session | Empty resources, no error |
| Unknown or empty profile ID | `服务器配置不存在`; Wails Promise rejects |
| Old startup returns after replacement | Old request completes; replacement details/resources remain intact |
| Log directory fails for a non-selected profile | Original error; selected connection unchanged; attempted profile remains stopped |
| Log directory fails for selected profile | Original error; non-running details and empty resources |

### 5. Good/Base/Bad Cases

- Good: UI has selected B while backend still selects A; querying B returns B's resources without changing the backend selection.
- Base: Selecting a disconnected profile displays an empty resource view.
- Bad: An old A startup fails after tray retry connects a new A session; publishing the old result would contradict the connection list and discard the new resource view.

### 6. Tests Required

`connections_test.go` controls startup ordering with channels, checks replacement/list/detail/resource agreement and successful stop, simulates directory failure with a file in a temporary APPDATA, verifies unrelated state plus selected rollback and retry, and asserts explicit-ID resource snapshots cannot mutate the session or selection. `frontend/src/connection-state.test.mjs` executes production functions with controlled Promises: queued selection, reverse responses, A→B→A, disconnect/resource-event invalidation, late startup error versus newer success, visible current failure, and no legacy query after unknown-ID rejection. Preview must honor the same explicit-ID contract. Test fixtures never use real accounts or logs.

### 7. Wrong vs Correct

Wrong: guard startup publishing only with `!c.stopping`, call `GetVPNResources()` after local selection changes, or turn every failed startup string into `receiveStatus({state:'error'})`.

Correct:

```go
if a.connections[p.ID] == c && !c.stopping {
    // Publish only this profile's current session.
}
```

```js
const rows = await window.go.main.App.GetProfileVPNResources(id);
if (id !== activeProfile?.id || version !== selectionVersion || request !== resourceRequest) return;
```

## Connection-stage feedback

VPNStatus adds stageDetail and stageStartedAt (Unix milliseconds). applyEngineStatus resets stageStartedAt when entering connecting or changing Stage/StageDetail, preserves it for repeated messages in the same stage, and clears it outside connecting. PhaseReconnecting retains Stage=重连中 so TUN and tray behavior remains compatible; StageDetail describes authentication, resource load, TUN update, resource publication or tunnel rebuild. Engine emits these statuses before the existing blocking steps and still logs them via emitStatusFull/operation diagnostics. No timing threshold changes recovery or implies a diagnosed root cause.

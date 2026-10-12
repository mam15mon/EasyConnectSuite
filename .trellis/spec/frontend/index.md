# Frontend guidelines

The desktop UI is plain JavaScript/CSS with Vite, not React or Vue.

- frontend/src/main.js owns rendering, profile editing and Wails status/resource subscriptions.
- frontend/src/app.css and style.css define the existing appearance.
- frontend/src/resource-filter.mjs contains filtering logic; resource-filter.test.mjs tests it.
- frontend/wailsjs/go/main/App.js and models.ts are generated bindings; regenerate them when exported Go APIs/models change.

Use existing Chinese labels and status flows. Show protocol-specific fields and unsupported options only where applicable. A VPN-type selection must preserve existing profiles and saved options.

Maintain the connection list separately from selected-profile details. Filter vpn-status, vpn-resources and tick events by profileId before updating the main view. Switching profiles must not stop a connection. Connect/stop actions pass the captured profile ID to Wails, and an async failure must not replace another profile's current status. UniVPN shows an optional authentication domain, hides unsupported 2FA/EasyConnect keepalive options, and preserves password whitespace. Preview states are demonstrations, never evidence of a real gateway connection.

The light workspace uses a persistent left connection list and a right detail workspace with resources/settings/logs tabs. Each list item has its own lifecycle action; global counters aggregate all profiles regardless of the current filter/selection. Selection updates locally then serializes SetActiveProfile calls; selection/request versions guard stale results. SetProfileTUNMode takes the captured profile ID explicitly. Resource events keep the {profileId, entries} envelope; TUN/proxy modes are immutable while that connection runs.

The logs tab calls GetProfileLogs(id) for the explicitly selected ID, guards request identity, renders using textContent and refreshes every 15 seconds only while visible. Clear old output whenever profile selection changes, including refreshProfiles after profile creation/deletion. Preserve the reader's scroll position unless already following the latest entry. Keyboard tab navigation and modal focus/Escape behavior must work at the 840x660 minimum window. Browser preview requires the explicit ?preview query and redacted fixtures; it must never initiate a real VPN connection.

Resource refresh calls GetProfileVPNResources(id), preserves selection/request guards and event invalidation, and never falls back to GetVPNResources. A failed startup string must trigger a current connection snapshot instead of generating a status that could overwrite a later tray retry. See [session identity and explicit resource contracts](../backend/configuration-guidelines.md#scenario-session-identity-and-explicit-profile-resources). Run node --test frontend/src/connection-state.test.mjs after changes to these flows; its controlled Promises exercise production functions.

Run npm run build in frontend after UI changes. Run node --test frontend/src/resource-filter.test.mjs when filtering changes. Do not add a framework, generic settings system or unrelated redesign for UniVPN.

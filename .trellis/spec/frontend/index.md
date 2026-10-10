# Frontend guidelines

The desktop UI is plain JavaScript/CSS with Vite, not React or Vue.

- frontend/src/main.js owns rendering, profile editing and Wails status/resource subscriptions.
- frontend/src/app.css and style.css define the existing appearance.
- frontend/src/resource-filter.mjs contains filtering logic; resource-filter.test.mjs tests it.
- frontend/wailsjs/go/main/App.js and models.ts are generated bindings; regenerate them when exported Go APIs/models change.

Use existing Chinese labels and status flows. Show protocol-specific fields and unsupported options only where applicable. A VPN-type selection must preserve existing profiles and saved options.

Maintain the connection list separately from selected-profile details. Filter vpn-status, vpn-resources and tick events by profileId before updating the main view. Switching profiles must not stop a connection. Connect/stop actions pass the captured profile ID to Wails, and an async failure must not replace another profile's current status. UniVPN shows an optional authentication domain, hides unsupported 2FA/EasyConnect keepalive options, and preserves password whitespace. Preview states are demonstrations, never evidence of a real gateway connection.

Run npm run build in frontend after UI changes. Run node --test frontend/src/resource-filter.test.mjs when filtering changes. Do not add a framework, generic settings system or unrelated redesign for UniVPN.

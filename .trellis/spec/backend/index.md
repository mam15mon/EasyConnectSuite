# Backend guidelines

The desktop backend is Go 1.25.6 with Wails v2. It embeds the local zju-connect fork.

| Guide | Scope |
| --- | --- |
| [Directory structure](directory-structure.md) | Application, engine and upstream fork |
| [Configuration](configuration-guidelines.md) | JSON profiles and Wails serialization |
| [Errors](error-handling.md) | Rollback, authentication refusal and cancellation |
| [Logging](logging-guidelines.md) | Engine logs and credentials |
| [Quality](quality-guidelines.md) | Minimal changes and Windows verification |
| [TopSAP](topsec-protocol.md) | Observed authentication, device identity and IPv4 wire contracts |

For UI changes read ../frontend/index.md. User rules in root AGENTS.md take precedence over generated workflow defaults.

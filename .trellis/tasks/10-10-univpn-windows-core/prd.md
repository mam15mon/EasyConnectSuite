# UniVPN Windows core validation

## Requirement
The user approved investigating Huawei/Leagsoft UniVPN as the next backend for a unified VPN client and requested installing Mindfold Trellis to support the work.

## Scope
Validate the independent experimental OpenConnect-based UniVPN engine on Windows before wiring it into the existing desktop. Preserve current EasyConnect behavior.

## Acceptance
- Official Trellis CLI and Codex project integration work; concrete project specs replace empty templates.
- A pinned UniVPN patch source and pinned upstream OpenConnect archive are used, with archive SHA-256 verification.
- The Windows core builds, starts and lists the experimental UniVPN protocol.
- Local protocol tests distinguish authentication refusal, frame handling and transport from actual gateway interoperability.
- Record reproducible commands, source revisions, dependencies and limitations.
- Live compatibility requires the user's client version, authentication method and an authorized test gateway; do not claim it without evidence.

## Constraints
No new fallback mechanisms. No credentials in commands, logs, task documents or Git. No GUI support claims before backend verification.

# UniVPN Go protocol core

## Requirement
The user approved Huawei/Leagsoft UniVPN as the next backend, installed Mindfold Trellis, and steered the implementation toward Go rather than bundling the C/OpenConnect engine. Authentication currently uses username and password.

## Acceptance
- Trellis CLI, Codex project files, concrete specs and task context work.
- Implement the observed password/dual-TLS IPv4 subset in Go using the existing protocol documentation as reference.
- Validate fragmented framing, authentication refusal, assigned IP/routes, matching TLS identities, packet round trips, cancellation and control-channel failure against local TLS fixtures.
- Windows Go compilation, package tests and vet pass.
- Record protocol source revision, license, evidence and unsupported features.
- Do not claim live gateway compatibility or GUI support until separately verified.

## Constraints
No protocol fallback. No insecure automatic TLS downgrade. No credentials in logs/Git.
Preserve the existing EasyConnect backend. GUI integration and real gateway tests are subsequent gates; the user's gateway and exact version are not yet available.

# UniVPN certificate verification switch

User explicitly asks to ignore the untrusted-certificate error. Add a UniVPN-only profile checkbox with a clear warning; default off, including old profiles. Persist the option and pass it to both TLS handshakes for that connection, without an automatic retry/fallback. The existing control/data public-key equality check remains required. Password authentication and other profiles retain their behavior.

Acceptance: default rejects an untrusted mock gateway, explicit opt-in connects, mismatched channel keys still fail with opt-in, profile persistence and option propagation pass, UI/bindings and Windows EXE rebuild succeed, and UPX integrity passes. Real gateway compatibility remains unverified. Existing executables/processes are preserved; deliver a new artifact name.

# UniVPN authentication diagnosis

User confirms the password works with official UniVPN 10781.16.0.0729 and supplies a settings screenshot with an IP endpoint, port, adaptive tunnel mode, no auth-domain choice and national crypto disabled. Existing code converts every nonzero AUTH status to a password-specific message, losing the result code. That diagnosis is unsupported.

Fix the known error-reporting bug: retain numeric status in decimal and hex, describe authentication rejection without asserting wrong credentials, preserve stop-on-first-rejection, and include a hint to check gateway policy/protocol compatibility. Keep existing transport/auth fields and domain behavior until additional evidence warrants a change. Do not try credentials automatically, log tokens, copy private config/logs, or change the official running client.

Acceptance: arbitrary nonzero statuses retain their exact codes, malformed/success responses remain separate, adapter retains typed rejection and terminal sentinel, mock rejection uses a nontrivial status, root race/vet and uncompressed EXE build pass. Live login resolution is still pending the actual gateway code; diagnostic completion must not be represented as a compatibility fix.

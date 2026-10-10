# Boundary and findings

Change protocol.go with a typed AuthenticationError carrying Code and unwrapping ErrLoginRejected. The sentinel becomes neutral authentication rejection. UniVPN adapter wraps both its typed cause and EasyConnect's terminal sentinel so gVisor stops immediately and callers can inspect the numeric result. Tests compare multiple result codes and an actual TLS mock response. No UI/API schema change required: Wails already displays the returned error string.

Go authentication framing, escaping and synthetic MAC match the pinned C reference. Its Python prototype uses another MAC representation; no evidence identifies that difference as this gateway's cause, so do not switch or retry variants. Blank domain currently uses host, like reference. Official local profile has no domain key; that does not prove the wire domain is empty or the host. Official log has login-success message identifiers but no exposed authentication wire data.

Public official release page identifies the supplied version as a Leagsoft release. The reference is an experimental observed subset, not verified compatibility with this firmware. Private metadata inspection outputs only booleans, field names and official static message identifiers.

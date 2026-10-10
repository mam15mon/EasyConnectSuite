# Go protocol core design

The user prefers a Go implementation to fit the current in-process gVisor/TUN/proxy engine. Add engine/univpn as a packet-oriented transport library without changing engine.Start or the UI yet.

The library establishes a verified TLS control channel, sends the observed FirstConnect/authentication/REQVIP messages, then opens a second TLS channel bound to the same server SPKI identity. It reads/writes complete IPv4 packets and maintains the observed 10/15-second heartbeats. A control-channel failure closes the data channel; caller cancellation closes both. It does not implement TCP itself: future integration reuses the current gVisor stack.

Sources: Bruce-lhq/univpn-connect 037acad1b5101c756f5583a3aa8effaa260a81db; the protocol and C reference cover one gateway and a limited firmware subset. Retain LGPL-2.1 attribution/license for the port.

C validation exposed skipped patches when git apply ran beneath a repository root, plus CRLF patch checkout issues. Applying the original LF blobs with --directory=build/openconnect fixed patch application. Native codec tests pass. The full C build currently fails on Windows/OpenSSL declarations and is not a shipping dependency.

Do not infer DNS or domain resources from the address/route TLVs. No MFA, IPv6, DTLS, compression or universal gateway compatibility is claimed. The Go core has no automatic reauthentication; the application will own reconnect decisions when integrated.

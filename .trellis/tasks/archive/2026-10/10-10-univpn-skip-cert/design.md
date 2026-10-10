# Change boundary

The gap is that UniVPN's GUI cannot opt into the existing transport TLSConfig setting. Add Profile.insecureSkipVerify, engine.Options.InsecureSkipVerify and a constructor flag; only the UniVPN factory consumes it. The transport already supports tls.Config and retains the channel identity check, so no core protocol change is needed. Update the existing profile editor with a UniVPN-only checkbox and one explanatory sentence. Preserve secure defaults and password behavior. No CA import, certificate pinning, protocol fallback or OS certificate-store changes.

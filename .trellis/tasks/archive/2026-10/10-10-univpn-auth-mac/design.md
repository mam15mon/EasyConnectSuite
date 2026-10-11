# Implementation boundary

session.go currently computes SHA256(username), uses fixed first byte 02 and emits colon-separated lowercase. Replace only that MAC calculation/format with the pinned Python prototype: SHA256(username + "|" + gateway host), first six bytes, first byte (b|2)&254, uppercase XXXX-XXXX-XXXX. Extract stableMAC into protocol.go for literal reference-vector coverage and call it from the AUTH builder; leave other fields unchanged.

Adapter Setup already receives AuthenticationError. Before returning the terminal wrapper, log only Code decimal/hex, avoiding raw response data. Integration test uses the existing logCapture/rotatingWriter to verify persistent output, even when startup fails.

Evidence: official log metadata shows authentication domain matches gateway, MAC length/shape 14/XXXX-XXXX-XXXX, MAC auth result 0. UI shows code 2; current engine.log stops at authentication and lacks the rejection code. No numeric mapping establishes code 2 as a MAC error. Treat this as an observed compatibility difference, not a proven complete login fix.

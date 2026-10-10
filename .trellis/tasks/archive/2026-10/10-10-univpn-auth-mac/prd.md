# Authentication MAC encoding and failure logging

Read-only inspection finds actual AUTH result 2 / 0x00000002 in the running diagnostic UI. Official successful authentication logs identify a domain matching the configured gateway, and a MAC TLV of 14 uppercase characters in four-hex-digit hyphen groups. Go currently emits 17 lowercase colon-separated characters. The pinned source's working Python prototype emits the observed official format.

Correct MAC wire encoding using that prototype's stable username+gateway algorithm, retain auth escaping/domain/certificate behavior and immediate rejection stop. Persist only numeric rejection metadata into the existing engine.log. Keep all private logs, configurations, identifiers and credentials out of task/Git. No automatic real login, protocol fallback, MAC probing or changes to the running official client. Deliver a new uncompressed EXE.

Acceptance: literal reference vectors pass for the MAC value and complete authentication frame, mock TLS authentication verifies all fields independently, rejected authentication persists numeric code without credentials, root race/vet and Wails build pass. Live login success remains a separate user test; code 2 has no confirmed semantic mapping.

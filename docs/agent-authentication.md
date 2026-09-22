# Agent WebSocket authentication — V1

Enrollment authorizes initial registration; it is not runtime authentication.
Agent ID, /exists, hostname, MAC and IP are not proof of identity.
TLS/WSS authenticates the server and protects transport. Ed25519 challenge-response
authenticates the Agent inside that connection. /ws/frontend retains its cookie,
Origin and RBAC checks and never participates in this handshake.

## Wire sequence (all modes, no UUID-only fallback)

1. Agent sends auth_hello: type, version, canonical lowercase UUID agent_id,
   and a fresh client_nonce (32 crypto/rand bytes, standard padded Base64).
2. Server loads agents.public_key using a credential-only SELECT. NULL, missing,
   malformed or wrong-length keys fail closed. No key comes from the WS client.
3. Server sends auth_challenge: type, version, challenge_id and server_nonce.
   Both random fields contain 32 independent crypto/rand bytes, Base64 encoded.
4. Agent loads its existing key through service.LoadPrivateKey, signs the
   transcript directly with ed25519.Sign and sends auth_proof: type, version,
   challenge_id, signature (64 bytes, standard padded Base64).
5. Server verifies with ed25519.Verify and sends auth_ok: type, version.
6. Agent immediately sends its existing system-info message, then normal traffic.
   No heartbeat, queued result or feature worker sends data before auth_ok;
   initial system info precedes asynchronous job results.
7. Server checks that initial system-info ID matches the authenticated ID, loads
   normal AgentInfo, then sets ONLINE and registry.Add under the Agent lifecycle
   lock. Both operations happen only after signature verification and auth_ok.
   A failed ONLINE database write leaves the old registry entry untouched.

The version for every auth message is THESIS-RAT-AGENT-AUTH-V1.
Auth messages are text JSON, bounded to 4096 bytes, parsed into strict structs.
Unsupported versions, unknown fields, malformed canonical Base64, invalid UUIDs,
wrong nonce/signature lengths and wrong message order fail closed. Binary auth
messages are rejected. Another auth_* message during normal traffic closes the
connection; proofs are not accepted again.

## Canonical transcript

Five fields, each encoded as uint32 big-endian byte length followed by its bytes:
1. UTF-8 THESIS-RAT-AGENT-AUTH-V1 (24 bytes).
2. Canonical lowercase hyphenated Agent UUID (36 ASCII bytes).
3. Decoded challenge ID (32 bytes).
4. Decoded client nonce (32 bytes).
5. Decoded server nonce (32 bytes).

Total: 176 bytes. Sign the transcript directly, not JSON and not a manual hash.
internal/agentauth/protocol.go is the single encoder in each repository.
Keep the two implementations and deterministic tests identical; there is no new
cross-repository runtime dependency.

The non-secret test vector uses ID 11111111-1111-4111-8111-111111111111,
challenge bytes 00..1f, client nonce bytes 20..3f and server nonce bytes 40..5f.
Both protocol_test.go files assert the same complete expected transcript hex
and exercise signing plus rejection after transcript tampering.

## Timeout, replay and duplicate connections

The server's 12-second deadline starts after WebSocket acceptance and covers
hello, credential lookup, challenge, proof and initial system info.
Credential DB lookup also has a maximum 5-second timeout.
The Agent uses a separate 12-second handshake deadline after the TLS/WS dial.
Normal sessions do not retain the authentication deadline. A failed attempt
terminates that socket; each reconnect gets fresh client/server randomness.

There is one local challenge and one proof attempt per connection, no reusable
global challenge cache. Old proofs and proofs from another socket fail against
the new transcript even when their challenge ID is replaced.

An unauthenticated socket cannot enter registry, change ONLINE/OFFLINE, replace
a legitimate Agent, publish status or start normal feature handling.
A new connection with a valid proof and matching initial metadata replaces the
old authenticated connection. The old socket is closed; registry.Remove compares
the connection pointer before removing anything. Bounded lifecycle lock stripes
serialize each ID's ONLINE/OFFLINE writes with registry replacement, preventing a
late old-handler OFFLINE write from overwriting the new connection's ONLINE.

These guarantees are for a single WS server process. Multiple active WS replicas
sharing one database still need distributed connection/status ownership design.
The existing one-server/many-Agent deployment is unchanged.

## Private-key and legacy identity behavior

Only the authoritative Phase 2 loader decrypts. The runtime holds its existing
lock, binds signing to the startup identity's public key, and clears the loaded
private-key byte buffer after signing. Clearing is best-effort Go memory hygiene,
not a guarantee about all compiler/runtime copies. Private keys, ciphertext,
DPAPI blobs, tokens, signatures and nonces are not written to auth logs.

LoadPrivateKey currently accepts only machine-scope identity with protected ACLs.
This applies to Console as well as Service. Ordinary legacy user-scope Console
identities cannot authenticate under this policy. There is no alternate decrypt
path or automatic conversion in Phase 4.

SCM can still start with legacy identity, but authenticated remote connectivity
cannot succeed until the operator completes explicit Phase 2 migration. Errors
flow through the existing bounded backoff; startup never generates replacement
keys, changes Agent ID or auto-migrates. Enrollment is not repeated by the WS
reconnect loop. Unknown/decrypt/ACL/key mismatch failures also fail closed.

Do not copy an identity to an unprotected working directory for testing.
To use an existing machine identity interactively, stop its Service, use an
elevated console in the protected runtime directory, and preserve the runtime
lock/config/path requirements. Never run Service and Console against one identity
at the same time.

## Deployment and manual verification

Agent and WS must be deployed together in a controlled test environment.
Old clients cannot authenticate to the new WS server; new clients do not downgrade
to old UUID-only servers. Do not disable TLS verification to resolve compatibility.

Manual checks still required:
1. Finish Phase 2 migration checks and verify LocalSystem LoadPrivateKey using the
   existing maintenance command. Preserve Agent ID, exact key pair and server key.
2. Deploy both Phase 4 binaries to a lab using the Phase 3 HTTPS/WSS topology.
3. Confirm valid identity authenticates; registry/ONLINE follows verification.
4. Use a disposable test client claiming the same ID with another key. Confirm
   rejection and that the legitimate Agent remains connected.
5. Reconnect the legitimate Agent, test captured-proof replay and inspect final
   registry/status after overlapping old/new cleanup.
6. Reboot and verify authenticated reconnect, intentional administrator stop and
   SCM recovery after a crash.
7. Smoke-test process, performance, power, antivirus, downloads/distribution and
   interactive-console screen capture. Service Session 0 limitation is unchanged.
8. Verify browser login, cookies and /ws/frontend behind the real proxy.

No automated developer-account test proves LocalSystem DPAPI/trust, actual protected
migration, Cloudflare, SCM deployment/reboot or real power-loss behavior.
Those Phase 2–4 manual items remain NOT VERIFIED until actually performed.

Future Phase 5 should prioritize bounded pre-auth connection admission, database
load limits, operational metrics, timeout/status fault injection and authenticated
connection ownership under deployment scaling. Key rotation/revocation or mTLS
requires separate design; this phase adds none of them.

# WebSocket/download HTTPS deployment

Production config (database and existing storage settings are still required):

```dotenv
TRANSPORT_MODE=production
SERVER_ADDRESS=127.0.0.1:8081
PUBLIC_BASE_URL=https://socket.example.com
FRONTEND_ORIGINS=lab.example.com
```

Process environment overrides .env. Default mode is development. Production
requires a literal loopback bind, HTTPS public download base and an explicit list
of exact frontend host[:port] entries; wildcard patterns and URL-format entries
are rejected. Do not add http:// or https:// to FRONTEND_ORIGINS.
Production browser connections additionally require Origin: https://<listed-host>;
an arbitrary Host or forwarding header cannot bypass this list. The existing
session-cookie lookup and per-command permissions remain required.

The Go HTTP origin need not terminate TLS. deploy/cloudflared.example.yml routes
public HTTPS/WSS to same-host loopback HTTP. Adapt placeholder hostnames/tunnel ID,
configure DNS and validate rules with cloudflared tunnel ingress validate and
cloudflared tunnel ingress rule before deployment. No live tunnel is changed here.
Require HTTPS at the public edge, enable WebSockets, and bypass caching for API,
session and temporary-download responses. Ensure edge policies permit the existing
Agent endpoints without a browser-only login challenge.

Keep the Dashboard and /ws/frontend at lab.example.com so __Host-session can be
sent by the browser. Route the rest of that hostname (including /api/*) to Next.js.
Agent /ws and /files/download/ can use socket.example.com; API uses api.example.com.
Configure PUBLIC_BASE_URL explicitly, never derive it from forwarded headers.
Download token TTL/hash/consumption, X-Agent-ID and payloads remain unchanged.
REST and WS must share the intended DB, and WS must access REST's stored files
within FILE_STORAGE_ROOT. Existing storage path requirements still apply.

Bind origins only to loopback and prevent external access to internal ports.
cloudflared and origin must share the same trusted host for plaintext loopback.
For a different host, use a validating HTTPS hop to a local origin proxy or
another protected channel. No origin noTLSVerify option is appropriate.
Preserve the public Host and browser Origin through the proxy. Strip/overwrite
client forwarding headers at the edge; current audit code records direct peers,
not purported visitor IPs. Next.js must also be bound to loopback.

TLS validates the server. Agent /ws now also requires Phase 4 Ed25519 proof before
ONLINE, registry.Add or connection replacement. See [Agent authentication](agent-authentication.md).
The public key is loaded through a credential-only query and is not added to
normal frontend AgentInfo JSON. /ws/frontend keeps its existing session model.

Manual checks: public HTTPS/WSS and downloads, Dashboard cookie-authenticated
WS plus RBAC, invalid browser Origin rejection, protected origin access,
Agent LocalSystem trusted/untrusted/wrong-host/expired certificates, tunnel outage
and reconnect, and unchanged feature protocols. Unit tests do not verify deployment.

References:
- https://developers.cloudflare.com/tunnel/features/locally-managed-tunnels/configuration-file/
- https://developers.cloudflare.com/network/websockets/

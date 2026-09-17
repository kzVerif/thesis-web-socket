# Mock shutdown (Phase 1)

The server routes power commands over the existing WebSockets. The agent repository
must implement its final operation using `MockPowerController`; no Windows shutdown
implementation is added here. `success: true` means the agent accepted the operation,
not that the machine powered off. A disconnect never counts as success.

## Authentication and authorization

Dashboard connects to `/ws/frontend` with the existing `__Host-session` cookie and
origin checks. Sessions are revalidated before every message. Both power actions
add a server-side `agents.control` permission check against that same session.
File distribution still checks `files.distribute`; neither permission implies the
other. No new token, permission seed, table, migration, or commands-table workflow
is required. Custom session stores must implement `FrontendPermissionStore` to
enable power; otherwise requests fail closed.

## Single agent

Dashboard sends:

```json
{"type":"power","action":"shutdown","agent_id":"11111111-1111-4111-8111-111111111111","request_id":"55555555-5555-4555-8555-555555555555"}
```

Server verifies the agent exists in PostgreSQL and is online in Registry, then sends:

```json
{"type":"power","action":"shutdown","request_id":"55555555-5555-4555-8555-555555555555"}
```

Agent replies:

```json
{"type":"power","action":"shutdown_result","request_id":"55555555-5555-4555-8555-555555555555","success":true,"mode":"mock","message":"shutdown command accepted"}
```

Server returns the same result to the originating Dashboard connection, adding
`agent_id` from the connection identity. A payload-supplied `agent_id` is ignored.
Results must come from the exact agent connection targeted by this request; a
replacement connection cannot resolve its predecessor's pending commands. Missing
`success`, a mode other than `mock`, malformed messages, unknown/expired request IDs,
and duplicate results are ignored. Agent failures retain `success: false` and their
message.

## Room

Dashboard sends one request:

```json
{"type":"power","action":"shutdown_room","room_id":"44444444-4444-4444-8444-444444444444","request_id":"55555555-5555-4555-8555-555555555555"}
```

Server queries **all** members using `agents.room_id`, including members whose
persisted status is disabled/offline, and determines online membership using Registry.
This snapshot defines the counts for the operation. Server sends the single-agent
`power/shutdown` command to every online target with the same `request_id`.
Agents do not need a `shutdown_room` handler. Membership changes or agents connecting
after the snapshot do not add targets to an operation already in progress.

One final aggregate is returned when every online target responds/fails to send,
or approximately ten seconds after registration:

```json
{"type":"power","action":"shutdown_room_result","request_id":"55555555-5555-4555-8555-555555555555","room_id":"44444444-4444-4444-8444-444444444444","total":10,"online":8,"offline":2,"accepted":7,"failed":1,"timeout":0}
```

`total = online + offline`; `online = accepted + failed + timeout`.
Send failures and agent-declared failures count as `failed`. Missing responses,
including disconnections before a response, count as `timeout`. Empty rooms and
rooms with no online agents return immediately with zero accepted/failed/timeout.

## Errors and lifecycle

All IDs must use the UUID shape `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`.
Use a fresh, globally unique `request_id` for each operation and echo it exactly
in the agent response. Concurrent reuse of a pending ID is rejected, including
reuse by another frontend. Completed requests are not persisted or replay-cached;
this is correlation, not a durable idempotency API. Do not reuse old IDs.

Request errors use the existing error envelope with power correlation fields:

```json
{"type":"error","stream":"power","action":"shutdown","request_id":"55555555-5555-4555-8555-555555555555","agent_id":"11111111-1111-4111-8111-111111111111","room_id":"","code":"forbidden","error":"agents.control permission required"}
```

Codes: `invalid_request`, `forbidden`, `authorization_unavailable`, `agent_not_found`,
`agent_offline`, `room_not_found`, `lookup_failed`, `duplicate_request`.
Single-agent dispatch failures/timeouts use `power/shutdown_result` with
`success: false`, `mode: "mock"`, an explanatory `message`, and respectively
`code: "send_failed"` or `code: "timeout"`. Server-generated failure responses do
not claim that the agent executed the command.

Pending state is synchronized, held in memory, and removed on completion, timeout,
or frontend disconnection. Closing Dashboard cancels pending dispatch work but
cannot undo commands already sent. Results are never broadcast to other dashboards.
Server restart loses pending operations. Existing agent registration identity and
trust model are unchanged.

## Audit and verification

Existing `frontend_request` audit entries include attempted power commands.
Accepted operations also record `power.shutdown.requested` /
`power.shutdown_room.requested` and final `power.shutdown.result` /
`power.shutdown_room.result` through the existing `logs` mechanism. These contain
the originating user, request/target IDs and outcome counts, without agent-supplied
message text. Disconnect-cancelled frontend requests have no final result event.
Audit failures are logged without rejecting commands.

Run `go test ./...`; the power integration tests use real local WebSocket connections
with fake session/agent stores and mock agent responses. They do not power off any
machine. Run `go test -race ./internal/wsserver` where a cgo C compiler is available.
Optional database tests use `POWER_TEST_DATABASE_URL` pointing to a disposable
PostgreSQL database; they use connection-local temporary tables.

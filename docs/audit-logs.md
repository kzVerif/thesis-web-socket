# WebSocket audit logs

Authenticated frontend messages create `frontend_request` rows in the existing
`logs` table with the session user, peer IP, target agent (when it exists), and
bounded metadata: type/action, IDs, PID, scan type, limit and target count.
`phase=requested` records an attempt, not a successful operation; denied and
invalid requests are included after session authentication. Cookies, tokens,
paths and arbitrary payloads are excluded. Peer IP is the direct connection IP;
forwarding headers are not trusted.

Existing `job_created` and `virus_scan_requested` rows identify created jobs.
Agent response logs are limited to matched `process_kill_result`, final
`virus_scan_result`, and final `file_download_result` summaries. Duplicate final
scan/download responses do not create more rows. Scan reports remain in
`av_scan_results`, rather than being copied into logs. Process outcomes include
PID and success; scan/download outcomes include IDs and status for lookup.

Screen frames, performance samples, process lists, heartbeats and progress do
not create audit rows. Download progress still updates its existing target row.
No schema migration is needed. Frontend/process audit write failures are reported
to the server logger without interrupting WebSocket traffic; final scan/download
logs are written atomically with their result updates. No automatic deletion or
retention policy is introduced.

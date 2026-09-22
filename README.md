# WebSocket agent server

Agent /ws now requires Ed25519 challenge-response before ONLINE or registry:
[authentication protocol and deployment](docs/agent-authentication.md).

HTTPS/WSS deployment: [transport security](docs/transport-security.md) and
[same-host Cloudflare example](deploy/cloudflared.example.yml).

ระบบรับ agent ผ่าน WebSocket ที่ `/ws` และเก็บสถานะ `ONLINE`/`OFFLINE` ใน PostgreSQL

## Project structure

```text
.
├── main.go                            # ประกอบ dependencies และเริ่ม HTTP server
└── internal
    ├── config/config.go               # environment configuration
    ├── database/postgres.go           # เปิด database และตรวจ schema
    ├── model/agent.go                 # agent model และ JSON contract
    ├── repository/agent_repository.go # database queries
    └── wsserver
        ├── server.go                  # WebSocket lifecycle และ handler
        ├── registry.go                # online client registry
        ├── heartbeat.go               # connection health check
        └── registry_test.go           # registry tests
```

## Configuration

ใช้ฐานข้อมูลเดียวกับ REST Server และ schema ที่ผู้ดูแลตรวจสอบกับ migrations ปัจจุบันแล้ว
Local repositories ไม่มี production `schema.sql` bootstrap ครบชุด
แอปพลิเคชันไม่สร้างหรือแก้ schema ขณะ runtime

ต้องกำหนด `DATABASE_URL` ผ่าน process environment หรือไฟล์ `.env` ที่ไม่ commit
`godotenv.Load` โหลด `.env` โดยไม่ทับค่าจาก environment; ไม่มี password fallback ใน source
เมื่อไม่ได้กำหนดจะหยุดพร้อม configuration error และไม่พิมพ์ DSN ใน startup error

```powershell
$env:SERVER_ADDRESS = ":8081"
$env:DATABASE_URL = "user=postgres password=your-password dbname=ratsystem sslmode=disable"
go run .
```

## Agent registration JSON

Client ต้องส่ง JSON แรกหลังเชื่อมต่อดังนี้:

```json
{
  "id": "86dbfbcc-a13c-4ffc-adab-7187018273e0",
  "hostname": "kanghunz",
  "os_info": {
    "name": "windows",
    "edition": "amd64",
    "version": "11"
  },
  "ip_address": "192.168.1.136",
  "mac_address": "90:e8:68:d0:16:f9"
}
```

เมื่อเชื่อมต่อสำเร็จ status จะเป็น `ONLINE` และ `last_seen` จะถูกอัปเดตเป็นเวลาปัจจุบัน เมื่อปิดหรือ heartbeat ล้มเหลว status จะเป็น `OFFLINE` โดยเก็บค่า `last_seen` ล่าสุดไว้

## Security boundary และงาน Phase 4

Phase 1 ไม่เปลี่ยน JSON/WebSocket protocol ปัจจุบัน Server ตรวจว่ามี Agent ID
ในฐานข้อมูลแล้วตั้ง ONLINE และเพิ่ม connection เข้า registry โดยยังไม่พิสูจน์ private key
ผู้ที่อ้าง ID เดิมจึงอาจแทนที่ connection เดิมได้; enrollment token และ `/exists`
ไม่แก้ข้อจำกัดนี้

Phase 4 ต้องออกแบบการโหลด public key ใน
`internal/repository/agent_repository.go:GetByID` และ model/credential type แยกที่เหมาะสม
โดยไม่เผลอเพิ่ม field ใน JSON ที่ส่งให้ frontend แล้วตรวจ challenge-response ใน
`internal/wsserver/server.go:HandleWebSocket` **ก่อน** `setStatus(ONLINE)`,
`registry.Add`, การปิด connection เดิม และการเริ่ม feature streams
ต้องรักษา `internal/wsserver/registry.go:Remove` ที่ตรวจ connection instance
และเพิ่ม tests ว่า connection ที่ยังไม่ผ่าน proof แทนที่หรือตัด connection ที่ถูกต้องไม่ได้
Phase 1 ยังไม่เพิ่ม challenge, Sign/Verify หรือเปลี่ยน feature protocols

## Verify

Protocol สำหรับเชื่อม Next.js:

- [`docs/frontend-performance.md`](docs/frontend-performance.md)
- [`docs/frontend-process.md`](docs/frontend-process.md)
- [`docs/file-distribution-agent.md`](docs/file-distribution-agent.md)
- [`docs/file-distribution-frontend.md`](docs/file-distribution-frontend.md)
- [`docs/frontend-screen.md`](docs/frontend-screen.md)
- [`docs/frontend-virus-scan.md`](docs/frontend-virus-scan.md)
- [`docs/virus-scan.md`](docs/virus-scan.md)
- [`docs/power.md`](docs/power.md) — Mock shutdown single agent / room (Phase 1)

Virus scan jobs: for an existing database, apply
[`migrations/20260905_add_av_jobs.sql`](migrations/20260905_add_av_jobs.sql)
before starting this version. A fresh `schema.sql` already includes `av_jobs`;
do not apply this migration again after loading the fresh schema.

```powershell
go test ./...
go vet ./...
```
"# thesis-web-socket" 

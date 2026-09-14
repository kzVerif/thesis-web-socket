# WebSocket agent server

ระบบรับ agent ผ่าน WebSocket ที่ `/ws` และเก็บสถานะ `ONLINE`/`OFFLINE` ใน PostgreSQL

## Project structure

```text
.
├── main.go                            # ประกอบ dependencies และเริ่ม HTTP server
├── schema.sql                         # PostgreSQL schema
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

โครงสร้างฐานข้อมูลอ้างอิงจาก `schema.sql` เท่านั้น แอปพลิเคชันจะไม่สร้างตาราง เพิ่มคอลัมน์ หรือแก้ schema ขณะ runtime ผู้ดูแลระบบต้อง apply `schema.sql` ก่อนเริ่ม server

```powershell
$env:SERVER_ADDRESS = ":8080"
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

## Verify

Protocol สำหรับเชื่อม Next.js:

- [`docs/frontend-performance.md`](docs/frontend-performance.md)
- [`docs/frontend-process.md`](docs/frontend-process.md)
- [`docs/file-distribution-agent.md`](docs/file-distribution-agent.md)
- [`docs/file-distribution-frontend.md`](docs/file-distribution-frontend.md)
- [`docs/frontend-screen.md`](docs/frontend-screen.md)
- [`docs/frontend-virus-scan.md`](docs/frontend-virus-scan.md)
- [`docs/virus-scan.md`](docs/virus-scan.md)

Virus scan jobs: for an existing database, apply
[`migrations/20260905_add_av_jobs.sql`](migrations/20260905_add_av_jobs.sql)
before starting this version. A fresh `schema.sql` already includes `av_jobs`;
do not apply this migration again after loading the fresh schema.

```powershell
go test ./...
go vet ./...
```
"# thesis-web-socket" 

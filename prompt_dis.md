คุณเป็น Senior Backend Engineer ผู้เชี่ยวชาญ Go, WebSocket, PostgreSQL และ Distributed Systems

จงพัฒนาระบบ "File Distribution" สำหรับ WebSocket Server ของระบบ Remote Administration

Tech Stack หลัก:

- Go
- PostgreSQL
- WebSocket
- Frontend: Next.js
- Agent: Go

ระบบมี Agent หลายเครื่องเชื่อมต่อกับ WebSocket Server อยู่แล้ว

แต่ละ Agent มี UUID และสามารถอยู่ใน Room ได้

Frontend ต้องสามารถสั่งกระจายไฟล์ได้ 2 รูปแบบ:

1. ส่งไปยังทุก Agent ใน Room
2. เลือก Agent หลายเครื่องหรือเครื่องเดียวโดยตรง

ตัวไฟล์ห้ามส่งผ่าน WebSocket

Agent ต้อง Download ผ่าน HTTPS Temporary URL

WebSocket ใช้สำหรับ:

- Command
- Status
- Progress
- Result

---

# สำคัญมาก: Database Schema

ภายใน project มีไฟล์:

```text
schema.sql
```

ให้อ่านไฟล์นี้ทั้งหมดก่อนเขียน Code

schema.sql เป็น Source of Truth

ห้ามเดาชื่อ:

- table
- column
- relation
- enum/check constraint
- foreign key

ให้ตรวจสอบก่อนว่ามี table ที่เกี่ยวข้องกับ:

```text
agents
rooms
files
commands
users
logs
```

รวมถึง table อื่นทั้งหมด

ใช้ table เดิมให้มากที่สุด

ห้ามสร้าง table ใหม่ที่ทำหน้าที่ซ้ำกับ schema เดิม

ถ้า schema เดิมยังไม่มีโครงสร้างที่สามารถเก็บ File Distribution Job และผลราย Agent ได้ ให้สร้าง SQL migration เพิ่มต่างหาก

ห้ามแก้ schema.sql ต้นฉบับแบบทำลายข้อมูล

---

# Database Design สำหรับ File Distribution

ก่อนสร้าง migration ให้ตรวจ schema เดิมก่อน

ระบบจำเป็นต้องสามารถตอบคำถามได้ว่า:

```text
ไฟล์อะไร
ใครเป็นคนสั่ง
สั่งเมื่อไหร่
ส่งไปห้องไหน หรือ Agent ไหน
มีเป้าหมายทั้งหมดกี่เครื่อง
Agent ไหนสำเร็จ
Agent ไหนล้มเหลว
Agent ไหน Offline
แต่ละ Agent download ไปกี่ %
Error คืออะไร
เริ่มเมื่อไหร่
สำเร็จเมื่อไหร่
```

ถ้า schema ปัจจุบันยังรองรับไม่ได้ ให้เพิ่มแนวคิดประมาณ:

```text
file_distribution_jobs
file_distribution_targets
```

แต่ชื่อจริงให้ตั้งตาม naming convention ของ schema.sql

ตัวอย่าง concept:

```text
file_distribution_jobs

id
file_id
requested_by
target_type
room_id
status
total_targets
completed_targets
failed_targets
created_at
updated_at
completed_at
```

และ:

```text
file_distribution_targets

id
job_id
agent_id
status
progress
downloaded_bytes
error_code
error_message
started_at
completed_at
updated_at
```

ต้องมี unique constraint:

```text
(job_id, agent_id)
```

เพื่อป้องกัน duplicate target

ถ้า `commands` table เดิมสามารถทำหน้าที่นี้ได้อยู่แล้ว ให้ reuse แทนการสร้าง table ซ้ำ

อธิบายเหตุผลก่อนเลือกแนวทาง

---

# Job Status

Job status อย่างน้อย:

```text
PENDING
DISPATCHING
IN_PROGRESS
COMPLETED
PARTIAL_FAILED
FAILED
CANCELLED
```

Target status:

```text
PENDING
SENT
DOWNLOADING
VERIFYING
COMPLETED
FAILED
OFFLINE
CANCELLED
```

หาก schema ใช้ naming/status อื่นอยู่แล้วให้ปรับเข้ากับของเดิม

---

# Request จาก Frontend

รองรับ message จาก Frontend:

## แจกทั้ง Room

```json
{
  "type": "FILE_DISTRIBUTE",
  "file_id": "uuid",
  "target": {
    "type": "ROOM",
    "room_id": "uuid"
  }
}
```

## เลือก Agent

```json
{
  "type": "FILE_DISTRIBUTE",
  "file_id": "uuid",
  "target": {
    "type": "AGENTS",
    "agent_ids": [
      "uuid-1",
      "uuid-2",
      "uuid-3"
    ]
  }
}
```

ต้อง validate UUID ทุกตัว

ห้ามเชื่อข้อมูลจาก Frontend โดยตรง

---

# Authentication / Authorization

Frontend WebSocket connection ต้องถูก authenticate ตามระบบ auth เดิมของ project

อ่าน schema และ source code ก่อน

User ที่สั่งกระจายไฟล์ต้องมี permission ที่เหมาะสม

ถ้าระบบ permission เดิมมี code สำหรับ file distribution ให้ใช้ของเดิม

ถ้ายังไม่มีให้เสนอ permission เช่น:

```text
files.distribute
```

แต่ห้ามเพิ่มโดยไม่ตรวจ schema/seed เดิมก่อน

Server ต้องรู้ว่า:

```text
requested_by = user UUID
```

จาก authenticated session/token

ห้ามรับ user_id จาก JSON แล้วเชื่อทันที

---

# File Validation

เมื่อ Frontend ส่ง file_id:

Query database เพื่อหาไฟล์

ตรวจว่า:

- file มีอยู่จริง
- file พร้อม download
- file ไม่ถูก delete
- storage object/path มีอยู่
- size ถูกต้อง
- SHA-256 มีค่า
- ผ่าน policy ที่ระบบกำหนด

ถ้ามี `av_scan_results` หรือระบบ antivirus ใน schema เดิม:

ให้ตรวจผล scan ก่อนอนุญาตกระจายไฟล์

ถ้า schema มีสถานะเช่น:

```text
CLEAN
INFECTED
PENDING
FAILED
```

ให้ใช้ค่าจริงจาก schema

ห้ามแจกไฟล์ที่ถูก mark ว่าอันตรายหรือยังไม่ผ่านเงื่อนไขที่ระบบกำหนด

---

# Resolve Target — ROOM

ถ้า:

```json
{
  "type": "ROOM",
  "room_id": "..."
}
```

Server ต้อง query Agent ทั้งหมดใน Room นั้นจาก Database

สำคัญ:

Snapshot รายชื่อ Agent ตอนสร้าง Job

ตัวอย่าง:

Room A ตอนกดแจกไฟล์มี:

```text
Agent 1
Agent 2
Agent 3
```

หลังจากนั้น Agent 4 ถูกย้ายเข้าห้อง

Job เดิมไม่ควรเพิ่ม Agent 4 ตามมาทีหลัง

ดังนั้นต้องสร้าง target records ตอนสร้าง Job

---

# Resolve Target — AGENTS

ถ้าเลือก Agent:

ตรวจทุก UUID ว่ามีอยู่จริง

deduplicate agent_ids ก่อน

เช่น:

```text
A
A
B
```

ต้องกลายเป็น:

```text
A
B
```

---

# Online / Offline

WebSocket Server มี Connection Manager อยู่แล้วหรือให้ตรวจ source code ก่อน

ควรมี concept:

```go
map[AgentID]*AgentConnection
```

หรือ architecture ที่ equivalent

Server ต้องตรวจ target แต่ละ Agent:

ถ้า connected:

```text
PENDING -> SENT
```

ถ้าไม่ connected:

```text
PENDING -> OFFLINE
```

เก็บ OFFLINE ลง database

ห้ามทำให้ Job ทั้งก้อน fail เพียงเพราะบาง Agent offline

ตัวอย่าง:

```text
Target = 30

Completed = 25
Failed    = 2
Offline   = 3

Job = PARTIAL_FAILED
```

---

# Temporary Download URL

สำหรับ Agent ที่ online:

Server ต้องสร้าง Temporary Download URL

ตัว URL ต้องหมดอายุ

แนะนำ:

```text
10 นาที
```

แต่ทำเป็น config

เช่น:

```text
https://server.example.com/files/download/{token}
```

ตัว token ต้อง cryptographically secure

Temporary token ต้องผูกอย่างน้อยกับ:

```text
file_id
agent_id
job_id
expires_at
```

Agent A ต้องไม่สามารถใช้ URL ที่ออกให้ Agent B ได้

---

# Temporary Token Security

ห้ามใช้:

```text
base64(file_id)
```

เป็น authentication

ให้ใช้:

- signed token
หรือ
- random opaque token stored server-side

ถ้าใช้ signed token:

ใช้ HMAC-SHA256 หรือ mechanism ที่เหมาะสม

secret ต้องมาจาก environment/config

ห้าม hardcode

Token ต้องตรวจ:

```text
signature
expires_at
file_id
agent_id
job_id
```

ถ้าเป็นไปได้ให้เป็น single-purpose token

---

# HTTP Download Endpoint

แม้ชื่อระบบคือ WebSocket Server แต่ตัวไฟล์ต้องให้ Agent โหลดผ่าน HTTP/HTTPS endpoint

เช่น:

```text
GET /files/download/{token}
```

WebSocket process สามารถ serve HTTP endpoint นี้ได้ถ้า architecture ปัจจุบันเหมาะสม

หรือถ้า project มี API/File Server แยกอยู่แล้ว:

ให้ WebSocket Server generate/ขอ temporary token แล้ว URL ชี้ไปยัง File Server

อย่าฝืน architecture

ตรวจ project ก่อน

---

# File Streaming

HTTP endpoint ต้อง stream file

ห้าม:

```go
os.ReadFile(largeFile)
```

แล้วส่งทั้งก้อน

ใช้ streaming เช่น:

```go
io.Copy
```

เพื่อไม่ให้ RAM เพิ่มตามขนาดไฟล์

ตั้ง headers ที่เหมาะสม:

```text
Content-Type
Content-Length
Content-Disposition
```

filename ต้อง sanitize

---

# Agent Command

หลังสร้าง Temporary URL แล้วส่ง Agent:

```json
{
  "type": "DOWNLOAD_FILE",
  "job_id": "uuid",
  "file_id": "uuid",
  "filename": "example.zip",
  "size": 58382912,
  "sha256": "abcdef1234567890...",
  "download_url": "https://server.example.com/files/download/token",
  "expires_at": "2026-09-01T15:30:00Z"
}
```

ส่งผ่าน existing Agent WebSocket connection

---

# สำคัญ: Temporary URL ต่อ Agent

อย่าสร้าง URL เดียวแล้วส่งให้ Agent ทุกเครื่อง

ควร generate token แยก:

```text
Job X

Agent A -> Token A
Agent B -> Token B
Agent C -> Token C
```

เพื่อ:

- revoke รายเครื่องได้
- audit ได้
- ป้องกัน URL แชร์ข้าม Agent
- ตรวจสิทธิ์ download ได้

---

# รับ Progress จาก Agent

รองรับ:

```json
{
  "type": "FILE_DOWNLOAD_PROGRESS",
  "job_id": "uuid",
  "agent_id": "uuid",
  "downloaded_bytes": 10485760,
  "total_bytes": 58382912,
  "progress": 17
}
```

Server ต้องตรวจว่า:

```text
WebSocket connection นี้คือ Agent ไหน
```

ห้ามเชื่อ `agent_id` ใน JSON เพียงอย่างเดียว

ต้อง verify:

```text
connection.agentID == message.agent_id
```

และตรวจว่า Agent เป็น target ของ job นี้จริง

---

# Progress Database Write

อย่า update PostgreSQL ทุก frame/chunk

Agent อาจส่ง progress ทุก 1 วินาที

ถ้ามี Agent 500 เครื่องจะทำให้ DB write เยอะ

ออกแบบให้เหมาะสม เช่น:

- update เมื่อ progress เปลี่ยน >= 5%
- หรือ debounce
- หรือ batch update
- หรือเก็บ latest progress ใน memory และ flush เป็นช่วง

แต่สถานะสำคัญ:

```text
DOWNLOADING
VERIFYING
COMPLETED
FAILED
```

ให้ persist ลง DB ทันที

อธิบาย tradeoff ที่เลือก

---

# Forward Progress ไป Frontend

WebSocket Server ต้องสามารถส่ง realtime update ไป Frontend

เช่น:

```json
{
  "type": "FILE_DISTRIBUTION_TARGET_UPDATE",
  "job_id": "uuid",
  "agent_id": "uuid",
  "hostname": "PC-001",
  "status": "DOWNLOADING",
  "progress": 72,
  "downloaded_bytes": 41943040,
  "total_bytes": 58382912
}
```

Frontend จะใช้ทำ Dashboard

---

# Agent Completed

รับ:

```json
{
  "type": "FILE_DOWNLOAD_RESULT",
  "job_id": "uuid",
  "agent_id": "uuid",
  "file_id": "uuid",
  "status": "COMPLETED",
  "bytes_downloaded": 58382912,
  "sha256": "abcdef..."
}
```

Server ต้อง:

1. Authenticate Agent connection
2. ตรวจ job
3. ตรวจ target
4. ตรวจ file_id
5. update target = COMPLETED
6. progress = 100
7. completed_at = NOW()
8. update aggregate job status
9. notify Frontend

---

# Agent Failed

ตัวอย่าง:

```json
{
  "type": "FILE_DOWNLOAD_RESULT",
  "job_id": "uuid",
  "agent_id": "uuid",
  "file_id": "uuid",
  "status": "FAILED",
  "error_code": "HASH_MISMATCH",
  "error_message": "downloaded file checksum does not match"
}
```

Server update:

```text
status = FAILED
error_code
error_message
completed_at
```

แล้ว notify Frontend

---

# Aggregate Job Status

สร้าง function กลาง เช่น concept:

```go
RecalculateDistributionJobStatus(jobID)
```

ตัวอย่าง logic:

ถ้ายังมี:

```text
PENDING
SENT
DOWNLOADING
VERIFYING
```

อย่างน้อยหนึ่ง target:

```text
IN_PROGRESS
```

ถ้าทั้งหมด COMPLETED:

```text
COMPLETED
```

ถ้ามีทั้ง COMPLETED และ FAILED/OFFLINE:

```text
PARTIAL_FAILED
```

ถ้าไม่มีเครื่องใดสำเร็จ:

```text
FAILED
```

ปรับ logic ให้เข้ากับ schema/status จริง

---

# Frontend Initial Response

เมื่อสร้าง Job สำเร็จให้ตอบทันที:

```json
{
  "type": "FILE_DISTRIBUTION_CREATED",
  "job_id": "uuid",
  "file_id": "uuid",
  "total_targets": 30,
  "online_targets": 27,
  "offline_targets": 3,
  "status": "IN_PROGRESS"
}
```

Frontend ไม่ต้องรอทั้ง 30 เครื่อง download เสร็จ

---

# Retry Failed

ออกแบบ architecture ให้ในอนาคตรองรับ:

```text
Retry Failed
```

เช่น Frontend ส่ง:

```json
{
  "type": "FILE_DISTRIBUTION_RETRY",
  "job_id": "uuid",
  "targets": "FAILED"
}
```

ไม่จำเป็นต้อง implement UI

แต่ backend architecture ต้องไม่ปิดทาง

เมื่อ retry ต้อง generate Temporary URL ใหม่

ห้าม reuse URL หมดอายุ

---

# Agent Disconnect

ถ้า Agent disconnect ระหว่าง:

```text
DOWNLOADING
```

อย่า mark FAILED ทันที

เพราะ HTTP download อาจยังทำงานอยู่

ให้ใช้ strategy เช่น:

```text
connection lost
       |
       v
WAITING / connection uncertain
       |
       | timeout
       v
FAILED
```

หรือใช้ timeout mechanism ที่เข้ากับ architecture เดิม

อย่าสร้าง status ใหม่ถ้า schema ไม่รองรับโดยไม่จำเป็น

อย่างน้อยต้องหลีกเลี่ยง false failure

---

# Idempotency

Frontend อาจส่ง request ซ้ำเพราะ reconnect

รองรับ `request_id` ถ้า architecture เหมาะสม:

```json
{
  "type": "FILE_DISTRIBUTE",
  "request_id": "uuid",
  ...
}
```

Server ไม่ควรสร้าง Job ซ้ำสำหรับ request เดิม

ถ้า schema รองรับ idempotency key ให้ใช้

ถ้ายังไม่รองรับให้เสนอ migration

---

# Transaction

ตอนสร้าง Job:

ควรทำ transaction:

```text
BEGIN

create distribution job

resolve target agents

insert distribution targets

COMMIT
```

หลัง COMMIT ค่อย dispatch WebSocket command

ห้ามถือ DB transaction เปิดไว้ระหว่าง network operation

ผิด:

```text
BEGIN
send websocket
wait response
COMMIT
```

---

# Architecture

แยก responsibility เช่น:

```text
internal/
├── distribution/
│   ├── service.go
│   ├── repository.go
│   ├── dispatcher.go
│   ├── status.go
│   └── models.go
│
├── websocket/
│   ├── hub.go
│   ├── agent_handler.go
│   ├── dashboard_handler.go
│   └── messages.go
│
├── downloadtoken/
│   ├── service.go
│   └── token.go
│
├── files/
│   └── http_handler.go
│
└── database/
```

เป็นเพียงตัวอย่าง

ต้องอ่าน architecture เดิมก่อน

ถ้ามี service/repository/hub อยู่แล้วให้ reuse

ห้าม restructure project ครั้งใหญ่โดยไม่จำเป็น

---

# WebSocket Connection Manager

ต้องสามารถหา Agent connection ด้วย UUID ได้อย่าง thread-safe

เช่น concept:

```go
type AgentHub struct {
    mu     sync.RWMutex
    agents map[uuid.UUID]*AgentConnection
}
```

ต้องมี method concept:

```go
GetAgent(agentID)
RegisterAgent(...)
UnregisterAgent(...)
IsOnline(agentID)
```

reuse implementation เดิมถ้ามี

---

# Backpressure

ห้ามให้ Agent ที่ช้าทำให้ Server block ทั้ง Hub

แต่ละ connection ควรมี bounded send queue

ถ้า queue เต็ม:

จัดการอย่างชัดเจน

ห้ามมี unbounded channel

---

# Security

ต้องมี:

- authenticated Dashboard
- authenticated Agent
- authorization ก่อนแจกไฟล์
- temporary signed/opaque token
- token expiration
- token ผูก agent_id
- token ผูก file_id
- token ผูก job_id
- HTTPS production
- SHA-256
- path validation
- no WebSocket binary file transfer
- parameterized SQL
- transaction
- safe error responses

ห้ามส่ง storage filesystem path จริงไป Agent

ผิด:

```text
D:\storage\files\123.exe
```

Agent ต้องเห็นแค่ HTTPS URL

---

# Logging / Audit

Log:

```text
job_created
target_resolved
command_dispatched
agent_offline
download_started
download_completed
download_failed
job_completed
```

ถ้า schema มี `logs` หรือ audit table ให้ตรวจและ reuse

อย่า log:

- full temporary token
- password
- auth token
- session secret

---

# Database Query Efficiency

หลีกเลี่ยง N+1 query

สำหรับ Room:

อย่า:

```text
query agents
loop 100 agents
query agent ทีละตัว
```

ให้ query targets เป็นชุด

INSERT targets แบบ batch ถ้าเหมาะสม

---

# Tests

ต้องมี tests อย่างน้อย:

1. distribute ไป Agent เดียว
2. distribute หลาย Agent
3. distribute ทั้ง Room
4. Room ไม่มี Agent
5. Agent offline
6. Agent UUID ซ้ำ
7. invalid file
8. unauthorized user
9. temporary token valid
10. temporary token expired
11. Agent A ใช้ token Agent B
12. progress update
13. completed
14. hash mismatch result
15. partial failure
16. all completed
17. all failed
18. duplicate frontend request
19. concurrent updates
20. SQL transaction rollback

ใช้ race detector ถ้า environment รองรับ:

```bash
go test -race ./...
```

---

# Database Migration

ถ้าต้องเพิ่ม table:

สร้าง migration file ใหม่

เช่น:

```text
migrations/
xxxx_add_file_distribution.sql
```

อย่า rewrite schema เดิมแบบ destructive

Migration ต้องมี:

- PK
- FK
- indexes
- unique constraints
- CHECK constraints ถ้า project ใช้ pattern นี้
- timestamps

ควร index อย่างน้อย field ที่ query บ่อย เช่น:

```text
job_id
agent_id
status
created_at
```

แต่ตรวจ schema จริงก่อน

---

# สิ่งที่ต้องส่งมอบ

ก่อนเขียน Code ให้สรุปสั้น ๆ:

```text
Existing schema
Existing relevant tables
Missing capability
Chosen design
```

จากนั้น implement จริง

สุดท้ายแสดง:

1. Database tables ที่ reuse
2. Migration ที่เพิ่ม ถ้ามี
3. Architecture
4. WebSocket protocol
5. Temporary URL mechanism
6. Frontend -> WS flow
7. WS -> Agent flow
8. Agent -> HTTP download flow
9. Agent -> WS progress/result flow
10. Database updates
11. Error handling
12. Source code
13. Test
14. วิธี run
15. ตัวอย่างการทดสอบ end-to-end

ห้ามตอบแค่ pseudo-code

ต้องเขียน Go code ที่ compile ได้จริง

ก่อนจบให้รัน:

```bash
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
go build ./...
```

ถ้าคำสั่งใดใช้ไม่ได้เพราะ environment ให้ระบุเหตุผล

แต่ต้องแก้ compile/test errors ที่เกิดจาก code ที่เขียนเองให้หมด
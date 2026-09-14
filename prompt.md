สร้าง WebSocket Server ด้วยภาษา Go สำหรับระบบ Remote Administration ที่ใช้รับ Screen Frame จาก Agent แล้ว Relay ไปยัง Next.js Dashboard แบบ realtime

Server ทำหน้าที่เป็น:

```text
Agent
    ↓
WebSocket Binary JPEG
    ↓
Go WebSocket Server
    ↓
WebSocket Binary JPEG
    ↓
Next.js Dashboard
```

Server **ห้ามบันทึกภาพหน้าจอ**

ห้าม:

```text
Save JPEG ลง Disk
Save JPEG ลง Database
Upload JPEG ลง Object Storage
เก็บ Screen History
```

Server สามารถถือ Frame ชั่วคราวใน RAM เพื่อ Relay ได้เท่านั้น

## Architecture

ระบบมี WebSocket Client สองประเภท:

```text
Agent
Dashboard Viewer
```

Server ต้องรู้ว่า:

```text
Agent connection ไหน = Agent ID อะไร
Viewer คนไหนกำลังดู Agent ตัวไหน
```

ตัวอย่าง:

```text
agent-001

Agent Connection
    ↓
WS Server
    ├── Viewer A
    └── Viewer B
```

## Dashboard Request

เมื่อ Dashboard ต้องการดูเครื่อง:

```json
{
  "type": "watch_screen",
  "agent_id": "agent-001"
}
```

Server เพิ่ม Dashboard connection เข้า Viewer List ของ:

```text
agent-001
```

ถ้า Viewer เปลี่ยนจาก:

```text
0 → 1
```

ให้ Server ส่ง Agent:

```json
{
  "type": "start_stream"
}
```

Agent จึงเริ่ม Capture Screen

## Viewer Count

Server ต้อง maintain:

```text
agentID → viewers
```

ตัวอย่าง:

```go
map[string]map[*Viewer]struct{}
```

หรือออกแบบด้วย struct ที่ thread-safe

เมื่อ Viewer ปิดหน้า Dashboard หรือ WebSocket disconnect:

ให้ลบ Viewer ออกจาก Agent นั้น

ถ้า Viewer count เปลี่ยนเป็น:

```text
1 → 0
```

ให้ส่ง:

```json
{
  "type": "stop_stream"
}
```

ไปยัง Agent

ดังนั้น Agent จะไม่ Capture หน้าจอถ้าไม่มีใครดู

## Screen Frame

Agent ส่ง Screen Frame เป็น:

```text
WebSocket Binary Message
```

ข้อมูลคือ JPEG bytes

ตัวอย่าง:

```text
Agent
↓
Binary Frame
↓
Server
```

Server ไม่ Decode JPEG ถ้าไม่จำเป็น

Server แค่:

```text
receive []byte
↓
forward []byte
```

เพื่อลด CPU usage

## Latest Frame Wins

ระบบต้องออกแบบสำหรับ realtime

ห้ามสร้าง queue แบบไม่จำกัด:

```text
Frame 100
Frame 101
Frame 102
Frame 103
Frame 104
...
```

เพราะถ้า Browser ช้า จะเกิด latency สะสม

ให้แต่ละ Viewer มี queue ขนาดประมาณ:

```text
1 frame
```

ถ้า Viewer ยังส่ง Frame 100 ไม่เสร็จ แต่ Server ได้:

```text
101
102
103
```

ไม่ต้องรอส่งทั้งหมด

สามารถทิ้ง:

```text
101
102
```

แล้วรักษา:

```text
103
```

ซึ่งเป็น Frame ล่าสุด

เป้าหมายคือ:

```text
Latest Frame Wins
```

ไม่ใช่ Guaranteed Frame Delivery

## Slow Viewer

Viewer ที่ Network ช้าต้องไม่สามารถ block Agent หรือ Viewer คนอื่นได้

ห้ามทำ:

```go
for _, viewer := range viewers {
    viewer.Write(frame)
}
```

ถ้า `Write()` เป็น blocking operation เพราะ Viewer คนเดียวที่ช้าจะทำให้ทั้ง loop ช้า

ให้แต่ละ Viewer มี send goroutine ของตัวเอง

แนวคิด:

```text
Agent Frame
    ↓
Server
    ├── viewer A queue [1]
    ├── viewer B queue [1]
    └── viewer C queue [1]
```

ตัวอย่างแนวคิด channel:

```go
type Viewer struct {
    send chan []byte
}
```

ขนาด:

```go
make(chan []byte, 1)
```

เมื่อ Frame ใหม่เข้ามาและ buffer เต็ม:

```text
drop old frame
↓
insert latest frame
```

## Memory

Server ต้องหลีกเลี่ยง Copy JPEG หลายครั้งโดยไม่จำเป็น

JPEG frame เดียวสามารถ share underlying `[]byte` เพื่อ broadcast ให้ Viewer ได้ หากไม่มีส่วนใดแก้ไข byte slice นั้น

Frame จะถูก GC หลังจากไม่มี reference แล้ว

Server ไม่ต้องทำ:

```go
os.WriteFile()
```

หรือ:

```go
database.Insert()
```

## Agent Registry

สร้าง Registry เช่น:

```go
type Agent struct {
    ID   string
    Conn *websocket.Conn
}
```

และ:

```go
type Hub struct {
    agents  map[string]*Agent
    viewers map[string]map[*Viewer]struct{}
}
```

ต้องรองรับ concurrent access อย่างปลอดภัยด้วย:

```text
sync.RWMutex
```

หรือ event-loop Hub architecture

## Disconnect Handling

ถ้า Agent disconnect:

Server ต้อง:

```text
mark agent offline
↓
remove agent connection
↓
แจ้ง Dashboard
↓
cleanup viewers/stream state
```

ถ้า Dashboard disconnect:

```text
remove viewer
↓
check viewer count
↓
ถ้าเหลือ 0 → STOP_STREAM
```

## Suggested Message Types

Agent → Server:

```json
{
  "type": "agent_register",
  "agent_id": "agent-001"
}
```

Agent → Server:

```text
Binary JPEG Frame
```

Server → Agent:

```json
{
  "type": "start_stream"
}
```

Server → Agent:

```json
{
  "type": "stop_stream"
}
```

Dashboard → Server:

```json
{
  "type": "watch_screen",
  "agent_id": "agent-001"
}
```

Dashboard → Server:

```json
{
  "type": "unwatch_screen",
  "agent_id": "agent-001"
}
```

Server → Dashboard สามารถมี:

```json
{
  "type": "stream_started",
  "agent_id": "agent-001"
}
```

และ Binary JPEG Frames ตามมา

## Security

ต้องตรวจสอบ Authentication ของ Dashboard ก่อนอนุญาต:

```text
watch_screen
```

และตรวจสอบ permission เช่น:

```text
agents.screen.read
```

Dashboard ห้ามสามารถดู Agent ที่ไม่มีสิทธิ์ได้

Agent connection ต้องมี authentication/token ของ Agent ด้วย

ห้ามเชื่อ `agent_id` ที่ Client ส่งมาอย่างเดียวโดยไม่มีการ verify identity

## Limits

เพิ่ม configuration เช่น:

```text
Max viewers per Agent: 3
Max JPEG frame size: 500 KB
Stream FPS expected: <= 5
Write timeout: 2-5 seconds
Read limit
Ping/Pong heartbeat
```

ถ้า Agent ส่ง Frame ใหญ่ผิดปกติให้ reject เพื่อป้องกัน Server memory abuse

## Code Structure

จัดโครงสร้างประมาณ:

```text
server/
├── main.go
├── websocket/
│   ├── hub.go
│   ├── agent.go
│   ├── viewer.go
│   ├── handler.go
│   └── messages.go
│
├── auth/
│   └── auth.go
│
└── config/
    └── config.go
```

## สิ่งที่ต้องการจากคำตอบ

สร้าง implementation ตัวอย่างที่ใช้งานได้จริง โดยแสดง:

1. Agent WebSocket connection
2. Dashboard WebSocket connection
3. Agent Registry
4. Viewer Registry
5. watch_screen
6. unwatch_screen
7. START_STREAM
8. STOP_STREAM
9. รับ JPEG Binary จาก Agent
10. Relay Binary ไป Dashboard
11. Latest-frame-wins
12. queue ต่อ Viewer ขนาด 1
13. slow viewer handling
14. disconnect cleanup
15. Ping/Pong
16. authentication structure
17. permission checking
18. concurrent-safe architecture
19. graceful shutdown

Server ต้องเป็น **realtime relay server** ไม่ใช่ Screen Recording Server

เน้นให้ CPU, RAM และ Network overhead ต่ำ รองรับหลาย Agent ได้ และไม่ให้ slow Dashboard หนึ่งเครื่องกระทบ Agent หรือ Dashboard เครื่องอื่น

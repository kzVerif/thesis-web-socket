# สั่งสแกนไวรัสจากหน้าบ้าน

All clients must authenticate using the __Host-session cookie on /ws/frontend. Bearer tokens are not accepted. Use HTTPS/WSS and configure FRONTEND_ORIGINS.

ต้องเป็นผู้ใช้ ACTIVE มี session ที่ยังไม่หมดอายุ/ไม่ถูก revoke ตอนเชื่อมต่อ และมี permission `av.scan` สำหรับคำสั่งสแกนและประวัติ ระบบตรวจ permission และสถานะผู้ใช้อีกครั้งทุกคำสั่ง ประวัติที่เรียกได้เป็นงานที่ผู้ใช้นั้นสั่งเท่านั้น

## 1. สั่งสแกน

หนึ่งคำสั่งสร้าง `av_jobs` หนึ่งแถว และมี `av_scan_results` หนึ่งแถวต่อเครื่อง เชื่อมด้วย `job_id` แต่ละเครื่องมี `commands.id` / `request_id` ของตัวเอง โปรโตคอลที่ส่งไป agent ยังคงเดิม

### สั่งหลายเครื่องใน job เดียว

```json
{"type":"virus_scan","agent_ids":["11111111-1111-1111-1111-111111111111","44444444-4444-4444-4444-444444444444"],"scan_type":"quick"}
```

ใช้ `agent_ids` 1–100 เครื่อง UUID ต้องไม่ซ้ำ หรือ `agent_id` แบบเดิมหนึ่งเครื่อง ห้ามส่งทั้งสองอย่างพร้อมกัน ทุกเครื่องต้องออนไลน์ตอนตรวจ และต้องมีอยู่/ไม่ DISABLED ตอนสร้างข้อมูล หากไม่ผ่านจะไม่สร้าง job และไม่ส่งคำสั่งเลย หากเครื่องหลุดหลังสร้าง job การส่งของเครื่องนั้นอาจเป็น `uncertain` โดยเครื่องอื่นยังได้รับคำสั่งตามปกติ

```json
{
  "type":"virus_scan_accepted",
  "job_id":"55555555-5555-5555-5555-555555555555",
  "scan_type":"quick",
  "total_targets":2,
  "targets":[
    {"agent_id":"11111111-1111-1111-1111-111111111111","request_id":"22222222-2222-2222-2222-222222222222","dispatch":"sent"},
    {"agent_id":"44444444-4444-4444-4444-444444444444","request_id":"33333333-3333-3333-3333-333333333333","dispatch":"sent"}
  ]
}
```

เก็บ `job_id` สำหรับติดตามทั้งงาน และใช้ `targets` จับคู่ command กับเครื่อง `job_id` และ `request_id` สร้างจาก server ห้ามส่งมาขณะสร้างงาน Custom scan ใช้ path เดียวกันทุกเครื่องใน job หากสั่งเครื่องเดียว response มี `job_id`, `total_targets`, `targets` เพิ่มจาก field เดิมด้วย

### สั่งเครื่องเดียว (รองรับ API เดิม)

```json
{"type":"virus_scan","agent_id":"11111111-1111-1111-1111-111111111111","scan_type":"quick"}
```

`scan_type` เลือก `quick`, `full`, `custom` เท่านั้น สำหรับ custom ต้องระบุ absolute Windows path บนเครื่อง agent:

```json
{"type":"virus_scan","agent_id":"11111111-1111-1111-1111-111111111111","scan_type":"custom","path":"C:\\Users\\Public\\Downloads"}
```

ห้าม wildcard และห้ามส่ง path ใน quick/full; agent ตรวจว่า path มีอยู่จริงอีกครั้ง Server สร้าง `request_id` ใหม่จาก `commands.id` ให้แต่ละคำสั่ง หน้าบ้านไม่ต้องและไม่ควรส่ง `request_id` ในคำสั่งสร้างงาน Agent ต้องออนไลน์และไม่ DISABLED

```json
{"type":"virus_scan_accepted","request_id":"22222222-2222-2222-2222-222222222222","agent_id":"11111111-1111-1111-1111-111111111111","scan_type":"quick","dispatch":"sent"}
```

หมายถึงสร้างงานและพยายามส่งแล้ว ยังไม่ยืนยันว่า Defender เริ่มสแกน `dispatch` เป็น `sent` เมื่อเขียน WebSocket สำเร็จ หรือ `uncertain` เมื่อเขียนไม่สำเร็จและไม่ทราบว่า agent ได้รับหรือไม่ ทั้งสองกรณีให้เก็บ request ID และติดตามผล ห้าม retry อัตโนมัติ เพราะ agent ไม่ deduplicate งาน

## 2. ดูสถานะและผล

ดูว่า job นี้ไปเครื่องไหนบ้างพร้อมผลแต่ละเครื่อง:

```json
{"type":"virus_scan_list","job_id":"55555555-5555-5555-5555-555555555555"}
```

Response มี `job_id` ตามตัวกรอง และ `scans` เป็นรายการเครื่องพร้อม `job_id`, `agent_id`, `request_id`, `status`, `result` ของแต่ละเครื่อง เมื่อกรอง job ค่าเริ่มต้น limit เป็น 100 จึงครอบคลุมทุกเครื่องของ job ที่ API สร้างได้ ถ้ากำหนด limit เองต่ำกว่าจำนวนเครื่องจะได้เพียงบางส่วน ห้ามใช้รายการที่ถูกจำกัดนั้นสรุปว่างานทั้ง job จบแล้ว

กรอง `job_id`, `agent_id`, `request_id` ร่วมกันได้ (ต้องตรงทุกเงื่อนไข) หรือไม่ส่งตัวกรองเพื่อดูประวัติของผู้ใช้ทุกเครื่อง แต่ละแถวมี `job_id` ให้จัดกลุ่ม ตาราง av_jobs เก็บข้อมูลคำขอ ส่วนสถานะอ้างอิงแต่ละ target โดยไม่มี status รวมซ้ำใน av_jobs ให้ UI รอจนทุก target เป็นสถานะสุดท้ายและแสดงจำนวนสำเร็จ/ล้มเหลวแยกกัน

ใช้ polling เช่นทุก 3 วินาที ไม่มี push event สำหรับผลสแกนไปหน้าบ้านใน API นี้

```json
{"type":"virus_scan_list","agent_id":"11111111-1111-1111-1111-111111111111","request_id":"22222222-2222-2222-2222-222222222222"}
```

```json
{
  "type":"virus_scan_list",
  "agent_id":"11111111-1111-1111-1111-111111111111",
  "request_id":"22222222-2222-2222-2222-222222222222",
  "scans":[{
    "request_id":"22222222-2222-2222-2222-222222222222",
    "agent_id":"11111111-1111-1111-1111-111111111111",
    "scan_type":"quick",
    "status":"SUCCEEDED",
    "created_at":"2026-09-05T10:00:00Z",
    "started_at":"2026-09-05T10:00:01Z",
    "finished_at":"2026-09-05T10:02:00Z",
    "result":{
      "type":"virus_scan_result",
      "request_id":"22222222-2222-2222-2222-222222222222",
      "scan_type":"quick",
      "status":"completed",
      "started_at":"2026-09-05T10:00:01Z",
      "finished_at":"2026-09-05T10:02:00Z",
      "report":{"exit_code":0,"output":"Defender output","output_truncated":false}
    }
  }]
}
```

`path`, `started_at`, `finished_at`, `result`, `message` อาจไม่มีค่าและถูกละไว้ `message` อธิบายข้อผิดพลาดหรือการส่งที่ไม่แน่นอน `result` เป็น event ล่าสุดที่บันทึกจาก agent จึงอาจเป็น `virus_scan_status` ระหว่างกำลังทำงาน ข้อความจาก agent ให้แสดงด้วย `textContent` หรือ text binding ของ framework

เรียกประวัติโดยไม่ส่ง `request_id` และกำหนด `limit` ได้ 1–100 (ค่าเริ่มต้น 20 หรือ 100 เมื่อกรอง job; 0 ใช้ค่าเริ่มต้น) เรียงใหม่ไปเก่า ไม่มี pagination ในรุ่นนี้ ไม่พบงานหรือไม่ใช่งานของผู้ใช้จะได้ `scans: []`

```json
{"type":"virus_scan_list","agent_id":"11111111-1111-1111-1111-111111111111","limit":20}
```

| status ของงาน | ความหมาย | status ใน av_scan_results |
|---|---|---|
| QUEUED | บันทึกแล้ว ยังไม่มีหลักฐานการส่งสำเร็จ | PENDING |
| DELIVERED | เขียนคำสั่งไป WebSocket สำเร็จ | PENDING |
| RUNNING | agent รับงานแล้ว | RUNNING |
| SUCCEEDED | agent ส่ง completed | COMPLETED |
| FAILED | agent ส่ง failed หรือ rejected; ดู result.status และ result.error | FAILED |
| CANCELLED | agent ส่ง cancelled | CANCELLED |

หยุด polling งานนั้นเมื่อเป็น SUCCEEDED/FAILED/CANCELLED; หากฐานข้อมูลถูกจัดการจากระบบอื่นจนเป็น EXPIRED ให้ถือเป็นสถานะสุดท้ายเช่นกัน ไม่มีเปอร์เซ็นต์ความคืบหน้าหรือคำสั่งยกเลิกจากหน้าบ้าน

**SUCCEEDED ไม่ได้แปลว่าไม่พบไวรัส**: exit code 0 อาจหมายถึงพบและจัดการแล้ว เช่นเดียวกับ FAILED ไม่ยืนยันว่าพบไวรัส ห้ามแสดงจำนวนไฟล์/ภัยคุกคามเป็น 0 จาก API นี้ เพราะ agent ไม่รายงานจำนวนเหล่านี้

## 3. ข้อผิดพลาดและ reconnect

```json
{"type":"error","stream":"virus_scan","error":"agent is offline"}
```

ตัวอย่าง error: `ไม่มี permission`, `invalid agent_id`, `invalid request_id`, `scan_type must be quick, full, or custom`, `custom path must be an absolute Windows path`, `cannot create scan`, `cannot load scans` ข้อผิดพลาดการเชื่อมต่อ session ส่ง HTTP 401 ก่อน upgrade ข้อผิดพลาดคำสั่งไม่มี request ID ให้ส่งคำสั่งสร้างทีละคำสั่งรอ accepted/error เพื่อจับคู่ได้ง่าย

เมื่อ frontend reconnect ให้เรียกประวัติหรือ request ID เดิม ห้ามส่งคำสั่งสร้างซ้ำอัตโนมัติ หากไม่ได้รับ accepted ให้ตรวจประวัติก่อนให้ผู้ใช้ตัดสินใจสั่งใหม่ งานซ้อนกันส่งถึง agent ได้และ agent จะตอบ rejected

เมื่อ agent หลุด งานอาจยังสแกนอยู่ Server ไม่เปลี่ยนงานเป็น FAILED และไม่ส่งงานซ้ำ ผลที่ agent ส่งกลับหลัง reconnect ยังจับคู่และบันทึกได้ หาก agent สูญเสียผล งานอาจค้าง QUEUED/DELIVERED/RUNNING ไม่มี timeout สรุปผลเอง ให้ UI แสดงว่า “ยังไม่ได้รับผล” และเปิดให้ผู้ใช้ตรวจสอบ ไม่ควรแสดงว่าสแกนล้มเหลวจากการขาดการเชื่อมต่อเพียงอย่างเดียว

## ตัวอย่าง JavaScript

```javascript
const socket = new WebSocket(`wss://${location.host}/ws/frontend`);
const agentId = "11111111-1111-1111-1111-111111111111";
let timer;
function refresh(requestId) {
  if (socket.readyState === WebSocket.OPEN) {
    socket.send(JSON.stringify({ type: "virus_scan_list", agent_id: agentId, request_id: requestId }));
  }
}
// เรียกจากปุ่มสแกนเมื่อ socket เปิดแล้ว และปิดปุ่มระหว่างรอ accepted/error
function startScan() {
  if (socket.readyState !== WebSocket.OPEN) return;
  socket.send(JSON.stringify({ type: "virus_scan", agent_id: agentId, scan_type: "quick" }));
}
socket.onmessage = ({ data }) => {
  const event = JSON.parse(data);
  if (event.type === "virus_scan_accepted" && event.agent_id === agentId) {
    clearInterval(timer);
    refresh(event.request_id);
    timer = setInterval(() => refresh(event.request_id), 3000);
  }
  if (event.type === "virus_scan_list" && event.agent_id === agentId) {
    console.log(event.scans); // อัปเดต UI จากข้อมูลที่บันทึกแล้ว
    if (event.request_id && event.scans.length &&
        ["SUCCEEDED", "FAILED", "CANCELLED", "EXPIRED"].includes(event.scans[0].status)) {
      clearInterval(timer);
    }
  }
  if (event.type === "error" && event.stream === "virus_scan") console.error(event.error);
};
socket.onclose = () => clearInterval(timer);
```

## การบันทึกและติดตั้ง

ฐานข้อมูลเดิมต้องรัน `migrations/20260905_add_av_jobs.sql` ก่อนใช้โค้ดรุ่นนี้ สำหรับฐานข้อมูลใหม่ใช้ `schema.sql` ที่ปรับแล้วได้เลย **ไม่รัน migration av_jobs ซ้ำหลัง schema ใหม่** ส่วน migration file distribution ยังคงต้องติดตั้งตามระบบเดิม

```powershell
psql "$env:DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/20260905_add_av_jobs.sql
```

Migration ทำใน transaction: เพิ่ม av_jobs, backfill job เก่าโดยใช้ command ID เดิมเป็น job ID แล้วเชื่อมผลเดิมผ่าน job_id โดยไม่เปลี่ยน request ID หรือผลเดิม เพิ่ม unique constraint สำหรับ (job_id, agent_id) และ command_id หากข้อมูลเก่ามีหลายผลต่อ command จะ rollback ทั้ง migration ให้ตรวจข้อมูลซ้ำก่อน ไม่ลบประวัติอัตโนมัติ Migration ใช้ครั้งเดียว ไม่ใช่สคริปต์ที่รันซ้ำได้

ความสัมพันธ์: `av_jobs → av_scan_results → agents / commands` โดยหนึ่ง job มีหลายผล/เครื่อง และหนึ่ง command มีผลได้หนึ่งแถว ใช้ foreign key และ unique constraints ตรวจความสัมพันธ์ การลบ agent/command ยังคง cascade ผลตาม schema เดิม จึงอาจทำให้รายการเครื่องใน job ลดลงหากมีการลบเครื่องจริง

- สร้าง av_jobs พร้อม commands และ av_scan_results ของทุกเครื่อง และ logs ใน transaction ก่อนส่งคำสั่งใด ๆ
- เก็บผู้สั่ง/scan_type/path ใน av_jobs และ scan_type/path ใน commands.payload; command ID เป็น request ID ต่อเครื่อง ส่วน job ID ใช้เชื่อมกลุ่มงาน
- เก็บ event ทั้งก้อนรวม report/error ใน av_scan_results.threat_details โดยไม่ parse ข้อความ Defender เพื่อคาดเดาภัยคุกคาม
- คอลัมน์ total_files_scanned/threats_found เดิมยังเป็น default 0 ของ schema ซึ่งหมายถึงไม่มีข้อมูลใน integration นี้ ไม่ส่งสองค่านี้ใน API
- อัปเดตสถานะและ audit log ใน transaction ตรวจ command ID, agent ID จาก connection และ scan_type ให้ตรงกัน ผลหลังสถานะสุดท้ายจะไม่เขียนทับผลเดิม
- หากฐานข้อมูลบันทึกผลไม่ได้ server เขียน error ลง log; protocol agent ยังไม่มี acknowledgement/retry ที่รับประกันผลถึง server

โปรโตคอล agent: [virus-scan.md](virus-scan.md)

ทดสอบ migration/repository กับ PostgreSQL สำหรับทดสอบเท่านั้น:

```powershell
$env:AV_TEST_DATABASE_URL = 'host=127.0.0.1 port=55439 user=postgres dbname=postgres sslmode=disable'
go test ./internal/virusscan -run TestRepositoryMigrationAndMultiAgentJob -v
```

การทดสอบสร้าง schema แยกและลบเมื่อจบ ครอบคลุม schema ใหม่, backfill ข้อมูลเดิม, สร้าง job หลายเครื่อง, ผลจากเครื่องผิด, ผลซ้ำ/มาผิดลำดับ, สิทธิ์อ่านตามเจ้าของ และ rollback เมื่อ target ไม่ถูกต้อง

Every frontend command revalidates the handshake session against the database. Expired or revoked sessions and inactive users receive ขาดการ login and the connection closes. Do not include session tokens in command JSON.

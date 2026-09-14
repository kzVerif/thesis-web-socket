# Virus scan ผ่าน WebSocket

Agent รองรับ Microsoft Defender บน Windows โดยเรียก `MpCmdRun.exe` โดยตรง ไม่มีการประกอบคำสั่งผ่าน shell ต้องติดตั้งและเปิดใช้งาน Defender และรัน agent ด้วยสิทธิ์ Administrator หากระบบกำหนด บนระบบอื่นจะส่ง `failed` กลับ

## คำสั่งจาก server

ส่ง JSON text message เลือกหนึ่งในสามแบบ:

```json
{"type":"virus_scan","request_id":"scan-001","scan_type":"quick"}
```

```json
{"type":"virus_scan","request_id":"scan-002","scan_type":"custom","path":"C:\\Users\\Public\\Downloads"}
```

```json
{"type":"virus_scan","request_id":"scan-003","scan_type":"full"}
```

- `request_id`: string ไม่ว่าง ยาวไม่เกิน 128 bytes ใช้จับคู่คำสั่งและผลลัพธ์ Server ควรสร้าง ID ใหม่แต่ละงาน
- `scan_type`: `quick`, `custom`, `full` ตัวพิมพ์เล็ก
- `path`: ระบุเฉพาะ custom เป็น absolute path ของไฟล์หรือโฟลเดอร์ที่มีอยู่บนเครื่อง agent ไม่รองรับ wildcard
- quick สแกนพื้นที่สำคัญตาม Defender; full สแกนทั้งระบบตามนโยบาย Defender; custom สแกนเป้าหมายที่ระบุ

## ข้อมูลที่ส่งกลับ

เมื่อรับงานแล้ว (ยังไม่ได้ยืนยันว่า Defender เริ่มสแกนสำเร็จ):

```json
{"type":"virus_scan_status","request_id":"scan-001","scan_type":"quick","status":"running","started_at":"2026-09-05T10:00:00Z"}
```

เมื่อคำสั่ง Defender จบสำเร็จ:

```json
{
  "type":"virus_scan_result",
  "request_id":"scan-001",
  "scan_type":"quick",
  "status":"completed",
  "started_at":"2026-09-05T10:00:00Z",
  "finished_at":"2026-09-05T10:02:00Z",
  "report":{"exit_code":0,"output":"ข้อความจาก Defender","output_truncated":false}
}
```

สถานะสุดท้าย: `completed` = คำสั่งจบด้วย exit code 0, `failed` = รันไม่สำเร็จหรือ exit code ไม่ใช่ 0, `rejected` = ข้อมูลไม่ถูกต้องหรือมีงานกำลังทำอยู่, `cancelled` = context ของ agent ถูกยกเลิก ข้อผิดพลาดอยู่ใน `error`; งานที่ถูกปฏิเสธไม่มี `report` และ `started_at` เวลาใช้ UTC RFC3339

`report.exit_code` เป็นรหัสจาก Defender (`-1` เมื่อไม่มีรหัสจบ) รหัส 0 หมายถึงไม่พบภัยคุกคาม **หรือพบและจัดการสำเร็จแล้ว** ส่วนรหัส 2 อาจหมายถึงพบภัยคุกคามที่ต้องดำเนินการต่อหรือเกิดข้อผิดพลาด จึงห้ามตีความ `completed` ว่าไม่พบไวรัส หรือ `failed` ว่าพบไวรัสแน่นอน ดู `output` และประวัติ Defender ประกอบ ไม่มีการสร้างจำนวนภัยคุกคามจากข้อความที่ขึ้นกับภาษาเครื่อง

`output` รวม stdout/stderr เก็บสูงสุด 32 KiB และตั้ง `output_truncated` เมื่อเกินขนาด ข้อความอาจขึ้นกับภาษา/encoding ของ Defender

## พฤติกรรมการทำงาน

- สแกนเบื้องหลัง รับ heartbeat และคำสั่งอื่นได้ระหว่างทำงาน รับงานสแกนครั้งละหนึ่งงานต่อ Client; งานที่ซ้อนกันถูกปฏิเสธ
- ใช้นโยบายจัดการภัยคุกคามและ exclusions ของ Defender ซึ่งอาจกักกันหรือลบภัยคุกคามโดยอัตโนมัติ
- ไม่มีเปอร์เซ็นต์ความคืบหน้า ไม่มีคำสั่งหยุดสแกน และไม่มี timeout เพิ่มเติมจาก agent; Defender ควบคุมระยะเวลาตามค่าของระบบ
- เมื่อ WebSocket หลุด งานยังทำต่อและเก็บผลสุดท้ายในคิวหน่วยความจำร่วมกับ download สูงสุด 32 ข้อความ ส่งหลังเชื่อมต่อใหม่ สถานะ `running` ไม่เก็บระหว่างหลุด
- คิวเต็มจะบันทึกข้อผิดพลาดและผลนั้นอาจสูญหาย; ปิดโปรแกรมแล้วคิวหาย ไม่มี server acknowledgement หรือการรับประกันส่งถึงปลายทาง
- ไม่มีการจำ ID เพื่อป้องกันการสแกนซ้ำหลังงานจบ Server ไม่ควรส่งงานเดิมซ้ำอัตโนมัติเมื่อ reconnect
- ปิด agent จะยกเลิก process คำสั่งที่รันอยู่ แต่ไม่รับประกันว่า Defender service จะหยุดงานภายในหรือผลจะส่งถึง server ก่อนโปรแกรมปิด

## ตัวอย่าง server (JavaScript / ws)

```javascript
const requestId = "scan-" + crypto.randomUUID();
ws.send(JSON.stringify({
  type: "virus_scan", request_id: requestId, scan_type: "quick"
}));
ws.on("message", raw => {
  const event = JSON.parse(raw.toString());
  if (event.request_id !== requestId) return;
  if (event.type === "virus_scan_status") console.log("เริ่มงาน", event);
  if (event.type === "virus_scan_result") console.log("ผลสแกน", event);
});
```

รายละเอียดคำสั่งและ exit code: [Microsoft Defender command-line reference](https://learn.microsoft.com/en-us/defender-endpoint/command-line-arguments-microsoft-defender-antivirus)

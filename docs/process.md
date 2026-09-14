# Process Streaming Protocol

เอกสารนี้อธิบายวิธีที่ WebSocket server ใช้สั่งให้ agent เริ่มและหยุดส่งรายการ process ของเครื่อง client

## การทำงาน

- หลังเชื่อมต่อ WebSocket agent จะยังไม่ส่งรายการ process
- server ต้องส่งคำสั่ง `start` ก่อน
- เมื่อได้รับ `start` agent จะส่งรายการ process ครั้งแรกทันที และส่งรายการล่าสุดซ้ำทุก 5 วินาที
- agent จะหยุดส่งเมื่อได้รับคำสั่ง `stop` หรือเมื่อการเชื่อมต่อถูกยกเลิก
- คำสั่ง Process และ Performance ควบคุมแยกกัน การเริ่มหรือหยุด Process จึงไม่เปลี่ยนสถานะของ Performance
- การส่ง `start` หรือ `stop` ซ้ำจะไม่สร้างรอบการส่งซ้ำและไม่มีผลข้างเคียง

## เริ่มรับรายการ Process

รูปแบบที่แนะนำ:

```json
{
  "type": "process",
  "action": "start"
}
```

รูปแบบย่อที่รองรับ:

```json
{
  "action": "start_process"
}
```

## หยุดรับรายการ Process

รูปแบบที่แนะนำ:

```json
{
  "type": "process",
  "action": "stop"
}
```

รูปแบบย่อที่รองรับ:

```json
{
  "action": "stop_process"
}
```

## ข้อมูลที่ agent ส่งกลับ

agent ส่ง JSON text message ที่มีค่าเป็น array ของ process:

```json
[
  {
    "pid": 1324,
    "name": "systemd"
  },
  {
    "pid": 8120,
    "name": "node"
  }
]
```

| Field | Type | ความหมาย |
| --- | --- | --- |
| `pid` | integer | Process ID บนเครื่อง client |
| `name` | string | ชื่อ executable ของ process |

รายการอาจเปลี่ยนในแต่ละรอบตาม process ที่เริ่มหรือหยุดทำงาน หาก agent ไม่มีสิทธิ์อ่านชื่อ process บางรายการ รายการนั้นจะไม่ถูกส่งกลับ

## ตัวอย่างฝั่ง WebSocket server (JavaScript)

```javascript
function startProcessStream(ws) {
  ws.send(JSON.stringify({
    type: "process",
    action: "start",
  }));
}

function stopProcessStream(ws) {
  ws.send(JSON.stringify({
    type: "process",
    action: "stop",
  }));
}

ws.on("message", (rawMessage) => {
  const message = JSON.parse(rawMessage.toString());

  if (Array.isArray(message)) {
    console.log("Process list:", message);
  }
});
```

## คำสั่งรูปแบบอื่นที่รองรับ

agent อ่านคำสั่งจาก field `type`, `action` หรือ `command` โดยไม่สนใจตัวพิมพ์เล็ก-ใหญ่ และเปลี่ยนเครื่องหมาย `-` เป็น `_` ก่อนตรวจสอบ

- เริ่ม: `start_process`, `process_start`, `request_process`
- หยุด: `stop_process`, `process_stop`
- `type` รองรับทั้ง `process` และ `processes`
- เมื่อใช้ `type` สามารถกำหนด `action` เป็น `start`, `request` หรือ `stop`

แนะนำให้ฝั่ง server ใช้ `type: "process"` ร่วมกับ `action: "start" | "stop"`

## ปิด Process ตาม PID

คำสั่งนี้เป็นคำสั่งครั้งเดียวและไม่เปลี่ยนสถานะการส่งรายการ Process ใช้รูปแบบ:

```json
{
  "type": "process",
  "action": "kill",
  "pid": 8120
}
```

`pid` ต้องเป็น integer ที่มากกว่า `0` และต้องเป็น process ที่ agent มีสิทธิ์ปิด ระบบปฏิบัติการอาจปฏิเสธการปิด system process หรือ process ที่ต้องใช้สิทธิ์สูงกว่า

รูปแบบคำสั่งทางเลือกที่รองรับ:

```json
{
  "action": "kill_process",
  "pid": 8120
}
```

ชื่อคำสั่งที่รองรับ ได้แก่ `kill_process`, `process_kill` และ `terminate_process` หรือใช้ `type: "process"` ร่วมกับ `action: "kill" | "terminate"`

### ผลลัพธ์เมื่อปิดสำเร็จ

```json
{
  "type": "process",
  "action": "kill_result",
  "pid": 8120,
  "success": true
}
```

### ผลลัพธ์เมื่อปิดไม่สำเร็จ

```json
{
  "type": "process",
  "action": "kill_result",
  "pid": 8120,
  "success": false,
  "error": "kill process 8120: Access is denied."
}
```

ฝั่ง server ควรตรวจ `type`, `action`, `pid` และ `success` เพื่อจับคู่ผลลัพธ์กับคำสั่ง หาก `success` เป็น `false` รายละเอียดจะอยู่ใน field `error`

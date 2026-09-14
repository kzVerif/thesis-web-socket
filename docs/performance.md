# Performance Streaming Protocol

เอกสารนี้อธิบายวิธีที่ WebSocket server ใช้สั่งให้ agent เริ่มและหยุดส่งข้อมูล Performance

## การทำงาน

- หลังจากเชื่อมต่อ WebSocket แล้ว agent จะยังไม่ส่งข้อมูล Performance
- server ต้องส่งคำสั่ง `start` ก่อน
- เมื่อได้รับคำสั่ง `start` agent จะส่งข้อมูลครั้งแรกทันที จากนั้นส่งซ้ำทุก 5 วินาที
- agent จะส่งต่อเนื่องจนกว่า server จะส่งคำสั่ง `stop` หรือการเชื่อมต่อถูกยกเลิก
- การส่ง `start` ซ้ำในขณะที่กำลังส่งอยู่จะไม่สร้างรอบการส่งเพิ่ม
- การส่ง `stop` ซ้ำไม่มีผลข้างเคียง

## เริ่มรับ Performance

รูปแบบที่แนะนำ:

```json
{
  "type": "performance",
  "action": "start"
}
```

รูปแบบย่อที่รองรับ:

```json
{
  "action": "start_performance"
}
```

## หยุดรับ Performance

รูปแบบที่แนะนำ:

```json
{
  "type": "performance",
  "action": "stop"
}
```

รูปแบบย่อที่รองรับ:

```json
{
  "action": "stop_performance"
}
```

## ข้อมูลที่ agent ส่งกลับ

agent ส่ง JSON text message ตามตัวอย่าง:

```json
{
  "cpu_usage": 18.42,
  "ram_total_gb": 15.87,
  "ram_used_gb": 8.31,
  "ram_usage": 52.36,
  "disk_total_gb": 475.82,
  "disk_used_gb": 201.54,
  "disk_free_gb": 274.28,
  "disk_usage": 42.36
}
```

| Field | Type | ความหมาย |
| --- | --- | --- |
| `cpu_usage` | number | เปอร์เซ็นต์การใช้งาน CPU |
| `ram_total_gb` | number | RAM ทั้งหมด หน่วย GB |
| `ram_used_gb` | number | RAM ที่ใช้งานอยู่ หน่วย GB |
| `ram_usage` | number | เปอร์เซ็นต์การใช้งาน RAM |
| `disk_total_gb` | number | พื้นที่ดิสก์ทั้งหมด หน่วย GB |
| `disk_used_gb` | number | พื้นที่ดิสก์ที่ใช้แล้ว หน่วย GB |
| `disk_free_gb` | number | พื้นที่ดิสก์ที่ว่าง หน่วย GB |
| `disk_usage` | number | เปอร์เซ็นต์การใช้งานดิสก์ |

ค่าประเภทเปอร์เซ็นต์อยู่ในช่วง `0` ถึง `100` และค่าขนาดพื้นที่ใช้หน่วย GB

## ตัวอย่าง JavaScript

```javascript
const ws = new WebSocket("ws://localhost:8081/ws");

ws.addEventListener("open", () => {
  ws.send(JSON.stringify({
    type: "performance",
    action: "start",
  }));
});

ws.addEventListener("message", (event) => {
  const performance = JSON.parse(event.data);
  console.log("Performance:", performance);
});

function stopPerformance() {
  ws.send(JSON.stringify({
    type: "performance",
    action: "stop",
  }));
}
```

## คำสั่งรูปแบบอื่นที่รองรับ

เพื่อรองรับ server หลายรูปแบบ agent สามารถอ่านคำสั่งจาก `type`, `action` หรือ `command` และไม่สนใจตัวพิมพ์เล็ก-ใหญ่ โดยรองรับชื่อดังนี้:

- เริ่ม: `start_performance`, `performance_start`, `request_performance`
- หยุด: `stop_performance`, `performance_stop`
- ใช้ `type: "performance"` ร่วมกับ `action: "start"`, `"request"` หรือ `"stop"`

แนะนำให้ใช้รูปแบบ `type: "performance"` และ `action: "start" | "stop"` เพื่อให้ protocol อ่านง่ายและขยายต่อได้

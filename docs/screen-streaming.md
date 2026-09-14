# Screen Streaming Protocol (Agent → WebSocket Server)

เอกสารนี้เป็น contract สำหรับให้ WebSocket server ควบคุมและรับภาพหน้าจอจาก agent แบบ **ดูอย่างเดียว (view-only)**

## สรุป protocol

| ทิศทาง | WebSocket message type | Payload |
| --- | --- | --- |
| Server → Agent | Text | JSON คำสั่งเริ่ม/หยุด |
| Agent → Server | Binary | ไฟล์ JPEG หนึ่งภาพต่อหนึ่ง message |

Agent จะยังไม่จับหรือส่งภาพจนกว่า server ที่ยืนยันตัวตนแล้วจะสั่งเริ่ม stream เมื่อหยุด stream หรือ connection หลุด agent จะยกเลิก capture ทันที

## คำสั่งจาก server

เริ่ม stream (รูปแบบที่แนะนำ):

```json
{
  "type": "screen",
  "action": "start"
}
```

หยุด stream:

```json
{
  "type": "screen",
  "action": "stop"
}
```

รองรับรูปแบบย่อ `{"type":"start_stream"}` และ `{"type":"stop_stream"}` ด้วย คำสั่งไม่สนใจตัวพิมพ์เล็ก-ใหญ่และยอมรับ `-` แทน `_` การส่ง start หรือ stop ซ้ำเป็น idempotent และจะไม่สร้าง capture goroutine ซ้ำ

## Frame จาก agent

- แต่ละ WebSocket binary message คือ JPEG ที่สมบูรณ์หนึ่งภาพ ไม่ได้ห่อ JSON และไม่ได้แปลง Base64
- จับภาพ primary display (display index 0)
- ขนาดไม่เกิน `1280x720` และรักษา aspect ratio; หน้าจอขนาดเล็กกว่านี้จะไม่ถูกขยาย
- JPEG quality `60`
- เป้าหมาย `5 FPS` หรือหนึ่งรอบทุก `200 ms`
- capture, resize, encode และ write ทำตามลำดับ จึงไม่มี queue สะสม ถ้า network ช้า FPS จะลดลงแทนการเก็บภาพเก่าค้างไว้

ฝั่ง server ต้องแยก message ด้วยชนิด WebSocket frame ก่อน parse: text message ใช้กับ JSON เดิมของระบบ ส่วน binary message ใน connection ของ agent คือ screen JPEG

## ตัวอย่าง server ด้วย Node.js (`ws`)

```javascript
agentSocket.send(JSON.stringify({ type: "screen", action: "start" }));

agentSocket.on("message", (data, isBinary) => {
  if (isBinary) {
    // data เป็น Buffer ของ JPEG โดยตรง
    viewerSocket.send(data, { binary: true });
    return;
  }

  const message = JSON.parse(data.toString("utf8"));
  handleAgentJson(message);
});

function stopViewing() {
  if (agentSocket.readyState === agentSocket.OPEN) {
    agentSocket.send(JSON.stringify({ type: "screen", action: "stop" }));
  }
}
```

หาก browser รับ frame ผ่าน WebSocket ให้ตั้ง `binaryType = "arraybuffer"` แล้วสร้าง `Blob` ชนิด `image/jpeg`; ควร revoke object URL เดิมทุกครั้งเพื่อไม่ให้ memory เพิ่มต่อเนื่อง

```javascript
viewerSocket.binaryType = "arraybuffer";
let previousUrl;

viewerSocket.onmessage = ({ data }) => {
  if (typeof data === "string") return;
  const nextUrl = URL.createObjectURL(new Blob([data], { type: "image/jpeg" }));
  screenImage.src = nextUrl;
  if (previousUrl) URL.revokeObjectURL(previousUrl);
  previousUrl = nextUrl;
};
```

## Lifecycle ที่ server ต้องดูแล

1. Authenticate connection และผูก agent กับผู้ใช้/สิทธิ์ก่อนยอมให้ส่งคำสั่ง screen
2. ส่ง start เมื่อมี viewer ที่มีสิทธิ์เริ่มดู
3. ส่ง stop เมื่อ viewer ออกจากหน้า, unsubscribe หรือ session หมดอายุ
4. เมื่อ agent disconnect ให้ล้างสถานะ stream และ buffer ฝั่ง server
5. จำกัด outbound queue ไปยัง viewer ไว้หนึ่ง frame โดยให้ frame ล่าสุดแทน frame เก่า (latest frame wins)

ห้าม expose คำสั่ง start/stop แก่ connection ที่ยังไม่ผ่าน authentication และ protocol นี้ไม่มี mouse, keyboard, input injection หรือ remote interaction ใด ๆ

## ข้อสังเกตเรื่องหลาย viewer

Agent เก็บสถานะเป็น boolean ไม่ได้นับจำนวน viewer ดังนั้น server ต้องเป็นผู้รวม subscription: ส่ง start ตอนจำนวน viewer เปลี่ยนจาก `0 → 1` และส่ง stop ตอนเปลี่ยนจาก `1 → 0` เพื่อไม่ให้ viewer คนหนึ่งหยุด stream ของ viewer คนอื่น

## Error และ disconnect

- ถ้าจับภาพรอบหนึ่งไม่สำเร็จ agent จะ log error และลองใหม่ในรอบถัดไป
- ถ้าส่ง binary frame ไม่สำเร็จ capture goroutine จะหยุด
- เมื่อ WebSocket read loop จบ, context ถูก cancel หรือโปรแกรม shutdown agent จะ cancel stream, หยุด ticker และรอ goroutine จบก่อนปิด connection
- connection layer จะลองเชื่อมต่อ server ใหม่ทุก 3 วินาทีจนกว่าโปรแกรมจะได้รับ shutdown signal โดย stream จะกลับมาในสถานะหยุดเสมอหลัง reconnect และ server ต้องส่ง start ใหม่

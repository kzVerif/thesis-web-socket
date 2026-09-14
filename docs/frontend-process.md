# Next.js Process WebSocket

ใช้ WebSocket endpoint เดียวกับ performance:

```text
ws://localhost:8081/ws/frontend
```

## เริ่มรับรายการ process

```json
{
  "type": "process",
  "action": "start",
  "agent_id": "86dbfbcc-a13c-4ffc-adab-7187018273e0"
}
```

Server ตอบรับ:

```json
{
  "type": "subscribed",
  "stream": "process",
  "agent_id": "86dbfbcc-a13c-4ffc-adab-7187018273e0"
}
```

เมื่อ agent ส่งรายการเข้ามา Server จะส่งให้ frontend ในรูปแบบ:

```json
{
  "type": "process",
  "agent_id": "86dbfbcc-a13c-4ffc-adab-7187018273e0",
  "data": [
    { "pid": 1324, "name": "systemd" },
    { "pid": 8120, "name": "node" }
  ],
  "received_at": "2026-08-25T08:00:00Z"
}
```

## หยุดรับรายการ process

```json
{
  "type": "process",
  "action": "stop",
  "agent_id": "86dbfbcc-a13c-4ffc-adab-7187018273e0"
}
```

Process และ Performance เป็นคนละ subscription การหยุด process จะไม่หยุด performance หาก frontend ปิด connection โดยไม่ส่ง `stop` server จะ unsubscribe และส่ง `stop` ให้ agent อัตโนมัติเมื่อไม่มีผู้รับ process เหลือ

## ปิด process ตาม PID

Frontend ส่งคำสั่ง:

```json
{
  "type": "process",
  "action": "kill",
  "agent_id": "86dbfbcc-a13c-4ffc-adab-7187018273e0",
  "pid": 8120
}
```

เมื่อ server รับคำสั่งและส่งต่อให้ agent แล้ว frontend จะได้รับ:

```json
{
  "type": "process",
  "action": "kill_accepted",
  "agent_id": "86dbfbcc-a13c-4ffc-adab-7187018273e0",
  "pid": 8120
}
```

ผลลัพธ์สำเร็จ:

```json
{
  "type": "process",
  "action": "kill_result",
  "agent_id": "86dbfbcc-a13c-4ffc-adab-7187018273e0",
  "pid": 8120,
  "success": true
}
```

หากไม่สำเร็จ `success` จะเป็น `false` และมีรายละเอียดใน `error` หาก agent ไม่ตอบภายใน 15 วินาทีหรือ disconnect ระหว่างรอ server จะส่ง `kill_result` ที่ไม่สำเร็จกลับมาเช่นกัน

## Next.js Client Component

```ts
const socket = new WebSocket("ws://localhost:8081/ws/frontend");

socket.addEventListener("open", () => {
  socket.send(JSON.stringify({
    type: "process",
    action: "start",
    agent_id: agentId,
  }));
});

socket.addEventListener("message", (event) => {
  const message = JSON.parse(event.data);
  if (message.type === "process" && message.agent_id === agentId) {
    setProcesses(message.data);
  }
});
```

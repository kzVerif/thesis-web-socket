# Next.js Performance WebSocket

Frontend เชื่อมต่อ endpoint:

```text
ws://localhost:8081/ws/frontend
```

ค่า origin ที่อนุญาตกำหนดด้วย `FRONTEND_ORIGINS` (คั่นหลายค่าด้วย comma) ค่าเริ่มต้นคือ `localhost:3000,127.0.0.1:3000`

## Start subscription

```json
{"type":"performance","action":"start","agent_id":"86dbfbcc-a13c-4ffc-adab-7187018273e0"}
```

Server ตอบรับด้วย:

```json
{"type":"subscribed","stream":"performance","agent_id":"86dbfbcc-a13c-4ffc-adab-7187018273e0"}
```

จากนั้น performance แต่ละ sample จะมีรูปแบบ:

```json
{
  "type": "performance",
  "agent_id": "86dbfbcc-a13c-4ffc-adab-7187018273e0",
  "data": {
    "cpu_usage": 18.42,
    "ram_total_gb": 15.87,
    "ram_used_gb": 8.31,
    "ram_usage": 52.36,
    "disk_total_gb": 475.82,
    "disk_used_gb": 201.54,
    "disk_free_gb": 274.28,
    "disk_usage": 42.36
  },
  "received_at": "2026-08-25T08:00:00Z"
}
```

## Stop subscription

```json
{"type":"performance","action":"stop","agent_id":"86dbfbcc-a13c-4ffc-adab-7187018273e0"}
```

ถ้า agent หลุด server จะส่ง `agent_status`:

```json
{"type":"agent_status","agent_id":"86dbfbcc-a13c-4ffc-adab-7187018273e0","status":"offline"}
```

ถ้า request ไม่ถูกต้องหรือ agent offline จะได้ event `error` พร้อมข้อความใน field `error`

## Next.js client example

โค้ดนี้ต้องอยู่ใน Client Component (`"use client"`):

```ts
const socket = new WebSocket("ws://localhost:8081/ws/frontend");

socket.addEventListener("open", () => {
  socket.send(JSON.stringify({
    type: "performance",
    action: "start",
    agent_id: agentId,
  }));
});

socket.addEventListener("message", (event) => {
  const message = JSON.parse(event.data);
  if (message.type === "performance" && message.agent_id === agentId) {
    setPerformance(message.data);
  }
});

// เรียกก่อน component unmount
socket.send(JSON.stringify({
  type: "performance",
  action: "stop",
  agent_id: agentId,
}));
socket.close();
```

# File Distribution สำหรับ Frontend (Next.js)

Frontend ใช้ WebSocket endpoint:

```text
wss://server.example.com/ws/frontend
```

Production ต้องใช้ `wss://` และ session cookie แบบ Secure/HttpOnly ผู้ใช้ต้อง active และ role ต้องมี permission `files.distribute`

All clients must authenticate using the __Host-session cookie on /ws/frontend. Bearer tokens are not accepted. Use HTTPS/WSS and configure FRONTEND_ORIGINS.

ตามข้อกำหนดของ cookie prefix `__Host-` ฝั่ง Auth ต้องตั้ง cookie ด้วย `Secure`, `Path=/` และห้ามกำหนด `Domain` ดังนั้น production ต้องเชื่อมผ่าน HTTPS/WSS และ WebSocket endpoint ต้องอยู่บน host เดียวกับ cookie

## แจกไฟล์ทั้ง Room

```json
{
  "type": "FILE_DISTRIBUTE",
  "request_id": "41a86b63-b28e-46da-98f4-b9e781ac89fc",
  "file_id": "2f07cc9a-9cc7-4ff3-8b40-c38965a25bd9",
  "target": {
    "type": "ROOM",
    "room_id": "59416395-f06f-4e66-9297-bf601693b7ae"
  }
}
```

Server snapshot Agent ใน Room ตอนสร้าง job การย้าย Agent เข้าหรือออกจาก Room ภายหลังไม่เปลี่ยน targets ของ job เดิม

## แจกให้ Agent ที่เลือก

```json
{
  "type": "FILE_DISTRIBUTE",
  "request_id": "41a86b63-b28e-46da-98f4-b9e781ac89fc",
  "file_id": "2f07cc9a-9cc7-4ff3-8b40-c38965a25bd9",
  "target": {
    "type": "AGENTS",
    "agent_ids": [
      "86dbfbcc-a13c-4ffc-adab-7187018273e0",
      "5d14d00e-d8cb-45e3-9922-fe2d0702f856"
    ]
  }
}
```

UUID ซ้ำถูก deduplicate ฝั่ง Server หากมี UUID ไม่ถูกต้อง, Agent ไม่มีจริง หรือ Agent ถูก disable request จะถูก reject

`request_id` เป็น UUID ที่ Frontend สร้างหนึ่งค่าต่อ user action เมื่อ reconnect หรือไม่ได้รับ response ให้ส่ง request เดิมพร้อม `request_id` เดิม เพื่อไม่สร้าง job ซ้ำ ห้ามสร้าง `request_id` ใหม่สำหรับ retry transport ของ action เดิม

## Response หลังสร้าง Job

Frontend ได้ response ทันทีหลัง snapshot และ dispatch โดยไม่รอ download เสร็จ:

```json
{
  "type": "FILE_DISTRIBUTION_CREATED",
  "job_id": "8b66e1fd-987f-4cb3-a316-a00c5dc47b7c",
  "file_id": "2f07cc9a-9cc7-4ff3-8b40-c38965a25bd9",
  "total_targets": 30,
  "online_targets": 27,
  "offline_targets": 3,
  "status": "IN_PROGRESS"
}
```

ถ้า Room ว่าง `total_targets` เป็น `0` และ job เป็น `FAILED`

## Realtime Target Update

Dashboard ทุก connection ที่ authenticate สำเร็จจะได้รับ distribution updates:

```json
{
  "type": "FILE_DISTRIBUTION_TARGET_UPDATE",
  "job_id": "8b66e1fd-987f-4cb3-a316-a00c5dc47b7c",
  "agent_id": "86dbfbcc-a13c-4ffc-adab-7187018273e0",
  "hostname": "PC-001",
  "status": "DOWNLOADING",
  "progress": 72,
  "downloaded_bytes": 41943040,
  "total_bytes": 58382912
}
```

สถานะ target ที่ schema รองรับ:

```text
PENDING -> SENT -> DOWNLOADING -> VERIFYING -> COMPLETED
                                      \------> FAILED
PENDING -> OFFLINE
```

Implementation ปัจจุบัน publish `DOWNLOADING`, `COMPLETED` และ `FAILED`; `VERIFYING` ถูกเตรียมไว้ใน schema แต่ Agent protocol ปัจจุบันยังไม่มี verifying message แยก

สถานะ job:

- `IN_PROGRESS`: ยังมี target ที่กำลังทำงาน
- `COMPLETED`: ทุก target สำเร็จ
- `PARTIAL_FAILED`: มีทั้งสำเร็จและ failed/offline
- `FAILED`: ไม่มี target ใดสำเร็จ
- `CANCELLED`: เตรียมไว้สำหรับ cancellation flow ในอนาคต

## Error Response

```json
{
  "type": "error",
  "error": "invalid file_id"
}
```

ข้อความ error ถูกทำให้ปลอดภัยสำหรับ client และไม่เปิดเผย storage path หรือ database query Frontend ควรแสดงข้อความทั่วไปแก่ผู้ใช้และเก็บรายละเอียดสำหรับ developer log โดยไม่ log session token

ข้อผิดพลาดที่พบบ่อย:

| Error | สาเหตุ |
| --- | --- |
| WebSocket handshake `401` | ไม่มี session, session หมดอายุ หรือ user ถูก disable |
| `ไม่มี permission` | role ไม่มี `files.distribute` |
| `invalid file_id` / `invalid agent_id` | ค่าไม่ใช่ UUID |
| `file unavailable` | ไม่มี record ใน `files` |
| `room not found` | ไม่มี Room ที่ระบุ |
| `one or more agents do not exist or are disabled` | target list มี Agent ที่ใช้ไม่ได้ |

## ตัวอย่าง Next.js Client Hook

```ts
"use client";

import { useEffect, useRef, useState } from "react";

type TargetUpdate = {
  type: "FILE_DISTRIBUTION_TARGET_UPDATE";
  job_id: string;
  agent_id: string;
  hostname?: string;
  status: string;
  progress: number;
  downloaded_bytes: number;
  total_bytes: number;
};

export function useFileDistribution() {
  const socketRef = useRef<WebSocket | null>(null);
  const [updates, setUpdates] = useState<Record<string, TargetUpdate>>({});

  useEffect(() => {
    const socket = new WebSocket(process.env.NEXT_PUBLIC_WS_URL!);
    socketRef.current = socket;

    socket.addEventListener("message", (event) => {
      const message = JSON.parse(event.data);
      if (message.type === "FILE_DISTRIBUTION_TARGET_UPDATE") {
        const key = `${message.job_id}:${message.agent_id}`;
        setUpdates((current) => ({ ...current, [key]: message }));
      }
    });

    return () => socket.close();
  }, []);

  function distributeToAgents(fileId: string, agentIds: string[]) {
    const socket = socketRef.current;
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      throw new Error("WebSocket is not connected");
    }
    socket.send(JSON.stringify({
      type: "FILE_DISTRIBUTE",
      request_id: crypto.randomUUID(),
      file_id: fileId,
      target: { type: "AGENTS", agent_ids: agentIds },
    }));
  }

  return { distributeToAgents, updates };
}
```

เก็บ `request_id` ไว้จนได้รับ `FILE_DISTRIBUTION_CREATED` หาก connection หลุดก่อน response ให้ reconnect แล้วส่ง payload เดิม

## Dashboard State ที่แนะนำ

ใช้ key `(job_id, agent_id)` สำหรับ target rows และใช้ event ล่าสุดแทนค่าเดิม อย่าบวก progress events เข้าด้วยกัน เพราะ event เป็น snapshot ไม่ใช่ delta

เมื่อ reconnect ปัจจุบัน Server ยังไม่มี query/snapshot message สำหรับดึง job state ย้อนหลัง Frontend จึงควรโหลด initial job/target state ผ่าน REST API เมื่อ API ดังกล่าวถูกเพิ่ม หรือเก็บ state จาก backend application layer ที่มีอยู่

## Retry

Schema รองรับการสร้าง temporary grant ใหม่ แต่ message `FILE_DISTRIBUTION_RETRY` ยังไม่ได้ expose ใน implementation ปัจจุบัน UI ไม่ควรแสดงปุ่ม Retry จน backend handler นี้พร้อม และต้องไม่ reuse `download_url` จาก attempt เดิม

Every frontend command revalidates the handshake session against the database. Expired or revoked sessions and inactive users receive ขาดการ login and the connection closes. Do not include session tokens in command JSON.

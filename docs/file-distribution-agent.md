# File Distribution Protocol สำหรับ Agent

เอกสารนี้อธิบาย protocol ที่ Agent ใช้รับคำสั่ง ดาวน์โหลดไฟล์ผ่าน HTTPS และรายงาน progress/result กลับทาง WebSocket

## ภาพรวม

```text
WebSocket Server -- DOWNLOAD_FILE --> Agent
Agent -- HTTPS GET --> /files/download/{token}
Agent -- FILE_DOWNLOAD_PROGRESS --> WebSocket Server
Agent -- FILE_DOWNLOAD_RESULT --> WebSocket Server
```

ไฟล์ไม่ถูกส่งผ่าน WebSocket โดยเด็ดขาด WebSocket ใช้ส่งเฉพาะ JSON command, progress และ result

## รับคำสั่งดาวน์โหลด

เมื่อ Agent online และเป็น target ของ job จะได้รับ JSON:

```json
{
  "type": "DOWNLOAD_FILE",
  "job_id": "8b66e1fd-987f-4cb3-a316-a00c5dc47b7c",
  "file_id": "2f07cc9a-9cc7-4ff3-8b40-c38965a25bd9",
  "filename": "example.zip",
  "size": 58382912,
  "sha256": "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
  "download_url": "https://server.example.com/files/download/opaque-token",
  "expires_at": "2026-09-01T15:30:00Z"
}
```

แต่ละ Agent ได้ token คนละตัว Token ผูกกับ `job_id`, `file_id`, `agent_id` และวันหมดอายุ ห้าม cache URL เพื่อใช้กับ job หรือ Agent อื่น

## ดาวน์โหลดผ่าน HTTPS

Agent ต้องส่ง UUID ของตัวเองใน header:

```http
GET /files/download/opaque-token HTTP/1.1
Host: server.example.com
X-Agent-ID: 86dbfbcc-a13c-4ffc-adab-7187018273e0
```

Response สำเร็จเป็น streaming response พร้อม `Content-Type`, `Content-Length` และ `Content-Disposition`

สถานะ HTTP ที่ควรรองรับ:

| Status | ความหมาย | แนวทาง |
| --- | --- | --- |
| `200` / `206` | ดาวน์โหลดได้ | stream ลง temporary file |
| `401` | token ผิด, หมดอายุ หรือเป็นของ Agent อื่น | รายงาน `FAILED`; อย่า retry URL เดิม |
| `403` | storage path ไม่ผ่าน policy | รายงาน `FAILED` |
| `404` | ไม่มีไฟล์ใน storage | รายงาน `FAILED` |
| `409` | size ใน storage ไม่ตรงฐานข้อมูล | รายงาน `FAILED` |
| `5xx` | server/storage ขัดข้องชั่วคราว | retry แบบ exponential backoff ก่อน URL หมดอายุ |

ควรดาวน์โหลดลงไฟล์ชั่วคราวใน directory ปลายทาง แล้ว rename แบบ atomic หลัง SHA-256 ถูกต้อง ห้าม overwrite ไฟล์ปลายทางก่อน verify สำเร็จ

## ส่ง Progress

ระหว่างดาวน์โหลดให้ส่งไม่เกินประมาณหนึ่งครั้งต่อวินาที:

```json
{
  "type": "FILE_DOWNLOAD_PROGRESS",
  "job_id": "8b66e1fd-987f-4cb3-a316-a00c5dc47b7c",
  "agent_id": "86dbfbcc-a13c-4ffc-adab-7187018273e0",
  "downloaded_bytes": 10485760,
  "total_bytes": 58382912,
  "progress": 17
}
```

เงื่อนไข:

- `progress` ต้องอยู่ระหว่าง `0` ถึง `100`
- byte counters ต้องไม่ติดลบ
- `agent_id` ต้องตรงกับ Agent ของ WebSocket connection
- Server forward event ที่ valid แบบ realtime แต่ persist ตัวเลขเมื่อเพิ่มอย่างน้อย 5% เพื่อลด PostgreSQL writes

## รายงานผลสำเร็จ

Agent ต้องคำนวณ SHA-256 จากไฟล์ที่ดาวน์โหลดจริง ไม่ใช่คัดลอกค่าจาก command:

```json
{
  "type": "FILE_DOWNLOAD_RESULT",
  "job_id": "8b66e1fd-987f-4cb3-a316-a00c5dc47b7c",
  "agent_id": "86dbfbcc-a13c-4ffc-adab-7187018273e0",
  "file_id": "2f07cc9a-9cc7-4ff3-8b40-c38965a25bd9",
  "status": "COMPLETED",
  "bytes_downloaded": 58382912,
  "sha256": "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
}
```

Server ตรวจ SHA-256 กับฐานข้อมูลอีกครั้ง หากไม่ตรงจะบันทึก target เป็น `FAILED` และ `HASH_MISMATCH`

## รายงานความล้มเหลว

```json
{
  "type": "FILE_DOWNLOAD_RESULT",
  "job_id": "8b66e1fd-987f-4cb3-a316-a00c5dc47b7c",
  "agent_id": "86dbfbcc-a13c-4ffc-adab-7187018273e0",
  "file_id": "2f07cc9a-9cc7-4ff3-8b40-c38965a25bd9",
  "status": "FAILED",
  "bytes_downloaded": 58382912,
  "error_code": "HASH_MISMATCH",
  "error_message": "downloaded file checksum does not match"
}
```

แนะนำ error codes: `HTTP_ERROR`, `TOKEN_EXPIRED`, `DISK_FULL`, `WRITE_FAILED`, `SIZE_MISMATCH`, `HASH_MISMATCH`, `CANCELLED`

อย่าส่ง path ภายในเครื่อง, token, credential หรือข้อมูลลับใน `error_message`

## ตัวอย่าง Go สำหรับ Download

```go
request, err := http.NewRequestWithContext(ctx, http.MethodGet, command.DownloadURL, nil)
if err != nil {
    return err
}
request.Header.Set("X-Agent-ID", agentID)

response, err := http.DefaultClient.Do(request)
if err != nil {
    return err
}
defer response.Body.Close()
if response.StatusCode != http.StatusOK {
    return fmt.Errorf("download returned %s", response.Status)
}

temporary, err := os.CreateTemp(destinationDirectory, ".download-*")
if err != nil {
    return err
}
defer os.Remove(temporary.Name())

hasher := sha256.New()
written, err := io.Copy(io.MultiWriter(temporary, hasher), response.Body)
if err != nil {
    temporary.Close()
    return err
}
if written != command.Size {
    temporary.Close()
    return fmt.Errorf("size mismatch")
}
actualHash := hex.EncodeToString(hasher.Sum(nil))
if !strings.EqualFold(actualHash, command.SHA256) {
    temporary.Close()
    return fmt.Errorf("hash mismatch")
}
if err := temporary.Close(); err != nil {
    return err
}
return os.Rename(temporary.Name(), destinationPath)
```

Production ต้องใช้ HTTPS และตั้ง timeout ของ HTTP client ให้เหมาะกับขนาดไฟล์ ห้ามใช้ timeout 10 นาทีของ URL เป็น HTTP client timeout โดยอัตโนมัติ เพราะ download ที่เริ่มก่อนหมดอายุอาจใช้เวลานานกว่านั้น

## Disconnect และ Retry

การหลุดของ WebSocket ระหว่าง HTTP download ไม่ได้ยกเลิก HTTP request โดยอัตโนมัติ Agent สามารถดาวน์โหลดต่อและส่ง result หลัง reconnect ได้ ตราบใดที่ token และ target ยัง valid

เมื่อ Server เพิ่ม retry command ในอนาคต Agent ต้องถือว่า command ที่มี `job_id` เดิมแต่ `download_url` ใหม่เป็น attempt ใหม่ และห้าม reuse URL เก่า


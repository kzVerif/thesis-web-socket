# เอกสารการใช้งาน Auth API

## ข้อมูลทั่วไป

- Base URL: `http://localhost:8080`
- Request body ใช้ `Content-Type: application/json`
- ระบบยืนยันตัวตนด้วย session cookie ชื่อ `__Host-session`
- Session มีอายุ 7 วันนับจากเวลาที่ Login
- Cookie เป็น `HttpOnly`, `Secure`, `SameSite=Lax` และใช้ได้กับ path `/`
- Session token จริงไม่ถูกเก็บในฐานข้อมูล ระบบเก็บค่า SHA-256 hash ไว้ใน `user_sessions.token_hash`

Endpoint ที่ไม่ต้อง Login:

- `POST /api/auth/register`
- `POST /api/auth/login`

Endpoint อื่นทั้งหมดในเอกสารนี้ต้อง Login ด้วยบัญชีสถานะ `ACTIVE`

## 1. สมัครบัญชี

```http
POST /api/auth/register
Content-Type: application/json
```

Request body:

```json
{
  "username": "newuser",
  "fullname": "New User",
  "email": "newuser@example.com",
  "password": "strong-password",
  "confirmPassword": "strong-password"
}
```

ข้อกำหนด:

- ต้องส่งข้อมูลทุกฟิลด์
- รหัสผ่านต้องมีอย่างน้อย 8 ตัวอักษร
- `password` และ `confirmPassword` ต้องตรงกัน
- Username และ Email ต้องไม่ซ้ำกับบัญชีเดิม
- Email จะถูกแปลงเป็นตัวพิมพ์เล็กก่อนบันทึก
- บัญชีใหม่จะได้รับ Role `VIEWER`
- บัญชีใหม่จะมีสถานะ `DISABLED` และยัง Login ไม่ได้
- ผู้มี Permission `users.manage` ต้องเปลี่ยนสถานะบัญชีเป็น `ACTIVE` ผ่าน Users API ก่อน

ตัวอย่าง:

```bash
curl -i \
  -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "username":"newuser",
    "fullname":"New User",
    "email":"newuser@example.com",
    "password":"strong-password",
    "confirmPassword":"strong-password"
  }'
```

Response สำเร็จ: HTTP `201 Created`

```json
{
  "message": "สร้างผู้ใช้สำเร็จ",
  "user_id": "11111111-1111-1111-1111-111111111111",
  "username": "newuser",
  "email": "newuser@example.com",
  "fullname": "New User"
}
```

ข้อผิดพลาดที่เป็นไปได้:

- HTTP `400`: ข้อมูลไม่ครบ รูปแบบข้อมูลไม่ถูกต้อง รหัสผ่านสั้นเกินไป หรือยืนยันรหัสผ่านไม่ตรงกัน
- HTTP `409`: Username หรือ Email ถูกใช้งานแล้ว
- HTTP `500`: เข้ารหัสรหัสผ่านไม่ได้, ไม่มี Role `VIEWER` หรือบันทึกข้อมูลไม่ได้

## 2. เข้าสู่ระบบ

```http
POST /api/auth/login
Content-Type: application/json
```

Request body:

```json
{
  "username": "admin",
  "password": "your-password"
}
```

ตัวอย่าง Login และบันทึก cookie ลงไฟล์:

```bash
curl -i -c cookies.txt \
  -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}'
```

Response สำเร็จ: HTTP `200 OK`

```json
{
  "message": "เข้าสู่ระบบสำเร็จ",
  "session_id": "22222222-2222-2222-2222-222222222222",
  "username": "admin"
}
```

Response จะมี header `Set-Cookie` สำหรับ cookie `__Host-session` หลังจากนั้นให้ส่ง cookie ไปกับ endpoint ที่ต้อง Login:

```bash
curl -i -b cookies.txt http://localhost:8080/api/auth/me
```

ข้อผิดพลาดที่เป็นไปได้:

- HTTP `400`: ไม่ส่ง Username/Password หรือรูปแบบข้อมูลไม่ถูกต้อง
- HTTP `401`: Username หรือ Password ไม่ถูกต้อง
- HTTP `403`: บัญชีมีสถานะอื่นที่ไม่ใช่ `ACTIVE`
- HTTP `500`: ไม่สามารถสร้าง Session หรือเข้าสู่ระบบได้

> Cookie กำหนดเป็น `Secure` จึงออกแบบให้ใช้งานผ่าน HTTPS ในระบบจริง หากทดสอบ HTTPS ด้วย certificate ภายใน สามารถเพิ่ม `-k` ให้ `curl` ได้

## 3. ดูข้อมูลผู้ใช้ปัจจุบัน

```http
GET /api/auth/me
```

```bash
curl -i -b cookies.txt http://localhost:8080/api/auth/me
```

Response สำเร็จ: HTTP `200 OK`

```json
{
  "id": "11111111-1111-1111-1111-111111111111",
  "username": "admin",
  "email": "admin@example.com",
  "display_name": "Administrator",
  "role_id": "33333333-3333-3333-3333-333333333333",
  "role": "ADMINISTRATOR"
}
```

หาก `email` หรือ `display_name` ไม่มีค่า endpoint ปัจจุบันจะตอบเป็น string ว่าง `""`

## 4. ดู Session ที่กำลังใช้งาน

```http
GET /api/auth/sessions
```

```bash
curl -i -b cookies.txt http://localhost:8080/api/auth/sessions
```

Response สำเร็จ: HTTP `200 OK`

```json
{
  "sessions": [
    {
      "id": "22222222-2222-2222-2222-222222222222",
      "ip_address": "127.0.0.1",
      "user_agent": "Mozilla/5.0",
      "created_at": "2026-08-11T10:00:00+07:00",
      "last_activity_at": "2026-08-11T10:10:00+07:00",
      "expires_at": "2026-08-18T10:00:00+07:00",
      "current": true
    }
  ]
}
```

- แสดงเฉพาะ Session ที่ยังไม่ถูกยกเลิกและยังไม่หมดอายุ
- `current: true` หมายถึง Session ที่ใช้เรียก request ปัจจุบัน
- หากไม่มี Session ระบบจะตอบ `"sessions": []`

## 5. ยกเลิก Session รายการเดียว

```http
DELETE /api/auth/sessions/:id
```

สามารถยกเลิกได้เฉพาะ Session ที่เป็นของผู้ใช้ปัจจุบัน

```bash
curl -i -b cookies.txt \
  -X DELETE http://localhost:8080/api/auth/sessions/22222222-2222-2222-2222-222222222222
```

Response สำเร็จ: HTTP `200 OK`

```json
{
  "message": "ยกเลิกเซสชันสำเร็จ"
}
```

หากยกเลิก Session ปัจจุบัน ระบบจะลบ cookie และ request ถัดไปต้อง Login ใหม่ หากไม่พบ Session หรือ Session ถูกยกเลิกแล้ว ระบบตอบ HTTP `404`

## 6. ออกจากระบบ

```http
POST /api/auth/logout
```

ยกเลิกเฉพาะ Session ปัจจุบันและลบ cookie

```bash
curl -i -b cookies.txt \
  -X POST http://localhost:8080/api/auth/logout
```

Response สำเร็จ: HTTP `200 OK`

```json
{
  "message": "ออกจากระบบสำเร็จ"
}
```

## 7. ออกจากระบบทุกอุปกรณ์

```http
POST /api/auth/logout-all
```

ยกเลิก Session ที่ยังใช้งานอยู่ทั้งหมดของผู้ใช้ รวมถึง Session ปัจจุบัน และลบ cookie

```bash
curl -i -b cookies.txt \
  -X POST http://localhost:8080/api/auth/logout-all
```

Response สำเร็จ: HTTP `200 OK`

```json
{
  "message": "ออกจากระบบทุกอุปกรณ์สำเร็จ"
}
```

## 8. เปลี่ยนรหัสผ่าน

```http
POST /api/auth/change-password
Content-Type: application/json
```

Request body:

```json
{
  "current_password": "current-password",
  "new_password": "new-strong-password",
  "confirm_password": "new-strong-password"
}
```

ข้อกำหนด:

- `current_password` ต้องตรงกับรหัสผ่านปัจจุบัน
- `new_password` ต้องมีอย่างน้อย 8 ตัวอักษร
- รหัสผ่านใหม่ต้องไม่ซ้ำกับรหัสผ่านเดิม
- `new_password` และ `confirm_password` ต้องตรงกัน

```bash
curl -i -b cookies.txt \
  -X POST http://localhost:8080/api/auth/change-password \
  -H "Content-Type: application/json" \
  -d '{
    "current_password":"current-password",
    "new_password":"new-strong-password",
    "confirm_password":"new-strong-password"
  }'
```

Response สำเร็จ: HTTP `200 OK`

```json
{
  "message": "เปลี่ยนรหัสผ่านสำเร็จ กรุณาเข้าสู่ระบบใหม่"
}
```

เมื่อเปลี่ยนรหัสผ่านสำเร็จ:

1. อัปเดต `password_changed_at`
2. ยกเลิก Session ทั้งหมดของผู้ใช้
3. ลบ cookie ปัจจุบัน
4. ผู้ใช้ต้อง Login ใหม่ด้วยรหัสผ่านใหม่

## ข้อผิดพลาดร่วมของ Endpoint ที่ต้อง Login

| HTTP Status | กรณี | Response |
| --- | --- | --- |
| `401 Unauthorized` | ไม่มี cookie, Session ไม่ถูกต้อง/หมดอายุ/ถูกยกเลิก หรือบัญชีไม่ใช่ `ACTIVE` | `{"error":"ไม่ได้รับอนุญาตให้เข้าใช้งาน"}` |
| `500 Internal Server Error` | ตรวจสอบ Session กับฐานข้อมูลไม่ได้ | `{"error":"ไม่สามารถตรวจสอบเซสชันได้"}` |

เมื่อ Session ผ่านการตรวจสอบ ระบบจะอัปเดต `last_activity_at` ทุกครั้ง

## สรุป Endpoint

| Method | Endpoint | ต้อง Login | รายละเอียด |
| --- | --- | --- | --- |
| `POST` | `/api/auth/register` | ไม่ต้อง | สมัครบัญชีสถานะ `DISABLED` |
| `POST` | `/api/auth/login` | ไม่ต้อง | Login และสร้าง Session |
| `GET` | `/api/auth/me` | ต้อง | ดูข้อมูลผู้ใช้ปัจจุบัน |
| `GET` | `/api/auth/sessions` | ต้อง | ดู Session ที่ยังใช้งานอยู่ |
| `DELETE` | `/api/auth/sessions/:id` | ต้อง | ยกเลิก Session รายการเดียว |
| `POST` | `/api/auth/logout` | ต้อง | ออกจาก Session ปัจจุบัน |
| `POST` | `/api/auth/logout-all` | ต้อง | ออกจากระบบทุกอุปกรณ์ |
| `POST` | `/api/auth/change-password` | ต้อง | เปลี่ยนรหัสผ่านและยกเลิกทุก Session |

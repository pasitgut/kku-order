# DocFlow

ระบบจัดการและสกัดข้อมูลเอกสารคำสั่งแต่งตั้งสำหรับวิทยาลัยการคอมพิวเตอร์ มหาวิทยาลัยขอนแก่น

โปรเจกต์นี้เป็นโครงตั้งต้นที่รันได้สำหรับ TOR ฉบับร่าง โดยแยกเป็น

- `Next.js 16` App Router สำหรับเว็บแอปและหน้าจอการทำงานหลัก
- `Go + Gin + GORM` สำหรับ REST API
- `MySQL 8.4` สำหรับฐานข้อมูลและประวัติการแก้ไข
- `Docker Compose` สำหรับจำลอง MySQL และ API ในเครื่อง
- Next.js ใช้ same-origin proxy `/backend/*` ไปยัง API container เพื่อให้ browser ของผู้ใช้เชื่อมต่อ backend จริงได้โดยไม่อ้าง `localhost` ของเครื่องผู้ใช้
- `IMPORT_DIR` สำหรับดึงเอกสารจากโฟลเดอร์ที่กำหนดทุก 5 นาที โดยไม่สร้างรายการซ้ำจาก file hash
- `backend/migrations/*.sql` สำหรับ migration แบบมี version และ `schema_migrations`
- `backend/one-ocr` สำหรับ OCR ภาษาไทยที่ทำงานภายในระบบ
- Scheduled sync สำหรับดึงรายชื่อผู้ใช้งานจากระบบของวิทยาลัยทุกวันเวลา 05:00 น. ตามเวลา `Asia/Bangkok`

## หน้าจอที่มีใน MVP

- `/` ภาพรวมระบบและคิวตรวจสอบ
- `/upload` นำเข้าเอกสารหลายไฟล์ รองรับ PDF, DOCX, XLSX และ CSV สูงสุด 50 MB ต่อไฟล์
- `/documents` รายการเอกสารพร้อมตัวกรองและสถานะ
- `/documents/:id` หน้าตรวจสอบ PDF แบบสองฝั่งและ Data Grid ของข้อมูลสกัด
- `/search` ค้นหาข้อมูลด้วยชื่อ ตำแหน่ง บทบาท ประเภทคำสั่ง และช่วงวันที่ พร้อมพิมพ์/บันทึก PDF
- `/settings` ผู้ใช้งาน สิทธิ์ และนโยบายข้อมูล

## เริ่มต้นฝั่งเว็บ

```powershell
npm install
npm run dev
```

เปิด `http://localhost:3000`

## เริ่มฐานข้อมูลและ API ด้วย Docker

ขั้นตอน build image ด้วย Docker, ส่งไฟล์ `.tar` ผ่าน SCP และ deploy บน server ด้วย Podman พร้อม Cloudflared `proxy-net` อยู่ที่ [DEPLOY_PODMAN.md](DEPLOY_PODMAN.md)

ต้องมี Docker Desktop หรือ Docker Engine บนเครื่องที่จะรันคำสั่ง

```powershell
if (Test-Path .env.local) {
  docker compose --env-file .env.local up --build
} else {
  docker compose up --build
}
```

API container จะโหลด `.env.local` เข้าไปโดยตรงเมื่อไฟล์มีอยู่ด้วย ดังนั้น `API_KEY` จะถูกส่งให้ Backend แม้เรียก `docker compose up` โดยไม่ได้ใส่ `--env-file`; ห้าม commit ไฟล์นี้และห้ามใส่ API key ใน Bruno

เมื่อติดตั้ง Docker แล้ว สามารถตรวจการเชื่อมต่อทั้ง stack ได้ด้วย

```powershell
.\scripts\verify-stack.ps1
```

สคริปต์จะเลือก `.env.local` (ถ้ามี) เพื่อส่ง `API_KEY` เข้า API container, แล้วตรวจ API health, จำนวน migration ใน MySQL และ HTTP status ของ Frontend โดยไม่แสดงค่า `API_KEY`

หลัง stack healthy ให้ทดสอบการเชื่อมต่อจริงและ one-ocr ด้วย PDF ตัวอย่าง:

```powershell
.\scripts\verify-sample-pdfs.ps1
```

สคริปต์จะ upload `malicious_file__open_with_care.pdf` และ `kku.pdf` ผ่าน API, รอ worker ประมวลผล และรายงานจำนวนหน้า รายชื่อ ฟิลด์ และ OCR provider จากข้อมูลที่อ่านกลับจากฐานข้อมูล

ถ้ารัน backend โดยตรง ให้รันจากโฟลเดอร์ `backend` ด้วย `go run .` เพื่อให้ Go compile ไฟล์ทั้งหมดใน package เดียวกัน โดยต้องมี MySQL ที่สร้าง database/user ตามค่า `DB_DSN` ก่อน คำสั่งนี้ไม่ได้อ่าน `.env.local` ให้อัตโนมัติ จึงต้องตั้งค่า environment เอง เช่น

```powershell
cd backend
$env:DB_DSN = 'docflow:docflow@tcp(localhost:3307)/docflow?charset=utf8mb4&parseTime=True&loc=Local'
go run .
```

ถ้าใช้ MySQL ที่ติดตั้งอยู่บนเครื่องแล้ว ต้องสร้างสิทธิ์ให้ตรงกับ DSN ก่อน (ใช้บัญชี administrator ของ MySQL):

```sql
CREATE DATABASE IF NOT EXISTS docflow CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'docflow'@'localhost' IDENTIFIED BY 'docflow';
ALTER USER 'docflow'@'localhost' IDENTIFIED BY 'docflow';
GRANT ALL PRIVILEGES ON docflow.* TO 'docflow'@'localhost';
FLUSH PRIVILEGES;
```

ถ้าจะใช้ MySQL80 เดิมที่พอร์ต `3306` ให้เปลี่ยนค่าในคำสั่งเป็น `tcp(localhost:3306)` หลังสร้าง user สำเร็จ

เมื่อใช้ Docker Compose, API container ต้องใช้ `mysql:3306` ภายใน network และไม่ควรสตาร์ท API ซ้ำด้วย `go run .` บน host

สำหรับ production ให้ตั้ง `AUTH_REQUIRED=true` และให้ reverse proxy/SSO ของมหาวิทยาลัยส่ง header `X-Auth-Request-User` กับ `X-Auth-Request-Role` หลังตรวจสอบตัวตนแล้ว โดย role ที่เขียนข้อมูลได้คือ `ADMIN` หรือ `STAFF`; ผู้ใช้ที่ไม่ได้ระบุ role จะถูกปฏิบัติเป็น `VIEWER` เมื่อเปิด auth แบบบังคับ (กำหนด `AUTH_DEFAULT_ROLE=VIEWER` ได้อย่างชัดเจน)

ไม่ควรใช้ `go run main.go` เพราะคำสั่งนี้จะ compile เฉพาะ `main.go` และจะมองไม่เห็นฟังก์ชันที่อยู่ในไฟล์ `sync.go` เช่น `NewUserSyncService` และ `userSyncHandler`

หลังจากเริ่มแล้ว

- MySQL จาก Docker ที่เข้าถึงจาก host: `localhost:3307` (ภายใน Compose ใช้ `mysql:3306`)
- API: `http://localhost:8080`
- Web: `http://localhost:3000`
- Health check: `http://localhost:8080/health`

เมื่อเปิดผ่าน Web ให้ Frontend เรียก API ผ่าน `http://localhost:3000/backend/...` โดย Next.js จะส่งต่อไปยัง `http://api:8080` ภายใน Compose network ส่วน `http://localhost:8080` ใช้สำหรับตรวจ API โดยตรงเท่านั้น Compose จะบังคับให้ใช้ proxy นี้เสมอ เพื่อไม่ให้ค่า `NEXT_PUBLIC_API_URL` ที่ค้างใน shell ทำให้ browser ยิงไปยัง localhost ผิดตัว

## Sync ผู้ใช้งานจาก External API

Backend เรียกข้อมูลจาก

```text
GET https://fs.computing.kku.ac.th/api/ext/v1/users
Authorization: Bearer ${API_KEY}
```

ตั้งค่าใน `.env` หรือ environment ของเครื่อง server โดยห้าม commit API key ลง repository

```env
EXTERNAL_USERS_URL=https://fs.computing.kku.ac.th/api/ext/v1/users
API_KEY=your-api-key
USER_SYNC_CRON=0 5 * * *
USER_SYNC_TZ=Asia/Bangkok
```

ระบบจะสร้าง `user_sync_runs` เป็นประวัติการทำงาน และเก็บข้อมูลผู้ใช้งานไว้ใน `directory_users` โดยใช้ `external_id` (จาก `user_id`) เป็น key หลักสำหรับ upsert หาก payload มีการเปลี่ยนแปลง ระบบจะ update ข้อมูลในฐานข้อมูล หาก payload เหมือนเดิมก็ยัง refresh mapping และเวลา `last_seen_at`/`synced_at` เพื่อเติมคอลัมน์ใหม่ให้ข้อมูลเดิมได้

สั่ง sync เองได้ที่

```text
POST /api/v1/sync/users
GET  /api/v1/sync/runs
```

ตัวอย่าง request สำหรับ Bruno อยู่ในโฟลเดอร์ `bruno/DocFlow` โดยใช้ `POST http://localhost:8080/api/v1/sync/users` เพื่อให้ backend ยิงไปยัง External API ด้วย `API_KEY` ที่อยู่ใน environment ของ container ไม่ต้องใส่ key ลงใน Bruno

ตัว parser รองรับ response แบบ array และ envelope ที่ใช้บ่อย เช่น `data`, `users`, `items`, `results` และเก็บ payload ต้นฉบับใน `raw_payload` สำหรับตรวจสอบย้อนหลัง

รูปแบบ response ที่ตรวจสอบกับ API จริงของวิทยาลัยเป็น envelope `{ data, filters, paging, success }` โดยข้อมูลผู้ใช้จะอยู่ใน `data` ระบบ map ฟิลด์ API ลง `directory_users` ตามชื่อจริง ได้แก่ `user_id`, `prefix`, `user_fname`, `user_lname`, `gender`, `email`, `tel`, `tel_format`, `position_title`, `position_en`, `prefix_position_en`, `manage_position`, `name_en`, `suffix_en`, `scopus_id`, `scholar_author_id`, `lab_name`, `room`, `cp_web_id`, `role_id`, `role_name`, `is_active` และ `updated_at` พร้อมเก็บ payload ต้นฉบับไว้ใน `raw_payload`

## ฐานข้อมูลและ migration

เมื่อ API เริ่มทำงาน ระบบจะรัน migration ใน `backend/migrations` ตามลำดับและบันทึก version ไว้ใน `schema_migrations` ไม่ใช้ `AutoMigrate` เป็นตัวควบคุม schema หลัก ตารางที่สร้างครอบคลุม:

- `documents` และ `extracted_fields` สำหรับเอกสารและข้อมูลสกัด
- `appointments` สำหรับชื่อ ตำแหน่ง บทบาท หน้าที่ ความมั่นใจ ตำแหน่งบนหน้าเอกสาร และการยืนยัน
- `document_references` สำหรับการอ้างอิงประกาศเพิ่มเติม
- `document_revisions` และ `audit_logs` สำหรับประวัติแก้ไขและกิจกรรม
- `retention_until` และ `import_source` สำหรับนโยบายเก็บข้อมูล 10 ปีและตรวจสอบแหล่งนำเข้า
- `directory_users` สำหรับฐานข้อมูลบุคลากรจาก External API
- `expiry_notifications` สำหรับแจ้งเตือนวาระ 3, 2 และ 1 เดือน
- `user_sync_runs` สำหรับตรวจสอบรอบการ sync และจำนวนข้อมูลที่เพิ่ม/แก้ไข

เอกสารจะเก็บ hash ของไฟล์เพื่อแจ้งเตือนเอกสารซ้ำก่อนบันทึก, `ocr_pages`/`ocr_lines` เก็บข้อความและกรอบตำแหน่งจาก one-ocr, และ `extracted_fields` รองรับข้อความต้นฉบับ หน้ากระดาษ และกรอบตำแหน่งข้อมูลสำหรับ PDF viewer

ถ้ารัน Next.js บนเครื่อง host ให้ตั้งค่าโดยคัดลอก `.env.example` เป็น `.env.local`

```powershell
Copy-Item .env.example .env.local
```

## API เบื้องต้น

```text
GET  /api/v1/documents?q=คำค้น&status=PROCESSING
GET  /api/v1/documents/:id
GET  /api/v1/documents/:id/file
GET  /api/v1/documents/:id/revisions
GET  /api/v1/documents/:id/audit
POST /api/v1/documents              multipart field: file
PATCH /api/v1/documents/:id         แก้ไขข้อมูลสกัดและบันทึก revision
POST /api/v1/documents/:id/confirm  JSON: { "userId": "staff-id" }
GET  /api/v1/search?q=ชื่อบุคคล
GET  /api/v1/reports/workload?q=ชื่อบุคคล&dateFrom=YYYY-MM-DD&dateTo=YYYY-MM-DD
GET  /api/v1/dashboard
GET  /api/v1/directory-users
POST /api/v1/sync/users
```

หลัง upload ระบบบันทึกไฟล์และ metadata ลง MySQL ก่อน จากนั้นประมวลผล background ด้วย `pdftoppm` และ `one-ocr` ภายในเครื่อง เก็บหน้า/บรรทัด/กรอบตำแหน่ง/ความมั่นใจ และตั้งสถานะให้เจ้าหน้าที่ตรวจสอบก่อนยืนยัน สำหรับ DOCX/XLSX/CSV ระบบนำเข้าข้อความแบบ structured โดยตรง

ค่า OCR ที่ใช้บ่อย:

```env
OCR_LANGUAGE=thai
OCR_DPI=200
OCR_MAX_SIDE=1536
OCR_PREPROCESS=none
EXPIRY_CRON=10 5 * * *
```

ค่าเริ่มต้นของ OCR ให้ตรงกับ `sample-code`: render PDF ที่ 200 DPI, ใช้ภาพ RGB ต้นฉบับ, ไม่ deskew เพิ่มเติม และกำหนด one-ocr เป็นแนว 0 องศา เพื่อให้ bounding box ตรงกับภาพที่แสดงใน Viewer หากเอกสารเอียงจริงค่อยตั้ง `OCR_PREPROCESS=standard` เป็นรายสภาพแวดล้อม

การแจ้งเตือนหมดวาระในระบบจะถูกสร้างที่ 3, 2 และ 1 เดือนล่วงหน้า หากตั้งค่า `SMTP_HOST`, `SMTP_FROM` และค่าบัญชี SMTP ระบบจะส่งอีเมลให้ผู้ได้รับแต่งตั้งที่จับคู่กับ `directory_users.email` และบันทึกสถานะการส่งใน `expiry_notifications`

วางไฟล์ PDF, DOCX, XLSX หรือ CSV ในโฟลเดอร์ `incoming` ที่ root ของโปรเจกต์ ระบบจะเห็นไฟล์ผ่าน bind mount `/app/incoming`, ตรวจไฟล์ที่เขียนเสร็จแล้ว และนำเข้าอัตโนมัติทุก 5 นาทีตาม `IMPORT_CRON`

การทดสอบ OCR ในเครื่องต้องมี dependencies ใน `backend/one-ocr/.venv` และ binary `pdftoppm`/`pdfinfo`; Dockerfile ติดตั้ง Poppler และ Python dependencies ให้อัตโนมัติ

## ตรวจสอบโค้ด

```powershell
npm run lint
npm run build
```

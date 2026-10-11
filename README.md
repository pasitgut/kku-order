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

ถ้ารัน backend โดยตรง ให้รันจากโฟลเดอร์ `backend` ด้วย `go run .` เพื่อให้ Go compile ไฟล์ทั้งหมดใน package เดียวกัน โดยต้องมี MySQL ที่สร้าง database/user ตามค่า `DB_DSN` ก่อน Backend จะโหลด `.env.local` จาก root ของโปรเจกต์ให้อัตโนมัติ โดย environment ที่กำหนดไว้ใน process จะมีลำดับความสำคัญสูงกว่า หากไม่มีไฟล์ที่ root จะใช้ `backend/.env.local` เป็น fallback

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

### นำเข้าไฟล์จาก Google Drive อัตโนมัติ

ตั้งค่าที่ **ตั้งค่า → Google Drive** ค่าเริ่มต้นดึงทุกวันเวลา **05:00 เขตเวลา Asia/Bangkok** เปลี่ยนลิงก์โฟลเดอร์ เวลา และเขตเวลาได้จากหน้าเว็บโดยไม่ต้องรีสตาร์ต ปิดนำเข้าอัตโนมัติได้ และใช้ปุ่ม **ดึงไฟล์ตอนนี้** เพื่อเริ่มรอบแรกหรือทำงานด้วยตนเอง

#### สร้างอีเมลระบบสำหรับแชร์โฟลเดอร์

1. สร้างหรือเลือก Google Cloud project ขององค์กร ติดตั้ง [Google Cloud CLI](https://cloud.google.com/sdk/docs/install) และลงชื่อเข้าใช้ด้วย `gcloud auth login` บัญชีผู้ตั้งค่าต้องมีสิทธิ์เปิด API สร้าง service account และสร้าง key
2. รันจาก root ของโปรเจกต์ (แทน `YOUR_PROJECT_ID` ด้วย Project ID จริง):

   ```powershell
   ./scripts/setup-google-drive.ps1 -ProjectId YOUR_PROJECT_ID
   ```

   สคริปต์เปิด Drive API และ IAM API สร้างบัญชี `docflow-drive-reader` และไฟล์ `secrets/google-drive-service-account.json` พร้อมแสดงอีเมลจริงที่ Google สร้างให้ ไม่เขียนทับ key เดิม บัญชีนี้ไม่ต้องมี IAM role ใน project เพื่ออ่านโฟลเดอร์ที่แชร์โดยตรง หากองค์กรปิดการสร้าง service account key ผู้ดูแล Google Cloud ต้องจัดการนโยบายก่อน
3. แชร์โฟลเดอร์เป้าหมายให้ **อีเมลที่สคริปต์แสดง** เป็น Viewer และตรวจสิทธิ์โฟลเดอร์ย่อยที่จำกัดการเข้าถึง สำหรับ Shared Drive ต้องให้บัญชีเข้าถึงรายการที่ต้องการด้วย อีเมลจะปรากฏพร้อมปุ่มคัดลอกในหน้าตั้งค่าหลัง backend อ่าน credential ได้
4. กำหนด `GOOGLE_DRIVE_CREDENTIALS_FILE` ใน `.env.local` หรือ environment ของ backend โดยใช้ path จริงสำหรับการรันในเครื่อง หรือ path ต่อไปนี้สำหรับ container:

   ```env
   GOOGLE_DRIVE_CREDENTIALS_FILE=/run/secrets/google-drive-service-account.json
   ```

5. เริ่ม backend พร้อม secret mount:

   ```powershell
   docker compose -f docker-compose.yml -f docker-compose.drive.yml up -d --build
   ```

   สำหรับ Podman production วาง key ใน `deploy/secrets/google-drive-service-account.json` และเพิ่ม override `deploy/podman-compose.drive.yml` ตาม [คู่มือติดตั้ง](DEPLOY_PODMAN.md)
6. ไปที่ **ตั้งค่า → Google Drive** วางลิงก์โฟลเดอร์ ทดสอบการเข้าถึง เลือก Google Drive เป็นแหล่งนำเข้า เปิดอัตโนมัติ และบันทึก จากนั้นกด **ดึงไฟล์ตอนนี้** เพื่อเริ่มนำเข้าไฟล์เดิมทั้งหมดได้ทันที

#### การทำงานและการกู้คืน

- ระบบสำรวจรายการไฟล์ทุกโฟลเดอร์ย่อยแบบแบ่งหน้า รวมโฟลเดอร์ว่าง โฟลเดอร์ที่ถูกย้ายเข้ามา และทางลัดไปยังไฟล์หรือโฟลเดอร์ที่ service account เข้าถึงได้ ทางลัดสามารถชี้ออกนอกต้นทางได้ ระบบป้องกันทางลัดวนกลับและรายการซ้ำด้วย ID รองรับ My Drive, Shared Drives และ resource key ในลิงก์
- แต่ละรอบอ่าน metadata ทั้งต้นทางเพื่อค้นหาไฟล์ใหม่หรือไฟล์เดิมที่ถูกย้ายเข้ามา โดย **ไม่ดาวน์โหลดไฟล์ที่มี Drive file ID นำเข้าสำเร็จแล้ว** แม้เปลี่ยนชื่อ ย้าย หรือแก้ไขเนื้อหา ถ้าต้องการแทนที่เอกสารที่นำเข้าแล้ว ให้ใช้การอัปโหลดแทนที่ในระบบ ไฟล์คนละ ID ที่เนื้อหาเหมือนกันต้องดาวน์โหลดครั้งแรกเพื่อตรวจ SHA-256 และจะไม่สร้างเอกสารซ้ำ
- MySQL เก็บประวัติรอบงาน คิวโฟลเดอร์ pagination คิวไฟล์ และบัญชีไฟล์ที่นำเข้าสำเร็จ (`drive_sync_runs`, `drive_sync_folders`, `drive_sync_items`, `drive_imported_files`) เก็บคิวรายละเอียด 10 รอบล่าสุดต่อต้นทางเพื่อลดการเติบโตของฐานข้อมูล ส่วนสรุปรอบงานและบัญชีไฟล์ที่นำเข้าสำเร็จเก็บไว้ต่อเนื่อง บันทึก checkpoint ทุกหน้าและทุกไฟล์ การบันทึกเอกสารและบัญชี Drive ID อยู่ใน transaction เดียวกัน ไฟล์ต้นฉบับไม่ถูกย้ายหรือลบ
- งานเบื้องหลังทำเป็นช่วงละ 10 นาทีและทำต่อใน tick ถัดไปทุกนาที ไม่ผูกกับ HTTP request หาก backend รีสตาร์ตจะทำต่อจากคิวเดิม รอบรายวันที่พลาดขณะปิดระบบจะถูกชดเชยเมื่อเปิดกลับมา งานหลาย backend ใช้ MySQL advisory lock เพื่อป้องกันการดึงพร้อมกัน
- ไฟล์ที่นำเข้าใหม่เข้าสู่คิว OCR ถาวร ประมวลผลครั้งละหนึ่งเอกสารสำหรับ Drive และกู้คืนงานที่ขาดช่วงหลังรีสตาร์ต ข้อผิดพลาด OCR ปรากฏในหน้าเอกสารและสั่งประมวลผลใหม่ได้
- ข้อผิดพลาด quota / HTTP 429 / 5xx มี exponential backoff งานที่ยังล้มเหลวจะลองใหม่แบบเว้นระยะ ส่วนไฟล์และโฟลเดอร์ที่ยังเข้าถึงไม่ได้จะแสดงเป็นรายการปัญหาและลองใหม่ในรอบรายวันถัดไป ถ้าต้นทางไม่มีสิทธิ์เข้าถึง จะพักและตรวจอีกครั้งทุก 5 นาที รอบที่มีไฟล์/โฟลเดอร์ล้มเหลวรายงานว่าเสร็จบางส่วน
- การเปลี่ยนต้นทางพักคิวเดิมไว้ และสร้างคิวของต้นทางใหม่ โดยใช้บัญชี Drive ID เดียวกันเพื่อไม่ดึงไฟล์เดิมซ้ำ กลับมาลิงก์เดิมสามารถทำต่อจากงานเดิมได้ ปิดอัตโนมัติจะหยุดต่อเมื่อจบชุดที่กำลังทำงาน งานที่สั่งด้วยปุ่มยังทำงานต่อได้
- สถานะ ความคืบหน้า จำนวนไฟล์ที่นำเข้า/ซ้ำ/รอดึง/ข้าม/ผิดพลาด รายการปัญหา และประวัติ 10 รอบล่าสุดแสดงในหน้า Google Drive อัปเดตทุก 5 วินาที
- สถานะ JSON จาก importer รุ่นเก่าไม่ใช่แหล่งสถานะอีกต่อไป การอัปเกรดจะสแกนใหม่และเติมบัญชี Drive ID โดยตรวจ hash กับเอกสารเดิมเพื่อป้องกันเอกสารซ้ำ

รองรับ PDF, DOCX, XLSX และ CSV รวม Google Docs เป็น PDF, Google Sheets เป็น XLSX และ Google Slides/Drawings เป็น PDF ไฟล์ต้องไม่เกิน 50 MB; [Google Workspace export จำกัด 10 MB](https://developers.google.com/workspace/drive/api/reference/rest/v3/files/export) ไฟล์ชนิดอื่นหรือเกินขนาดจะแสดงเป็นรายการที่ข้าม และตรวจ metadata ใหม่ในรอบถัดไป หากต้องนำเข้ารูปภาพ วิดีโอ หรือไฟล์ขนาดใหญ่ ต้องขยายความสามารถของตัวประมวลผลก่อน

API เพิ่มเติม: `GET /api/v1/settings/drive/status` อ่านสถานะ, `POST /api/v1/settings/drive/test` ทดสอบสิทธิ์, `POST /api/v1/settings/drive/sync` ตอบ `202` พร้อม run ID หลังบันทึกคิวสำเร็จ การเปลี่ยนค่าหรือสั่งงานต้องเป็น ADMIN

เก็บ service-account JSON ใน secrets ของ backend ไม่เพิ่มใน Git หรือ image ใช้ scope `drive.readonly` และสำรอง MySQL ควบคู่กับ storage volume เพื่อรักษาทั้งไฟล์และประวัติการนำเข้า

การทดสอบ OCR ในเครื่องต้องมี dependencies ใน `backend/one-ocr/.venv` และ binary `pdftoppm`/`pdfinfo`; Dockerfile ติดตั้ง Poppler และ Python dependencies ให้อัตโนมัติ

## ยกเลิกเอกสาร

ผู้ดูแลระบบ (ADMIN) และเจ้าหน้าที่ (STAFF) เปิดหน้าเอกสารแล้วเลือก **ยกเลิกเอกสาร** ระบุเหตุผล 1–2,000 ตัวอักษร และกดยืนยัน ระบบเก็บเหตุผล ผู้ยกเลิก เวลา และประวัติการเปลี่ยนสถานะ โดยเก็บไฟล์ต้นฉบับและข้อมูลที่บันทึกแล้วไว้ดูย้อนหลัง เลือกตัวกรอง **ยกเลิกแล้ว** ในหน้าเอกสารทั้งหมดได้

เอกสารสถานะ `CANCELLED` ไม่สามารถแก้ไข ยืนยัน อัปโหลดทับ หรือประมวลผลซ้ำได้ และไม่รวมในรายงานภาระงานหรือแจ้งเตือนหมดอายุ งาน OCR ที่เริ่มไปแล้วอาจทำงานจนจบ แต่ผลจะไม่เขียนทับเอกสารที่ยกเลิก การแชร์ Drive และบัญชีไฟล์ที่นำเข้าแล้วคงอยู่ จึงไม่ดึงเอกสารเดิมกลับเข้ามาเพราะการยกเลิก

API: `POST /api/v1/documents/:id/cancel` พร้อม JSON `{ "reason": "เหตุผลที่ยกเลิก" }` ตอบเอกสารที่อัปเดตแล้ว การส่งคำขอซ้ำคงเหตุผล ผู้ยกเลิก และเวลาเดิมไว้ ไม่สร้างประวัติซ้ำ เอกสารที่ถูกยกเลิกคืน `409` เมื่อมีคำขอแก้ไขหรือยืนยัน

Migration `0009_document_cancellation.sql` เพิ่มข้อมูลการยกเลิกเมื่อเริ่ม backend รุ่นใหม่ อีเมลแจ้งเตือนที่เริ่มส่งก่อนยกเลิกอาจส่งเสร็จก่อนคำขอยกเลิกสำเร็จ

## ตรวจสอบโค้ด

```powershell
npm run lint
npm run build
```

# เชื่อม DocFlow กับ OCR ภายนอกผ่าน API (thaidoc-extract)

- วันที่: 2026-10-11
- สถานะ: รอผู้ใช้ตรวจสเปก
- เอกสารอ่านประกอบ (อธิบายละเอียด มีแผนภาพ): https://claude.ai/code/artifact/5148db85-c6c1-469c-8c2d-33710a8afa30

## เป้าหมาย

เลิกใช้ one-ocr และ heuristic ฝั่ง Go สำหรับ PDF แล้วส่งไฟล์ไปให้ thaidoc-extract
(https://github.com/First364600/document-extraction-project-ai) ที่รันบนเครื่อง GPU แยกต่างหากผ่าน HTTP API
ระบบต้องเปลี่ยนไปใช้ API เจ้าอื่นได้โดยไม่ต้องแก้ส่วนอื่นของ DocFlow

### สิ่งที่อยู่นอกขอบเขต

- การอ่าน DOCX, XLSX, CSV ยังใช้ `ocr_worker.py` แบบเดิม (ไม่ต้องทำ OCR)
- เอกสารเก่าที่ประมวลผลด้วย one-ocr แล้วคงไว้ตามเดิม ไม่ประมวลผลใหม่อัตโนมัติ
- การจับคู่ชื่อยังใช้ `directory_matching.go` ไม่ใช้ค่า `similarity` ของ thaidoc
- อีเมลแจ้งแอดมินเมื่อประมวลผลล้มเหลว เป็นงานแยก (ดูหัวข้อ "จุดต่อกับงานอื่น")

## การตัดสินใจ

| # | เรื่อง | ข้อตกลง |
|---|---|---|
| 1 | thaidoc แทน one-ocr หรือไม่ | แทนทั้งหมดสำหรับ PDF ต้องต่อ API เจ้าอื่นได้ |
| 2 | รูปแบบของ API เจ้าอื่น | รองรับทั้งสองแบบ: API ที่ตอบตาม DocFlow contract และ API ที่มีรูปแบบของตัวเองผ่าน adapter เฉพาะ รอบแรกทำ `thaidoc` กับ `docflow-contract` |
| 3 | บรรทัดดิบพร้อมกรอบ | ไม่บังคับ มีก็แสดงกรอบฟ้า ไม่มีก็ไม่แสดง ผลที่แยกแล้วต้องมีเสมอ |
| 4 | ที่รัน thaidoc | เครื่องแยกที่มี GPU เรียกข้ามเครื่องผ่าน URL |
| 5 | การป้องกัน API | reverse proxy (Caddy หรือ nginx) บนเครื่อง GPU ทำ HTTPS และตรวจ `Authorization: Bearer <key>` ไม่แก้โค้ด thaidoc |
| 6 | เมื่อเรียก API ไม่สำเร็จ | ลองใหม่อัตโนมัติเฉพาะปัญหาชั่วคราว ปัญหาถาวรให้ล้มทันที |
| - | โครงสร้างโค้ด | interface `Extractor` ฝั่ง Go พร้อม adapter แยกไฟล์ต่อ API หนึ่งเจ้า |

## ข้อเท็จจริงเกี่ยวกับ thaidoc ที่การออกแบบต้องรองรับ

ตรวจจากโค้ดของ thaidoc (`app/main.py`, `app/export_json.py`, `app/pipeline.py`) เมื่อ 2026-10-11

- `POST /api/v1/extract-json` รับ multipart field `file` คืนหัวคำสั่ง
  (`subject`, `date`, `order`, `effective date`, `number`, `expiry date`, `ref_number`, `signer`),
  กลุ่มกรรมการใน `data[]` (`dept`, `duty`, `list name[]` แต่ละคนมี `prefix`, `name`, `position`, `role`),
  กรอบตำแหน่งของทุกค่า (`page`, `x0`, `y0`, `x1`, `y1` ค่า 0–1 นับจากมุมซ้ายบน) และ `pages[]` (`page`, `width`, `height`)
- ค่าความมั่นใจรายคน (`"ค่าความมั่นใจ"`) เป็น `null` ทุกคน และหัวคำสั่งไม่มีค่าความมั่นใจ
  ช่อง `confidence` ในผลดิบ (`/api/v1/extract`) คือความเหมือนกับรายชื่อบุคลากร ไม่ใช่ความมั่นใจของการอ่าน
- ไม่มีข้อความดิบรายบรรทัด (`raw_page_lines` มีแค่จำนวนบรรทัด)
- ชื่อ key มีเว้นวรรคและภาษาไทย
- ไม่มี auth, เปิด CORS `*`, รัน uvicorn `workers=1`
- status: `503` ตอนโมเดลยังโหลดไม่เสร็จ, `400` ไฟล์อ่านไม่ได้ (`ValueError`), `422` ไม่มี field `file`, `500` ความผิดพลาดอื่นทุกชนิด

### ผลทดสอบจริง (CPU Docker บน Mac, 2026-10-11)

ไฟล์ที่ใช้: `public/sample-command.pdf` (14 หน้า) ซึ่งเป็นเอกสาร TOR ไม่ใช่คำสั่งแต่งตั้ง
ผลนี้จึงยืนยันรูปแบบคำตอบ เวลา และ error ได้ แต่ยังวัดความแม่นยำไม่ได้

- build image CPU ใช้ 11.5 นาที ขนาด 790 MB; ตอนเปิดใช้ 6 วินาทีกว่า `/health` ตอบ `200`
  ระหว่างโหลดโมเดลพอร์ตยังไม่รับการเชื่อมต่อ (ไม่ได้ `503`) ซึ่งเข้ากรณี "เชื่อมต่อไม่ได้" ที่ลองใหม่ได้อยู่แล้ว
- `extract-json`: ครั้งแรก 6.6 วินาที, ครั้งถัดไป 5.3 วินาที (ราว 0.4 วินาทีต่อหน้าบน CPU)
  PDF 100 หน้าจึงน่าจะใช้ราว 40–50 วินาทีบน CPU; timeout 10 นาทีเหลือพอ
- หน่วยความจำ 1.1 GiB ตอนว่าง, 1.4–2.5 GiB ระหว่างประมวลผล
- ไฟล์ที่ไม่ใช่ PDF → `400` `{"detail":"a.txt: not a readable PDF or image"}`; ไม่มี field `file` → `422`
- `ค่าความมั่นใจ` เป็น `null` ทุกคน, `similarity` เป็น `[]` ทุกคน (รายชื่อบุคลากรว่าง) ยืนยันกฎค่าความมั่นใจ
- `pages[]` มี `width`/`height` เป็นพิกเซลที่ 200 dpi (1654×2339)
- กลุ่มและสมาชิกที่ได้จากเอกสารที่ไม่ใช่คำสั่งมีค่าว่างหรือข้อความยาวผิดที่ (เช่น `name` ว่าง, `duty` ยาวหลายหน้า, ตำแหน่งติดมากับชื่อ)
  adapter จึงต้องข้ามสมาชิกที่ `name` และ `position` ว่างทั้งคู่และไม่ใช่ `ExOfficio`, ส่วนค่าที่ผิดที่ปล่อยให้ผู้ตรวจแก้

## สถาปัตยกรรม

```
แหล่งเอกสาร (อัปโหลด / Google Drive / โฟลเดอร์)
        │
        ▼
คิวงาน (dispatcher, ทำพร้อมกัน OCR_API_CONCURRENCY งาน) ◄── cron ทุก 1 นาที กวาดงานที่ถึงเวลา/ค้าง
        │
        ▼
Extractor (adapter: thaidoc | docflow-contract) ──HTTPS + Bearer key──► reverse proxy ──► thaidoc (127.0.0.1:8000, เครื่อง GPU)
        │                                       ◄──────── JSON ────────
        ▼
ExtractionResult ──► บันทึกลง document_workflows, extracted_fields, appointments (+ ocr_pages/ocr_lines ถ้ามี)
        │
        ▼
directory_matching.go (จับคู่ชื่อเหมือนเดิม)
```

## ส่วนที่ 1: รูปแบบข้อมูลกลาง `ExtractionResult`

```go
type ExtractionResult struct {
    Engine string           // "thaidoc", "docflow-contract", ...
    Header ExtractedHeader
    Groups []ExtractedGroup // ต้องมี (ว่างได้ถ้าเอกสารไม่มีรายชื่อ)
    Pages  []ExtractedPage  // ไม่บังคับ
}

type Box struct {
    Page           int
    X0, Y0, X1, Y1 float64 // 0–1 นับจากมุมซ้ายบน
}

type ExtractedValue struct {
    Text       string
    Confidence *float32 // nil = API ไม่ได้บอก
    Boxes      []Box
}

type ExtractedHeader struct {
    OrderNo, Subject, IssuedBy, Date, EffectiveDate, ExpiryDate, Signer ExtractedValue
    References []ExtractedValue
}

type ExtractedGroup struct {
    Department, Duty ExtractedValue
    Members          []ExtractedMember
}

type ExtractedMember struct {
    Prefix, Name, Position, Role ExtractedValue
    ExOfficio bool // กรรมการโดยตำแหน่ง ชื่อว่างได้
}

type ExtractedPage struct {
    Page          int
    Width, Height int            // พิกเซลของภาพที่ API ใช้ (0 = ไม่ทราบ)
    Lines         []ExtractedValue // บรรทัดดิบ (ไม่บังคับ)
}
```

### การบันทึก (ไม่แก้ schema ของตารางเดิม)

| ส่วนของผล | ลงที่ |
|---|---|
| `Header` | คอลัมน์ของ `Document`: `OrderNo`, `Title` (จาก `Subject` ถ้ามี), `SignerName`, `AdditionalRefs`, `IssuedDate`, `EffectiveDate`, `ExpiryDate` และแถว `ExtractedField` หนึ่งแถวต่อฟิลด์ที่มีข้อความ พร้อม `PageNo` และ `BoundingBoxJSON` ของกรอบแรก |
| `Groups[].Members` | แถว `Appointment` หนึ่งแถวต่อคน: `FullName` = `Prefix`+`Name`, `Position`, `CommitteeRole` = `Role`, `Department` = กลุ่ม `Department`, `Responsibilities` = กลุ่ม `Duty`, กรอบจาก `Name` (ถ้าว่างใช้ `Position`) |
| `Pages[].Lines` | `OCRPage` และ `OCRLine` เมื่อมี; `Document.OCRText` = ข้อความรวมของทุกบรรทัด (ว่างถ้าไม่มี) |

- วันที่ภาษาไทย (เช่น "1 ตุลาคม 2569") แปลงด้วย `parseThaiDate` ที่มีอยู่ใน `document_processing.go` (ย้ายไว้ไฟล์ที่ไม่ถูกลบตอนถอด heuristic); แปลงไม่ได้ให้เก็บเป็นข้อความใน `ExtractedField` และปล่อยคอลัมน์วันที่เป็น `NULL`
- `ExOfficio` ที่ชื่อว่าง: บันทึก `Appointment` โดย `FullName` ว่าง ข้ามการจับคู่ชื่อ
- `Document.OCRProvider` = `Engine` ของผล

### กฎค่าความมั่นใจ

- ค่าใดใน `Header` หรือ `Members` มี `Confidence == nil` → เอกสารเป็น `NEEDS_REVIEW`
- ทุกค่ามีค่าความมั่นใจ → ใช้ `needsReview` / `lowConfidenceAt` เดิมกับ `OCRSettings.ReviewThreshold`
- `Document.Confidence` = ค่าเฉลี่ยของค่าที่มีค่าความมั่นใจ (0 ถ้าไม่มีเลย)

### DocFlow contract

JSON ของ `ExtractionResult` ชื่อ field แบบ camelCase (`engine`, `header.orderNo.text`, `groups[].members[].name.confidence`, ...)
เขียนเป็น `docs/ocr-api-contract.md` พร้อมไฟล์ตัวอย่าง `backend/testdata/extraction/docflow-contract.json`
ซึ่งใช้เป็น fixture ของเทสต์ด้วย

## ส่วนที่ 2: การเรียก API, การยืนยันตัว, การตั้งค่า

```go
type Extractor interface {
    Name() string
    Extract(ctx context.Context, pdfPath, filename string) (ExtractionResult, error)
}

type ExtractError struct {
    Retryable bool
    Status    int    // HTTP status, 0 ถ้าไม่ได้คำตอบ
    Message   string // ภาษาไทย แสดงในหน้าเว็บ
}
```

- คำขอ: `POST` multipart field `file`; มี key → header `Authorization: Bearer <key>`
- `thaidoc`: `{URL}/api/v1/extract-json` แล้วแปลงเป็น `ExtractionResult`
- `docflow-contract`: `{URL}` ตรง ๆ คำตอบต้องเป็น `ExtractionResult` JSON
- ตรวจ 100 หน้าและ 50 MB ก่อนส่งเหมือนเดิม; timeout จาก `OCR_API_TIMEOUT`

### การแบ่ง error

| สิ่งที่เกิด | Retryable |
|---|---|
| เชื่อมต่อไม่ได้, connection reset, timeout | true |
| `429`, `502`, `503`, `504` | true |
| `500` ครั้งแรกของเอกสาร | true |
| `500` ครั้งที่สองขึ้นไปของเอกสาร | false |
| `400`, `413`, `415`, `422` | false |
| `401`, `403` | false (ข้อความ: "API key ไม่ถูกต้อง หรือไม่มีสิทธิ์เรียกใช้ OCR") |
| คำตอบ `2xx` ที่แปลงเป็น `ExtractionResult` ไม่ได้ | false |

การนับ `500` ใช้คอลัมน์ `extraction_last_status` (ส่วนที่ 3)

### การตั้งค่า

| ค่า | เก็บที่ | ค่าเริ่มต้น / หมายเหตุ |
|---|---|---|
| `OCRSettings.Engine` | `app_settings` | `thaidoc`; ค่าที่รับ: `thaidoc`, `docflow-contract` (ลบ `one-ocr`) |
| `OCRSettings.APIURL` (`apiUrl`) | `app_settings` | จาก env `OCR_API_URL`; ต้องเป็น `http://` หรือ `https://`; หน้าเว็บเตือนถ้าไม่ใช่ `https://` |
| API key | env `OCR_API_KEY` หรือไฟล์ตาม `OCR_API_KEY_FILE` | ไม่เก็บใน DB ไม่ส่งไปหน้าเว็บ; settings response มี `ocrApiKeyConfigured bool` |
| `OCR_API_TIMEOUT` | env | `10m` |
| `OCR_API_CONCURRENCY` | env | `1` |

- endpoint ใหม่ `POST /api/v1/settings/ocr/test` (สิทธิ์ ADMIN) เรียก `GET {URL}/health` พร้อม key
  คืน `{"status":"ok"|"unauthorized"|"unreachable","detail":"..."}`; หน้าตั้งค่ามีปุ่ม "ทดสอบการเชื่อมต่อ"
- ซ่อนช่องภาษา, DPI, ขนาดภาพ, preprocess ในหน้าตั้งค่า (ยังอ่านค่าเดิมจาก DB ได้ ไม่ลบ field)
- `pdftoppm` ยังใช้สำหรับ `GET /api/v1/documents/:id/pages/:page/image` และการนับหน้า

### Reverse proxy บนเครื่อง GPU

- HTTPS, ตรวจ `Authorization` ไม่ตรงตอบ `401`, จำกัด body 50 MB
- เปิดเฉพาะ `GET /health` และ `POST /api/v1/extract-json`; ทางอื่น (`/docs`, `/api/v1/ground-truth`, `/api/v1/demo-pdf`, ...) ตอบ `404`
- thaidoc ฟังที่ `127.0.0.1:8000`
- ไฟล์ตัวอย่าง `deploy/gpu/Caddyfile` และขั้นตอนใน `DEPLOY_PODMAN.md`

## ส่วนที่ 3: คิว, การลองใหม่, สถานะ

### คิว

- `DocumentProcessor` มี dispatcher ตัวเดียว ทำงานพร้อมกันได้ `OCR_API_CONCURRENCY` งาน
- `Enqueue(id)` ส่ง id เข้า channel (ไม่บล็อก ถ้าคิวเต็มปล่อยให้ cron หยิบภายหลัง)
- ก่อนเริ่มงาน dispatcher อัปเดตเป็น `EXTRACTING` แบบมีเงื่อนไข
  (`WHERE id = ? AND processing_stage IN ('QUEUED','RETRY_WAIT')`) ถ้า `RowsAffected == 0` ข้าม เพื่อไม่ให้ส่งซ้ำ
- cron ทุก 1 นาที ส่งเข้าคิว:
  - `RETRY_WAIT` ที่ `next_attempt_at <= now`
  - `QUEUED` ที่อยู่นานเกิน 1 นาที (หลุดคิวตอนรีสตาร์ต)
  - `EXTRACTING` ที่ `processing_started_at` เก่ากว่า timeout + 2 นาที (ตั้งกลับเป็น `QUEUED` ก่อน)
- ไฟล์ที่ไม่ใช่ PDF ใช้คิวเดียวกัน แต่เรียก `runStructuredImport` แทน Extractor และไม่มีการลองใหม่

### คอลัมน์ใหม่ (migration `0008_extraction_retry.sql`)

```sql
ALTER TABLE document_workflows
  ADD COLUMN extraction_attempts INT NOT NULL DEFAULT 0,
  ADD COLUMN next_attempt_at DATETIME NULL,
  ADD COLUMN extraction_last_status INT NOT NULL DEFAULT 0,
  ADD KEY ix_document_workflows_next_attempt (processing_stage, next_attempt_at);
```

### ลำดับสถานะ

| เหตุการณ์ | status / processing_stage | การเปลี่ยนแปลงอื่น |
|---|---|---|
| เข้าคิว | `PROCESSING` / `QUEUED` | |
| เริ่มเรียก API | `PROCESSING` / `EXTRACTING` | `extraction_attempts += 1`, `processing_started_at = now` |
| สำเร็จ | `NEEDS_REVIEW` หรือ `REVIEW` / `EXTRACTED` | บันทึกผลตามส่วนที่ 1, `next_attempt_at = NULL`, `ocr_error = ''` |
| ล้ม Retryable และ attempts < 5 | `PROCESSING` / `RETRY_WAIT` | `next_attempt_at = now + backoff(attempts)`, `ocr_error`, `extraction_last_status` |
| ล้มถาวร หรือ attempts = 5 | `NEEDS_REVIEW` / `FAILED` | `ocr_error`, `next_attempt_at = NULL` |

backoff หลังครั้งที่ 1, 2, 3, 4 = 1, 5, 15, 60 นาที (รวมราว 81 นาทีก่อนล้มถาวร)

- ตอนนี้ยังไม่มีปุ่มประมวลผลใหม่ (มีแค่อัปโหลดไฟล์ทับ `POST /api/v1/documents/:id/file`) จึงเพิ่ม
  `POST /api/v1/documents/:id/reprocess` (สิทธิ์ ADMIN, STAFF) รับเฉพาะเอกสาร `FAILED` หรือ `RETRY_WAIT`
  ตั้งเป็น `PROCESSING` / `QUEUED` แล้ว `Enqueue` พร้อมปุ่ม "ประมวลผลใหม่" ในหน้ารายละเอียดเอกสาร และบันทึก `AuditLog`
- ทั้ง endpoint ใหม่นี้และ `replacedFileUpdates` (อัปโหลดทับ) รีเซ็ต `extraction_attempts = 0`,
  `extraction_last_status = 0`, `next_attempt_at = NULL`
- หน้าเว็บ: เอกสาร `RETRY_WAIT` แสดง "รอลองใหม่ ครั้งที่ n เวลา hh:mm" พร้อม `ocrError` ในรายการเอกสารและหน้ารายละเอียด
  (API ส่ง `extractionAttempts`, `nextAttemptAt`)

### จุดต่อกับงานอื่น

การเปลี่ยนเป็น `FAILED` ทุกกรณีผ่านฟังก์ชันเดียว `markExtractionFailed(doc, err ExtractError)`
งานอีเมลแจ้งแอดมิน (พักไว้) จะต่อที่ฟังก์ชันนี้

## ส่วนที่ 4: เทสต์, deploy, ถอด one-ocr

### เทสต์ (TDD, ไม่ต้องใช้เครื่อง GPU)

- adapter `thaidoc`: `httptest.Server` ตอบด้วย `backend/testdata/extraction/thaidoc-extract-json.json`
  (ผลจริงจาก thaidoc ของ `public/sample-command.pdf`) ตรวจการแปลงทุก field, กรอบ, `Confidence == nil`,
  การข้ามสมาชิกที่ว่าง; กรรมการโดยตำแหน่งใช้ fixture เล็กที่เขียนเอง จนกว่าจะได้ผลจากคำสั่งแต่งตั้งจริง
- adapter `docflow-contract`: fixture `docflow-contract.json`
- error: ทุกแถวในตาราง "การแบ่ง error" รวมกรณีเชื่อมต่อไม่ได้และ timeout; ส่ง `Authorization` เมื่อมี key และไม่ส่งเมื่อไม่มี
- การบันทึก: `ExtractionResult` → แถวใน DB ตามตาราง "การบันทึก" และกฎค่าความมั่นใจ
- คิว: นาฬิกาจำลอง ตรวจลำดับสถานะ, backoff, หยุดที่ครั้งที่ 5, `500` สองครั้ง, การกู้ `QUEUED`/`EXTRACTING` ที่ค้าง, ไม่ส่งซ้ำ
- settings: `Engine` ที่รับได้, `APIURL` ไม่ถูกต้องถูกปฏิเสธ, key ไม่รั่วใน response, endpoint ทดสอบการเชื่อมต่อ
- reprocess: รับเฉพาะ `FAILED`/`RETRY_WAIT`, รีเซ็ตคอลัมน์ลองใหม่, ปฏิเสธเอกสารสถานะอื่น, สิทธิ์ VIEWER ถูกปฏิเสธ
- หน้าเว็บ: เทสต์การแสดงข้อความ `RETRY_WAIT` และปุ่มประมวลผลใหม่

### Deploy

- เครื่อง GPU: thaidoc จาก `Dockerfile.gpu` ฟัง `127.0.0.1:8000`, Caddy ตาม `deploy/gpu/Caddyfile`
- DocFlow: เพิ่ม `OCR_API_URL`, `OCR_API_KEY`, `OCR_API_TIMEOUT`, `OCR_API_CONCURRENCY` ใน `.env.example`,
  `docker-compose.yml`, `deploy/podman-compose.yml`
- ลำดับ: ขึ้นเครื่อง GPU → ทดสอบการเชื่อมต่อจากหน้าตั้งค่า → เปลี่ยน Engine

### ถอด one-ocr

- ลบเส้นทาง PDF ที่ใช้ one-ocr และ heuristic ใน `document_processing.go`
  (`runOCRWorker`, `extractCommandMetadata*`, `groupOCRRows`, `parseAppointmentRow`, `fixedSampleProfile*` และฟังก์ชันที่ใช้เฉพาะเส้นทางนี้)
  พร้อมเทสต์ของฟังก์ชันเหล่านั้น
- `ocr_worker.py`: import `oneocr` และ PIL เฉพาะตอนทำ OCR ภาพ ให้ `--input-file` สำหรับ DOCX/XLSX/CSV ทำงานได้โดยไม่มี one-ocr
- ถอด `backend/one-ocr` และโมเดลออกจาก `backend/Dockerfile`
- เอกสารเก่า (`ocr_provider = one-ocr`) เปิดดูได้ตามปกติ

## ความเสี่ยงและคำถามที่ยังเปิด

- เครื่อง GPU ล่มนานกว่า 81 นาที → เอกสารเป็น `FAILED` ต้องกดประมวลผลใหม่ทีละฉบับ (ลดได้ด้วยอีเมลแจ้งแอดมิน)
- thaidoc ทดสอบกับเอกสารจริงจำนวนน้อย; ทุกเอกสารจาก thaidoc ต้องผ่านคนตรวจ (กฎค่าความมั่นใจ)
- รูปแบบผลของ thaidoc อาจเปลี่ยน → ตรึงเวอร์ชัน image บนเครื่อง GPU; เทสต์ adapter จับการเปลี่ยนแปลง
- thaidoc ใช้ PyMuPDF (AGPL)
- เปิด: ยังไม่ได้วัดความแม่นยำกับคำสั่งแต่งตั้งจริง (ไฟล์ตัวอย่างใน repo เป็น TOR) ควรทดสอบกับคำสั่งจริงก่อนเปลี่ยน Engine บน production
- เปิด: เครื่อง GPU อยู่ในเครือข่ายเดียวกับ server DocFlow หรือไม่
- เปิด: ขอให้ thaidoc เพิ่มค่าความมั่นใจของการอ่านใน `extract-json` หรือไม่

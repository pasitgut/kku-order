# แผนการเชื่อม DocFlow กับ OCR ภายนอก (thaidoc-extract) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (แนะนำ) หรือ superpowers:executing-plans เพื่อทำแผนนี้ทีละงาน ทุก step ใช้ checkbox `- [ ]` ติดตามความคืบหน้า ทำตามลำดับ Task 1 ถึง Task 12

**Goal:** เลิกใช้ one-ocr และ heuristic ฝั่ง Go สำหรับ PDF แล้วส่งไฟล์ไปให้ thaidoc-extract (รันบนเครื่อง VM ผ่าน reverse proxy ที่มี key) ผ่าน adapter ที่แยกตามผู้ให้บริการ พร้อมคิวที่ลองใหม่อัตโนมัติเฉพาะปัญหาชั่วคราว และหน้าตั้งค่าที่ทดสอบการเชื่อมต่อได้

**Architecture:** `Extractor` interface ใน Go มีสองตัวคือ `thaidoc` (แปลงรูปแบบของ thaidoc) และ `docflow-contract` (รับ `ExtractionResult` ตรง ๆ) ทั้งสองใช้ `httpExtractor` ตัวเดียวกันสำหรับ HTTP, multipart และการแบ่ง error ผลถูกบันทึกผ่านฟังก์ชันบริสุทธิ์ `buildExtractionRecords` แล้วเขียนลงตารางเดิมผ่านชั้น store คิวเป็น dispatcher ตัวเดียวใน `DocumentProcessor` ทำงานพร้อมกันตาม `OCR_API_CONCURRENCY` และ cron ทุกนาทีกวาดงานที่ถึงเวลา

**Tech Stack:** Go 1.27 (gin, gorm, robfig/cron, `testing`, `net/http/httptest`), MySQL (migration ผ่าน `backend/migrations`), Next.js/React + TypeScript (`node:test` ไฟล์ `src/lib/*.test.ts`), Docker Compose, Caddy 2.8.4 เป็น reverse proxy, thaidoc-extract (FastAPI, CPU สำหรับทดสอบ, GPU สำหรับ production ในอนาคต)

**Spec path:** `docs/superpowers/specs/2026-10-11-thaidoc-ocr-integration-design.md` (อ่านก่อนเริ่มทุกครั้ง โดยเฉพาะส่วน "การตัดสินใจ" และ "ส่วนที่ 1–4")

## Global Constraints

- ห้ามแก้ไฟล์นอก Task ที่ระบุไว้ใน "Files" ของ Task นั้น ถ้าต้องแก้ไฟล์อื่น ให้หยุดและถามผู้ใช้ก่อน
- ทุก Task ต้องจบด้วย `cd backend && go test ./...` และ/หรือ `npm test` ผ่าน ยกเว้น Task 1 (ไม่มี Go code) และ Task 12 (deploy)
- ยกเว้น baseline: `TestNewManualDirectoryUserStoresValidJSONPayload` ล้มอยู่แล้วก่อนเริ่มงาน ให้จดไว้ ห้ามแก้ ห้ามใช้เป็นเหตุผลละเว้นเทสต์อื่น
- TDD: เขียนเทสต์ก่อน รันแล้วต้องเห็น FAIL ด้วยเหตุผลที่คาดไว้ แล้วจึง implement
- เทสต์ Go ใช้ `testing` ธรรมดา, `httptest`, fake store (แบบ `fakeSettingsStore` ใน `settings_test.go`) ไม่มี test DB ดังนั้นโค้ดที่แตะ DB ต้องอยู่หลัง interface เล็ก ๆ หรือเป็นฟังก์ชันบริสุทธิ์
- เทสต์ frontend ใช้ `node:test` ไฟล์ `src/lib/*.test.ts` และ import ด้วยนามสกุล `.ts`
- ห้าม commit ค่า key จริง ห้ามใส่ key ใน response หรือ log
- ข้อความที่แสดงผู้ใช้เป็นภาษาไทย ชื่อ field ใน JSON contract เป็น camelCase ภาษาอังกฤษ
- Commit ทุก Task ใช้ข้อความ commit ตามแบบในแต่ละ Task และลงท้ายด้วย trailer:
  `Co-Authored-By: Claude Haiku 5.5 <noreply@anthropic.com>`
- Deploy (Task 1 บน VM และ Task 12) เป็นขั้นตอน remote/manual ที่ main session ทำ เป็นงานที่ย้อนกลับไม่ได้หรือกระทบ server ให้ทำตามที่เขียนเท่านั้น และไม่ push ไป remote โดยไม่ได้รับคำสั่ง

## Review Focus

ห้าข้อนี้เป็นความเสี่ยงที่ต้องมีเทสต์ในงานที่เป็นเจ้าของ (ระบุใน Task เจ้าของ)

1. ค่า settings เก่า `"engine":"one-ocr"` ต้องไม่ทำให้การ merge settings ทั้งก้อนกลับไปเป็นค่าเริ่มต้นและลบค่า import/Drive ที่ตั้งไว้ (Task 8)
2. ค่าที่ยาวเกินคอลัมน์ต้องไม่ทำให้ insert ล้ม ตัดด้วยจำนวน rune ก่อนบันทึก (Task 5)
3. เอกสารที่ค้างสถานะ PREPARING, EXTRACTING, QUEUED หลังรีสตาร์ตต้องถูกหยิบกลับเข้าคิว (Task 7)
4. ค่าความมั่นใจ 0 ต้องไม่แสดง "กำลังคำนวณ" ตลอดไป (Task 10)
5. กรรมการโดยตำแหน่ง (ExOfficio) ต้องมีป้ายชื่อและยังยืนยันได้ (Task 5 สำหรับ blocker, Task 10 สำหรับการแสดงผล)

## ข้อที่ต่างจาก spec

รายการนี้เป็นสิ่งที่แผนนี้เปลี่ยนจาก spec ทั้งหมด ผู้ตรวจต้องรับรู้ก่อนรับแผน

1. **โฟลเดอร์ deploy ชื่อ `deploy/ocr/` แทน `deploy/gpu/`** เพราะรอบแรกรันบน VM ที่ไม่มี GPU และคู่มือเป็น `deploy/ocr/README.md` แทน `DEPLOY_PODMAN.md`
2. **สร้าง `OCRPage` ทุกหน้าที่ API ตอบ** แม้ไม่มีบรรทัดดิบ เพื่อให้กรอบของ field แสดงได้ (frontend คำนวณกรอบจาก `imageWidth/imageHeight` ของ OCRPage) ถ้าไม่รู้ขนาดหน้าให้ใช้ width=height=1 และคงกรอบเป็นค่า normalized ตาม `parseBox`
3. **สร้างแถว `ExtractedField` ทุกฟิลด์ของหัวคำสั่ง แม้ค่าว่าง** (ตาม behaviour เดิม เพื่อให้หน้าตรวจแก้แถวที่มีอยู่ได้) พร้อม `issued_by` และ `additional_references` ไม่ใช่เฉพาะฟิลด์ที่มีข้อความ
4. **ถอด heuristic ออกเฉพาะเส้นทาง PDF** ส่วน DOCX/XLSX/CSV ใช้ `extractCommandMetadata` แบบไม่มี profile และตั้ง `ocr_provider = structured-import`
5. **การ merge settings แปลง `one-ocr` เป็นค่าเริ่มต้นก่อน Validate** และถ้า `apiUrl` ว่างใน DB ใช้ค่าจาก env `OCR_API_URL`
6. **ป้ายกรรมการโดยตำแหน่ง** แสดงเป็น `<ตำแหน่ง> (โดยตำแหน่ง)` ในรายการ blocker และในหน้าการแสดงชื่อ
7. **เพิ่ม `confidenceLabel`** ในหน้ารายการเอกสาร (spec ไม่ได้ระบุ แต่จำเป็นเพื่อไม่ให้เอกสาร thaidoc แสดง "กำลังคำนวณ" ตลอดไป)
8. **เพิ่มการกู้งานตอนเริ่มระบบ** (`Recover`) สำหรับ PREPARING, EXTRACTING และ QUEUED ทั้งหมดตอนสตาร์ต เพราะ API มี instance เดียว งานที่ค้างจึงไม่มีใครทำต่อ (spec ระบุแค่ cron)
9. **Endpoint ทดสอบการเชื่อมต่อรับค่าที่ยังไม่ได้บันทึก** (`engine`, `apiUrl` ใน body) ได้ถ้าส่งมา ไม่ส่งจะใช้ค่าที่บันทึก
10. **URL สำหรับ health ของ `docflow-contract`** คือ origin + `/health` (spec ไม่ได้ระบุ) และถ้า `/health` ตอบ `models_loaded=false` ให้รายงาน `unreachable` พร้อมข้อความว่ากำลังโหลดโมเดล
11. **การตัดข้อความยาวนับเป็น rune** (spec ระบุแค่ "ค่าที่ยาว") โดยใช้ความยาวคอลัมน์ที่ระบุในหัวข้อ Task 5
12. **การประมวลผลใหม่ (reprocess) และการอัปโหลดทับ** ใช้ helper `retryResetUpdates()` ร่วมกัน รวมทั้งเส้นทาง duplicate-overwrite ใน `uploadDocument` (main.go) ไม่ใช่เฉพาะ `replacedFileUpdates`
13. **กรณี HTTP status ที่ตารางใน spec ไม่ได้ระบุ** (เช่น 404, 405) ถือเป็นถาวร เพราะมักหมายถึง URL ตั้งค่าผิด
14. **ไฟล์ที่ไม่ใช่ PDF เข้าคิวเดียวกันแต่ไม่ลองใหม่** ตาม spec (เขียนไว้เพื่อความชัดเจน)
15. **`markExtractionFailed(doc Document, failure ExtractError, started time.Time)`** ใช้ `(doc, err)` ตาม spec บรรทัด 250 และเพิ่ม `started` เพื่อบันทึกระยะเวลาใน `processing_duration_ms` (ฉบับก่อนหน้าเปลี่ยนเป็น `(id, failure)` โดยไม่บันทึกไว้)
16. **ฟิลด์ฟังก์ชันใน `DocumentProcessor`** (`pageCount`, `importStructured`, `now`, `newExtractor`) และ `checkPDFLimits(ctx, path, countPages)` รับตัวนับหน้าเป็นพารามิเตอร์ เพื่อให้เทสต์ไม่ต้องใช้ `pdfinfo`, Python หรือไฟล์นอก `t.TempDir()`
17. **DOCX/XLSX/CSV ใช้เส้นทางบันทึกเดียวกับ PDF** โดยแปลงผลด้วย `structuredExtraction` (`engine = structured-import`) ความมั่นใจของทุกค่าเป็นค่าเฉลี่ยของเอกสาร เพื่อให้ threshold เดิมใช้ได้ `IssuedBy` = ชื่อเรื่องเพื่อให้ `committee` เหมือนเดิม กรอบของบรรทัดไม่ normalize (เก็บ `[]`) และไฟล์ที่ไม่ใช่ตัวอย่างยังไม่มีรายชื่อสมาชิก (พฤติกรรมเดิมของ `extractCommandMetadata` ที่ไม่มี profile)
18. **คอลัมน์ `committee` และ `order_type` ของ PDF เติมจาก `IssuedBy` (ผู้ออกคำสั่ง)** spec ไม่ได้ระบุ (ค่าเดิมของ `committee` มาจากชื่อเรื่อง)
19. **Signature ที่เปลี่ยน:** `NewDocumentProcessor(db, logger, cfg)`, `getOCRSettingsHandler(store, cfg)` (ตอบ `ocrApiKeyConfigured`), `checkOCRConnection(ctx, client, healthURL, apiKey)` ไม่รับ `thaidoc` (ไม่ได้ใช้); `applyDirectoryMatches` ถูกลบ เหลือ `matchAppointments` ที่ข้ามแถวไม่มีชื่อ
20. **`extractionStatus` ใช้ `needsReview` เดิม** (`settings.go`) แทนการเขียนกฎ threshold ใหม่ และ `confidenceLabel` ใน frontend ใช้ `"ไม่ระบุ"` เมื่อค่าเป็น 0 ของเอกสารที่ประมวลผลเสร็จแล้ว
21. **คำสั่งในแผนชี้ไปที่ worktree `/Users/kit/kku/kku-order-thaidoc`** (แผนเดิมชี้ไปที่ `/Users/kit/kku/kku-order-thaidoc` ซึ่งเป็น checkout หลัก) ยกเว้นคำสั่งบน VM และ `deploy/ocr` ของ Task 1 ที่ไม่เปลี่ยน

## โครงสร้างไฟล์ (ภาพรวม)

| ไฟล์ | สถานะ | หน้าที่ |
|---|---|---|
| `backend/extraction.go` | ใหม่ | types กลาง `ExtractionResult` ฯลฯ, `Extractor`, `ExtractError` |
| `backend/extractor_thaidoc.go` | ใหม่ | `parseThaidocResponse` |
| `backend/extractor_contract.go` | ใหม่ | `parseContractResponse` |
| `backend/extractor_http.go` | ใหม่ | `httpExtractor`, `classifyHTTPFailure` |
| `backend/ocr_config.go` | ใหม่ | `ocrAPIConfig`, `loadOCRAPIConfig`, `newExtractor`, `checkPDFLimits` |
| `backend/extraction_persist.go` | ใหม่ | `buildExtractionRecords`, `extractionStatus`, `truncateRunes` |
| `backend/document_queue.go` | ใหม่ | `planExtractionFailure`, `extractionJobs`, dispatcher |
| `backend/document_queue_store.go` | ใหม่ | `gormExtractionJobs`, `retryResetUpdates` |
| `backend/reprocess.go` | ใหม่ | `reprocessStore`, `reprocessDocumentHandler` |
| `backend/document_retry.go` | ใหม่ | `planExtractionFailure`, `retryResetUpdates` |
| `backend/structured_extraction.go` | ใหม่ | `structuredExtraction` (DOCX/XLSX/CSV เป็น `ExtractionResult`) |
| `backend/ocr_connection.go` | ใหม่ | `checkOCRConnection`, `testOCRConnectionHandler` |
| `backend/migrations/0008_extraction_retry.sql` | ใหม่ | คอลัมน์ลองใหม่ |
| `docs/ocr-api-contract.md` | ใหม่ | DocFlow contract |
| `backend/testdata/extraction/docflow-contract.json` | ใหม่ | fixture ของ contract |
| `deploy/ocr/*` | ใหม่ | compose, Caddyfile, .env.example, README |
| `frontend: src/lib/processing-status.ts` | ใหม่ | `processingNote`, `canReprocess`, `confidenceLabel` |

---

## Task 1: thaidoc หลัง proxy บน VM (งาน deploy เป็นอิสระจากโค้ด)

เป้าหมาย: มี thaidoc ทำงานบน VM ผ่าน proxy ที่มี key เพื่อทดสอบเอกสารจริงได้เร็ว งานนี้ไม่แตะ Go code

**Files:**
- Create: `deploy/ocr/compose.yml`
- Create: `deploy/ocr/Caddyfile`
- Create: `deploy/ocr/.env.example`
- Create: `deploy/ocr/README.md`

**Interfaces:**
- ให้บริการ: `http://ocr-proxy:8000` บน network `docflow-ocr` (ภายในเท่านั้น) และ `127.0.0.1:18000` บน host ของ VM
- ผู้ใช้: Task 12 ใช้ `OCR_API_URL=http://ocr-proxy:8000` และ `OCR_API_KEY`

- [ ] **Step 1: ตรวจชื่อ Dockerfile ของ thaidoc ที่ใช้ build CPU**

รันในเครื่อง Mac ที่มี clone ของ thaidoc:

```sh
ls /private/tmp/claude-501/-Users-kit-kku-kku-order/c29e7e4e-73cb-4b9d-b8ce-52b050e10a31/scratchpad/thaidoc-src
```

คาดหวัง: มี `Dockerfile` สำหรับ CPU ถ้าไม่ตรงให้จดชื่อจริงแล้วแก้ `dockerfile:` ใน compose ให้ตรง

- [ ] **Step 2: เขียน `deploy/ocr/compose.yml`**

```yaml
name: docflow-ocr

services:
  thaidoc:
    build:
      context: ${THAIDOC_SRC:?ตั้ง THAIDOC_SRC ใน .env (บน VM คือ ../thaidoc-src)}
      dockerfile: Dockerfile
    image: thaidoc-extract:${THAIDOC_VERSION:?ตั้ง THAIDOC_VERSION ใน .env}
    command: ["serve"]
    restart: unless-stopped
    networks:
      - ocr-internal
    healthcheck:
      test:
        - CMD
        - python
        - -c
        - "import urllib.request,json,sys; d=json.load(urllib.request.urlopen('http://127.0.0.1:8000/health')); sys.exit(0 if d.get('models_loaded') else 1)"
      interval: 15s
      timeout: 5s
      retries: 40
      start_period: 60s

  proxy:
    image: caddy:2.8.4-alpine
    restart: unless-stopped
    environment:
      OCR_API_KEY: ${OCR_API_KEY:?ตั้ง OCR_API_KEY ใน .env}
      OCR_SITE_ADDRESS: ${OCR_SITE_ADDRESS::8000}
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
    ports:
      - "${OCR_PROXY_PUBLISH:-127.0.0.1:18000}:8000"
    depends_on:
      thaidoc:
        condition: service_healthy
    networks:
      ocr-internal: {}
      docflow-ocr:
        aliases:
          - ocr-proxy

networks:
  ocr-internal: {}
  docflow-ocr:
    name: docflow-ocr
    driver: bridge
```

หมายเหตุ: `OCR_SITE_ADDRESS: ${OCR_SITE_ADDRESS::8000}` ใช้ค่าเริ่มต้น `:8000` (HTTP ภายใน) ตอนขึ้น GPU ให้ตั้งเป็นโดเมนจริง

- [ ] **Step 3: เขียน `deploy/ocr/Caddyfile`**

```caddyfile
{$OCR_SITE_ADDRESS::8000} {
	admin off

	route {
		request_body {
			max_size 50MB
		}

		@unauthorized {
			not header Authorization "Bearer {$OCR_API_KEY}"
		}
		respond @unauthorized 401

		@health {
			method GET
			path /health
		}
		@extract {
			method POST
			path /api/v1/extract-json
		}
		reverse_proxy @health thaidoc:8000
		reverse_proxy @extract thaidoc:8000

		respond 404
	}
}
```

- [ ] **Step 4: เขียน `deploy/ocr/.env.example`**

```dotenv
# คัดลอกเป็น .env แล้วแก้ค่า อย่า commit ไฟล์ .env
# บน VM ผังไฟล์คือ ~/docflow/ocr (compose นี้) และ ~/docflow/thaidoc-src
THAIDOC_SRC=../thaidoc-src
THAIDOC_VERSION=2026.10.11-cpu
OCR_API_KEY=ใส่ผลจาก openssl rand -hex 32
OCR_SITE_ADDRESS=:8000
OCR_PROXY_PUBLISH=127.0.0.1:18000
```

- [ ] **Step 5: เขียน `deploy/ocr/README.md` (ภาษาไทย)**

เนื้อหาต้องมีตามลำดับนี้:
1. ทำไมต้อง copy source: repo thaidoc เป็น private จึง clone บน Mac ด้วย `git clone --depth 1` แล้วส่งขึ้น VM ด้วย `rsync -a --exclude .git <path> project@10.198.200.62:~/docflow/thaidoc-src/`
2. สร้าง key ด้วย `openssl rand -hex 32` แล้วใส่ใน `.env` บน VM (ไม่ commit)
3. build ใช้เวลา 10 นาทีขึ้นไป รันเป็น background: `docker compose up -d --build` แล้วดูสถานะด้วย `docker compose ps`
4. ตรวจผลด้วย curl ตามลำดับนี้ (ทุกคำสั่งรันบน VM):
   - ไม่ส่ง key: `curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18000/health` คาดหวัง `401`
   - ส่ง key: `curl -s -H "Authorization: Bearer $OCR_API_KEY" http://127.0.0.1:18000/health` คาดหวัง `models_loaded` เป็น `true`
   - `/docs`: `curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $OCR_API_KEY" http://127.0.0.1:18000/docs` คาดหวัง `404`
   - extract: `time curl -s -H "Authorization: Bearer $OCR_API_KEY" -F file=@$HOME/kku-order-ocr/docs-samples/kku.pdf http://127.0.0.1:18000/api/v1/extract-json | head -c 2000` จับเวลาและตรวจว่ามี `data` และ `pages`
5. ขั้นถัดไปบนเครื่อง GPU: ใช้ `Dockerfile.gpu` ของ thaidoc, ตั้ง `OCR_SITE_ADDRESS` เป็นโดเมน และเปิดพอร์ต 80/443

- [ ] **Step 6: ตรวจไฟล์ก่อน commit**

```sh
cd /Users/kit/kku/kku-order-thaidoc && docker compose -f deploy/ocr/compose.yml --env-file deploy/ocr/.env.example config >/dev/null 2>&1; echo "exit=$?"
```

คาดหวัง: exit อาจไม่เป็น 0 เพราะ `OCR_API_KEY` ไม่ถูกตั้ง ให้ตรวจว่า error มีแค่เรื่องตัวแปรที่ `:?` บังคับ ถ้ามี error อื่นให้แก้

- [ ] **Step 7: ขั้น deploy บน VM (remote/manual)**

1. `rsync` source thaidoc ไปที่ `~/docflow/thaidoc-src/` และ `deploy/ocr/` ไปที่ `~/docflow/ocr/`
2. สร้าง `.env` จาก `.env.example` และใส่ key
3. `cd ~/docflow/ocr && docker compose up -d --build` (background, รอ 10–15 นาที)
4. รันคำสั่งตรวจจาก Step 5 ข้อ 4 ทั้งหมด จดเวลาของ extract และผลของ `pages`

- [ ] **Step 8: Commit**

```sh
git add deploy/ocr && git commit -m "Add thaidoc OCR stack behind a keyed Caddy proxy

Co-Authored-By: Claude Haiku 5.5 <noreply@anthropic.com>"
```

---

## Task 2: ชนิดข้อมูลกลาง และ decoder ของ thaidoc

**Files:**
- Create: `backend/extraction.go`
- Create: `backend/extractor_thaidoc.go`
- Create: `backend/extractor_thaidoc_test.go`

**Interfaces:**
- ผลิต: `ExtractionResult`, `ExtractedHeader`, `ExtractedGroup`, `ExtractedMember`, `ExtractedPage`, `ExtractedValue`, `Box`, `Extractor`, `ExtractError`
- ผลิต: `func parseThaidocResponse(raw []byte) (ExtractionResult, error)`

- [ ] **Step 1: ตรวจรูปแบบ fixture ก่อนเขียนเทสต์**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && jq '{subject, number, ref_number, signer, date, order, bbox_subject: .bbox.subject, bbox_ref: .bbox.ref_number, keys: (keys)}' testdata/extraction/thaidoc-extract-json.json | head -60
```

คาดหวัง: `subject`, `number`, `signer` เป็น string, `ref_number` เป็น array ของ string, `bbox.subject` เป็น array ของ box, `bbox.ref_number` เป็น array ของ array ถ้าไม่ตรงให้ปรับ struct ในขั้นตอน implement ให้ตรงกับผลจริง

- [ ] **Step 2: เขียนเทสต์ที่ล้มเหลว `backend/extractor_thaidoc_test.go`**

```go
package main

import (
	"os"
	"testing"
)

func loadThaidocFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/extraction/thaidoc-extract-json.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return raw
}

func TestParseThaidocResponseMapsHeaderAndBoxes(t *testing.T) {
	got, err := parseThaidocResponse(loadThaidocFixture(t))
	if err != nil {
		t.Fatalf("parseThaidocResponse: %v", err)
	}
	if got.Engine != "thaidoc" {
		t.Fatalf("Engine = %q, want thaidoc", got.Engine)
	}
	if got.Header.Subject.Text != "ระบบจัดการเอกสารคำสั่งแต่งตั้ง" {
		t.Fatalf("Subject.Text = %q", got.Header.Subject.Text)
	}
	if got.Header.Subject.Confidence != nil {
		t.Fatalf("Subject.Confidence = %v, want nil", *got.Header.Subject.Confidence)
	}
	box := got.Header.Subject.Boxes
	if len(box) != 1 || box[0].Page != 1 || box[0].X0 != 0.4171 || box[0].Y1 != 0.1929 {
		t.Fatalf("Subject.Boxes = %+v", box)
	}
	if got.Header.IssuedBy.Text != "ข้อกำหนดและขอบเขตของงาน" {
		t.Fatalf("IssuedBy.Text = %q (order -> IssuedBy)", got.Header.IssuedBy.Text)
	}
	if got.Header.Signer.Text != "ตัวพิมพ์" {
		t.Fatalf("Signer.Text = %q", got.Header.Signer.Text)
	}
	if got.Header.OrderNo.Text != "" {
		t.Fatalf("OrderNo.Text = %q, want empty (number is empty in fixture)", got.Header.OrderNo.Text)
	}
}

func TestParseThaidocResponseMapsGroupsAndMembers(t *testing.T) {
	got, err := parseThaidocResponse(loadThaidocFixture(t))
	if err != nil {
		t.Fatalf("parseThaidocResponse: %v", err)
	}
	if len(got.Groups) != 6 {
		t.Fatalf("len(Groups) = %d, want 6", len(got.Groups))
	}
	total := 0
	for _, g := range got.Groups {
		total += len(g.Members)
	}
	if total != 6 {
		t.Fatalf("total members = %d, want 6", total)
	}
	first := got.Groups[0].Members[0]
	if first.Name.Text != "วัชรวิศว์ ชัยปลื้ม" {
		t.Fatalf("first member name = %q", first.Name.Text)
	}
	if len(first.Name.Boxes) != 1 || first.Name.Boxes[0].X0 != 0.1903 {
		t.Fatalf("first member name boxes = %+v", first.Name.Boxes)
	}
	if first.Name.Confidence != nil || first.ExOfficio {
		t.Fatalf("first member: confidence/exOfficio = %v/%v", first.Name.Confidence, first.ExOfficio)
	}
}

func TestParseThaidocResponseMarksExOfficioWhenNameEmpty(t *testing.T) {
	got, err := parseThaidocResponse(loadThaidocFixture(t))
	if err != nil {
		t.Fatalf("parseThaidocResponse: %v", err)
	}
	found := false
	for _, g := range got.Groups {
		for _, m := range g.Members {
			if m.Name.Text == "" && m.Position.Text != "" {
				found = true
				if !m.ExOfficio {
					t.Fatalf("member with empty name and position %q not ExOfficio", m.Position.Text)
				}
			}
		}
	}
	if !found {
		t.Fatal("fixture has no ex-officio member; check fixture facts")
	}
}

func TestParseThaidocResponseMapsPageSize(t *testing.T) {
	got, err := parseThaidocResponse(loadThaidocFixture(t))
	if err != nil {
		t.Fatalf("parseThaidocResponse: %v", err)
	}
	if len(got.Pages) != 14 {
		t.Fatalf("len(Pages) = %d, want 14", len(got.Pages))
	}
	if got.Pages[0].Width != 1654 || got.Pages[0].Height != 2339 {
		t.Fatalf("page 1 size = %dx%d", got.Pages[0].Width, got.Pages[0].Height)
	}
}

func TestParseThaidocResponseRequiresDataKey(t *testing.T) {
	_, err := parseThaidocResponse([]byte(`{"subject":"x","pages":[]}`))
	if err == nil {
		t.Fatal("expected error when data key is missing")
	}
}
```

- [ ] **Step 3: รันเทสต์ ต้อง FAIL**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && go test ./... -run ParseThaidoc 2>&1 | tail -20
```

คาดหวัง: build fail ด้วย `undefined: parseThaidocResponse` (ยังไม่มี implementation)

- [ ] **Step 4: เขียน `backend/extraction.go`**

```go
package main

import (
	"context"
	"fmt"
)

// ExtractionResult คือผลกลางที่ทุก adapter ต้องคืน (JSON ของ type นี้คือ DocFlow contract)
type ExtractionResult struct {
	Engine string            `json:"engine"`
	Header ExtractedHeader   `json:"header"`
	Groups []ExtractedGroup  `json:"groups"`
	Pages  []ExtractedPage   `json:"pages,omitempty"`
}

type Box struct {
	Page int     `json:"page"`
	X0   float64 `json:"x0"`
	Y0   float64 `json:"y0"`
	X1   float64 `json:"x1"`
	Y1   float64 `json:"y1"`
}

type ExtractedValue struct {
	Text       string   `json:"text"`
	Confidence *float32 `json:"confidence,omitempty"`
	Boxes      []Box    `json:"boxes,omitempty"`
}

type ExtractedHeader struct {
	OrderNo       ExtractedValue   `json:"orderNo"`
	Subject       ExtractedValue   `json:"subject"`
	IssuedBy      ExtractedValue   `json:"issuedBy"`
	Date          ExtractedValue   `json:"date"`
	EffectiveDate ExtractedValue   `json:"effectiveDate"`
	ExpiryDate    ExtractedValue   `json:"expiryDate"`
	Signer        ExtractedValue   `json:"signer"`
	References    []ExtractedValue `json:"references"`
}

type ExtractedGroup struct {
	Department ExtractedValue   `json:"department"`
	Duty       ExtractedValue   `json:"duty"`
	Members    []ExtractedMember `json:"members"`
}

type ExtractedMember struct {
	Prefix    ExtractedValue `json:"prefix"`
	Name      ExtractedValue `json:"name"`
	Position  ExtractedValue `json:"position"`
	Role      ExtractedValue `json:"role"`
	ExOfficio bool           `json:"exOfficio"`
}

type ExtractedPage struct {
	Page   int              `json:"page"`
	Width  int              `json:"width"`
	Height int              `json:"height"`
	Lines  []ExtractedValue `json:"lines,omitempty"`
}

// Extractor คือ adapter ของ OCR API หนึ่งเจ้า
type Extractor interface {
	Name() string
	Extract(ctx context.Context, pdfPath, filename string) (ExtractionResult, error)
}

// ExtractError คือ error ที่คิวใช้ตัดสินว่าจะลองใหม่หรือไม่
type ExtractError struct {
	Retryable bool
	Status    int    // HTTP status, 0 ถ้าไม่ได้คำตอบ
	Message   string // ภาษาไทย แสดงในหน้าเว็บ
}

func (e *ExtractError) Error() string {
	return fmt.Sprintf("%s (status=%d retryable=%t)", e.Message, e.Status, e.Retryable)
}
```

- [ ] **Step 5: เขียน `backend/extractor_thaidoc.go`**

```go
package main

import (
	"encoding/json"
	"errors"
	"strings"
)

type thaidocPage struct {
	Page   int `json:"page"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type thaidocMemberBBox struct {
	Prefix   []Box `json:"prefix"`
	Name     []Box `json:"name"`
	Position []Box `json:"position"`
	Role     []Box `json:"role"`
}

type thaidocMember struct {
	Prefix   string            `json:"prefix"`
	Name     string            `json:"name"`
	Position string            `json:"position"`
	Role     string            `json:"role"`
	BBox     thaidocMemberBBox `json:"bbox"`
}

type thaidocGroup struct {
	Dept     string          `json:"dept"`
	Duty     string          `json:"duty"`
	ListName []thaidocMember `json:"list name"`
	BBox     struct {
		Dept []Box `json:"dept"`
		Duty []Box `json:"duty"`
	} `json:"bbox"`
}

type thaidocResponse struct {
	Subject       string         `json:"subject"`
	Date          string         `json:"date"`
	Order         string         `json:"order"`
	EffectiveDate string         `json:"effective date"`
	Number        string         `json:"number"`
	ExpiryDate    string         `json:"expiry date"`
	RefNumber     []string       `json:"ref_number"`
	Signer        string         `json:"signer"`
	Data          *[]thaidocGroup `json:"data"`
	Pages         []thaidocPage  `json:"pages"`
	BBox          struct {
		Subject       []Box   `json:"subject"`
		Date          []Box   `json:"date"`
		Order         []Box   `json:"order"`
		EffectiveDate []Box   `json:"effective date"`
		Number        []Box   `json:"number"`
		ExpiryDate    []Box   `json:"expiry date"`
		RefNumber     [][]Box `json:"ref_number"`
		Signer        []Box   `json:"signer"`
	} `json:"bbox"`
}

func thaidocValue(text string, boxes []Box) ExtractedValue {
	return ExtractedValue{Text: strings.TrimSpace(text), Boxes: boxes}
}

// parseThaidocResponse แปลงคำตอบของ thaidoc `/api/v1/extract-json` เป็น ExtractionResult
// thaidoc ไม่ให้ค่าความมั่นใจ จึงทุกค่า Confidence เป็น nil
func parseThaidocResponse(raw []byte) (ExtractionResult, error) {
	var resp thaidocResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return ExtractionResult{}, err
	}
	if resp.Data == nil {
		return ExtractionResult{}, errors.New("thaidoc response has no data key")
	}

	result := ExtractionResult{
		Engine: "thaidoc",
		Header: ExtractedHeader{
			OrderNo:       thaidocValue(resp.Number, resp.BBox.Number),
			Subject:       thaidocValue(resp.Subject, resp.BBox.Subject),
			IssuedBy:      thaidocValue(resp.Order, resp.BBox.Order),
			Date:          thaidocValue(resp.Date, resp.BBox.Date),
			EffectiveDate: thaidocValue(resp.EffectiveDate, resp.BBox.EffectiveDate),
			ExpiryDate:    thaidocValue(resp.ExpiryDate, resp.BBox.ExpiryDate),
			Signer:        thaidocValue(resp.Signer, resp.BBox.Signer),
		},
	}
	for i, ref := range resp.RefNumber {
		var boxes []Box
		if i < len(resp.BBox.RefNumber) {
			boxes = resp.BBox.RefNumber[i]
		}
		result.Header.References = append(result.Header.References, thaidocValue(ref, boxes))
	}

	for _, g := range *resp.Data {
		group := ExtractedGroup{
			Department: thaidocValue(g.Dept, g.BBox.Dept),
			Duty:       thaidocValue(g.Duty, g.BBox.Duty),
		}
		for _, m := range g.ListName {
			// ข้ามแถวที่ทั้งชื่อและตำแหน่งว่าง (ผลจากเอกสารที่ไม่ใช่คำสั่ง)
			if strings.TrimSpace(m.Name) == "" && strings.TrimSpace(m.Position) == "" {
				continue
			}
			member := ExtractedMember{
				Prefix:   thaidocValue(m.Prefix, m.BBox.Prefix),
				Name:     thaidocValue(m.Name, m.BBox.Name),
				Position: thaidocValue(m.Position, m.BBox.Position),
				Role:     thaidocValue(m.Role, m.BBox.Role),
			}
			member.ExOfficio = member.Name.Text == "" && member.Position.Text != ""
			group.Members = append(group.Members, member)
		}
		result.Groups = append(result.Groups, group)
	}

	for _, p := range resp.Pages {
		result.Pages = append(result.Pages, ExtractedPage{Page: p.Page, Width: p.Width, Height: p.Height})
	}
	return result, nil
}
```

หมายเหตุ: กรอบของสมาชิกที่ `name` ว่าง จะตกไปที่ `position` ใน `buildExtractionRecords` (Task 5) ไม่ต้องทำ fallback ใน decoder

- [ ] **Step 6: รันเทสต์ ต้อง PASS**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && gofmt -l . ; go test ./... -run ParseThaidoc -v 2>&1 | tail -30 && go test ./... 2>&1 | grep -E "^(--- FAIL|FAIL|ok)"
```

คาดหวัง: 5 เทสต์ PASS ถ้า `gofmt -l` แสดงไฟล์ให้รัน `gofmt -w` กับไฟล์ที่ระบุ และรันเต็มชุดเหลือเฉพาะ baseline `TestNewManualDirectoryUserStoresValidJSONPayload`

- [ ] **Step 7: Commit**

```sh
git add backend/extraction.go backend/extractor_thaidoc.go backend/extractor_thaidoc_test.go && git commit -m "Add extraction result types and thaidoc response decoder

Co-Authored-By: Claude Haiku 5.5 <noreply@anthropic.com>"
```

---

## Task 3: decoder ของ DocFlow contract และเอกสาร contract

**Files:**
- Create: `backend/extractor_contract.go`
- Create: `backend/extractor_contract_test.go`
- Create: `backend/testdata/extraction/docflow-contract.json`
- Create: `docs/ocr-api-contract.md`

**Interfaces:**
- ผลิต: `func parseContractResponse(raw []byte) (ExtractionResult, error)`

- [ ] **Step 1: เขียน fixture `backend/testdata/extraction/docflow-contract.json`**

```json
{
  "engine": "docflow-contract",
  "header": {
    "orderNo": {"text": "1234/2569", "confidence": 0.94, "boxes": [{"page": 1, "x0": 0.4, "y0": 0.1, "x1": 0.6, "y1": 0.12}]},
    "subject": {"text": "แต่งตั้งคณะกรรมการ", "confidence": 0.91, "boxes": [{"page": 1, "x0": 0.2, "y0": 0.2, "x1": 0.8, "y1": 0.23}]},
    "issuedBy": {"text": "คณะกรรมการบริหาร", "confidence": 0.9},
    "date": {"text": "1 ตุลาคม 2569", "confidence": 0.88},
    "effectiveDate": {"text": "2 ตุลาคม 2569", "confidence": 0.88},
    "expiryDate": {"text": "", "confidence": 0.0},
    "signer": {"text": "นายตัวอย่าง ทดสอบ", "confidence": 0.85},
    "references": [{"text": "ที่ ศธ 0001/2569", "confidence": 0.8}]
  },
  "groups": [
    {
      "department": {"text": "สำนักงานอธิการบดี", "confidence": 0.9},
      "duty": {"text": "กรรมการ", "confidence": 0.9},
      "members": [
        {
          "prefix": {"text": "นาย", "confidence": 0.95},
          "name": {"text": "สมชาย ใจดี", "confidence": 0.93, "boxes": [{"page": 1, "x0": 0.2, "y0": 0.4, "x1": 0.35, "y1": 0.42}]},
          "position": {"text": "ผู้อำนวยการ", "confidence": 0.9},
          "role": {"text": "ประธาน", "confidence": 0.9},
          "exOfficio": false
        },
        {
          "prefix": {"text": "", "confidence": 0.0},
          "name": {"text": "", "confidence": 0.0},
          "position": {"text": "เลขานุการ", "confidence": 0.9},
          "role": {"text": "", "confidence": 0.0},
          "exOfficio": true
        }
      ]
    }
  ],
  "pages": [
    {"page": 1, "width": 1654, "height": 2339, "lines": [{"text": "คำสั่งแต่งตั้ง", "confidence": 0.97, "boxes": [{"page": 1, "x0": 0.3, "y0": 0.05, "x1": 0.7, "y1": 0.07}]}]}
  ]
}
```

- [ ] **Step 2: เขียนเทสต์ที่ล้มเหลว `backend/extractor_contract_test.go`**

```go
package main

import (
	"os"
	"testing"
)

func TestParseContractResponseReadsFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/extraction/docflow-contract.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	got, err := parseContractResponse(raw)
	if err != nil {
		t.Fatalf("parseContractResponse: %v", err)
	}
	if got.Engine != "docflow-contract" {
		t.Fatalf("Engine = %q", got.Engine)
	}
	if got.Header.OrderNo.Text != "1234/2569" || got.Header.OrderNo.Confidence == nil || *got.Header.OrderNo.Confidence != 0.94 {
		t.Fatalf("OrderNo = %+v", got.Header.OrderNo)
	}
	if len(got.Header.References) != 1 || got.Header.References[0].Text != "ที่ ศธ 0001/2569" {
		t.Fatalf("References = %+v", got.Header.References)
	}
	if len(got.Groups) != 1 || len(got.Groups[0].Members) != 2 {
		t.Fatalf("Groups = %+v", got.Groups)
	}
	if got.Groups[0].Members[0].Name.Boxes[0].X0 != 0.2 {
		t.Fatalf("member name boxes = %+v", got.Groups[0].Members[0].Name.Boxes)
	}
	if !got.Groups[0].Members[1].ExOfficio {
		t.Fatal("second member should be ExOfficio")
	}
	if len(got.Pages) != 1 || got.Pages[0].Width != 1654 || len(got.Pages[0].Lines) != 1 {
		t.Fatalf("Pages = %+v", got.Pages)
	}
}

func TestParseContractResponseRequiresGroupsKey(t *testing.T) {
	_, err := parseContractResponse([]byte(`{"engine":"docflow-contract","header":{}}`))
	if err == nil {
		t.Fatal("expected error when groups key is missing")
	}
}
```

- [ ] **Step 3: รันเทสต์ ต้อง FAIL**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && go test ./... -run ParseContract 2>&1 | tail -10
```

คาดหวัง: `undefined: parseContractResponse`

- [ ] **Step 4: เขียน `backend/extractor_contract.go`**

```go
package main

import (
	"encoding/json"
	"errors"
)

// parseContractResponse อ่าน JSON ที่ตรงกับ DocFlow contract (ดู docs/ocr-api-contract.md)
// ต้องมี key "groups" (ว่างได้) มิฉะนั้นถือว่าไม่ใช่ contract
func parseContractResponse(raw []byte) (ExtractionResult, error) {
	var probe struct {
		Groups *[]ExtractedGroup `json:"groups"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return ExtractionResult{}, err
	}
	if probe.Groups == nil {
		return ExtractionResult{}, errors.New("contract response has no groups key")
	}
	var result ExtractionResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return ExtractionResult{}, err
	}
	if result.Engine == "" {
		result.Engine = "docflow-contract"
	}
	return result, nil
}
```

- [ ] **Step 5: รันเทสต์ ต้อง PASS**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && go test ./... -run ParseContract -v 2>&1 | tail -10 && go test ./... 2>&1 | grep -E "^(--- FAIL|FAIL|ok)"
```

- [ ] **Step 6: เขียน `docs/ocr-api-contract.md` (ภาษาไทย)**

หัวข้อที่ต้องมีตามลำดับ:
1. ภาพรวม: เอนด์พอยต์ `POST {URL}` multipart field `file` และ header `Authorization: Bearer <key>` (ถ้ามี key)
2. ตารางโครงสร้าง: `engine`, `header` (`orderNo`, `subject`, `issuedBy`, `date`, `effectiveDate`, `expiryDate`, `signer`, `references[]`), `groups[]` (`department`, `duty`, `members[]` ที่มี `prefix`, `name`, `position`, `role`, `exOfficio`), `pages[]` (`page`, `width`, `height`, `lines[]`)
3. Value object: `{text, confidence?, boxes[]}` โดย `confidence` เป็น 0–1 หรือไม่ส่ง, `boxes` เป็นค่า normalized 0–1 จากมุมซ้ายบน
4. กฎ: ถ้า `confidence` ใดไม่ส่ง ระบบถือว่าไม่ทราบและเอกสารต้องผ่านคนตรวจ; `exOfficio=true` เมื่อชื่อว่างและตำแหน่งมีค่า
5. Health: `GET {origin}/health` ตอบ 200 และ `{"models_loaded": true|false}` (ถ้าไม่มีฟิลด์นี้ถือว่าพร้อม)
6. ตัวอย่างเต็ม: ชี้ไปที่ `backend/testdata/extraction/docflow-contract.json`
7. HTTP status: 2xx = สำเร็จ, 400/413/415/422 = ไฟล์ไม่ถูกต้องถาวร, 401/403 = key ผิด, 429/502/503/504 และการเชื่อมต่อล้ม = ลองใหม่ได้, 500 ครั้งแรก = ลองใหม่ ครั้งที่สอง = ถาวร

- [ ] **Step 7: Commit**

```sh
git add backend/extractor_contract.go backend/extractor_contract_test.go backend/testdata/extraction/docflow-contract.json docs/ocr-api-contract.md && git commit -m "Add DocFlow contract decoder, fixture and API contract doc

Co-Authored-By: Claude Haiku 5.5 <noreply@anthropic.com>"
```

---

## Task 4: HTTP extractor, การแบ่ง error, config และการตรวจไฟล์

**Files:**
- Create: `backend/extractor_http.go`
- Create: `backend/extractor_http_test.go`
- Create: `backend/ocr_config.go`
- Create: `backend/ocr_config_test.go`
- Modify: `backend/settings.go` (เพิ่มฟิลด์ `APIURL string json:"apiUrl"` ใน `OCRSettings` เท่านั้น ฟิลด์นี้ต้องมีตั้งแต่ Task นี้เพราะ `newExtractor` อ่านค่า)

**Interfaces:**
- ผลิต: `type httpExtractor struct{ engine, endpoint, apiKey string; client *http.Client; decode func([]byte) (ExtractionResult, error) }` ที่ implement `Extractor`
- ผลิต: `func classifyHTTPFailure(status int, body []byte) *ExtractError` (ใช้ฟิลด์ `detail` ของคำตอบ 400/413/415/422 ในข้อความ)
- ผลิต: `type ocrAPIConfig struct{ DefaultURL, APIKey string; Timeout time.Duration; Concurrency int }`, `loadOCRAPIConfig(getenv func(string) string, readFile func(string) ([]byte, error)) ocrAPIConfig`, `loadOCRAPIConfigFromEnv() ocrAPIConfig`
- ผลิต: `func ocrBaseURL(settingsURL, defaultURL string) string` (ใช้ค่าที่บันทึกก่อน ถ้าว่างใช้ env; Task 8 ใช้ร่วม)
- ผลิต: `func newExtractor(settings OCRSettings, cfg ocrAPIConfig) (Extractor, error)`
- ผลิต: `type pageCounter func(ctx context.Context, path string) int`, `func checkPDFLimits(ctx context.Context, path string, countPages pageCounter) error`
- ใช้: `detectPDFPageCount(ctx context.Context, path string) int` ใน `backend/document_processing.go` ซึ่งมี signature ตรงกับ `pageCounter`

- [ ] **Step 1: ยืนยันฟังก์ชันนับหน้าที่มีอยู่**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && grep -n "func detectPDFPageCount" document_processing.go
```

คาดหวัง: หนึ่งบรรทัด `func detectPDFPageCount(ctx context.Context, path string) int` (ใช้ `pdfinfo`) เดิมจำกัด 100 หน้าใน `runOCR` ซึ่งจะถูกลบใน Task 7 โค้ดเดิมไม่มีการตรวจ 50 MB จึงเพิ่มใน `checkPDFLimits`

- [ ] **Step 2: เขียนเทสต์ที่ล้มเหลว `backend/extractor_http_test.go`** (ไฟล์ใหม่)

```go
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTempPDF(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4 test"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func newTestHTTPExtractor(url, key string) *httpExtractor {
	return &httpExtractor{
		engine:   "docflow-contract",
		endpoint: url,
		apiKey:   key,
		client:   &http.Client{Timeout: 2 * time.Second},
		decode: func(raw []byte) (ExtractionResult, error) {
			var r ExtractionResult
			err := json.Unmarshal(raw, &r)
			return r, err
		},
	}
}

func TestHTTPExtractorSendsBearerOnlyWhenKeySet(t *testing.T) {
	var gotAuth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{"engine":"x","groups":[]}`))
	}))
	defer srv.Close()
	pdf := writeTempPDF(t)

	if _, err := newTestHTTPExtractor(srv.URL, "secret").Extract(context.Background(), pdf, "a.pdf"); err != nil {
		t.Fatal(err)
	}
	if _, err := newTestHTTPExtractor(srv.URL, "").Extract(context.Background(), pdf, "a.pdf"); err != nil {
		t.Fatal(err)
	}
	if gotAuth[0] != "Bearer secret" || gotAuth[1] != "" {
		t.Fatalf("Authorization headers = %q", gotAuth)
	}
}

func TestHTTPExtractorSendsMultipartFileField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		f.Close()
		_, _ = w.Write([]byte(`{"engine":"x","groups":[]}`))
	}))
	defer srv.Close()
	if _, err := newTestHTTPExtractor(srv.URL, "").Extract(context.Background(), writeTempPDF(t), "a.pdf"); err != nil {
		t.Fatalf("multipart field file missing: %v", err)
	}
}

func TestHTTPExtractorClassifiesStatusCodes(t *testing.T) {
	cases := []struct {
		status    int
		retryable bool
	}{
		{http.StatusTooManyRequests, true},
		{http.StatusBadGateway, true},
		{http.StatusServiceUnavailable, true},
		{http.StatusGatewayTimeout, true},
		{http.StatusInternalServerError, true},
		{http.StatusBadRequest, false},
		{http.StatusRequestEntityTooLarge, false},
		{http.StatusUnsupportedMediaType, false},
		{http.StatusUnprocessableEntity, false},
		{http.StatusUnauthorized, false},
		{http.StatusForbidden, false},
		{http.StatusNotFound, false},
	}
	for _, c := range cases {
		c := c
		t.Run(http.StatusText(c.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
			}))
			defer srv.Close()
			_, err := newTestHTTPExtractor(srv.URL, "").Extract(context.Background(), writeTempPDF(t), "a.pdf")
			ee, ok := err.(*ExtractError)
			if !ok {
				t.Fatalf("err = %T %v, want *ExtractError", err, err)
			}
			if ee.Retryable != c.retryable || ee.Status != c.status || ee.Message == "" {
				t.Fatalf("got %+v", ee)
			}
		})
	}
}

func TestClassifyHTTPFailureIncludesAPIDetail(t *testing.T) {
	ee := classifyHTTPFailure(http.StatusBadRequest, []byte(`{"detail":"a.txt: not a readable PDF or image"}`))
	if ee.Retryable || !strings.Contains(ee.Message, "not a readable PDF") {
		t.Fatalf("got %+v", ee)
	}
}

func TestHTTPExtractorConnectionFailureIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	_, err := newTestHTTPExtractor(url, "").Extract(context.Background(), writeTempPDF(t), "a.pdf")
	ee, ok := err.(*ExtractError)
	if !ok || !ee.Retryable || ee.Status != 0 {
		t.Fatalf("err = %#v, want retryable ExtractError with status 0", err)
	}
}

func TestHTTPExtractorTimeoutIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()
	ex := newTestHTTPExtractor(srv.URL, "")
	ex.client = &http.Client{Timeout: 50 * time.Millisecond}
	_, err := ex.Extract(context.Background(), writeTempPDF(t), "a.pdf")
	ee, ok := err.(*ExtractError)
	if !ok || !ee.Retryable {
		t.Fatalf("err = %#v, want retryable timeout", err)
	}
}

func TestHTTPExtractorBadBodyIsPermanent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	_, err := newTestHTTPExtractor(srv.URL, "").Extract(context.Background(), writeTempPDF(t), "a.pdf")
	ee, ok := err.(*ExtractError)
	if !ok || ee.Retryable {
		t.Fatalf("err = %#v, want permanent ExtractError", err)
	}
}
```

- [ ] **Step 3: เขียนเทสต์ที่ล้มเหลว `backend/ocr_config_test.go`** (ไฟล์ใหม่)

```go
package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func fakeEnv(values map[string]string) func(string) string {
	return func(k string) string { return values[k] }
}

func fixedPageCount(n int) pageCounter {
	return func(context.Context, string) int { return n }
}

func TestLoadOCRAPIConfigDefaults(t *testing.T) {
	cfg := loadOCRAPIConfig(fakeEnv(nil), func(string) ([]byte, error) { return nil, errors.New("none") })
	if cfg.Timeout != 10*time.Minute || cfg.Concurrency != 1 || cfg.APIKey != "" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestLoadOCRAPIConfigReadsKeyFileAndClampsConcurrency(t *testing.T) {
	env := map[string]string{
		"OCR_API_KEY_FILE":    "/run/secrets/ocr",
		"OCR_API_TIMEOUT":     "3m",
		"OCR_API_CONCURRENCY": "99",
	}
	cfg := loadOCRAPIConfig(fakeEnv(env), func(p string) ([]byte, error) {
		if p != "/run/secrets/ocr" {
			return nil, errors.New("unexpected path")
		}
		return []byte("  key-from-file\n"), nil
	})
	if cfg.APIKey != "key-from-file" || cfg.Timeout != 3*time.Minute || cfg.Concurrency != 8 {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestOCRBaseURLPrefersSettingsThenDefault(t *testing.T) {
	if got := ocrBaseURL(" https://a.example/ ", "https://b.example"); got != "https://a.example" {
		t.Fatalf("settings url = %q", got)
	}
	if got := ocrBaseURL("", "https://b.example/"); got != "https://b.example" {
		t.Fatalf("default url = %q", got)
	}
}

func TestNewExtractorBuildsEndpointPerEngine(t *testing.T) {
	cfg := ocrAPIConfig{DefaultURL: "https://ocr.example", Timeout: time.Minute, Concurrency: 1}
	ex, err := newExtractor(OCRSettings{Engine: "thaidoc"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := ex.(*httpExtractor)
	if h.endpoint != "https://ocr.example/api/v1/extract-json" || h.engine != "thaidoc" {
		t.Fatalf("thaidoc endpoint = %q", h.endpoint)
	}
	ex, err = newExtractor(OCRSettings{Engine: "docflow-contract", APIURL: "https://x.example/extract"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if ex.(*httpExtractor).endpoint != "https://x.example/extract" {
		t.Fatalf("contract endpoint = %q", ex.(*httpExtractor).endpoint)
	}
}

func TestNewExtractorRejectsMissingURLAndUnknownEngine(t *testing.T) {
	if _, err := newExtractor(OCRSettings{Engine: "thaidoc"}, ocrAPIConfig{}); err == nil {
		t.Fatal("expected error when no URL is configured")
	}
	if _, err := newExtractor(OCRSettings{Engine: "nope", APIURL: "https://x"}, ocrAPIConfig{}); err == nil {
		t.Fatal("expected error for unknown engine")
	}
}

func TestCheckPDFLimitsRejectsMissingFile(t *testing.T) {
	err := checkPDFLimits(context.Background(), "/nonexistent/file.pdf", fixedPageCount(1))
	var ee *ExtractError
	if !errors.As(err, &ee) || ee.Retryable {
		t.Fatalf("err = %#v, want permanent ExtractError", err)
	}
}

func TestCheckPDFLimitsRejectsOversizeFile(t *testing.T) {
	path := writeTempPDF(t)
	if err := os.Truncate(path, maxPDFBytes+1); err != nil {
		t.Fatal(err)
	}
	err := checkPDFLimits(context.Background(), path, fixedPageCount(1))
	var ee *ExtractError
	if !errors.As(err, &ee) || ee.Retryable || ee.Status != 413 {
		t.Fatalf("err = %#v, want permanent 413", err)
	}
}

func TestCheckPDFLimitsRejectsMoreThan100Pages(t *testing.T) {
	err := checkPDFLimits(context.Background(), writeTempPDF(t), fixedPageCount(101))
	var ee *ExtractError
	if !errors.As(err, &ee) || ee.Retryable || ee.Status != 413 {
		t.Fatalf("err = %#v, want permanent 413 for 101 pages", err)
	}
}

func TestCheckPDFLimitsAcceptsExactly100Pages(t *testing.T) {
	if err := checkPDFLimits(context.Background(), writeTempPDF(t), fixedPageCount(100)); err != nil {
		t.Fatalf("100 pages must be accepted: %v", err)
	}
}
```

- [ ] **Step 4: รันเทสต์ ต้อง FAIL**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && go test ./... -run 'HTTPExtractor|ClassifyHTTP|OCRAPIConfig|OCRBaseURL|NewExtractor|CheckPDFLimits' 2>&1 | tail -10
```

คาดหวัง: build fail ด้วย `undefined` ของ `httpExtractor`, `classifyHTTPFailure`, `ocrBaseURL`, `checkPDFLimits` และ `APIURL`

- [ ] **Step 5: implement**

1. `backend/settings.go` ใน `OCRSettings` เพิ่มบรรทัดหลัง `Engine`:

```go
	APIURL                string  `json:"apiUrl"`
```

2. `backend/extractor_http.go` (ไฟล์ใหม่):

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type httpExtractor struct {
	engine   string
	endpoint string
	apiKey   string
	client   *http.Client
	decode   func([]byte) (ExtractionResult, error)
}

func (e *httpExtractor) Name() string { return e.engine }

func (e *httpExtractor) Extract(ctx context.Context, pdfPath, filename string) (ExtractionResult, error) {
	file, err := os.Open(pdfPath)
	if err != nil {
		return ExtractionResult{}, &ExtractError{Message: "เปิดไฟล์ PDF ไม่ได้: " + err.Error()}
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return ExtractionResult{}, &ExtractError{Message: "สร้างคำขอไม่ได้: " + err.Error()}
	}
	if _, err := io.Copy(part, file); err != nil {
		return ExtractionResult{}, &ExtractError{Message: "อ่านไฟล์ไม่ได้: " + err.Error()}
	}
	if err := writer.Close(); err != nil {
		return ExtractionResult{}, &ExtractError{Message: "สร้างคำขอไม่ได้: " + err.Error()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, body)
	if err != nil {
		return ExtractionResult{}, &ExtractError{Message: "URL ของ OCR API ไม่ถูกต้อง"}
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		// การเชื่อมต่อล้มหรือ timeout เป็นปัญหาชั่วคราว ลองใหม่ได้
		return ExtractionResult{}, &ExtractError{Retryable: true, Message: "เชื่อมต่อ OCR API ไม่ได้: " + err.Error()}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return ExtractionResult{}, &ExtractError{Retryable: true, Message: "อ่านคำตอบจาก OCR API ไม่ครบ: " + err.Error()}
	}
	if resp.StatusCode/100 != 2 {
		return ExtractionResult{}, classifyHTTPFailure(resp.StatusCode, raw)
	}
	result, err := e.decode(raw)
	if err != nil {
		return ExtractionResult{}, &ExtractError{Message: "คำตอบจาก OCR API อ่านไม่ได้: " + err.Error()}
	}
	return result, nil
}

// classifyHTTPFailure ตามตาราง "การแบ่ง error" ใน spec
// 500 คืนเป็น Retryable เสมอ คิวจะทำให้ครั้งที่สองของเอกสารเป็นถาวรจาก extraction_last_status
func classifyHTTPFailure(status int, body []byte) *ExtractError {
	ee := &ExtractError{Status: status}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		ee.Message = "API key ไม่ถูกต้อง หรือไม่มีสิทธิ์เรียกใช้ OCR"
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity:
		ee.Message = fmt.Sprintf("OCR API ปฏิเสธไฟล์ (%d)", status)
		if detail := apiDetail(body); detail != "" {
			ee.Message += ": " + detail
		}
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		ee.Retryable = true
		ee.Message = fmt.Sprintf("OCR API ยังไม่พร้อมให้บริการ (%d)", status)
	case http.StatusInternalServerError:
		ee.Retryable = true
		ee.Message = "OCR API เกิดข้อผิดพลาดภายใน (500)"
	default:
		ee.Message = fmt.Sprintf("OCR API ตอบสถานะที่ไม่คาดคิด (%d) ตรวจ URL ที่ตั้งค่าไว้", status)
	}
	return ee
}

// apiDetail อ่านฟิลด์ detail ของคำตอบ error (รูปแบบ FastAPI) ถ้ามี
func apiDetail(body []byte) string {
	var v struct {
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return ""
	}
	return strings.TrimSpace(v.Detail)
}
```

3. `backend/ocr_config.go` (ไฟล์ใหม่):

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	maxPDFBytes = 50 << 20
	maxPDFPages = 100
)

// pageCounter นับหน้า PDF ตอนใช้งานจริงคือ detectPDFPageCount (pdfinfo) เทสต์ส่งฟังก์ชันปลอมแทน
type pageCounter func(ctx context.Context, path string) int

type ocrAPIConfig struct {
	DefaultURL  string
	APIKey      string
	Timeout     time.Duration
	Concurrency int
}

func loadOCRAPIConfig(getenv func(string) string, readFile func(string) ([]byte, error)) ocrAPIConfig {
	cfg := ocrAPIConfig{
		DefaultURL:  strings.TrimSpace(getenv("OCR_API_URL")),
		APIKey:      strings.TrimSpace(getenv("OCR_API_KEY")),
		Timeout:     10 * time.Minute,
		Concurrency: 1,
	}
	if cfg.APIKey == "" {
		if path := strings.TrimSpace(getenv("OCR_API_KEY_FILE")); path != "" {
			if raw, err := readFile(path); err == nil {
				cfg.APIKey = strings.TrimSpace(string(raw))
			}
		}
	}
	if d, err := time.ParseDuration(getenv("OCR_API_TIMEOUT")); err == nil && d > 0 {
		cfg.Timeout = d
	}
	if n, err := strconv.Atoi(getenv("OCR_API_CONCURRENCY")); err == nil {
		cfg.Concurrency = min(max(n, 1), 8)
	}
	return cfg
}

// loadOCRAPIConfigFromEnv ใช้ตอน start ระบบจริง
func loadOCRAPIConfigFromEnv() ocrAPIConfig {
	return loadOCRAPIConfig(os.Getenv, os.ReadFile)
}

// ocrBaseURL ใช้ URL ที่บันทึกไว้ก่อน ถ้าว่างใช้ค่าจาก env (OCR_API_URL)
func ocrBaseURL(settingsURL, defaultURL string) string {
	base := strings.TrimRight(strings.TrimSpace(settingsURL), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(defaultURL), "/")
	}
	return base
}

func newExtractor(settings OCRSettings, cfg ocrAPIConfig) (Extractor, error) {
	base := ocrBaseURL(settings.APIURL, cfg.DefaultURL)
	if base == "" {
		return nil, &ExtractError{Message: "ยังไม่ได้ตั้งค่า URL ของ OCR API"}
	}
	client := &http.Client{Timeout: cfg.Timeout}
	switch settings.Engine {
	case "thaidoc":
		return &httpExtractor{
			engine:   "thaidoc",
			endpoint: base + "/api/v1/extract-json",
			apiKey:   cfg.APIKey,
			client:   client,
			decode:   parseThaidocResponse,
		}, nil
	case "docflow-contract":
		return &httpExtractor{
			engine:   "docflow-contract",
			endpoint: base,
			apiKey:   cfg.APIKey,
			client:   client,
			decode:   parseContractResponse,
		}, nil
	default:
		return nil, fmt.Errorf("unknown OCR engine %q", settings.Engine)
	}
}

// checkPDFLimits ตรวจก่อนส่ง: ไม่มีไฟล์, เกิน 50 MB หรือเกิน 100 หน้าเป็นถาวร (ไม่ลองใหม่)
func checkPDFLimits(ctx context.Context, path string, countPages pageCounter) error {
	info, err := os.Stat(path)
	if err != nil {
		return &ExtractError{Message: "ไม่พบไฟล์ PDF"}
	}
	if info.Size() > maxPDFBytes {
		return &ExtractError{Status: 413, Message: "ไฟล์ PDF ใหญ่เกิน 50 MB"}
	}
	if pages := countPages(ctx, path); pages > maxPDFPages {
		return &ExtractError{Status: 413, Message: fmt.Sprintf("PDF มี %d หน้า เกินขอบเขตสูงสุด %d หน้า", pages, maxPDFPages)}
	}
	return nil
}
```

- [ ] **Step 6: รันเทสต์ ต้อง PASS**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && gofmt -l . ; go vet ./... && go test ./... -run 'HTTPExtractor|ClassifyHTTP|OCRAPIConfig|OCRBaseURL|NewExtractor|CheckPDFLimits' -v 2>&1 | tail -40 && go test ./... 2>&1 | grep -E "^(--- FAIL|FAIL|ok)"
```

คาดหวัง: เทสต์ของ Task นี้ PASS และการรันเต็มชุดเหลือเฉพาะ baseline `TestNewManualDirectoryUserStoresValidJSONPayload`

- [ ] **Step 7: Commit**

```sh
cd /Users/kit/kku/kku-order-thaidoc && git add backend/extractor_http.go backend/extractor_http_test.go backend/ocr_config.go backend/ocr_config_test.go backend/settings.go && git commit -m "Add HTTP OCR extractor, error classification, config and PDF limits

Co-Authored-By: Claude Haiku 5.5 <noreply@anthropic.com>"
```

---

## Task 5: การแปลงผลเป็นแถวในฐานข้อมูล (ฟังก์ชันบริสุทธิ์)

**Files:**
- Create: `backend/extraction_persist.go`
- Create: `backend/extraction_persist_test.go`
- Modify: `backend/directory_matching.go` (เพิ่ม `matchAppointments` ที่ข้ามแถวไม่มีชื่อ, ให้ `applyDirectoryMatches` เรียกใช้, `confirmBlockers` ใช้ `personLabel`)
- Modify: `backend/directory_matching_test.go` (**มีไฟล์อยู่แล้ว ต้องต่อท้ายเทสต์ ห้ามสร้างใหม่หรือเขียนทับ**)

**Interfaces:**
- ใช้ของเดิม (ไม่ประกาศซ้ำ): `truncateRunes(value string, limit int) string` ใน `backend/document_store.go`, `needsReview(averageConfidence float32, lowConfidence bool, threshold float64) bool` ใน `settings.go`, `parseThaiDate`, `inferOrderType`, `findOrderYear` ใน `document_processing.go`, `loadDirectory(db) ([]DirectoryUser, map[string]uint, error)` ใน `directory_repository.go`
- ผลิต: `type extractionRecords struct{ Fields []ExtractedField; Appointments []Appointment; OCRPages []OCRPage; OCRLines []OCRLine; References []DocumentReference; Updates map[string]any; AverageConfidence float32; MissingConfidence bool; LowConfidence bool }`
- ผลิต: `func buildExtractionRecords(result ExtractionResult, threshold float64) extractionRecords` (ไม่แตะ DB)
- ผลิต: `func extractionStatus(rec extractionRecords, threshold float64) (status, stage string)` คืน `NEEDS_REVIEW`/`REVIEW` และ `EXTRACTED`
- ผลิต: `func personLabel(a Appointment) string` คืน `FullName` หรือ `<Position> (โดยตำแหน่ง)`
- ผลิต: `func matchAppointments(appointments []Appointment, users []DirectoryUser, aliases map[string]uint) bool` ใน `directory_matching.go`

**กฎที่ใช้ (ตาม spec บรรทัด 138):** นับความมั่นใจเฉพาะค่าที่ไม่ว่างใน `Header` และ `Members` (`prefix`, `name`, `position`, `role`) ไม่นับ `Department`/`Duty`

- [ ] **Step 1: เขียนเทสต์ที่ล้มเหลว `backend/extraction_persist_test.go`** (ไฟล์ใหม่)

```go
package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func f32(v float32) *float32 { return &v }

func sampleResult() ExtractionResult {
	return ExtractionResult{
		Engine: "thaidoc",
		Header: ExtractedHeader{
			OrderNo:       ExtractedValue{Text: "1234/2569", Boxes: []Box{{Page: 1, X0: 0.4, Y0: 0.1, X1: 0.6, Y1: 0.12}}},
			Subject:       ExtractedValue{Text: "แต่งตั้งคณะกรรมการ"},
			IssuedBy:      ExtractedValue{Text: "คณะกรรมการบริหาร"},
			Date:          ExtractedValue{Text: "1 ตุลาคม 2569"},
			EffectiveDate: ExtractedValue{Text: "ไม่ใช่วันที่"},
			Signer:        ExtractedValue{Text: "นายตัวอย่าง ทดสอบ"},
			References:    []ExtractedValue{{Text: "ที่ ศธ 0001/2569"}},
		},
		Groups: []ExtractedGroup{{
			Department: ExtractedValue{Text: "สำนักงาน"},
			Duty:       ExtractedValue{Text: "กรรมการ"},
			Members: []ExtractedMember{
				{Name: ExtractedValue{Text: "สมชาย ใจดี"}, Position: ExtractedValue{Text: "ผู้อำนวยการ"}, Role: ExtractedValue{Text: "ประธาน"}},
				{Position: ExtractedValue{Text: "เลขานุการ"}, ExOfficio: true},
			},
		}},
		Pages: []ExtractedPage{{Page: 1, Width: 1654, Height: 2339}},
	}
}

func TestBuildExtractionRecordsNilConfidenceNeedsReview(t *testing.T) {
	rec := buildExtractionRecords(sampleResult(), 0.8)
	if !rec.MissingConfidence {
		t.Fatal("MissingConfidence should be true when API gives no confidence")
	}
	status, stage := extractionStatus(rec, 0.8)
	if status != "NEEDS_REVIEW" || stage != "EXTRACTED" {
		t.Fatalf("status/stage = %s/%s", status, stage)
	}
	if rec.Updates["confidence"] != float32(0) {
		t.Fatalf("confidence = %v, want 0 when nothing has a value", rec.Updates["confidence"])
	}
}

func TestBuildExtractionRecordsAverageAndThreshold(t *testing.T) {
	r := sampleResult()
	r.Header.OrderNo.Confidence = f32(0.9)
	r.Header.Subject.Confidence = f32(0.9)
	r.Header.IssuedBy.Confidence = f32(0.9)
	r.Header.Date.Confidence = f32(0.9)
	r.Header.EffectiveDate.Confidence = f32(0.9)
	r.Header.Signer.Confidence = f32(0.9)
	r.Header.References[0].Confidence = f32(0.9)
	r.Groups[0].Members[0].Name.Confidence = f32(0.9)
	r.Groups[0].Members[0].Position.Confidence = f32(0.9)
	r.Groups[0].Members[0].Role.Confidence = f32(0.9)
	r.Groups[0].Members[1].Position.Confidence = f32(0.5)
	rec := buildExtractionRecords(r, 0.8)
	if rec.MissingConfidence {
		t.Fatal("MissingConfidence should be false: every non-empty Header/Member value has confidence (Department and Duty are not counted)")
	}
	if !rec.LowConfidence {
		t.Fatal("LowConfidence should be true when a value is below threshold")
	}
	status, _ := extractionStatus(rec, 0.8)
	if status != "NEEDS_REVIEW" {
		t.Fatalf("status = %s, want NEEDS_REVIEW because of low confidence", status)
	}
}

func TestBuildExtractionRecordsAppointmentsAndExOfficio(t *testing.T) {
	rec := buildExtractionRecords(sampleResult(), 0.8)
	if len(rec.Appointments) != 2 {
		t.Fatalf("appointments = %d, want 2", len(rec.Appointments))
	}
	if rec.Appointments[0].FullName != "สมชาย ใจดี" || rec.Appointments[0].Department != "สำนักงาน" {
		t.Fatalf("first appointment = %+v", rec.Appointments[0])
	}
	if rec.Appointments[1].FullName != "" || rec.Appointments[1].Position != "เลขานุการ" || rec.Appointments[1].NameMatchMethod != "EX_OFFICIO" {
		t.Fatalf("ex-officio appointment = %+v", rec.Appointments[1])
	}
	if got := personLabel(rec.Appointments[1]); got != "เลขานุการ (โดยตำแหน่ง)" {
		t.Fatalf("personLabel = %q", got)
	}
}

func TestBuildExtractionRecordsUnparsedDateKeepsRawText(t *testing.T) {
	rec := buildExtractionRecords(sampleResult(), 0.8)
	var found bool
	for _, f := range rec.Fields {
		if strings.Contains(f.Value, "ไม่ใช่วันที่") {
			found = true
		}
	}
	if !found {
		t.Fatal("unparsed effective date should be kept as raw text in a field row")
	}
	if rec.Updates["effective_date"] != nil {
		t.Fatalf("effective_date column = %v, want NULL", rec.Updates["effective_date"])
	}
}

func TestBuildExtractionRecordsCommitteeFromIssuer(t *testing.T) {
	rec := buildExtractionRecords(sampleResult(), 0.8)
	if rec.Updates["committee"] != "คณะกรรมการบริหาร" || rec.Updates["additional_refs"] != "ที่ ศธ 0001/2569" {
		t.Fatalf("updates = %v", rec.Updates)
	}
}

func TestBuildExtractionRecordsCreatesOnePageRowPerResultPage(t *testing.T) {
	r := sampleResult()
	r.Pages = append(r.Pages, ExtractedPage{Page: 2}) // ไม่รู้ขนาด
	rec := buildExtractionRecords(r, 0.8)
	if len(rec.OCRPages) != 2 {
		t.Fatalf("OCRPages = %d, want 2", len(rec.OCRPages))
	}
	if rec.OCRPages[1].ImageWidth != 1 || rec.OCRPages[1].ImageHeight != 1 {
		t.Fatalf("unknown page size should default to 1x1, got %+v", rec.OCRPages[1])
	}
}

func TestBuildExtractionRecordsKeepsOCRLinesAndText(t *testing.T) {
	r := sampleResult()
	r.Pages[0].Lines = []ExtractedValue{{Text: "บรรทัด 1"}, {Text: "บรรทัด 2"}}
	rec := buildExtractionRecords(r, 0.8)
	if rec.Updates["ocr_text"] != "บรรทัด 1\nบรรทัด 2" {
		t.Fatalf("ocr_text = %v", rec.Updates["ocr_text"])
	}
	if len(rec.OCRLines) != 2 || rec.OCRLines[1].PageNo != 1 || rec.OCRLines[1].LineIndex != 1 {
		t.Fatalf("OCRLines = %+v", rec.OCRLines)
	}
	if rec.OCRPages[0].RawText != "บรรทัด 1\nบรรทัด 2" {
		t.Fatalf("page raw text = %q", rec.OCRPages[0].RawText)
	}
}

func TestBuildExtractionRecordsEmptyOCRTextWithoutLines(t *testing.T) {
	rec := buildExtractionRecords(sampleResult(), 0.8)
	if rec.Updates["ocr_text"] != "" {
		t.Fatalf("ocr_text = %v, want empty string", rec.Updates["ocr_text"])
	}
}

func TestBuildExtractionRecordsTruncatesOverlongValues(t *testing.T) {
	r := sampleResult()
	long := strings.Repeat("ก", 600)
	r.Groups[0].Members[0].Position.Text = long
	r.Header.Subject.Text = long
	rec := buildExtractionRecords(r, 0.8)

	if got := rec.Appointments[0].Position; utf8.RuneCountInString(got) != 255 {
		t.Fatalf("Position runes = %d, want 255", utf8.RuneCountInString(got))
	}
	if got := rec.Updates["title"].(string); utf8.RuneCountInString(got) != 500 {
		t.Fatalf("title runes = %d, want 500", utf8.RuneCountInString(got))
	}
}

func TestTruncateRunesDoesNotSplitThaiCharacters(t *testing.T) {
	if got := truncateRunes("กขค", 2); got != "กข" {
		t.Fatalf("truncateRunes = %q", got)
	}
	if got := truncateRunes("abc", 10); got != "abc" {
		t.Fatalf("short string changed: %q", got)
	}
}
```

- [ ] **Step 2: ต่อท้ายเทสต์ใน `backend/directory_matching_test.go` (ไฟล์มีอยู่แล้ว ให้ใช้ `Edit` หรือ append ต่อท้าย ไม่สร้างใหม่)**

```go
func TestConfirmBlockersLabelsExOfficioRows(t *testing.T) {
	blockers := confirmBlockers([]Appointment{
		{FullName: "", Position: "เลขานุการ"},
		{FullName: "สมชาย", Position: "ผอ."},
	})
	if len(blockers) != 2 || blockers[0] != "เลขานุการ (โดยตำแหน่ง)" || blockers[1] != "สมชาย" {
		t.Fatalf("blockers = %v", blockers)
	}
}

func TestMatchAppointmentsSkipsBlankNames(t *testing.T) {
	apps := []Appointment{{FullName: "", Position: "เลขานุการ"}}
	weak := matchAppointments(apps, nil, nil)
	if weak || apps[0].DirectoryUserID != nil || apps[0].NameMatchMethod != "" {
		t.Fatalf("weak=%t apps=%+v", weak, apps[0])
	}
}
```

- [ ] **Step 3: รันเทสต์ ต้อง FAIL**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && go test ./... -run 'BuildExtractionRecords|TruncateRunes|ConfirmBlockersLabels|MatchAppointmentsSkips' 2>&1 | tail -15
```

คาดหวัง: build fail ด้วย `undefined: buildExtractionRecords`, `personLabel`, `matchAppointments`

- [ ] **Step 4: implement**

1. สร้าง `backend/extraction_persist.go` (ไม่ประกาศ `truncateRunes` ซ้ำ ใช้ตัวใน `document_store.go`):

```go
package main

import (
	"fmt"
	"strings"
)

// ขนาดคอลัมน์ตาม struct ใน main.go (Document, Appointment)
const (
	colLimitAppointmentPosition = 255
	colLimitAppointmentRole     = 255
	colLimitAppointmentFullName = 500
	colLimitAppointmentDept     = 500
	colLimitDocumentOrderNo     = 100
	colLimitDocumentTitle       = 500
	colLimitDocumentCommittee   = 500
	colLimitDocumentSignerName  = 500
)

type extractionRecords struct {
	Fields            []ExtractedField
	Appointments      []Appointment
	OCRPages          []OCRPage
	OCRLines          []OCRLine
	References        []DocumentReference
	Updates           map[string]any
	AverageConfidence float32
	MissingConfidence bool
	LowConfidence     bool
}

// personLabel ใช้แสดงแถวในรายการ blocker และหน้าการแสดงชื่อ
func personLabel(a Appointment) string {
	if strings.TrimSpace(a.FullName) != "" {
		return a.FullName
	}
	return a.Position + " (โดยตำแหน่ง)"
}

// buildExtractionRecords แปลง ExtractionResult เป็นแถวที่จะเขียน ไม่แตะ DB
func buildExtractionRecords(result ExtractionResult, threshold float64) extractionRecords {
	rec := extractionRecords{Updates: map[string]any{}}
	var sum float32
	var count int

	// track นับเฉพาะค่าที่ไม่ว่างใน Header และ Members (spec บรรทัด 138)
	track := func(v ExtractedValue) {
		if strings.TrimSpace(v.Text) == "" {
			return
		}
		if v.Confidence == nil {
			rec.MissingConfidence = true
			return
		}
		sum += *v.Confidence
		count++
		if float64(*v.Confidence) < threshold {
			rec.LowConfidence = true
		}
	}

	addField := func(key, label string, v ExtractedValue, pageNo int) {
		row := ExtractedField{FieldKey: key, FieldLabel: label, Value: v.Text, PageNo: pageNo, BoundingBoxJSON: firstBoxJSON(v.Boxes)}
		if v.Confidence != nil {
			row.Confidence = *v.Confidence
		}
		rec.Fields = append(rec.Fields, row)
	}

	h := result.Header
	for _, item := range []struct {
		key, label string
		v          ExtractedValue
	}{
		{"order_no", "เลขที่คำสั่ง", h.OrderNo},
		{"subject", "เรื่อง", h.Subject},
		{"issued_by", "ออกโดย", h.IssuedBy},
		{"issued_date", "วันที่ออกคำสั่ง", h.Date},
		{"effective_date", "วันที่มีผล", h.EffectiveDate},
		{"expiry_date", "วันที่สิ้นสุด", h.ExpiryDate},
		{"signer_name", "ผู้ลงนาม", h.Signer},
	} {
		addField(item.key, item.label, item.v, pageOf(item.v))
		track(item.v)
	}
	refTexts := make([]string, 0, len(h.References))
	for _, ref := range h.References {
		track(ref)
		if strings.TrimSpace(ref.Text) == "" {
			continue
		}
		refTexts = append(refTexts, ref.Text)
		rec.References = append(rec.References, DocumentReference{ReferenceType: "ADDITIONAL_REFERENCE", ReferenceText: ref.Text, PageNo: pageOf(ref), BoundingBoxJSON: firstBoxJSON(ref.Boxes)})
	}
	addField("additional_references", "เอกสารอ้างอิง", ExtractedValue{Text: strings.Join(refTexts, "\n")}, 0)

	rec.Updates["order_no"] = truncateRunes(h.OrderNo.Text, colLimitDocumentOrderNo)
	if title := truncateRunes(h.Subject.Text, colLimitDocumentTitle); title != "" {
		rec.Updates["title"] = title
	}
	rec.Updates["committee"] = truncateRunes(h.IssuedBy.Text, colLimitDocumentCommittee)
	rec.Updates["signer_name"] = truncateRunes(h.Signer.Text, colLimitDocumentSignerName)
	rec.Updates["additional_refs"] = strings.Join(refTexts, "\n")
	rec.Updates["order_type"] = inferOrderType(h.IssuedBy.Text)
	rec.Updates["order_year_be"] = findOrderYear(h.OrderNo.Text)
	rec.Updates["issued_date"] = parseThaiDateOrNil(h.Date.Text)
	rec.Updates["effective_date"] = parseThaiDateOrNil(h.EffectiveDate.Text)
	rec.Updates["expiry_date"] = parseThaiDateOrNil(h.ExpiryDate.Text)

	for _, g := range result.Groups {
		dept := truncateRunes(g.Department.Text, colLimitAppointmentDept)
		for _, m := range g.Members {
			track(m.Prefix)
			track(m.Name)
			track(m.Position)
			track(m.Role)
			box := m.Name
			if len(box.Boxes) == 0 {
				box = m.Position // กรอบของสมาชิกที่ชื่อว่าง ใช้กรอบของตำแหน่งแทน
			}
			rec.Appointments = append(rec.Appointments, Appointment{
				FullName:         truncateRunes(m.Prefix.Text+m.Name.Text, colLimitAppointmentFullName),
				Position:         truncateRunes(m.Position.Text, colLimitAppointmentPosition),
				CommitteeRole:    truncateRunes(m.Role.Text, colLimitAppointmentRole),
				Department:       dept,
				Responsibilities: g.Duty.Text,
				NameMatchMethod:  matchMethodFor(m.ExOfficio),
				PageNo:           pageOf(box),
				BoundingBoxJSON:  firstBoxJSON(box.Boxes),
			})
		}
	}

	var allLines []string
	for _, p := range result.Pages {
		width, height := p.Width, p.Height
		if width <= 0 || height <= 0 {
			width, height = 1, 1
		}
		pageTexts := make([]string, 0, len(p.Lines))
		for i, line := range p.Lines {
			pageTexts = append(pageTexts, line.Text)
			rec.OCRLines = append(rec.OCRLines, OCRLine{PageNo: p.Page, LineIndex: i, Text: line.Text, BoundingBoxJSON: boxesJSON(line.Boxes)})
		}
		allLines = append(allLines, pageTexts...)
		rec.OCRPages = append(rec.OCRPages, OCRPage{PageNo: p.Page, ImageWidth: width, ImageHeight: height, RawText: strings.Join(pageTexts, "\n"), Status: "COMPLETED"})
	}
	rec.Updates["ocr_text"] = strings.Join(allLines, "\n")

	if count > 0 {
		rec.AverageConfidence = sum / float32(count)
	}
	rec.Updates["confidence"] = rec.AverageConfidence
	rec.Updates["page_count"] = len(result.Pages)
	rec.Updates["person_count"] = len(rec.Appointments)
	return rec
}

// extractionStatus ตาม spec: ค่าที่ไม่มี confidence หรือ needsReview เดิมที่ threshold ให้ NEEDS_REVIEW
func extractionStatus(rec extractionRecords, threshold float64) (string, string) {
	if rec.MissingConfidence || needsReview(rec.AverageConfidence, rec.LowConfidence, threshold) {
		return "NEEDS_REVIEW", "EXTRACTED"
	}
	return "REVIEW", "EXTRACTED"
}

func pageOf(v ExtractedValue) int {
	if len(v.Boxes) > 0 {
		return v.Boxes[0].Page
	}
	return 0
}

func matchMethodFor(exOfficio bool) string {
	if exOfficio {
		return "EX_OFFICIO"
	}
	return ""
}

func parseThaiDateOrNil(text string) any {
	if t := parseThaiDate(text); t != nil {
		return *t
	}
	return nil
}

func boxesJSON(boxes []Box) string {
	parts := make([]string, 0, len(boxes))
	for _, b := range boxes {
		parts = append(parts, fmt.Sprintf(`{"page":%d,"x0":%g,"y0":%g,"x1":%g,"y1":%g}`, b.Page, b.X0, b.Y0, b.X1, b.Y1))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// firstBoxJSON เก็บเฉพาะกรอบแรกตาม spec (ตารางการบันทึก)
func firstBoxJSON(boxes []Box) string {
	if len(boxes) == 0 {
		return "[]"
	}
	return boxesJSON(boxes[:1])
}
```

2. `backend/directory_matching.go`: แทนที่ฟังก์ชัน `applyDirectoryMatches` ด้วยโค้ดต่อไปนี้ และเพิ่ม `matchAppointments` (ใช้ import `strings` ที่มีอยู่แล้ว):

```go
// matchAppointments จับคู่แถวที่มีชื่อกับ directory แถวที่ไม่มีชื่อ (กรรมการโดยตำแหน่ง) ข้ามไป
// คืน true ถ้ามีแถวที่มีชื่อแต่จับคู่ไม่ได้หรือคะแนนต่ำ
func matchAppointments(appointments []Appointment, users []DirectoryUser, aliases map[string]uint) bool {
	weak := false
	for index := range appointments {
		appointment := &appointments[index]
		if strings.TrimSpace(appointment.FullName) == "" {
			continue
		}
		match := matchDirectoryUser(appointment.FullName, users, aliases)
		applyMatchToAppointment(appointment, match)
		if !match.Matched || match.Score < 0.90 || appointment.Confidence < 0.90 {
			weak = true
		}
	}
	return weak
}

func (p *DocumentProcessor) applyDirectoryMatches(metadata *commandMetadata) {
	users, aliases, err := loadDirectory(p.db)
	if err != nil {
		p.logger.Printf("directory matching skipped: %v", err)
		return
	}
	if matchAppointments(metadata.appointments, users, aliases) {
		metadata.lowConfidence = true
	}
}
```

3. `confirmBlockers` ใน `directory_matching.go`: เปลี่ยน `blockers = append(blockers, appointment.FullName)` เป็น `blockers = append(blockers, personLabel(appointment))`

- [ ] **Step 5: รันเทสต์ ต้อง PASS**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && gofmt -w extraction_persist.go && go vet ./... && go test ./... -run 'BuildExtractionRecords|TruncateRunes|ConfirmBlockersLabels|MatchAppointmentsSkips' -v 2>&1 | tail -40 && go test ./... 2>&1 | grep -E "^(--- FAIL|FAIL|ok)"
```

คาดหวัง: เทสต์ของ Task นี้ PASS และการรันเต็มชุดเหลือเฉพาะ baseline `TestNewManualDirectoryUserStoresValidJSONPayload`

- [ ] **Step 6: Commit**

```sh
cd /Users/kit/kku/kku-order-thaidoc && git add backend/extraction_persist.go backend/extraction_persist_test.go backend/directory_matching.go backend/directory_matching_test.go && git commit -m "Map extraction results to document rows with rune truncation

Co-Authored-By: Claude Haiku 5.5 <noreply@anthropic.com>"
```

---

## Task 6: migration 0008, ฟิลด์ใน Document, planExtractionFailure, retryResetUpdates

**Files:**
- Create: `backend/migrations/0008_extraction_retry.sql`
- Modify: `backend/main.go` (type `Document`: เพิ่มฟิลด์ใหม่; เส้นทาง duplicate-overwrite ใน `uploadDocument` merge `retryResetUpdates()` เข้า map `updates`)
- Modify: `backend/api_handlers.go` (`replacedFileUpdates` merge `retryResetUpdates()` เข้าผลลัพธ์)
- Create: `backend/document_retry.go`
- Create: `backend/document_retry_test.go`

**Interfaces:**
- ผลิต: `type failurePlan struct{ Retry bool; NextAttempt time.Time; LastStatus int; Failure ExtractError }`
- ผลิต: `func planExtractionFailure(err error, attempts int, lastStatus int, now time.Time) failurePlan`
- ผลิต: `func retryResetUpdates() map[string]any`
- ผลิต: `const maxExtractionAttempts = 5`
- ใช้ซ้ำใน Task 7 และ 9: `t0` (`var t0 = time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)`) ประกาศในไฟล์เทสต์นี้ไฟล์เดียว

- [ ] **Step 1: เขียน migration `backend/migrations/0008_extraction_retry.sql`**

ตรวจรูปแบบ `0007_app_settings_and_user_profiles.sql` ก่อน (ใช้ statement ต่อหนึ่งบรรทัด) ตัว runner ใน `backend/migrations.go` รองรับเฉพาะรูปแบบ `ALTER TABLE x ADD COLUMN IF NOT EXISTS ...` (regex `addColumnIfMissingPattern`) และ `ALTER TABLE x ADD KEY ...` (regex `addKeyPattern`)

```sql
ALTER TABLE document_workflows ADD COLUMN IF NOT EXISTS extraction_attempts INT NOT NULL DEFAULT 0;
ALTER TABLE document_workflows ADD COLUMN IF NOT EXISTS next_attempt_at DATETIME NULL;
ALTER TABLE document_workflows ADD COLUMN IF NOT EXISTS extraction_last_status INT NOT NULL DEFAULT 0;
ALTER TABLE document_workflows ADD KEY ix_document_workflows_next_attempt (processing_stage, next_attempt_at);
```

- [ ] **Step 2: เขียนเทสต์ที่ล้มเหลว `backend/document_retry_test.go`** (ไฟล์ใหม่)

```go
package main

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)

func retryable(status int) error {
	return &ExtractError{Retryable: true, Status: status, Message: "temp"}
}

func permanent(status int) error {
	return &ExtractError{Retryable: false, Status: status, Message: "perm"}
}

func TestPlanExtractionFailureBackoffTable(t *testing.T) {
	cases := []struct {
		attempts int
		wantWait time.Duration
	}{
		{1, time.Minute},
		{2, 5 * time.Minute},
		{3, 15 * time.Minute},
		{4, 60 * time.Minute},
	}
	for _, c := range cases {
		plan := planExtractionFailure(retryable(503), c.attempts, 0, t0)
		if !plan.Retry {
			t.Fatalf("attempts=%d: want retry", c.attempts)
		}
		if got := plan.NextAttempt.Sub(t0); got != c.wantWait {
			t.Fatalf("attempts=%d: wait = %v, want %v", c.attempts, got, c.wantWait)
		}
		if plan.LastStatus != 503 {
			t.Fatalf("attempts=%d: LastStatus = %d", c.attempts, plan.LastStatus)
		}
	}
}

func TestPlanExtractionFailureStopsAtFifthAttempt(t *testing.T) {
	plan := planExtractionFailure(retryable(503), maxExtractionAttempts, 0, t0)
	if plan.Retry {
		t.Fatal("5th attempt must fail permanently")
	}
	if plan.Failure.Message != "temp" {
		t.Fatalf("Failure = %+v", plan.Failure)
	}
}

func TestPlanExtractionFailurePermanentErrorFailsImmediately(t *testing.T) {
	plan := planExtractionFailure(permanent(400), 1, 0, t0)
	if plan.Retry {
		t.Fatal("400 must not retry")
	}
}

func TestPlanExtractionFailureFirst500RetriesSecondFails(t *testing.T) {
	first := planExtractionFailure(retryable(500), 1, 0, t0)
	if !first.Retry || first.LastStatus != 500 {
		t.Fatalf("first 500: %+v", first)
	}
	second := planExtractionFailure(retryable(500), 2, first.LastStatus, t0)
	if second.Retry {
		t.Fatal("second consecutive 500 must fail")
	}
}

func TestPlanExtractionFailureNon500ResetsStatusMemory(t *testing.T) {
	plan := planExtractionFailure(retryable(503), 2, 500, t0)
	if !plan.Retry {
		t.Fatal("503 after a 500 should still retry")
	}
	next := planExtractionFailure(retryable(500), 3, plan.LastStatus, t0)
	if !next.Retry {
		t.Fatal("first 500 after a 503 should retry")
	}
}

func TestPlanExtractionFailureNonExtractErrorFails(t *testing.T) {
	plan := planExtractionFailure(errors.New("db down"), 1, 0, t0)
	if plan.Retry {
		t.Fatal("unknown error must not retry")
	}
	if plan.Failure.Message == "" {
		t.Fatal("failure message must be set")
	}
}

func TestRetryResetUpdatesClearsAllRetryColumns(t *testing.T) {
	u := retryResetUpdates()
	if u["extraction_attempts"] != 0 || u["extraction_last_status"] != 0 {
		t.Fatalf("updates = %v", u)
	}
	if v, ok := u["next_attempt_at"]; !ok || v != nil {
		t.Fatalf("next_attempt_at = %v (present=%t), want nil", v, ok)
	}
}

// เทสต์นี้เป็นข้อบังคับ: การอัปโหลดทับต้องรีเซ็ตการลองใหม่ (spec ส่วนที่ 3)
func TestReplacedFileUpdatesResetsRetryState(t *testing.T) {
	u := replacedFileUpdates(replacementFile{Filename: "b.pdf"}, "tester", t0)
	if u["extraction_attempts"] != 0 || u["extraction_last_status"] != 0 {
		t.Fatalf("retry counters not reset: %v", u)
	}
	if v, ok := u["next_attempt_at"]; !ok || v != nil {
		t.Fatalf("next_attempt_at = %v (present=%t), want nil", v, ok)
	}
}

// ตรวจว่า statement ใน migration 0008 อยู่ในรูปแบบที่ runner รองรับ (ไม่ต้องใช้ DB)
func TestMigration0008StatementsUseSupportedForms(t *testing.T) {
	raw, err := os.ReadFile("migrations/0008_extraction_retry.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	count := 0
	for _, statement := range strings.Split(string(raw), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		count++
		if !addColumnIfMissingPattern.MatchString(statement) && !addKeyPattern.MatchString(statement) {
			t.Fatalf("statement not supported by migration runner: %s", statement)
		}
	}
	if count != 4 {
		t.Fatalf("statements = %d, want 4", count)
	}
}
```

- [ ] **Step 3: รันเทสต์ ต้อง FAIL**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && go test ./... -run 'PlanExtractionFailure|RetryResetUpdates|ReplacedFileUpdates|Migration0008' 2>&1 | tail -10
```

คาดหวัง: build fail ด้วย `undefined: planExtractionFailure`, `retryResetUpdates`

- [ ] **Step 4: implement**

1. `backend/document_retry.go` (ไฟล์ใหม่):

```go
package main

import (
	"errors"
	"time"
)

const maxExtractionAttempts = 5

var extractionBackoff = []time.Duration{
	time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	60 * time.Minute,
}

type failurePlan struct {
	Retry       bool
	NextAttempt time.Time
	LastStatus  int
	Failure     ExtractError
}

// planExtractionFailure ตัดสินว่าหลัง error หนึ่งครั้งควรลองใหม่หรือล้มถาวร
// attempts คือจำนวนครั้งที่เริ่มเรียก API แล้วรวมครั้งนี้; lastStatus คือ status ของความล้มเหลวก่อนหน้า
func planExtractionFailure(err error, attempts int, lastStatus int, now time.Time) failurePlan {
	var ee *ExtractError
	if !errors.As(err, &ee) {
		return failurePlan{Failure: ExtractError{Message: "ประมวลผลไม่สำเร็จ: " + err.Error()}}
	}
	fail := failurePlan{Failure: *ee, LastStatus: ee.Status}
	if !ee.Retryable || attempts >= maxExtractionAttempts {
		return fail
	}
	if ee.Status == 500 && lastStatus == 500 {
		return fail
	}
	if attempts < 1 || attempts > len(extractionBackoff) {
		return fail
	}
	fail.Retry = true
	fail.NextAttempt = now.Add(extractionBackoff[attempts-1])
	return fail
}

// retryResetUpdates ใช้ร่วมกันทุกเส้นทางที่เริ่มเอกสารใหม่ (reprocess, อัปโหลดทับ, เขียนผลสำเร็จ)
func retryResetUpdates() map[string]any {
	return map[string]any{
		"extraction_attempts":    0,
		"extraction_last_status": 0,
		"next_attempt_at":        nil,
	}
}
```

2. `backend/main.go`: ใน `type Document struct` เพิ่มสามบรรทัดหลังบรรทัด `ProcessingFinishedAt`:

```go
	ExtractionAttempts   int        `json:"extractionAttempts" gorm:"not null;default:0"`
	NextAttemptAt        *time.Time `json:"nextAttemptAt"`
	ExtractionLastStatus int        `json:"-" gorm:"not null;default:0"`
```

ใน `uploadDocument` (เส้นทาง duplicate-overwrite) หลังบล็อก `updates := map[string]any{...}` ให้เพิ่ม:

```go
			for key, value := range retryResetUpdates() {
				updates[key] = value
			}
```

3. `backend/api_handlers.go`: ใน `replacedFileUpdates` เปลี่ยน `return map[string]any{` เป็น `updates := map[string]any{` และก่อน `return` ท้ายฟังก์ชันให้เพิ่ม:

```go
	for key, value := range retryResetUpdates() {
		updates[key] = value
	}
	return updates
```

- [ ] **Step 5: รันเทสต์ ต้อง PASS**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && gofmt -l . ; go vet ./... && go test ./... -run 'PlanExtractionFailure|RetryResetUpdates|ReplacedFileUpdates|Migration0008' -v 2>&1 | tail -30 && go test ./... 2>&1 | grep -E "^(--- FAIL|FAIL|ok)"
```

คาดหวัง: เทสต์ของ Task นี้ PASS (รวม `TestMigration0008StatementsUseSupportedForms` และ `TestReplacedFileUpdatesResetsRetryState` ซึ่งต้องรันจริง ไม่ใช่ข้าม) และการรันเต็มชุดเหลือเฉพาะ baseline

- [ ] **Step 6: Commit**

```sh
cd /Users/kit/kku/kku-order-thaidoc && git add backend/migrations/0008_extraction_retry.sql backend/main.go backend/api_handlers.go backend/document_retry.go backend/document_retry_test.go && git commit -m "Add extraction retry columns and failure planning

Co-Authored-By: Claude Haiku 5.5 <noreply@anthropic.com>"
```

---

## Task 7: คิว, dispatcher, store, cron และการกู้งานตอนเริ่มระบบ

**Files:**
- Create: `backend/document_queue.go`
- Create: `backend/document_queue_test.go`
- Create: `backend/document_queue_store.go`
- Create: `backend/structured_extraction.go` (แปลงผล DOCX/XLSX/CSV เป็น `ExtractionResult`)
- Create: `backend/structured_extraction_test.go`
- Modify: `backend/document_processing.go` (struct `DocumentProcessor` มีฟิลด์ใหม่; `NewDocumentProcessor` รับ `cfg ocrAPIConfig`; **ลบ** `Enqueue`, `Process`, `runOCR`, `persistOCRResult` เดิม; คง `runStructuredImport` เป็น method ของ `DocumentProcessor` ไว้ (ยังเรียก `ocr_worker.py` เดิม))
- Modify: `backend/main.go` (สร้าง `ocrConfig`, เรียก `NewDocumentProcessor(db, logger, ocrConfig)`, `processor.Start`, `processor.Recover`, cron `processor.Sweep`)

**Interfaces:**
- ผลิต: `type extractionJobs interface { Claim(id uint, now time.Time) (Document, bool, error); SaveExtraction(doc *Document, result ExtractionResult, started time.Time) error; SaveStructured(doc *Document, result workerResult, started time.Time) error; MarkRetry(id uint, plan failurePlan) error; MarkFailed(id uint, failure ExtractError, started time.Time) error; DueForQueue(now time.Time, startup bool) ([]uint, error) }`
- ผลิต: `gormExtractionJobs{db, logger, settings SettingsStore, timeout}` implement `extractionJobs`
- ผลิต: ฟิลด์ของ `DocumentProcessor`: `jobs extractionJobs`, `ocrConfig ocrAPIConfig`, `newExtractor func(OCRSettings, ocrAPIConfig) (Extractor, error)`, `pageCount pageCounter`, `importStructured func(ctx context.Context, path, sourceType string) (workerResult, error)`, `now func() time.Time`, `queue chan uint` (buffer 256)
- ผลิต: `func (p *DocumentProcessor) Enqueue(documentID uint)` (ไม่บล็อก; ยังใช้ signature เดิมที่ `api_handlers.go` และ `main.go` เรียก), `Start(ctx context.Context)`, `Sweep()`, `Recover()`, `processOne(ctx context.Context, id uint)`
- ผลิต: `func (p *DocumentProcessor) markExtractionFailed(doc Document, failure ExtractError, started time.Time)` จุดเดียวที่เรียก `jobs.MarkFailed` (รูปแบบ `(doc, err)` ตาม spec + `started` สำหรับระยะเวลา)
- ผลิต: `func staleBefore(now time.Time, timeout time.Duration, startup bool) time.Time`
- ผลิต: `func structuredExtraction(result workerResult) ExtractionResult`, `const structuredImportEngine = "structured-import"`, `func thaiDateText(t *time.Time) string`

- [ ] **Step 1: เขียนเทสต์ที่ล้มเหลว `backend/document_queue_test.go`** (ไฟล์ใหม่)

```go
package main

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"sync"
	"testing"
	"time"
)

type fakeJobs struct {
	mu            sync.Mutex
	claimed       map[uint]int
	doc           Document
	claimOK       bool
	saved         int
	savedStructured int
	saveErr       error
	retries       []failurePlan
	failures      []ExtractError
	dueIDs        []uint
	dueStartup    []bool
}

// Claim คืน ok เฉพาะครั้งแรกของเอกสารเดียวกัน เหมือนเงื่อนไข WHERE ของ store จริง
func (f *fakeJobs) Claim(id uint, now time.Time) (Document, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.claimed == nil {
		f.claimed = map[uint]int{}
	}
	f.claimed[id]++
	if f.claimed[id] > 1 || !f.claimOK {
		return Document{}, false, nil
	}
	return f.doc, true, nil
}

func (f *fakeJobs) SaveExtraction(doc *Document, result ExtractionResult, started time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saved++
	return f.saveErr
}

func (f *fakeJobs) SaveStructured(doc *Document, result workerResult, started time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.savedStructured++
	return f.saveErr
}

func (f *fakeJobs) MarkRetry(id uint, plan failurePlan) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retries = append(f.retries, plan)
	return nil
}

func (f *fakeJobs) MarkFailed(id uint, failure ExtractError, started time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures = append(f.failures, failure)
	return nil
}

func (f *fakeJobs) DueForQueue(now time.Time, startup bool) ([]uint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dueStartup = append(f.dueStartup, startup)
	return f.dueIDs, nil
}

type fakeExtractor struct {
	result ExtractionResult
	err    error
	calls  int
}

func (e *fakeExtractor) Name() string { return "fake" }
func (e *fakeExtractor) Extract(ctx context.Context, path, name string) (ExtractionResult, error) {
	e.calls++
	return e.result, e.err
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newTestProcessor(jobs *fakeJobs, ex Extractor, now time.Time) *DocumentProcessor {
	return &DocumentProcessor{
		logger:       log.New(io.Discard, "", 0),
		settings:     &fakeSettingsStore{settings: defaultOCRSettings()},
		jobs:         jobs,
		ocrConfig:    ocrAPIConfig{Timeout: time.Minute, Concurrency: 1},
		newExtractor: func(OCRSettings, ocrAPIConfig) (Extractor, error) { return ex, nil },
		pageCount:    func(context.Context, string) int { return 1 },
		importStructured: func(context.Context, string, string) (workerResult, error) {
			return workerResult{Pages: []workerPage{{PageNo: 1}}}, nil
		},
		now:   func() time.Time { return now },
		queue: make(chan uint, 8),
	}
}

func pdfDoc(t *testing.T) Document {
	t.Helper()
	path := t.TempDir() + "/a.pdf"
	writeFile(t, path, "%PDF-1.4")
	return Document{ID: 7, SourceType: "PDF", OriginalFilename: "a.pdf", StoragePath: path, ProcessingStage: "EXTRACTING"}
}

func TestProcessOneSavesSuccessOnce(t *testing.T) {
	jobs := &fakeJobs{claimOK: true, doc: pdfDoc(t)}
	ex := &fakeExtractor{result: ExtractionResult{Engine: "fake"}}
	p := newTestProcessor(jobs, ex, t0)

	p.processOne(context.Background(), 7)
	p.processOne(context.Background(), 7) // ส่งซ้ำ แต่ claim ครั้งที่สองต้องคืน ok=false

	if jobs.saved != 1 {
		t.Fatalf("saved = %d, want 1", jobs.saved)
	}
	if ex.calls != 1 {
		t.Fatalf("extractor calls = %d, want 1", ex.calls)
	}
}

func TestProcessOneSkipsWhenClaimLost(t *testing.T) {
	jobs := &fakeJobs{claimOK: false}
	ex := &fakeExtractor{}
	newTestProcessor(jobs, ex, t0).processOne(context.Background(), 7)
	if ex.calls != 0 {
		t.Fatal("extractor must not run when claim is lost")
	}
}

func TestProcessOne503SchedulesRetryOneMinute(t *testing.T) {
	jobs := &fakeJobs{claimOK: true, doc: pdfDoc(t)}
	jobs.doc.ExtractionAttempts = 1
	ex := &fakeExtractor{err: &ExtractError{Retryable: true, Status: 503, Message: "busy"}}
	newTestProcessor(jobs, ex, t0).processOne(context.Background(), 7)

	if len(jobs.retries) != 1 {
		t.Fatalf("retries = %d, want 1", len(jobs.retries))
	}
	if got := jobs.retries[0].NextAttempt; !got.Equal(t0.Add(time.Minute)) {
		t.Fatalf("NextAttempt = %v, want %v", got, t0.Add(time.Minute))
	}
}

func TestProcessOneFifthAttemptFails(t *testing.T) {
	jobs := &fakeJobs{claimOK: true, doc: pdfDoc(t)}
	jobs.doc.ExtractionAttempts = 5
	ex := &fakeExtractor{err: &ExtractError{Retryable: true, Status: 503, Message: "busy"}}
	newTestProcessor(jobs, ex, t0).processOne(context.Background(), 7)

	if len(jobs.retries) != 0 || len(jobs.failures) != 1 {
		t.Fatalf("retries=%d failures=%d", len(jobs.retries), len(jobs.failures))
	}
}

func TestProcessOneSecond500FailsPermanently(t *testing.T) {
	jobs := &fakeJobs{claimOK: true, doc: pdfDoc(t)}
	jobs.doc.ExtractionAttempts = 2
	jobs.doc.ExtractionLastStatus = 500
	ex := &fakeExtractor{err: &ExtractError{Retryable: true, Status: 500, Message: "boom"}}
	newTestProcessor(jobs, ex, t0).processOne(context.Background(), 7)

	if len(jobs.failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(jobs.failures))
	}
}

func TestProcessOneFirst500Retries(t *testing.T) {
	jobs := &fakeJobs{claimOK: true, doc: pdfDoc(t)}
	jobs.doc.ExtractionAttempts = 1
	ex := &fakeExtractor{err: &ExtractError{Retryable: true, Status: 500, Message: "boom"}}
	newTestProcessor(jobs, ex, t0).processOne(context.Background(), 7)

	if len(jobs.retries) != 1 || jobs.retries[0].LastStatus != 500 {
		t.Fatalf("retries = %+v", jobs.retries)
	}
}

func TestProcessOne400FailsImmediately(t *testing.T) {
	jobs := &fakeJobs{claimOK: true, doc: pdfDoc(t)}
	jobs.doc.ExtractionAttempts = 1
	ex := &fakeExtractor{err: &ExtractError{Retryable: false, Status: 400, Message: "bad"}}
	newTestProcessor(jobs, ex, t0).processOne(context.Background(), 7)

	if len(jobs.failures) != 1 || len(jobs.retries) != 0 {
		t.Fatalf("failures=%d retries=%d", len(jobs.failures), len(jobs.retries))
	}
}

func TestProcessOneSaveErrorFailsDocument(t *testing.T) {
	jobs := &fakeJobs{claimOK: true, doc: pdfDoc(t), saveErr: errors.New("db")}
	ex := &fakeExtractor{result: ExtractionResult{Engine: "fake"}}
	newTestProcessor(jobs, ex, t0).processOne(context.Background(), 7)

	if len(jobs.failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(jobs.failures))
	}
}

func TestProcessOneOversizePDFFailsBeforeCallingAPI(t *testing.T) {
	doc := pdfDoc(t)
	if err := os.Truncate(doc.StoragePath, maxPDFBytes+1); err != nil {
		t.Fatal(err)
	}
	jobs := &fakeJobs{claimOK: true, doc: doc}
	ex := &fakeExtractor{}
	newTestProcessor(jobs, ex, t0).processOne(context.Background(), 7)

	if ex.calls != 0 || len(jobs.failures) != 1 || len(jobs.retries) != 0 {
		t.Fatalf("calls=%d failures=%d retries=%d", ex.calls, len(jobs.failures), len(jobs.retries))
	}
}

func TestProcessOneNonPDFUsesStructuredImportWithoutRetry(t *testing.T) {
	doc := pdfDoc(t)
	doc.SourceType = "DOCX"
	jobs := &fakeJobs{claimOK: true, doc: doc}
	ex := &fakeExtractor{}
	p := newTestProcessor(jobs, ex, t0)
	structuredCalls := 0
	p.importStructured = func(context.Context, string, string) (workerResult, error) {
		structuredCalls++
		return workerResult{Pages: []workerPage{{PageNo: 1}}}, nil
	}
	p.processOne(context.Background(), 7)

	if structuredCalls != 1 || jobs.savedStructured != 1 || ex.calls != 0 || len(jobs.retries) != 0 {
		t.Fatalf("structured=%d savedStructured=%d extractorCalls=%d retries=%d", structuredCalls, jobs.savedStructured, ex.calls, len(jobs.retries))
	}
}

func TestSweepEnqueuesDueIDs(t *testing.T) {
	jobs := &fakeJobs{dueIDs: []uint{3, 9}}
	p := newTestProcessor(jobs, &fakeExtractor{}, t0)
	p.Sweep()
	got := []uint{<-p.queue, <-p.queue}
	if got[0] != 3 || got[1] != 9 {
		t.Fatalf("queued = %v", got)
	}
	if jobs.dueStartup[0] {
		t.Fatal("Sweep must call DueForQueue with startup=false")
	}
}

func TestRecoverRequeuesOrphansAtStartup(t *testing.T) {
	jobs := &fakeJobs{dueIDs: []uint{4}}
	p := newTestProcessor(jobs, &fakeExtractor{}, t0)
	p.Recover()
	if got := <-p.queue; got != 4 {
		t.Fatalf("queued = %d, want 4", got)
	}
	if !jobs.dueStartup[0] {
		t.Fatal("Recover must call DueForQueue with startup=true")
	}
}

func TestStaleBeforeUsesTimeoutPlusTwoMinutes(t *testing.T) {
	got := staleBefore(t0, 10*time.Minute, false)
	if !got.Equal(t0.Add(-12 * time.Minute)) {
		t.Fatalf("staleBefore = %v", got)
	}
	if got := staleBefore(t0, 10*time.Minute, true); !got.After(t0) {
		t.Fatalf("startup cutoff should be in the future so every EXTRACTING row is stale, got %v", got)
	}
}
```

- [ ] **Step 2: เขียนเทสต์ที่ล้มเหลว `backend/structured_extraction_test.go`** (ไฟล์ใหม่)

```go
package main

import (
	"testing"
	"time"
)

func TestStructuredExtractionTagsEngineAndKeepsPageSizes(t *testing.T) {
	result := workerResult{Pages: []workerPage{{
		PageNo: 1, ImageWidth: 1654, ImageHeight: 2339, FullText: "คำสั่งแต่งตั้ง",
		Lines: []workerLine{{Text: "คำสั่งแต่งตั้ง", Confidence: 0.9}},
	}}}
	got := structuredExtraction(result)
	if got.Engine != structuredImportEngine {
		t.Fatalf("Engine = %q", got.Engine)
	}
	if len(got.Pages) != 1 || got.Pages[0].Width != 1654 || len(got.Pages[0].Lines) != 1 {
		t.Fatalf("Pages = %+v", got.Pages)
	}
	if got.Header.Subject.Confidence == nil {
		t.Fatal("structured values must carry the page average confidence so the threshold rule applies")
	}
}

func TestThaiDateTextRoundTripsThroughParser(t *testing.T) {
	d := time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	got := parseThaiDate(thaiDateText(&d))
	if got == nil || !got.Equal(d) {
		t.Fatalf("parseThaiDate(%q) = %v, want %v", thaiDateText(&d), got, d)
	}
}
```

- [ ] **Step 3: รันเทสต์ ต้อง FAIL**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && go test ./... -run 'ProcessOne|Sweep|Recover|StaleBefore|StructuredExtraction|ThaiDateText' 2>&1 | tail -10
```

คาดหวัง: build fail ด้วย `undefined` ของ `processOne`, `Sweep`, `Recover`, `staleBefore`, `structuredExtraction`, `thaiDateText`, `structuredImportEngine` และฟิลด์ใหม่ของ `DocumentProcessor`

- [ ] **Step 4: implement**

1. `backend/document_processing.go`:
   - ใน `type DocumentProcessor struct` เพิ่มฟิลด์ (คงฟิลด์เดิม `db`, `logger`, `settings`):

```go
	jobs             extractionJobs
	ocrConfig        ocrAPIConfig
	newExtractor     func(OCRSettings, ocrAPIConfig) (Extractor, error)
	pageCount        pageCounter
	importStructured func(ctx context.Context, path, sourceType string) (workerResult, error)
	now              func() time.Time
	queue            chan uint
```

   - แทนที่ `NewDocumentProcessor` ทั้งฟังก์ชันด้วย:

```go
func NewDocumentProcessor(db *gorm.DB, logger *log.Logger, cfg ocrAPIConfig) *DocumentProcessor {
	settings := NewSettingsStore(db)
	p := &DocumentProcessor{
		db:           db,
		logger:       logger,
		settings:     settings,
		ocrConfig:    cfg,
		newExtractor: newExtractor,
		pageCount:    detectPDFPageCount,
		now:          time.Now,
		queue:        make(chan uint, 256),
	}
	p.jobs = gormExtractionJobs{db: db, logger: logger, settings: settings, timeout: cfg.Timeout}
	p.importStructured = p.runStructuredImport
	return p
}
```

   - ลบ `func (p *DocumentProcessor) Enqueue`, `func (p *DocumentProcessor) Process`, `func (p *DocumentProcessor) runOCR`, `func (p *DocumentProcessor) persistOCRResult` ทั้งฟังก์ชัน (ฟังก์ชันใหม่อยู่ใน `document_queue.go`) และลบ import ที่ไม่ได้ใช้หลังลบ (`go vet` จะบอก)

2. `backend/document_queue.go` (ไฟล์ใหม่):

```go
package main

import (
	"context"
	"errors"
	"strings"
	"time"
)

type extractionJobs interface {
	Claim(id uint, now time.Time) (Document, bool, error)
	SaveExtraction(doc *Document, result ExtractionResult, started time.Time) error
	SaveStructured(doc *Document, result workerResult, started time.Time) error
	MarkRetry(id uint, plan failurePlan) error
	MarkFailed(id uint, failure ExtractError, started time.Time) error
	DueForQueue(now time.Time, startup bool) ([]uint, error)
}

// staleBefore คืนเวลาที่ EXTRACTING/PREPARING เก่ากว่านี้ถือว่าค้าง
// ตอน startup ทุกแถวที่ค้างถือว่าค้าง จึงคืนเวลาในอนาคต
func staleBefore(now time.Time, timeout time.Duration, startup bool) time.Time {
	if startup {
		return now.Add(time.Hour * 24 * 365)
	}
	return now.Add(-(timeout + 2*time.Minute))
}

// Enqueue ส่ง id เข้าคิวแบบไม่บล็อก ถ้าคิวเต็มปล่อยให้ Sweep หยิบภายหลัง
func (p *DocumentProcessor) Enqueue(documentID uint) {
	select {
	case p.queue <- documentID:
	default:
		p.logger.Printf("ocr queue full, document %d will be picked up by sweep", documentID)
	}
}

// Start เปิด worker เท่ากับ ocrConfig.Concurrency และหยุดเมื่อ ctx ยกเลิก
func (p *DocumentProcessor) Start(ctx context.Context) {
	n := p.ocrConfig.Concurrency
	if n < 1 {
		n = 1
	}
	for i := 0; i < n; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case id := <-p.queue:
					p.processOne(ctx, id)
				}
			}
		}()
	}
}

// Sweep ทำงานทุก 1 นาทีจาก cron
func (p *DocumentProcessor) Sweep() {
	ids, err := p.jobs.DueForQueue(p.now(), false)
	if err != nil {
		p.logger.Printf("sweep failed: %v", err)
		return
	}
	for _, id := range ids {
		p.Enqueue(id)
	}
}

// Recover ทำตอนเริ่มระบบ: งานที่ค้างทุกชนิดถูกหยิบกลับเข้าคิว
func (p *DocumentProcessor) Recover() {
	ids, err := p.jobs.DueForQueue(p.now(), true)
	if err != nil {
		p.logger.Printf("recover failed: %v", err)
		return
	}
	for _, id := range ids {
		p.Enqueue(id)
	}
}

func (p *DocumentProcessor) processOne(ctx context.Context, id uint) {
	now := p.now()
	doc, ok, err := p.jobs.Claim(id, now)
	if err != nil {
		p.logger.Printf("claim document %d: %v", id, err)
		return
	}
	if !ok {
		return
	}
	started := now

	if !strings.EqualFold(doc.SourceType, "pdf") {
		result, err := p.importStructured(ctx, doc.StoragePath, doc.SourceType)
		if err != nil {
			p.markExtractionFailed(doc, ExtractError{Message: "อ่านไฟล์ไม่สำเร็จ: " + err.Error()}, started)
			return
		}
		if err := p.jobs.SaveStructured(&doc, result, started); err != nil {
			p.markExtractionFailed(doc, ExtractError{Message: "บันทึกผลการอ่านไม่สำเร็จ"}, started)
		}
		return
	}

	if err := checkPDFLimits(ctx, doc.StoragePath, p.pageCount); err != nil {
		p.markExtractionFailed(doc, asExtractError(err), started)
		return
	}
	settings, err := p.settings.LoadOCR()
	if err != nil {
		p.markExtractionFailed(doc, ExtractError{Message: "อ่านค่าตั้งค่า OCR ไม่ได้"}, started)
		return
	}
	extractor, err := p.newExtractor(settings, p.ocrConfig)
	if err != nil {
		p.markExtractionFailed(doc, asExtractError(err), started)
		return
	}

	result, err := extractor.Extract(ctx, doc.StoragePath, doc.OriginalFilename)
	if err != nil {
		plan := planExtractionFailure(err, doc.ExtractionAttempts, doc.ExtractionLastStatus, p.now())
		if plan.Retry {
			if mErr := p.jobs.MarkRetry(id, plan); mErr != nil {
				p.logger.Printf("mark retry %d: %v", id, mErr)
			}
			return
		}
		p.markExtractionFailed(doc, plan.Failure, started)
		return
	}

	if err := p.jobs.SaveExtraction(&doc, result, started); err != nil {
		p.markExtractionFailed(doc, ExtractError{Message: "บันทึกผลการอ่านไม่สำเร็จ"}, started)
	}
}

// markExtractionFailed เป็นจุดเดียวที่ตั้งสถานะ FAILED (งานอีเมลแจ้งแอดมินจะต่อที่นี่)
func (p *DocumentProcessor) markExtractionFailed(doc Document, failure ExtractError, started time.Time) {
	if err := p.jobs.MarkFailed(doc.ID, failure, started); err != nil {
		p.logger.Printf("mark failed %d: %v", doc.ID, err)
	}
}

func asExtractError(err error) ExtractError {
	var ee *ExtractError
	if errors.As(err, &ee) {
		return *ee
	}
	return ExtractError{Message: err.Error()}
}
```

3. `backend/structured_extraction.go` (ไฟล์ใหม่):

```go
package main

import (
	"fmt"
	"strings"
	"time"
)

const structuredImportEngine = "structured-import"

var thaiMonthNames = [...]string{"มกราคม", "กุมภาพันธ์", "มีนาคม", "เมษายน", "พฤษภาคม", "มิถุนายน", "กรกฎาคม", "สิงหาคม", "กันยายน", "ตุลาคม", "พฤศจิกายน", "ธันวาคม"}

// thaiDateText เขียนวันที่เป็นข้อความไทย (พ.ศ.) ให้ parseThaiDate อ่านกลับได้
func thaiDateText(t *time.Time) string {
	if t == nil {
		return ""
	}
	return fmt.Sprintf("%d %s %d", t.Day(), thaiMonthNames[t.Month()-1], t.Year()+543)
}

// structuredExtraction แปลงผลของ ocr_worker.py (DOCX/XLSX/CSV) เป็น ExtractionResult
// ใช้ extractCommandMetadata แบบไม่มี profile และตั้งความมั่นใจของทุกค่าเป็นค่าเฉลี่ยของเอกสาร
// เพื่อให้กฎ NEEDS_REVIEW ตาม threshold ยังใช้ได้เหมือนเดิม
// เอนจินเดิม committee = ชื่อเรื่อง จึงตั้ง IssuedBy = title
func structuredExtraction(result workerResult) ExtractionResult {
	pages := result.Pages
	meta := extractCommandMetadata(pages, joinPageText(pages))
	avg := averageOCRConfidence(pages)
	val := func(text string) ExtractedValue {
		c := avg
		return ExtractedValue{Text: text, Confidence: &c}
	}

	out := ExtractionResult{
		Engine: structuredImportEngine,
		Header: ExtractedHeader{
			OrderNo:       val(meta.orderNo),
			Subject:       val(meta.title),
			IssuedBy:      val(meta.title),
			Date:          val(thaiDateText(meta.issuedDate)),
			EffectiveDate: val(thaiDateText(meta.effectiveDate)),
			ExpiryDate:    val(thaiDateText(meta.expiryDate)),
			Signer:        val(meta.signer),
		},
	}
	if meta.additionalRefs != "" {
		out.Header.References = []ExtractedValue{val(meta.additionalRefs)}
	}
	for _, a := range meta.appointments {
		out.Groups = append(out.Groups, ExtractedGroup{
			Department: val(a.Department),
			Duty:       val(a.Responsibilities),
			Members: []ExtractedMember{{
				Name:      val(a.FullName),
				Position:  val(a.Position),
				Role:      val(a.CommitteeRole),
				ExOfficio: strings.TrimSpace(a.FullName) == "" && strings.TrimSpace(a.Position) != "",
			}},
		})
	}
	for _, p := range pages {
		page := ExtractedPage{Page: p.PageNo, Width: p.ImageWidth, Height: p.ImageHeight}
		for _, line := range p.Lines {
			conf := line.Confidence
			page.Lines = append(page.Lines, ExtractedValue{Text: line.Text, Confidence: &conf})
		}
		out.Pages = append(out.Pages, page)
	}
	return out
}

func joinPageText(pages []workerPage) string {
	texts := make([]string, 0, len(pages))
	for _, p := range pages {
		texts = append(texts, p.FullText)
	}
	return strings.TrimSpace(strings.Join(texts, "\n"))
}
```

4. `backend/document_queue_store.go` (ไฟล์ใหม่). ชื่อคอลัมน์ทั้งหมดตรงกับ struct จริงใน `main.go`; `AuditLog` ใช้ `DocumentID *uint`, `UserID`, `Action`, `Details`, `CreatedAt`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
)

type gormExtractionJobs struct {
	db       *gorm.DB
	logger   *log.Logger
	settings SettingsStore
	timeout  time.Duration
}

// Claim เปลี่ยนเป็น EXTRACTING แบบมีเงื่อนไข ถ้าไม่มีแถวตรงเงื่อนไขคืน ok=false (กันการส่งซ้ำ)
func (g gormExtractionJobs) Claim(id uint, now time.Time) (Document, bool, error) {
	res := g.db.Model(&Document{}).
		Where("id = ? AND processing_stage IN ?", id, []string{"QUEUED", "RETRY_WAIT"}).
		Updates(map[string]any{
			"processing_stage":      "EXTRACTING",
			"status":                "PROCESSING",
			"extraction_attempts":   gorm.Expr("extraction_attempts + 1"),
			"processing_started_at": now,
			"next_attempt_at":       nil,
		})
	if res.Error != nil {
		return Document{}, false, res.Error
	}
	if res.RowsAffected == 0 {
		return Document{}, false, nil
	}
	var doc Document
	if err := g.db.First(&doc, id).Error; err != nil {
		return Document{}, false, err
	}
	return doc, true, nil
}

// MarkRetry ตาม spec: RETRY_WAIT, next_attempt_at, extraction_last_status, ocr_error
func (g gormExtractionJobs) MarkRetry(id uint, plan failurePlan) error {
	return g.db.Model(&Document{}).Where("id = ?", id).Updates(map[string]any{
		"status":                 "PROCESSING",
		"processing_stage":       "RETRY_WAIT",
		"next_attempt_at":        plan.NextAttempt,
		"extraction_last_status": plan.LastStatus,
		"ocr_error":              plan.Failure.Message,
	}).Error
}

// MarkFailed ตั้ง NEEDS_REVIEW/FAILED และเขียน AuditLog ในทรานแซกชันเดียว
func (g gormExtractionJobs) MarkFailed(id uint, failure ExtractError, started time.Time) error {
	finished := time.Now()
	docID := id
	detail, _ := json.Marshal(failure)
	return g.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Document{}).Where("id = ?", id).Updates(map[string]any{
			"status":                 "NEEDS_REVIEW",
			"processing_stage":       "FAILED",
			"next_attempt_at":        nil,
			"extraction_last_status": failure.Status,
			"ocr_error":              failure.Message,
			"processing_finished_at": finished,
			"processing_duration_ms": finished.Sub(started).Milliseconds(),
		}).Error; err != nil {
			return err
		}
		return tx.Create(&AuditLog{DocumentID: &docID, UserID: "system", Action: "DOCUMENT_EXTRACTION_FAILED", Details: string(detail), CreatedAt: finished}).Error
	})
}

// SaveExtraction เขียนผลตาม extractionRecords ในทรานแซกชันเดียว แล้วล้าง retry state
// ลำดับ: Updates ของ document -> ลบแถวเดิม -> สร้าง fields, appointments, references, ocr_pages (แล้ว ocr_lines ที่ผูก ocr_page_id) -> sync core
func (g gormExtractionJobs) SaveExtraction(doc *Document, result ExtractionResult, started time.Time) error {
	threshold := loadOCRSettingsOrDefault(g.settings).ReviewThreshold
	rec := buildExtractionRecords(result, threshold)
	matchRecordAppointments(g.db, rec.Appointments)
	status, stage := extractionStatus(rec, threshold)
	docID := doc.ID

	return g.db.Transaction(func(tx *gorm.DB) error {
		rec.Updates["status"] = status
		rec.Updates["processing_stage"] = stage
		rec.Updates["ocr_provider"] = result.Engine
		rec.Updates["ocr_error"] = ""
		rec.Updates["processing_finished_at"] = time.Now()
		rec.Updates["processing_duration_ms"] = time.Since(started).Milliseconds()
		for key, value := range retryResetUpdates() {
			rec.Updates[key] = value
		}
		if err := tx.Model(&Document{}).Where("id = ?", docID).Updates(rec.Updates).Error; err != nil {
			return err
		}
		for _, model := range []any{&OCRLine{}, &OCRPage{}, &ExtractedField{}, &Appointment{}, &DocumentReference{}} {
			if err := tx.Where("document_id = ?", docID).Delete(model).Error; err != nil {
				return err
			}
		}
		for i := range rec.Fields {
			rec.Fields[i].DocumentID = docID
		}
		for i := range rec.Appointments {
			rec.Appointments[i].DocumentID = docID
		}
		for i := range rec.References {
			rec.References[i].DocumentID = docID
		}
		if len(rec.Fields) > 0 {
			if err := tx.Create(&rec.Fields).Error; err != nil {
				return err
			}
		}
		if len(rec.Appointments) > 0 {
			if err := tx.Create(&rec.Appointments).Error; err != nil {
				return err
			}
		}
		if len(rec.References) > 0 {
			if err := tx.Create(&rec.References).Error; err != nil {
				return err
			}
		}
		pageIDs := make(map[int]uint, len(rec.OCRPages))
		for i := range rec.OCRPages {
			rec.OCRPages[i].DocumentID = docID
			if err := tx.Create(&rec.OCRPages[i]).Error; err != nil {
				return err
			}
			pageIDs[rec.OCRPages[i].PageNo] = rec.OCRPages[i].ID
		}
		for i := range rec.OCRLines {
			rec.OCRLines[i].DocumentID = docID
			rec.OCRLines[i].OCRPageID = pageIDs[rec.OCRLines[i].PageNo]
			if err := tx.Create(&rec.OCRLines[i]).Error; err != nil {
				return err
			}
		}
		var updated Document
		if err := tx.First(&updated, docID).Error; err != nil {
			return err
		}
		if err := syncCoreOrderFromWorkflow(tx, &updated); err != nil {
			return err
		}
		if err := syncCoreCommitteeFromWorkflow(tx, &updated); err != nil {
			return err
		}
		return tx.Create(&AuditLog{DocumentID: &docID, UserID: "system", Action: "DOCUMENT_PROCESSED", Details: fmt.Sprintf("engine=%s pages=%d fields=%d appointments=%d", result.Engine, len(rec.OCRPages), len(rec.Fields), len(rec.Appointments))}).Error
	})
}

// SaveStructured บันทึกผล DOCX/XLSX/CSV ทางเดียวกับ PDF (engine = structured-import)
func (g gormExtractionJobs) SaveStructured(doc *Document, result workerResult, started time.Time) error {
	return g.SaveExtraction(doc, structuredExtraction(result), started)
}

// matchRecordAppointments จับคู่ชื่อกับ directory ก่อนเขียน; โหลด directory ไม่ได้ก็ข้ามการจับคู่
func matchRecordAppointments(db *gorm.DB, appointments []Appointment) {
	users, aliases, err := loadDirectory(db)
	if err != nil {
		return
	}
	matchAppointments(appointments, users, aliases)
}

// DueForQueue ตามกฎ spec: ดึง EXTRACTING/PREPARING ที่ค้างกลับเป็น QUEUED แล้วคืน id ที่ถึงเวลา
func (g gormExtractionJobs) DueForQueue(now time.Time, startup bool) ([]uint, error) {
	cutoff := staleBefore(now, g.timeout, startup)
	if err := g.db.Model(&Document{}).
		Where("processing_stage IN ? AND processing_started_at < ?", []string{"EXTRACTING", "PREPARING"}, cutoff).
		Update("processing_stage", "QUEUED").Error; err != nil {
		return nil, err
	}
	var ids []uint
	queuedBefore := now.Add(-time.Minute)
	query := g.db.Model(&Document{}).Select("id").Where(
		"(processing_stage = ? AND next_attempt_at <= ?) OR (processing_stage = ? AND (updated_at < ? OR ?))",
		"RETRY_WAIT", now, "QUEUED", queuedBefore, startup,
	)
	if err := query.Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}
```

5. `backend/main.go`:
   - ก่อนบรรทัด `processor := NewDocumentProcessor(db, logger)` ให้เขียน `ocrConfig := loadOCRAPIConfigFromEnv()` แล้วเปลี่ยนเป็น `processor := NewDocumentProcessor(db, logger, ocrConfig)`
   - หลัง `processor` ถูกสร้างและก่อน `router.Run` ให้เพิ่ม `processor.Start(context.Background())` และ `processor.Recover()`
   - ใน cron ใกล้ `IMPORT_CRON` เพิ่ม:

```go
	if _, err := scheduler.AddFunc("* * * * *", processor.Sweep); err != nil {
		panic(fmt.Errorf("schedule sweep: %w", err))
	}
```

- [ ] **Step 5: รันเทสต์ ต้อง PASS**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && gofmt -l . ; go vet ./... && go test ./... -run 'ProcessOne|Sweep|Recover|StaleBefore|StructuredExtraction|ThaiDateText' -v 2>&1 | tail -50 && go test ./... 2>&1 | grep -E "^(--- FAIL|FAIL|ok)"
```

คาดหวัง: เทสต์ของ Task นี้ PASS (ไม่ต้องใช้ pdfinfo, python หรือไฟล์นอก `t.TempDir()`) และการรันเต็มชุดเหลือเฉพาะ baseline `TestNewManualDirectoryUserStoresValidJSONPayload` ถ้า `go vet` ฟ้องว่า `DocumentProcessor` ยังมีเมธอดเดิมซ้ำ ให้ลบเมธอดเดิมนั้น

- [ ] **Step 6: Commit**

```sh
cd /Users/kit/kku/kku-order-thaidoc && git add backend/document_queue.go backend/document_queue_test.go backend/document_queue_store.go backend/structured_extraction.go backend/structured_extraction_test.go backend/document_processing.go backend/main.go && git commit -m "Add extraction queue with claim, retry sweep and startup recovery

Co-Authored-By: Claude Haiku 5.5 <noreply@anthropic.com>"
```

---

## Task 8: การตั้งค่า OCR และ endpoint ทดสอบการเชื่อมต่อ

**Files:**
- Modify: `backend/settings.go` (เอนจินใหม่, `Validate` ตรวจ `APIURL`, `mergeOCRSettings` แปลง `one-ocr` และ URL ว่าง, `getOCRSettingsHandler(store, cfg)`, ลบ `testOCRSettingsHandler`, `summarizeOCRTest`, `ocrTestResult`)
- Modify: `backend/settings_test.go` (เปลี่ยนค่า `one-ocr` ที่เทสต์เดิมคาดไว้ และ `settingsRouter`; ต่อท้ายเทสต์ใหม่)
- Create: `backend/ocr_connection.go`
- Create: `backend/ocr_connection_test.go` (มี helper `performRequest` และ `fakeRole` ที่ Task 9 ใช้ซ้ำ)
- Modify: `backend/main.go` (route `POST /api/v1/settings/ocr/test` และ `GET /api/v1/settings/ocr`)

**Interfaces:**
- ผลิต: `type ocrConnectionResult struct { Status string \`json:"status"\`; Detail string \`json:"detail"\` }`
- ผลิต: `func ocrHealthURL(engine, base string) (string, error)` (thaidoc: `{base}/health`; docflow-contract: origin + `/health`)
- ผลิต: `func checkOCRConnection(ctx context.Context, client *http.Client, healthURL, apiKey string) ocrConnectionResult`
- ผลิต: `func testOCRConnectionHandler(load func() (OCRSettings, error), cfg ocrAPIConfig) gin.HandlerFunc`
- ผลิต: `func getOCRSettingsHandler(store SettingsStore, cfg ocrAPIConfig) gin.HandlerFunc` ตอบ `{"data": settings, "ocrApiKeyConfigured": bool}` (แก้ signature นี้ พร้อม caller ใน `main.go` และ `settingsRouter` ใน `settings_test.go`)
- ผลิต helper ใน `ocr_connection_test.go`: `performRequest(t *testing.T, method, route, requestPath string, body io.Reader, handlers ...gin.HandlerFunc) *httptest.ResponseRecorder`, `fakeRole(role string) gin.HandlerFunc`
- ใช้ของเดิม: `OCRSettings.APIURL` (เพิ่มใน Task 4), `ocrBaseURL` (Task 4), `defaultOCRSettings`, `loadOCRSettingsOrDefault`

- [ ] **Step 1: เขียนเทสต์ที่ล้มเหลว `backend/ocr_connection_test.go`** (ไฟล์ใหม่)

```go
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// performRequest ส่ง request หนึ่งครั้งผ่าน gin router ที่ mount handlers บน route (รองรับ :param)
func performRequest(t *testing.T, method, route, requestPath string, body io.Reader, handlers ...gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Handle(method, route, handlers...)
	req := httptest.NewRequest(method, requestPath, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// fakeRole ตั้ง user และ role ใน context เหมือน middleware จริง (ดู auth.go)
func fakeRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(authUserKey, "tester")
		c.Set(authRoleKey, role)
		c.Next()
	}
}

func TestOCRHealthURLPerEngine(t *testing.T) {
	got, err := ocrHealthURL("thaidoc", "https://ocr.example:8443")
	if err != nil || got != "https://ocr.example:8443/health" {
		t.Fatalf("thaidoc = %q, %v", got, err)
	}
	got, err = ocrHealthURL("docflow-contract", "https://ocr.example/extract/v2")
	if err != nil || got != "https://ocr.example/health" {
		t.Fatalf("contract = %q, %v", got, err)
	}
}

func TestCheckOCRConnectionStatuses(t *testing.T) {
	cases := []struct {
		name     string
		handler  http.HandlerFunc
		wantStat string
	}{
		{"ok", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"status":"ok","models_loaded":true}`))
		}, "ok"},
		{"unauthorized", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }, "unauthorized"},
		{"forbidden", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }, "unauthorized"},
		{"models loading", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"status":"ok","models_loaded":false}`))
		}, "unreachable"},
		{"server error", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }, "unreachable"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(c.handler)
			defer srv.Close()
			got := checkOCRConnection(context.Background(), &http.Client{Timeout: time.Second}, srv.URL+"/health", "k")
			if got.Status != c.wantStat || got.Detail == "" {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestCheckOCRConnectionSendsBearerKey(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"models_loaded":true}`))
	}))
	defer srv.Close()
	checkOCRConnection(context.Background(), &http.Client{Timeout: time.Second}, srv.URL, "secret")
	if auth != "Bearer secret" {
		t.Fatalf("Authorization = %q", auth)
	}
}

func TestCheckOCRConnectionUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	u := srv.URL
	srv.Close()
	got := checkOCRConnection(context.Background(), &http.Client{Timeout: time.Second}, u+"/health", "")
	if got.Status != "unreachable" {
		t.Fatalf("status = %q", got.Status)
	}
}

func TestOCRConnectionHandlerUsesUnsavedValuesAndHidesKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models_loaded":true}`))
	}))
	defer srv.Close()

	load := func() (OCRSettings, error) {
		return OCRSettings{Engine: "thaidoc", APIURL: "https://stored.example"}, nil
	}
	cfg := ocrAPIConfig{APIKey: "TOPSECRET-KEY", Timeout: time.Second}
	body := strings.NewReader(`{"engine":"thaidoc","apiUrl":"` + srv.URL + `"}`)
	rec := performRequest(t, http.MethodPost, "/api/v1/settings/ocr/test", "/api/v1/settings/ocr/test", body, testOCRConnectionHandler(load, cfg))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "TOPSECRET-KEY") {
		t.Fatal("response leaked the API key")
	}
	var out ocrConnectionResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Status != "ok" {
		t.Fatalf("out = %+v err=%v", out, err)
	}
}
```

- [ ] **Step 2: แก้และต่อท้าย `backend/settings_test.go`** (ไฟล์มีอยู่แล้ว ใช้ `Edit` ไม่สร้างใหม่)

แก้ค่าที่คาด `one-ocr` (ค่าเริ่มต้นเปลี่ยนเป็น `thaidoc`):
- บรรทัด `if settings.Engine != "one-ocr" || ...` ใน `TestDefaultOCRSettingsReadEnvironment` → `"thaidoc"`
- บรรทัด `body := ...{"engine":"one-ocr",...}` ใน `TestUpdateOCRSettingsSavesValidPayload` → `"engine":"thaidoc"`
- บรรทัด `strings.NewReader(`{"engine":"one-ocr","dpi":123}`)` → `"engine":"thaidoc"`
- ใน `TestGetOCRSettingsFallsBackToDefaultsWhenStoreFails` ค่า `"one-ocr"` → `"thaidoc"`
- ใน `settingsRouter` บรรทัด `router.GET("/api/v1/settings/ocr", getOCRSettingsHandler(store))` → `getOCRSettingsHandler(store, ocrAPIConfig{})`

ต่อท้ายไฟล์:

```go
func TestMergeOCRSettingsMapsLegacyOneOCRWithoutLosingImportSettings(t *testing.T) {
	defaults := defaultOCRSettings()
	defaults.APIURL = "https://default.example"
	stored := `{"engine":"one-ocr","reviewThreshold":0.7,"apiUrl":"https://kept.example","importIntervalMinutes":30}`
	got, err := mergeOCRSettings(defaults, stored)
	if err != nil {
		t.Fatalf("mergeOCRSettings: %v", err)
	}
	if got.Engine != "thaidoc" {
		t.Fatalf("Engine = %q, want thaidoc", got.Engine)
	}
	if got.APIURL != "https://kept.example" || got.ReviewThreshold != 0.7 || got.ImportIntervalMinutes != 30 {
		t.Fatalf("stored values were wiped: %+v", got)
	}
}

func TestMergeOCRSettingsEmptyStoredURLFallsBackToDefault(t *testing.T) {
	defaults := defaultOCRSettings()
	defaults.APIURL = "https://env.example"
	got, err := mergeOCRSettings(defaults, `{"engine":"thaidoc","apiUrl":""}`)
	if err != nil {
		t.Fatal(err)
	}
	if got.APIURL != "https://env.example" {
		t.Fatalf("APIURL = %q", got.APIURL)
	}
}

func TestOCRSettingsValidateChecksAPIURL(t *testing.T) {
	base := defaultOCRSettings()
	base.Engine = "thaidoc"
	for _, bad := range []string{"ftp://x", "https://"} {
		s := base
		s.APIURL = bad
		if err := s.Validate(); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
	for _, good := range []string{"", "https://ocr.example", "http://10.0.0.5:8000"} {
		s := base
		s.APIURL = good
		if err := s.Validate(); err != nil {
			t.Fatalf("%q rejected: %v", good, err)
		}
	}
	s := base
	s.Engine = "one-ocr"
	if err := s.Validate(); err == nil {
		t.Fatal("one-ocr must not be a valid engine after the change")
	}
}

func TestGetOCRSettingsReportsKeyConfiguredWithoutValue(t *testing.T) {
	h := getOCRSettingsHandler(&fakeSettingsStore{settings: defaultOCRSettings()}, ocrAPIConfig{APIKey: "SECRET-XYZ"})
	rec := performRequest(t, http.MethodGet, "/api/v1/settings/ocr", "/api/v1/settings/ocr", nil, h)
	if strings.Contains(rec.Body.String(), "SECRET-XYZ") {
		t.Fatal("GET leaked the key")
	}
	var out struct {
		OCRAPIKeyConfigured bool `json:"ocrApiKeyConfigured"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || !out.OCRAPIKeyConfigured {
		t.Fatalf("body = %s err=%v", rec.Body.String(), err)
	}
}
```

ต้องเพิ่ม import `"net/http"` หรือ `"encoding/json"` ใน `settings_test.go` ถ้ายังไม่มี (ไฟล์นี้มีทั้งสองอยู่แล้ว)

- [ ] **Step 3: รันเทสต์ ต้อง FAIL**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && go test ./... -run 'OCRHealthURL|CheckOCRConnection|OCRConnectionHandler|MergeOCRSettings|OCRSettingsValidate|GetOCRSettings' 2>&1 | tail -20
```

คาดหวัง: build fail ด้วย `undefined: ocrHealthURL`, `checkOCRConnection` (signature เดิมไม่มี) และ `getOCRSettingsHandler` (argument ไม่พอ)

- [ ] **Step 4: implement**

1. `backend/ocr_connection.go` (ไฟล์ใหม่):

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type ocrConnectionResult struct {
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// ocrHealthURL: thaidoc ใช้ {base}/health; docflow-contract ใช้ origin + /health (ข้อที่ต่างจาก spec ข้อ 10)
func ocrHealthURL(engine, base string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid url %q", base)
	}
	if engine == "thaidoc" {
		return strings.TrimRight(strings.TrimSpace(base), "/") + "/health", nil
	}
	return u.Scheme + "://" + u.Host + "/health", nil
}

func checkOCRConnection(ctx context.Context, client *http.Client, healthURL, apiKey string) ocrConnectionResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return ocrConnectionResult{Status: "unreachable", Detail: "URL ไม่ถูกต้อง"}
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return ocrConnectionResult{Status: "unreachable", Detail: "เชื่อมต่อไม่ได้: " + err.Error()}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ocrConnectionResult{Status: "unauthorized", Detail: "API key ไม่ถูกต้อง หรือไม่มีสิทธิ์"}
	case resp.StatusCode/100 != 2:
		return ocrConnectionResult{Status: "unreachable", Detail: fmt.Sprintf("OCR API ตอบสถานะ %d", resp.StatusCode)}
	}
	var health struct {
		ModelsLoaded *bool `json:"models_loaded"`
	}
	_ = json.Unmarshal(raw, &health)
	if health.ModelsLoaded != nil && !*health.ModelsLoaded {
		return ocrConnectionResult{Status: "unreachable", Detail: "OCR API กำลังโหลดโมเดล ลองใหม่อีกสักครู่"}
	}
	return ocrConnectionResult{Status: "ok", Detail: "เชื่อมต่อได้และพร้อมใช้งาน"}
}

type ocrConnectionRequest struct {
	Engine string `json:"engine"`
	APIURL string `json:"apiUrl"`
}

// testOCRConnectionHandler ใช้ค่าที่ส่งมาถ้ามี ไม่เช่นนั้นใช้ค่าที่บันทึก; key มาจาก env/ไฟล์เท่านั้น ไม่ส่งกลับ
func testOCRConnectionHandler(load func() (OCRSettings, error), cfg ocrAPIConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings, err := load()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "อ่านค่าตั้งค่าไม่ได้"})
			return
		}
		var req ocrConnectionRequest
		if c.Request.ContentLength > 0 {
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "รูปแบบคำขอไม่ถูกต้อง"})
				return
			}
		}
		if req.Engine != "" {
			settings.Engine = req.Engine
		}
		if req.APIURL != "" {
			settings.APIURL = req.APIURL
		}
		healthURL, err := ocrHealthURL(settings.Engine, ocrBaseURL(settings.APIURL, cfg.DefaultURL))
		if err != nil {
			c.JSON(http.StatusOK, ocrConnectionResult{Status: "unreachable", Detail: "ยังไม่ได้ตั้งค่า URL ของ OCR API"})
			return
		}
		client := &http.Client{Timeout: 10 * time.Second}
		c.JSON(http.StatusOK, checkOCRConnection(c.Request.Context(), client, healthURL, cfg.APIKey))
	}
}
```

2. `backend/settings.go`:
   - `allowedOCREngines = map[string]bool{"one-ocr": true}` → `map[string]bool{"thaidoc": true, "docflow-contract": true}`
   - ใน `defaultOCRSettings()` เปลี่ยน `Engine: "one-ocr"` เป็น `Engine: "thaidoc"`
   - ใน `Validate()` ก่อน `return nil` สุดท้าย (หลังบล็อก `DriveFolderURL`) เพิ่ม:

```go
	if s.APIURL != "" {
		parsed, err := url.Parse(s.APIURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("URL ของ OCR API ต้องขึ้นต้นด้วย http:// หรือ https:// และมีชื่อโฮสต์")
		}
	}
```

   - แทนที่ `mergeOCRSettings` ทั้งฟังก์ชันด้วย:

```go
// mergeOCRSettings overlays stored JSON on the defaults. A broken or invalid stored value must
// never stop document processing, so it returns the defaults. The retired "one-ocr" engine maps to
// the default engine, and an empty stored apiUrl falls back to the default (env OCR_API_URL).
func mergeOCRSettings(defaults OCRSettings, stored string) (OCRSettings, error) {
	merged := defaults
	if strings.TrimSpace(stored) == "" {
		return defaults, nil
	}
	if err := json.Unmarshal([]byte(stored), &merged); err != nil {
		return defaults, fmt.Errorf("อ่านค่าตั้งค่า OCR: %w", err)
	}
	if merged.Engine == "one-ocr" {
		merged.Engine = defaults.Engine
	}
	if strings.TrimSpace(merged.APIURL) == "" {
		merged.APIURL = defaults.APIURL
	}
	if err := merged.Validate(); err != nil {
		return defaults, err
	}
	return merged, nil
}
```

   - แทนที่ `getOCRSettingsHandler` ด้วย:

```go
func getOCRSettingsHandler(store SettingsStore, cfg ocrAPIConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": loadOCRSettingsOrDefault(store), "ocrApiKeyConfigured": cfg.APIKey != ""})
	}
}
```

   - ลบ `type ocrTestResult struct` (บรรทัดประมาณ 288), `func testOCRSettingsHandler()` และ `func summarizeOCRTest(...)` ทั้งฟังก์ชัน; คง `hostOnly` และ `runOCRWorker` ไว้ (Task 11 ลบ)

3. `backend/main.go`:
   - `router.GET("/api/v1/settings/ocr", getOCRSettingsHandler(settingsStore))` → `getOCRSettingsHandler(settingsStore, ocrConfig)`
   - `router.POST("/api/v1/settings/ocr/test", requireRole("ADMIN"), testOCRSettingsHandler())` → `router.POST("/api/v1/settings/ocr/test", requireRole("ADMIN"), testOCRConnectionHandler(settingsStore.LoadOCR, ocrConfig))`

- [ ] **Step 5: รันเทสต์ ต้อง PASS**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && gofmt -l . ; go vet ./... && go test ./... -run 'OCRHealthURL|CheckOCRConnection|OCRConnectionHandler|MergeOCRSettings|OCRSettingsValidate|GetOCRSettings|UpdateOCRSettings|DefaultOCRSettings' -v 2>&1 | tail -60 && go test ./... 2>&1 | grep -E "^(--- FAIL|FAIL|ok)"
```

คาดหวัง: เทสต์ของ Task นี้ PASS และการรันเต็มชุดเหลือเฉพาะ baseline ถ้า `go vet` ฟ้อง `settings_test.go` ที่ยังมี `summarizeOCRTest` หรือ `testOCRSettingsHandler` ให้ลบเทสต์นั้น

- [ ] **Step 6: Commit**

```sh
cd /Users/kit/kku/kku-order-thaidoc && git add backend/settings.go backend/settings_test.go backend/ocr_connection.go backend/ocr_connection_test.go backend/main.go && git commit -m "Map legacy one-ocr settings and add OCR connection test

Co-Authored-By: Claude Haiku 5.5 <noreply@anthropic.com>"
```

---

## Task 9: ประมวลผลใหม่ (reprocess)

**Files:**
- Create: `backend/reprocess.go`
- Create: `backend/reprocess_test.go`
- Modify: `backend/main.go` (route `POST /api/v1/documents/:id/reprocess`)

**Interfaces:**
- ผลิต: `var errDocumentNotFound = errors.New("document not found")`, `var errReprocessNotAllowed = errors.New("reprocess not allowed")`
- ผลิต: `type reprocessStore interface { RequestReprocess(id uint, actor string, now time.Time) error }`
- ผลิต: `type gormReprocessStore struct{ db *gorm.DB }` implement `reprocessStore`
- ผลิต: `func reprocessDocumentHandler(store reprocessStore, enqueue func(uint)) gin.HandlerFunc`
- ใช้: `currentUserID(c *gin.Context) string` ใน `auth.go` (ไม่ต้องสร้าง helper ชื่อผู้ใช้ใหม่)
- ใช้ helper ของ Task 8: `performRequest`, `fakeRole` (อยู่ใน `ocr_connection_test.go`; ห้ามประกาศซ้ำ)

- [ ] **Step 1: เขียนเทสต์ที่ล้มเหลว `backend/reprocess_test.go`** (ไฟล์ใหม่)

```go
package main

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

type fakeReprocessStore struct {
	err      error
	calledID uint
	actor    string
}

func (f *fakeReprocessStore) RequestReprocess(id uint, actor string, now time.Time) error {
	f.calledID = id
	f.actor = actor
	return f.err
}

func TestReprocessHandlerAcceptedEnqueuesDocument(t *testing.T) {
	store := &fakeReprocessStore{}
	var enqueued []uint
	h := reprocessDocumentHandler(store, func(id uint) { enqueued = append(enqueued, id) })
	rec := performRequest(t, http.MethodPost, "/api/v1/documents/:id/reprocess", "/api/v1/documents/12/reprocess", nil, fakeRole("ADMIN"), h)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(enqueued) != 1 || enqueued[0] != 12 || store.calledID != 12 || store.actor != "tester" {
		t.Fatalf("enqueued=%v store=%+v", enqueued, store)
	}
}

func TestReprocessHandlerStatusMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not found", errDocumentNotFound, http.StatusNotFound},
		{"wrong state", errReprocessNotAllowed, http.StatusConflict},
		{"unexpected", errors.New("db"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			h := reprocessDocumentHandler(&fakeReprocessStore{err: c.err}, func(uint) {
				t.Fatal("must not enqueue on error")
			})
			rec := performRequest(t, http.MethodPost, "/api/v1/documents/:id/reprocess", "/api/v1/documents/3/reprocess", nil, fakeRole("ADMIN"), h)
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d", rec.Code, c.want)
			}
		})
	}
}

func TestReprocessHandlerRejectsBadID(t *testing.T) {
	h := reprocessDocumentHandler(&fakeReprocessStore{}, func(uint) {})
	rec := performRequest(t, http.MethodPost, "/api/v1/documents/:id/reprocess", "/api/v1/documents/abc/reprocess", nil, fakeRole("ADMIN"), h)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestReprocessRouteRejectsViewerAndAllowsStaff(t *testing.T) {
	route := "/api/v1/documents/:id/reprocess"
	handler := func() func(uint) { return func(uint) {} }
	viewer := performRequest(t, http.MethodPost, route, "/api/v1/documents/1/reprocess", nil,
		fakeRole("VIEWER"), requireRole("ADMIN", "STAFF"), reprocessDocumentHandler(&fakeReprocessStore{}, handler()))
	if viewer.Code != http.StatusForbidden {
		t.Fatalf("VIEWER status = %d, want 403", viewer.Code)
	}
	staff := performRequest(t, http.MethodPost, route, "/api/v1/documents/1/reprocess", nil,
		fakeRole("STAFF"), requireRole("ADMIN", "STAFF"), reprocessDocumentHandler(&fakeReprocessStore{}, handler()))
	if staff.Code != http.StatusAccepted {
		t.Fatalf("STAFF status = %d, want 202", staff.Code)
	}
}
```

- [ ] **Step 2: รันเทสต์ ต้อง FAIL**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && go test ./... -run 'Reprocess' 2>&1 | tail -10
```

คาดหวัง: build fail ด้วย `undefined: reprocessDocumentHandler`, `errDocumentNotFound`, `errReprocessNotAllowed`

- [ ] **Step 3: implement `backend/reprocess.go`** (ไฟล์ใหม่)

```go
package main

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var (
	errDocumentNotFound    = errors.New("document not found")
	errReprocessNotAllowed = errors.New("reprocess not allowed")
)

type reprocessStore interface {
	RequestReprocess(id uint, actor string, now time.Time) error
}

type gormReprocessStore struct {
	db *gorm.DB
}

// RequestReprocess รับเฉพาะ FAILED หรือ RETRY_WAIT รีเซ็ตคอลัมน์ลองใหม่ และบันทึก AuditLog
func (s gormReprocessStore) RequestReprocess(id uint, actor string, now time.Time) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var doc Document
		if err := tx.Select("id", "processing_stage").First(&doc, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errDocumentNotFound
			}
			return err
		}
		if doc.ProcessingStage != "FAILED" && doc.ProcessingStage != "RETRY_WAIT" {
			return errReprocessNotAllowed
		}
		updates := map[string]any{
			"status":                "PROCESSING",
			"processing_stage":      "QUEUED",
			"ocr_error":             "",
			"processing_started_at": nil,
		}
		for key, value := range retryResetUpdates() {
			updates[key] = value
		}
		if err := tx.Model(&Document{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		docID := id
		return tx.Create(&AuditLog{DocumentID: &docID, UserID: actor, Action: "DOCUMENT_REPROCESS_REQUESTED", CreatedAt: now}).Error
	})
}

func reprocessDocumentHandler(store reprocessStore, enqueue func(uint)) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "รหัสเอกสารไม่ถูกต้อง"})
			return
		}
		if err := store.RequestReprocess(uint(id), currentUserID(c), time.Now()); err != nil {
			switch {
			case errors.Is(err, errDocumentNotFound):
				c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบเอกสาร"})
			case errors.Is(err, errReprocessNotAllowed):
				c.JSON(http.StatusConflict, gin.H{"error": "ประมวลผลใหม่ได้เฉพาะเอกสารที่ล้มเหลวหรือรอลองใหม่"})
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"error": "ประมวลผลใหม่ไม่สำเร็จ"})
			}
			return
		}
		enqueue(uint(id))
		c.JSON(http.StatusAccepted, gin.H{"id": id, "processingStage": "QUEUED"})
	}
}
```

- [ ] **Step 4: เพิ่ม route ใน `backend/main.go`** วางใกล้ `POST /api/v1/documents/:id/file`:

```go
	router.POST("/api/v1/documents/:id/reprocess", requireRole("ADMIN", "STAFF"), reprocessDocumentHandler(gormReprocessStore{db: db}, processor.Enqueue))
```

- [ ] **Step 5: รันเทสต์ ต้อง PASS**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && gofmt -l . ; go vet ./... && go test ./... -run 'Reprocess' -v 2>&1 | tail -30 && go test ./... 2>&1 | grep -E "^(--- FAIL|FAIL|ok)"
```

คาดหวัง: เทสต์ของ Task นี้ PASS และการรันเต็มชุดเหลือเฉพาะ baseline `gormReprocessStore` ไม่มีเทสต์ unit (ไม่มี test DB) ความถูกต้องของ SQL ตรวจใน Task 12

- [ ] **Step 6: Commit**

```sh
cd /Users/kit/kku/kku-order-thaidoc && git add backend/reprocess.go backend/reprocess_test.go backend/main.go && git commit -m "Add reprocess endpoint for failed and waiting documents

Co-Authored-By: Claude Haiku 5.5 <noreply@anthropic.com>"
```

---

## Task 10: หน้าเว็บ (frontend)

**Files:**
- Create: `src/lib/processing-status.ts`
- Create: `src/lib/processing-status.test.ts`
- Modify: `src/lib/member-review.ts` (เพิ่ม `personLabel`, เพิ่มกรณี `EX_OFFICIO` ใน `matchState`)
- Modify: `src/lib/member-review.test.ts` (**มีไฟล์อยู่แล้ว ต่อท้ายและแก้บรรทัด import เดิม ห้ามสร้างใหม่**)
- Modify: `src/lib/api.ts` (`ApiDocument` เพิ่มสองฟิลด์; `OCRSettings` เพิ่ม `apiUrl`; เพิ่ม `OCRConnectionResult`, `testOCRConnection`, `reprocessDocument`; ลบ `OCRTestResult` และ `testOCRSettings`)
- Modify: `src/app/settings/ocr/page.tsx` (เอนจินสองค่า, ซ่อนช่อง, การ์ดการเชื่อมต่อ, คำเตือน https; ลบการ์ดทดสอบด้วยไฟล์ตัวอย่างและ state ที่เกี่ยวข้อง)
- Modify: `src/components/document-workspace.tsx` (แสดงหมายเหตุ, ปุ่มประมวลผลใหม่, ป้ายกรรมการโดยตำแหน่งในรายการ blocker)
- Modify: `src/app/documents/page.tsx` (คอลัมน์ความมั่นใจใช้ `confidenceLabel` และแสดง `processingNote`)

**Interfaces:**
- ผลิต (`processing-status.ts`): `type ProcessingDoc = { status: string; processingStage?: string; extractionAttempts?: number; nextAttemptAt?: string | null; ocrError?: string; confidence: number }`, `processingNote(doc: ProcessingDoc): string | null`, `canReprocess(doc: ProcessingDoc): boolean`, `confidenceLabel(doc: ProcessingDoc): string`, `isInsecureURL(url: string): boolean`
- ผลิต (`member-review.ts`): `personLabel(row: { fullName: string; position: string }): string` (ไม่อยู่ใน `processing-status.ts`)
- ผลิต (`api.ts`): `OCRConnectionResult`, `testOCRConnection(body: { engine?: string; apiUrl?: string })`, `reprocessDocument(id: number)`
- ใช้ของเดิม: `apiFetch<T>(path, init)` ใน `api.ts` (ไม่มี helper ชื่อ `request`), `ApiDocument` (ไม่มี type ชื่อ `Document`), `useMe()` จาก `@/components/me-context`, `can(role, key)` จาก `@/lib/permissions`, สเกล `confidence` 0–1 (`document-workspace.tsx:314` ใช้ `× 100`)

- [ ] **Step 1: เขียนเทสต์ที่ล้มเหลว `src/lib/processing-status.test.ts`** (ไฟล์ใหม่)

```ts
import { test } from "node:test";
import assert from "node:assert/strict";
import { canReprocess, confidenceLabel, isInsecureURL, processingNote } from "./processing-status.ts";

const base = { status: "PROCESSING", confidence: 0 };

test("RETRY_WAIT shows the upcoming attempt number and Bangkok time", () => {
  const note = processingNote({
    ...base,
    processingStage: "RETRY_WAIT",
    extractionAttempts: 1,
    nextAttemptAt: "2026-10-11T02:30:00Z",
  });
  assert.equal(note, "รอลองใหม่ ครั้งที่ 2 เวลา 09:30");
});

test("FAILED shows failure note", () => {
  assert.equal(processingNote({ status: "NEEDS_REVIEW", processingStage: "FAILED", confidence: 0 }), "ประมวลผลไม่สำเร็จ");
});

test("normal documents have no note", () => {
  assert.equal(processingNote({ status: "REVIEW", processingStage: "EXTRACTED", confidence: 0.9 }), null);
});

test("reprocess allowed only for FAILED or RETRY_WAIT", () => {
  assert.equal(canReprocess({ status: "NEEDS_REVIEW", processingStage: "FAILED", confidence: 0 }), true);
  assert.equal(canReprocess({ status: "PROCESSING", processingStage: "RETRY_WAIT", confidence: 0 }), true);
  assert.equal(canReprocess({ status: "REVIEW", processingStage: "EXTRACTED", confidence: 0.9 }), false);
  assert.equal(canReprocess({ status: "PROCESSING", processingStage: "EXTRACTING", confidence: 0 }), false);
});

test("confidence 0 is not shown as calculating", () => {
  assert.equal(confidenceLabel({ status: "NEEDS_REVIEW", processingStage: "EXTRACTED", confidence: 0 }), "ไม่ระบุ");
});

test("processing documents show calculating label", () => {
  assert.equal(confidenceLabel({ status: "PROCESSING", processingStage: "EXTRACTING", confidence: 0 }), "กำลังคำนวณ");
});

test("known confidence is shown as percent (confidence is 0-1)", () => {
  assert.equal(confidenceLabel({ status: "REVIEW", processingStage: "EXTRACTED", confidence: 0.87 }), "87%");
});

test("http url is flagged as insecure", () => {
  assert.equal(isInsecureURL("http://10.0.0.5:8000"), true);
  assert.equal(isInsecureURL("https://ocr.example"), false);
  assert.equal(isInsecureURL(""), false);
});
```

- [ ] **Step 2: แก้ `src/lib/member-review.test.ts`** (ไฟล์มีอยู่แล้ว)

- แก้บรรทัด import เดิม `import { countByFilter, filterMembers, groupByDepartment, matchState, rosterRows, summarizeDepartments } from "./member-review.ts";` ให้เพิ่ม `personLabel` ในวงเล็บเดียวกัน
- ต่อท้ายไฟล์:

```ts
test("ex-officio row without name is labelled by position", () => {
  assert.equal(personLabel({ fullName: "", position: "เลขานุการ" }), "เลขานุการ (โดยตำแหน่ง)");
  assert.equal(personLabel({ fullName: "สมชาย", position: "ผอ." }), "สมชาย");
});

test("ex-officio row waits for a reviewer to choose the person", () => {
  const row = appointment({ id: 6, fullName: "", position: "เลขานุการ", nameMatchMethod: "EX_OFFICIO" });
  assert.equal(matchState(row).label, "รอผู้ตรวจเลือกบุคคล");
});
```

- [ ] **Step 3: รันเทสต์ ต้อง FAIL**

```sh
cd /Users/kit/kku/kku-order-thaidoc && npm test 2>&1 | tail -20
```

คาดหวัง: FAIL เพราะ `processing-status.ts` และ `personLabel` ยังไม่มี

- [ ] **Step 4: implement `src/lib/processing-status.ts`** (ไฟล์ใหม่)

```ts
export type ProcessingDoc = {
  status: string;
  processingStage?: string;
  extractionAttempts?: number;
  nextAttemptAt?: string | null;
  ocrError?: string;
  confidence: number;
};

const timeFormat = new Intl.DateTimeFormat("en-GB", {
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
  timeZone: "Asia/Bangkok",
});

// n คือครั้งที่จะลองถัดไป (spec: "รอลองใหม่ ครั้งที่ n เวลา hh:mm") จึงบวก 1 จากจำนวนครั้งที่เริ่มแล้ว
export function processingNote(doc: ProcessingDoc): string | null {
  if (doc.processingStage === "RETRY_WAIT") {
    const upcoming = (doc.extractionAttempts ?? 0) + 1;
    const when = doc.nextAttemptAt ? timeFormat.format(new Date(doc.nextAttemptAt)) : "-";
    return `รอลองใหม่ ครั้งที่ ${upcoming} เวลา ${when}`;
  }
  if (doc.processingStage === "FAILED") {
    return "ประมวลผลไม่สำเร็จ";
  }
  return null;
}

export function canReprocess(doc: ProcessingDoc): boolean {
  return doc.processingStage === "FAILED" || doc.processingStage === "RETRY_WAIT";
}

// confidence 0 เมื่อยังประมวลผลอยู่ = กำลังคำนวณ; 0 เมื่อประมวลผลเสร็จแล้ว = ไม่ระบุ (ไม่ค้างที่ "กำลังคำนวณ")
export function confidenceLabel(doc: ProcessingDoc): string {
  if (doc.status === "PROCESSING") return "กำลังคำนวณ";
  if (doc.confidence === 0) return "ไม่ระบุ";
  return `${Math.round(doc.confidence * 100)}%`;
}

export function isInsecureURL(url: string): boolean {
  return url.trim().toLowerCase().startsWith("http://");
}
```

- [ ] **Step 5: `src/lib/member-review.ts`**

- เพิ่มฟังก์ชัน:

```ts
export function personLabel(row: { fullName: string; position: string }): string {
  return row.fullName.trim() !== "" ? row.fullName : `${row.position} (โดยตำแหน่ง)`;
}
```

- ใน `matchState` ภายในบล็อก `if (!isLinked(row)) {` ก่อนบรรทัด fallback `return { label: "ไม่พบในระบบ"...}` ให้เพิ่ม:

```ts
    if (method === "EX_OFFICIO") return { label: "รอผู้ตรวจเลือกบุคคล", detail: "กรรมการโดยตำแหน่งที่ไม่มีชื่อ ต้องให้ผู้ตรวจเลือกบุคคล", tone: "warning" };
```

รัน `npm test` ต้อง PASS

- [ ] **Step 6: `src/lib/api.ts`**

- ใน `export type ApiDocument = {` เพิ่ม `extractionAttempts?: number; nextAttemptAt?: string | null;` (มี `processingStage`, `ocrError`, `confidence` อยู่แล้ว)
- ใน `export type OCRSettings = {` เพิ่ม `apiUrl: string;`
- `getOCRSettings` ให้มี type ตอบเป็น `apiFetch<{ data: OCRSettings; ocrApiKeyConfigured: boolean }>("/api/v1/settings/ocr")`
- ลบ `export type OCRTestResult = ...` (บรรทัด ~365) และ `export async function testOCRSettings(file: File, settings: OCRSettings)` ทั้งฟังก์ชัน
- เพิ่ม (รูปแบบเดียวกับ `updateOCRSettings`):

```ts
export type OCRConnectionResult = { status: "ok" | "unauthorized" | "unreachable"; detail: string };

export async function testOCRConnection(body: { engine?: string; apiUrl?: string }) {
  return apiFetch<OCRConnectionResult>("/api/v1/settings/ocr/test", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

export async function reprocessDocument(id: number) {
  return apiFetch<{ id: number; processingStage: string }>(`/api/v1/documents/${id}/reprocess`, { method: "POST" });
}
```

- [ ] **Step 7: `src/app/settings/ocr/page.tsx`**

- บรรทัด import `import { OCRSettings, OCRTestResult, getOCRSettings, getSystemSettings, testOCRSettings, updateOCRSettings } from "@/lib/api";` ให้เหลือ `import { OCRSettings, getOCRSettings, getSystemSettings, testOCRConnection, updateOCRSettings } from "@/lib/api";` และเพิ่ม `import { isInsecureURL } from "@/lib/processing-status";`
- รายการเอนจิน: แทน `one-ocr` ด้วย `thaidoc` (ป้าย "thaidoc-extract") และ `docflow-contract` (ป้าย "DocFlow contract") ลบ one-ocr ออก
- ซ่อนแถวภาษา, DPI, ขนาดภาพสูงสุด, preprocess (ไม่ลบ field ใน state หรือ payload ที่ส่ง `updateOCRSettings`)
- คง input `reviewThreshold`
- ลบการ์ด "ทดสอบด้วยไฟล์ตัวอย่าง" และ state `sample`, `result` ที่เกี่ยวข้อง
- เพิ่มการ์ด "การเชื่อมต่อ OCR API": input `apiUrl` (ผูกกับ state `form.apiUrl`), แถว "API key: ตั้งค่าแล้ว/ยังไม่ได้ตั้งค่า" จาก `ocrApiKeyConfigured`, คำเตือนสีเหลืองเมื่อ `isInsecureURL(form.apiUrl)` (ข้อความ: "การเชื่อมต่อไม่ได้เข้ารหัส แนะนำให้ใช้ https://"), ปุ่ม "ทดสอบการเชื่อมต่อ" ที่เรียก `testOCRConnection({ engine: form.engine, apiUrl: form.apiUrl })` และแสดง `detail` พร้อมไอคอนตาม `status`
- ตรวจ `getOCRSettings` ที่ใช้ใน page ให้เก็บ `ocrApiKeyConfigured` ลง state

- [ ] **Step 8: `src/components/document-workspace.tsx`**

- เพิ่ม import: `import { useMe } from "@/components/me-context";`, `import { can } from "@/lib/permissions";`, `import { canReprocess, processingNote } from "@/lib/processing-status";`, `import { reprocessDocument } from "@/lib/api";` (รวมกับ import จาก `@/lib/api` เดิม) และ `personLabel` จาก `@/lib/member-review` (รวมกับ import เดิม)
- แสดง `processingNote(doc)` และ `doc.ocrError` ใต้หัวเอกสาร
- ปุ่ม "ประมวลผลใหม่" แสดงเมื่อ `can(role, "import") && canReprocess(doc)` โดย `role` มาจาก `useMe()` (ดูการใช้งานที่ `src/app/settings/ocr/page.tsx`) เมื่อกดให้เรียก `reprocessDocument(doc.id)` แล้วโหลดเอกสารใหม่ด้วยวิธีเดียวกับ flow อัปโหลดทับไฟล์เดิม (`replaceDocumentFile`)
- รายการ blocker ที่แสดงชื่อ ให้ใช้ `personLabel(row)` แทน `fullName` ตรง ๆ (ค้นด้วย `grep -n "fullName" src/components/document-workspace.tsx` แล้วเลือกจุดที่เป็นรายการ blocker)

- [ ] **Step 9: `src/app/documents/page.tsx`**

- ค้น `กำลังคำนวณ` (พบที่บรรทัดตาราง ~43) แล้วแทนการแสดงความมั่นใจด้วย `confidenceLabel(doc)` และแสดง `processingNote(doc)` เป็นข้อความรองใต้สถานะ

- [ ] **Step 10: รันเทสต์และ typecheck ต้อง PASS**

```sh
cd /Users/kit/kku/kku-order-thaidoc && npm test 2>&1 | tail -30 && npx tsc --noEmit 2>&1 | tail -20 && npm run lint 2>&1 | tail -15
```

คาดหวัง: เทสต์ PASS, `tsc` ไม่มี error, lint ไม่มี error ใหม่

- [ ] **Step 11: Commit**

```sh
cd /Users/kit/kku/kku-order-thaidoc && git add src/lib/processing-status.ts src/lib/processing-status.test.ts src/lib/member-review.ts src/lib/member-review.test.ts src/lib/api.ts src/app/settings/ocr/page.tsx src/components/document-workspace.tsx src/app/documents/page.tsx && git commit -m "Show retry state, reprocess button and OCR connection test in the UI

Co-Authored-By: Claude Haiku 5.5 <noreply@anthropic.com>"
```

---

## Task 11: ถอด one-ocr ออกจากเส้นทาง PDF

**Files:**
- Modify: `backend/document_processing.go` (รวม `extractCommandMetadataWithProfile` เป็น `extractCommandMetadata(pages, fullText)` ที่ไม่มี profile; ลบ `extractCommandMetadataForDocument`, `fixedSampleProfile`, `fixedSampleProfileFor`, `fixedSampleRows`; ลบฟังก์ชันที่เหลือไม่มีผู้เรียกหลังแก้)
- Modify: `backend/directory_matching.go` (ลบ `applyDirectoryMatches` เพราะไม่มีผู้เรียกหลัง Task 7)
- Modify: `backend/settings.go` (ลบ `preprocessMode`, `renderArgs`, `workerArgs`, `runOCRWorker`)
- Modify: `backend/settings_test.go` (ลบ `TestOCRSettingsBuildWorkerArguments`)
- Modify: `backend/sync_test.go` (ลบ `TestExtractAppointmentsJoinsFixedSampleRowColumns`, `TestFixedSampleProfileKeepsOnlyTableRegion`)
- Modify: `backend/ocr_worker.py` (ย้าย `import oneocr` และ PIL ไปเฉพาะตอน OCR ภาพ)
- Modify: `backend/Dockerfile` (ลบ `one-ocr` และ `OCR_MODELS_DIR`)
- Modify: `backend/.dockerignore` (เพิ่ม `one-ocr`)

**Interfaces:**
- ไม่มี interface ใหม่ `runStructuredImport` (method ของ `DocumentProcessor`) ไม่ต้องแก้: การแปลงผล DOCX/XLSX/CSV ทำใน `structuredExtraction` (Task 7) ซึ่งเรียก `extractCommandMetadata(pages, fullText)` แบบไม่มี profile
- ผลลัพธ์พฤติกรรม: `extractCommandMetadata` ที่ไม่มี profile ใช้เส้นทาง `groupOCRRows` เดิม (เหมือนที่ไฟล์ที่ไม่ใช่ตัวอย่างใช้อยู่แล้ว)

- [ ] **Step 1: เทสต์พฤติกรรมเดิมเป็นตัวกันถอย (regression guard) ต้อง PASS ก่อนลบ**

ไม่มีเทสต์ใหม่ เพราะเป็นงานลบโค้ดที่พฤติกรรมไม่ควรเปลี่ยน ใช้เทสต์เดิมที่ assert พฤติกรรมจริง ได้แก่ `TestExtractCommandMetadataFromThaiOCR`, `TestExtractCommandMetadataNormalizesThaiDigits`, `TestExtractAppointmentsRebuildsTableRowsWithoutPhantomRoles` (ใน `sync_test.go`) และ `TestStructuredExtractionTagsEngineAndKeepsPageSizes` จาก Task 7

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && go test ./... -run 'ExtractCommandMetadata|ExtractAppointments|StructuredExtraction' -v 2>&1 | grep -E "^(--- |ok|FAIL)"
```

คาดหวัง: ทุกเทสต์ที่ระบุ PASS ก่อนเริ่มลบ และต้อง PASS อีกครั้งหลังลบใน Step 4

- [ ] **Step 2: หาผู้เรียกก่อนลบ**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && grep -rn --include='*.go' -e 'runOCRWorker' -e 'renderArgs' -e 'workerArgs' -e 'preprocessMode' -e 'fixedSampleProfile' -e 'fixedSampleRows' -e 'extractCommandMetadataForDocument' -e 'extractCommandMetadataWithProfile' -e 'applyDirectoryMatches' -e 'groupOCRRows' -e 'parseAppointmentRow' .
```

คาดหวัง: `groupOCRRows` และ `parseAppointmentRow` ยังมีผู้เรียกจาก `extractCommandMetadata` จึง**ไม่ลบ** (ข้อแก้ไขจากรายการเดิมของ spec-แผน); `runOCRWorker` เรียกจาก `settings.go` และ `testOCRSettingsHandler` (ลบแล้วใน Task 8) เท่านั้น

- [ ] **Step 3: ลบและแก้โค้ด**

1. `backend/document_processing.go`:
   - ลบ `func extractCommandMetadataForDocument(...)` ทั้งฟังก์ชัน, `type fixedSampleProfile struct {...}`, `func fixedSampleProfileFor(...)` ทั้งฟังก์ชัน และ `func fixedSampleRows(...)` ทั้งฟังก์ชัน
   - ลบฟังก์ชัน wrapper `func extractCommandMetadata(pages []workerPage, fullText string) commandMetadata { return extractCommandMetadataWithProfile(pages, fullText, nil) }`
   - เปลี่ยนชื่อ `extractCommandMetadataWithProfile(pages []workerPage, fullText string, profile *fixedSampleProfile) commandMetadata` เป็น `extractCommandMetadata(pages []workerPage, fullText string) commandMetadata`
   - ในลูปที่เดิมเรียก `for _, row := range fixedSampleRows(page, profile) {` ให้เปลี่ยนเป็น `for _, row := range groupOCRRows(page) {`
   - หลังแก้ ให้ค้นฟังก์ชันที่ไม่มีผู้เรียกแล้ว (`grep -n` ชื่อนั้นทั้ง `backend`) และลบเฉพาะที่ไม่มีผู้เรียกเลย (คอมไพเลอร์ไม่แจ้งฟังก์ชันที่ไม่ได้ใช้ จึงต้องตรวจเอง)
2. `backend/directory_matching.go`: ลบ `func (p *DocumentProcessor) applyDirectoryMatches(...)` ทั้งฟังก์ชัน (คง `matchAppointments` และ `applyMatchToAppointment`)
3. `backend/settings.go`: ลบ `func (s OCRSettings) preprocessMode() string`, `func (s OCRSettings) renderArgs() []string`, `func (s OCRSettings) workerArgs(...)`, `func runOCRWorker(...)` ทั้งหมด ลบ import ที่ไม่ได้ใช้ (`go vet` จะแจ้ง) ค่าฟิลด์ `Preprocess`, `DPI`, `MaxSide`, `Language` ใน `OCRSettings` คงไว้เพื่อให้ JSON เดิมอ่านได้
4. `backend/settings_test.go`: ลบ `func TestOCRSettingsBuildWorkerArguments(t *testing.T)` ทั้งฟังก์ชัน
5. `backend/sync_test.go`: ลบ `func TestExtractAppointmentsJoinsFixedSampleRowColumns` และ `func TestFixedSampleProfileKeepsOnlyTableRegion` ทั้งสองฟังก์ชัน (ท้ายไฟล์)
6. `backend/ocr_worker.py`: ย้าย `import oneocr` และ `from PIL import ...` ที่อยู่ระดับ module ไปไว้ในฟังก์ชันที่ทำ OCR ภาพเท่านั้น เพื่อให้ `--input-file` สำหรับ DOCX/XLSX/CSV รันได้โดยไม่มี one-ocr หรือ PIL

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && grep -n "^import \|^from " ocr_worker.py
```

7. `backend/Dockerfile`: ลบบรรทัด `COPY one-ocr /app/one-ocr`, `pip install ... -e /app/one-ocr` และ ENV `OCR_MODELS_DIR`; ปรับ `RUN` ให้เหลือ `mkdir -p /app/storage && chown -R app:app /app`
8. `backend/.dockerignore`: เพิ่มบรรทัด `one-ocr` (ไม่ลบโฟลเดอร์ `backend/one-ocr` ออกจากดิสก์)

- [ ] **Step 4: ตรวจ ต้องผ่าน**

```sh
cd /Users/kit/kku/kku-order-thaidoc/backend && gofmt -l . ; go vet ./... && go test ./... -run 'ExtractCommandMetadata|ExtractAppointments|StructuredExtraction' -v 2>&1 | grep -E "^(--- |ok|FAIL)" && go test ./... 2>&1 | grep -E "^(--- FAIL|FAIL|ok)" && python3 -I -c "import ast; ast.parse(open('ocr_worker.py').read()); print('syntax ok')"
```

คาดหวัง: ทุกเทสต์ที่ระบุ PASS, การรันเต็มชุดเหลือเฉพาะ baseline `TestNewManualDirectoryUserStoresValidJSONPayload`, `syntax ok`

- [ ] **Step 5: Commit**

```sh
cd /Users/kit/kku/kku-order-thaidoc && git add backend/document_processing.go backend/directory_matching.go backend/settings.go backend/settings_test.go backend/sync_test.go backend/ocr_worker.py backend/Dockerfile backend/.dockerignore && git commit -m "Remove one-ocr PDF path and heuristics; keep structured import

Co-Authored-By: Claude Haiku 5.5 <noreply@anthropic.com>"
```

---

## Task 12: deploy แอป DocFlow และทดสอบ end-to-end (remote/manual)

งานนี้ทำโดย main session เท่านั้น ห้ามให้ runner หรือ agent อื่นทำ เพราะเกี่ยวข้องกับ VM ที่ใช้งานอยู่

**Files:**
- Modify: `docker-compose.yml` (พอร์ต `${API_HOST_PORT:-8080}:8080`, `${WEB_HOST_PORT:-3000}:3000`, `WEB_ORIGIN: ${WEB_ORIGIN:-http://localhost:3000}`, env `OCR_API_URL`, `OCR_API_KEY`, `OCR_API_KEY_FILE`, `OCR_API_TIMEOUT`, `OCR_API_CONCURRENCY`)
- Modify: `deploy/podman-compose.yml` (env เดียวกัน)
- Modify: `.env.example` (ตัวแปรเดียวกัน พร้อมคำอธิบายภาษาไทย)
- Create: `docker-compose.ocr-net.yml`

**Interfaces:**
- ใช้: network ภายนอกชื่อ `docflow-ocr` ที่ Task 1 สร้าง, alias `ocr-proxy:8000`

- [ ] **Step 1: แก้ `docker-compose.yml` และ `deploy/podman-compose.yml`**

api service:
```yaml
    environment:
      OCR_API_URL: ${OCR_API_URL:-}
      OCR_API_KEY: ${OCR_API_KEY:-}
      OCR_API_KEY_FILE: ${OCR_API_KEY_FILE:-}
      OCR_API_TIMEOUT: ${OCR_API_TIMEOUT:-10m}
      OCR_API_CONCURRENCY: ${OCR_API_CONCURRENCY:-1}
      WEB_ORIGIN: ${WEB_ORIGIN:-http://localhost:3000}
    ports:
      - "${API_HOST_PORT:-8080}:8080"
```
web service: `ports: - "${WEB_HOST_PORT:-3000}:3000"`

- [ ] **Step 2: เขียน `docker-compose.ocr-net.yml`**

```yaml
services:
  api:
    networks:
      - default
      - docflow-ocr

networks:
  docflow-ocr:
    external: true
    name: docflow-ocr
```

- [ ] **Step 3: ตรวจไฟล์ compose**

```sh
cd /Users/kit/kku/kku-order-thaidoc && docker compose -f docker-compose.yml -f docker-compose.ocr-net.yml config >/dev/null; echo "exit=$?"
```

คาดหวัง: exit 0 (ถ้า network `docflow-ocr` ยังไม่มีบนเครื่องนี้ ให้ใช้ `config` ที่ไม่ตรวจการมีอยู่จริง และบันทึกไว้)

- [ ] **Step 4: Commit**

```sh
git add docker-compose.yml deploy/podman-compose.yml .env.example docker-compose.ocr-net.yml && git commit -m "Expose OCR API settings and join the OCR network in compose

Co-Authored-By: Claude Haiku 5.5 <noreply@anthropic.com>"
```

- [ ] **Step 5: ส่งโค้ดขึ้น VM (remote/manual)**

```sh
cd /Users/kit/kku/kku-order-thaidoc && git archive HEAD | ssh project@10.198.200.62 'mkdir -p ~/docflow/kku-order && tar -x -C ~/docflow/kku-order'
```

- [ ] **Step 6: ตั้งค่า `.env` บน VM (remote/manual)**

บน VM ที่ `~/docflow/kku-order/.env` ใส่:

```dotenv
MYSQL_HOST_PORT=127.0.0.1:13307
API_HOST_PORT=127.0.0.1:18080
WEB_HOST_PORT=3000
OCR_API_URL=http://ocr-proxy:8000
OCR_API_KEY=<ค่าเดียวกับ OCR stack>
```

ไม่ต้องใส่ค่า key ลงในแชตหรือ commit

- [ ] **Step 7: เปิด stack (remote/manual)**

```sh
ssh project@10.198.200.62 'cd ~/docflow/kku-order && docker compose -f docker-compose.yml -f docker-compose.ocr-net.yml up -d --build'
```

ตรวจว่า migration 0008 รันสำเร็จจาก log ของ api:

```sh
ssh project@10.198.200.62 'cd ~/docflow/kku-order && docker compose logs api | grep -i "migrat\|error" | tail -20'
```

- [ ] **Step 8: ทดสอบ end-to-end (remote/manual)**

1. เปิดหน้า settings OCR ด้วย ADMIN แล้วกด "ทดสอบการเชื่อมต่อ" คาดหวัง `ok` และคำเตือน "ไม่ได้เข้ารหัส" เพราะ URL เป็น http
2. อัปโหลด `~/kku-order-ocr/docs-samples/kku.pdf` คาดหวังสถานะ `QUEUED` → `EXTRACTING` → `NEEDS_REVIEW` และมีแถว appointment
3. ตรวจ DB ว่า `extraction_attempts = 1` และ `ocr_provider = thaidoc`
4. หยุด proxy ด้วย `ssh project@10.198.200.62 'cd ~/docflow/ocr && docker compose stop proxy'` อัปโหลดไฟล์ใหม่ คาดหวัง `RETRY_WAIT` พร้อมข้อความ "รอลองใหม่ ครั้งที่ 2 เวลา hh:mm" (ครั้งที่ 1 ล้มเหลวแล้ว จึงแสดงครั้งถัดไป)
5. เปิด proxy กลับ แล้วกด "ประมวลผลใหม่" คาดหวังไปถึง `NEEDS_REVIEW`
6. บันทึกผลในรายงาน: เวลาประมวลผลจริง, ผลการตรวจ, และสิ่งที่ผิดปกติ

---

## Self-review (ตรวจกับ spec ก่อนส่ง)

**ความครอบคลุมของ spec**

| หัวข้อ spec | Task |
|---|---|
| การตัดสินใจ 1–6 และสถาปัตยกรรม | 2, 3, 4, 7 |
| ส่วนที่ 1 ชนิดข้อมูล, การบันทึก, กฎค่าความมั่นใจ, DocFlow contract | 2, 3, 5 |
| ส่วนที่ 2 Extractor, error, การตั้งค่า, endpoint ทดสอบ, reverse proxy | 1, 4, 8 |
| ส่วนที่ 3 คิว, คอลัมน์ใหม่, ลำดับสถานะ, reprocess, หน้าเว็บ, markExtractionFailed | 6, 7, 9, 10 |
| ส่วนที่ 4 เทสต์, deploy, ถอด one-ocr | 1–12 (เทสต์กระจายตาม Task) |
| ความเสี่ยงและคำถามที่เปิด | ไม่ใช่งานโค้ด; ระบุในรายงาน |
| อีเมลแจ้งแอดมิน | อยู่นอกขอบเขต (spec ระบุว่าเป็นงานแยก) ไม่มี Task |
| `DEPLOY_PODMAN.md` | แทนด้วย `deploy/ocr/README.md` (ดูข้อที่ต่างจาก spec ข้อ 1) |

**ความสอดคล้องของชื่อ:** `ExtractionResult`, `ExtractError`, `Extractor`, `httpExtractor`, `failurePlan`, `planExtractionFailure`, `retryResetUpdates`, `extractionJobs`, `markExtractionFailed`, `processingNote`, `canReprocess`, `confidenceLabel`, `personLabel` ใช้ชื่อเดียวกันทุก Task ที่อ้างถึง

**ยืนยันกับโค้ดจริงแล้ว (รอบแก้ไข 2026-10-11):** ชื่อ field ของ `ExtractedField` (`FieldKey`, `FieldLabel`), `Appointment`, `OCRPage`, `OCRLine` (`OCRPageID`), `AuditLog` (`DocumentID *uint`, `UserID`), `DocumentReference`; `currentUserID`; `ApiDocument`, `apiFetch`, `useMe`, `can`; สเกล `confidence` 0–1; `detectPDFPageCount` ใช้เป็น `pageCounter`; `truncateRunes` (ใช้ของเดิม). ยังต้องตรวจตอนลงมือ: ชื่อคอลัมน์ `extraction_*` (ตรวจด้วย DB จริงใน Task 12 เพราะไม่มี test DB) และ import ที่ไม่ได้ใช้ใน `document_processing.go` หลังลบ (`go vet`)

**Placeholder scan:** ไม่มี "TBD" หรือ "เพิ่ม error handling" ทั่วไป ทุก Task มี test ที่เขียนจริงสำหรับ logic ที่มีเงื่อนไข; งานที่ทดสอบไม่ได้ด้วย unit test (DB store, UI, deploy) ระบุวิธีตรวจที่รันได้จริง

**Known gap:** `gormExtractionJobs` ไม่มีเทสต์ unit เพราะไม่มี test DB (ตาม Global Constraints) ความถูกต้องของ SQL ตรวจใน Task 12 Step 8 ด้วยเอกสารจริง

## ความเสี่ยงที่ควรรู้

- ผลจริง (ผู้ควบคุมรายงาน): ไฟล์คำสั่งจริง `kku.pdf` ผ่าน thaidoc ได้ 119 สมาชิก, 31 คนที่ชื่อว่าง (กรรมการโดยตำแหน่ง เก็บเฉพาะตำแหน่ง), ใช้เวลา 15.5 วินาทีสำหรับ 7 หน้าบน CPU ของ VM
- DOCX/XLSX/CSV ไม่มีรายชื่อสมาชิกจากเส้นทาง `extractCommandMetadata` ที่ไม่มี profile (เหมือนเดิมสำหรับไฟล์ที่ไม่ใช่ตัวอย่าง) ต้องพิจารณาในงานถัดไป
- หน้า `src/app/upload/page.tsx` และ `src/app/page.tsx` ยังมีข้อความ "one-ocr" ในคำอธิบาย แผนนี้ไม่แก้ข้อความเหล่านั้น

- thaidoc ทดสอบบนเอกสาร TOR เท่านั้น ยังไม่รู้ความแม่นยำกับคำสั่งแต่งตั้ง
- ค่า `confidence` เป็น nil ทั้งหมดจาก thaidoc ทุกเอกสารจึงเป็น NEEDS_REVIEW (ตามที่ spec ตั้งใจ)
- `DueForQueue` ใช้ `updated_at` เป็นเกณฑ์ QUEUED ค้าง ถ้า gorm ไม่อัปเดต field นี้เองในบาง path ให้เปลี่ยนเป็น `processing_started_at` หรือเพิ่มคอลัมน์ `queued_at`

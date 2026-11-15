# ชื่อที่ไม่พบในระบบ Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** ชื่อที่ผูกไม่ได้ไม่ขวางการยืนยันอัตโนมัติ แสดงป้าย "ไม่อยู่ในระบบ" และผูกให้เองเมื่อคนนั้นเข้าระบบ รวมเอกสารที่ยืนยันแล้ว

**Architecture:** "คนนอก" ดูจากสภาพของแถว ไม่มี migration เงื่อนไขอยู่ที่ `readyToAutoConfirm` ส่วนการผูกย้อนหลังใช้ฟังก์ชันใหม่ `linkExternalNames` ซึ่งถูกเรียกหลัง sync และหลังเพิ่มอาจารย์ เสร็จแล้วยืนยันเอกสารที่พร้อมด้วยตัวยืนยันอัตโนมัติชุดเดิม

**Tech Stack:** Go + Gin + GORM (MySQL 8.4) · Next.js 16 · `node --test`

**Spec:** `docs/superpowers/specs/2026-11-15-external-names-design.md`

## Global Constraints

- ผูกเองได้เฉพาะชื่อที่ตรงเป๊ะ (EXACT) หรือชื่อที่เคยผูกไว้ (ALIAS) ห้ามผูก FUZZY เอง
- เอกสารที่ยืนยันแล้ว แตะได้เฉพาะแถวที่ `TRIM(full_name) <> ''` และ `directory_user_id IS NULL`
- DB test รันกับ `docflow_test` เท่านั้น ใช้ `go vet` แทน `go build` และ revert `AGENTS.md` ก่อน commit
- ข้อความที่ผู้ใช้เห็นและคอมเมนต์เขียนเป็นภาษาไทย
- รัน code-review ก่อน commit และห้าม push

## Review Focus

1. เอกสารที่ยกเลิกแล้ว ต้องไม่ถูกผูกย้อนหลัง → Task 3 `TestLinkExternalNamesSkipsCancelledDocuments`
2. ชื่อเดียวกันที่อยู่ในหลายเอกสาร ต้องผูกครบทุกเอกสาร และมี revision ครบทุกฉบับ → Task 3 (case ใน `TestLinkExternalNamesFillsConfirmedDocuments`)
3. ปิดการยืนยันอัตโนมัติไว้ หลังผูกย้อนหลังต้องไม่มีเอกสารถูกยืนยัน → Task 2 `TestAutoConfirmPendingIfEnabledRespectsTheSwitch`
4. แถวที่เจ้าหน้าที่เอาคนออกเอง (MANUAL และไม่มีคน) ต้องไม่ถูกผูกกลับ → Task 3 (case `manualCleared`)
5. ในหน้าตรวจ ชื่อคนนอกต้องไม่อยู่ในตัวกรอง "ต้องดู" (warning) → Task 4 `countByFilter leaves external names out of warning`

---

### Task 1: `readyToAutoConfirm` ไม่นับชื่อคนนอก

**Files:**
- Modify: `backend/document_status.go:113-117`
- Modify: `backend/document_status_test.go` (`TestAutoConfirmTakesOnlyDocumentsWithNothingToFix`)

**Interfaces:**
- Produces: `readyToAutoConfirm` นับเป็นงานค้างเฉพาะแถว `directory_user_id IS NULL AND (TRIM(COALESCE(full_name,'')) = '' OR name_match_method = 'DIRECTORY_EMPTY')`

- [ ] **Step 1: แก้เทสต์ให้ fail** ใน `TestAutoConfirmTakesOnlyDocumentsWithNothingToFix`
  - case "unlinked name" (สร้างด้วย `NameMatchMethod: matchMethodNotFound`) เปลี่ยน want เป็น `true`
  - เพิ่ม case `"external fuzzy"` (`NameMatchMethod: matchMethodFuzzy`) want `true`
  - เพิ่ม case `"vacant ex-officio"` (`FullName: ""`, `Position: "คณบดี"`, `NameMatchMethod: matchMethodExOfficio`) want `false`
  - เพิ่ม case `"empty directory"` (`NameMatchMethod: matchMethodEmpty`) want `false`
  - ทุก case ใช้ `order_no` / `issued_date` ที่ confidence 0.98 และ `BoundingBoxJSON: "null"`
- [ ] **Step 2:** รัน `go test -count=1 -run TestAutoConfirmTakesOnly ./...` ด้วย DSN ใน `CLAUDE.md` ต้อง FAIL ที่ "unlinked name" และ "external fuzzy"
- [ ] **Step 3:** แก้ query ใน `readyToAutoConfirm` ตาม Interfaces
- [ ] **Step 4:** รันเทสต์เดิม ต้อง PASS
- [ ] **Step 5:** commit `fix: external names no longer block auto-confirm`

### Task 2: ยืนยันเอกสารที่พร้อม เมื่อเปิดการยืนยันอัตโนมัติไว้

**Files:**
- Modify: `backend/document_status.go` (`setAutoConfirm`)
- Modify: `backend/main.go:487` (ต่อจาก `refreshPendingMatches` ตอน backend เริ่ม)
- Test: `backend/document_status_test.go`

**Interfaces:**
- Produces:
  - `func confirmReadyPending(db *gorm.DB, threshold float64) (int, error)` คือลูปเดิมใน `setAutoConfirm` แยกออกมา `setAutoConfirm` เรียกตัวนี้แทน
  - `func autoConfirmPendingIfEnabled(db *gorm.DB) (int, error)` ถ้า `autoConfirmEnabled` เป็น false คืน 0 ถ้าเปิดไว้เรียก `confirmReadyPending(db, reviewThresholdOf(NewSettingsStore(db)))`

- [ ] **Step 1: เทสต์** `TestAutoConfirmPendingIfEnabledRespectsTheSwitch`
  - สร้างเอกสาร REVIEW ที่มีแต่ชื่อ NOT_FOUND และ field ครบ
  - ปิดสวิตช์ (เขียน `AppSetting` ตรง ๆ) → เรียก `autoConfirmPendingIfEnabled` → เอกสารยังเป็น REVIEW
  - เปิดสวิตช์ → เรียกอีกครั้ง → เอกสารเป็น CONFIRMED และ `confirmed_by` เป็น `autoConfirmActor`
  - เทสต์ต้องคืนค่าสวิตช์เดิมตอนจบ (`t.Cleanup`) เพราะ `docflow_test` ใช้ร่วมกัน
- [ ] **Step 2:** รันเทสต์ ต้อง FAIL เพราะ undefined `autoConfirmPendingIfEnabled`
- [ ] **Step 3:** implement ตาม Interfaces และใน `main.go` ถ้า error ให้ log ด้วยข้อความ `"auto-confirm pending at startup: %v"`
- [ ] **Step 4:** รันเทสต์นี้ และ `TestAutoConfirm*` ที่มีอยู่ ต้อง PASS
- [ ] **Step 5:** commit `feat: confirm ready documents with one shared helper`

### Task 3: `linkExternalNames` ผูกชื่อคนนอกย้อนหลัง

**Files:**
- Create: `backend/external_names.go`
- Create: `backend/external_names_test.go`
- Modify: `backend/sync.go:132-136` เรียกหลัง `refreshPendingMatches` สำเร็จ
- Modify: `backend/api_handlers.go:716-721` (`createDirectoryUser`) เรียกหลังสร้างคนสำเร็จ แล้วบวกจำนวนเข้า `linked`

**Interfaces:**
- Consumes: `autoConfirmPendingIfEnabled` (Task 2) · `loadDirectory`, `matchDirectoryUser`, `applyMatchToAppointment`, `displayDirectoryName`
- Produces: `func linkExternalNames(db *gorm.DB, changedBy string) (int, error)` คืนจำนวนแถวที่ผูก ถ้าผูกได้อย่างน้อยหนึ่งแถว ให้เรียก `autoConfirmPendingIfEnabled(db)` ต่อ

- [ ] **Step 1: เทสต์** ทุกเทสต์สร้างคนและชื่อในเอกสารโดยมี stamp
  - `TestLinkExternalNamesFillsConfirmedDocuments`:
    - มีเอกสาร CONFIRMED 2 ฉบับ ทั้งสองมีชื่อ `"ดร.ภายนอก<stamp> ทดสอบ"` (NOT_FOUND)
    - สร้าง `DirectoryPerson` ที่ `FirstName "ภายนอก<stamp>"`, `LastName "ทดสอบ"` แล้วเรียก `linkExternalNames(db, "sync")`
    - ทั้งสองแถวต้องได้ `directory_user_id` = คนนั้น และ `name_match_method` เป็น EXACT
    - แต่ละเอกสารต้องมี `DocumentRevision{FieldKey: "appointment_link", OldValue: "ดร.ภายนอก<stamp> ทดสอบ", ChangedBy: "sync"}` และ `NewValue` เป็นชื่อบุคลากร
    - สถานะเอกสารยังเป็น CONFIRMED
  - `TestLinkExternalNamesLeavesOtherRowsAlone` (ใช้เอกสาร CONFIRMED ฉบับเดียว) แถวพวกนี้ต้องไม่เปลี่ยน:
    - `fuzzy`: ชื่อที่ต่างจากคนในระบบ 1 ตัวอักษร
    - `linkedElsewhere`: ชื่อตรงกับคนใหม่ แต่ผูกกับคนอื่นอยู่แล้ว
    - `manualCleared`: ชื่อตรง แต่ `NameMatchMethod: matchMethodManual` และไม่มีคน
    - `exOfficio`: ไม่มีชื่อ มีแค่ตำแหน่ง
  - `TestLinkExternalNamesSkipsCancelledDocuments`: เอกสารที่ `status = documentCancelled` และมีชื่อตรง ต้องไม่ถูกผูก
- [ ] **Step 2:** รันเทสต์ ต้อง FAIL เพราะ undefined `linkExternalNames`
- [ ] **Step 3: implement**
  - query: `appointments JOIN document_workflows` โดย `d.status <> documentCancelled`, `TRIM(a.full_name) <> ''`, `a.directory_user_id IS NULL`, `COALESCE(a.name_match_method,'') <> 'MANUAL'`
  - แต่ละแถวเทียบด้วย `matchDirectoryUser` ถ้าได้ EXACT หรือ ALIAS ให้ update `directory_user_id`, `name_match_method` และ `name_match_score` (ไม่แก้ `full_name`) แล้วเขียน `DocumentRevision` ใน transaction เดียวกัน
  - ตัวเรียกใน `sync.go` และ `createDirectoryUser`: log error แล้วทำงานต่อ ไม่ทำให้ sync หรือการสร้างคนล้ม `changedBy` เป็น `"sync"` และผู้ใช้ที่สร้างคน (`createdBy`) ตามลำดับ
- [ ] **Step 4:** รันเทสต์ Task 3 และทั้งชุด backend พร้อม `go vet ./...` ต้อง PASS
- [ ] **Step 5:** commit `feat: link external names once the person joins the directory`

### Task 4: หน้าตรวจเอกสารแสดง "ไม่อยู่ในระบบ" และไม่นับเป็นงานค้าง

**Files:**
- Modify: `src/lib/member-review.ts`
- Modify: `src/lib/member-review.test.ts`
- Modify: `src/components/document-workspace.tsx:102-107, 434-464, 512`

**Interfaces:**
- Produces:
  - `export function isExternalName(row: Appointment): boolean` ได้ true เมื่อ `!isLinked(row) && row.fullName.trim() !== "" && row.nameMatchMethod !== "DIRECTORY_EMPTY"`
  - `MatchTone` เพิ่มค่า `"neutral"`

- [ ] **Step 1: เทสต์** ใน `member-review.test.ts`
  - `isExternalName`: NOT_FOUND, AMBIGUOUS และ FUZZY ที่ยังไม่ผูก ได้ true · แถวที่ผูกแล้ว แถวโดยตำแหน่ง (ไม่มีชื่อ) และ DIRECTORY_EMPTY ได้ false
  - `matchState`: NOT_FOUND ได้ `{ label: "ไม่อยู่ในระบบ", tone: "neutral" }` · AMBIGUOUS ยังได้ label "ชื่อซ้ำ 2 คน" แต่ tone เป็น `"neutral"`
  - `countByFilter leaves external names out of warning`: จาก `rows` เดิม `warning` ต้องเท่ากับ 1 (เหลือแค่ FUZZY ที่ผูกแล้ว id 2)
  - แก้ assertion เดิมใน `matchState keeps unlinked names out of the good tone` ให้ตรงกฎใหม่
- [ ] **Step 2:** รัน `npm test` ต้อง FAIL
- [ ] **Step 3: implement**
  - ใน `matchState` ถ้า `isExternalName(row)` ให้ tone เป็น `"neutral"` ส่วน label ให้ NOT_FOUND เป็น "ไม่อยู่ในระบบ" แบบอื่นใช้ label เดิม
  - `matchesFilter` "warning" ให้เป็น `tone === "warning" || tone === "danger"`
  - ทุกที่ที่ map tone เป็นสี ต้องรองรับ `"neutral"` ใช้สี muted (หาด้วย `grep -rn "tone" src/components`)
- [ ] **Step 4: แก้ `document-workspace.tsx`**
  - แยก `unlinkedAppointments` เป็น `blockingRows` (ยังไม่ผูก และไม่ใช่ `isExternalName`) กับ `externalRows`
  - ตัวนับ "ยังไม่ผูก" (บรรทัด 434, 460) และแถบล่าง (บรรทัด 512) ใช้ `blockingRows`
  - เพิ่มตัวนับสีกลาง "ไม่อยู่ในระบบ {externalRows.length}"
  - `missingPeople` นับจาก `externalRows` ที่ไม่มี candidates
  - ปุ่มกรอง "ดูเฉพาะแถวนี้" แสดงทั้ง blocking และ external
- [ ] **Step 5:** รัน `npm test && npx tsc --noEmit -p . && node_modules/.bin/eslint <ไฟล์ที่แก้>` ต้องผ่าน แล้วเปิดหน้าตรวจเอกสารในเครื่องดูหนึ่งครั้ง
- [ ] **Step 6:** commit `feat: show external names as ไม่อยู่ในระบบ without blocking review`

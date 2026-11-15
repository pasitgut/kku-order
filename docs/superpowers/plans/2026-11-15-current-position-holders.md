# บุคลากรโดยตำแหน่ง Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** หน้า `/positions` แสดงผู้ดำรงตำแหน่งบริหาร ณ วันนี้ ตรงกับคนที่ระบบผูกให้แถวโดยตำแหน่งในเอกสารที่ออกวันนี้

**Architecture:** backend เพิ่ม `currentPositionHolders(db, today)` ที่ใช้ `loadPositionHolders` และ `pickHolder` ชุดเดียวกับการผูกเอกสาร แล้วเปิดเป็น `GET /api/v1/position-holders/current` ฝั่งหน้าเว็บมีหน้าใหม่ที่แสดงอย่างเดียว

**Tech Stack:** Go + Gin + GORM (MySQL 8.4) · Next.js 16 App Router + Tailwind · `@phosphor-icons/react` · `node --test`

**Spec:** `docs/superpowers/specs/2026-11-15-current-position-holders-design.md`

## Global Constraints

- ข้อความที่ผู้ใช้เห็นและคอมเมนต์ในโค้ดเขียนเป็นภาษาไทย
- DB test ต้องรันกับ `docflow_test` เท่านั้น ห้ามใช้ `docflow` ดูคำสั่งใน `CLAUDE.md`
- ใช้ `go vet ./...` แทน `go build ./...`
- `next dev` จะแก้ `AGENTS.md` เอง (เปลี่ยน `#` เป็น `##`) ต้อง revert ก่อน commit
- ห้าม push
- ก่อน commit แต่ละครั้ง ให้รัน `code-review` กับ diff และรายงานผลก่อน (ตาม CLAUDE.md ของ Kit)
- fix compile error ใน `backend/drive_automation.go` ที่ค้างอยู่ commit แยกก่อนเริ่ม Task 1 (`fix: use renamed Drive file after dated rename`)

## Review Focus

1. คนเดียวดำรง 2 ตำแหน่ง ต้องขึ้น 2 แถว → Task 1 `TestCurrentHoldersListEveryPositionOfOnePerson`
2. `ends_on` เท่ากับวันนี้ ต้องยังแสดง เพราะนับรวมวันสุดท้าย → Task 1 `TestCurrentHoldersFollowPickHolder`
3. ตำแหน่งที่มีชื่อวิทยาลัยต่อท้าย เช่น "คณบดีวิทยาลัยการคอมพิวเตอร์" ต้องได้ rank เดียวกับ "คณบดี" → Task 1 `TestPositionRank`
4. ฐานข้อมูลไม่มีผู้ดำรงเลย ต้องได้ `data: []` ไม่ใช่ `null` → Task 1 `TestCurrentHoldersHandlerReturnsAnArray`
5. ตำแหน่งที่มีแต่ข้อมูลจากการตรวจเอกสาร (LEARNED) ต้องแสดง และมีป้าย "จากการตรวจเอกสาร" → Task 1 (case ใน `TestCurrentHoldersFollowPickHolder`) และ Task 2 `sourceLabel`

---

### Task 1: backend `GET /api/v1/position-holders/current`

**Files:**
- Create: `backend/current_position_holders.go`
- Create: `backend/current_position_holders_test.go`
- Modify: `backend/main.go` ลงทะเบียน route ต่อจาก `router.GET("/api/v1/position-holders/vacant", …)` ประมาณบรรทัด 368 และไม่ใส่ `requireRole`

**Interfaces:**
- Produces:
  - `type CurrentPositionHolder struct { PositionTitle string \`json:"positionTitle"\`; DirectoryUserID uint \`json:"directoryUserId"\`; FullName string \`json:"fullName"\`; StartsOn *time.Time \`json:"startsOn"\`; EndsOn *time.Time \`json:"endsOn"\`; Source string \`json:"source"\` }`
  - `func positionRank(title string) int`
  - `func sortCurrentHolders(items []CurrentPositionHolder)`
  - `func currentPositionHolders(db *gorm.DB, today time.Time) ([]CurrentPositionHolder, error)`
  - `func listCurrentPositionHolders(db *gorm.DB) gin.HandlerFunc` คืนค่า `{"data": [...]}`
  - route `GET /api/v1/position-holders/current`

- [ ] **Step 1: เขียนเทสต์ที่ fail**

ทุกเทสต์ที่ใช้ DB ใช้ `testDatabase(t)` และต่อท้ายชื่อตำแหน่งกับชื่อคนด้วย stamp `time.Now().Format("150405.000000")` เพราะ `docflow_test` ใช้ร่วมกันหลายเทสต์ ให้หาเฉพาะแถวที่มี stamp ของตัวเอง ใช้ `today := time.Date(2026, 11, 15, 0, 0, 0, 0, time.Local)` เป็นวันตายตัว สร้างคนเป็นแถว `DirectoryPerson` (Prefix/FirstName/LastName หรือ FullName และ status ว่างหรือ "INACTIVE") แล้วสร้าง `PositionHolder` ตรง ๆ โดยตั้ง `PositionKey: positionKey(title)`

```go
func TestPositionRank(t *testing.T) {
	cases := map[string]int{
		"คณบดี": 0, "คณบดีวิทยาลัยการคอมพิวเตอร์": 0,
		"รองคณบดีฝ่ายวิชาการ": 1, "ผู้ช่วยคณบดีฝ่ายวิจัย": 2,
		"หัวหน้าสาขาวิชาวิทยาการคอมพิวเตอร์": 3, "เลขานุการวิทยาลัยการคอมพิวเตอร์": 4,
	}
	// assert positionRank(title) == want ทุกคู่
}

// TestCurrentHoldersFollowPickHolder: แต่ละ case คือหนึ่งตำแหน่ง (ชื่อมี stamp) ต้องได้ผลตามนี้
//  a) SYNC ไม่มีวัน (คน A) + MANUAL ไม่มีวัน (คน B)           → B, Source "MANUAL"
//  b) MANUAL เริ่ม 2026-01-01 จบ 2026-11-15 (เท่ากับวันนี้)     → แสดง, EndsOn = 2026-11-15
//  c) MANUAL จบ 2026-11-14                                       → ไม่มีแถว
//  d) MANUAL เริ่ม 2026-12-01                                    → ไม่มีแถว
//  e) MANUAL ไม่มีวัน แต่คนเป็น INACTIVE                          → ไม่มีแถว
//  f) LEARNED เริ่ม 2026-06-01 อย่างเดียว                         → แสดง, Source "LEARNED"
// และทุกแถวที่ได้ DirectoryUserID ต้องเท่ากับ holderFor(title, loadPositionHolders(db), &today).DirectoryPersonID
// FullName ของ B = "ผศ.ดร. <ชื่อ> <สกุล>" เมื่อ Prefix = "ผศ.ดร."

func TestCurrentHoldersListEveryPositionOfOnePerson(t *testing.T) // คนเดียว MANUAL 2 ตำแหน่ง → 2 แถว ชื่อเดียวกัน

func TestSortCurrentHolders(t *testing.T) // unit test ไม่ใช้ DB
// input PositionTitle: ["เลขานุการวิทยาลัย", "หัวหน้าสาขา", "รองคณบดีฝ่ายวิจัย", "รองคณบดีฝ่ายวิชาการ", "คณบดี"]
// หลัง sortCurrentHolders → ["คณบดี", "รองคณบดีฝ่ายวิจัย", "รองคณบดีฝ่ายวิชาการ", "หัวหน้าสาขา", "เลขานุการวิทยาลัย"]

func TestCurrentHoldersHandlerReturnsAnArray(t *testing.T)
// เรียก handler ผ่าน httptest (ดูแบบใน position_holders_test.go) → 200 และ body มี "data":[ (ห้ามเป็น null)
```

- [ ] **Step 2: รันเทสต์ ต้อง fail**

Run: `cd backend && DOCFLOW_TEST_DSN='docflow:docflow@tcp(127.0.0.1:3307)/docflow_test?charset=utf8mb4&parseTime=True&loc=Local' go test -count=1 -run 'PositionRank|CurrentHolders' ./...`
(`TestSortCurrentHolders` ถูกจับด้วย pattern `CurrentHolders`)
Expected: FAIL เพราะ undefined `positionRank` / `currentPositionHolders` / `sortCurrentHolders` / `listCurrentPositionHolders`

- [ ] **Step 3: implement ใน `backend/current_position_holders.go`**

- `positionRank`: ใช้ `k := positionKey(title)` แล้วเช็คตามลำดับนี้ `HasPrefix(k, "รองคณบดี")` → 1, `HasPrefix(k, "ผู้ช่วยคณบดี")` → 2, `k == "คณบดี"` → 0, `HasPrefix(k, "หัวหน้า")` → 3, อื่น ๆ → 4
- `sortCurrentHolders(items []CurrentPositionHolder)`: เรียงด้วย `sort.SliceStable` ตาม rank, PositionTitle แล้ว FullName
- `currentPositionHolders`:
  1. เรียก `loadPositionHolders(db)`
  2. แต่ละ key ข้ามถ้า `committeeRoleOnly(list[0].PositionTitle)` แล้วเรียก `pickHolder(list, &today)` ถ้าได้ nil ให้ข้าม
  3. โหลด `DirectoryPerson` ของ id ที่เลือกได้ด้วย query เดียว
  4. ชื่อใช้ `FullName` ถ้ามีค่า ถ้าไม่มีให้ต่อ `Prefix FirstName LastName` แบบเดียวกับ `directoryPersonAPI`
  5. เรียกเสร็จแล้ว sort ผลลัพธ์ และคืน slice ว่าง (ไม่ใช่ nil) ถ้าไม่มีแถว
- handler: ใช้ `today := dateOnly(time.Now().In(time.FixedZone("Asia/Bangkok", 7*60*60)))` ถ้า error ตอบ 500 `{"error": "ไม่สามารถโหลดรายชื่อผู้ดำรงตำแหน่งได้"}`
- `main.go`: `router.GET("/api/v1/position-holders/current", listCurrentPositionHolders(db))`

- [ ] **Step 4: รันเทสต์ ต้องผ่าน และรันทั้งชุด**

Run: คำสั่ง DSN เดียวกัน ตามด้วย `go test -count=1 ./...` และ `go vet ./...`
Expected: ทุกเทสต์ PASS และ vet ไม่มี output

- [ ] **Step 5: Commit**

```bash
git add backend/current_position_holders.go backend/current_position_holders_test.go backend/main.go
git commit -m "feat: list current executive position holders"
```

### Task 2: API client และป้ายที่มาของข้อมูล

**Files:**
- Modify: `src/lib/api.ts` (ใส่ใกล้ `getVacantPositions` ประมาณบรรทัด 311)
- Create: `src/lib/current-positions.ts`
- Create: `src/lib/current-positions.test.ts`

**Interfaces:**
- Consumes: response ของ Task 1
- Produces:
  - `export type CurrentPositionHolder = { positionTitle: string; directoryUserId: number; fullName: string; startsOn: string | null; endsOn: string | null; source: string }`
  - `export async function getCurrentPositionHolders(): Promise<{ data: CurrentPositionHolder[] }>` เรียก `apiFetch` ไปที่ `/api/v1/position-holders/current`
  - `export function sourceLabel(source: string): string`

- [ ] **Step 1: เขียนเทสต์ที่ fail** `src/lib/current-positions.test.ts` (ใช้ `node:test` และ `node:assert/strict` แบบ `position-holder.test.ts`)

```ts
test("sourceLabel names where the holder came from", () => {
  assert.equal(sourceLabel("SYNC"), "ระบบวิทยาลัย");
  assert.equal(sourceLabel("MANUAL"), "กรอกเอง");
  assert.equal(sourceLabel("LEARNED"), "จากการตรวจเอกสาร");
  assert.equal(sourceLabel("OTHER"), "OTHER");
});
```

- [ ] **Step 2:** รัน `npm test` ต้อง FAIL เพราะ import ไม่เจอ
- [ ] **Step 3:** implement `sourceLabel` และเพิ่ม type กับ fetch ใน `api.ts`
- [ ] **Step 4:** รัน `npm test && npx tsc --noEmit -p .` ต้อง PASS
- [ ] **Step 5: Commit** `git add src/lib/api.ts src/lib/current-positions.ts src/lib/current-positions.test.ts && git commit -m "feat: client for current position holders"`

### Task 3: หน้า `/positions` และเมนู

**Files:**
- Create: `src/app/positions/page.tsx`
- Modify: `src/components/app-shell.tsx:12-17` เพิ่มเมนูต่อจาก "ค้นหาข้อมูล"

**Interfaces:**
- Consumes: `getCurrentPositionHolders`, `CurrentPositionHolder`, `sourceLabel` (Task 2) · `formatThaiDate` (`src/lib/api.ts`) · `useMe` (`@/components/me-context`) · `can(role, "createPerson")` (`@/lib/permissions`)

- [ ] **Step 1: เมนู** เพิ่ม `{ href: "/positions", label: "บุคลากรโดยตำแหน่ง", shortLabel: "ตำแหน่ง", icon: IdentificationBadge }` และ import ไอคอนจาก phosphor
- [ ] **Step 2: หน้า** ทำเป็น client component ตามโครงหน้า `src/app/search/page.tsx`
  - หัวหน้า "บุคลากรโดยตำแหน่ง" ใต้หัวมีบรรทัด "ข้อมูล ณ วันที่ {formatThaiDate(วันนี้)}"
  - ตารางมีคอลัมน์ ตำแหน่ง | ชื่อ-สกุล | วันเริ่ม | วันสิ้นสุด | ที่มา วันที่ว่างแสดง "—"
  - ถ้า `can(me.role, "createPerson")` แสดงลิงก์ "แก้ไขในข้อมูลอาจารย์" ไป `/settings/teachers`
  - ระหว่างโหลดแสดงสถานะกำลังโหลด
  - ถ้า error แสดงข้อความ error ที่ได้จาก API
  - ถ้าไม่มีข้อมูล แสดง "ยังไม่มีข้อมูลผู้ดำรงตำแหน่ง ข้อมูลมาจากการ sync รายชื่อของวิทยาลัย หรือการกรอกในหน้าข้อมูลอาจารย์"
- [ ] **Step 3: ตรวจ** รัน `npx tsc --noEmit -p . && npx eslint src && npm test` ต้องผ่านหมด
- [ ] **Step 4: ตรวจในเครื่อง**
  - สร้างข้อมูลทดสอบ 1 แถวใน `docflow` ของเครื่อง ผ่าน `POST /api/v1/directory-users` พร้อม `managePosition: "คณบดี"` และ `managePositionSince: "2026-10-01"`
  - เปิด http://localhost:3001/positions ต้องเห็นแถวนั้น
  - `curl -s localhost:8080/api/v1/position-holders/current` ต้องได้ข้อมูลเดียวกัน
  - เสร็จแล้วลบข้อมูลทดสอบออก
- [ ] **Step 5: Commit** revert `AGENTS.md` ก่อนถ้าถูกแก้ แล้ว `git add src/app/positions/page.tsx src/components/app-shell.tsx docs/superpowers && git commit -m "feat: add บุคลากรโดยตำแหน่ง page"`

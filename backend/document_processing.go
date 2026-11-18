package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type DocumentProcessor struct {
	db               *gorm.DB
	logger           *log.Logger
	settings         SettingsStore
	jobs             extractionJobs
	ocrConfig        ocrAPIConfig
	newExtractor     func(OCRSettings, ocrAPIConfig) (Extractor, error)
	pageCount        pageCounter
	importStructured func(ctx context.Context, path, sourceType string) (workerResult, error)
	now              func() time.Time
	queue            chan uint
}

type workerResult struct {
	Pages []workerPage `json:"pages"`
}

type workerPage struct {
	PageNo      int          `json:"pageNo"`
	ImageWidth  int          `json:"imageWidth"`
	ImageHeight int          `json:"imageHeight"`
	ImageAngle  float32      `json:"imageAngle"`
	FullText    string       `json:"fullText"`
	Lines       []workerLine `json:"lines"`
}

type workerLine struct {
	Text        string       `json:"text"`
	Confidence  float32      `json:"confidence"`
	BoundingBox []float64    `json:"boundingBox"`
	Words       []workerWord `json:"words"`
}

type workerWord struct {
	Text        string    `json:"text"`
	Confidence  float32   `json:"confidence"`
	BoundingBox []float64 `json:"boundingBox"`
}

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

// buildExtractor reads the effective API key at call time, so a key saved from the web applies without a restart.
func (p *DocumentProcessor) buildExtractor(settings OCRSettings) (Extractor, error) {
	cfg := p.ocrConfig
	cfg.APIKey, _ = effectiveOCRAPIKey(p.settings, p.ocrConfig)
	return p.newExtractor(settings, cfg)
}

func (p *DocumentProcessor) runStructuredImport(ctx context.Context, documentPath, sourceType string) (workerResult, error) {
	tempDir, err := os.MkdirTemp("", "docflow-import-")
	if err != nil {
		return workerResult{}, fmt.Errorf("สร้างพื้นที่นำเข้าข้อมูล: %w", err)
	}
	defer os.RemoveAll(tempDir)
	outputJSON := filepath.Join(tempDir, "import.json")
	ocrScript := getenv("OCR_SCRIPT", filepath.Join(".", "ocr_worker.py"))
	pythonBin := getenv("OCR_PYTHON", "python")
	args := []string{ocrScript, "--input-file", documentPath, "--source-type", strings.ToLower(sourceType), "--output-json", outputJSON}
	command := exec.CommandContext(ctx, pythonBin, args...)
	command.Env = append(os.Environ(), "PYTHONUTF8=1")
	if output, err := command.CombinedOutput(); err != nil {
		// traceback มี path ของเซิร์ฟเวอร์ จึงเก็บไว้ใน log เท่านั้น ผู้ใช้เห็นแค่ข้อความที่ worker ตั้งใจส่งให้
		if p.logger != nil {
			p.logger.Printf("structured import %s failed: %v: %s", sourceType, err, strings.TrimSpace(string(output)))
		}
		return workerResult{}, fmt.Errorf("นำเข้าข้อมูลจาก %s: %s", sourceType, structuredImportUserError(string(output)))
	}
	contents, err := os.ReadFile(outputJSON)
	if err != nil {
		return workerResult{}, fmt.Errorf("อ่านผลลัพธ์การนำเข้าข้อมูล: %w", err)
	}
	var result workerResult
	if err := json.Unmarshal(contents, &result); err != nil {
		return workerResult{}, fmt.Errorf("แปลงผลลัพธ์การนำเข้าข้อมูล: %w", err)
	}
	if len(result.Pages) == 0 {
		return workerResult{}, errors.New("ไม่พบข้อมูลในไฟล์นำเข้า")
	}
	return result, nil
}

const structuredImportErrorPrefix = "DOCFLOW_IMPORT_ERROR: "

// structuredImportUserError ดึงข้อความสำหรับผู้ใช้จาก output ของ ocr_worker.py
// ถ้าไม่มีข้อความนั้น (เช่น python ล้มก่อนเริ่ม) ใช้ข้อความกลางแทน ไม่ส่ง output ดิบออกไป
func structuredImportUserError(output string) string {
	lines := strings.Split(output, "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		if message, ok := strings.CutPrefix(strings.TrimSpace(lines[index]), structuredImportErrorPrefix); ok && strings.TrimSpace(message) != "" {
			return strings.TrimSpace(message)
		}
	}
	return "ตัวอ่านไฟล์ทำงานไม่สำเร็จ"
}

type commandMetadata struct {
	title            string
	orderType        string
	orderNo          string
	orderYearBE      int
	signer           string
	signerPosition   string
	responsibilities string
	additionalRefs   string
	issuedDate       *time.Time
	effectiveDate    *time.Time
	expiryDate       *time.Time
	lowConfidence    bool
	fields           []ExtractedField
	appointments     []Appointment
}

func extractCommandMetadata(pages []workerPage, fullText string) commandMetadata {
	metadata := commandMetadata{}
	metadata.orderType = inferOrderType(fullText)
	orderNo := findOrderNo(fullText)
	metadata.orderNo = orderNo
	metadata.orderYearBE = findOrderYear(orderNo)
	metadata.title = extractCommandTitle(pages, fullText)
	metadata.responsibilities = extractAfterLabel(fullText, "หน้าที่รับผิดชอบ")
	metadata.additionalRefs = extractAfterLabel(fullText, "การอ้างอิงประกาศเพิ่มเติม")
	metadata.signer = extractSigner(fullText)
	metadata.signerPosition = extractSignerPosition(fullText)
	metadata.issuedDate = findDateAfterLabels(fullText, "วันที่ออกคำสั่ง", "ลงวันที่", "วันที่")
	if metadata.issuedDate == nil {
		// คำสั่งส่วนใหญ่มีวันที่เดียวคือวันที่ออกคำสั่ง จึงใช้วันที่แรกในข้อความได้เฉพาะช่องนี้
		metadata.issuedDate = parseThaiDate(fullText)
	}
	metadata.effectiveDate = findDateAfterLabels(fullText, "วันที่เริ่มมีผล", "มีผลตั้งแต่")
	metadata.expiryDate = findDateAfterLabels(fullText, "วันหมดอายุ", "วาระการดำรงตำแหน่ง", "หมดวาระ")
	if metadata.effectiveDate == nil {
		metadata.effectiveDate = metadata.issuedDate
	}

	fieldSpecs := []struct {
		key, label, fieldType string
		value                 string
		confidence            float32
		page                  int
	}{
		{"order_type", "ประเภทเอกสาร", "text", metadata.orderType, confidenceFor(metadata.orderType), findPage(pages, metadata.orderType)},
		{"order_no", "เลขที่คำสั่ง", "text", metadata.orderNo, confidenceFor(metadata.orderNo), findPage(pages, metadata.orderNo)},
		{"order_year_be", "ปีที่ออกคำสั่ง (พ.ศ.)", "number", strconv.Itoa(metadata.orderYearBE), confidenceFor(strconv.Itoa(metadata.orderYearBE)), findPage(pages, strconv.Itoa(metadata.orderYearBE))},
		{"committee_name", "ชื่อเรื่อง/ชื่อคณะกรรมการ", "text", metadata.title, confidenceFor(metadata.title), findPage(pages, metadata.title)},
		{"issued_date", "วันที่ออกคำสั่ง", "date", formatDate(metadata.issuedDate), confidenceFor(formatDate(metadata.issuedDate)), findPage(pages, formatDate(metadata.issuedDate))},
		{"effective_date", "วันที่เริ่มมีผล", "date", formatDate(metadata.effectiveDate), confidenceFor(formatDate(metadata.effectiveDate)), findPage(pages, formatDate(metadata.effectiveDate))},
		{"expiry_date", "วันหมดอายุ/วาระการดำรงตำแหน่ง", "date", formatDate(metadata.expiryDate), confidenceFor(formatDate(metadata.expiryDate)), findPage(pages, formatDate(metadata.expiryDate))},
		{"signer_name", "ชื่อผู้ลงนาม", "text", metadata.signer, confidenceFor(metadata.signer), findPage(pages, metadata.signer)},
		{"signer_position", "ตำแหน่งผู้ลงนาม", "text", metadata.signerPosition, confidenceFor(metadata.signerPosition), findPage(pages, metadata.signerPosition)},
		{"responsibilities", "หน้าที่รับผิดชอบ", "text", metadata.responsibilities, confidenceFor(metadata.responsibilities), findPage(pages, metadata.responsibilities)},
		{"additional_references", "การอ้างอิงประกาศเพิ่มเติม", "text", metadata.additionalRefs, confidenceFor(metadata.additionalRefs), findPage(pages, metadata.additionalRefs)},
	}
	for _, spec := range fieldSpecs {
		bbox := findLineBoundingBox(pages, spec.value)
		metadata.fields = append(metadata.fields, ExtractedField{DocumentID: 0, FieldKey: spec.key, FieldLabel: spec.label, FieldType: spec.fieldType, Value: spec.value, SourceText: spec.value, Confidence: spec.confidence, PageNo: spec.page, BoundingBoxJSON: bbox, Verified: false})
		if spec.confidence < 0.90 {
			metadata.lowConfidence = true
		}
	}
	for _, page := range pages {
		department := ""
		departmentResponsibilities := ""
		for _, row := range groupOCRRows(page) {
			rowText := strings.Join(cellTexts(row), " ")
			if name, ok := parseRowDepartmentHeader(row, rowText); ok {
				department = name
				departmentResponsibilities = ""
				continue
			}
			if duty := extractDepartmentResponsibility(rowText); duty != "" {
				departmentResponsibilities = duty
				continue
			}
			appointment, ok := parseAppointmentRow(row)
			if !ok {
				continue
			}
			appointment.PageNo = page.PageNo
			appointment.Department = department
			appointment.Responsibilities = departmentResponsibilities
			metadata.appointments = append(metadata.appointments, appointment)
			if appointment.Confidence < 0.90 {
				metadata.lowConfidence = true
			}
		}
	}
	for index := range metadata.fields {
		metadata.fields[index].DocumentID = 0
	}
	for index := range metadata.appointments {
		metadata.appointments[index].DocumentID = 0
	}
	return metadata
}

// ocrCell is an OCR line with its position retained. one-ocr emits line boxes
// as [x, y, width, height], which lets us reconstruct table rows before
// interpreting names and roles.
type ocrCell struct {
	text       string
	confidence float32
	x, y       float64
	width      float64
	height     float64
	// tabular คือเซลล์ที่มาจากแถว CSV/XLSX ซึ่ง ocr_worker.py คั่นคอลัมน์ด้วย tab
	tabular bool
}

func groupOCRRows(page workerPage) [][]ocrCell {
	cells := make([]ocrCell, 0, len(page.Lines))
	for index, line := range page.Lines {
		text := strings.TrimSpace(line.Text)
		if text == "" {
			continue
		}
		cell := ocrCell{text: text, confidence: line.Confidence}
		if len(line.BoundingBox) >= 4 {
			cell.x, cell.y, cell.width, cell.height = line.BoundingBox[0], line.BoundingBox[1], line.BoundingBox[2], line.BoundingBox[3]
		} else {
			// Imported structured text may not have coordinates. Keep every line
			// separate so unrelated lines are never merged into one appointment.
			cell.x, cell.y, cell.width, cell.height = 0, float64(index)*1000, 0, 1
			if strings.Contains(text, "\t") {
				// แถวตารางจาก CSV/XLSX: แยกคอลัมน์เป็นเซลล์ในแถวเดียวกัน เรียงตามลำดับคอลัมน์
				for column, value := range strings.Split(text, "\t") {
					if value = strings.TrimSpace(value); value != "" {
						cells = append(cells, ocrCell{text: value, confidence: line.Confidence, x: float64(column), y: cell.y, height: 1, tabular: true})
					}
				}
				continue
			}
		}
		cells = append(cells, cell)
	}
	sort.SliceStable(cells, func(i, j int) bool {
		if cells[i].y == cells[j].y {
			return cells[i].x < cells[j].x
		}
		return cells[i].y < cells[j].y
	})
	rows := make([][]ocrCell, 0)
	for _, cell := range cells {
		placed := false
		for index := range rows {
			row := rows[index]
			rowY := row[0].y
			// Keep the same fixed row tolerance as sample-code. The two test
			// PDFs put the index, name/position, and role in separate OCR boxes
			// whose top edges are commonly 10-18px apart.
			if math.Abs(cell.y-rowY) <= 18.0 {
				rows[index] = append(row, cell)
				placed = true
				break
			}
		}
		if !placed {
			rows = append(rows, []ocrCell{cell})
		}
	}
	for index := range rows {
		sort.SliceStable(rows[index], func(i, j int) bool { return rows[index][i].x < rows[index][j].x })
	}
	return rows
}

func parseAppointmentRow(row []ocrCell) (Appointment, bool) {
	if len(row) == 0 {
		return Appointment{}, false
	}
	if row[0].tabular {
		return parseTabularAppointmentRow(row)
	}
	joined := strings.Join(cellTexts(row), " ")
	normalized := replaceThaiDigits(strings.TrimSpace(joined))
	if !regexp.MustCompile(`^\s*\d+\.\d+(?:[.)])?\s*`).MatchString(normalized) {
		return Appointment{}, false
	}
	role := appointmentRole(normalized)
	hasRole := role != ""
	if role == "" {
		// sample-code keeps an indexed member even when the rightmost role
		// column is missed by OCR, then asks the reviewer to verify it.
		role = "กรรมการ"
	}
	roleIndex := strings.Index(normalized, roleMarker(normalized, role))
	beforeRole := normalized
	if hasRole && roleIndex >= 0 {
		beforeRole = strings.TrimSpace(normalized[:roleIndex])
	}
	beforeRole = regexp.MustCompile(`^\s*\d+\.\d+(?:[.)])?\s*`).ReplaceAllString(beforeRole, "")
	name := choosePersonName(row, beforeRole)
	if name == "" || onlyAppointmentNumber(name) || len([]rune(name)) < 3 {
		return Appointment{}, false
	}
	confidence := float32(1)
	for _, cell := range row {
		confidence = minConfidence(confidence, cell.confidence)
	}
	box := combinedBoundingBox(row)
	return Appointment{FullName: name, Position: choosePersonPosition(row, name), CommitteeRole: role, Confidence: confidence, BoundingBoxJSON: box, NameMatchMethod: "UNMATCHED"}, true
}

var tabularIndexPattern = regexp.MustCompile(`^\d+(?:\.\d+)*[.)]?$`)

// parseTabularAppointmentRow อ่านแถว CSV/XLSX ที่มีคอลัมน์ ลำดับ | ชื่อ | ตำแหน่ง | บทบาท
// ลำดับเป็นเลขจำนวนเต็มได้ (1, 2, 3) เพราะคอลัมน์แยกกันชัดเจน ไม่ปนกับย่อหน้าเนื้อความแบบ OCR
func parseTabularAppointmentRow(row []ocrCell) (Appointment, bool) {
	if len(row) < 2 || !tabularIndexPattern.MatchString(replaceThaiDigits(row[0].text)) {
		return Appointment{}, false
	}
	role := ""
	values := make([]string, 0, len(row)-1)
	nextColumn := row[0].x + 1
	for _, cell := range row[1:] {
		// groupOCRRows ทิ้งเซลล์ว่างแต่เก็บเลขคอลัมน์ไว้ใน x จึงเติมช่องว่างกลับ
		// ไม่ให้ตำแหน่งเลื่อนไปอยู่ในช่องชื่อเมื่อคอลัมน์ชื่อว่าง
		for ; nextColumn < cell.x; nextColumn++ {
			values = append(values, "")
		}
		nextColumn = cell.x + 1
		if role == "" {
			if bare := tabularRole(cell.text); bare != "" {
				role = bare
				continue
			}
		}
		values = append(values, cell.text)
	}
	// คอลัมน์เรียงตามแบบฟอร์มนำเข้า: ชื่อมาก่อนตำแหน่ง ไม่เดาจากคำนำหน้า เพราะ "อาจารย์" เป็นได้ทั้งคำนำหน้าและตำแหน่ง
	name, position := "", ""
	if len(values) > 0 {
		name = values[0]
	}
	if len(values) > 1 {
		position = values[1]
	}
	if name == "" {
		// คอลัมน์ชื่อว่างแต่มีตำแหน่ง คือแต่งตั้งโดยตำแหน่ง เก็บเป็นแถวบทบาทล้วนเหมือนแถวที่ผู้ตรวจเพิ่มเอง
		if position == "" || onlyAppointmentNumber(position) {
			return Appointment{}, false
		}
	} else if len([]rune(name)) < 3 || onlyAppointmentNumber(name) {
		return Appointment{}, false
	}
	if role == "" {
		// เหมือนแถวจาก OCR: ไม่มีคอลัมน์บทบาทก็เก็บไว้เป็นกรรมการ แล้วให้ผู้ตรวจยืนยัน
		role = "กรรมการ"
	}
	confidence := float32(1)
	for _, cell := range row {
		confidence = minConfidence(confidence, cell.confidence)
	}
	return Appointment{FullName: name, Position: position, CommitteeRole: role, Confidence: confidence, BoundingBoxJSON: combinedBoundingBox(row), NameMatchMethod: "UNMATCHED"}, true
}

// tabularRole คืนบทบาทเมื่อทั้งเซลล์เป็นคำบทบาท จะมีคำว่า "เป็น" นำหน้าหรือไม่ก็ได้
func tabularRole(text string) string {
	text = strings.TrimPrefix(strings.TrimSpace(text), "เป็น")
	for _, role := range appointmentRoles() {
		if bare := strings.TrimPrefix(role, "เป็น"); text == bare {
			return bare
		}
	}
	return ""
}

// tabularSectionIndexPattern คือลำดับของหัวกลุ่มในตาราง ซึ่งเป็นเลขจำนวนเต็มล้วน
var tabularSectionIndexPattern = regexp.MustCompile(`^\d+$`)

// parseRowDepartmentHeader ใช้กฎของ parseDepartmentHeader กับทุกแถว และกับแถวตาราง CSV/XLSX/DOCX
// ที่มีแค่ ลำดับ | หัวกลุ่ม (เช่น 1 | คณะกรรมการอำนวยการ) ซึ่งลำดับไม่มีจุด แถวนี้ต้องเป็นหัวฝ่าย ไม่ใช่รายชื่อคน
func parseRowDepartmentHeader(row []ocrCell, rowText string) (string, bool) {
	if name, ok := parseDepartmentHeader(rowText); ok {
		return name, true
	}
	if len(row) != 2 || !row[0].tabular {
		return "", false
	}
	index := strings.TrimRight(replaceThaiDigits(strings.TrimSpace(row[0].text)), ".)")
	if !tabularSectionIndexPattern.MatchString(index) {
		return "", false
	}
	return parseDepartmentHeader(index + ". " + row[1].text)
}

func parseDepartmentHeader(text string) (string, bool) {
	text = strings.TrimSpace(replaceThaiDigits(text))
	match := regexp.MustCompile(`^\s*\d+\.\s*(คณะกรรมการ|คณะทำงาน|ฝ่าย|งาน|ส่วน|หมวด)\s*(.*)$`).FindStringSubmatch(text)
	if len(match) != 3 {
		return "", false
	}
	return strings.TrimSpace(match[1] + " " + match[2]), true
}

func extractDepartmentResponsibility(text string) string {
	text = strings.TrimSpace(text)
	for _, label := range []string{"ภาระหน้าที่", "หน้าที่รับผิดชอบ", "มีหน้าที่รับผิดชอบ", "มีหน้าที่"} {
		if index := strings.Index(text, label); index >= 0 {
			value := strings.TrimSpace(strings.Trim(text[index+len(label):], " :-"))
			if value != "" {
				return value
			}
		}
	}
	return ""
}

func cellTexts(row []ocrCell) []string {
	texts := make([]string, 0, len(row))
	for _, cell := range row {
		texts = append(texts, cell.text)
	}
	return texts
}

func choosePersonName(row []ocrCell, beforeRole string) string {
	for _, cell := range row {
		candidate := cleanAppointmentName(cell.text)
		if hasThaiPersonPrefix(candidate) {
			return candidate
		}
	}
	return cleanAppointmentName(beforeRole)
}

func choosePersonPosition(row []ocrCell, name string) string {
	for _, cell := range row {
		candidate := cleanAppointmentName(cell.text)
		if candidate != "" && candidate != name && !hasThaiPersonPrefix(candidate) && appointmentRole(cell.text) == "" && !strings.Contains(candidate, "หน้าที่") {
			return candidate
		}
	}
	return ""
}

func hasThaiPersonPrefix(text string) bool {
	text = strings.TrimSpace(text)
	for _, prefix := range []string{"นาย", "นางสาว", "นาง", "ดร.", "รศ.", "ผศ.", "ศ.", "อาจารย์"} {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

func combinedBoundingBox(row []ocrCell) string {
	if len(row) == 0 {
		return "[]"
	}
	left, top := row[0].x, row[0].y
	right, bottom := row[0].x+row[0].width, row[0].y+row[0].height
	for _, cell := range row[1:] {
		left, top = math.Min(left, cell.x), math.Min(top, cell.y)
		right, bottom = math.Max(right, cell.x+cell.width), math.Max(bottom, cell.y+cell.height)
	}
	box, _ := json.Marshal([]float64{left, top, right - left, bottom - top})
	return string(box)
}

func findLineBoundingBox(pages []workerPage, value string) string {
	value = normalizeOCRSearchText(value)
	if value == "" {
		return "[]"
	}
	for _, page := range pages {
		for _, line := range page.Lines {
			text := normalizeOCRSearchText(line.Text)
			if strings.Contains(text, value) || strings.Contains(value, text) {
				bbox, err := json.Marshal(line.BoundingBox)
				if err == nil {
					return string(bbox)
				}
			}
		}
	}
	return "[]"
}

func normalizeOCRSearchText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func confidenceFor(value string) float32 {
	if strings.TrimSpace(value) == "" || value == "0" {
		return 0
	}
	return 0.94
}

func inferOrderType(text string) string {
	for _, candidate := range []string{"คำสั่ง", "ประกาศ", "ระเบียบ", "บันทึกข้อความ"} {
		if strings.Contains(text, candidate) {
			return candidate
		}
	}
	return "เอกสารคำสั่ง"
}

func averageOCRConfidence(pages []workerPage) float32 {
	var total float32
	count := 0
	for _, page := range pages {
		for _, line := range page.Lines {
			total += line.Confidence
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / float32(count)
}

func appointmentRole(text string) string {
	for _, role := range appointmentRoles() {
		if strings.Contains(text, role) {
			return strings.TrimPrefix(role, "เป็น")
		}
	}
	return ""
}

func appointmentRoles() []string {
	return []string{"เป็นกรรมการและผู้ช่วยเลขานุการ", "เป็นกรรมการและเลขานุการ", "เป็นรองประธานกรรมการ", "เป็นประธานกรรมการ", "เป็นผู้ช่วยเลขานุการ", "เป็นเลขานุการ", "เป็นกรรมการ", "เป็นที่ปรึกษา", "เป็นเหรัญญิก"}
}

func roleMarker(text, role string) string {
	marker := "เป็น" + role
	if strings.Contains(text, marker) {
		return marker
	}
	return role
}

func cleanAppointmentName(text string) string {
	text = replaceThaiDigits(text)
	name := regexp.MustCompile(`^\s*\d+(?:\.\d+)*[.)]?\s*`).ReplaceAllString(strings.TrimSpace(text), "")
	name = strings.Trim(name, " -:.")
	if appointmentRole(name) != "" || strings.Contains(name, "หน้าที่รับผิดชอบ") || strings.HasPrefix(name, "/") {
		return ""
	}
	return name
}

func onlyAppointmentNumber(text string) bool {
	return regexp.MustCompile(`^\d+(?:\.\d+)*$`).MatchString(strings.TrimSpace(replaceThaiDigits(text)))
}

func minConfidence(values ...float32) float32 {
	minimum := float32(1)
	for _, value := range values {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func findOrderNo(text string) string {
	text = replaceThaiDigits(text)
	re := regexp.MustCompile(`(?i)(?:เลขที่คำสั่ง|ที่)\s*([\pL\pN./\-\s]{1,30}\d+\s*/\s*(?:25|20)\d{2})`)
	match := re.FindStringSubmatch(text)
	if len(match) < 2 {
		return ""
	}
	return strings.Join(strings.Fields(strings.Trim(match[1], " .")), " ")
}

func findOrderYear(orderNo string) int {
	orderNo = replaceThaiDigits(orderNo)
	re := regexp.MustCompile(`(?:25|20)\d{2}`)
	year, _ := strconv.Atoi(re.FindString(orderNo))
	return year
}

func extractAfterLabel(text, label string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "\t") {
			if value := tabularValueAfterLabel(line, label); value != "" {
				return value
			}
			continue
		}
		if index := strings.Index(line, label); index >= 0 {
			value := strings.TrimSpace(strings.Trim(line[index+len(label):], " :-"))
			if value != "" {
				return value
			}
		}
	}
	return ""
}

// tabularValueAfterLabel อ่านค่าจากแถวตาราง: ค่าในเซลล์เดียวกับ label หรือเซลล์ถัดไป
// เมื่อแถวมีแค่สองเซลล์ (ป้ายกับค่า) แถวหัวตารางอย่าง ลำดับ | ชื่อ | ตำแหน่ง | บทบาท จึงไม่ถูกอ่านเป็นค่า
func tabularValueAfterLabel(line, label string) string {
	cells := make([]string, 0)
	for _, cell := range strings.Split(line, "\t") {
		if cell = strings.TrimSpace(cell); cell != "" {
			cells = append(cells, cell)
		}
	}
	for index, cell := range cells {
		position := strings.Index(cell, label)
		if position < 0 {
			continue
		}
		if value := strings.TrimSpace(strings.Trim(cell[position+len(label):], " :-")); value != "" {
			return value
		}
		if len(cells) == 2 && index == 0 {
			return cells[1]
		}
	}
	return ""
}

func extractCommandTitle(pages []workerPage, fallbackText string) string {
	lines := make([]string, 0)
	if len(pages) > 0 {
		lines = strings.Split(pages[0].FullText, "\n")
	} else {
		lines = strings.Split(fallbackText, "\n")
	}
	collecting := false
	parts := make([]string, 0, 3)
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		// แถวตารางจาก CSV/XLSX: บรรทัดเรื่องอาจอยู่ในเซลล์ แต่แถวตารางถัดไปไม่ใช่ส่วนของชื่อเรื่อง
		tabular := strings.Contains(line, "\t")
		if tabular {
			if collecting {
				break
			}
			line = strings.Join(strings.Fields(line), " ")
		}
		if !collecting {
			if index := strings.Index(line, "เรื่อง"); index >= 0 {
				line = strings.TrimSpace(strings.Trim(line[index+len("เรื่อง"):], " :-"))
				collecting = true
			} else {
				continue
			}
		}
		if line == "" {
			continue
		}
		if len(parts) > 0 && isCommandBodyMarker(line) {
			break
		}
		parts = append(parts, line)
		if len(parts) >= 4 {
			break
		}
	}
	return strings.Join(parts, " ")
}

func isCommandBodyMarker(line string) bool {
	line = replaceThaiDigits(strings.TrimSpace(line))
	if regexp.MustCompile(`^\d+\.\d+`).MatchString(line) {
		return true
	}
	for _, marker := range []string{"เพื่อให้", "เนื่องด้วย", "ด้วย", "อาศัยอำนาจ", "จึงแต่งตั้ง", "1.", "1)"} {
		if strings.HasPrefix(line, marker) {
			return true
		}
	}
	return false
}

func extractSigner(text string) string {
	for _, label := range []string{"ลงชื่อ", "ผู้ลงนาม", "ลงนาม"} {
		if value := extractAfterLabel(text, label); value != "" {
			return value
		}
	}
	return ""
}

func extractSignerPosition(text string) string {
	for _, label := range []string{"ตำแหน่งผู้ลงนาม", "ตำแหน่ง", "ในตำแหน่ง"} {
		if value := extractAfterLabel(text, label); value != "" {
			return value
		}
	}
	return ""
}

func findDateAfterLabels(text string, labels ...string) *time.Time {
	for _, line := range strings.Split(text, "\n") {
		for _, label := range labels {
			if strings.Contains(line, label) {
				if parsed := parseThaiDate(line); parsed != nil {
					return parsed
				}
			}
		}
	}
	return nil
}

func parseThaiDate(text string) *time.Time {
	parsed, _ := normalizeThaiDate(text)
	return parsed
}

func replaceThaiDigits(text string) string {
	replacer := strings.NewReplacer("๐", "0", "๑", "1", "๒", "2", "๓", "3", "๔", "4", "๕", "5", "๖", "6", "๗", "7", "๘", "8", "๙", "9")
	return replacer.Replace(text)
}

func formatDate(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("2006-01-02")
}

func findPage(pages []workerPage, value string) int {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	for _, page := range pages {
		if strings.Contains(page.FullText, value) {
			return page.PageNo
		}
	}
	return 1
}

func chooseNonEmpty(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

func detectPDFPageCount(ctx context.Context, path string) int {
	output, err := exec.CommandContext(ctx, getenv("PDFINFO_BIN", "pdfinfo"), path).Output()
	if err != nil {
		return 0
	}
	match := regexp.MustCompile(`(?m)^Pages:\s+(\d+)`).FindSubmatch(output)
	if len(match) != 2 {
		return 0
	}
	count, _ := strconv.Atoi(string(match[1]))
	return count
}

func getenvInt(key string, fallback int) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return strconv.Itoa(fallback)
}

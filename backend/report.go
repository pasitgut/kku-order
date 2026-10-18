package main

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const reportTitle = "สรุปรายงานการออกคำสั่งแต่งตั้งคณะกรรมการดำเนินงานต่างๆ ภายในวิทยาลัยการคอมพิวเตอร์ สังกัด วิทยาลัยการคอมพิวเตอร์ มหาวิทยาลัยขอนแก่น"

// reportColumns หัวตารางตามไฟล์ตัวอย่าง ~/kku/รูปแบบรายงาน * (10 คอลัมน์) ความกว้างเป็นสัดส่วนจากไฟล์ Excel ตัวอย่าง
var reportColumns = []struct {
	Title string
	Width float64
}{
	{"ลำดับที่", 5.4}, {"ชื่อ-นามสกุล", 15.2}, {"ประเภทคำสั่ง", 17.6}, {"เลขที่คำสั่ง", 7.6}, {"เรื่อง", 15.5},
	{"ภาระ/หน้าที่ ที่ได้รับมอบหมาย", 12.5}, {"วันที่ออกคำสั่ง", 10}, {"วันเดือนปีที่มีผล", 15.9}, {"วันที่ครบวาระ", 9.6}, {"หมายเหตุ", 12.4},
}

type reportFilter struct {
	PersonID   uint
	From, To   *time.Time
	ActiveOnly bool
}

// reportRow หนึ่งแถว = หนึ่งคนในหนึ่งคำสั่ง ทุกช่องเป็นข้อความพร้อมพิมพ์
type reportRow struct {
	No        int
	Name      string
	OrderType string
	OrderNo   string
	Subject   string
	Duty      string
	Issued    string
	Effective string
	Expiry    string
	Note      string
}

func (row reportRow) cells() []string {
	return []string{strconv.Itoa(row.No), row.Name, row.OrderType, row.OrderNo, row.Subject, row.Duty, row.Issued, row.Effective, row.Expiry, row.Note}
}

type appointmentReport struct {
	Title      string
	Scope      string
	PersonName string // ว่างถ้าเป็นรายงานของทุกคน
	Rows       []reportRow
}

func dashIfEmpty(value string) string {
	if value = strings.TrimSpace(value); value == "" {
		return "-"
	}
	return value
}

func thaiDateOrDash(value *time.Time) string {
	return dashIfEmpty(thaiDateText(value))
}

// reportDuty: ใช้หน้าที่ของฝ่ายตามคำสั่ง ถ้าคำสั่งไม่ได้ระบุ ใช้บทบาทที่ถูกแต่งตั้ง เช่น "เป็นกรรมการและเลขานุการ"
func reportDuty(responsibilities, role string) string {
	if duty := strings.TrimSpace(responsibilities); duty != "" {
		return duty
	}
	role = strings.TrimSpace(role)
	if role == "" {
		return ""
	}
	if !strings.HasPrefix(role, "เป็น") {
		role = "เป็น" + role
	}
	return role
}

func reportEffective(effectiveText string, effective *time.Time) string {
	if isFromNow(effectiveText) {
		return "ทั้งนี้ ตั้งแต่บัดนี้เป็นต้นไป"
	}
	if effective == nil {
		return "-"
	}
	return "ทั้งนี้ ตั้งแต่วันที่ " + thaiDateText(effective) + " เป็นต้นไป"
}

func reportNote(status, reason, replacedBy string) string {
	if status != documentCancelled {
		return "-"
	}
	if replacedBy = strings.TrimSpace(replacedBy); replacedBy != "" {
		note := "ยกเลิก ใช้คำสั่ง " + replacedBy + " แทน"
		// เหตุผลที่ผู้ใช้พิมพ์เองแสดงต่อท้าย ส่วนข้อความที่ระบบเขียนแทนเหตุผลไม่ต้องซ้ำ
		if reason = strings.TrimSpace(reason); reason != "" && reason != "ใช้คำสั่งเลขที่ "+replacedBy+" แทน" {
			note += " (" + reason + ")"
		}
		return note
	}
	if reason = strings.TrimSpace(reason); reason != "" {
		return "ยกเลิก: " + reason
	}
	return "ยกเลิก"
}

// orderNumber อ่านเลขลำดับหน้าเครื่องหมาย / ของเลขที่คำสั่ง ให้ "3/2569" มาก่อน "12/2569"
func orderNumber(orderNo string) (int, bool) {
	text := replaceThaiDigits(strings.TrimSpace(orderNo))
	end := 0
	for end < len(text) && text[end] >= '0' && text[end] <= '9' {
		end++
	}
	number, err := strconv.Atoi(text[:end])
	return number, err == nil
}

func orderNoLess(left, right string) bool {
	l, lok := orderNumber(left)
	r, rok := orderNumber(right)
	if lok && rok && l != r {
		return l < r
	}
	return left < right
}

func reportScope(name string, filter reportFilter) string {
	parts := []string{}
	if name != "" {
		parts = append(parts, "ของ "+name)
	}
	switch {
	case filter.From != nil && filter.To != nil:
		parts = append(parts, "คำสั่งที่ออกระหว่าง "+thaiDateText(filter.From)+" ถึง "+thaiDateText(filter.To))
	case filter.From != nil:
		parts = append(parts, "คำสั่งที่ออกตั้งแต่ "+thaiDateText(filter.From))
	case filter.To != nil:
		parts = append(parts, "คำสั่งที่ออกถึง "+thaiDateText(filter.To))
	}
	if filter.ActiveOnly {
		parts = append(parts, "เฉพาะที่ยังมีผล")
	}
	if len(parts) == 0 {
		return "คำสั่งทั้งหมดที่ยืนยันแล้ว"
	}
	return strings.Join(parts, " · ")
}

// buildAppointmentReport รวมแถวรายงานจากคำสั่งที่ยืนยันแล้วและที่ยกเลิกหลังยืนยัน ตามตัวกรองบนหน้าค้นหา
// เรียงตามวันที่ออกคำสั่ง แล้วตามเลขที่คำสั่ง (filter.To คือวันสุดท้ายที่รวมด้วย)
func buildAppointmentReport(db *gorm.DB, filter reportFilter, today time.Time) (appointmentReport, error) {
	query := db.Table("appointments AS a").
		Select(`a.id, a.directory_user_id, a.full_name, a.position, a.committee_role, a.responsibilities,
			d.id AS document_id, d.order_no, d.title, d.issuing_unit, d.issued_date, d.effective_date, d.expiry_date,
			d.status, d.cancellation_reason, d.replaced_by_order_no, COALESCE(ef.value, '') AS effective_text`).
		Joins("JOIN document_workflows AS d ON d.id = a.document_id").
		Joins("LEFT JOIN extracted_fields AS ef ON ef.document_id = d.id AND ef.field_key = 'effective_date'").
		Where(historyDocuments)
	if filter.PersonID != 0 {
		query = query.Where("a.directory_user_id = ?", filter.PersonID)
	}
	if filter.From != nil {
		query = query.Where("d.issued_date >= ?", dateOnly(*filter.From))
	}
	if filter.To != nil {
		query = query.Where("d.issued_date < ?", dayAfter(*filter.To))
	}
	var rows []struct {
		ID                 uint
		DirectoryUserID    *uint
		FullName           string
		Position           string
		CommitteeRole      string
		Responsibilities   string
		DocumentID         uint
		OrderNo            string
		Title              string
		IssuingUnit        string
		IssuedDate         *time.Time
		EffectiveDate      *time.Time
		ExpiryDate         *time.Time
		Status             string
		CancellationReason string
		ReplacedByOrderNo  string
		EffectiveText      string
	}
	if err := query.Order("a.id").Scan(&rows).Error; err != nil {
		return appointmentReport{}, err
	}

	unique := map[uint]bool{}
	if filter.PersonID != 0 {
		unique[filter.PersonID] = true
	}
	for _, row := range rows {
		if row.DirectoryUserID != nil {
			unique[*row.DirectoryUserID] = true
		}
	}
	ids := make([]uint, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	names := map[uint]string{}
	if len(ids) > 0 {
		people, err := searchPeople(db, db.Where("id IN ?", ids))
		if err != nil {
			return appointmentReport{}, err
		}
		for _, person := range people {
			names[person.ID] = officialName(person)
		}
	}

	type entry struct {
		row    reportRow
		issued *time.Time
		first  uint
		duties []string
	}
	entries := []*entry{}
	byKey := map[string]*entry{}
	for _, row := range rows {
		if filter.ActiveOnly {
			if state := appointmentState(row.Status, row.ExpiryDate, today); state != appointmentActive && state != appointmentExpiring {
				continue
			}
		}
		name, key := strings.TrimSpace(row.FullName), ""
		if row.DirectoryUserID != nil && names[*row.DirectoryUserID] != "" {
			name, key = names[*row.DirectoryUserID], "u"+strconv.FormatUint(uint64(*row.DirectoryUserID), 10)
		}
		if name == "" {
			// แถวโดยตำแหน่งที่ยังไม่ผูกคน แสดงตำแหน่งแทนชื่อ
			name, _ = splitExOfficioPosition(row.Position)
		}
		if key == "" {
			key = "n" + name
			if name == "" {
				// แถวที่ไม่มีทั้งชื่อและตำแหน่ง ไม่รู้ว่าเป็นใคร จึงไม่รวมกับแถวอื่น
				key = "a" + strconv.FormatUint(uint64(row.ID), 10)
			}
		}
		key = strconv.FormatUint(uint64(row.DocumentID), 10) + "|" + key
		duty := reportDuty(row.Responsibilities, row.CommitteeRole)
		// คนเดียวมีหลายบทบาทในคำสั่งเดียว รวมไว้แถวเดียว
		if existing, ok := byKey[key]; ok {
			if duty != "" && !containsString(existing.duties, duty) {
				existing.duties = append(existing.duties, duty)
			}
			continue
		}
		item := &entry{issued: row.IssuedDate, first: row.ID, row: reportRow{
			Name: dashIfEmpty(name), OrderType: dashIfEmpty(row.IssuingUnit), OrderNo: dashIfEmpty(row.OrderNo), Subject: dashIfEmpty(row.Title),
			Issued: thaiDateOrDash(row.IssuedDate), Effective: reportEffective(row.EffectiveText, row.EffectiveDate), Expiry: thaiDateOrDash(row.ExpiryDate),
			Note: reportNote(row.Status, row.CancellationReason, row.ReplacedByOrderNo),
		}}
		if duty != "" {
			item.duties = []string{duty}
		}
		byKey[key] = item
		entries = append(entries, item)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		left, right := entries[i], entries[j]
		switch {
		case left.issued == nil || right.issued == nil:
			if (left.issued == nil) != (right.issued == nil) {
				return right.issued == nil
			}
		case !left.issued.Equal(*right.issued):
			return left.issued.Before(*right.issued)
		}
		if left.row.OrderNo != right.row.OrderNo {
			return orderNoLess(left.row.OrderNo, right.row.OrderNo)
		}
		return left.first < right.first
	})
	report := appointmentReport{Title: reportTitle, Scope: reportScope(names[filter.PersonID], filter), PersonName: names[filter.PersonID], Rows: make([]reportRow, 0, len(entries))}
	for i, item := range entries {
		item.row.No = i + 1
		item.row.Duty = dashIfEmpty(strings.Join(item.duties, "\n"))
		report.Rows = append(report.Rows, item.row)
	}
	return report, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

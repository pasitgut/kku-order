package main

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// สถานะของการแต่งตั้งหนึ่งรายการ ในมุมของคนที่ได้รับแต่งตั้ง (หน้าค้นหาและรายงาน)
const (
	appointmentActive    = "ACTIVE"
	appointmentExpiring  = "EXPIRING"
	appointmentExpired   = "EXPIRED"
	appointmentCancelled = "CANCELLED"

	suggestionLimit = 6
	topPeopleLimit  = 8
)

type personSummary struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Position string `json:"position"`
	Orders   int    `json:"orders"`
}

type personCard struct {
	personSummary
	Active   int `json:"active"`
	Expiring int `json:"expiring"`
}

type historyRow struct {
	DocumentID uint       `json:"documentId"`
	OrderNo    string     `json:"orderNo"`
	Title      string     `json:"title"`
	Role       string     `json:"role"`
	IssuedDate *time.Time `json:"issuedDate"`
	ExpiryDate *time.Time `json:"expiryDate"`
	State      string     `json:"state"`
}

type personHistory struct {
	Person       personCard   `json:"person"`
	Appointments []historyRow `json:"appointments"`
}

type orderSuggestion struct {
	ID         uint       `json:"id"`
	OrderNo    string     `json:"orderNo"`
	Title      string     `json:"title"`
	IssuedDate *time.Time `json:"issuedDate"`
	Status     string     `json:"status"`
}

// nameSummary คือชื่อที่เขียนอยู่ในคำสั่งแต่ยังไม่ได้ผูกกับบุคคลในระบบ (เช่น อาจารย์ต่างคณะ)
type nameSummary struct {
	Name     string `json:"name"`
	Position string `json:"position"`
	Orders   int    `json:"orders"`
}

type searchSuggestions struct {
	People []personSummary   `json:"people"`
	Names  []nameSummary     `json:"names"`
	Orders []orderSuggestion `json:"orders"`
}

type unlinkedAppointment struct {
	ID         uint
	DocumentID uint
	FullName   string
	Position   string
	Status     string
}

// nameProbe คือคำแรกของชื่อที่พิมพ์ (ตัดคำนำหน้าแล้ว) ใช้กรองหยาบใน SQL ก่อนเทียบจริงด้วย normalizePersonName
// ใช้คำเดียวเพราะ SQL ตัดช่องว่างพิเศษหรือเครื่องหมายจาก OCR แบบเดียวกับ Go ไม่ได้
func nameProbe(text string) string {
	words := strings.Fields(stripThaiNamePrefixes(text))
	if len(words) == 0 {
		return ""
	}
	return strings.TrimFunc(words[0], func(r rune) bool { return unicode.IsPunct(r) })
}

// unlinkedNameRows หาแถวรายชื่อที่ยังไม่ผูกคนในคำสั่งที่นับเป็นประวัติ เทียบชื่อแบบตัดคำนำหน้า ช่องว่าง และเครื่องหมาย
// exact = ชื่อต้องตรงทั้งชื่อ ไม่งั้นแค่มีคำค้นอยู่ในชื่อ เรียงแถวใหม่ก่อน ตำแหน่งที่แสดงจึงมาจากคำสั่งล่าสุด
func unlinkedNameRows(db *gorm.DB, text string, exact bool) ([]unlinkedAppointment, error) {
	key, probe := normalizePersonName(text), nameProbe(text)
	if key == "" || probe == "" {
		return nil, nil
	}
	var rows []unlinkedAppointment
	query := db.Table("appointments AS a").Select("a.id, a.document_id, a.full_name, a.position, d.status").
		Joins("JOIN document_workflows AS d ON d.id = a.document_id").
		Where("a.directory_user_id IS NULL AND TRIM(COALESCE(a.full_name, '')) <> '' AND "+historyDocuments).
		Where("a.full_name LIKE ? ESCAPE '!'", "%"+auditLikeEscaper.Replace(probe)+"%").
		Order("a.id DESC")
	if !exact {
		// ช่องค้นหาแค่ต้องการรายการแนะนำ ไม่ต้องอ่านทุกแถว
		query = query.Limit(2000)
	}
	err := query.Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	matched := rows[:0]
	for _, row := range rows {
		name := normalizePersonName(row.FullName)
		if name == key || (!exact && strings.Contains(name, key)) {
			matched = append(matched, row)
		}
	}
	return matched, nil
}

// summarizeUnlinked รวมแถวของชื่อเดียวกันเป็นหนึ่งรายการ นับเฉพาะคำสั่งที่ยืนยันแล้วเหมือนการนับของคนในระบบ
func summarizeUnlinked(rows []unlinkedAppointment) []nameSummary {
	order := []string{}
	byKey := map[string]*nameSummary{}
	documents := map[string]map[uint]bool{}
	for _, row := range rows {
		key := normalizePersonName(row.FullName)
		item, ok := byKey[key]
		if !ok {
			item = &nameSummary{Name: strings.Join(strings.Fields(row.FullName), " ")}
			byKey[key], documents[key] = item, map[uint]bool{}
			order = append(order, key)
		}
		if item.Position == "" {
			item.Position = strings.TrimSpace(row.Position)
		}
		if row.Status == "CONFIRMED" && !documents[key][row.DocumentID] {
			documents[key][row.DocumentID] = true
			item.Orders++
		}
	}
	summaries := make([]nameSummary, 0, len(order))
	for _, key := range order {
		summaries = append(summaries, *byKey[key])
	}
	sort.SliceStable(summaries, func(i, j int) bool { return summaries[i].Orders > summaries[j].Orders })
	return summaries
}

func dateOnly(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.Local)
}

// appointmentState: คำสั่งยกเลิกแล้วถือว่ายกเลิก ไม่มีวันครบวาระถือว่ายังมีผล
func appointmentState(documentStatus string, expiry *time.Time, today time.Time) string {
	if documentStatus == documentCancelled {
		return appointmentCancelled
	}
	if expiry == nil {
		return appointmentActive
	}
	end, start := dateOnly(*expiry), dateOnly(today)
	switch {
	case end.Before(start):
		return appointmentExpired
	case !end.After(start.AddDate(0, expiryNoticeMonths[0], 0)):
		return appointmentExpiring
	}
	return appointmentActive
}

// searchPeople โหลดคนตามเงื่อนไขพร้อมโปรไฟล์ รวมคนที่ออกไปแล้ว เพราะประวัติการแต่งตั้งเก่ายังต้องค้นเจอ
func searchPeople(db *gorm.DB, query *gorm.DB) ([]DirectoryUser, error) {
	var people []DirectoryPerson
	if err := query.Find(&people).Error; err != nil {
		return nil, err
	}
	profiles, err := loadDirectoryProfiles(db, people)
	if err != nil {
		return nil, err
	}
	users := make([]DirectoryUser, 0, len(people))
	for _, person := range people {
		users = append(users, directoryPersonAPI(person, profiles[person.ID]))
	}
	return users, nil
}

// historyDocuments คือคำสั่งที่นับเป็นประวัติ: ยืนยันแล้ว หรือยกเลิกหลังจากเคยยืนยัน
// (เอกสารที่ยกเลิกตั้งแต่ยังไม่มีใครตรวจ รายชื่อยังไม่ได้ยืนยัน จึงไม่นับ)
const historyDocuments = "(d.status = 'CONFIRMED' OR (d.status = '" + documentCancelled + "' AND d.confirmed_at IS NOT NULL))"

func personPosition(user DirectoryUser) string {
	for _, value := range []string{user.ManagePosition, user.PositionTitle, user.JobTitle} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func personName(user DirectoryUser) string {
	return strings.TrimSpace(user.Prefix + " " + displayDirectoryName(user))
}

// officialName เขียนคำนำหน้าภาษาไทยติดชื่อแบบหนังสือราชการ ("นางสาวจิราภรณ์ หอมอ่อน" "ผศ.ดร.สมชาย ใจดี")
// คำนำหน้าภาษาอังกฤษ ("Dr.") เว้นวรรคตามปกติ
func officialName(user DirectoryUser) string {
	prefix := strings.TrimSpace(user.Prefix)
	if prefix == "" || !containsThai(prefix) {
		return personName(user)
	}
	return prefix + displayDirectoryName(user)
}

func containsThai(text string) bool {
	for _, r := range text {
		if r >= 0x0E00 && r <= 0x0E7F {
			return true
		}
	}
	return false
}

// requestedDates อ่าน from / to ที่หน้าค้นหาส่งมา (to คือวันสุดท้ายที่รวมด้วย) ค่าที่อ่านไม่ได้ถือว่าไม่กรอง
// ใช้ทั้งประวัติบนหน้าจอและรายงานที่ส่งออก ช่วงวันที่จึงตรงกันเสมอ
func requestedDates(c *gin.Context) (from, to *time.Time) {
	if parsed, ok := normalizeThaiDate(c.Query("from")); ok {
		from = parsed
	}
	if parsed, ok := normalizeThaiDate(c.Query("to")); ok {
		to = parsed
	}
	return from, to
}

// dayAfter คือขอบบน (ไม่รวม) ของช่วงที่รวมวัน last ทั้งวัน
func dayAfter(last time.Time) time.Time {
	return dateOnly(last).AddDate(0, 0, 1)
}

// issuedRange แปลงช่วงจากหน้าค้นหาเป็นขอบบนแบบไม่รวม สำหรับเทียบกับ issued_date
func issuedRange(c *gin.Context) (from, to *time.Time) {
	from, last := requestedDates(c)
	if last != nil {
		end := dayAfter(*last)
		to = &end
	}
	return from, to
}

func withinRange(issued, from, to *time.Time) bool {
	if from == nil && to == nil {
		return true
	}
	if issued == nil {
		return false
	}
	return (from == nil || !issued.Before(*from)) && (to == nil || issued.Before(*to))
}

// confirmedOrderCounts นับคำสั่งที่ยืนยันแล้วต่อคน ids ว่างคือนับทุกคน
func confirmedOrderCounts(db *gorm.DB, ids []uint, from, to *time.Time) (map[uint]int, error) {
	query := db.Table("appointments AS a").Select("a.directory_user_id AS person_id, COUNT(DISTINCT a.document_id) AS orders").
		Joins("JOIN document_workflows AS d ON d.id = a.document_id").
		Where("d.status = ? AND a.directory_user_id IS NOT NULL", "CONFIRMED").Group("a.directory_user_id")
	if len(ids) > 0 {
		query = query.Where("a.directory_user_id IN ?", ids)
	}
	if from != nil {
		query = query.Where("d.issued_date >= ?", *from)
	}
	if to != nil {
		query = query.Where("d.issued_date < ?", *to)
	}
	var rows []struct {
		PersonID uint
		Orders   int
	}
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := make(map[uint]int, len(rows))
	for _, row := range rows {
		counts[row.PersonID] = row.Orders
	}
	return counts, nil
}

func sortByOrders(people []personSummary) {
	sort.SliceStable(people, func(i, j int) bool {
		if people[i].Orders != people[j].Orders {
			return people[i].Orders > people[j].Orders
		}
		return people[i].Name < people[j].Name
	})
}

func searchSuggestHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		result := searchSuggestions{People: []personSummary{}, Names: []nameSummary{}, Orders: []orderSuggestion{}}
		text := strings.TrimSpace(c.Query("q"))
		if text == "" {
			c.JSON(http.StatusOK, gin.H{"data": result})
			return
		}
		// เทียบแบบตัดคำนำหน้า ตำแหน่งวิชาการ และช่องว่างออก พิมพ์ "ผศ.สมชาย" ก็เจอ "สมชาย ใจดี"
		key := normalizePersonName(text)
		matched := []DirectoryUser{}
		if key != "" {
			pattern := "%" + auditLikeEscaper.Replace(key) + "%"
			var err error
			matched, err = searchPeople(db, db.Where("REPLACE(CONCAT(first_name, last_name), ' ', '') LIKE ? ESCAPE '!'", pattern).Limit(50))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถค้นหารายชื่อได้"})
				return
			}
		}
		ids := make([]uint, 0, len(matched))
		for _, user := range matched {
			ids = append(ids, user.ID)
		}
		counts := map[uint]int{}
		if len(ids) > 0 {
			var err error
			if counts, err = confirmedOrderCounts(db, ids, nil, nil); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถนับคำสั่งได้"})
				return
			}
		}
		// ชื่อที่ตรงทั้งหมดมาก่อน ตามด้วยชื่อที่ขึ้นต้นด้วยคำค้น แล้วจึงเรียงตามจำนวนคำสั่ง
		rank := map[uint]int{}
		for _, user := range matched {
			name := directoryUserKey(user)
			switch {
			case name == key:
				rank[user.ID] = 0
			case strings.HasPrefix(name, key):
				rank[user.ID] = 1
			default:
				rank[user.ID] = 2
			}
			result.People = append(result.People, personSummary{ID: user.ID, Name: personName(user), Position: personPosition(user), Orders: counts[user.ID]})
		}
		sortByOrders(result.People)
		sort.SliceStable(result.People, func(i, j int) bool { return rank[result.People[i].ID] < rank[result.People[j].ID] })
		result.People = result.People[:minInt(len(result.People), suggestionLimit)]

		unlinked, err := unlinkedNameRows(db, text, false)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถค้นหาชื่อในคำสั่งได้"})
			return
		}
		// ชื่อที่ตรงกับคนในรายการ "คน" อยู่แล้วไม่ต้องแสดงซ้ำ
		known := map[string]bool{}
		for _, user := range matched {
			known[directoryUserKey(user)] = true
		}
		for _, name := range summarizeUnlinked(unlinked) {
			if !known[normalizePersonName(name.Name)] && len(result.Names) < suggestionLimit {
				result.Names = append(result.Names, name)
			}
		}

		like := "%" + auditLikeEscaper.Replace(text) + "%"
		var documents []Document
		if err := db.Select("id", "order_no", "title", "issued_date", "status").
			Where("order_no LIKE ? ESCAPE '!' OR title LIKE ? ESCAPE '!' OR committee LIKE ? ESCAPE '!'", like, like, like).
			Order("issued_date IS NULL, issued_date DESC, id DESC").Limit(suggestionLimit).Find(&documents).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถค้นหาคำสั่งได้"})
			return
		}
		for _, document := range documents {
			result.Orders = append(result.Orders, orderSuggestion{ID: document.ID, OrderNo: document.OrderNo, Title: document.Title, IssuedDate: document.IssuedDate, Status: document.Status})
		}
		c.JSON(http.StatusOK, gin.H{"data": result})
	}
}

// topPeopleHandler คนที่ได้รับแต่งตั้งมากที่สุดในช่วงวันที่ที่ส่งมา (หน้าเว็บส่งปีงบประมาณปัจจุบัน)
func topPeopleHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		from, to := issuedRange(c)
		counts, err := confirmedOrderCounts(db, nil, from, to)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถนับคำสั่งได้"})
			return
		}
		ids := make([]uint, 0, len(counts))
		for id := range counts {
			ids = append(ids, id)
		}
		users := []DirectoryUser{}
		if len(ids) > 0 {
			if users, err = searchPeople(db, db.Where("id IN ?", ids)); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดรายชื่อได้"})
				return
			}
		}
		people := []personSummary{}
		for _, user := range users {
			if counts[user.ID] > 0 {
				people = append(people, personSummary{ID: user.ID, Name: personName(user), Position: personPosition(user), Orders: counts[user.ID]})
			}
		}
		sortByOrders(people)
		c.JSON(http.StatusOK, gin.H{"data": people[:minInt(len(people), topPeopleLimit)]})
	}
}

// loadPersonHistory คืนคำสั่งที่ยืนยันแล้วและยกเลิกแล้วของคนคนนี้ เรียงจากใหม่ไปเก่า หนึ่งแถวต่อหนึ่งคำสั่ง
func loadPersonHistory(db *gorm.DB, personID uint, today time.Time) ([]historyRow, error) {
	return loadHistory(db, today, "a.directory_user_id = ?", personID)
}

// loadHistory คืนคำสั่งของแถวรายชื่อที่ตรงเงื่อนไข เรียงจากใหม่ไปเก่า หนึ่งแถวต่อหนึ่งคำสั่ง
func loadHistory(db *gorm.DB, today time.Time, condition string, args ...any) ([]historyRow, error) {
	var rows []struct {
		DocumentID    uint
		OrderNo       string
		Title         string
		Status        string
		IssuedDate    *time.Time
		ExpiryDate    *time.Time
		CommitteeRole string
		Position      string
	}
	err := db.Table("appointments AS a").
		Select("d.id AS document_id, d.order_no, d.title, d.status, d.issued_date, d.expiry_date, a.committee_role, a.position").
		Joins("JOIN document_workflows AS d ON d.id = a.document_id").
		Where(historyDocuments).Where(condition, args...).
		Order("d.issued_date IS NULL, d.issued_date DESC, d.order_no DESC, a.id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	history := []historyRow{}
	index := map[uint]int{}
	roles := map[uint]map[string]bool{}
	for _, row := range rows {
		role := strings.TrimSpace(row.CommitteeRole)
		if role == "" {
			role = strings.TrimSpace(row.Position)
		}
		// คนเดียวอยู่หลายฝ่ายในคำสั่งเดียว รวมบทบาทไว้ในแถวเดียว บทบาทซ้ำกันทุกตัวอักษรจึงนับครั้งเดียว
		if at, seen := index[row.DocumentID]; seen {
			if role != "" && !roles[row.DocumentID][role] {
				roles[row.DocumentID][role] = true
				history[at].Role = strings.TrimPrefix(history[at].Role+", "+role, ", ")
			}
			continue
		}
		index[row.DocumentID] = len(history)
		roles[row.DocumentID] = map[string]bool{role: true}
		history = append(history, historyRow{DocumentID: row.DocumentID, OrderNo: row.OrderNo, Title: row.Title, Role: role, IssuedDate: row.IssuedDate, ExpiryDate: row.ExpiryDate, State: appointmentState(row.Status, row.ExpiryDate, today)})
	}
	return history, nil
}

func personHistoryHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "รหัสบุคคลไม่ถูกต้อง"})
			return
		}
		found, err := searchPeople(db, db.Where("id = ?", id))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดรายชื่อได้"})
			return
		}
		if len(found) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบบุคคลนี้ในระบบ"})
			return
		}
		user := &found[0]
		history, err := loadPersonHistory(db, user.ID, time.Now())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดประวัติการแต่งตั้งได้"})
			return
		}
		// การ์ดคนนับทุกช่วงเวลา ตารางกรองตามช่วงที่เลือก
		card := personCard{personSummary: personSummary{ID: user.ID, Name: personName(*user), Position: personPosition(*user)}}
		shown := summarizeHistory(&card, history, c)
		c.JSON(http.StatusOK, gin.H{"data": personHistory{Person: card, Appointments: shown}})
	}
}

// summarizeHistory นับตัวเลขบนการ์ด (ทุกช่วงเวลา) และคืนแถวตามช่วงวันที่และตัวกรอง "เฉพาะที่ยังมีผล" ที่ส่งมา
func summarizeHistory(card *personCard, history []historyRow, c *gin.Context) []historyRow {
	from, to := issuedRange(c)
	activeOnly := c.Query("activeOnly") == "1"
	shown := []historyRow{}
	for _, row := range history {
		if row.State != appointmentCancelled {
			card.Orders++
		}
		if row.State == appointmentActive || row.State == appointmentExpiring {
			card.Active++
		}
		if row.State == appointmentExpiring {
			card.Expiring++
		}
		if !withinRange(row.IssuedDate, from, to) || (activeOnly && row.State != appointmentActive && row.State != appointmentExpiring) {
			continue
		}
		shown = append(shown, row)
	}
	return shown
}

func appointmentIDs(rows []unlinkedAppointment) []uint {
	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

// nameHistoryHandler ประวัติของชื่อที่ยังไม่ผูกกับบุคคลในระบบ (/search?name=…) หน้าตาเหมือนประวัติของคน
func nameHistoryHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := unlinkedNameRows(db, c.Query("name"), true)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถค้นหาชื่อในคำสั่งได้"})
			return
		}
		if len(rows) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบชื่อนี้ในคำสั่งที่ยืนยันแล้ว"})
			return
		}
		history, err := loadHistory(db, time.Now(), "a.id IN ?", appointmentIDs(rows))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดประวัติการแต่งตั้งได้"})
			return
		}
		summary := summarizeUnlinked(rows)[0]
		card := personCard{personSummary: personSummary{Name: summary.Name, Position: summary.Position}}
		shown := summarizeHistory(&card, history, c)
		c.JSON(http.StatusOK, gin.H{"data": personHistory{Person: card, Appointments: shown}})
	}
}

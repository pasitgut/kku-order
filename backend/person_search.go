package main

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

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

type searchSuggestions struct {
	People []personSummary   `json:"people"`
	Orders []orderSuggestion `json:"orders"`
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

// issuedRange อ่านช่วงวันที่ออกคำสั่งจาก from / to (รวมวันสุดท้าย) ค่าที่อ่านไม่ได้ถือว่าไม่กรอง
func issuedRange(c *gin.Context) (from, to *time.Time) {
	if parsed, ok := normalizeThaiDate(c.Query("from")); ok {
		from = parsed
	}
	if parsed, ok := normalizeThaiDate(c.Query("to")); ok && parsed != nil {
		end := dateOnly(*parsed).AddDate(0, 0, 1)
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
		result := searchSuggestions{People: []personSummary{}, Orders: []orderSuggestion{}}
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
		Where("a.directory_user_id = ? AND "+historyDocuments, personID).
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
		c.JSON(http.StatusOK, gin.H{"data": personHistory{Person: card, Appointments: shown}})
	}
}

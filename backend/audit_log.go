package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// The audit trail is append-only. application_audit_logs only receives INSERTs.
// There are deliberately no update or delete endpoints for it; do not add one.

// auditLocation is Asia/Bangkok. Thailand has no daylight saving time, so a
// fixed +07:00 zone is exact and needs no tzdata in the container.
var auditLocation = time.FixedZone("Asia/Bangkok", 7*60*60)

const (
	auditDefaultPageSize = 50
	auditMaxPageSize     = 200
	auditExportMaxRows   = 10000
	auditMaxSearchRunes  = 200
	auditMaxUserRunes    = 255
)

var auditLikeEscaper = strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")

// auditLogFilter is the set of query-string filters on the audit list and export.
type auditLogFilter struct {
	Search     string
	UserID     string
	Actions    []string
	DocumentID uint
	From       *time.Time // inclusive
	To         *time.Time // exclusive
}

// auditLogRecord is one audit row joined with its document, as read from the database.
type auditLogRecord struct {
	ID             uint      `gorm:"column:id"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UserID         string    `gorm:"column:user_id"`
	Action         string    `gorm:"column:action"`
	DocumentID     *uint     `gorm:"column:document_id"`
	Details        string    `gorm:"column:details"`
	DocumentTitle  string    `gorm:"column:document_title"`
	DocumentExists bool      `gorm:"column:document_exists"`
}

type auditLogItem struct {
	ID             uint      `json:"id"`
	CreatedAt      time.Time `json:"createdAt"`
	UserID         string    `json:"userId"`
	UserName       string    `json:"userName"`
	Action         string    `json:"action"`
	ActionLabel    string    `json:"actionLabel"`
	DocumentID     *uint     `json:"documentId"`
	DocumentTitle  string    `json:"documentTitle"`
	DocumentExists bool      `json:"documentExists"`
	Details        string    `json:"details"`
}

type auditActionOption struct {
	Action string `json:"action"`
	Label  string `json:"label"`
}

// auditLogStore is the persistence the audit handlers need. Tests use a fake.
type auditLogStore interface {
	FindAuditLogs(filter auditLogFilter, offset, limit int) ([]auditLogRecord, int64, error)
	ExportAuditLogs(filter auditLogFilter, limit int) ([]auditLogRecord, error)
	DirectoryNames(userIDs []string) map[string]string
}

// buildAuditLogWhere turns the filters into a SQL condition over the aliases
// a (application_audit_logs) and w (document_workflows). An empty filter gives
// an empty condition.
func buildAuditLogWhere(filter auditLogFilter) (string, []any) {
	var clauses []string
	var args []any
	if filter.Search != "" {
		pattern := "%" + auditLikeEscaper.Replace(filter.Search) + "%"
		clauses = append(clauses, "(a.details LIKE ? ESCAPE '!' OR w.title LIKE ? ESCAPE '!')")
		args = append(args, pattern, pattern)
	}
	if filter.UserID != "" {
		clauses = append(clauses, "a.user_id LIKE ? ESCAPE '!'")
		args = append(args, "%"+auditLikeEscaper.Replace(filter.UserID)+"%")
	}
	if len(filter.Actions) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(filter.Actions)), ",")
		clauses = append(clauses, "a.action IN ("+placeholders+")")
		for _, action := range filter.Actions {
			args = append(args, action)
		}
	}
	if filter.DocumentID != 0 {
		clauses = append(clauses, "a.document_id = ?")
		args = append(args, filter.DocumentID)
	}
	if filter.From != nil {
		clauses = append(clauses, "a.created_at >= ?")
		args = append(args, *filter.From)
	}
	if filter.To != nil {
		clauses = append(clauses, "a.created_at < ?")
		args = append(args, *filter.To)
	}
	return strings.Join(clauses, " AND "), args
}

// parseAuditDay reads a YYYY-MM-DD day in Bangkok and returns its midnight.
func parseAuditDay(value string) (time.Time, error) {
	day, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(value), auditLocation)
	if err != nil {
		return time.Time{}, errors.New("วันที่ต้องอยู่ในรูปแบบ YYYY-MM-DD")
	}
	return day, nil
}

// parseAuditFilter reads the filter query parameters shared by the list and the export.
func parseAuditFilter(c *gin.Context) (auditLogFilter, error) {
	var filter auditLogFilter
	filter.Search = strings.TrimSpace(c.Query("q"))
	if len([]rune(filter.Search)) > auditMaxSearchRunes {
		return filter, errors.New("คำค้นหายาวเกิน 200 ตัวอักษร")
	}
	filter.UserID = strings.TrimSpace(c.Query("userId"))
	if len([]rune(filter.UserID)) > auditMaxUserRunes {
		return filter, errors.New("userId ยาวเกินไป")
	}
	for _, raw := range c.QueryArray("action") {
		for _, action := range strings.Split(raw, ",") {
			action = strings.TrimSpace(action)
			if action == "" {
				continue
			}
			if _, known := activityLabels[action]; !known {
				return filter, errors.New("action ไม่ถูกต้อง")
			}
			filter.Actions = append(filter.Actions, action)
		}
	}
	if raw := strings.TrimSpace(c.Query("documentId")); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || id == 0 {
			return filter, errors.New("documentId ต้องเป็นตัวเลขบวก")
		}
		filter.DocumentID = uint(id)
	}
	var fromDay, toDay time.Time
	if raw := strings.TrimSpace(c.Query("from")); raw != "" {
		day, err := parseAuditDay(raw)
		if err != nil {
			return filter, err
		}
		fromDay = day
		filter.From = &day
	}
	if raw := strings.TrimSpace(c.Query("to")); raw != "" {
		day, err := parseAuditDay(raw)
		if err != nil {
			return filter, err
		}
		toDay = day
		end := day.AddDate(0, 0, 1)
		filter.To = &end
	}
	if filter.From != nil && filter.To != nil && toDay.Before(fromDay) {
		return filter, errors.New("วันเริ่มต้นต้องไม่หลังวันสิ้นสุด")
	}
	return filter, nil
}

// parseAuditPagination reads page (from 1) and pageSize (1 to 200).
func parseAuditPagination(c *gin.Context) (int, int, error) {
	page := 1
	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			return 0, 0, errors.New("page ต้องเป็นตัวเลขตั้งแต่ 1")
		}
		page = value
	}
	pageSize := auditDefaultPageSize
	if raw := strings.TrimSpace(c.Query("pageSize")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > auditMaxPageSize {
			return 0, 0, errors.New("pageSize ต้องอยู่ระหว่าง 1 ถึง 200")
		}
		pageSize = value
	}
	return page, pageSize, nil
}

// auditDocumentTitle prefers the live document title. A deleted document keeps
// its title only inside the DOCUMENT_DELETED details.
func auditDocumentTitle(record auditLogRecord) string {
	if record.DocumentExists {
		return record.DocumentTitle
	}
	if record.Action == "DOCUMENT_DELETED" {
		var deleted deletedDocumentDetails
		if err := json.Unmarshal([]byte(record.Details), &deleted); err == nil {
			return deleted.Title
		}
	}
	return ""
}

func formatAuditTime(value time.Time) string {
	return value.In(auditLocation).Format("2006-01-02 15:04:05")
}

func auditUserIDs(records []auditLogRecord) []string {
	seen := make(map[string]bool, len(records))
	var ids []string
	for _, record := range records {
		if !seen[record.UserID] {
			seen[record.UserID] = true
			ids = append(ids, record.UserID)
		}
	}
	return ids
}

func auditActionOptions() []auditActionOption {
	actions := make([]string, 0, len(activityLabels))
	for action := range activityLabels {
		actions = append(actions, action)
	}
	sort.Strings(actions)
	options := make([]auditActionOption, 0, len(actions))
	for _, action := range actions {
		options = append(options, auditActionOption{Action: action, Label: activityLabels[action]})
	}
	return options
}

func listAuditLogsHandler(store auditLogStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		filter, err := parseAuditFilter(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		page, pageSize, err := parseAuditPagination(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		records, total, err := store.FindAuditLogs(filter, (page-1)*pageSize, pageSize)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดประวัติการใช้งานได้"})
			return
		}
		names := store.DirectoryNames(auditUserIDs(records))
		items := make([]auditLogItem, 0, len(records))
		for _, record := range records {
			items = append(items, auditItemFor(record, names))
		}
		c.JSON(http.StatusOK, gin.H{"data": items, "total": total, "page": page, "pageSize": pageSize})
	}
}

func auditActionsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": auditActionOptions()})
	}
}

func exportAuditLogsHandler(store auditLogStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		filter, err := parseAuditFilter(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		records, err := store.ExportAuditLogs(filter, auditExportMaxRows)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถส่งออกประวัติการใช้งานได้"})
			return
		}
		names := store.DirectoryNames(auditUserIDs(records))

		var buffer bytes.Buffer
		// The BOM tells Excel to read the file as UTF-8, so Thai text is not garbled.
		buffer.WriteString("\xEF\xBB\xBF")
		writer := csv.NewWriter(&buffer)
		_ = writer.Write([]string{"เวลา", "รหัสผู้ใช้", "ชื่อผู้ใช้", "การกระทำ", "เอกสาร", "รายละเอียด"})
		for _, record := range records {
			_ = writer.Write([]string{
				formatAuditTime(record.CreatedAt), record.UserID, names[record.UserID],
				activityLabel(record.Action), auditDocumentTitle(record), record.Details,
			})
		}
		writer.Flush()
		c.Header("Content-Disposition", `attachment; filename="audit-logs.csv"`)
		c.Data(http.StatusOK, "text/csv; charset=utf-8", buffer.Bytes())
	}
}

func auditItemFor(record auditLogRecord, names map[string]string) auditLogItem {
	return auditLogItem{
		ID: record.ID, CreatedAt: record.CreatedAt, UserID: record.UserID, UserName: names[record.UserID],
		Action: record.Action, ActionLabel: activityLabel(record.Action), DocumentID: record.DocumentID,
		DocumentTitle: auditDocumentTitle(record), DocumentExists: record.DocumentExists, Details: record.Details,
	}
}

type gormAuditLogStore struct {
	db *gorm.DB
}

const auditSelectColumns = `a.id, a.created_at, a.user_id, a.action, a.document_id, a.details,
	COALESCE(w.title, '') AS document_title, (w.id IS NOT NULL) AS document_exists`

func (s gormAuditLogStore) query(filter auditLogFilter) *gorm.DB {
	query := s.db.Table("application_audit_logs AS a").Joins("LEFT JOIN document_workflows AS w ON w.id = a.document_id")
	if where, args := buildAuditLogWhere(filter); where != "" {
		query = query.Where(where, args...)
	}
	return query
}

func (s gormAuditLogStore) FindAuditLogs(filter auditLogFilter, offset, limit int) ([]auditLogRecord, int64, error) {
	var total int64
	if err := s.query(filter).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count audit logs: %w", err)
	}
	var records []auditLogRecord
	err := s.query(filter).Select(auditSelectColumns).Order("a.created_at DESC, a.id DESC").Offset(offset).Limit(limit).Scan(&records).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list audit logs: %w", err)
	}
	return records, total, nil
}

func (s gormAuditLogStore) ExportAuditLogs(filter auditLogFilter, limit int) ([]auditLogRecord, error) {
	var records []auditLogRecord
	err := s.query(filter).Select(auditSelectColumns).Order("a.created_at DESC, a.id DESC").Limit(limit).Scan(&records).Error
	if err != nil {
		return nil, fmt.Errorf("export audit logs: %w", err)
	}
	return records, nil
}

// DirectoryNames returns the directory display name for each SSO account that has one.
func (s gormAuditLogStore) DirectoryNames(userIDs []string) map[string]string {
	names := make(map[string]string, len(userIDs))
	for _, userID := range userIDs {
		user, found := loadDirectoryUserForAccount(s.db, userID)
		if !found {
			continue
		}
		if name := strings.TrimSpace(displayDirectoryName(user)); name != "" {
			names[userID] = name
		}
	}
	return names
}

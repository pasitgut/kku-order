package main

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// VacantPosition คือตำแหน่งที่เอกสารรอยืนยันอ้างถึง แต่ระบบยังหาผู้ดำรงไม่ได้
type VacantPosition struct {
	PositionTitle string `json:"positionTitle"`
	Documents     int    `json:"documents"`
}

// vacantPositions รวมแถวโดยตำแหน่งที่ยังไม่ผูกคน ในเอกสารที่ยังไม่ยืนยันหรือยกเลิก นับเป็นจำนวนเอกสารต่อตำแหน่ง
func vacantPositions(db *gorm.DB) ([]VacantPosition, error) {
	var rows []struct {
		DocumentID uint
		Position   string
	}
	err := db.Table("appointments AS a").Select("a.document_id, a.position").
		Joins("JOIN document_workflows AS d ON d.id = a.document_id").
		Where("d.status NOT IN ?", []string{"CONFIRMED", documentCancelled}).
		Where("a.directory_user_id IS NULL AND TRIM(COALESCE(a.full_name, '')) = '' AND TRIM(COALESCE(a.position, '')) <> ''").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	titles := map[string]string{}
	documents := map[string]map[uint]bool{}
	for _, row := range rows {
		if committeeRoleOnly(row.Position) {
			continue
		}
		key := positionKey(row.Position)
		if key == "" {
			continue
		}
		// แสดงชื่อตำแหน่งแบบสั้นที่สุดที่พบ (ตัดชื่อหน่วยงานและบทบาทท้ายออกแล้ว)
		title, _ := splitExOfficioPosition(row.Position)
		title = strings.TrimSpace(positionSpacing.ReplaceAllString(organisationWords.ReplaceAllString(title, ""), " "))
		if current, ok := titles[key]; !ok || len([]rune(title)) < len([]rune(current)) {
			titles[key] = title
		}
		if documents[key] == nil {
			documents[key] = map[uint]bool{}
		}
		documents[key][row.DocumentID] = true
	}
	vacant := make([]VacantPosition, 0, len(titles))
	for key, title := range titles {
		vacant = append(vacant, VacantPosition{PositionTitle: title, Documents: len(documents[key])})
	}
	sort.Slice(vacant, func(i, j int) bool {
		if vacant[i].Documents != vacant[j].Documents {
			return vacant[i].Documents > vacant[j].Documents
		}
		return vacant[i].PositionTitle < vacant[j].PositionTitle
	})
	return vacant, nil
}

func listVacantPositions(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		vacant, err := vacantPositions(db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดตำแหน่งที่ยังไม่มีผู้ดำรงได้"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": vacant})
	}
}

// readPositionHolderInput ตรวจชื่อตำแหน่งและวันที่เริ่มดำรงตำแหน่งที่ผู้ใช้กรอก คืนข้อความ error ภาษาไทยถ้าใช้ไม่ได้
func readPositionHolderInput(title, startsOn string) (string, *time.Time, string) {
	title, _ = splitExOfficioPosition(title)
	if positionKey(title) == "" {
		return "", nil, "กรุณาระบุตำแหน่งบริหาร"
	}
	if committeeRoleOnly(title) {
		return "", nil, "ตำแหน่งนี้เป็นบทบาทในคณะกรรมการ ระบบดึงคนตามตำแหน่งไม่ได้ ให้เลือกคนในหน้าตรวจเอกสาร"
	}
	since, ok := normalizeThaiDate(startsOn)
	if !ok {
		return "", nil, "อ่านวันที่เริ่มดำรงตำแหน่งไม่ได้ ลองพิมพ์แบบ 1 ต.ค. 2568"
	}
	return truncateRunes(title, 255), since, ""
}

// assignPositionHolder บันทึกผู้ดำรงตำแหน่ง ถ้าตำแหน่งเดียวกันเริ่มวันเดียวกันเคยบันทึกคนอื่นไว้ ถือว่าแก้ไขรายการเดิม
func assignPositionHolder(tx *gorm.DB, title string, personID uint, startsOn *time.Time, source, createdBy string) error {
	if source == positionSourceManual && startsOn != nil {
		var previous []PositionHolder
		if err := tx.Where("position_key = ? AND source = ? AND starts_on < ? AND ends_on IS NULL", positionKey(title), positionSourceManual, startsOn.Format("2006-01-02")).Find(&previous).Error; err != nil {
			return err
		}
		lastDay := startsOn.AddDate(0, 0, -1)
		for _, holder := range previous {
			if err := tx.Model(&PositionHolder{}).Where("id = ?", holder.ID).Update("ends_on", lastDay).Error; err != nil {
				return err
			}
		}
	}
	replaced := tx.Where("position_key = ? AND source <> ?", positionKey(title), positionSourceSync)
	if startsOn == nil {
		replaced = replaced.Where("starts_on IS NULL")
	} else {
		replaced = replaced.Where("starts_on = ?", startsOn.Format("2006-01-02"))
	}
	if err := replaced.Where("directory_person_id <> ?", personID).Delete(&PositionHolder{}).Error; err != nil {
		return err
	}
	return rememberPositionHolder(tx, title, personID, startsOn, source, createdBy)
}

// linkPendingByPosition ผูกแถวโดยตำแหน่งในเอกสารที่รอยืนยัน แล้วคืนจำนวนแถวที่เพิ่งผูกกับคนนี้
func linkPendingByPosition(db *gorm.DB, personID uint) (int, error) {
	count := func() (int64, error) {
		var total int64
		err := db.Table("appointments AS a").Joins("JOIN document_workflows AS d ON d.id = a.document_id").
			Where("d.status NOT IN ?", []string{"CONFIRMED", documentCancelled}).
			Where("a.directory_user_id = ? AND a.name_match_method = ?", personID, matchMethodPosition).
			Count(&total).Error
		return total, err
	}
	before, err := count()
	if err != nil {
		return 0, err
	}
	if err := refreshPendingMatches(db); err != nil {
		return 0, err
	}
	after, err := count()
	return int(after - before), err
}

func setPositionHolder(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload struct {
			PositionTitle   string `json:"positionTitle"`
			DirectoryUserID uint   `json:"directoryUserId"`
			StartsOn        string `json:"startsOn"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
			return
		}
		title, startsOn, problem := readPositionHolderInput(payload.PositionTitle, payload.StartsOn)
		if problem != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": problem})
			return
		}
		var person DirectoryPerson
		if err := db.First(&person, payload.DirectoryUserID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบบุคคลนี้ในระบบ"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถตรวจสอบบุคคลได้"})
			return
		}
		createdBy := currentUserID(c)
		if err := assignPositionHolder(db, title, person.ID, startsOn, positionSourceManual, createdBy); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถบันทึกผู้ดำรงตำแหน่งได้"})
			return
		}
		linked, err := linkPendingByPosition(db, person.ID)
		if err != nil {
			linked = 0
		}
		_ = db.Create(&AuditLog{UserID: createdBy, Action: "POSITION_HOLDER_SET", Details: title + ": " + strings.TrimSpace(person.Prefix+" "+person.FirstName+" "+person.LastName)}).Error
		c.JSON(http.StatusCreated, gin.H{"data": gin.H{"positionTitle": title, "directoryUserId": person.ID, "startsOn": startsOn}, "linked": linked})
	}
}

func listDirectoryPersonPositions(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		personID, ok := parseDirectoryPersonID(c.Param("id"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "รหัสบุคคลไม่ถูกต้อง"})
			return
		}
		var person DirectoryPerson
		if err := db.Select("id").First(&person, personID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบบุคคลนี้ในระบบ"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถตรวจสอบข้อมูลอาจารย์ได้"})
			return
		}
		var positions []PositionHolder
		if err := db.Where("directory_person_id = ?", personID).Order("starts_on DESC").Order("id DESC").Find(&positions).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถโหลดประวัติตำแหน่งได้"})
			return
		}
		if positions == nil {
			positions = []PositionHolder{}
		}
		c.JSON(http.StatusOK, gin.H{"data": positions})
	}
}

var errPositionPeriodOverlap = errors.New("ช่วงเวลาดำรงตำแหน่งทับซ้อนกับรายการที่มีอยู่")

func addDirectoryPersonPosition(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		personID, ok := parseDirectoryPersonID(c.Param("id"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "รหัสบุคคลไม่ถูกต้อง"})
			return
		}
		var payload struct {
			PositionTitle string `json:"positionTitle"`
			StartsOn      string `json:"startsOn"`
			EndsOn        string `json:"endsOn"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
			return
		}
		title, startsOn, problem := readPositionHolderInput(payload.PositionTitle, payload.StartsOn)
		if problem != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": problem})
			return
		}
		var endsOn *time.Time
		if strings.TrimSpace(payload.EndsOn) != "" {
			parsed, valid := normalizeThaiDate(payload.EndsOn)
			if !valid || parsed == nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "อ่านวันที่สิ้นสุดไม่ได้ ลองพิมพ์แบบ 30 ก.ย. 2568"})
				return
			}
			if parsed.Before(*startsOn) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "วันที่สิ้นสุดต้องไม่อยู่ก่อนวันที่เริ่มดำรงตำแหน่ง"})
				return
			}
			endsOn = parsed
		}

		var person DirectoryPerson
		if err := db.First(&person, personID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบบุคคลนี้ในระบบ"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถตรวจสอบบุคคลได้"})
			return
		}

		changedBy := currentUserID(c)
		changedAt := time.Now()
		var created PositionHolder
		err := db.Transaction(func(tx *gorm.DB) error {
			var existing []PositionHolder
			if err := tx.Where("position_key = ? AND source = ?", positionKey(title), positionSourceManual).Find(&existing).Error; err != nil {
				return err
			}
			changes := make([]DirectoryPersonChange, 0, 2)
			for index := range existing {
				previous := &existing[index]
				if previous.StartsOn == nil {
					continue
				}
				if previous.EndsOn == nil && previous.StartsOn.Before(*startsOn) {
					oldValue := positionTimelineValue(*previous)
					lastDay := startsOn.AddDate(0, 0, -1)
					if err := tx.Model(&PositionHolder{}).Where("id = ?", previous.ID).Update("ends_on", lastDay).Error; err != nil {
						return err
					}
					previous.EndsOn = &lastDay
					changes = append(changes, DirectoryPersonChange{Field: "positionTimeline", OldValue: oldValue, NewValue: positionTimelineValue(*previous)})
				}
				if positionPeriodsOverlap(startsOn, endsOn, previous.StartsOn, previous.EndsOn) {
					return errPositionPeriodOverlap
				}
			}
			created = PositionHolder{
				PositionKey: positionKey(title), PositionTitle: title, DirectoryPersonID: personID,
				StartsOn: startsOn, EndsOn: endsOn, Source: positionSourceManual, CreatedBy: changedBy,
			}
			if err := tx.Create(&created).Error; err != nil {
				return err
			}
			changes = append(changes, DirectoryPersonChange{Field: "positionTimeline", OldValue: "", NewValue: positionTimelineValue(created)})
			period := startsOn.Format("2006-01-02") + "–"
			if endsOn == nil {
				period += "ปัจจุบัน"
			} else {
				period += endsOn.Format("2006-01-02")
			}
			if err := recordDirectoryPersonRevision(tx, personID, changes, changedBy, changedAt); err != nil {
				return err
			}
			name := strings.TrimSpace(strings.Join([]string{person.Prefix, person.FirstName, person.LastName}, " "))
			return tx.Create(&AuditLog{UserID: changedBy, Action: "DIRECTORY_POSITION_ADDED", Details: "personId=" + strconv.FormatUint(uint64(personID), 10) + " title=" + title + " period=" + period + " name=" + name}).Error
		})
		if errors.Is(err, errPositionPeriodOverlap) {
			c.JSON(http.StatusConflict, gin.H{"error": "ช่วงเวลานี้ทับซ้อนกับตำแหน่งที่บันทึกไว้แล้ว กรุณาตรวจสอบวันที่เดิม"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ไม่สามารถบันทึกประวัติตำแหน่งได้"})
			return
		}
		linked, _ := linkPendingByPosition(db, personID)
		c.JSON(http.StatusCreated, gin.H{"data": created, "linked": linked})
	}
}

func positionPeriodsOverlap(start, end, otherStart, otherEnd *time.Time) bool {
	if start == nil || otherStart == nil {
		return false
	}
	if end != nil && end.Before(*otherStart) {
		return false
	}
	if otherEnd != nil && start.After(*otherEnd) {
		return false
	}
	return true
}

func positionTimelineValue(position PositionHolder) string {
	period := "ไม่ระบุวันเริ่ม"
	if position.StartsOn != nil {
		period = position.StartsOn.Format("2006-01-02")
	}
	period += "–"
	if position.EndsOn == nil {
		period += "ปัจจุบัน"
	} else {
		period += position.EndsOn.Format("2006-01-02")
	}
	return position.PositionTitle + " · " + period
}

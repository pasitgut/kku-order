package main

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	positionSourceSync    = "SYNC"
	positionSourceManual  = "MANUAL"
	positionSourceLearned = "LEARNED"
	matchMethodPosition   = "POSITION"
)

// PositionHolder บอกว่าใครดำรงตำแหน่งบริหารไหน ตั้งแต่วันไหน ใช้หาคนให้แถว "โดยตำแหน่ง" ที่เอกสารไม่ได้เขียนชื่อ
type PositionHolder struct {
	ID                uint       `json:"id" gorm:"primaryKey"`
	PositionKey       string     `json:"-" gorm:"size:255;not null"`
	PositionTitle     string     `json:"positionTitle" gorm:"size:255;not null"`
	DirectoryPersonID uint       `json:"directoryUserId" gorm:"column:directory_person_id;not null"`
	StartsOn          *time.Time `json:"startsOn" gorm:"type:date"`
	// EndsOn เป็นวันสุดท้ายที่ดำรงตำแหน่ง (นับรวมวันนั้น)
	EndsOn    *time.Time `json:"endsOn" gorm:"type:date"`
	Source    string     `json:"source" gorm:"size:20;not null"`
	CreatedBy string     `json:"createdBy" gorm:"size:255"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

func (PositionHolder) TableName() string { return "position_holders" }

var (
	organisationWords  = regexp.MustCompile(`วิทยาลัยการคอมพิวเตอร์|มหาวิทยาลัยขอนแก่น`)
	trailingRolePhrase = regexp.MustCompile(`\s+เป็น\s*(.*)$`)
	positionSpacing    = regexp.MustCompile(`\s+`)
	committeeRoleTitle = regexp.MustCompile(`^(รอง)?ประธาน(กรรมการ)?$|^กรรมการ|^(ผู้ช่วย)?เลขานุการ|^ที่ปรึกษา$`)
)

// committeeRoleOnly: บทบาทในคณะกรรมการล้วนๆ ("ประธานกรรมการ", "กรรมการและเลขานุการ") ไม่ใช่ตำแหน่งในองค์กร
// แต่ละคำสั่งตั้งคนต่างกัน จึงดึงคนตามตำแหน่งไม่ได้ ให้ผู้ตรวจเลือกเอง
// ถ้ามีชื่อหน่วยงานต่อท้าย ("เลขานุการวิทยาลัยการคอมพิวเตอร์") ถือเป็นตำแหน่งในองค์กร
func committeeRoleOnly(title string) bool {
	title, _ = splitExOfficioPosition(replaceThaiDigits(title))
	if organisationWords.MatchString(title) {
		return false
	}
	return committeeRoleTitle.MatchString(positionSpacing.ReplaceAllString(title, ""))
}

// positionKey: ชื่อตำแหน่งในเอกสารยาวกว่าใน sync ("คณบดีวิทยาลัยการคอมพิวเตอร์" กับ "คณบดี")
// จึงตัดชื่อหน่วยงานและช่องว่างออกก่อนเทียบ แล้วต้องตรงกันทุกตัวอักษร
func positionKey(title string) string {
	title, _ = splitExOfficioPosition(title)
	title = organisationWords.ReplaceAllString(replaceThaiDigits(title), "")
	return positionSpacing.ReplaceAllString(title, "")
}

// splitExOfficioPosition แยก "… เป็นประธานกรรมการ" ที่ OCR ติดมาท้ายตำแหน่งออกเป็นบทบาท
// ต้องมีช่องว่างก่อน "เป็น" จึงไม่ตัดคำอย่าง "ความเป็นเลิศ"
func splitExOfficioPosition(text string) (position, role string) {
	text = strings.TrimSpace(text)
	if match := trailingRolePhrase.FindStringSubmatchIndex(text); match != nil {
		return strings.TrimSpace(text[:match[0]]), strings.TrimSpace(text[match[2]:match[3]])
	}
	return text, ""
}

// pickHolder เลือกผู้ดำรงตำแหน่ง ณ วันที่ออกคำสั่ง เชื่อข้อมูลตามลำดับนี้
//  1. ที่เจ้าหน้าที่กรอกพร้อมวันเริ่ม: คนที่เริ่มล่าสุดแต่ไม่หลังวันนั้น ใช้ไปจนกว่าจะกรอกคนถัดไป
//  2. ผู้ดำรงตำแหน่งปัจจุบัน: ที่เจ้าหน้าที่กรอกโดยไม่ระบุวัน ก่อนข้อมูล sync
//  3. คนที่ผู้ตรวจเคยเลือกให้ตำแหน่งนี้ (ใช้กับตำแหน่งที่ sync ไม่มี) เลือกครั้งที่ใกล้วันนั้นที่สุด
//
// การเลือกในเอกสารเก่าฉบับเดียวจึงไม่ทับข้อมูล sync ของคำสั่งใหม่
func pickHolder(holders []PositionHolder, on *time.Time) *PositionHolder {
	referenceDate := on
	if referenceDate == nil {
		now := time.Now()
		referenceDate = &now
	}
	if holder := latestStarted(holders, positionSourceManual, referenceDate); holder != nil {
		return holder
	}
	var current *PositionHolder
	for i := range holders {
		holder := &holders[i]
		if holder.StartsOn != nil || (holder.Source != positionSourceManual && holder.Source != positionSourceSync) || (holder.EndsOn != nil && holder.EndsOn.Before(*referenceDate)) {
			continue
		}
		if current == nil || (holder.Source == positionSourceManual && (current.Source == positionSourceSync || holder.UpdatedAt.After(current.UpdatedAt))) {
			current = holder
		}
	}
	if current != nil {
		return current
	}
	if holder := latestStarted(holders, positionSourceLearned, referenceDate); holder != nil {
		return holder
	}
	for i := range holders {
		if holders[i].Source == positionSourceLearned && holders[i].StartsOn == nil && (holders[i].EndsOn == nil || !holders[i].EndsOn.Before(*referenceDate)) {
			return &holders[i]
		}
	}
	return nil
}

// latestStarted คืนแถวจากแหล่งที่ระบุที่เริ่มล่าสุดแต่ไม่หลังวันที่ on (on ว่างคือเอาแถวล่าสุด)
func latestStarted(holders []PositionHolder, source string, on *time.Time) *PositionHolder {
	var best *PositionHolder
	for i := range holders {
		holder := &holders[i]
		if holder.Source != source || holder.StartsOn == nil || (on != nil && (holder.StartsOn.After(*on) || (holder.EndsOn != nil && holder.EndsOn.Before(*on)))) {
			continue
		}
		if best == nil || holder.StartsOn.After(*best.StartsOn) || (holder.StartsOn.Equal(*best.StartsOn) && holder.UpdatedAt.After(best.UpdatedAt)) {
			best = holder
		}
	}
	return best
}

// holderFor หาผู้ดำรงตำแหน่งให้แถวโดยตำแหน่ง บทบาทในคณะกรรมการไม่มีผู้ดำรงตำแหน่ง
func holderFor(position string, holders map[string][]PositionHolder, issued *time.Time) *PositionHolder {
	if strings.TrimSpace(position) == "" || committeeRoleOnly(position) {
		return nil
	}
	return pickHolder(holders[positionKey(position)], issued)
}

// loadPositionHolders ข้ามคนที่ออกไปแล้ว (สถานะไม่ active แบบเดียวกับ loadDirectory)
func loadPositionHolders(db *gorm.DB) (map[string][]PositionHolder, error) {
	var rows []PositionHolder
	active := db.Model(&DirectoryPerson{}).Select("id").Where("status IS NULL OR UPPER(status) NOT IN ?", []string{"I", "INACTIVE", "0", "FALSE", "NO"})
	if err := db.Where("directory_person_id IN (?)", active).Find(&rows).Error; err != nil {
		return nil, err
	}
	byKey := make(map[string][]PositionHolder, len(rows))
	for _, row := range rows {
		byKey[row.PositionKey] = append(byKey[row.PositionKey], row)
	}
	return byKey, nil
}

// linkByPosition ผูกแถวที่มีแต่ตำแหน่งกับผู้ดำรงตำแหน่ง ณ วันที่ออกคำสั่ง คืนจำนวนแถวที่ผูกได้
func linkByPosition(appointments []Appointment, holders map[string][]PositionHolder, issued *time.Time) int {
	linked := 0
	for i := range appointments {
		row := &appointments[i]
		if strings.TrimSpace(row.FullName) != "" || row.DirectoryUserID != nil {
			continue
		}
		holder := holderFor(row.Position, holders, issued)
		if holder == nil {
			continue
		}
		personID := holder.DirectoryPersonID
		row.DirectoryUserID = &personID
		row.NameMatchMethod = matchMethodPosition
		row.NameMatchScore = 1
		linked++
	}
	return linked
}

// rememberPositionHolder จำว่าผู้ตรวจเลือกใครให้ตำแหน่งนี้ เอกสารฉบับต่อไปจะผูกให้เอง
func rememberPositionHolder(tx *gorm.DB, title string, personID uint, startsOn *time.Time, source, createdBy string) error {
	title, _ = splitExOfficioPosition(title)
	key := positionKey(title)
	if key == "" || personID == 0 || committeeRoleOnly(title) {
		return nil
	}
	var existing PositionHolder
	query := tx.Where("position_key = ? AND directory_person_id = ?", key, personID)
	if startsOn == nil {
		query = query.Where("starts_on IS NULL")
	} else {
		query = query.Where("starts_on = ?", startsOn.Format("2006-01-02"))
	}
	if err := query.Take(&existing).Error; err == nil {
		return nil
	}
	return tx.Create(&PositionHolder{PositionKey: key, PositionTitle: truncateRunes(title, 255), DirectoryPersonID: personID, StartsOn: startsOn, Source: source, CreatedBy: createdBy}).Error
}

// syncPositionHolders เขียนตำแหน่งบริหารจากข้อมูล sync ใหม่ทั้งชุด แถวที่เจ้าหน้าที่กรอกหรือระบบเรียนรู้ไม่ถูกแตะ
func syncPositionHolders(db *gorm.DB) error {
	users, _, err := loadDirectory(db)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("source = ?", positionSourceSync).Delete(&PositionHolder{}).Error; err != nil {
			return err
		}
		for _, user := range users {
			managed := managePositionOf(user)
			if managed == "" || user.Source == sourceManual {
				continue
			}
			if err := rememberPositionHolder(tx, managed, user.ID, nil, positionSourceSync, "sync"); err != nil {
				return err
			}
		}
		return nil
	})
}

// managePositionOf อ่านตำแหน่งบริหารจากข้อมูลที่ sync มา (เก็บไว้ใน raw payload ของโปรไฟล์)
func managePositionOf(user DirectoryUser) string {
	if value := strings.TrimSpace(user.ManagePosition); value != "" {
		return value
	}
	var item map[string]any
	if json.Unmarshal([]byte(user.RawPayload), &item) != nil {
		return ""
	}
	return strings.TrimSpace(firstString(item, "manage_position", "managePosition"))
}

package main

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// คำนำหน้าเรียงคำยาวก่อน เพื่อให้ "ผู้ช่วยศาสตราจารย์" ไม่ถูกจับเป็น "ศาสตราจารย์"
// rank ว่างคือคำนำหน้าทั่วไป (นาย/นาง/ดร.) ที่ไม่ใช่ตำแหน่งวิชาการ
var thaiHonorifics = []struct{ text, rank string }{
	{"ผู้ช่วยศาสตราจารย์", "ผู้ช่วยศาสตราจารย์"},
	{"รองศาสตราจารย์", "รองศาสตราจารย์"},
	{"ศาสตราจารย์", "ศาสตราจารย์"},
	{"อาจารย์", "อาจารย์"},
	{"นางสาว", ""},
	{"ผศ.", "ผู้ช่วยศาสตราจารย์"},
	{"รศ.", "รองศาสตราจารย์"},
	{"น.ส.", ""},
	{"ดร.", ""},
	{"ศ.", "ศาสตราจารย์"},
	{"อ.", "อาจารย์"},
	{"นาย", ""},
	{"นาง", ""},
}

// splitThaiHonorific แยกตำแหน่งวิชาการและคำนำหน้าออกจากชื่อ ไม่ว่า OCR จะเว้นวรรคหรือเขียนติดกัน
// คำนำหน้าทั่วไป (นาย/นาง/นางสาว) ถูกตัดออก ส่วน "ดร." คงไว้หน้าชื่อเพราะเป็นส่วนหนึ่งของการเรียกชื่อ
func splitThaiHonorific(value string) (rank, name string) {
	name = strings.TrimSpace(value)
	doctor := false
	for {
		matched := false
		for _, honorific := range thaiHonorifics {
			if strings.HasPrefix(name, honorific.text) {
				if honorific.text == "ดร." {
					doctor = true
				} else if rank == "" {
					rank = honorific.rank
				}
				name = strings.TrimSpace(strings.TrimPrefix(name, honorific.text))
				matched = true
				break
			}
		}
		if !matched {
			break
		}
	}
	if doctor && name != "" {
		name = "ดร." + name
	}
	return rank, name
}

var rankAbbreviations = map[string]string{"ผู้ช่วยศาสตราจารย์": "ผศ.", "รองศาสตราจารย์": "รศ.", "ศาสตราจารย์": "ศ.", "อาจารย์": "อ."}

// nameAndPosition: ตำแหน่งวิชาการไปอยู่ช่องตำแหน่ง ถ้าช่องนั้นมีตำแหน่งจาก OCR อยู่แล้ว ให้เก็บตัวย่อไว้หน้าชื่อ ไม่ให้หาย
func nameAndPosition(rawName, position string) (string, string) {
	rank, name := splitThaiHonorific(rawName)
	position = strings.TrimSpace(position)
	if rank == "" || name == "" {
		return name, position
	}
	if position == "" {
		return name, rank
	}
	return rankAbbreviations[rank] + name, position
}

// tidyAppointmentNames แก้แถวรายชื่อที่นำเข้าก่อนมีตัวแยกคำนำหน้า เช่น "ผู้ช่วยศาสตราจารย์ภาณุพงษ์"
// แตะเฉพาะเอกสารที่ยังไม่ยืนยันหรือยกเลิก และบันทึกการแก้ในประวัติว่าระบบเป็นผู้แก้
func tidyAppointmentNames(db *gorm.DB) error {
	var rows []Appointment
	err := db.Table("appointments AS a").Select("a.id", "a.document_id", "a.full_name", "a.position").
		Joins("JOIN document_workflows AS d ON d.id = a.document_id").
		Where("d.status NOT IN ?", []string{"CONFIRMED", documentCancelled}).
		Where("a.full_name LIKE ? OR a.full_name LIKE ? OR a.full_name LIKE ? OR a.full_name LIKE ? OR a.full_name LIKE ? OR a.full_name LIKE ? OR a.full_name LIKE ? OR a.full_name LIKE ?",
			"ผู้ช่วยศาสตราจารย์%", "รองศาสตราจารย์%", "ศาสตราจารย์%", "อาจารย์%", "นาย%", "นาง%", "น.ส.%", "_ศ.%").
		Scan(&rows).Error
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, row := range rows {
			name, position := nameAndPosition(row.FullName, row.Position)
			if name == row.FullName && position == row.Position {
				continue
			}
			if err := tx.Model(&Appointment{}).Where("id = ?", row.ID).Updates(map[string]any{"full_name": name, "position": position}).Error; err != nil {
				return err
			}
			revision := DocumentRevision{DocumentID: row.DocumentID, FieldKey: "appointment_name", OldValue: row.FullName, NewValue: name, ChangedBy: "ระบบ", ChangedAt: time.Now()}
			if err := tx.Create(&revision).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// refreshPendingMatches จับคู่รายชื่อในเอกสารที่ยังไม่ยืนยันใหม่อีกครั้ง: ผูกชื่อที่ตอนนี้ตรงเป๊ะกับคนในระบบ
// และถอดการผูกแบบชื่อใกล้เคียงที่ระบบเคยผูกเองออก ให้เป็นตัวเลือก "ใช่คนนี้" แทน
func refreshPendingMatches(db *gorm.DB) error {
	var rows []Appointment
	err := db.Table("appointments AS a").Select("a.*").
		Joins("JOIN document_workflows AS d ON d.id = a.document_id").
		Where("d.status NOT IN ?", []string{"CONFIRMED", documentCancelled}).
		Where("a.directory_user_id IS NULL OR a.name_match_method IN ?", []string{matchMethodFuzzy, matchMethodPosition}).
		Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return err
	}
	users, aliases, err := loadDirectory(db)
	if err != nil {
		return err
	}
	holders, err := loadPositionHolders(db)
	if err != nil {
		return err
	}
	issuedDates, err := pendingIssuedDates(db, rows)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, row := range rows {
			if strings.TrimSpace(row.FullName) == "" {
				if err := relinkByPosition(tx, row, holders, issuedDates[row.DocumentID]); err != nil {
					return err
				}
				continue
			}
			if row.NameMatchMethod == matchMethodFuzzy && row.DirectoryUserID != nil {
				if err := tx.Model(&Appointment{}).Where("id = ?", row.ID).Updates(map[string]any{"directory_user_id": nil, "name_match_score": 0}).Error; err != nil {
					return err
				}
				continue
			}
			match := matchDirectoryUser(row.FullName, users, aliases)
			if !match.Matched || (match.Method != matchMethodExact && match.Method != matchMethodAlias) {
				continue
			}
			linked := row
			applyMatchToAppointment(&linked, match)
			if err := tx.Model(&Appointment{}).Where("id = ?", row.ID).Updates(map[string]any{"directory_user_id": linked.DirectoryUserID, "name_match_method": linked.NameMatchMethod, "name_match_score": linked.NameMatchScore, "full_name": linked.FullName, "position": linked.Position}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// relinkByPosition ผูกแถวโดยตำแหน่งกับผู้ดำรงตำแหน่งตามข้อมูลล่าสุด แถวที่ระบบเคยผูกไว้เปลี่ยนคนตามได้เมื่อแก้ผู้ดำรงตำแหน่ง
// ส่วนแถวที่ผู้ตรวจเอาคนออกเอง (MANUAL ไม่มีคน) ปล่อยไว้ตามนั้น
func relinkByPosition(tx *gorm.DB, row Appointment, holders map[string][]PositionHolder, issued *time.Time) error {
	if row.NameMatchMethod == matchMethodManual && row.DirectoryUserID == nil {
		return nil
	}
	holder := holderFor(row.Position, holders, issued)
	switch {
	case holder == nil && row.NameMatchMethod == matchMethodPosition && row.DirectoryUserID != nil:
		return tx.Model(&Appointment{}).Where("id = ?", row.ID).Updates(map[string]any{"directory_user_id": nil, "name_match_method": matchMethodExOfficio, "name_match_score": 0}).Error
	case holder == nil || (row.DirectoryUserID != nil && *row.DirectoryUserID == holder.DirectoryPersonID):
		return nil
	}
	return tx.Model(&Appointment{}).Where("id = ?", row.ID).Updates(map[string]any{"directory_user_id": holder.DirectoryPersonID, "name_match_method": matchMethodPosition, "name_match_score": 1}).Error
}

// pendingIssuedDates คืนวันที่ออกคำสั่งของเอกสารที่มีแถวต้องจับคู่ ใช้เลือกผู้ดำรงตำแหน่ง ณ วันนั้น
func pendingIssuedDates(db *gorm.DB, rows []Appointment) (map[uint]*time.Time, error) {
	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.DocumentID)
	}
	var documents []Document
	if err := db.Select("id", "issued_date").Where("id IN ?", ids).Find(&documents).Error; err != nil {
		return nil, err
	}
	dates := make(map[uint]*time.Time, len(documents))
	for _, document := range documents {
		dates[document.ID] = document.IssuedDate
	}
	return dates, nil
}

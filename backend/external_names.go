package main

import (
	"time"

	"gorm.io/gorm"
)

// linkExternalNames ผูกชื่อที่เคยหาในระบบไม่เจอ (ส่วนใหญ่เป็นคนนอกคณะ) กับคนที่เพิ่งเข้าระบบ
// ทำกับเอกสารทุกสถานะยกเว้นยกเลิก รวมที่ยืนยันแล้ว แต่แตะเฉพาะแถวที่มีชื่อและยังไม่ผูกกับใคร
// ผูกเฉพาะชื่อที่ตรงเป๊ะหรือเคยผูกไว้ ทุกการผูกลงประวัติการแก้ไขของเอกสาร
// แถวที่ผู้ตรวจเอาคนออกเอง (MANUAL) ไม่ถูกผูกกลับ
// คืนจำนวนแถวที่ผูก ถ้าผูกได้ เอกสารที่รอยืนยันและพร้อมแล้วจะถูกยืนยันอัตโนมัติ (ถ้าเปิดสวิตช์ไว้)
func linkExternalNames(db *gorm.DB, changedBy string) (int, error) {
	var rows []Appointment
	err := db.Table("appointments AS a").Select("a.*").
		Joins("JOIN documents AS d ON d.id = a.document_id").
		Where("d.status <> ?", documentCancelled).
		Where("a.directory_user_id IS NULL AND TRIM(COALESCE(a.full_name, '')) <> ''").
		Where("COALESCE(a.name_match_method, '') <> ?", matchMethodManual).
		Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	users, aliases, err := loadDirectory(db)
	if err != nil {
		return 0, err
	}
	linked, retagged := 0, 0
	err = db.Transaction(func(tx *gorm.DB) error {
		for _, row := range rows {
			match := matchDirectoryUser(row.FullName, users, aliases)
			if !match.Matched || (match.Method != matchMethodExact && match.Method != matchMethodAlias) {
				// แถวที่อ่านก่อน sync ครั้งแรกติดป้าย "ยังไม่มีฐานบุคลากร" ซึ่งขวางการยืนยันอัตโนมัติ
				// พอมีรายชื่อแล้วให้เก็บผลการจับคู่จริงแทน แถวจึงกลายเป็นชื่อคนนอกที่ไม่ขวางแล้ว
				if row.NameMatchMethod == matchMethodEmpty && match.Method != matchMethodEmpty {
					if err := tx.Model(&Appointment{}).Where("id = ? AND directory_user_id IS NULL", row.ID).Update("name_match_method", match.Method).Error; err != nil {
						return err
					}
					retagged++
				}
				continue
			}
			personID := match.User.ID
			// เงื่อนไข directory_user_id IS NULL กันการทับแถวที่มีคนผูกไประหว่างทาง
			result := tx.Model(&Appointment{}).Where("id = ? AND directory_user_id IS NULL", row.ID).
				Updates(map[string]any{"directory_user_id": personID, "name_match_method": match.Method, "name_match_score": match.Score})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				continue
			}
			revision := DocumentRevision{DocumentID: row.DocumentID, FieldKey: "appointment_link", OldValue: row.FullName, NewValue: displayDirectoryName(match.User), ChangedBy: changedBy, ChangedAt: time.Now()}
			if err := tx.Create(&revision).Error; err != nil {
				return err
			}
			linked++
		}
		return nil
	})
	if err != nil || linked+retagged == 0 {
		return linked, err
	}
	_, err = autoConfirmPendingIfEnabled(db)
	return linked, err
}

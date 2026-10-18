package main

import (
	"regexp"
	"strings"

	"gorm.io/gorm"
)

const (
	issuingUnitKey   = "issuing_unit"
	issuingUnitLabel = "หน่วยงานที่ออกคำสั่ง"
	universityName   = "มหาวิทยาลัยขอนแก่น"
)

var (
	documentKindPrefix = regexp.MustCompile(`^(คำสั่ง|ประกาศ|ระเบียบ|บันทึกข้อความ)\s*`)
	issuingUnitPrefix  = regexp.MustCompile(`^(มหาวิทยาลัย|วิทยาลัย|คณะ|สำนัก|กอง|ศูนย์|สถาบัน|ภาควิชา|โรงเรียน|โรงพยาบาล|กรม|กระทรวง)`)
)

// orderIssuer เขียนหน่วยงานที่ออกคำสั่งแบบที่รายงานใช้เป็นคอลัมน์ "ประเภทคำสั่ง"
// เช่น "คำสั่งวิทยาลัยการคอมพิวเตอร์ มหาวิทยาลัยขอนแก่น" → "คำสั่งวิทยาลัยการคอมพิวเตอร์"
// ชื่อมหาวิทยาลัยต่อท้ายหน่วยงานย่อยตัดออก ยกเว้นคำสั่งของมหาวิทยาลัยเอง เอกสารชนิดอื่น (ประกาศ ระเบียบ) คงชนิดเดิม
// ข้อความที่ไม่ได้ขึ้นต้นด้วยชื่อหน่วยงาน (เช่น ชื่อเรื่องจากไฟล์ Word/Excel) ไม่ใช่หน่วยงาน คืนค่าว่างให้ผู้ตรวจกรอกเอง
func orderIssuer(issuedBy string) string {
	unit := strings.Join(strings.Fields(issuedBy), " ")
	kind := "คำสั่ง"
	if match := documentKindPrefix.FindStringSubmatch(unit); match != nil {
		kind, unit = match[1], strings.TrimSpace(unit[len(match[0]):])
	}
	if !issuingUnitPrefix.MatchString(unit) || strings.HasPrefix(unit, "คณะกรรมการ") {
		return ""
	}
	if unit != universityName {
		unit = strings.TrimSpace(strings.TrimSuffix(unit, universityName))
	}
	return truncateRunes(kind+unit, colLimitDocumentIssuingUnit)
}

// backfillIssuingUnits เติมหน่วยงานที่ออกคำสั่งให้เอกสารที่นำเข้าก่อนมีช่องนี้ จากค่า "ออกโดย" ที่ OCR อ่านไว้
// และสร้างแถวช่องให้ผู้ตรวจแก้ได้ ทำซ้ำได้โดยไม่เปลี่ยนเอกสารที่มีค่าแล้ว
func backfillIssuingUnits(db *gorm.DB) error {
	var sources []ExtractedField
	err := db.Table("extracted_fields AS f").Select("f.*").
		Joins("JOIN document_workflows AS d ON d.id = f.document_id").
		Where("f.field_key = ? AND d.issuing_unit = ''", "issued_by").
		Where("NOT EXISTS (SELECT 1 FROM extracted_fields AS u WHERE u.document_id = f.document_id AND u.field_key = ?)", issuingUnitKey).
		Scan(&sources).Error
	if err != nil || len(sources) == 0 {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, source := range sources {
			unit := orderIssuer(source.Value)
			box := source.BoundingBoxJSON
			if box == "" {
				box = "null"
			}
			field := ExtractedField{DocumentID: source.DocumentID, FieldKey: issuingUnitKey, FieldLabel: issuingUnitLabel, FieldType: "text", Value: unit, SourceText: source.SourceText, Confidence: source.Confidence, PageNo: source.PageNo, BoundingBoxJSON: box}
			if err := tx.Create(&field).Error; err != nil {
				return err
			}
			if err := tx.Model(&Document{}).Where("id = ?", source.DocumentID).Update("issuing_unit", unit).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

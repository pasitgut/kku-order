package main

import (
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DirectoryPreviousName struct {
	DirectoryPersonID uint   `gorm:"primaryKey;autoIncrement:false"`
	NormalizedName    string `gorm:"primaryKey"`
	FirstName         string
	LastName          string
	Source            string
	CreatedAt         time.Time
}

func (DirectoryPreviousName) TableName() string { return "directory_previous_names" }

func rememberPreviousName(tx *gorm.DB, person DirectoryPerson, firstName, lastName, source string) error {
	key := normalizePersonName(person.FirstName + " " + person.LastName)
	if key == "" || key == normalizePersonName(firstName+" "+lastName) {
		return nil
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&DirectoryPreviousName{
		DirectoryPersonID: person.ID, NormalizedName: key, FirstName: person.FirstName, LastName: person.LastName, Source: source,
	}).Error
}

type DocumentConfirmation struct {
	ID          uint               `json:"id" gorm:"primaryKey"`
	DocumentID  uint               `json:"-"`
	ConfirmedBy string             `json:"confirmedBy"`
	ConfirmedAt time.Time          `json:"confirmedAt"`
	Names       []ConfirmationName `json:"names" gorm:"serializer:json;type:json"`
}

type ConfirmationName struct {
	AppointmentID     uint   `json:"appointmentId"`
	DirectoryPersonID *uint  `json:"directoryPersonId"`
	DocumentName      string `json:"documentName"`
	Prefix            string `json:"prefix"`
	FirstName         string `json:"firstName"`
	LastName          string `json:"lastName"`
	Position          string `json:"position"`
}

func (name ConfirmationName) displayName() string {
	return chooseNonEmpty(strings.TrimSpace(name.DocumentName), strings.TrimSpace(strings.Join([]string{name.Prefix, name.FirstName, name.LastName}, " ")))
}

func recordDocumentConfirmation(tx *gorm.DB, id uint, by string, at time.Time) error {
	names := []ConfirmationName{}
	if err := tx.Table("appointments AS a").
		Select("a.id AS appointment_id, a.directory_user_id AS directory_person_id, a.full_name AS document_name, a.position, p.prefix, p.first_name, p.last_name").
		Joins("LEFT JOIN directory_people AS p ON p.id = a.directory_user_id").
		Where("a.document_id = ?", id).Order("a.id").Clauses(clause.Locking{Strength: "UPDATE"}).Scan(&names).Error; err != nil {
		return err
	}
	return tx.Create(&DocumentConfirmation{DocumentID: id, ConfirmedBy: by, ConfirmedAt: at, Names: names}).Error
}

func checkConfirmationNames(tx *gorm.DB, id uint) error {
	var names []string
	if err := tx.Model(&Appointment{}).Where("document_id = ? AND TRIM(full_name) <> '' AND COALESCE(name_match_method, '') <> ?", id, matchMethodManual).
		Clauses(clause.Locking{Strength: "UPDATE"}).Pluck("full_name", &names).Error; err != nil || len(names) == 0 {
		return err
	}
	users, aliases, err := loadDirectory(tx)
	if err != nil {
		return err
	}
	for _, name := range names {
		if matchDirectoryUser(name, users, aliases).Method == matchMethodAmbiguous {
			return errAmbiguousConfirmationName
		}
	}
	return nil
}

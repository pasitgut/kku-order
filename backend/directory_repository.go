package main

import (
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

func loadDirectory(db *gorm.DB) ([]DirectoryUser, map[string]uint, error) {
	var people []DirectoryPerson
	if err := db.Where("status IS NULL OR UPPER(status) NOT IN ?", []string{"I", "INACTIVE", "0", "FALSE", "NO"}).Find(&people).Error; err != nil {
		return nil, nil, err
	}
	profiles, err := loadDirectoryProfiles(db, people)
	if err != nil {
		return nil, nil, err
	}
	users := make([]DirectoryUser, 0, len(people))
	for _, person := range people {
		users = append(users, directoryPersonAPI(person, profiles[person.ID]))
	}

	var rows []PersonNameAlias
	if err := db.Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	aliases := make(map[string]uint, len(rows))
	for _, row := range rows {
		aliases[row.NormalizedName] = row.DirectoryUserID
	}
	return users, aliases, nil
}

func loadDirectoryProfiles(db *gorm.DB, people []DirectoryPerson) (map[uint]DirectoryPersonProfile, error) {
	profiles := make(map[uint]DirectoryPersonProfile, len(people))
	if len(people) == 0 {
		return profiles, nil
	}
	ids := make([]uint, 0, len(people))
	for _, person := range people {
		ids = append(ids, person.ID)
	}
	var rows []DirectoryPersonProfile
	if err := db.Where("person_id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		profiles[row.PersonID] = row
	}
	return profiles, nil
}

func directoryPersonAPI(person DirectoryPerson, profile DirectoryPersonProfile) DirectoryUser {
	personID := ""
	if person.ExternalID != nil {
		personID = strconv.FormatInt(*person.ExternalID, 10)
	}
	fullName := person.FullName
	if fullName == "" {
		fullName = strings.TrimSpace(strings.Join([]string{person.Prefix, person.FirstName, person.LastName}, " "))
	}
	phone := ""
	if person.Phone != nil {
		phone = *person.Phone
	}
	email := ""
	if person.Email != nil {
		email = *person.Email
	}
	department := profile.Department
	faculty := profile.Faculty
	source := strings.TrimSpace(profile.Source)
	if source == "" {
		source = sourceSynced
		if person.ExternalID == nil {
			source = sourceManual
		}
	}
	return DirectoryUser{
		ID: person.ID, UserID: personID, ExternalID: personID, Prefix: person.Prefix,
		FullName: fullName, FirstName: person.FirstName, LastName: person.LastName,
		Email: email, Phone: phone, Department: department, Faculty: faculty,
		PositionTitle: person.PositionTitle, JobTitle: person.PositionTitle,
		IsActive: activeStatusCode(person.Status), Source: source, CreatedBy: profile.CreatedBy,
		SourceUpdatedAt: profile.SourceUpdatedAt, LastSeenAt: person.SyncedAt,
		SyncedAt: person.SyncedAt, CreatedAt: person.CreatedAt, UpdatedAt: person.UpdatedAt,
		Fingerprint: profile.Fingerprint,
	}
}

func directoryPersonProfileFromPayload(item map[string]any, source, createdBy, fingerprint, rawPayload string) DirectoryPersonProfile {
	return DirectoryPersonProfile{
		Department: firstString(item, "department", "department_name", "departmentName", "unit"),
		Faculty:    firstString(item, "faculty", "faculty_name", "facultyName"),
		Source:     source, CreatedBy: createdBy,
		SourceUpdatedAt: firstString(item, "updated_at", "updatedAt"),
		RawPayload:      rawPayload, Fingerprint: fingerprint,
	}
}

func activeStatusCode(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "i", "inactive", "false", "0", "no", "n", "disabled":
		return "I"
	default:
		return activeUserCode
	}
}

func normalizePersonStatus(active string) string {
	if activeStatusCode(active) == "I" {
		return "INACTIVE"
	}
	return "ACTIVE"
}

func nonEmptySQLValue(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func enrichAppointmentDirectoryPeople(db *gorm.DB, document *Document) error {
	people := make([]DirectoryPerson, 0, len(document.Appointments))
	seen := make(map[uint]struct{}, len(document.Appointments))
	for _, appointment := range document.Appointments {
		if appointment.DirectoryPerson == nil {
			continue
		}
		if _, ok := seen[appointment.DirectoryPerson.ID]; ok {
			continue
		}
		seen[appointment.DirectoryPerson.ID] = struct{}{}
		people = append(people, *appointment.DirectoryPerson)
	}
	profiles, err := loadDirectoryProfiles(db, people)
	if err != nil {
		return fmt.Errorf("load directory person profiles: %w", err)
	}
	for index := range document.Appointments {
		person := document.Appointments[index].DirectoryPerson
		if person == nil {
			continue
		}
		user := directoryPersonAPI(*person, profiles[person.ID])
		document.Appointments[index].DirectoryUser = &user
	}
	return nil
}

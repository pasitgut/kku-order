package main

import (
	"strings"
	"unicode"
)

func (p *DocumentProcessor) applyDirectoryMatches(metadata *commandMetadata) {
	var users []DirectoryUser
	if err := p.db.Where("is_active = ?", true).Find(&users).Error; err != nil {
		p.logger.Printf("directory matching skipped: %v", err)
		return
	}
	for index := range metadata.appointments {
		appointment := &metadata.appointments[index]
		matched, score, method, ok := findDirectoryUser(appointment.FullName, users)
		if !ok {
			appointment.NameMatchMethod = "NOT_FOUND"
			metadata.lowConfidence = true
			continue
		}
		appointment.DirectoryUserID = &matched.ID
		appointment.NameMatchMethod = method
		appointment.NameMatchScore = score
		appointment.Confidence = minConfidence(appointment.Confidence, score)
		if matched.FullName != "" {
			appointment.FullName = matched.FullName
		}
		if appointment.Position == "" {
			appointment.Position = matched.JobTitle
		}
		if score < 0.90 || appointment.Confidence < 0.90 {
			metadata.lowConfidence = true
		}
	}
}

func findDirectoryUser(name string, users []DirectoryUser) (DirectoryUser, float32, string, bool) {
	normalizedName := normalizePersonName(name)
	if normalizedName == "" {
		return DirectoryUser{}, 0, "", false
	}
	best := DirectoryUser{}
	bestScore := float32(0)
	bestMethod := ""
	for _, user := range users {
		// The external directory's first_name/last_name are the canonical
		// identity. FullName may contain a Thai honorific, so use it only as
		// a fallback when one of the canonical parts is missing.
		candidate := normalizePersonName(strings.TrimSpace(user.FirstName + " " + user.LastName))
		if candidate == "" {
			candidate = normalizePersonName(user.FullName)
		}
		if candidate == "" {
			continue
		}
		if candidate == normalizedName {
			return user, 1, "EXACT", true
		}
		score := similarityScore(normalizedName, candidate)
		if score > bestScore {
			best = user
			bestScore = score
			bestMethod = "FUZZY"
		}
	}
	if bestScore < 0.78 {
		return DirectoryUser{}, 0, "", false
	}
	return best, bestScore, bestMethod, true
}

func normalizePersonName(value string) string {
	value = stripThaiNamePrefixes(value)
	var builder strings.Builder
	for _, character := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsSpace(character) || unicode.IsPunct(character) {
			continue
		}
		builder.WriteRune(character)
	}
	return builder.String()
}

func stripThaiNamePrefixes(value string) string {
	value = strings.TrimSpace(value)
	prefixes := []string{
		"รองศาสตราจารย์ ดร.", "ผู้ช่วยศาสตราจารย์ ดร.", "ศาสตราจารย์ ดร.",
		"รองศาสตราจารย์", "ผู้ช่วยศาสตราจารย์", "ศาสตราจารย์",
		"รศ.ดร.", "ผศ.ดร.", "ศ.ดร.", "รศ. ดร.", "ผศ. ดร.", "ศ. ดร.",
		"ดร.", "รศ.", "ผศ.", "ศ.", "อ.", "อาจารย์", "นาย", "นางสาว", "นาง",
	}
	for {
		before := value
		for _, prefix := range prefixes {
			if strings.HasPrefix(value, prefix) {
				value = strings.TrimSpace(strings.TrimPrefix(value, prefix))
				break
			}
		}
		if value == before {
			return value
		}
	}
}

func similarityScore(left, right string) float32 {
	leftRunes, rightRunes := []rune(left), []rune(right)
	if len(leftRunes) == 0 || len(rightRunes) == 0 {
		return 0
	}
	previous := make([]int, len(rightRunes)+1)
	for index := range previous {
		previous[index] = index
	}
	for leftIndex, leftRune := range leftRunes {
		current := make([]int, len(rightRunes)+1)
		current[0] = leftIndex + 1
		for rightIndex, rightRune := range rightRunes {
			cost := 0
			if leftRune != rightRune {
				cost = 1
			}
			current[rightIndex+1] = minInt(current[rightIndex]+1, previous[rightIndex+1]+1, previous[rightIndex]+cost)
		}
		previous = current
	}
	distance := previous[len(rightRunes)]
	maxLength := len(leftRunes)
	if len(rightRunes) > maxLength {
		maxLength = len(rightRunes)
	}
	return 1 - float32(distance)/float32(maxLength)
}

func minInt(values ...int) int {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

package main

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode"

	"gorm.io/gorm"
)

const (
	matchMethodAlias     = "ALIAS"
	matchMethodExact     = "EXACT"
	matchMethodFuzzy     = "FUZZY"
	matchMethodAmbiguous = "AMBIGUOUS"
	matchMethodNotFound  = "NOT_FOUND"
	matchMethodEmpty     = "DIRECTORY_EMPTY"
	matchMethodManual    = "MANUAL"

	activeUserCode = "A"
	sourceSynced   = "SYNCED"
	sourceManual   = "MANUAL"

	// fuzzyScoreThreshold is the lowest combined score accepted without a
	// reviewer confirming the row. surnameScoreThreshold additionally guards
	// the surname on its own, so an identical given name cannot carry a
	// different person over the line.
	fuzzyScoreThreshold     = 0.78
	surnameScoreThreshold   = 0.75
	candidateScoreThreshold = 0.55
	firstNameWeight         = 0.4
	lastNameWeight          = 0.6
	maxCandidates           = 3
)

// directoryMatch is the full outcome of resolving one OCR name against the
// directory. Method is reported even when nothing matched so the reviewer can
// tell "this person is not in the directory" from "the directory is empty".
type directoryMatch struct {
	User       DirectoryUser
	Score      float32
	Method     string
	Matched    bool
	Candidates []directoryCandidate
}

type directoryCandidate struct {
	UserID        uint    `json:"userId"`
	FullName      string  `json:"fullName"`
	PositionTitle string  `json:"positionTitle"`
	Score         float32 `json:"score"`
}

// matchDirectoryUser resolves an OCR name against the directory. A stored
// alias is a decision a reviewer already made, so it wins over any computed
// score. Anything that does not resolve to exactly one person is reported
// with the reason, so the reviewer sees why rather than a bare "not found".
func matchDirectoryUser(name string, users []DirectoryUser, aliases map[string]uint) directoryMatch {
	if len(users) == 0 {
		return directoryMatch{Method: matchMethodEmpty}
	}
	normalized := normalizePersonName(name)
	if normalized == "" {
		return directoryMatch{Method: matchMethodNotFound}
	}
	if userID, ok := aliases[normalized]; ok {
		for _, user := range users {
			if user.ID == userID {
				return directoryMatch{User: user, Score: 1, Method: matchMethodAlias, Matched: true}
			}
		}
	}

	ocrFirst, ocrLast := splitPersonName(name)
	exact := make([]DirectoryUser, 0, 2)
	best := DirectoryUser{}
	bestScore := float32(0)
	scored := make([]directoryCandidate, 0, len(users))
	for _, user := range users {
		key := directoryUserKey(user)
		if key == "" {
			continue
		}
		if key == normalized {
			exact = append(exact, user)
			continue
		}
		userFirst, userLast := directoryUserNameParts(user)
		lastScore := similarityScore(ocrLast, userLast)
		score := firstNameWeight*similarityScore(ocrFirst, userFirst) + lastNameWeight*lastScore
		if score >= candidateScoreThreshold {
			scored = append(scored, directoryCandidate{UserID: user.ID, FullName: displayDirectoryName(user), PositionTitle: user.PositionTitle, Score: score})
		}
		// A Thai surname carries most of the identity, so a diverging one
		// rules the person out even when the given name is identical.
		if lastScore < surnameScoreThreshold || score < fuzzyScoreThreshold {
			continue
		}
		if score > bestScore {
			best, bestScore = user, score
		}
	}

	switch {
	case len(exact) == 1:
		return directoryMatch{User: exact[0], Score: 1, Method: matchMethodExact, Matched: true}
	case len(exact) > 1:
		return directoryMatch{Method: matchMethodAmbiguous, Candidates: candidatesFor(exact, 1)}
	}
	if bestScore > 0 {
		return directoryMatch{User: best, Score: bestScore, Method: matchMethodFuzzy, Matched: true}
	}
	return directoryMatch{Method: matchMethodNotFound, Candidates: topCandidates(scored)}
}

// splitPersonName treats the final whitespace-separated token as the surname,
// which is how Thai appointment rows are written once the honorific is gone.
func splitPersonName(value string) (first string, last string) {
	parts := strings.Fields(stripThaiNamePrefixes(value))
	if len(parts) == 0 {
		return "", ""
	}
	last = normalizePersonName(parts[len(parts)-1])
	if len(parts) > 1 {
		first = normalizePersonName(strings.Join(parts[:len(parts)-1], " "))
	}
	return first, last
}

func directoryUserNameParts(user DirectoryUser) (first string, last string) {
	if strings.TrimSpace(user.FirstName) != "" && strings.TrimSpace(user.LastName) != "" {
		return normalizePersonName(user.FirstName), normalizePersonName(user.LastName)
	}
	return splitPersonName(user.FullName)
}

func topCandidates(scored []directoryCandidate) []directoryCandidate {
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
	if len(scored) > maxCandidates {
		scored = scored[:maxCandidates]
	}
	return scored
}

// directoryUserKey is the name the directory considers canonical. The
// external directory's first_name/last_name are authoritative; FullName may
// carry a Thai honorific, so it is only a fallback.
func directoryUserKey(user DirectoryUser) string {
	if key := normalizePersonName(strings.TrimSpace(user.FirstName + " " + user.LastName)); key != "" {
		return key
	}
	return normalizePersonName(user.FullName)
}

func candidatesFor(users []DirectoryUser, score float32) []directoryCandidate {
	candidates := make([]directoryCandidate, 0, len(users))
	for _, user := range users {
		candidates = append(candidates, directoryCandidate{
			UserID:        user.ID,
			FullName:      displayDirectoryName(user),
			PositionTitle: user.PositionTitle,
			Score:         score,
		})
	}
	return candidates
}

func displayDirectoryName(user DirectoryUser) string {
	if name := strings.TrimSpace(user.FirstName + " " + user.LastName); name != "" {
		return name
	}
	return user.FullName
}

func (p *DocumentProcessor) applyDirectoryMatches(metadata *commandMetadata) {
	users, aliases, err := loadDirectory(p.db)
	if err != nil {
		p.logger.Printf("directory matching skipped: %v", err)
		return
	}
	for index := range metadata.appointments {
		appointment := &metadata.appointments[index]
		match := matchDirectoryUser(appointment.FullName, users, aliases)
		applyMatchToAppointment(appointment, match)
		if !match.Matched || match.Score < 0.90 || appointment.Confidence < 0.90 {
			metadata.lowConfidence = true
		}
	}
}

// loadDirectory reads the people a name may resolve to, plus the spellings
// reviewers have already bound.
func loadDirectory(db *gorm.DB) ([]DirectoryUser, map[string]uint, error) {
	var users []DirectoryUser
	if err := db.Where("is_active = ?", activeUserCode).Find(&users).Error; err != nil {
		return nil, nil, err
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

func applyMatchToAppointment(appointment *Appointment, match directoryMatch) {
	appointment.NameMatchMethod = match.Method
	appointment.Candidates = match.Candidates
	if !match.Matched {
		appointment.DirectoryUserID = nil
		appointment.NameMatchScore = 0
		return
	}
	userID := match.User.ID
	appointment.DirectoryUserID = &userID
	appointment.NameMatchScore = match.Score
	appointment.Confidence = minConfidence(appointment.Confidence, match.Score)
	if name := displayDirectoryName(match.User); name != "" {
		appointment.FullName = name
	}
	if appointment.Position == "" {
		appointment.Position = chooseNonEmpty(match.User.PositionTitle, match.User.JobTitle)
	}
}

// normalizeBoundingBox guards the JSON columns that store OCR boxes. MySQL
// rejects an empty string outright, and a row without a box is ordinary: OCR
// may miss one, and a reviewer can add a person by hand.
func normalizeBoundingBox(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !json.Valid([]byte(value)) {
		return "[]"
	}
	return value
}

// candidatesToOffer is what the reviewer gets to click on an unlinked row.
// When the name does resolve, the resolved person is the offer: the row is
// unlinked but matchable, which happens when somebody joined the directory
// after this document was processed.
func candidatesToOffer(match directoryMatch) []directoryCandidate {
	if match.Matched {
		return candidatesFor([]DirectoryUser{match.User}, match.Score)
	}
	return match.Candidates
}

// rememberNameAlias stores the spelling OCR produced, not the name the
// reviewer may have typed over it, so the next document with the same slip
// resolves without help.
func rememberNameAlias(tx *gorm.DB, previous Appointment, input appointmentRequest, match directoryMatch, changedBy string) error {
	sourceText := strings.TrimSpace(previous.FullName)
	if sourceText == "" {
		sourceText = strings.TrimSpace(input.FullName)
	}
	key := normalizePersonName(sourceText)
	if key == "" || key == directoryUserKey(match.User) {
		return nil
	}
	alias := PersonNameAlias{NormalizedName: key}
	return tx.Where(PersonNameAlias{NormalizedName: key}).
		Assign(map[string]any{"directory_user_id": match.User.ID, "source_text": sourceText, "created_by": changedBy}).
		FirstOrCreate(&alias).Error
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

// resolveAppointmentDirectoryUser decides who an edited row belongs to. A
// reviewer who picked somebody has already answered the question, so that
// choice is kept as-is; re-matching it by name would silently overwrite it.
func resolveAppointmentDirectoryUser(input appointmentRequest, users []DirectoryUser, aliases map[string]uint) directoryMatch {
	if input.DirectoryUserID != nil && *input.DirectoryUserID != 0 {
		for _, user := range users {
			if user.ID == *input.DirectoryUserID {
				return directoryMatch{User: user, Score: 1, Method: matchMethodManual, Matched: true}
			}
		}
	}
	return matchDirectoryUser(input.FullName, users, aliases)
}

// confirmBlockers names the rows that still have nobody attached. A document
// is not confirmable while this is non-empty.
func confirmBlockers(appointments []Appointment) []string {
	blockers := make([]string, 0)
	for _, appointment := range appointments {
		if appointment.DirectoryUserID == nil || *appointment.DirectoryUserID == 0 {
			blockers = append(blockers, appointment.FullName)
		}
	}
	return blockers
}

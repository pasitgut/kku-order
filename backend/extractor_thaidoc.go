package main

import (
	"encoding/json"
	"errors"
	"strings"
)

type thaidocPage struct {
	Page   int `json:"page"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type thaidocMemberBBox struct {
	Prefix   []Box `json:"prefix"`
	Name     []Box `json:"name"`
	Position []Box `json:"position"`
	Role     []Box `json:"role"`
}

type thaidocMember struct {
	Prefix   string            `json:"prefix"`
	Name     string            `json:"name"`
	Position string            `json:"position"`
	Role     string            `json:"role"`
	BBox     thaidocMemberBBox `json:"bbox"`
}

type thaidocGroup struct {
	Dept     string          `json:"dept"`
	Duty     string          `json:"duty"`
	ListName []thaidocMember `json:"list name"`
	BBox     struct {
		Dept []Box `json:"dept"`
		Duty []Box `json:"duty"`
	} `json:"bbox"`
}

type thaidocResponse struct {
	Subject       string          `json:"subject"`
	Date          string          `json:"date"`
	Order         string          `json:"order"`
	EffectiveDate string          `json:"effective date"`
	Number        string          `json:"number"`
	ExpiryDate    string          `json:"expiry date"`
	RefNumber     []string        `json:"ref_number"`
	Signer        string          `json:"signer"`
	Data          *[]thaidocGroup `json:"data"`
	Pages         []thaidocPage   `json:"pages"`
	BBox          struct {
		Subject       []Box   `json:"subject"`
		Date          []Box   `json:"date"`
		Order         []Box   `json:"order"`
		EffectiveDate []Box   `json:"effective date"`
		Number        []Box   `json:"number"`
		ExpiryDate    []Box   `json:"expiry date"`
		RefNumber     [][]Box `json:"ref_number"`
		Signer        []Box   `json:"signer"`
	} `json:"bbox"`
}

func thaidocValue(text string, boxes []Box) ExtractedValue {
	return ExtractedValue{Text: strings.TrimSpace(text), Boxes: boxes}
}

// parseThaidocResponse แปลงคำตอบของ thaidoc `/api/v1/extract-json` เป็น ExtractionResult
// thaidoc ไม่ให้ค่าความมั่นใจ จึงทุกค่า Confidence เป็น nil
func parseThaidocResponse(raw []byte) (ExtractionResult, error) {
	var resp thaidocResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return ExtractionResult{}, err
	}
	if resp.Data == nil {
		return ExtractionResult{}, errors.New("thaidoc response has no data key")
	}

	result := ExtractionResult{
		Engine: "thaidoc",
		Header: ExtractedHeader{
			OrderNo:       thaidocValue(resp.Number, resp.BBox.Number),
			Subject:       thaidocValue(resp.Subject, resp.BBox.Subject),
			IssuedBy:      thaidocValue(resp.Order, resp.BBox.Order),
			Date:          thaidocValue(resp.Date, resp.BBox.Date),
			EffectiveDate: thaidocValue(resp.EffectiveDate, resp.BBox.EffectiveDate),
			ExpiryDate:    thaidocValue(resp.ExpiryDate, resp.BBox.ExpiryDate),
			Signer:        thaidocValue(resp.Signer, resp.BBox.Signer),
		},
	}
	for i, ref := range resp.RefNumber {
		var boxes []Box
		if i < len(resp.BBox.RefNumber) {
			boxes = resp.BBox.RefNumber[i]
		}
		result.Header.References = append(result.Header.References, thaidocValue(ref, boxes))
	}

	for _, g := range *resp.Data {
		group := ExtractedGroup{
			Department: thaidocValue(g.Dept, g.BBox.Dept),
			Duty:       thaidocValue(g.Duty, g.BBox.Duty),
		}
		for _, m := range g.ListName {
			// ข้ามแถวที่ทั้งชื่อและตำแหน่งว่าง (ผลจากเอกสารที่ไม่ใช่คำสั่ง)
			if strings.TrimSpace(m.Name) == "" && strings.TrimSpace(m.Position) == "" {
				continue
			}
			member := ExtractedMember{
				Prefix:   thaidocValue(m.Prefix, m.BBox.Prefix),
				Name:     thaidocValue(m.Name, m.BBox.Name),
				Position: thaidocValue(m.Position, m.BBox.Position),
				Role:     thaidocValue(m.Role, m.BBox.Role),
			}
			member.ExOfficio = member.Name.Text == "" && member.Position.Text != ""
			group.Members = append(group.Members, member)
		}
		result.Groups = append(result.Groups, group)
	}

	for _, p := range resp.Pages {
		result.Pages = append(result.Pages, ExtractedPage{Page: p.Page, Width: p.Width, Height: p.Height})
	}
	return result, nil
}

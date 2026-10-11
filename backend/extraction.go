package main

import (
	"context"
	"fmt"
)

// ExtractionResult คือผลกลางที่ทุก adapter ต้องคืน (JSON ของ type นี้คือ DocFlow contract)
type ExtractionResult struct {
	Engine string           `json:"engine"`
	Header ExtractedHeader  `json:"header"`
	Groups []ExtractedGroup `json:"groups"`
	Pages  []ExtractedPage  `json:"pages,omitempty"`
}

type Box struct {
	Page int     `json:"page"`
	X0   float64 `json:"x0"`
	Y0   float64 `json:"y0"`
	X1   float64 `json:"x1"`
	Y1   float64 `json:"y1"`
}

type ExtractedValue struct {
	Text       string   `json:"text"`
	Confidence *float32 `json:"confidence,omitempty"`
	Boxes      []Box    `json:"boxes,omitempty"`
}

type ExtractedHeader struct {
	OrderNo       ExtractedValue   `json:"orderNo"`
	Subject       ExtractedValue   `json:"subject"`
	IssuedBy      ExtractedValue   `json:"issuedBy"`
	Date          ExtractedValue   `json:"date"`
	EffectiveDate ExtractedValue   `json:"effectiveDate"`
	ExpiryDate    ExtractedValue   `json:"expiryDate"`
	Signer        ExtractedValue   `json:"signer"`
	References    []ExtractedValue `json:"references"`
}

type ExtractedGroup struct {
	Department ExtractedValue    `json:"department"`
	Duty       ExtractedValue    `json:"duty"`
	Members    []ExtractedMember `json:"members"`
}

type ExtractedMember struct {
	Prefix    ExtractedValue `json:"prefix"`
	Name      ExtractedValue `json:"name"`
	Position  ExtractedValue `json:"position"`
	Role      ExtractedValue `json:"role"`
	ExOfficio bool           `json:"exOfficio"`
}

type ExtractedPage struct {
	Page   int              `json:"page"`
	Width  int              `json:"width"`
	Height int              `json:"height"`
	Lines  []ExtractedValue `json:"lines,omitempty"`
}

// Extractor คือ adapter ของ OCR API หนึ่งเจ้า
type Extractor interface {
	Name() string
	Extract(ctx context.Context, pdfPath, filename string) (ExtractionResult, error)
}

// ExtractError คือ error ที่คิวใช้ตัดสินว่าจะลองใหม่หรือไม่
type ExtractError struct {
	Retryable bool
	Status    int    // HTTP status, 0 ถ้าไม่ได้คำตอบ
	Message   string // ภาษาไทย แสดงในหน้าเว็บ
}

func (e *ExtractError) Error() string {
	return fmt.Sprintf("%s (status=%d retryable=%t)", e.Message, e.Status, e.Retryable)
}

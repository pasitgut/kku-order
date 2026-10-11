package main

import (
	"encoding/json"
	"errors"
)

// parseContractResponse อ่าน JSON ที่ตรงกับ DocFlow contract (ดู docs/ocr-api-contract.md)
// ต้องมี key "groups" (ว่างได้) มิฉะนั้นถือว่าไม่ใช่ contract
func parseContractResponse(raw []byte) (ExtractionResult, error) {
	var probe struct {
		Groups *[]ExtractedGroup `json:"groups"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return ExtractionResult{}, err
	}
	if probe.Groups == nil {
		return ExtractionResult{}, errors.New("contract response has no groups key")
	}
	var result ExtractionResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return ExtractionResult{}, err
	}
	if result.Engine == "" {
		result.Engine = "docflow-contract"
	}
	return result, nil
}

package main

import (
	"bytes"
	"testing"
)

func TestValidateFileSignatureRejectsMismatchedPDF(t *testing.T) {
	if err := validateFileSignature(bytes.NewReader([]byte("not a pdf")), ".pdf"); err == nil {
		t.Fatal("validateFileSignature accepted a non-PDF payload")
	}
}

func TestValidateFileSignatureAcceptsPDFAndOfficeZip(t *testing.T) {
	if err := validateFileSignature(bytes.NewReader([]byte("%PDF-1.7\n")), ".pdf"); err != nil {
		t.Fatalf("validateFileSignature rejected PDF: %v", err)
	}
	if err := validateFileSignature(bytes.NewReader([]byte("PK\x03\x04")), ".docx"); err != nil {
		t.Fatalf("validateFileSignature rejected DOCX container: %v", err)
	}
}

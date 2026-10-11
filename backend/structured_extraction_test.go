package main

import (
	"testing"
	"time"
)

func TestStructuredExtractionTagsEngineAndKeepsPageSizes(t *testing.T) {
	result := workerResult{Pages: []workerPage{{
		PageNo: 1, ImageWidth: 1654, ImageHeight: 2339, FullText: "คำสั่งแต่งตั้ง",
		Lines: []workerLine{{Text: "คำสั่งแต่งตั้ง", Confidence: 0.9}},
	}}}
	got := structuredExtraction(result)
	if got.Engine != structuredImportEngine {
		t.Fatalf("Engine = %q", got.Engine)
	}
	if len(got.Pages) != 1 || got.Pages[0].Width != 1654 || len(got.Pages[0].Lines) != 1 {
		t.Fatalf("Pages = %+v", got.Pages)
	}
	if got.Header.Subject.Confidence == nil {
		t.Fatal("structured values must carry the page average confidence so the threshold rule applies")
	}
}

func TestThaiDateTextRoundTripsThroughParser(t *testing.T) {
	d := time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	got := parseThaiDate(thaiDateText(&d))
	if got == nil || !got.Equal(d) {
		t.Fatalf("parseThaiDate(%q) = %v, want %v", thaiDateText(&d), got, d)
	}
}

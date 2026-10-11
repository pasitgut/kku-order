package main

import (
	"strings"
	"testing"
	"time"
)

func TestPendingOrderUsesDatabaseAssignedDocumentID(t *testing.T) {
	now := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	workflow := &Document{Title: strings.Repeat("ก", 300)}

	order := pendingOrderForDocument(5, 7, workflow, now)

	if order.DocumentID != 5 {
		t.Fatalf("DocumentID = %d, want the id supplied by the database (5)", order.DocumentID)
	}
	if order.OrderTypeID != 7 {
		t.Fatalf("OrderTypeID = %d, want 7", order.OrderTypeID)
	}
	if order.OrderNumber != "PENDING-5" {
		t.Fatalf("OrderNumber = %q, want PENDING-5", order.OrderNumber)
	}
	if order.OrderYear != 2569 {
		t.Fatalf("OrderYear = %d, want Buddhist year 2569", order.OrderYear)
	}
	if got := len([]rune(order.Title)); got != 255 {
		t.Fatalf("Title has %d runes, want it truncated to 255", got)
	}
	if !order.CreatedAt.Equal(now) || !order.UpdatedAt.Equal(now) {
		t.Fatalf("timestamps = %v / %v, want %v", order.CreatedAt, order.UpdatedAt, now)
	}
}

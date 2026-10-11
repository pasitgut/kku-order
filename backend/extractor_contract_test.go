package main

import (
	"os"
	"testing"
)

func TestParseContractResponseReadsFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/extraction/docflow-contract.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	got, err := parseContractResponse(raw)
	if err != nil {
		t.Fatalf("parseContractResponse: %v", err)
	}
	if got.Engine != "docflow-contract" {
		t.Fatalf("Engine = %q", got.Engine)
	}
	if got.Header.OrderNo.Text != "1234/2569" || got.Header.OrderNo.Confidence == nil || *got.Header.OrderNo.Confidence != 0.94 {
		t.Fatalf("OrderNo = %+v", got.Header.OrderNo)
	}
	if len(got.Header.References) != 1 || got.Header.References[0].Text != "ที่ ศธ 0001/2569" {
		t.Fatalf("References = %+v", got.Header.References)
	}
	if len(got.Groups) != 1 || len(got.Groups[0].Members) != 2 {
		t.Fatalf("Groups = %+v", got.Groups)
	}
	if got.Groups[0].Members[0].Name.Boxes[0].X0 != 0.2 {
		t.Fatalf("member name boxes = %+v", got.Groups[0].Members[0].Name.Boxes)
	}
	if !got.Groups[0].Members[1].ExOfficio {
		t.Fatal("second member should be ExOfficio")
	}
	if len(got.Pages) != 1 || got.Pages[0].Width != 1654 || len(got.Pages[0].Lines) != 1 {
		t.Fatalf("Pages = %+v", got.Pages)
	}
}

func TestParseContractResponseRequiresGroupsKey(t *testing.T) {
	_, err := parseContractResponse([]byte(`{"engine":"docflow-contract","header":{}}`))
	if err == nil {
		t.Fatal("expected error when groups key is missing")
	}
}

package main

import (
	"reflect"
	"testing"
)

func TestParseStatusFilterAcceptsSeveralStatuses(t *testing.T) {
	cases := map[string][]string{
		"":                        nil,
		"ALL":                     nil,
		"REVIEW":                  {"REVIEW"},
		"REVIEW,NEEDS_REVIEW":     {"REVIEW", "NEEDS_REVIEW"},
		" REVIEW , ,NEEDS_REVIEW": {"REVIEW", "NEEDS_REVIEW"},
		"REVIEW,ALL":              nil,
	}
	for raw, want := range cases {
		if got := parseStatusFilter(raw); !reflect.DeepEqual(got, want) {
			t.Errorf("parseStatusFilter(%q) = %v, want %v", raw, got, want)
		}
	}
}

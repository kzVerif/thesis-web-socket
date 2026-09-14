package virusscan

import (
	"strings"
	"testing"
	"time"
)

func TestMultipleTargets(t *testing.T) {
	a := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	b := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	for _, req := range []Request{
		{ScanType: "quick"},
		{AgentID: a, AgentIDs: []string{b}, ScanType: "quick"},
		{AgentIDs: []string{a, strings.ToUpper(a)}, ScanType: "quick"},
		{AgentIDs: []string{a, "invalid"}, ScanType: "quick"},
		{AgentIDs: make([]string, 101), ScanType: "quick"},
		{AgentIDs: []string{a}, JobID: b, ScanType: "quick"},
	} {
		if req.Validate() == nil {
			t.Fatalf("accepted invalid request: %+v", req)
		}
	}
	req := Request{AgentIDs: []string{strings.ToUpper(a), b}, ScanType: "quick"}
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}
	ids, _ := req.TargetIDs()
	if len(ids) != 2 || ids[0] != a {
		t.Fatalf("unexpected targets: %v", ids)
	}
}

func TestRequestValidation(t *testing.T) {
	for _, tc := range []struct {
		kind, path string
		valid      bool
	}{
		{"quick", "", true}, {"full", "", true}, {"custom", `C:\Users\Public`, true}, {"custom", `\\server\share\folder`, true},
		{"custom", "relative", false}, {"custom", `C:folder`, false}, {"custom", `C:\*.exe`, false}, {"custom", `\\.\C:\`, false}, {"quick", `C:\`, false}, {"unknown", "", false},
	} {
		t.Run(tc.kind+tc.path, func(t *testing.T) {
			r := Request{AgentID: "11111111-1111-1111-1111-111111111111", ScanType: tc.kind, Path: tc.path}
			if (r.Validate() == nil) != tc.valid {
				t.Fatalf("unexpected validation: %v", r.Validate())
			}
		})
	}
}
func TestEventValidation(t *testing.T) {
	now := time.Now().UTC()
	e := Event{Type: "virus_scan_result", RequestID: "11111111-1111-1111-1111-111111111111", ScanType: "quick", Status: "rejected", FinishedAt: &now, Error: "busy"}
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	e.Status = "completed"
	if e.Validate() == nil {
		t.Fatal("accepted missing completed report")
	}
	e.StartedAt = &now
	e.Report = &Report{ExitCode: 0}
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	e.Report.ExitCode = 2
	if e.Validate() == nil {
		t.Fatal("accepted nonzero completed exit code")
	}
	e.Status = "failed"
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	e.Type = "virus_scan_status"
	if e.Validate() == nil {
		t.Fatal("accepted terminal status as running event")
	}
}

package virusscan

import "testing"

func TestDashboardCountsLoadedJobsAndAllTargets(t *testing.T) {
	d := NewDashboard([]JobRecord{
		{Scans: []Record{{Status: "QUEUED"}, {Status: "DELIVERED"}, {Status: "RUNNING"}}},
		{Scans: []Record{{Status: "SUCCEEDED"}, {Status: "FAILED"}, {Status: "CANCELLED"}, {Status: "EXPIRED"}}},
	})
	if d.Summary != (Summary{LoadedJobs: 2, Pending: 3, Succeeded: 1, Failed: 3}) {
		t.Fatalf("summary: %+v", d.Summary)
	}
	if empty := NewDashboard(nil); empty.Jobs == nil || empty.Summary != (Summary{}) {
		t.Fatalf("empty: %+v", empty)
	}
}

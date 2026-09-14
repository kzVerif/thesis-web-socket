package wsserver

import "testing"

func TestProcessKillTrackerRejectsDuplicatePendingCommand(t *testing.T) {
	tracker := NewProcessKillTracker()
	frontend := &FrontendClient{}
	if err := tracker.Register("agent-1", 8120, frontend); err != nil {
		t.Fatal(err)
	}
	defer tracker.Cancel("agent-1", 8120, frontend)
	if err := tracker.Register("agent-1", 8120, frontend); err == nil {
		t.Fatal("duplicate pending process kill should be rejected")
	}
}

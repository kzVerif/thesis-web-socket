package distribution

import "testing"

func TestValidateDeduplicatesAgents(t *testing.T) {
	r := CreateRequest{FileID: "11111111-1111-4111-8111-111111111111", Target: Target{Type: "AGENTS", AgentIDs: []string{"22222222-2222-4222-8222-222222222222", "22222222-2222-4222-8222-222222222222"}}}
	if err := Validate(&r); err != nil {
		t.Fatal(err)
	}
	if len(r.Target.AgentIDs) != 1 {
		t.Fatalf("got %d", len(r.Target.AgentIDs))
	}
}
func TestValidateRejectsInvalidUUID(t *testing.T) {
	r := CreateRequest{FileID: "bad", Target: Target{Type: "ROOM", RoomID: "bad"}}
	if Validate(&r) == nil {
		t.Fatal("expected error")
	}
}

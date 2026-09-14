package wsserver

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFrontendAuditExcludesPayloadAndBoundsStrings(t *testing.T) {
	raw := json.RawMessage(`{"type":"process","action":"kill","pid":42,"token":"secret","payload":"large","agent_id":"` + strings.Repeat("x", 1000) + `"}`)
	detail, _ := frontendAuditDetail(raw)
	encoded, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "large") {
		t.Fatal("unexpected payload in audit")
	}
	if len(detail["agent_id"].(string)) != 128 || detail["pid"] != int64(42) {
		t.Fatalf("invalid summary: %v", detail)
	}
}

func TestPeerIP(t *testing.T) {
	for input, want := range map[string]string{"127.0.0.1:1234": "127.0.0.1", "[::1]:1234": "::1", "invalid": ""} {
		if got := peerIP(input); got != want {
			t.Errorf("peerIP(%q) = %q; want %q", input, got, want)
		}
	}
}

package wsserver

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"ws-rat/internal/virusscan"
)

type watchScanStore struct {
	fakeScanStore
	mu     sync.Mutex
	status string
	deny   bool
	users  chan string
}

func (s *watchScanStore) Allowed(context.Context, string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.deny, nil
}
func (s *watchScanStore) Snapshot(_ context.Context, user string, req virusscan.Request) (virusscan.Dashboard, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users <- user
	return virusscan.NewDashboard([]virusscan.JobRecord{{ID: req.JobID, Scans: []virusscan.Record{{RequestID: scanTestID, Status: s.status}}}}), nil
}

func TestScanWatchSnapshotPushAndPermissionRevocation(t *testing.T) {
	store := &watchScanStore{status: "RUNNING", users: make(chan string, 10)}
	s := New(nil, log.New(io.Discard, "", 0), nil)
	s.sessions = validFrontendSession{}
	s.ConfigureVirusScan(store)
	h := httptest.NewServer(http.HandlerFunc(s.HandleFrontendWebSocket))
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(h.URL, "http"), &websocket.DialOptions{HTTPHeader: http.Header{"Cookie": {"__Host-session=test-session"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	if err := wsjson.Write(ctx, c, map[string]any{"type": "virus_scan_subscribe", "job_id": scanTestID, "limit": 1}); err != nil {
		t.Fatal(err)
	}
	var first virusscan.Dashboard
	if err := wsjson.Read(ctx, c, &first); err != nil {
		t.Fatal(err)
	}
	if first.Type != "virus_scan_snapshot" || first.Summary.Pending != 1 || first.Jobs[0].ID != scanTestID {
		t.Fatalf("snapshot: %+v", first)
	}
	if user := <-store.users; user != "user" {
		t.Fatalf("owner: %s", user)
	}
	store.mu.Lock()
	store.status = "SUCCEEDED"
	store.mu.Unlock()
	s.notifyScanWatchers()
	var next virusscan.Dashboard
	if err := wsjson.Read(ctx, c, &next); err != nil {
		t.Fatal(err)
	}
	if next.Summary.Succeeded != 1 || next.Summary.Pending != 0 {
		t.Fatalf("push: %+v", next)
	}
	store.mu.Lock()
	store.deny = true
	store.mu.Unlock()
	s.notifyScanWatchers()
	var denied map[string]any
	if err := wsjson.Read(ctx, c, &denied); err != nil {
		t.Fatal(err)
	}
	if denied["type"] != "error" || denied["error"] != "ไม่มี permission" {
		t.Fatalf("denial: %v", denied)
	}
	if err := wsjson.Write(ctx, c, map[string]string{"type": "virus_scan_unsubscribe"}); err != nil {
		t.Fatal(err)
	}
	var stopped map[string]any
	if err := wsjson.Read(ctx, c, &stopped); err != nil {
		t.Fatal(err)
	}
	if stopped["type"] != "unsubscribed" {
		t.Fatalf("stop: %v", stopped)
	}
	s.scanWatchMu.Lock()
	count := len(s.scanWatchers)
	s.scanWatchMu.Unlock()
	if count != 0 {
		t.Fatalf("leaked watchers: %d", count)
	}
}

func TestScanWatchRejectsInvalidFilters(t *testing.T) {
	s := New(nil, log.New(io.Discard, "", 0), nil)
	s.ConfigureVirusScan(&watchScanStore{})
	for _, raw := range []string{
		`{"type":"virus_scan_subscribe","job_id":"bad"}`,
		`{"type":"virus_scan_subscribe","limit":101}`,
		`{"type":"virus_scan_subscribe","agent_id":"anything"}`,
	} {
		var stop func()
		if err := s.configureScanWatch(context.Background(), nil, "user", "token", json.RawMessage(raw), &stop); err == nil {
			t.Fatalf("accepted %s", raw)
		}
		if stop != nil {
			stop()
			t.Fatal("started invalid subscription")
		}
	}
}

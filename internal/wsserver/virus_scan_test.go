package wsserver

import (
	"context"
	"encoding/json"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"ws-rat/internal/model"
	"ws-rat/internal/virusscan"
)

const scanTestID = "11111111-1111-1111-1111-111111111111"

type fakeScanStore struct {
	allowed bool
	created chan virusscan.Request
	applied chan virusscan.Event
	agents  chan string
}

func (s *fakeScanStore) Allowed(context.Context, string) (bool, error) { return s.allowed, nil }
func (s *fakeScanStore) Create(_ context.Context, _ string, r virusscan.Request) (virusscan.Job, error) {
	s.created <- r
	ids, _ := r.TargetIDs()
	job := virusscan.Job{ID: "22222222-2222-2222-2222-222222222222"}
	for i, id := range ids {
		commandID := scanTestID
		if i > 0 {
			commandID = "33333333-3333-3333-3333-333333333333"
		}
		job.Targets = append(job.Targets, virusscan.Target{AgentID: id, RequestID: commandID})
	}
	return job, nil
}
func (s *fakeScanStore) Dispatched(context.Context, string, error) error { return nil }
func (s *fakeScanStore) Apply(_ context.Context, agent string, e virusscan.Event) error {
	s.agents <- agent
	s.applied <- e
	return nil
}
func (s *fakeScanStore) List(context.Context, string, virusscan.Request) ([]virusscan.Record, error) {
	return []virusscan.Record{}, nil
}

func TestVirusScanRoundTrip(t *testing.T) {
	agentStore := &fakeAgentStore{agent: model.AgentInfo{ID: scanTestID}, statuses: make(chan string, 2)}
	scans := &fakeScanStore{allowed: true, created: make(chan virusscan.Request, 1), applied: make(chan virusscan.Event, 2), agents: make(chan string, 2)}
	server := New(agentStore, log.New(io.Discard, "", 0), nil)
	server.ConfigureVirusScan(scans)
	handlerErrors := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", server.HandleWebSocket)
	// The normal endpoint authenticates through PostgreSQL. Supply its authenticated user here.
	mux.HandleFunc("/frontend", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			handlerErrors <- err
			return
		}
		defer c.CloseNow()
		var raw json.RawMessage
		if err = wsjson.Read(r.Context(), c, &raw); err == nil {
			err = server.handleVirusScanFrontend(&FrontendClient{Conn: c}, "user", raw)
		}
		handlerErrors <- err
	})
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()
	base := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	agent, _, err := websocket.Dial(ctx, base+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer agent.CloseNow()
	authenticateTestAgent(t, ctx, agent, scanTestID)
	if err = wsjson.Write(ctx, agent, model.AgentInfo{ID: scanTestID}); err != nil {
		t.Fatal(err)
	}
	// Registration's status update precedes registry insertion; wait for the registry itself.
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for !server.Online(scanTestID) {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	frontend, _, err := websocket.Dial(ctx, base+"/frontend", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer frontend.CloseNow()
	if err = wsjson.Write(ctx, frontend, virusscan.Request{Type: "virus_scan", AgentID: scanTestID, ScanType: "quick"}); err != nil {
		t.Fatal(err)
	}
	var command map[string]any
	if err = wsjson.Read(ctx, agent, &command); err != nil {
		t.Fatal(err)
	}
	if command["type"] != "virus_scan" || command["request_id"] != scanTestID || command["scan_type"] != "quick" {
		t.Fatalf("bad command: %#v", command)
	}
	select {
	case <-scans.created:
	default:
		t.Fatal("command sent before creation")
	}
	var ack map[string]any
	if err = wsjson.Read(ctx, frontend, &ack); err != nil {
		t.Fatal(err)
	}
	if ack["dispatch"] != "sent" || ack["request_id"] != scanTestID || ack["job_id"] != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("bad acknowledgement: %#v", ack)
	}
	if err = <-handlerErrors; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, e := range []virusscan.Event{
		{Type: "virus_scan_status", RequestID: scanTestID, ScanType: "quick", Status: "running", StartedAt: &now},
		{Type: "virus_scan_result", RequestID: scanTestID, ScanType: "quick", Status: "completed", StartedAt: &now, FinishedAt: &now, Report: &virusscan.Report{ExitCode: 0, Output: "Defender output"}},
	} {
		if err = wsjson.Write(ctx, agent, e); err != nil {
			t.Fatal(err)
		}
		select {
		case saved := <-scans.applied:
			if saved.Status != e.Status || <-scans.agents != scanTestID {
				t.Fatal("incorrect persisted event")
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}

func TestVirusScanMultiAgentDispatch(t *testing.T) {
	server := New(nil, log.New(io.Discard, "", 0), nil)
	scans := &fakeScanStore{allowed: true, created: make(chan virusscan.Request, 1)}
	server.ConfigureVirusScan(scans)
	commands := make(chan map[string]string, 2)
	ready := make(chan struct{}, 2)
	errs := make(chan error, 3)
	mux := http.NewServeMux()
	mux.HandleFunc("/agent", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			errs <- err
			return
		}
		defer conn.CloseNow()
		id := r.URL.Query().Get("id")
		server.registry.Add(&Client{Info: model.AgentInfo{ID: id}, Conn: conn})
		ready <- struct{}{}
		_, _, _ = conn.Read(r.Context()) // Keep the server connection alive until the client closes.
	})
	mux.HandleFunc("/frontend", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			errs <- err
			return
		}
		defer conn.CloseNow()
		var raw json.RawMessage
		if err = wsjson.Read(r.Context(), conn, &raw); err == nil {
			err = server.handleVirusScanFrontend(&FrontendClient{Conn: conn}, "user", raw)
		}
		if err != nil {
			errs <- err
		}
	})
	host := httptest.NewServer(mux)
	defer host.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	base := "ws" + strings.TrimPrefix(host.URL, "http")
	ids := []string{scanTestID, "44444444-4444-4444-4444-444444444444"}
	for _, id := range ids {
		conn, _, err := websocket.Dial(ctx, base+"/agent?id="+id, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.CloseNow()
		go func(agentID string, agentConn *websocket.Conn) {
			var command map[string]string
			if err := wsjson.Read(ctx, agentConn, &command); err != nil {
				errs <- err
				return
			}
			command["agent_id"] = agentID
			commands <- command
		}(id, conn)
		select {
		case <-ready:
		case err := <-errs:
			t.Fatal(err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	frontend, _, err := websocket.Dial(ctx, base+"/frontend", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer frontend.CloseNow()
	if err = wsjson.Write(ctx, frontend, virusscan.Request{Type: "virus_scan", AgentIDs: ids, ScanType: "full"}); err != nil {
		t.Fatal(err)
	}
	var ack struct {
		Type    string             `json:"type"`
		JobID   string             `json:"job_id"`
		Targets []virusscan.Target `json:"targets"`
	}
	if err = wsjson.Read(ctx, frontend, &ack); err != nil {
		t.Fatal(err)
	}
	if ack.Type != "virus_scan_accepted" || ack.JobID == "" || len(ack.Targets) != 2 {
		t.Fatalf("bad acknowledgement: %+v", ack)
	}
	seen := map[string]string{}
	for range ids {
		select {
		case command := <-commands:
			if command["type"] != "virus_scan" || command["scan_type"] != "full" {
				t.Fatalf("bad command: %v", command)
			}
			seen[command["agent_id"]] = command["request_id"]
		case err := <-errs:
			t.Fatal(err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if seen[ids[0]] == seen[ids[1]] {
		t.Fatal("agents received the same request ID")
	}
	for _, target := range ack.Targets {
		if target.Dispatch != "sent" || target.RequestID != seen[target.AgentID] {
			t.Fatalf("target mismatch: %+v", target)
		}
	}
}

func TestVirusScanDeniedAndOffline(t *testing.T) {
	scans := &fakeScanStore{}
	server := New(nil, log.New(io.Discard, "", 0), nil)
	server.ConfigureVirusScan(scans)
	raw := json.RawMessage(`{"type":"virus_scan","agent_id":"` + scanTestID + `","scan_type":"quick"}`)
	if err := server.handleVirusScanFrontend(nil, "user", raw); err == nil || err.Error() != "ไม่มี permission" {
		t.Fatalf("expected ไม่มี permission: %v", err)
	}
	scans.allowed = true
	if err := server.handleVirusScanFrontend(nil, "user", raw); err == nil || err.Error() != "agent is offline" {
		t.Fatalf("expected offline: %v", err)
	}
	if err := server.handleVirusScanFrontend(nil, "", raw); err == nil {
		t.Fatal("accepted unauthenticated scan")
	}
}

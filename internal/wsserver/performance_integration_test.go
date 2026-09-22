package wsserver

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"ws-rat/internal/model"
)

type fakeAgentStore struct {
	agent    model.AgentInfo
	statuses chan string
}

func (store *fakeAgentStore) GetByID(context.Context, string) (*model.AgentInfo, error) {
	agent := store.agent
	return &agent, nil
}

func (store *fakeAgentStore) UpdateStatus(_ context.Context, _ string, status string) error {
	store.statuses <- status
	return nil
}

func TestFrontendPerformanceRoundTrip(t *testing.T) {
	store := &fakeAgentStore{
		agent:    model.AgentInfo{ID: authTestID, Hostname: "test-agent"},
		statuses: make(chan string, 2),
	}
	server := New(store, log.New(io.Discard, "", 0), []string{"frontend.test"})
	server.sessions = validFrontendSession{}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", server.HandleWebSocket)
	mux.HandleFunc("/ws/frontend", server.HandleFrontendWebSocket)
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	agentConn, _, err := websocket.Dial(ctx, wsURL+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer agentConn.CloseNow()
	authenticateTestAgent(t, ctx, agentConn, authTestID)
	if err := wsjson.Write(ctx, agentConn, model.AgentInfo{ID: authTestID}); err != nil {
		t.Fatal(err)
	}
	select {
	case status := <-store.statuses:
		if status != model.StatusOnline {
			t.Fatalf("unexpected status: %s", status)
		}
	case <-ctx.Done():
		t.Fatal("agent registration timed out")
	}

	frontendConn, _, err := websocket.Dial(ctx, wsURL+"/ws/frontend", &websocket.DialOptions{
		HTTPHeader: http.Header{"Cookie": []string{"__Host-session=test-session"}, "Origin": []string{"http://frontend.test"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer frontendConn.CloseNow()
	request := StreamRequest{Type: "performance", Action: "start", AgentID: authTestID}
	if err := wsjson.Write(ctx, frontendConn, request); err != nil {
		t.Fatal(err)
	}

	var command StreamCommand
	if err := wsjson.Read(ctx, agentConn, &command); err != nil {
		t.Fatal(err)
	}
	if command.Type != "performance" || command.Action != "start" {
		t.Fatalf("unexpected agent command: %+v", command)
	}
	var acknowledgement map[string]any
	if err := wsjson.Read(ctx, frontendConn, &acknowledgement); err != nil {
		t.Fatal(err)
	}
	if acknowledgement["type"] != "subscribed" {
		t.Fatalf("unexpected acknowledgement: %+v", acknowledgement)
	}

	sample := model.PerformanceSample{CPUUsage: 25, RAMTotalGB: 16, RAMUsedGB: 8, RAMUsage: 50, DiskTotalGB: 500, DiskUsedGB: 200, DiskFreeGB: 300, DiskUsage: 40}
	if err := wsjson.Write(ctx, agentConn, sample); err != nil {
		t.Fatal(err)
	}
	var event model.PerformanceEvent
	if err := wsjson.Read(ctx, frontendConn, &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "performance" || event.AgentID != authTestID || event.Data.CPUUsage != 25 {
		t.Fatalf("unexpected performance event: %+v", event)
	}

	if err := wsjson.Write(ctx, frontendConn, StreamRequest{Type: "process", Action: "start", AgentID: authTestID}); err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Read(ctx, agentConn, &command); err != nil {
		t.Fatal(err)
	}
	if command.Type != "process" || command.Action != "start" {
		t.Fatalf("unexpected process command: %+v", command)
	}
	if err := wsjson.Read(ctx, frontendConn, &acknowledgement); err != nil {
		t.Fatal(err)
	}
	processes := []model.ProcessInfo{{PID: 1324, Name: "systemd"}, {PID: 8120, Name: "node"}}
	if err := wsjson.Write(ctx, agentConn, processes); err != nil {
		t.Fatal(err)
	}
	var processEvent model.ProcessEvent
	if err := wsjson.Read(ctx, frontendConn, &processEvent); err != nil {
		t.Fatal(err)
	}
	if processEvent.Type != "process" || processEvent.AgentID != authTestID || len(processEvent.Data) != 2 {
		t.Fatalf("unexpected process event: %+v", processEvent)
	}

	if err := wsjson.Write(ctx, frontendConn, StreamRequest{Type: "process", Action: "kill", AgentID: authTestID, PID: 8120}); err != nil {
		t.Fatal(err)
	}
	var killAccepted map[string]any
	if err := wsjson.Read(ctx, frontendConn, &killAccepted); err != nil {
		t.Fatal(err)
	}
	if killAccepted["action"] != "kill_accepted" {
		t.Fatalf("unexpected kill acknowledgement: %+v", killAccepted)
	}
	if err := wsjson.Read(ctx, agentConn, &command); err != nil {
		t.Fatal(err)
	}
	if command.Type != "process" || command.Action != "kill" || command.PID != 8120 {
		t.Fatalf("unexpected process kill command: %+v", command)
	}
	if err := wsjson.Write(ctx, agentConn, model.ProcessKillResult{
		Type: "process", Action: "kill_result", PID: 8120, Success: true,
	}); err != nil {
		t.Fatal(err)
	}
	var killResult model.ProcessKillResult
	if err := wsjson.Read(ctx, frontendConn, &killResult); err != nil {
		t.Fatal(err)
	}
	if !killResult.Success || killResult.PID != 8120 || killResult.AgentID != authTestID {
		t.Fatalf("unexpected process kill result: %+v", killResult)
	}
}

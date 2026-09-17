package wsserver

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"ws-rat/internal/model"
)

const powerAgentA = "11111111-1111-4111-8111-111111111111"
const powerAgentB = "22222222-2222-4222-8222-222222222222"
const powerAgentC = "33333333-3333-4333-8333-333333333333"
const powerRoom = "44444444-4444-4444-8444-444444444444"
const powerRequestID = "55555555-5555-4555-8555-555555555555"
const powerRequestOther = "66666666-6666-4666-8666-666666666666"

type powerTestStore struct {
	agents map[string]model.AgentInfo
	rooms  map[string][]string
}

func (s *powerTestStore) GetByID(_ context.Context, id string) (*model.AgentInfo, error) {
	a, ok := s.agents[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return &a, nil
}
func (*powerTestStore) UpdateStatus(context.Context, string, string) error { return nil }
func (s *powerTestStore) ListAgentIDsByRoom(_ context.Context, id string) ([]string, error) {
	ids, ok := s.rooms[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return ids, nil
}

type powerTestSession struct{ allowed atomic.Bool }

func (*powerTestSession) Authenticate(_ context.Context, hash string) (string, bool, error) {
	if hash != hashToken("test-session") {
		return "", false, fmt.Errorf("invalid session")
	}
	// Power must not depend on the file distribution permission.
	return "user", false, nil
}
func (s *powerTestSession) AuthenticatePermission(ctx context.Context, hash, permission string) (string, bool, error) {
	user, _, err := s.Authenticate(ctx, hash)
	return user, permission == "agents.control" && s.allowed.Load(), err
}

type powerHarness struct {
	s       *Server
	session *powerTestSession
	url     string
	ctx     context.Context
}

func newPowerHarness(t *testing.T, members []string, timeout time.Duration) *powerHarness {
	t.Helper()
	store := &powerTestStore{agents: map[string]model.AgentInfo{}, rooms: map[string][]string{powerRoom: members}}
	for _, id := range members {
		store.agents[id] = model.AgentInfo{ID: id, RoomID: powerRoom}
	}
	s := New(store, log.New(io.Discard, "", 0), nil)
	if timeout != 0 {
		s.power.timeout = timeout
	}
	session := &powerTestSession{}
	session.allowed.Store(true)
	s.sessions = session
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.HandleWebSocket)
	mux.HandleFunc("/ws/frontend", s.HandleFrontendWebSocket)
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return &powerHarness{s: s, session: session, url: "ws" + strings.TrimPrefix(httpServer.URL, "http"), ctx: ctx}
}

func waitPowerCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition timed out")
		}
		time.Sleep(time.Millisecond)
	}
}

func (h *powerHarness) connect(t *testing.T, agent string) *websocket.Conn {
	t.Helper()
	path := "/ws"
	var options *websocket.DialOptions
	if agent == "" {
		path = "/ws/frontend"
		options = &websocket.DialOptions{HTTPHeader: http.Header{"Cookie": {"__Host-session=test-session"}}}
	}
	c, _, err := websocket.Dial(h.ctx, h.url+path, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	if agent != "" {
		powerWrite(t, h.ctx, c, model.AgentInfo{ID: agent})
		waitPowerCondition(t, func() bool { _, ok := h.s.registry.Get(agent); return ok })
	}
	return c
}

func powerWrite(t *testing.T, ctx context.Context, c *websocket.Conn, v any) {
	t.Helper()
	if err := wsjson.Write(ctx, c, v); err != nil {
		t.Fatal(err)
	}
}
func powerRead(t *testing.T, ctx context.Context, c *websocket.Conn) map[string]any {
	t.Helper()
	var v map[string]any
	if err := wsjson.Read(ctx, c, &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func assertPowerCommand(t *testing.T, h *powerHarness, c *websocket.Conn, id string) {
	t.Helper()
	v := powerRead(t, h.ctx, c)
	if len(v) != 3 || v["type"] != "power" || v["action"] != "shutdown" || v["request_id"] != id {
		t.Fatalf("incorrect command: %v", v)
	}
}
func assertNoPowerCommand(t *testing.T, c *websocket.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	var v any
	if err := wsjson.Read(ctx, c, &v); err == nil {
		t.Fatalf("unexpected command: %v", v)
	}
}

func TestPowerSingleWebSocketRoundTrip(t *testing.T) {
	for _, success := range []bool{true, false} {
		t.Run(fmt.Sprint(success), func(t *testing.T) {
			h := newPowerHarness(t, []string{powerAgentA}, 0)
			a, f := h.connect(t, powerAgentA), h.connect(t, "")
			powerWrite(t, h.ctx, f, PowerRequest{Type: "power", Action: "shutdown", AgentID: powerAgentA, RequestID: powerRequestID})
			assertPowerCommand(t, h, a, powerRequestID)
			powerWrite(t, h.ctx, a, PowerResult{Type: "power", Action: "shutdown_result", RequestID: powerRequestOther, Success: true, Mode: "mock"})
			powerWrite(t, h.ctx, a, PowerResult{Type: "power", Action: "shutdown_result", RequestID: powerRequestID, AgentID: powerAgentB, Success: success, Mode: "mock", Message: "shutdown command accepted"})
			r := powerRead(t, h.ctx, f)
			if r["action"] != "shutdown_result" || r["agent_id"] != powerAgentA || r["request_id"] != powerRequestID || r["success"] != success || r["mode"] != "mock" || r["message"] != "shutdown command accepted" {
				t.Fatalf("incorrect result: %v", r)
			}
			if pendingPowerCount(h.s.power) != 0 {
				t.Fatal("request leaked")
			}
		})
	}
}

func TestPowerRejectsInvalidOfflineMissingAndUnauthorized(t *testing.T) {
	for _, tc := range []struct {
		name, action, agent, room, id, code string
		denied                              bool
	}{
		{name: "offline", action: "shutdown", agent: powerAgentB, id: powerRequestID, code: "agent_offline"},
		{name: "missing agent", action: "shutdown", agent: powerAgentC, id: powerRequestID, code: "agent_not_found"},
		{name: "missing room", action: "shutdown_room", room: powerAgentC, id: powerRequestID, code: "room_not_found"},
		{name: "invalid request", action: "shutdown", agent: powerAgentA, id: "bad", code: "invalid_request"},
		{name: "empty request", action: "shutdown", agent: powerAgentA, code: "invalid_request"},
		{name: "invalid agent", action: "shutdown", agent: "bad", id: powerRequestID, code: "invalid_request"},
		{name: "invalid room", action: "shutdown_room", room: "bad", id: powerRequestID, code: "invalid_request"},
		{name: "unknown action", action: "reboot", agent: powerAgentA, id: powerRequestID, code: "invalid_request"},
		{name: "denied single", action: "shutdown", agent: powerAgentA, id: powerRequestID, code: "forbidden", denied: true},
		{name: "denied room", action: "shutdown_room", room: powerRoom, id: powerRequestID, code: "forbidden", denied: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newPowerHarness(t, []string{powerAgentA, powerAgentB}, 0)
			a, f := h.connect(t, powerAgentA), h.connect(t, "")
			h.session.allowed.Store(!tc.denied)
			powerWrite(t, h.ctx, f, PowerRequest{Type: "power", Action: tc.action, AgentID: tc.agent, RoomID: tc.room, RequestID: tc.id})
			r := powerRead(t, h.ctx, f)
			if r["type"] != "error" || r["code"] != tc.code || r["request_id"] != tc.id {
				t.Fatalf("incorrect error: %v", r)
			}
			if pendingPowerCount(h.s.power) != 0 {
				t.Fatal("rejected request registered")
			}
			assertNoPowerCommand(t, a)
		})
	}
}

func TestPowerRoomWebSocketAggregation(t *testing.T) {
	for _, tc := range []struct {
		name                                       string
		members, online, accepted, failed, timeout int
	}{
		{"all online", 3, 3, 3, 0, 0},
		{"mixed offline", 3, 2, 2, 0, 0},
		{"partial failure", 3, 3, 2, 1, 0},
		{"partial timeout", 3, 2, 1, 0, 1},
		{"empty", 0, 0, 0, 0, 0},
		{"all offline", 3, 0, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ids := []string{powerAgentA, powerAgentB, powerAgentC}[:tc.members]
			h := newPowerHarness(t, ids, 200*time.Millisecond)
			var agents []*websocket.Conn
			for _, id := range ids[:tc.online] {
				agents = append(agents, h.connect(t, id))
			}
			f := h.connect(t, "")
			powerWrite(t, h.ctx, f, PowerRequest{Type: "power", Action: "shutdown_room", RoomID: powerRoom, RequestID: powerRequestID})
			for _, a := range agents {
				assertPowerCommand(t, h, a, powerRequestID)
			}
			for i := 0; i < tc.accepted+tc.failed; i++ {
				powerWrite(t, h.ctx, agents[i], PowerResult{Type: "power", Action: "shutdown_result", RequestID: powerRequestID, Success: i < tc.accepted, Mode: "mock"})
			}
			r := powerRead(t, h.ctx, f)
			if r["action"] != "shutdown_room_result" || r["room_id"] != powerRoom || r["request_id"] != powerRequestID {
				t.Fatalf("incorrect room result: %v", r)
			}
			for key, want := range map[string]int{"total": tc.members, "online": tc.online, "offline": tc.members - tc.online, "accepted": tc.accepted, "failed": tc.failed, "timeout": tc.timeout} {
				if r[key] != float64(want) {
					t.Errorf("%s: want %d, got %v", key, want, r)
				}
			}
			if pendingPowerCount(h.s.power) != 0 {
				t.Fatal("room request leaked")
			}
		})
	}
}

func TestPowerSingleTimeoutAfterAgentDisconnect(t *testing.T) {
	h := newPowerHarness(t, []string{powerAgentA}, 100*time.Millisecond)
	a, f := h.connect(t, powerAgentA), h.connect(t, "")
	powerWrite(t, h.ctx, f, PowerRequest{Type: "power", Action: "shutdown", AgentID: powerAgentA, RequestID: powerRequestID})
	assertPowerCommand(t, h, a, powerRequestID)
	a.CloseNow()
	r := powerRead(t, h.ctx, f)
	if r["code"] != "timeout" || r["success"] != false {
		t.Fatalf("disconnect must not imply success: %v", r)
	}
}

func TestPowerFrontendDisconnectCleansPending(t *testing.T) {
	h := newPowerHarness(t, []string{powerAgentA}, 0)
	a, f := h.connect(t, powerAgentA), h.connect(t, "")
	powerWrite(t, h.ctx, f, PowerRequest{Type: "power", Action: "shutdown", AgentID: powerAgentA, RequestID: powerRequestID})
	assertPowerCommand(t, h, a, powerRequestID)
	f.CloseNow()
	waitPowerCondition(t, func() bool { return pendingPowerCount(h.s.power) == 0 })
}

func TestPowerConcurrentFrontendsReceiveOnlyTheirResults(t *testing.T) {
	h := newPowerHarness(t, []string{powerAgentA}, 0)
	a, f, g := h.connect(t, powerAgentA), h.connect(t, ""), h.connect(t, "")
	powerWrite(t, h.ctx, f, PowerRequest{Type: "power", Action: "shutdown", AgentID: powerAgentA, RequestID: powerRequestID})
	assertPowerCommand(t, h, a, powerRequestID)
	powerWrite(t, h.ctx, g, PowerRequest{Type: "power", Action: "shutdown", AgentID: powerAgentA, RequestID: powerRequestID})
	if r := powerRead(t, h.ctx, g); r["code"] != "duplicate_request" {
		t.Fatalf("duplicate accepted: %v", r)
	}
	powerWrite(t, h.ctx, g, PowerRequest{Type: "power", Action: "shutdown", AgentID: powerAgentA, RequestID: powerRequestOther})
	assertPowerCommand(t, h, a, powerRequestOther)
	for _, id := range []string{powerRequestOther, powerRequestID} {
		powerWrite(t, h.ctx, a, PowerResult{Type: "power", Action: "shutdown_result", RequestID: id, Success: true, Mode: "mock"})
	}
	if r := powerRead(t, h.ctx, f); r["request_id"] != powerRequestID {
		t.Fatalf("wrong frontend: %v", r)
	}
	if r := powerRead(t, h.ctx, g); r["request_id"] != powerRequestOther {
		t.Fatalf("wrong frontend: %v", r)
	}
	if pendingPowerCount(h.s.power) != 0 {
		t.Fatal("requests leaked")
	}
}

func TestPowerSendFailure(t *testing.T) {
	for _, action := range []string{"shutdown", "shutdown_room"} {
		t.Run(action, func(t *testing.T) {
			h := newPowerHarness(t, []string{powerAgentA}, 0)
			a, f := h.connect(t, powerAgentA), h.connect(t, "")
			client, _ := h.s.registry.Get(powerAgentA)
			a.CloseNow()
			waitPowerCondition(t, func() bool { _, online := h.s.registry.Get(powerAgentA); return !online })
			// Model the interval where a closed socket is still in Registry.
			h.s.registry.Add(client)
			req := PowerRequest{Type: "power", Action: action, RequestID: powerRequestID}
			if action == "shutdown" {
				req.AgentID = powerAgentA
			} else {
				req.RoomID = powerRoom
			}
			powerWrite(t, h.ctx, f, req)
			r := powerRead(t, h.ctx, f)
			if action == "shutdown" {
				if r["code"] != "send_failed" || r["success"] != false {
					t.Fatalf("incorrect failure: %v", r)
				}
			} else if r["failed"] != float64(1) || r["timeout"] != float64(0) {
				t.Fatalf("incorrect aggregate: %v", r)
			}
			if pendingPowerCount(h.s.power) != 0 {
				t.Fatal("send failure leaked")
			}
		})
	}
}

func TestPowerRejectsMalformedAndNonMockResults(t *testing.T) {
	h := newPowerHarness(t, []string{powerAgentA}, 100*time.Millisecond)
	a, f := h.connect(t, powerAgentA), h.connect(t, "")
	powerWrite(t, h.ctx, f, PowerRequest{Type: "power", Action: "shutdown", AgentID: powerAgentA, RequestID: powerRequestID})
	assertPowerCommand(t, h, a, powerRequestID)
	powerWrite(t, h.ctx, a, map[string]any{"type": "power", "action": "shutdown_result", "request_id": powerRequestID, "mode": "mock"})
	powerWrite(t, h.ctx, a, PowerResult{Type: "power", Action: "shutdown_result", RequestID: powerRequestID, Success: true, Mode: "real"})
	r := powerRead(t, h.ctx, f)
	if r["code"] != "timeout" || r["success"] != false {
		t.Fatalf("malformed result resolved: %v", r)
	}
}

func TestPowerFailsClosedWithoutGenericPermissionStore(t *testing.T) {
	h := newPowerHarness(t, []string{powerAgentA}, 0)
	h.s.sessions = validFrontendSession{}
	a, f := h.connect(t, powerAgentA), h.connect(t, "")
	powerWrite(t, h.ctx, f, PowerRequest{Type: "power", Action: "shutdown", AgentID: powerAgentA, RequestID: powerRequestID})
	if r := powerRead(t, h.ctx, f); r["code"] != "authorization_unavailable" {
		t.Fatalf("incorrect permission result: %v", r)
	}
	assertNoPowerCommand(t, a)
}

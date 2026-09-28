package wsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"ws-rat/internal/model"
)

type appsTestSession struct{ powerTestSession }

func (s *appsTestSession) AuthenticatePermission(ctx context.Context, hash, permission string) (string, bool, error) {
	u, _, err := s.Authenticate(ctx, hash)
	return u, permission == "monitor.read" && s.allowed.Load(), err
}
func newAppsHarness(t *testing.T, timeout time.Duration) (*powerHarness, *appsTestSession) {
	h := newPowerHarness(t, []string{powerAgentA, powerAgentB}, 0)
	session := &appsTestSession{}
	session.allowed.Store(true)
	h.s.sessions = session
	if timeout != 0 {
		h.s.installedApps.timeout = timeout
	}
	return h, session
}
func appsRequest(agent, id string) InstalledAppsRequest {
	return InstalledAppsRequest{Type: "installed_apps", Action: "get", AgentID: agent, RequestID: id}
}
func appsResult(id string) map[string]any {
	return map[string]any{"type": "installed_apps", "action": "result", "request_id": id, "success": true, "apps": []any{map[string]any{"name": "Secret Product", "version": "1", "estimated_size_kb": 2048}}}
}
func appsRead(t *testing.T, h *powerHarness, f *websocket.Conn) map[string]any {
	t.Helper()
	for {
		r := powerRead(t, h.ctx, f)
		if r["type"] == "installed_apps" {
			return r
		}
	}
}
func appsCommand(t *testing.T, h *powerHarness, a *websocket.Conn, id string) {
	t.Helper()
	r := powerRead(t, h.ctx, a)
	if len(r) != 3 || r["type"] != "installed_apps" || r["action"] != "get" || r["request_id"] != id {
		t.Fatal(r)
	}
}
func appsPending(t *InstalledAppsTracker) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.pending)
}

func TestInstalledAppsRoundTripAndIsolation(t *testing.T) {
	h, _ := newAppsHarness(t, 0)
	a, b := h.connect(t, powerAgentA), h.connect(t, powerAgentB)
	f, g := h.connect(t, ""), h.connect(t, "")
	powerWrite(t, h.ctx, f, appsRequest(powerAgentA, powerRequestID))
	powerWrite(t, h.ctx, g, appsRequest(powerAgentB, powerRequestOther))
	appsCommand(t, h, a, powerRequestID)
	appsCommand(t, h, b, powerRequestOther)
	// Wrong connection, unknown ID and a duplicate must never resolve another request.
	powerWrite(t, h.ctx, b, appsResult(powerRequestID))
	powerWrite(t, h.ctx, a, appsResult(powerAgentC))
	r := appsResult(powerRequestID)
	r["agent_id"] = powerAgentB
	r["code"] = "forged"
	powerWrite(t, h.ctx, a, r)
	got := appsRead(t, h, f)
	if got["agent_id"] != powerAgentA || got["request_id"] != powerRequestID || got["success"] != true || got["code"] != nil {
		t.Fatal(got)
	}
	powerWrite(t, h.ctx, a, r)
	powerWrite(t, h.ctx, b, appsResult(powerRequestOther))
	got = appsRead(t, h, g)
	if got["agent_id"] != powerAgentB || got["request_id"] != powerRequestOther {
		t.Fatal("response crossed frontends", got)
	}
	waitPowerCondition(t, func() bool { return appsPending(h.s.installedApps) == 0 })
}
func TestInstalledAppsRejectRequests(t *testing.T) {
	for _, tc := range []struct {
		name, agent, id, action, code string
		deny                          bool
	}{
		{"offline", powerAgentA, powerRequestID, "get", "agent_offline", false},
		{"missing", powerAgentC, powerRequestID, "get", "agent_not_found", false},
		{"bad agent", "bad", powerRequestID, "get", "invalid_request", false},
		{"bad request", powerAgentA, "bad", "get", "invalid_request", false},
		{"missing request", powerAgentA, "", "get", "invalid_request", false},
		{"wrong action", powerAgentA, powerRequestID, "start", "invalid_request", false},
		{"denied", powerAgentA, powerRequestID, "get", "forbidden", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, session := newAppsHarness(t, 0)
			session.allowed.Store(!tc.deny)
			f := h.connect(t, "")
			req := appsRequest(tc.agent, tc.id)
			req.Action = tc.action
			powerWrite(t, h.ctx, f, req)
			if r := appsRead(t, h, f); r["code"] != tc.code || r["success"] != false {
				t.Fatal(r)
			}
			if appsPending(h.s.installedApps) != 0 {
				t.Fatal("invalid request registered")
			}
		})
	}
}
func TestInstalledAppsDuplicateTimeoutAndFrontendCleanup(t *testing.T) {
	h, _ := newAppsHarness(t, 80*time.Millisecond)
	a, f := h.connect(t, powerAgentA), h.connect(t, "")
	powerWrite(t, h.ctx, f, appsRequest(powerAgentA, powerRequestID))
	appsCommand(t, h, a, powerRequestID)
	powerWrite(t, h.ctx, f, appsRequest(powerAgentA, powerRequestID))
	if r := appsRead(t, h, f); r["code"] != "duplicate_request" {
		t.Fatal(r)
	}
	if r := appsRead(t, h, f); r["code"] != "timeout" {
		t.Fatal(r)
	}
	powerWrite(t, h.ctx, a, appsResult(powerRequestID))
	powerWrite(t, h.ctx, f, appsRequest(powerAgentA, powerRequestOther))
	appsCommand(t, h, a, powerRequestOther)
	f.CloseNow()
	waitPowerCondition(t, func() bool { return appsPending(h.s.installedApps) == 0 })
}
func TestInstalledAppsDisconnectAndReplacement(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(fmt.Sprint(replace), func(t *testing.T) {
			h, _ := newAppsHarness(t, 0)
			a, f := h.connect(t, powerAgentA), h.connect(t, "")
			old, _ := h.s.registry.Get(powerAgentA)
			powerWrite(t, h.ctx, f, appsRequest(powerAgentA, powerRequestID))
			appsCommand(t, h, a, powerRequestID)
			if replace {
				replacement := h.connect(t, powerAgentA)
				waitPowerCondition(t, func() bool { current, _ := h.s.registry.Get(powerAgentA); return current != old })
				// Same authenticated ID still cannot finish a request sent to old socket.
				powerWrite(t, h.ctx, replacement, appsResult(powerRequestID))
			} else {
				a.CloseNow()
			}
			if r := appsRead(t, h, f); r["code"] != "agent_offline" {
				t.Fatal(r)
			}
			waitPowerCondition(t, func() bool { return appsPending(h.s.installedApps) == 0 })
		})
	}
}
func TestInstalledAppsInvalidAndFailedResults(t *testing.T) {
	for _, tc := range []struct {
		name  string
		patch func(map[string]any)
		code  string
	}{
		{"failure", func(r map[string]any) { r["success"] = false; r["error"] = "private registry path"; delete(r, "apps") }, "collection_failed"},
		{"missing success", func(r map[string]any) { delete(r, "success") }, "invalid_result"},
		{"wrong success", func(r map[string]any) { r["success"] = "true" }, "invalid_result"},
		{"missing apps", func(r map[string]any) { delete(r, "apps") }, "invalid_result"},
		{"null apps", func(r map[string]any) { r["apps"] = nil }, "invalid_result"},
		{"empty name", func(r map[string]any) { r["apps"] = []any{map[string]any{"name": " "}} }, "invalid_result"},
		{"long name", func(r map[string]any) { r["apps"] = []any{map[string]any{"name": strings.Repeat("ก", 513)}} }, "invalid_result"},
		{"negative size", func(r map[string]any) { r["apps"] = []any{map[string]any{"name": "App", "estimated_size_kb": -1}} }, "invalid_result"},
		{"huge size", func(r map[string]any) { r["apps"] = []any{map[string]any{"name": "App", "estimated_size_kb": 1 << 40}} }, "invalid_result"},
		{"too many apps", func(r map[string]any) { r["apps"] = make([]InstalledApp, maxInstalledApps+1) }, "invalid_result"},
		{"oversize raw", func(r map[string]any) { r["padding"] = strings.Repeat("x", maxInstalledAppsPayload) }, "invalid_result"},
		{"bad action", func(r map[string]any) { r["action"] = "get" }, "invalid_result"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := newAppsHarness(t, 0)
			a, f := h.connect(t, powerAgentA), h.connect(t, "")
			powerWrite(t, h.ctx, f, appsRequest(powerAgentA, powerRequestID))
			appsCommand(t, h, a, powerRequestID)
			r := appsResult(powerRequestID)
			tc.patch(r)
			powerWrite(t, h.ctx, a, r)
			got := appsRead(t, h, f)
			if got["code"] != tc.code || got["apps"] != nil || strings.Contains(fmt.Sprint(got), "private registry") {
				t.Fatal(got)
			}
		})
	}
}
func TestInstalledAppsRevalidatesPermissionAtResult(t *testing.T) {
	h, session := newAppsHarness(t, 0)
	a, f := h.connect(t, powerAgentA), h.connect(t, "")
	powerWrite(t, h.ctx, f, appsRequest(powerAgentA, powerRequestID))
	appsCommand(t, h, a, powerRequestID)
	session.allowed.Store(false)
	powerWrite(t, h.ctx, a, appsResult(powerRequestID))
	if r := appsRead(t, h, f); r["code"] != "forbidden" || r["apps"] != nil {
		t.Fatal(r)
	}
}

func TestInstalledAppsTrackerBoundsIdentityAndRaces(t *testing.T) {
	tracker := NewInstalledAppsTracker()
	f, g := &FrontendClient{}, &FrontendClient{}
	a := &Client{Info: model.AgentInfo{ID: powerAgentA}}
	replacement := &Client{Info: a.Info}
	var resolved atomic.Int32
	complete := func(InstalledAppsResult) { resolved.Add(1) }
	p, code := tracker.Register(appsRequest(powerAgentA, powerRequestID), f, a, complete)
	if code != "" {
		t.Fatal(code)
	}
	if tracker.Resolve(replacement, InstalledAppsResult{RequestID: powerRequestID}) {
		t.Fatal("replacement accepted")
	}
	if _, code = tracker.Register(p.req, g, a, complete); code != "duplicate_request" {
		t.Fatal(code)
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); tracker.Resolve(a, InstalledAppsResult{RequestID: powerRequestID}) }()
	}
	wg.Wait()
	if resolved.Load() != 1 || appsPending(tracker) != 0 {
		t.Fatal("duplicate completion")
	}
	for i := 0; i < maxInstalledAppsPerAgent; i++ {
		req := appsRequest(powerAgentA, fmt.Sprint(i))
		if _, code = tracker.Register(req, f, a, complete); code != "" {
			t.Fatal(code)
		}
	}
	if _, code = tracker.Register(appsRequest(powerAgentA, "overflow"), g, a, complete); code != "busy" {
		t.Fatal(code)
	}
	tracker.RemoveFrontend(f)
	if appsPending(tracker) != 0 {
		t.Fatal("frontend leaked")
	}
	// Fresh request may reuse an ID, but an old timer/send failure carries its pointer.
	q, _ := tracker.Register(p.req, f, a, complete)
	if tracker.resolve(a, appsError(p.req, "timeout"), p) {
		t.Fatal("stale timer resolved new request")
	}
	tracker.resolve(a, appsError(q.req, "timeout"), q)
}

type appsAudit struct {
	mu      sync.Mutex
	entries [][]byte
}

func (a *appsAudit) WriteAudit(_ context.Context, _ string, _ string, _ string, _ string, raw []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, append([]byte(nil), raw...))
	return nil
}
func TestInstalledAppsAuditExcludesInventory(t *testing.T) {
	h, _ := newAppsHarness(t, 0)
	audit := &appsAudit{}
	h.s.ConfigureAudit(audit)
	a, f := h.connect(t, powerAgentA), h.connect(t, "")
	powerWrite(t, h.ctx, f, appsRequest(powerAgentA, powerRequestID))
	appsCommand(t, h, a, powerRequestID)
	powerWrite(t, h.ctx, a, appsResult(powerRequestID))
	appsRead(t, h, f)
	waitPowerCondition(t, func() bool { return appsPending(h.s.installedApps) == 0 })
	audit.mu.Lock()
	defer audit.mu.Unlock()
	if len(audit.entries) != 2 {
		t.Fatal(len(audit.entries))
	}
	for _, raw := range audit.entries {
		if strings.Contains(string(raw), "Secret Product") {
			t.Fatal("inventory logged")
		}
	}
	var detail map[string]any
	json.Unmarshal(audit.entries[1], &detail)
	if detail["count"] != float64(1) || detail["success"] != true {
		t.Fatal(detail)
	}
}

func TestInstalledAppsFrontendAndGlobalCapacity(t *testing.T) {
	tracker := NewInstalledAppsTracker()
	f := &FrontendClient{}
	defer tracker.RemoveFrontend(f)
	noop := func(InstalledAppsResult) {}
	for i := 0; i < maxInstalledAppsPerFrontend; i++ {
		a := &Client{Info: model.AgentInfo{ID: fmt.Sprint(i)}}
		if _, code := tracker.Register(appsRequest(a.Info.ID, fmt.Sprint(i)), f, a, noop); code != "" {
			t.Fatal(code)
		}
	}
	if _, code := tracker.Register(appsRequest("new", "overflow"), f, &Client{Info: model.AgentInfo{ID: "new"}}, noop); code != "busy" {
		t.Fatal(code)
	}
	tracker.RemoveFrontend(f)
	for i := 0; i < maxInstalledAppsPending; i++ {
		front := &FrontendClient{}
		defer tracker.RemoveFrontend(front)
		a := &Client{Info: model.AgentInfo{ID: fmt.Sprint(i)}}
		if _, code := tracker.Register(appsRequest(a.Info.ID, fmt.Sprint(i)), front, a, noop); code != "" {
			t.Fatal(code)
		}
	}
	if _, code := tracker.Register(appsRequest("new", "overflow"), f, &Client{Info: model.AgentInfo{ID: "new"}}, noop); code != "busy" {
		t.Fatal(code)
	}
}

func TestInstalledAppsMalformedJSONTimesOutAndEmptyInventorySucceeds(t *testing.T) {
	h, _ := newAppsHarness(t, 100*time.Millisecond)
	a, f := h.connect(t, powerAgentA), h.connect(t, "")
	powerWrite(t, h.ctx, f, appsRequest(powerAgentA, powerRequestID))
	appsCommand(t, h, a, powerRequestID)
	if err := a.Write(h.ctx, websocket.MessageText, []byte(`{"type":"installed_apps","request_id":`)); err != nil {
		t.Fatal(err)
	}
	if r := appsRead(t, h, f); r["code"] != "timeout" {
		t.Fatal(r)
	}
	powerWrite(t, h.ctx, f, appsRequest(powerAgentA, powerRequestOther))
	appsCommand(t, h, a, powerRequestOther)
	r := appsResult(powerRequestOther)
	r["apps"] = []any{}
	powerWrite(t, h.ctx, a, r)
	got := appsRead(t, h, f)
	apps, ok := got["apps"].([]any)
	if got["success"] != true || !ok || len(apps) != 0 {
		t.Fatal(got)
	}
}

func TestInstalledAppOptionalFieldBounds(t *testing.T) {
	for _, raw := range []string{
		`[{"name":"A","version":"` + strings.Repeat("v", 257) + `"}]`,
		`[{"name":"A","publisher":"` + strings.Repeat("p", 513) + `"}]`,
		`[{"name":"A","install_date":"` + strings.Repeat("d", 33) + `"}]`,
		`[{"name":"A","estimated_size_kb":1.5}]`,
		`[{"name":"A","estimated_size_kb":"2048"}]`,
		`[null]`, `null`, `{}`,
	} {
		if _, err := decodeInstalledApps([]byte(raw)); err == nil {
			t.Fatal("accepted malformed inventory")
		}
	}
	if apps, err := decodeInstalledApps([]byte(`[{"name":"A","estimated_size_kb":0},{"name":"B"}]`)); err != nil || len(apps) != 2 || *apps[0].EstimatedSizeKB != 0 {
		t.Fatal(apps, err)
	}
}

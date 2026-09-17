package wsserver

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const powerTimeout = 10 * time.Second

type PowerRequest struct {
	Type      string `json:"type"`
	Action    string `json:"action"`
	RequestID string `json:"request_id"`
	AgentID   string `json:"agent_id,omitempty"`
	RoomID    string `json:"room_id,omitempty"`
}

type PowerResult struct {
	Type      string `json:"type"`
	Action    string `json:"action"`
	RequestID string `json:"request_id"`
	AgentID   string `json:"agent_id"`
	Success   bool   `json:"success"`
	Mode      string `json:"mode"`
	Message   string `json:"message"`
	Code      string `json:"code,omitempty"`
}

type PowerRoomResult struct {
	Type      string `json:"type"`
	Action    string `json:"action"`
	RequestID string `json:"request_id"`
	RoomID    string `json:"room_id"`
	Total     int    `json:"total"`
	Online    int    `json:"online"`
	Offline   int    `json:"offline"`
	Accepted  int    `json:"accepted"`
	Failed    int    `json:"failed"`
	Timeout   int    `json:"timeout"`
}

type powerTarget struct {
	client *Client
	done   bool
}

type pendingPower struct {
	request   PowerRequest
	frontend  *FrontendClient
	targets   map[string]*powerTarget
	room      PowerRoomResult
	remaining int
	ctx       context.Context
	cancel    context.CancelFunc
	timer     *time.Timer
	complete  func(any)
}

// PowerTracker owns only in-flight operations. All transitions are serialized;
// socket writes and audit callbacks always happen outside the lock.
type PowerTracker struct {
	mu      sync.Mutex
	pending map[string]*pendingPower
	timeout time.Duration
}

func NewPowerTracker() *PowerTracker {
	return &PowerTracker{pending: make(map[string]*pendingPower), timeout: powerTimeout}
}

func (t *PowerTracker) Register(req PowerRequest, f *FrontendClient, clients []*Client, total int, complete func(any)) (*pendingPower, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.pending[req.RequestID]; exists {
		return nil, fmt.Errorf("request_id is already pending")
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &pendingPower{request: req, frontend: f, targets: make(map[string]*powerTarget), ctx: ctx, cancel: cancel, complete: complete}
	for _, c := range clients {
		p.targets[c.Info.ID] = &powerTarget{client: c}
	}
	p.remaining = len(p.targets)
	p.room = PowerRoomResult{Type: "power", Action: "shutdown_room_result", RequestID: req.RequestID, RoomID: req.RoomID, Total: total, Online: p.remaining, Offline: total - p.remaining}
	t.pending[req.RequestID] = p
	p.timer = time.AfterFunc(t.timeout, func() { t.expire(p) })
	return p, nil
}

func (t *PowerTracker) finishLocked(p *pendingPower) {
	delete(t.pending, p.request.RequestID)
	p.timer.Stop()
	p.cancel()
}

// Resolve binds results to the actual connection that received the command.
func (t *PowerTracker) Resolve(client *Client, result PowerResult) bool {
	return t.resolve(client, result, nil)
}

// expected protects a delayed dispatch failure from affecting a newer request.
func (t *PowerTracker) resolve(client *Client, result PowerResult, expected *pendingPower) bool {
	t.mu.Lock()
	p := t.pending[result.RequestID]
	if p == nil || (expected != nil && p != expected) {
		t.mu.Unlock()
		return false
	}
	target := p.targets[client.Info.ID]
	if target == nil || target.client != client || target.done {
		t.mu.Unlock()
		return false
	}
	target.done = true
	p.remaining--
	var response any
	if p.request.Action == "shutdown" {
		result.Type, result.Action, result.AgentID = "power", "shutdown_result", client.Info.ID
		response = result
	} else {
		if result.Success {
			p.room.Accepted++
		} else {
			p.room.Failed++
		}
		if p.remaining == 0 {
			response = p.room
		}
	}
	if response != nil {
		t.finishLocked(p)
	}
	t.mu.Unlock()
	if response != nil {
		p.complete(response)
	}
	return true
}

// CompleteEmpty also covers rooms whose members are all offline.
func (t *PowerTracker) CompleteEmpty(p *pendingPower) {
	t.mu.Lock()
	if t.pending[p.request.RequestID] != p || p.remaining != 0 {
		t.mu.Unlock()
		return
	}
	t.finishLocked(p)
	result := p.room
	t.mu.Unlock()
	p.complete(result)
}

func (t *PowerTracker) expire(p *pendingPower) {
	t.mu.Lock()
	if t.pending[p.request.RequestID] != p {
		t.mu.Unlock()
		return
	}
	var result any
	if p.request.Action == "shutdown" {
		result = PowerResult{Type: "power", Action: "shutdown_result", RequestID: p.request.RequestID, AgentID: p.request.AgentID, Mode: "mock", Code: "timeout", Message: "agent response timed out"}
	} else {
		p.room.Timeout = p.remaining
		result = p.room
	}
	t.finishLocked(p)
	t.mu.Unlock()
	p.complete(result)
}

func (t *PowerTracker) RemoveFrontend(f *FrontendClient) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, p := range t.pending {
		if p.frontend == f {
			t.finishLocked(p)
		}
	}
}

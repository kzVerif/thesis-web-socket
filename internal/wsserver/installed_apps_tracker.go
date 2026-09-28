package wsserver

import (
	"context"
	"sync"
	"time"
)

const (
	installedAppsTimeout        = 15 * time.Second
	maxInstalledAppsPending     = 256
	maxInstalledAppsPerFrontend = 16
	maxInstalledAppsPerAgent    = 8
)

type installedAppsPending struct {
	req      InstalledAppsRequest
	frontend *FrontendClient
	agent    *Client
	ctx      context.Context
	cancel   context.CancelFunc
	timer    *time.Timer
	complete func(InstalledAppsResult)
	done     bool
}

// Separate from stream subscriptions and PowerTracker. Pointer identity binds
// both endpoints, including when an authenticated Agent reconnects with the same ID.
type InstalledAppsTracker struct {
	mu       sync.Mutex
	pending  map[string]*installedAppsPending
	timeout  time.Duration
	dispatch chan struct{}
}

func NewInstalledAppsTracker() *InstalledAppsTracker {
	return &InstalledAppsTracker{pending: make(map[string]*installedAppsPending), timeout: installedAppsTimeout, dispatch: make(chan struct{}, maxInstalledAppsPending)}
}
func (t *InstalledAppsTracker) Register(req InstalledAppsRequest, f *FrontendClient, a *Client, complete func(InstalledAppsResult)) (*installedAppsPending, string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.pending[req.RequestID]; ok {
		return nil, "duplicate_request"
	}
	if len(t.pending) >= maxInstalledAppsPending {
		return nil, "busy"
	}
	front, agent := 0, 0
	for _, p := range t.pending {
		if p.frontend == f {
			front++
		}
		if p.agent.Info.ID == a.Info.ID {
			agent++
		}
	}
	if front >= maxInstalledAppsPerFrontend || agent >= maxInstalledAppsPerAgent {
		return nil, "busy"
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &installedAppsPending{req: req, frontend: f, agent: a, ctx: ctx, cancel: cancel, complete: complete}
	t.pending[req.RequestID] = p
	p.timer = time.AfterFunc(t.timeout, func() { t.resolve(a, appsError(req, "timeout"), p) })
	return p, ""
}

func (t *InstalledAppsTracker) Resolve(a *Client, r InstalledAppsResult) bool {
	return t.resolve(a, r, nil)
}
func (t *InstalledAppsTracker) resolve(a *Client, r InstalledAppsResult, expected *installedAppsPending) bool {
	p := t.take(a, r.RequestID, expected)
	if p == nil {
		return false
	}
	t.finish(p, r)
	return true
}

func (t *InstalledAppsTracker) take(a *Client, id string, expected *installedAppsPending) *installedAppsPending {
	t.mu.Lock()
	p := t.pending[id]
	if p == nil || p.done || p.agent != a || (expected != nil && expected != p) {
		t.mu.Unlock()
		return nil
	}
	p.done = true
	p.timer.Stop()
	p.cancel()
	// Keep the slot reserved during response I/O/audit to bound completion work.
	t.mu.Unlock()
	return p
}

func (t *InstalledAppsTracker) finish(p *installedAppsPending, r InstalledAppsResult) {
	r.Type, r.Action, r.AgentID, r.RequestID = "installed_apps", "result", p.req.AgentID, p.req.RequestID
	p.complete(r)
	t.mu.Lock()
	if t.pending[r.RequestID] == p {
		delete(t.pending, r.RequestID)
	}
	t.mu.Unlock()
}
func (t *InstalledAppsTracker) RemoveFrontend(f *FrontendClient) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, p := range t.pending {
		if p.frontend == f {
			p.cancel()
			p.timer.Stop()
			if !p.done {
				delete(t.pending, id)
			}
		}
	}
}
func (t *InstalledAppsTracker) FailAgent(a *Client) {
	t.mu.Lock()
	var list []*installedAppsPending
	for _, p := range t.pending {
		if p.agent == a && !p.done {
			list = append(list, p)
		}
	}
	t.mu.Unlock()
	for _, p := range list {
		if claimed := t.take(a, p.req.RequestID, p); claimed != nil {
			go t.finish(claimed, appsError(p.req, "agent_offline"))
		}
	}
}

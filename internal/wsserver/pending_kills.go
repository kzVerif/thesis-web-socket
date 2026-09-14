package wsserver

import (
	"fmt"
	"sync"
	"time"

	"ws-rat/internal/model"
)

const processKillTimeout = 15 * time.Second

type processKillKey struct {
	agentID string
	pid     int64
}

type pendingProcessKill struct {
	frontend *FrontendClient
	timer    *time.Timer
}

type ProcessKillTracker struct {
	mu      sync.Mutex
	pending map[processKillKey]*pendingProcessKill
}

func NewProcessKillTracker() *ProcessKillTracker {
	return &ProcessKillTracker{pending: make(map[processKillKey]*pendingProcessKill)}
}

func (tracker *ProcessKillTracker) Register(agentID string, pid int64, frontend *FrontendClient) error {
	key := processKillKey{agentID: agentID, pid: pid}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if _, exists := tracker.pending[key]; exists {
		return fmt.Errorf("a kill command for pid %d is already pending", pid)
	}
	pending := &pendingProcessKill{frontend: frontend}
	pending.timer = time.AfterFunc(processKillTimeout, func() {
		tracker.Resolve(agentID, model.ProcessKillResult{
			Type: "process", Action: "kill_result", PID: pid,
			Success: false, Error: "agent response timed out",
		})
	})
	tracker.pending[key] = pending
	return nil
}

func (tracker *ProcessKillTracker) Resolve(agentID string, result model.ProcessKillResult) bool {
	key := processKillKey{agentID: agentID, pid: result.PID}
	tracker.mu.Lock()
	pending, exists := tracker.pending[key]
	if exists {
		delete(tracker.pending, key)
		pending.timer.Stop()
	}
	tracker.mu.Unlock()
	if !exists {
		return false
	}
	result.AgentID = agentID
	_ = pending.frontend.WriteJSON(result)
	return true
}

func (tracker *ProcessKillTracker) Cancel(agentID string, pid int64, frontend *FrontendClient) {
	key := processKillKey{agentID: agentID, pid: pid}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if pending, exists := tracker.pending[key]; exists && pending.frontend == frontend {
		pending.timer.Stop()
		delete(tracker.pending, key)
	}
}

func (tracker *ProcessKillTracker) RemoveFrontend(frontend *FrontendClient) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	for key, pending := range tracker.pending {
		if pending.frontend == frontend {
			pending.timer.Stop()
			delete(tracker.pending, key)
		}
	}
}

func (tracker *ProcessKillTracker) FailAgent(agentID, message string) {
	tracker.mu.Lock()
	results := make([]struct {
		frontend *FrontendClient
		result   model.ProcessKillResult
	}, 0)
	for key, pending := range tracker.pending {
		if key.agentID != agentID {
			continue
		}
		pending.timer.Stop()
		delete(tracker.pending, key)
		results = append(results, struct {
			frontend *FrontendClient
			result   model.ProcessKillResult
		}{pending.frontend, model.ProcessKillResult{
			Type: "process", Action: "kill_result", AgentID: agentID,
			PID: key.pid, Success: false, Error: message,
		}})
	}
	tracker.mu.Unlock()
	for _, item := range results {
		_ = item.frontend.WriteJSON(item.result)
	}
}

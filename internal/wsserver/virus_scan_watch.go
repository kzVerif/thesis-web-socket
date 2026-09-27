package wsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/coder/websocket"
	"ws-rat/internal/virusscan"
)

type scanSnapshotStore interface {
	Snapshot(context.Context, string, virusscan.Request) (virusscan.Dashboard, error)
}

// Buffered signals coalesce bursts; each read reloads committed data for its owner.
func (s *Server) notifyScanWatchers() {
	s.scanWatchMu.Lock()
	defer s.scanWatchMu.Unlock()
	for ch := range s.scanWatchers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (s *Server) configureScanWatch(parent context.Context, f *FrontendClient, user, token string, raw json.RawMessage, stop *func()) error {
	var req virusscan.Request
	if json.Unmarshal(raw, &req) != nil {
		return fmt.Errorf("invalid request")
	}
	if req.Type == "virus_scan_unsubscribe" {
		if *stop != nil {
			(*stop)()
			*stop = nil
		}
		return f.WriteJSON(map[string]any{"type": "unsubscribed", "stream": "virus_scan"})
	}
	store, ok := s.virusScans.(scanSnapshotStore)
	if !ok {
		return fmt.Errorf("virus scan unavailable")
	}
	if req.JobID != "" && !virusscan.ValidID(req.JobID) {
		return fmt.Errorf("invalid job_id")
	}
	if req.AgentID != "" || len(req.AgentIDs) > 0 || req.RequestID != "" {
		return fmt.Errorf("subscription supports job_id and limit only")
	}
	if req.Limit < 0 || req.Limit > 100 {
		return fmt.Errorf("limit must be between 1 and 100")
	}
	if *stop != nil {
		(*stop)()
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	wake := make(chan struct{}, 1)
	s.scanWatchMu.Lock()
	if s.scanWatchers == nil {
		s.scanWatchers = make(map[chan struct{}]struct{})
	}
	s.scanWatchers[wake] = struct{}{}
	s.scanWatchMu.Unlock()
	*stop = func() { cancel(); <-done }
	go func() {
		defer close(done)
		defer func() {
			s.scanWatchMu.Lock()
			delete(s.scanWatchers, wake)
			s.scanWatchMu.Unlock()
		}()
		// Reconcile external DB updates and revalidate idle sessions too.
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		var previous string
		for {
			if ctx.Err() != nil {
				return
			}
			readCtx, readCancel := context.WithTimeout(ctx, databaseTimeout)
			currentUser, _, authErr := s.authenticateFrontend(readCtx, token)
			if authErr != nil || currentUser != user || currentUser == "" {
				readCancel()
				if ctx.Err() == nil {
					_ = f.WriteJSON(map[string]any{"type": "error", "stream": "virus_scan", "error": "ขาดการ login"})
					_ = f.Conn.Close(websocket.StatusPolicyViolation, "session expired")
				}
				return
			}
			allowed, err := s.virusScans.Allowed(readCtx, user)
			if err != nil || !allowed {
				readCancel()
				if ctx.Err() == nil {
					_ = f.WriteJSON(map[string]any{"type": "error", "stream": "virus_scan", "error": "ไม่มี permission"})
				}
				return
			}
			snapshot, err := store.Snapshot(readCtx, user, req)
			readCancel()
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				s.logger.Printf("scan snapshot: %v", err)
				_ = f.WriteJSON(map[string]any{"type": "error", "stream": "virus_scan", "error": "cannot load scans"})
				return
			}
			encoded, err := json.Marshal(snapshot)
			if err != nil {
				return
			}
			if string(encoded) != previous {
				if err := f.WriteJSON(snapshot); err != nil {
					return
				}
				previous = string(encoded)
			}
			select {
			case <-ctx.Done():
				return
			case <-wake:
			case <-ticker.C:
			}
		}
	}()
	return nil
}

package wsserver

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"ws-rat/internal/virusscan"
)

const maxInstalledApps = 2000
const maxInstalledAppsPayload = 4 << 20
const maxInstalledAppSizeKB = uint64(1<<32 - 1)
const (
	maxInstalledAppName      = 512
	maxInstalledAppVersion   = 256
	maxInstalledAppPublisher = 512
	maxInstalledAppDate      = 32
	maxInstalledAppsCommand  = 1024
	maxInstalledAppsError    = 1024
)

type InstalledApp struct {
	Name            string  `json:"name"`
	Version         string  `json:"version,omitempty"`
	Publisher       string  `json:"publisher,omitempty"`
	InstallDate     string  `json:"install_date,omitempty"`
	EstimatedSizeKB *uint64 `json:"estimated_size_kb,omitempty"`
}
type InstalledAppsRequest struct {
	Type      string `json:"type"`
	Action    string `json:"action"`
	AgentID   string `json:"agent_id"`
	RequestID string `json:"request_id"`
}
type InstalledAppsResult struct {
	Type      string          `json:"type"`
	Action    string          `json:"action"`
	AgentID   string          `json:"agent_id"`
	RequestID string          `json:"request_id"`
	Success   bool            `json:"success"`
	Apps      *[]InstalledApp `json:"apps,omitempty"`
	Code      string          `json:"code,omitempty"`
	Error     string          `json:"error,omitempty"`
}

func appsError(req InstalledAppsRequest, code string) InstalledAppsResult {
	messages := map[string]string{
		"invalid_request":           "valid agent_id, request_id UUIDs and get action are required",
		"forbidden":                 "monitor.read permission required",
		"authorization_unavailable": "cannot authorize installed applications request",
		"agent_not_found":           "agent does not exist", "lookup_failed": "cannot load agent",
		"agent_offline": "agent is offline", "duplicate_request": "request_id already pending",
		"busy": "too many installed applications requests", "timeout": "installed applications request timed out",
		"send_failed":       "could not send installed applications request",
		"collection_failed": "agent could not collect installed applications",
		"invalid_result":    "agent returned an invalid installed applications result",
	}
	return InstalledAppsResult{Type: "installed_apps", Action: "result", AgentID: req.AgentID, RequestID: req.RequestID, Code: code, Error: messages[code]}
}

func (s *Server) handleInstalledAppsRequest(f *FrontendClient, user, tokenHash string, raw json.RawMessage) {
	var req InstalledAppsRequest
	err := json.Unmarshal(raw, &req)
	fail := func(code string) {
		// Invalid caller strings are never reflected unboundedly.
		if len(req.AgentID) > 36 {
			req.AgentID = ""
		}
		if len(req.RequestID) > 36 {
			req.RequestID = ""
		}
		_ = f.WriteJSON(appsError(req, code))
	}
	if err != nil || len(raw) > maxInstalledAppsCommand || req.Type != "installed_apps" || req.Action != "get" || !virusscan.ValidID(req.AgentID) || !virusscan.ValidID(req.RequestID) {
		fail("invalid_request")
		return
	}
	req.AgentID = strings.ToLower(req.AgentID)
	permissions, ok := s.sessions.(FrontendPermissionStore)
	if !ok {
		fail("authorization_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	authUser, allowed, err := permissions.AuthenticatePermission(ctx, tokenHash, "monitor.read")
	if err != nil || authUser == "" || authUser != user {
		fail("authorization_unavailable")
		return
	}
	if !allowed {
		fail("forbidden")
		return
	}
	agent, err := s.agents.GetByID(ctx, req.AgentID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && agent == nil) {
		fail("agent_not_found")
		return
	}
	if err != nil || agent.ID != req.AgentID {
		fail("lookup_failed")
		return
	}
	lock := s.lifecycleLock(req.AgentID)
	lock.Lock()
	client, online := s.registry.Get(req.AgentID)
	if !online {
		lock.Unlock()
		fail("agent_offline")
		return
	}
	p, code := s.installedApps.Register(req, f, client, func(r InstalledAppsResult) {
		// A permission/session revoked during collection must not expose inventory.
		checkCtx, checkCancel := context.WithTimeout(context.Background(), databaseTimeout)
		current, permitted, e := permissions.AuthenticatePermission(checkCtx, tokenHash, "monitor.read")
		checkCancel()
		if e != nil || current != user || current == "" {
			r = appsError(req, "authorization_unavailable")
		} else if !permitted {
			r = appsError(req, "forbidden")
		}
		_ = f.WriteJSON(r)
		count := 0
		if r.Apps != nil {
			count = len(*r.Apps)
		}
		s.recordAudit(user, "installed_apps.result", req.AgentID, "", map[string]any{"request_id": req.RequestID, "success": r.Success, "code": r.Code, "count": count})
	})
	lock.Unlock()
	if code != "" {
		fail(code)
		return
	}
	// A disconnected frontend frees its pending entry before a blocked socket
	// writer necessarily returns. Bound dispatchers independently as well.
	select {
	case s.installedApps.dispatch <- struct{}{}:
	default:
		s.installedApps.resolve(client, appsError(req, "busy"), p)
		return
	}
	go func() {
		defer func() { <-s.installedApps.dispatch }()
		sendCtx, sendCancel := context.WithTimeout(p.ctx, 5*time.Second)
		defer sendCancel()
		if sendCtx.Err() != nil {
			return
		}
		command := map[string]string{"type": "installed_apps", "action": "get", "request_id": req.RequestID}
		if err := client.WriteJSON(sendCtx, command); err != nil {
			s.installedApps.resolve(client, appsError(req, "send_failed"), p)
		}
	}()
}

func validInstalledApp(a InstalledApp) bool {
	return strings.TrimSpace(a.Name) != "" && utf8.RuneCountInString(a.Name) <= maxInstalledAppName && utf8.RuneCountInString(a.Version) <= maxInstalledAppVersion && utf8.RuneCountInString(a.Publisher) <= maxInstalledAppPublisher && utf8.RuneCountInString(a.InstallDate) <= maxInstalledAppDate && (a.EstimatedSizeKB == nil || *a.EstimatedSizeKB <= maxInstalledAppSizeKB)
}

func (s *Server) handleAgentInstalledApps(client *Client, raw json.RawMessage) {
	// Extract only correlation first. Malformed/unidentifiable JSON times out;
	// a identifiable invalid result fails only its own connection-bound request.
	var header struct {
		Type      string `json:"type"`
		Action    string `json:"action"`
		RequestID string `json:"request_id"`
	}
	if json.Unmarshal(raw, &header) != nil || !virusscan.ValidID(header.RequestID) {
		return
	}
	req := InstalledAppsRequest{AgentID: client.Info.ID, RequestID: header.RequestID}
	resolve := func(r InstalledAppsResult) {
		lock := s.lifecycleLock(client.Info.ID)
		lock.Lock()
		current, online := s.registry.Get(client.Info.ID)
		var pending *installedAppsPending
		if online && current == client {
			pending = s.installedApps.take(client, r.RequestID, nil)
		}
		lock.Unlock()
		// The reserved tracker slot bounds this response worker. A slow browser or
		// audit database must not stall the Agent's performance/process read loop.
		if pending != nil {
			go s.installedApps.finish(pending, r)
		}
	}
	bad := func() { resolve(appsError(req, "invalid_result")) }
	if len(raw) > maxInstalledAppsPayload || header.Type != "installed_apps" || header.Action != "result" {
		bad()
		return
	}
	var event struct {
		Success *bool           `json:"success"`
		Apps    json.RawMessage `json:"apps"`
		Error   string          `json:"error"`
	}
	if json.Unmarshal(raw, &event) != nil || event.Success == nil || len(event.Error) > maxInstalledAppsError {
		bad()
		return
	}
	if !*event.Success {
		if event.Error == "" || event.Apps != nil {
			bad()
			return
		}
		resolve(appsError(req, "collection_failed"))
		return
	}
	if event.Apps == nil || event.Error != "" {
		bad()
		return
	}
	apps, err := decodeInstalledApps(event.Apps)
	if err != nil {
		bad()
		return
	}
	resolve(InstalledAppsResult{RequestID: header.RequestID, Success: true, Apps: &apps})
}

// Enforce the count while decoding instead of allocating an attacker-supplied
// million-element slice first. The enclosing envelope already validates JSON.
func decodeInstalledApps(raw json.RawMessage) ([]InstalledApp, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('[') {
		return nil, errors.New("apps must be an array")
	}
	apps := make([]InstalledApp, 0)
	for decoder.More() {
		if len(apps) >= maxInstalledApps {
			return nil, errors.New("too many apps")
		}
		var app InstalledApp
		if err := decoder.Decode(&app); err != nil {
			return nil, err
		}
		if !validInstalledApp(app) {
			return nil, errors.New("invalid app")
		}
		apps = append(apps, app)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return apps, nil
}

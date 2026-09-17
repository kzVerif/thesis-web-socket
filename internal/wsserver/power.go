package wsserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"ws-rat/internal/virusscan"
)

type FrontendPermissionStore interface {
	AuthenticatePermission(context.Context, string, string) (string, bool, error)
}

type PowerRoomStore interface {
	ListAgentIDsByRoom(context.Context, string) ([]string, error)
}

func (s *Server) handlePowerRequest(f *FrontendClient, user, tokenHash string, raw json.RawMessage) {
	var req PowerRequest
	err := json.Unmarshal(raw, &req)
	fail := func(code, message string) {
		_ = f.WriteJSON(map[string]any{"type": "error", "stream": "power", "action": req.Action, "request_id": req.RequestID, "agent_id": req.AgentID, "room_id": req.RoomID, "code": code, "error": message})
	}
	if err != nil || req.Type != "power" || !virusscan.ValidID(req.RequestID) ||
		(req.Action != "shutdown" && req.Action != "shutdown_room") ||
		(req.Action == "shutdown" && (!virusscan.ValidID(req.AgentID) || req.RoomID != "")) ||
		(req.Action == "shutdown_room" && (!virusscan.ValidID(req.RoomID) || req.AgentID != "")) {
		fail("invalid_request", "valid UUID request_id and matching agent_id or room_id are required")
		return
	}
	// UUIDs from PostgreSQL are canonical lowercase. Preserve request_id on the wire.
	req.AgentID, req.RoomID = strings.ToLower(req.AgentID), strings.ToLower(req.RoomID)
	permissions, ok := s.sessions.(FrontendPermissionStore)
	if !ok {
		fail("authorization_unavailable", "power authorization unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	authUser, allowed, err := permissions.AuthenticatePermission(ctx, tokenHash, "agents.control")
	if err != nil || authUser != user || authUser == "" {
		fail("authorization_unavailable", "cannot authorize power request")
		return
	}
	if !allowed {
		fail("forbidden", "agents.control permission required")
		return
	}
	var clients []*Client
	total := 1
	if req.Action == "shutdown" {
		agent, err := s.agents.GetByID(ctx, req.AgentID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && agent == nil) {
			fail("agent_not_found", "agent does not exist")
			return
		}
		if err != nil {
			fail("lookup_failed", "cannot load agent")
			return
		}
		client, online := s.registry.Get(agent.ID)
		if !online {
			fail("agent_offline", "agent is offline")
			return
		}
		clients = append(clients, client)
	} else {
		rooms, ok := s.agents.(PowerRoomStore)
		if !ok {
			fail("lookup_failed", "room lookup unavailable")
			return
		}
		ids, err := rooms.ListAgentIDsByRoom(ctx, req.RoomID)
		if errors.Is(err, sql.ErrNoRows) {
			fail("room_not_found", "room does not exist")
			return
		}
		if err != nil {
			fail("lookup_failed", "cannot load room agents")
			return
		}
		total = len(ids)
		for _, id := range ids {
			if c, online := s.registry.Get(id); online {
				clients = append(clients, c)
			}
		}
	}
	p, err := s.power.Register(req, f, clients, total, func(result any) {
		_ = f.WriteJSON(result)
		detail := map[string]any{"request_id": req.RequestID, "room_id": req.RoomID}
		switch r := result.(type) {
		case PowerResult:
			detail["success"], detail["mode"], detail["code"] = r.Success, r.Mode, r.Code
		case PowerRoomResult:
			detail["total"], detail["online"], detail["offline"] = r.Total, r.Online, r.Offline
			detail["accepted"], detail["failed"], detail["timeout"] = r.Accepted, r.Failed, r.Timeout
		}
		s.recordAudit(user, "power."+req.Action+".result", req.AgentID, "", detail)
	})
	if err != nil {
		fail("duplicate_request", err.Error())
		return
	}
	s.recordAudit(user, "power."+req.Action+".requested", req.AgentID, "", map[string]any{"request_id": req.RequestID, "room_id": req.RoomID, "total": total, "online": len(clients)})
	if len(clients) == 0 {
		s.power.CompleteEmpty(p)
		return
	}
	// Independent sends prevent one slow socket from delaying the whole room or
	// blocking frontend disconnect cleanup. The pending operation owns cancellation.
	for _, c := range clients {
		go func(client *Client) {
			ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
			defer cancel()
			if ctx.Err() != nil {
				return
			}
			command := map[string]string{"type": "power", "action": "shutdown", "request_id": req.RequestID}
			if err := client.WriteJSON(ctx, command); err != nil {
				s.power.resolve(client, PowerResult{RequestID: req.RequestID, Mode: "mock", Code: "send_failed", Message: "could not send shutdown command"}, p)
			}
		}(c)
	}
}

func (s *Server) handleAgentPower(client *Client, raw json.RawMessage) {
	// A missing success flag must not be interpreted as a legitimate failure.
	var event struct {
		PowerResult
		Success *bool `json:"success"`
	}
	if json.Unmarshal(raw, &event) != nil || event.Type != "power" || event.Action != "shutdown_result" ||
		!virusscan.ValidID(event.RequestID) || event.Success == nil || event.Mode != "mock" || len(event.Message) > 4096 {
		s.logger.Printf("invalid power result from %s", client.Info.ID)
		return
	}
	event.PowerResult.Success = *event.Success
	event.PowerResult.Code = "" // Server error codes cannot be supplied by an agent.
	s.power.Resolve(client, event.PowerResult)
}

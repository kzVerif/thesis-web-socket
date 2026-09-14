package wsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type StreamRequest struct {
	Type    string `json:"type"`
	Action  string `json:"action"`
	AgentID string `json:"agent_id"`
	RoomID  string `json:"room_id"`
	PID     int64  `json:"pid,omitempty"`
}

type StreamCommand struct {
	Type   string `json:"type"`
	Action string `json:"action"`
	PID    int64  `json:"pid,omitempty"`
}

func (server *Server) HandleFrontendWebSocket(writer http.ResponseWriter, request *http.Request) {
	cookie, err := request.Cookie("__Host-session")
	if err != nil || cookie.Value == "" {
		http.Error(writer, "ขาดการ login", http.StatusUnauthorized)
		return
	}
	tokenHash := hashToken(cookie.Value)
	user, allowed, err := server.authenticateFrontend(request.Context(), tokenHash)
	if err != nil || user == "" {
		http.Error(writer, "ขาดการ login", http.StatusUnauthorized)
		return
	}
	conn, err := websocket.Accept(writer, request, &websocket.AcceptOptions{OriginPatterns: server.frontendOrigins})
	if err != nil {
		server.logger.Printf("accept frontend WebSocket: %v", err)
		return
	}
	defer conn.CloseNow()
	frontend := &FrontendClient{Conn: conn}
	server.subscriptions.AddDashboard(frontend)
	defer server.subscriptions.RemoveDashboard(frontend)
	defer server.removeFrontend(frontend)

	for {
		var raw json.RawMessage
		if err := wsjson.Read(context.Background(), conn, &raw); err != nil {
			return
		}
		user, allowed, err = server.authenticateFrontend(request.Context(), tokenHash)
		if err != nil || user == "" {
			_ = frontend.WriteJSON(map[string]any{"type": "error", "error": "ขาดการ login"})
			_ = conn.Close(websocket.StatusPolicyViolation, "ขาดการ login")
			return
		}
		detail, target := frontendAuditDetail(raw)
		detail["phase"] = "requested"
		server.recordAudit(user, "frontend_request", target, peerIP(request.RemoteAddr), detail)
		var envelope struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &envelope) != nil {
			_ = frontend.WriteJSON(map[string]any{"type": "error", "error": "invalid JSON"})
			continue
		}
		if envelope.Type == "FILE_DISTRIBUTE" {
			if !allowed {
				_ = frontend.WriteJSON(map[string]any{"type": "error", "error": "ไม่มี permission"})
				continue
			}
			if err := server.handleDistributionFrontend(frontend, user, raw); err != nil {
				_ = frontend.WriteJSON(map[string]any{"type": "error", "error": err.Error()})
			}
			continue
		}
		if envelope.Type == "virus_scan" || envelope.Type == "virus_scan_list" {
			if err := server.handleVirusScanFrontend(frontend, user, raw); err != nil {
				_ = frontend.WriteJSON(map[string]any{"type": "error", "stream": "virus_scan", "error": err.Error()})
			}
			continue
		}
		var command StreamRequest
		if json.Unmarshal(raw, &command) != nil {
			continue
		}
		if err := server.handleStreamRequest(frontend, command); err != nil {
			_ = frontend.WriteJSON(map[string]any{"type": "error", "stream": command.Type, "agent_id": command.AgentID, "error": err.Error()})
		}
	}
}

func (server *Server) handleStreamRequest(frontend *FrontendClient, request StreamRequest) error {
	stream := strings.ToLower(request.Type)
	if stream == "screen" {
		return server.handleScreenRequest(frontend, request)
	}
	if (stream != "performance" && stream != "process") || request.AgentID == "" {
		return fmt.Errorf("type must be performance or process and agent_id is required")
	}
	action := strings.ToLower(request.Action)
	if action == "kill" {
		return server.handleProcessKill(frontend, request)
	}
	if action != "start" && action != "stop" {
		return fmt.Errorf("action must be start, stop, or kill")
	}

	agent, online := server.registry.Get(request.AgentID)
	if action == "start" {
		if !online {
			return fmt.Errorf("agent is offline")
		}
		first := server.subscriptions.Subscribe(stream, request.AgentID, frontend)
		if first {
			if err := server.sendStreamCommand(agent, stream, "start"); err != nil {
				server.subscriptions.Unsubscribe(stream, request.AgentID, frontend)
				return err
			}
		}
		return frontend.WriteJSON(map[string]any{"type": "subscribed", "stream": stream, "agent_id": request.AgentID})
	}

	last := server.subscriptions.Unsubscribe(stream, request.AgentID, frontend)
	if last && online {
		if err := server.sendStreamCommand(agent, stream, "stop"); err != nil {
			return err
		}
	}
	return frontend.WriteJSON(map[string]any{"type": "unsubscribed", "stream": stream, "agent_id": request.AgentID})
}

func (server *Server) handleScreenRequest(frontend *FrontendClient, request StreamRequest) error {
	if request.RoomID == "" {
		return fmt.Errorf("room_id is required for screen stream")
	}
	action := strings.ToLower(request.Action)
	if action != "start" && action != "stop" {
		return fmt.Errorf("screen action must be start or stop")
	}
	if action == "start" {
		first := server.subscriptions.SubscribeRoom(request.RoomID, frontend)
		if first {
			server.sendRoomStreamCommand(request.RoomID, "start")
		}
		return frontend.WriteJSON(map[string]any{"type": "subscribed", "stream": "screen", "room_id": request.RoomID})
	}
	last := server.subscriptions.UnsubscribeRoom(request.RoomID, frontend)
	if last {
		server.sendRoomStreamCommand(request.RoomID, "stop")
	}
	return frontend.WriteJSON(map[string]any{"type": "unsubscribed", "stream": "screen", "room_id": request.RoomID})
}

func (server *Server) sendRoomStreamCommand(roomID, action string) {
	for _, agent := range server.registry.InRoom(roomID) {
		if err := server.sendStreamCommand(agent, "screen", action); err != nil {
			server.logger.Printf("send screen %s to agent %s: %v", action, agent.Info.ID, err)
		}
	}
}

func (server *Server) handleProcessKill(frontend *FrontendClient, request StreamRequest) error {
	if strings.ToLower(request.Type) != "process" {
		return fmt.Errorf("kill action is only supported for process")
	}
	if request.PID <= 0 {
		return fmt.Errorf("pid must be an integer greater than 0")
	}
	agent, online := server.registry.Get(request.AgentID)
	if !online {
		return fmt.Errorf("agent is offline")
	}
	if err := server.processKills.Register(request.AgentID, request.PID, frontend); err != nil {
		return err
	}
	if err := frontend.WriteJSON(map[string]any{
		"type": "process", "action": "kill_accepted",
		"agent_id": request.AgentID, "pid": request.PID,
	}); err != nil {
		server.processKills.Cancel(request.AgentID, request.PID, frontend)
		return err
	}
	if err := server.sendProcessKillCommand(agent, request.PID); err != nil {
		server.processKills.Cancel(request.AgentID, request.PID, frontend)
		return err
	}
	return nil
}

func (server *Server) sendProcessKillCommand(agent *Client, pid int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := StreamCommand{Type: "process", Action: "kill", PID: pid}
	if err := agent.WriteJSON(ctx, command); err != nil {
		return fmt.Errorf("send process kill for pid %d to agent %s: %w", pid, agent.Info.ID, err)
	}
	return nil
}

func (server *Server) sendStreamCommand(agent *Client, stream, action string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := agent.WriteJSON(ctx, StreamCommand{Type: stream, Action: action}); err != nil {
		return fmt.Errorf("send %s %s to agent %s: %w", stream, action, agent.Info.ID, err)
	}
	return nil
}

func (server *Server) removeFrontend(frontend *FrontendClient) {
	server.processKills.RemoveFrontend(frontend)
	for _, roomID := range server.subscriptions.RemoveClientRooms(frontend) {
		server.sendRoomStreamCommand(roomID, "stop")
	}
	for _, key := range server.subscriptions.RemoveClient(frontend) {
		if agent, online := server.registry.Get(key.AgentID); online {
			if err := server.sendStreamCommand(agent, key.Stream, "stop"); err != nil {
				server.logger.Printf("stop %s for %s: %v", key.Stream, key.AgentID, err)
			}
		}
	}
}

// FrontendSessionStore validates the session and returns file distribution permission.
type FrontendSessionStore interface {
	Authenticate(context.Context, string) (string, bool, error)
}

func (server *Server) authenticateFrontend(ctx context.Context, tokenHash string) (string, bool, error) {
	if server.sessions == nil {
		return "", false, fmt.Errorf("session authentication unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, databaseTimeout)
	defer cancel()
	return server.sessions.Authenticate(ctx, tokenHash)
}

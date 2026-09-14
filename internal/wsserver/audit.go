package wsserver

import (
	"context"
	"encoding/json"
	"net"

	"ws-rat/internal/virusscan"
)

type AuditStore interface {
	WriteAudit(context.Context, string, string, string, string, []byte) error
}

func (s *Server) ConfigureAudit(store AuditStore) { s.audit = store }

func (s *Server) recordAudit(user, action, agent, ip string, detail map[string]any) {
	if s.audit == nil {
		return
	}
	if !virusscan.ValidID(agent) {
		agent = ""
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		s.logger.Printf("encode audit: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	if err := s.audit.WriteAudit(ctx, user, action, agent, ip, raw); err != nil {
		s.logger.Printf("write audit %s: %v", action, err)
	}
}

// Select metadata explicitly; never persist credentials, binary data or arbitrary payloads.
func frontendAuditDetail(raw json.RawMessage) (map[string]any, string) {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	detail := map[string]any{}
	agent := ""
	for _, key := range []string{"type", "action", "agent_id", "room_id", "job_id", "request_id", "file_id", "scan_type"} {
		var value string
		if json.Unmarshal(fields[key], &value) == nil && value != "" {
			if key == "agent_id" {
				agent = value
			}
			runes := []rune(value)
			if len(runes) > 128 {
				value = string(runes[:128])
			}
			detail[key] = value
		}
	}
	for _, key := range []string{"pid", "limit"} {
		var value int64
		if json.Unmarshal(fields[key], &value) == nil {
			detail[key] = value
		}
	}
	var ids []string
	if json.Unmarshal(fields["agent_ids"], &ids) == nil {
		detail["target_count"] = len(ids)
	}
	var target struct {
		Type     string   `json:"type"`
		RoomID   string   `json:"room_id"`
		AgentIDs []string `json:"agent_ids"`
	}
	if json.Unmarshal(fields["target"], &target) == nil {
		if target.Type == "ROOM" || target.Type == "AGENTS" {
			detail["target_type"] = target.Type
		}
		if virusscan.ValidID(target.RoomID) {
			detail["room_id"] = target.RoomID
		}
		detail["target_count"] = len(target.AgentIDs)
	}
	return detail, agent
}

func peerIP(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return ""
}

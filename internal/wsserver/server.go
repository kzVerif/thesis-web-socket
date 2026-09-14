package wsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"ws-rat/internal/distribution"
	"ws-rat/internal/model"
)

const databaseTimeout = 5 * time.Second
const maxAgentMessageSize = 10 << 20 // Screen JPEG plus a small safety margin.

type AgentStore interface {
	GetByID(ctx context.Context, id string) (*model.AgentInfo, error)
	UpdateStatus(ctx context.Context, id, status string) error
}

type Server struct {
	audit            AuditStore
	sessions         FrontendSessionStore
	virusScans       VirusScanStore
	agents           AgentStore
	registry         *Registry
	subscriptions    *SubscriptionHub
	processKills     *ProcessKillTracker
	logger           *log.Logger
	frontendOrigins  []string
	distributionRepo *distribution.Repository
	distributions    *distribution.Service
	storageRoot      string
}

func New(agents AgentStore, logger *log.Logger, frontendOrigins []string) *Server {
	return &Server{
		agents:          agents,
		registry:        NewRegistry(),
		subscriptions:   NewSubscriptionHub(),
		processKills:    NewProcessKillTracker(),
		logger:          logger,
		frontendOrigins: frontendOrigins,
	}
}

func (server *Server) ConfigureDistribution(repo *distribution.Repository, baseURL, storageRoot string, ttl time.Duration) {
	server.distributionRepo = repo
	server.sessions = repo
	server.storageRoot = storageRoot
	server.distributions = &distribution.Service{Repo: repo, Sender: server, BaseURL: baseURL, TTL: ttl}
}

func (server *Server) HandleWebSocket(writer http.ResponseWriter, request *http.Request) {
	conn, err := websocket.Accept(writer, request, nil)
	if err != nil {
		server.logger.Printf("accept WebSocket: %v", err)
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(maxAgentMessageSize)

	ctx := context.Background()
	var registration model.AgentInfo
	if err := wsjson.Read(ctx, conn, &registration); err != nil {
		server.logger.Printf("read agent registration: %v", err)
		return
	}
	if registration.ID == "" {
		server.logger.Print("agent ID is empty")
		return
	}

	agent, err := server.getAgent(registration.ID)
	if err != nil {
		server.logger.Printf("cannot get agent: %v", err)
		return
	}
	if err := server.setStatus(agent.ID, model.StatusOnline); err != nil {
		server.logger.Printf("cannot mark agent online: %v", err)
		return
	}

	client := &Client{Info: *agent, Conn: conn}
	previous := server.registry.Add(client)
	if previous != nil && previous.Conn != conn {
		_ = previous.Conn.Close(websocket.StatusPolicyViolation, "replaced by a newer connection")
	}

	done := make(chan struct{})
	defer close(done)
	go monitorConnection(conn, done)
	defer server.disconnect(client)

	server.logConnected(agent)
	if agent.RoomID != "" && server.subscriptions.RoomActive(agent.RoomID) {
		if err := server.sendStreamCommand(client, "screen", "start"); err != nil {
			server.logger.Printf("start screen stream for %s: %v", agent.ID, err)
		}
	}
	for _, stream := range server.subscriptions.ActiveStreams(agent.ID) {
		if err := server.sendStreamCommand(client, stream, "start"); err != nil {
			server.logger.Printf("restart %s stream for %s: %v", stream, agent.ID, err)
		}
	}
	server.readMessages(ctx, client)
}

func (server *Server) getAgent(id string) (*model.AgentInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	return server.agents.GetByID(ctx, id)
}

func (server *Server) setStatus(id, status string) error {
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	return server.agents.UpdateStatus(ctx, id, status)
}

func (server *Server) disconnect(client *Client) {
	if !server.registry.Remove(client.Info.ID, client.Conn) {
		return
	}
	if err := server.setStatus(client.Info.ID, model.StatusOffline); err != nil {
		server.logger.Printf("cannot mark agent %s offline: %v", client.Info.ID, err)
	}
	server.subscriptions.PublishStatus(client.Info.ID, "offline")
	server.processKills.FailAgent(client.Info.ID, "agent disconnected")
	server.logger.Printf("agent disconnected: id=%s hostname=%s", client.Info.ID, client.Info.Hostname)
	server.logOnlineClients()
}

func (server *Server) readMessages(ctx context.Context, client *Client) {
	for {
		messageType, payload, err := client.Conn.Read(ctx)
		if err != nil {
			server.logger.Printf("connection closed: hostname=%s error=%v", client.Info.Hostname, err)
			return
		}
		if messageType == websocket.MessageBinary {
			if client.Info.RoomID == "" || len(payload) < 4 || payload[0] != 0xff || payload[1] != 0xd8 {
				server.logger.Printf("invalid screen frame from %s", client.Info.ID)
				continue
			}
			server.subscriptions.PublishScreen(client.Info.ID, client.Info.RoomID, payload)
			continue
		}
		message := json.RawMessage(payload)
		message = bytes.TrimSpace(message)
		if len(message) > 0 && message[0] == '[' {
			var processes []model.ProcessInfo
			if err := json.Unmarshal(message, &processes); err != nil {
				server.logger.Printf("invalid process list from %s: %v", client.Info.ID, err)
				continue
			}
			if err := model.ValidateProcesses(processes); err != nil {
				server.logger.Printf("invalid process list from %s: %v", client.Info.ID, err)
				continue
			}
			server.subscriptions.PublishProcesses(client.Info.ID, processes)
			continue
		}
		var fields map[string]any
		if err := json.Unmarshal(message, &fields); err == nil {
			if kind, _ := fields["type"].(string); server.handleAgentVirusScan(client, message, kind) {
				continue
			}
			if kind, _ := fields["type"].(string); server.distributionRepo != nil && server.handleAgentDistribution(client, message, kind) {
				continue
			}
			if fields["type"] == "process" && fields["action"] == "kill_result" {
				var result model.ProcessKillResult
				if err := json.Unmarshal(message, &result); err != nil || result.PID <= 0 {
					server.logger.Printf("invalid process kill result from %s: %s", client.Info.ID, message)
					continue
				}
				if server.processKills.Resolve(client.Info.ID, result) {
					server.recordAudit("", "process_kill_result", client.Info.ID, "", map[string]any{"pid": result.PID, "success": result.Success})
				} else {
					server.logger.Printf("unmatched process kill result from %s for pid %d", client.Info.ID, result.PID)
				}
				continue
			}
			if _, isPerformance := fields["cpu_usage"]; isPerformance {
				var sample model.PerformanceSample
				if err := json.Unmarshal(message, &sample); err != nil {
					server.logger.Printf("invalid performance from %s: %v", client.Info.ID, err)
					continue
				}
				if err := sample.Validate(); err != nil {
					server.logger.Printf("invalid performance from %s: %v", client.Info.ID, err)
					continue
				}
				server.subscriptions.PublishPerformance(client.Info.ID, sample)
				continue
			}
		}
		server.logger.Printf("message from %s: %s", client.Info.Hostname, message)
	}
}

func (server *Server) logConnected(agent *model.AgentInfo) {
	server.logger.Printf("agent connected: id=%s hostname=%s os=%s/%s/%s ip=%s mac=%s",
		agent.ID, agent.Hostname, agent.OSInfo.Name, agent.OSInfo.Edition,
		agent.OSInfo.Version, agent.IPAddress, agent.MACAddress)
	server.logOnlineClients()
}

func (server *Server) logOnlineClients() {
	agents := server.registry.Snapshot()
	server.logger.Printf("online agents: %d", len(agents))
	for _, agent := range agents {
		server.logger.Printf("id=%s hostname=%s", agent.ID, agent.Hostname)
	}
}

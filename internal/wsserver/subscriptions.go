package wsserver

import (
	"context"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"ws-rat/internal/distribution"
	"ws-rat/internal/model"
)

const frontendWriteTimeout = 5 * time.Second

type SubscriptionKey struct {
	Stream  string
	AgentID string
}

type FrontendClient struct {
	Conn    *websocket.Conn
	writeMu sync.Mutex
}

func (client *FrontendClient) WriteScreen(agentID, roomID string, jpeg []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), frontendWriteTimeout)
	defer cancel()
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	header := map[string]any{"type": "screen", "agent_id": agentID, "room_id": roomID}
	if err := wsjson.Write(ctx, client.Conn, header); err != nil {
		return err
	}
	return client.Conn.Write(ctx, websocket.MessageBinary, jpeg)
}

func (client *FrontendClient) WriteJSON(value any) error {
	ctx, cancel := context.WithTimeout(context.Background(), frontendWriteTimeout)
	defer cancel()
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	return wsjson.Write(ctx, client.Conn, value)
}

type SubscriptionHub struct {
	mu          sync.RWMutex
	subscribers map[SubscriptionKey]map[*FrontendClient]struct{}
	rooms       map[string]map[*FrontendClient]struct{}
	dashboards  map[*FrontendClient]struct{}
}

func NewSubscriptionHub() *SubscriptionHub {
	return &SubscriptionHub{
		subscribers: make(map[SubscriptionKey]map[*FrontendClient]struct{}),
		rooms:       make(map[string]map[*FrontendClient]struct{}),
		dashboards:  make(map[*FrontendClient]struct{}),
	}
}
func (h *SubscriptionHub) AddDashboard(c *FrontendClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dashboards[c] = struct{}{}
}
func (h *SubscriptionHub) RemoveDashboard(c *FrontendClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.dashboards, c)
}
func (h *SubscriptionHub) PublishDistribution(v distribution.Update) {
	h.mu.RLock()
	cs := make([]*FrontendClient, 0, len(h.dashboards))
	for c := range h.dashboards {
		cs = append(cs, c)
	}
	h.mu.RUnlock()
	for _, c := range cs {
		_ = c.WriteJSON(v)
	}
}

func (hub *SubscriptionHub) SubscribeRoom(roomID string, client *FrontendClient) bool {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	clients := hub.rooms[roomID]
	if clients == nil {
		clients = make(map[*FrontendClient]struct{})
		hub.rooms[roomID] = clients
	}
	_, existed := clients[client]
	clients[client] = struct{}{}
	return !existed && len(clients) == 1
}

func (hub *SubscriptionHub) UnsubscribeRoom(roomID string, client *FrontendClient) bool {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	clients := hub.rooms[roomID]
	if _, exists := clients[client]; !exists {
		return false
	}
	delete(clients, client)
	if len(clients) == 0 {
		delete(hub.rooms, roomID)
		return true
	}
	return false
}

func (hub *SubscriptionHub) RoomActive(roomID string) bool {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	return len(hub.rooms[roomID]) > 0
}

func (hub *SubscriptionHub) PublishScreen(agentID, roomID string, jpeg []byte) {
	hub.mu.RLock()
	clients := make([]*FrontendClient, 0, len(hub.rooms[roomID]))
	for client := range hub.rooms[roomID] {
		clients = append(clients, client)
	}
	hub.mu.RUnlock()
	for _, client := range clients {
		_ = client.WriteScreen(agentID, roomID, jpeg)
	}
}

func (hub *SubscriptionHub) Subscribe(stream, agentID string, client *FrontendClient) bool {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	key := SubscriptionKey{Stream: stream, AgentID: agentID}
	clients := hub.subscribers[key]
	if clients == nil {
		clients = make(map[*FrontendClient]struct{})
		hub.subscribers[key] = clients
	}
	_, existed := clients[client]
	clients[client] = struct{}{}
	return !existed && len(clients) == 1
}

func (hub *SubscriptionHub) Unsubscribe(stream, agentID string, client *FrontendClient) bool {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	key := SubscriptionKey{Stream: stream, AgentID: agentID}
	clients := hub.subscribers[key]
	if clients == nil {
		return false
	}
	if _, exists := clients[client]; !exists {
		return false
	}
	delete(clients, client)
	if len(clients) == 0 {
		delete(hub.subscribers, key)
		return true
	}
	return false
}

func (hub *SubscriptionHub) RemoveClient(client *FrontendClient) []SubscriptionKey {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	var nowEmpty []SubscriptionKey
	for key, clients := range hub.subscribers {
		if _, exists := clients[client]; !exists {
			continue
		}
		delete(clients, client)
		if len(clients) == 0 {
			delete(hub.subscribers, key)
			nowEmpty = append(nowEmpty, key)
		}
	}
	return nowEmpty
}

func (hub *SubscriptionHub) RemoveClientRooms(client *FrontendClient) []string {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	var empty []string
	for roomID, clients := range hub.rooms {
		if _, exists := clients[client]; !exists {
			continue
		}
		delete(clients, client)
		if len(clients) == 0 {
			delete(hub.rooms, roomID)
			empty = append(empty, roomID)
		}
	}
	return empty
}

func (hub *SubscriptionHub) ActiveStreams(agentID string) []string {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	var streams []string
	for key, clients := range hub.subscribers {
		if key.AgentID == agentID && len(clients) > 0 {
			streams = append(streams, key.Stream)
		}
	}
	return streams
}

func (hub *SubscriptionHub) snapshot(stream, agentID string) []*FrontendClient {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	key := SubscriptionKey{Stream: stream, AgentID: agentID}
	clients := make([]*FrontendClient, 0, len(hub.subscribers[key]))
	for client := range hub.subscribers[key] {
		clients = append(clients, client)
	}
	return clients
}

func (hub *SubscriptionHub) PublishPerformance(agentID string, sample model.PerformanceSample) {
	event := model.PerformanceEvent{Type: "performance", AgentID: agentID, Data: sample, ReceivedAt: time.Now().UTC()}
	hub.publish("performance", agentID, event)
}

func (hub *SubscriptionHub) PublishProcesses(agentID string, processes []model.ProcessInfo) {
	event := model.ProcessEvent{Type: "process", AgentID: agentID, Data: processes, ReceivedAt: time.Now().UTC()}
	hub.publish("process", agentID, event)
}

func (hub *SubscriptionHub) PublishStatus(agentID, status string) {
	event := map[string]any{"type": "agent_status", "agent_id": agentID, "status": status}
	clients := make(map[*FrontendClient]struct{})
	for _, stream := range []string{"performance", "process"} {
		for _, client := range hub.snapshot(stream, agentID) {
			clients[client] = struct{}{}
		}
	}
	for client := range clients {
		_ = client.WriteJSON(event)
	}
}

func (hub *SubscriptionHub) publish(stream, agentID string, event any) {
	for _, client := range hub.snapshot(stream, agentID) {
		_ = client.WriteJSON(event)
	}
}

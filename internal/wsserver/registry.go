package wsserver

import (
	"context"
	"sync"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"ws-rat/internal/model"
)

type Client struct {
	Info    model.AgentInfo
	Conn    *websocket.Conn
	writeMu sync.Mutex
}

func (client *Client) WriteJSON(ctx context.Context, value any) error {
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	return wsjson.Write(ctx, client.Conn, value)
}

type Registry struct {
	mu      sync.RWMutex
	clients map[string]*Client
}

func NewRegistry() *Registry {
	return &Registry{clients: make(map[string]*Client)}
}

func (registry *Registry) Add(client *Client) (previous *Client) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	previous = registry.clients[client.Info.ID]
	registry.clients[client.Info.ID] = client
	return previous
}

func (registry *Registry) Remove(id string, conn *websocket.Conn) bool {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	client, exists := registry.clients[id]
	if !exists || client.Conn != conn {
		return false
	}
	delete(registry.clients, id)
	return true
}

func (registry *Registry) Snapshot() []model.AgentInfo {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	agents := make([]model.AgentInfo, 0, len(registry.clients))
	for _, client := range registry.clients {
		agents = append(agents, client.Info)
	}
	return agents
}

func (registry *Registry) Get(id string) (*Client, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	client, exists := registry.clients[id]
	return client, exists
}

func (registry *Registry) InRoom(roomID string) []*Client {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	clients := make([]*Client, 0)
	for _, client := range registry.clients {
		if client.Info.RoomID == roomID {
			clients = append(clients, client)
		}
	}
	return clients
}

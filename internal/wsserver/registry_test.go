package wsserver

import (
	"testing"

	"github.com/coder/websocket"
	"ws-rat/internal/model"
)

func TestRegistryDoesNotRemoveReplacementConnection(t *testing.T) {
	registry := NewRegistry()
	oldConnection := &websocket.Conn{}
	newConnection := &websocket.Conn{}
	oldClient := &Client{Info: model.AgentInfo{ID: "agent-1"}, Conn: oldConnection}
	newClient := &Client{Info: model.AgentInfo{ID: "agent-1"}, Conn: newConnection}

	registry.Add(oldClient)
	if previous := registry.Add(newClient); previous != oldClient {
		t.Fatal("expected old client to be returned")
	}
	if registry.Remove("agent-1", oldConnection) {
		t.Fatal("old connection must not remove its replacement")
	}
	if len(registry.Snapshot()) != 1 {
		t.Fatal("replacement connection should remain registered")
	}
	if !registry.Remove("agent-1", newConnection) {
		t.Fatal("active connection should be removable")
	}
}

package wsserver

import "testing"

func TestSubscriptionReferenceCounting(t *testing.T) {
	hub := NewSubscriptionHub()
	first := &FrontendClient{}
	second := &FrontendClient{}
	if !hub.Subscribe("performance", "agent-1", first) {
		t.Fatal("first subscriber should start the agent stream")
	}
	if hub.Subscribe("performance", "agent-1", second) {
		t.Fatal("second subscriber must not start a duplicate stream")
	}
	if hub.Unsubscribe("performance", "agent-1", first) {
		t.Fatal("stream must continue while another subscriber exists")
	}
	if !hub.Unsubscribe("performance", "agent-1", second) {
		t.Fatal("last subscriber should stop the agent stream")
	}
}

func TestRemoveFrontendReturnsOnlyEmptyAgentStreams(t *testing.T) {
	hub := NewSubscriptionHub()
	frontend := &FrontendClient{}
	other := &FrontendClient{}
	hub.Subscribe("performance", "agent-1", frontend)
	hub.Subscribe("process", "agent-2", frontend)
	hub.Subscribe("process", "agent-2", other)
	empty := hub.RemoveClient(frontend)
	if len(empty) != 1 || empty[0] != (SubscriptionKey{Stream: "performance", AgentID: "agent-1"}) {
		t.Fatalf("unexpected empty subscriptions: %v", empty)
	}
}

func TestProcessAndPerformanceSubscriptionsAreIndependent(t *testing.T) {
	hub := NewSubscriptionHub()
	frontend := &FrontendClient{}
	if !hub.Subscribe("performance", "agent-1", frontend) {
		t.Fatal("first performance subscription should start its stream")
	}
	if !hub.Subscribe("process", "agent-1", frontend) {
		t.Fatal("first process subscription should start its own stream")
	}
	if !hub.Unsubscribe("process", "agent-1", frontend) {
		t.Fatal("process stream should stop independently")
	}
	streams := hub.ActiveStreams("agent-1")
	if len(streams) != 1 || streams[0] != "performance" {
		t.Fatalf("performance stream should remain active: %v", streams)
	}
}

func TestRoomScreenSubscriptionReferenceCounting(t *testing.T) {
	hub := NewSubscriptionHub()
	first := &FrontendClient{}
	second := &FrontendClient{}
	if !hub.SubscribeRoom("room-1", first) {
		t.Fatal("first room viewer should start screen streams")
	}
	if hub.SubscribeRoom("room-1", second) {
		t.Fatal("second room viewer must not start duplicate streams")
	}
	if hub.UnsubscribeRoom("room-1", first) {
		t.Fatal("screen streams must continue while a room viewer remains")
	}
	if !hub.UnsubscribeRoom("room-1", second) {
		t.Fatal("last room viewer should stop screen streams")
	}
}

package wsserver

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"ws-rat/internal/model"
)

func TestFrontendRoomScreenRoundTrip(t *testing.T) {
	store := &fakeAgentStore{agent: model.AgentInfo{ID: authTestID, RoomID: "room-1", Hostname: "screen-agent"}, statuses: make(chan string, 2)}
	server := New(store, log.New(io.Discard, "", 0), []string{"frontend.test"})
	server.sessions = validFrontendSession{}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", server.HandleWebSocket)
	mux.HandleFunc("/ws/frontend", server.HandleFrontendWebSocket)
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	agentConn, _, err := websocket.Dial(ctx, wsURL+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer agentConn.CloseNow()
	authenticateTestAgent(t, ctx, agentConn, authTestID)
	if err := wsjson.Write(ctx, agentConn, model.AgentInfo{ID: authTestID}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-store.statuses:
	case <-ctx.Done():
		t.Fatal("agent registration timed out")
	}

	frontendConn, _, err := websocket.Dial(ctx, wsURL+"/ws/frontend", &websocket.DialOptions{HTTPHeader: http.Header{"Cookie": []string{"__Host-session=test-session"}, "Origin": []string{"http://frontend.test"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer frontendConn.CloseNow()
	if err := wsjson.Write(ctx, frontendConn, StreamRequest{Type: "screen", Action: "start", RoomID: "room-1"}); err != nil {
		t.Fatal(err)
	}
	var command StreamCommand
	if err := wsjson.Read(ctx, agentConn, &command); err != nil {
		t.Fatal(err)
	}
	if command.Type != "screen" || command.Action != "start" {
		t.Fatalf("unexpected screen command: %+v", command)
	}
	var acknowledgement map[string]any
	if err := wsjson.Read(ctx, frontendConn, &acknowledgement); err != nil {
		t.Fatal(err)
	}

	jpeg := []byte{0xff, 0xd8, 0xff, 0xd9}
	if err := agentConn.Write(ctx, websocket.MessageBinary, jpeg); err != nil {
		t.Fatal(err)
	}
	var header map[string]any
	if err := wsjson.Read(ctx, frontendConn, &header); err != nil {
		t.Fatal(err)
	}
	if header["type"] != "screen" || header["agent_id"] != authTestID || header["room_id"] != "room-1" {
		t.Fatalf("unexpected screen header: %+v", header)
	}
	messageType, frame, err := frontendConn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if messageType != websocket.MessageBinary || string(frame) != string(jpeg) {
		t.Fatalf("unexpected screen frame: type=%v data=%v", messageType, frame)
	}
}

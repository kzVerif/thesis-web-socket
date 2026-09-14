package wsserver

import (
	"context"
	"time"

	"github.com/coder/websocket"
)

const (
	heartbeatInterval = 30 * time.Second
	heartbeatTimeout  = 10 * time.Second
)

func monitorConnection(conn *websocket.Conn, done <-chan struct{}) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), heartbeatTimeout)
			err := conn.Ping(ctx)
			cancel()
			if err != nil {
				conn.CloseNow()
				return
			}
		}
	}
}

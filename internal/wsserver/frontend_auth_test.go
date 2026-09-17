package wsserver

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type validFrontendSession struct{}

func (validFrontendSession) Authenticate(_ context.Context, hash string) (string, bool, error) {
	if hash != hashToken("test-session") {
		return "", false, fmt.Errorf("invalid session")
	}
	return "user", false, nil
}

type expiredFrontendSession struct{ calls int }

func (s *expiredFrontendSession) Authenticate(context.Context, string) (string, bool, error) {
	s.calls++
	if s.calls > 1 {
		return "", false, fmt.Errorf("expired or revoked")
	}
	return "user", true, nil
}

func TestFrontendRequiresSessionCookie(t *testing.T) {
	for _, tc := range []struct {
		name, cookie, bearer string
		configured           bool
	}{
		{"missing", "", "", true},
		{"bearer only", "", "Bearer test-session", true},
		{"empty", "__Host-session=", "", true},
		{"invalid", "__Host-session=invalid", "Bearer test-session", true},
		{"unconfigured", "__Host-session=test-session", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(nil, log.New(io.Discard, "", 0), nil)
			if tc.configured {
				s.sessions = validFrontendSession{}
			}
			r := httptest.NewRequest(http.MethodGet, "/ws/frontend", nil)
			r.Header.Set("Cookie", tc.cookie)
			r.Header.Set("Authorization", tc.bearer)
			w := httptest.NewRecorder()
			s.HandleFrontendWebSocket(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d", w.Code)
			}
		})
	}
}

func TestFrontendRevalidatesSessionBeforeEveryCommand(t *testing.T) {
	for _, kind := range []string{"screen", "performance", "process", "virus_scan", "virus_scan_list", "FILE_DISTRIBUTE", "power"} {
		t.Run(kind, func(t *testing.T) {
			s := New(nil, log.New(io.Discard, "", 0), nil)
			s.sessions = &expiredFrontendSession{}
			h := httptest.NewServer(http.HandlerFunc(s.HandleFrontendWebSocket))
			defer h.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(h.URL, "http"), &websocket.DialOptions{HTTPHeader: http.Header{"Cookie": {"__Host-session=test-session"}}})
			if err != nil {
				t.Fatal(err)
			}
			defer c.CloseNow()
			if err := wsjson.Write(ctx, c, map[string]string{"type": kind, "action": "start"}); err != nil {
				t.Fatal(err)
			}
			var result map[string]any
			if err := wsjson.Read(ctx, c, &result); err != nil {
				t.Fatal(err)
			}
			if result["error"] != "ขาดการ login" {
				t.Fatalf("unexpected response: %v", result)
			}
		})
	}
}

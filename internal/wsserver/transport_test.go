package wsserver

import (
	"net/http/httptest"
	"testing"
)

func TestProductionFrontendOrigin(t *testing.T) {
	server := New(nil, nil, []string{"lab.example.com"})
	server.ConfigureProductionTransport(true)
	if !server.trustedHTTPSOrigin("https://lab.example.com") {
		t.Fatal("trusted HTTPS origin rejected")
	}
	for _, origin := range []string{"", "http://lab.example.com", "https://evil.example.com", "https://lab.example.com/path", "https://secret@lab.example.com", "null"} {
		request := httptest.NewRequest("GET", "http://lab.example.com/ws/frontend", nil)
		request.Header.Set("Origin", origin)
		request.Header.Set("X-Forwarded-Host", "lab.example.com")
		request.Header.Set("X-Forwarded-Proto", "https")
		response := httptest.NewRecorder()
		server.HandleFrontendWebSocket(response, request)
		if response.Code != 403 {
			t.Fatalf("untrusted origin reached authentication: %d", response.Code)
		}
	}
	// A trusted origin still needs the existing cookie/session authentication.
	request := httptest.NewRequest("GET", "http://localhost/ws/frontend", nil)
	request.Header.Set("Origin", "https://lab.example.com")
	response := httptest.NewRecorder()
	server.HandleFrontendWebSocket(response, request)
	if response.Code != 401 {
		t.Fatal("existing session authentication bypassed")
	}
}

package wsserver

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"ws-rat/internal/agentauth"
	"ws-rat/internal/model"
)

const authTestID = "11111111-1111-4111-8111-111111111111"

func authTestKey() ed25519.PrivateKey { return ed25519.NewKeyFromSeed(make([]byte, 32)) }

// Existing feature integration fixtures now use the real authentication gate.
func (store *fakeAgentStore) GetPublicKey(_ context.Context, id string) (ed25519.PublicKey, error) {
	if id != store.agent.ID {
		return nil, sql.ErrNoRows
	}
	return authTestKey().Public().(ed25519.PublicKey), nil
}
func (store *powerTestStore) GetPublicKey(_ context.Context, id string) (ed25519.PublicKey, error) {
	if _, ok := store.agents[id]; !ok {
		return nil, sql.ErrNoRows
	}
	return authTestKey().Public().(ed25519.PublicKey), nil
}

func testAuthProof(t *testing.T, ctx context.Context, conn *websocket.Conn, id string, key ed25519.PrivateKey) (agentauth.Hello, agentauth.Challenge, agentauth.Proof) {
	t.Helper()
	nonce, err := agentauth.Random()
	if err != nil {
		t.Fatal(err)
	}
	hello := agentauth.Hello{Type: "auth_hello", Version: agentauth.Version, AgentID: id, ClientNonce: nonce}
	if err := wsjson.Write(ctx, conn, hello); err != nil {
		t.Fatal(err)
	}
	var challenge agentauth.Challenge
	if err := wsjson.Read(ctx, conn, &challenge); err != nil {
		t.Fatal(err)
	}
	transcript, err := agentauth.Transcript(id, challenge.ChallengeID, nonce, challenge.ServerNonce)
	if err != nil {
		t.Fatal(err)
	}
	proof := agentauth.Proof{Type: "auth_proof", Version: agentauth.Version, ChallengeID: challenge.ChallengeID, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, transcript))}
	return hello, challenge, proof
}
func authenticateTestAgent(t *testing.T, ctx context.Context, conn *websocket.Conn, id string) {
	t.Helper()
	_, _, proof := testAuthProof(t, ctx, conn, id, authTestKey())
	if err := wsjson.Write(ctx, conn, proof); err != nil {
		t.Fatal(err)
	}
	var ok agentauth.OK
	if err := wsjson.Read(ctx, conn, &ok); err != nil || ok.Type != "auth_ok" || ok.Version != agentauth.Version {
		t.Fatalf("auth_ok missing: %v", err)
	}
}

type credentialTestStore struct {
	mu             sync.Mutex
	key            string
	unknown        bool
	statuses       []string
	offlineEntered chan struct{}
	offlineRelease chan struct{}
}

func (s *credentialTestStore) GetPublicKey(context.Context, string) (ed25519.PublicKey, error) {
	if s.unknown {
		return nil, sql.ErrNoRows
	}
	return agentauth.PublicKey(s.key)
}
func (s *credentialTestStore) GetByID(context.Context, string) (*model.AgentInfo, error) {
	return &model.AgentInfo{ID: authTestID}, nil
}
func (s *credentialTestStore) UpdateStatus(_ context.Context, _ string, status string) error {
	if status == model.StatusOffline && s.offlineEntered != nil {
		select {
		case s.offlineEntered <- struct{}{}:
		default:
		}
		<-s.offlineRelease
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statuses = append(s.statuses, status)
	return nil
}
func (s *credentialTestStore) state() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.statuses...)
}
func authHarness(t *testing.T) (*Server, *credentialTestStore, string, context.Context) {
	t.Helper()
	store := &credentialTestStore{key: base64.StdEncoding.EncodeToString(authTestKey().Public().(ed25519.PublicKey))}
	server := New(store, log.New(io.Discard, "", 0), nil)
	server.authTimeout = 700 * time.Millisecond
	httpServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	t.Cleanup(httpServer.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return server, store, "ws" + strings.TrimPrefix(httpServer.URL, "http"), ctx
}
func authDial(t *testing.T, ctx context.Context, url string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	return conn
}
func waitOnline(t *testing.T, server *Server) *Client {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c, ok := server.registry.Get(authTestID); ok {
			return c
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("authenticated Agent never entered registry")
	return nil
}
func rejected(t *testing.T, ctx context.Context, conn *websocket.Conn, server *Server, store *credentialTestStore) {
	t.Helper()
	if _, _, err := conn.Read(ctx); err == nil {
		t.Fatal("unauthenticated message accepted")
	}
	if len(server.registry.Snapshot()) != 0 || len(store.state()) != 0 {
		t.Fatal("unauthenticated connection changed trusted state")
	}
}

func TestAgentAuthenticationRejectsInvalidProof(t *testing.T) {
	for _, scenario := range []string{"wrong key", "short signature", "invalid signature Base64", "tampered signature", "client nonce", "server nonce", "wrong challenge", "wrong version", "wrong sequence", "normal message", "oversized", "binary"} {
		t.Run(scenario, func(t *testing.T) {
			server, store, url, ctx := authHarness(t)
			conn := authDial(t, ctx, url)
			hello, challenge, proof := testAuthProof(t, ctx, conn, authTestID, authTestKey())
			switch scenario {
			case "wrong key":
				seed := make([]byte, 32)
				seed[0] = 1
				transcript, _ := agentauth.Transcript(authTestID, challenge.ChallengeID, hello.ClientNonce, challenge.ServerNonce)
				proof.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.NewKeyFromSeed(seed), transcript))
			case "short signature":
				proof.Signature = base64.StdEncoding.EncodeToString(make([]byte, 32))
			case "invalid signature Base64":
				proof.Signature = strings.Repeat("!", 88)
			case "tampered signature":
				signature, _ := base64.StdEncoding.DecodeString(proof.Signature)
				signature[0] ^= 1
				proof.Signature = base64.StdEncoding.EncodeToString(signature)
			case "client nonce", "server nonce":
				nonce, _ := agentauth.Random()
				if scenario == "client nonce" {
					hello.ClientNonce = nonce
				} else {
					challenge.ServerNonce = nonce
				}
				transcript, _ := agentauth.Transcript(authTestID, challenge.ChallengeID, hello.ClientNonce, challenge.ServerNonce)
				proof.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(authTestKey(), transcript))
			case "wrong challenge":
				proof.ChallengeID, _ = agentauth.Random()
			case "wrong version":
				proof.Version = "V0"
			case "wrong sequence":
				proof.Type = "auth_hello"
			case "normal message":
				proof.Type = "power"
			}
			if scenario == "oversized" {
				_ = conn.Write(ctx, websocket.MessageText, []byte(strings.Repeat("x", agentauth.MessageLimit+1)))
			} else if scenario == "binary" {
				_ = conn.Write(ctx, websocket.MessageBinary, []byte("{}"))
			} else {
				_ = wsjson.Write(ctx, conn, proof)
			}
			rejected(t, ctx, conn, server, store)
		})
	}
}

func TestAgentAuthenticationHelloAndCredentialFailures(t *testing.T) {
	for _, scenario := range []string{"unknown", "NULL", "malformed key", "private key", "version", "uuid", "nonce", "proof first", "legacy info", "oversized hello"} {
		t.Run(scenario, func(t *testing.T) {
			server, store, url, ctx := authHarness(t)
			switch scenario {
			case "unknown":
				store.unknown = true
			case "NULL":
				store.key = ""
			case "malformed key":
				store.key = "not base64"
			case "private key":
				store.key = base64.StdEncoding.EncodeToString(make([]byte, 64))
			}
			conn := authDial(t, ctx, url)
			nonce, _ := agentauth.Random()
			hello := agentauth.Hello{Type: "auth_hello", Version: agentauth.Version, AgentID: authTestID, ClientNonce: nonce}
			switch scenario {
			case "version":
				hello.Version = "V0"
			case "uuid":
				hello.AgentID = "not-uuid"
			case "nonce":
				hello.ClientNonce = "bad"
			case "proof first":
				hello.Type = "auth_proof"
			case "legacy info":
				hello.Type = ""
			}
			if scenario == "oversized hello" {
				hello.ClientNonce = strings.Repeat("x", 5000)
			}
			_ = wsjson.Write(ctx, conn, hello)
			rejected(t, ctx, conn, server, store)
		})
	}
}

func TestAgentAuthenticationTimeoutAndReplay(t *testing.T) {
	t.Run("hello timeout", func(t *testing.T) {
		server, store, url, ctx := authHarness(t)
		conn := authDial(t, ctx, url)
		rejected(t, ctx, conn, server, store)
	})
	t.Run("expired proof", func(t *testing.T) {
		server, store, url, ctx := authHarness(t)
		conn := authDial(t, ctx, url)
		_, _, proof := testAuthProof(t, ctx, conn, authTestID, authTestKey())
		time.Sleep(800 * time.Millisecond)
		_ = wsjson.Write(ctx, conn, proof)
		rejected(t, ctx, conn, server, store)
	})
	for _, scenario := range []string{"old proof", "old successful proof", "cross connection", "same proof new challenge ID", "multiple proofs"} {
		t.Run(scenario, func(t *testing.T) {
			server, store, url, ctx := authHarness(t)
			first := authDial(t, ctx, url)
			_, originalChallenge, proof := testAuthProof(t, ctx, first, authTestID, authTestKey())
			if scenario == "old successful proof" {
				_ = wsjson.Write(ctx, first, proof)
				var ok agentauth.OK
				if err := wsjson.Read(ctx, first, &ok); err != nil || ok.Type != "auth_ok" {
					t.Fatalf("original proof failed: %v", err)
				}
				first.CloseNow()
			}
			if scenario == "multiple proofs" {
				_ = wsjson.Write(ctx, first, proof)
				var ok agentauth.OK
				if err := wsjson.Read(ctx, first, &ok); err != nil {
					t.Fatal(err)
				}
				_ = wsjson.Write(ctx, first, proof)
				rejected(t, ctx, first, server, store)
				return
			}
			if scenario == "old proof" {
				first.CloseNow()
			}
			second := authDial(t, ctx, url)
			_, challenge, _ := testAuthProof(t, ctx, second, authTestID, authTestKey())
			if challenge.ChallengeID == originalChallenge.ChallengeID || challenge.ServerNonce == originalChallenge.ServerNonce {
				t.Fatal("server reused challenge randomness")
			}
			if scenario == "same proof new challenge ID" {
				proof.ChallengeID = challenge.ChallengeID
			}
			_ = wsjson.Write(ctx, second, proof)
			rejected(t, ctx, second, server, store)
		})
	}
}

func TestAuthenticatedSessionOutlivesAuthDeadline(t *testing.T) {
	server, _, url, ctx := authHarness(t)
	server.authTimeout = 250 * time.Millisecond
	conn := authDial(t, ctx, url)
	authenticateTestAgent(t, ctx, conn, authTestID)
	_ = wsjson.Write(ctx, conn, model.AgentInfo{ID: authTestID})
	waitOnline(t, server)
	readCtx := conn.CloseRead(ctx)
	time.Sleep(350 * time.Millisecond)
	if err := conn.Ping(readCtx); err != nil {
		t.Fatalf("authentication deadline leaked into feature session: %v", err)
	}
	if _, ok := server.registry.Get(authTestID); !ok {
		t.Fatal("authenticated session expired with auth deadline")
	}
}

func TestAuthenticatedDuplicateAndImpostor(t *testing.T) {
	server, store, url, ctx := authHarness(t)
	first := authDial(t, ctx, url)
	authenticateTestAgent(t, ctx, first, authTestID)
	if len(store.state()) != 0 || len(server.registry.Snapshot()) != 0 {
		t.Fatal("ONLINE before initial metadata")
	}
	_ = wsjson.Write(ctx, first, model.AgentInfo{ID: authTestID})
	original := waitOnline(t, server)
	impostor := authDial(t, ctx, url)
	_, _, proof := testAuthProof(t, ctx, impostor, authTestID, authTestKey())
	proof.Signature = base64.StdEncoding.EncodeToString(make([]byte, 64))
	_ = wsjson.Write(ctx, impostor, proof)
	if _, _, err := impostor.Read(ctx); err == nil {
		t.Fatal("impostor accepted")
	}
	current, _ := server.registry.Get(authTestID)
	if current != original || len(store.state()) != 1 {
		t.Fatal("impostor replaced or marked legitimate Agent offline")
	}
	second := authDial(t, ctx, url)
	authenticateTestAgent(t, ctx, second, authTestID)
	_ = wsjson.Write(ctx, second, model.AgentInfo{ID: authTestID})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		current, _ = server.registry.Get(authTestID)
		if current != original {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if current == nil || current == original {
		t.Fatal("authenticated duplicate not installed")
	}
	server.disconnect(original)
	after, _ := server.registry.Get(authTestID)
	if after != current {
		t.Fatal("old cleanup removed newer connection")
	}
	for _, status := range store.state() {
		if status != model.StatusOnline {
			t.Fatal("old cleanup marked replacement OFFLINE")
		}
	}
}

func TestAuthenticatedDisconnectOnlineOrdering(t *testing.T) {
	server, store, url, ctx := authHarness(t)
	store.offlineEntered = make(chan struct{}, 1)
	store.offlineRelease = make(chan struct{})
	first := authDial(t, ctx, url)
	authenticateTestAgent(t, ctx, first, authTestID)
	_ = wsjson.Write(ctx, first, model.AgentInfo{ID: authTestID})
	waitOnline(t, server)
	first.CloseNow()
	select {
	case <-store.offlineEntered:
	case <-ctx.Done():
		t.Fatal("disconnect not started")
	}
	second := authDial(t, ctx, url)
	authenticateTestAgent(t, ctx, second, authTestID)
	_ = wsjson.Write(ctx, second, model.AgentInfo{ID: authTestID})
	close(store.offlineRelease)
	waitOnline(t, server)
	statuses := store.state()
	if statuses[len(statuses)-1] != model.StatusOnline {
		t.Fatal("late OFFLINE overwrote authenticated reconnect")
	}
}

package wsserver

import (
	"context"
	"crypto/ed25519"
	"errors"
	"hash/fnv"
	"sync"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"ws-rat/internal/agentauth"
)

type AgentCredentialStore interface {
	GetPublicKey(context.Context, string) (ed25519.PublicKey, error)
}

// One challenge and one proof attempt, scoped to this connection/deadline.
// This routine performs no status, registry or subscription mutations.
func (server *Server) authenticateAgent(ctx context.Context, conn *websocket.Conn) (string, error) {
	conn.SetReadLimit(agentauth.MessageLimit)
	var hello agentauth.Hello
	if err := agentauth.Read(ctx, conn, &hello); err != nil {
		return "", err
	}
	id, err := agentauth.CanonicalID(hello.AgentID)
	if err != nil || id != hello.AgentID || hello.Type != "auth_hello" || hello.Version != agentauth.Version {
		return "", agentauth.ErrProtocol
	}
	if _, err := agentauth.Decode(hello.ClientNonce, agentauth.NonceSize); err != nil {
		return "", err
	}
	credentials, ok := server.agents.(AgentCredentialStore)
	if !ok {
		return "", agentauth.ErrCredential
	}
	dbCtx, cancel := context.WithTimeout(ctx, databaseTimeout)
	publicKey, err := credentials.GetPublicKey(dbCtx, id)
	cancel()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "", err
	}
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return "", agentauth.ErrCredential
	}
	challengeID, err := agentauth.Random()
	if err != nil {
		return "", err
	}
	nonce, err := agentauth.Random()
	if err != nil {
		return "", err
	}
	challenge := agentauth.Challenge{Type: "auth_challenge", Version: agentauth.Version, ChallengeID: challengeID, ServerNonce: nonce}
	if err := wsjson.Write(ctx, conn, challenge); err != nil {
		return "", err
	}
	var proof agentauth.Proof
	if err := agentauth.Read(ctx, conn, &proof); err != nil {
		return "", err
	}
	if proof.Type != "auth_proof" || proof.Version != agentauth.Version || proof.ChallengeID != challengeID {
		return "", agentauth.ErrProtocol
	}
	signature, err := agentauth.Decode(proof.Signature, ed25519.SignatureSize)
	if err != nil {
		return "", err
	}
	transcript, err := agentauth.Transcript(id, challengeID, hello.ClientNonce, nonce)
	if err != nil {
		return "", err
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if !ed25519.Verify(publicKey, transcript, signature) {
		return "", agentauth.ErrProtocol
	}
	if err := wsjson.Write(ctx, conn, agentauth.OK{Type: "auth_ok", Version: agentauth.Version}); err != nil {
		return "", err
	}
	return id, nil
}

func authFailureCategory(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, agentauth.ErrCredential):
		return "unknown agent or unsupported credential"
	default:
		return "invalid proof, protocol or closed connection"
	}
}

// Bounded lock stripes serialize each Agent's registry and DB status lifecycle.
// There is no global challenge map or unbounded per-ID lock allocation.
func (server *Server) lifecycleLock(id string) *sync.Mutex {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(id))
	return &server.lifecycle[hash.Sum32()%uint32(len(server.lifecycle))]
}

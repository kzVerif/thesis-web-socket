// Package agentauth defines THESIS-RAT-AGENT-AUTH-V1. Keep the counterpart in
// the Agent/WS repository synchronized using the deterministic transcript vector.
package agentauth

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/coder/websocket"
)

const Version = "THESIS-RAT-AGENT-AUTH-V1"
const NonceSize = 32
const MessageLimit = 4096
const Timeout = 12 * time.Second

var ErrProtocol = errors.New("agent authentication protocol rejected")
var ErrCredential = errors.New("agent credential missing or unsupported")

type Hello struct {
	Type        string `json:"type"`
	Version     string `json:"version"`
	AgentID     string `json:"agent_id"`
	ClientNonce string `json:"client_nonce"`
}
type Challenge struct {
	Type        string `json:"type"`
	Version     string `json:"version"`
	ChallengeID string `json:"challenge_id"`
	ServerNonce string `json:"server_nonce"`
}
type Proof struct {
	Type        string `json:"type"`
	Version     string `json:"version"`
	ChallengeID string `json:"challenge_id"`
	Signature   string `json:"signature"`
}
type OK struct {
	Type    string `json:"type"`
	Version string `json:"version"`
}

func CanonicalID(id string) (string, error) {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return "", ErrProtocol
	}
	raw := strings.ReplaceAll(id, "-", "")
	if len(raw) != 32 {
		return "", ErrProtocol
	}
	if _, err := hex.DecodeString(raw); err != nil {
		return "", ErrProtocol
	}
	return strings.ToLower(id), nil
}

func Decode(value string, size int) ([]byte, error) {
	if len(value) != base64.StdEncoding.EncodedLen(size) {
		return nil, ErrProtocol
	}
	data, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil || len(data) != size || base64.StdEncoding.EncodeToString(data) != value {
		return nil, ErrProtocol
	}
	return data, nil
}

func PublicKey(value string) (ed25519.PublicKey, error) {
	if len(value) > 256 {
		return nil, ErrCredential
	}
	key, err := Decode(strings.TrimSpace(value), ed25519.PublicKeySize)
	if err != nil {
		return nil, ErrCredential
	}
	return ed25519.PublicKey(key), nil
}

func Random() (string, error) {
	b := make([]byte, NonceSize)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// Transcript is five uint32 big-endian length-prefixed fields:
// version UTF-8, canonical lowercase UUID ASCII, raw challenge ID,
// raw client nonce, raw server nonce. Sign these bytes directly with Ed25519.
func Transcript(id, challenge, client, server string) ([]byte, error) {
	canonical, err := CanonicalID(id)
	if err != nil || canonical != id {
		return nil, ErrProtocol
	}
	c, err := Decode(challenge, NonceSize)
	if err != nil {
		return nil, err
	}
	cn, err := Decode(client, NonceSize)
	if err != nil {
		return nil, err
	}
	sn, err := Decode(server, NonceSize)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	for _, field := range [][]byte{[]byte(Version), []byte(id), c, cn, sn} {
		_ = binary.Write(&out, binary.BigEndian, uint32(len(field)))
		_, _ = out.Write(field)
	}
	return out.Bytes(), nil
}

func Read(ctx context.Context, conn *websocket.Conn, value any) error {
	kind, data, err := conn.Read(ctx)
	if err != nil {
		return err
	}
	if kind != websocket.MessageText || len(data) > MessageLimit {
		return ErrProtocol
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return ErrProtocol
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ErrProtocol
	}
	return nil
}

package agentauth

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestTranscriptVector(t *testing.T) {
	challenge, client, server := make([]byte, 32), make([]byte, 32), make([]byte, 32)
	for i := range challenge {
		challenge[i] = byte(i)
		client[i] = byte(i + 32)
		server[i] = byte(i + 64)
	}
	actual, err := Transcript("11111111-1111-4111-8111-111111111111", base64.StdEncoding.EncodeToString(challenge), base64.StdEncoding.EncodeToString(client), base64.StdEncoding.EncodeToString(server))
	if err != nil {
		t.Fatal(err)
	}
	const expected = "000000185448455349532d5241542d4147454e542d415554482d56310000002431313131313131312d313131312d343131312d383131312d31313131313131313131313100000020000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f00000020202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f00000020404142434445464748494a4b4c4d4e4f505152535455565758595a5b5c5d5e5f"
	if hex.EncodeToString(actual) != expected {
		t.Fatal("cross-repository transcript drift")
	}
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	defer clear(key)
	signature := ed25519.Sign(key, actual)
	if !ed25519.Verify(key.Public().(ed25519.PublicKey), actual, signature) {
		t.Fatal("signature mismatch")
	}
	altered := bytes.Clone(actual)
	altered[len(altered)-1] ^= 1
	if ed25519.Verify(key.Public().(ed25519.PublicKey), altered, signature) {
		t.Fatal("tampered transcript accepted")
	}
}
func TestPublicKeyAndNonceValidation(t *testing.T) {
	good := base64.StdEncoding.EncodeToString(make([]byte, 32))
	for _, bad := range []string{"", "bad", base64.StdEncoding.EncodeToString(make([]byte, 64)), good + "\n", good[:len(good)-1]} {
		if _, err := Decode(bad, 32); err == nil {
			t.Fatal("malformed auth field accepted")
		}
	}
	for _, bad := range []string{"", "invalid", base64.StdEncoding.EncodeToString(make([]byte, 64))} {
		if _, err := PublicKey(bad); err == nil {
			t.Fatal("invalid stored credential accepted")
		}
	}
	if _, err := PublicKey(good); err != nil {
		t.Fatal(err)
	}
	first, _ := Random()
	second, _ := Random()
	if first == second {
		t.Fatal("reused nonce")
	}
}

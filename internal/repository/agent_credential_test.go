package repository

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"errors"
	"io"
	"testing"
)

// Exercise database/sql NULL scanning and the real credential repository without
// credentials or mutations to a live database.
type credentialConnector struct {
	value   driver.Value
	missing bool
	failure error
}

func (c credentialConnector) Connect(context.Context) (driver.Conn, error) {
	return credentialConn{c}, nil
}
func (c credentialConnector) Driver() driver.Driver { return credentialDriver{} }

type credentialDriver struct{}

func (credentialDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("use test connector")
}

type credentialConn struct{ credentialConnector }

func (credentialConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not supported") }
func (credentialConn) Close() error                        { return nil }
func (credentialConn) Begin() (driver.Tx, error)           { return nil, errors.New("not supported") }
func (c credentialConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if query != "SELECT public_key FROM agents WHERE id = $1" || len(args) != 1 {
		return nil, errors.New("unexpected credential query")
	}
	if c.failure != nil {
		return nil, c.failure
	}
	return &credentialRows{value: c.value, done: c.missing}, nil
}

type credentialRows struct {
	value driver.Value
	done  bool
}

func (*credentialRows) Columns() []string { return []string{"public_key"} }
func (*credentialRows) Close() error      { return nil }
func (r *credentialRows) Next(values []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	values[0] = r.value
	return nil
}

func TestGetPublicKeyCredentialValidation(t *testing.T) {
	good := base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize))
	for _, tc := range []struct {
		name      string
		connector credentialConnector
		valid     bool
	}{
		{"valid", credentialConnector{value: good}, true},
		{"NULL", credentialConnector{}, false},
		{"empty", credentialConnector{value: ""}, false},
		{"invalid Base64", credentialConnector{value: "garbage"}, false},
		{"private key length", credentialConnector{value: base64.StdEncoding.EncodeToString(make([]byte, 64))}, false},
		{"unknown ID", credentialConnector{missing: true}, false},
		{"database unavailable", credentialConnector{failure: errors.New("test-only failure")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := sql.OpenDB(tc.connector)
			defer db.Close()
			key, err := NewAgentRepository(db).GetPublicKey(context.Background(), "11111111-1111-4111-8111-111111111111")
			if (err == nil) != tc.valid || (tc.valid && len(key) != ed25519.PublicKeySize) {
				t.Fatalf("unexpected credential result: %v", err)
			}
		})
	}
}

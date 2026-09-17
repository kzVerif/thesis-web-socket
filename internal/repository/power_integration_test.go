package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"ws-rat/internal/distribution"
	"ws-rat/internal/repository"
)

// These tables exist only on this connection and shadow application tables.
// No schema migration or persistent application writes are performed.
func TestPowerPostgresPermissionsAndRoomMembership(t *testing.T) {
	dsn := os.Getenv("POWER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set POWER_TEST_DATABASE_URL to run PostgreSQL power tests")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	exec := func(query string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	exec(`
CREATE TEMP TABLE rooms(id uuid PRIMARY KEY);
CREATE TEMP TABLE agents(id uuid PRIMARY KEY, room_id uuid, status text);
CREATE TEMP TABLE users(id uuid PRIMARY KEY, role_id int, status text);
CREATE TEMP TABLE user_sessions(user_id uuid, token_hash text, revoked_at timestamptz, expires_at timestamptz);
CREATE TEMP TABLE permissions(id int PRIMARY KEY, code text);
CREATE TEMP TABLE role_permissions(role_id int, permission_id int);
INSERT INTO rooms VALUES ('11111111-1111-4111-8111-111111111111'),('22222222-2222-4222-8222-222222222222');
INSERT INTO agents VALUES ('33333333-3333-4333-8333-333333333333','11111111-1111-4111-8111-111111111111','OFFLINE'),('44444444-4444-4444-8444-444444444444','11111111-1111-4111-8111-111111111111','DISABLED');
INSERT INTO users VALUES ('55555555-5555-4555-8555-555555555555',1,'ACTIVE');
INSERT INTO user_sessions VALUES ('55555555-5555-4555-8555-555555555555','test-hash',NULL,NOW()+INTERVAL '1 hour');
INSERT INTO permissions VALUES (1,'files.distribute'),(2,'agents.control');
INSERT INTO role_permissions VALUES (1,1);`)
	agents := repository.NewAgentRepository(db)
	ids, err := agents.ListAgentIDsByRoom(ctx, "11111111-1111-4111-8111-111111111111")
	if err != nil || len(ids) != 2 {
		t.Fatalf("all room members: %v %v", ids, err)
	}
	ids, err = agents.ListAgentIDsByRoom(ctx, "22222222-2222-4222-8222-222222222222")
	if err != nil || len(ids) != 0 {
		t.Fatalf("empty room: %v %v", ids, err)
	}
	_, err = agents.ListAgentIDsByRoom(ctx, "66666666-6666-4666-8666-666666666666")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing room: %v", err)
	}
	sessions := &distribution.Repository{DB: db}
	check := func(files, control bool) {
		t.Helper()
		user, allowed, err := sessions.Authenticate(ctx, "test-hash")
		if err != nil || user == "" || allowed != files {
			t.Fatalf("files permission: %q %v %v", user, allowed, err)
		}
		user, allowed, err = sessions.AuthenticatePermission(ctx, "test-hash", "agents.control")
		if err != nil || user == "" || allowed != control {
			t.Fatalf("power permission: %q %v %v", user, allowed, err)
		}
	}
	check(true, false)
	exec(`INSERT INTO role_permissions VALUES (1,2)`)
	check(true, true)
	exec(`DELETE FROM role_permissions WHERE permission_id=1`)
	check(false, true)
	exec(`DELETE FROM role_permissions WHERE permission_id=2`)
	check(false, false)
	for _, state := range []string{"revoked", "expired", "inactive"} {
		t.Run(state, func(t *testing.T) {
			exec(`UPDATE users SET status='ACTIVE'; UPDATE user_sessions SET revoked_at=NULL,expires_at=NOW()+INTERVAL '1 hour'`)
			switch state {
			case "revoked":
				exec(`UPDATE user_sessions SET revoked_at=NOW()`)
			case "expired":
				exec(`UPDATE user_sessions SET expires_at=NOW()-INTERVAL '1 second'`)
			case "inactive":
				exec(`UPDATE users SET status='DISABLED'`)
			}
			for _, permission := range []string{"files.distribute", "agents.control"} {
				_, _, err := sessions.AuthenticatePermission(ctx, "test-hash", permission)
				if !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("invalid session accepted for %s: %v", permission, err)
				}
			}
		})
	}
}

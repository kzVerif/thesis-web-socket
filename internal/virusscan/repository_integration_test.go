package virusscan

import (
	"context"
	"database/sql"
	_ "github.com/lib/pq"
	"os"
	"strings"
	"testing"
	"time"
)

// Run against a disposable PostgreSQL database with AV_TEST_DATABASE_URL.
// Each run creates and drops its own schema; it never uses application tables.
func TestRepositoryMigrationAndMultiAgentJob(t *testing.T) {
	dsn := os.Getenv("AV_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set AV_TEST_DATABASE_URL for PostgreSQL integration tests")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	schema := "av_test_" + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE SCHEMA ` + schema)
	defer func() {
		if _, err := db.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); err != nil {
			t.Error(err)
		}
	}()
	exec(`SET search_path TO ` + schema + `, public`)
	fresh, err := os.ReadFile("../../schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	exec(string(fresh))
	// Restore the pre-migration table shape, then insert an existing scan.
	exec(`ALTER TABLE av_scan_results DROP COLUMN job_id; ALTER TABLE av_scan_results DROP CONSTRAINT uq_av_scan_command; DROP TABLE av_jobs`)
	var role, user, agent1, agent2, legacy string
	row := func(query string, out *string, args ...any) {
		t.Helper()
		if err := db.QueryRowContext(ctx, query, args...).Scan(out); err != nil {
			t.Fatal(err)
		}
	}
	row(`INSERT INTO roles(name) VALUES('test') RETURNING id::text`, &role)
	row(`INSERT INTO users(username,password_hash,role_id) VALUES('test','unused',$1) RETURNING id::text`, &user, role)
	row(`INSERT INTO agents(hostname) VALUES('first') RETURNING id::text`, &agent1)
	row(`INSERT INTO agents(hostname) VALUES('second') RETURNING id::text`, &agent2)
	row(`INSERT INTO commands(agent_id,issued_by,command_type,payload,status) VALUES($1,$2,'virus_scan','{"scan_type":"quick"}','SUCCEEDED') RETURNING id::text`, &legacy, agent1, user)
	exec(`INSERT INTO av_scan_results(agent_id,command_id,scan_type,status,threat_details) VALUES($1,$2,'quick','COMPLETED','{"status":"completed"}')`, agent1, legacy)
	migration, err := os.ReadFile("../../migrations/20260905_add_av_jobs.sql")
	if err != nil {
		t.Fatal(err)
	}
	exec(string(migration))
	repo := &Repository{DB: db}
	old, err := repo.List(ctx, user, Request{JobID: legacy})
	if err != nil || len(old) != 1 || old[0].JobID != legacy || old[0].Status != "SUCCEEDED" {
		t.Fatalf("backfill: %#v %v", old, err)
	}
	job, err := repo.Create(ctx, user, Request{AgentIDs: []string{agent1, agent2}, ScanType: "quick"})
	if err != nil {
		t.Fatal(err)
	}
	if len(job.Targets) != 2 || job.ID == job.Targets[0].RequestID || job.Targets[0].RequestID == job.Targets[1].RequestID {
		t.Fatalf("invalid job: %+v", job)
	}
	now := time.Now().UTC()
	first := job.Targets[0]
	second := job.Targets[1]
	completed := Event{Type: "virus_scan_result", RequestID: first.RequestID, ScanType: "quick", Status: "completed", StartedAt: &now, FinishedAt: &now, Report: &Report{ExitCode: 0, Output: "report preserved"}}
	if err := repo.Apply(ctx, second.AgentID, completed); err == nil {
		t.Fatal("accepted result from another agent")
	}
	mismatch := completed
	mismatch.ScanType = "full"
	if err := repo.Apply(ctx, first.AgentID, mismatch); err == nil {
		t.Fatal("accepted mismatched scan type")
	}
	if err := repo.Apply(ctx, first.AgentID, completed); err != nil {
		t.Fatal(err)
	}
	if err := repo.Apply(ctx, first.AgentID, completed); err != nil {
		t.Fatal(err)
	}
	running := completed
	running.Type = "virus_scan_status"
	running.Status = "running"
	running.FinishedAt = nil
	running.Report = nil
	if err := repo.Apply(ctx, first.AgentID, running); err != nil {
		t.Fatal(err)
	}
	if err := repo.Dispatched(ctx, first.RequestID, nil); err != nil {
		t.Fatal(err)
	}
	rejected := Event{Type: "virus_scan_result", RequestID: second.RequestID, ScanType: "quick", Status: "rejected", FinishedAt: &now, Error: "busy"}
	if err := repo.Apply(ctx, second.AgentID, rejected); err != nil {
		t.Fatal(err)
	}
	records, err := repo.List(ctx, user, Request{JobID: job.ID})
	if err != nil || len(records) != 2 {
		t.Fatalf("list: %+v %v", records, err)
	}
	for _, record := range records {
		if record.JobID != job.ID {
			t.Fatal("lost job mapping")
		}
		if record.RequestID == first.RequestID && (record.Status != "SUCCEEDED" || record.Result.Report.Output != "report preserved") {
			t.Fatalf("terminal result overwritten: %+v", record)
		}
		if record.RequestID == second.RequestID && (record.Status != "FAILED" || record.Result.Status != "rejected") {
			t.Fatalf("rejection lost: %+v", record)
		}
	}
	hidden, err := repo.List(ctx, "00000000-0000-0000-0000-000000000000", Request{JobID: job.ID})
	if err != nil || len(hidden) != 0 {
		t.Fatalf("ownership: %+v %v", hidden, err)
	}
	var before, after int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM av_jobs`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, user, Request{AgentIDs: []string{agent1, "00000000-0000-0000-0000-000000000000"}, ScanType: "quick"}); err == nil {
		t.Fatal("accepted missing agent")
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM av_jobs`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("partial job survived rollback")
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO av_scan_results(job_id,agent_id,command_id,scan_type,status) VALUES($1,$2,$3,'quick','PENDING')`, job.ID, first.AgentID, first.RequestID); err == nil {
		t.Fatal("accepted duplicate target")
	}
}

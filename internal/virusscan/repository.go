package virusscan

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type Repository struct{ DB *sql.DB }

func (r *Repository) Allowed(ctx context.Context, user string) (bool, error) {
	var allowed bool
	err := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN role_permissions rp ON rp.role_id=u.role_id JOIN permissions p ON p.id=rp.permission_id WHERE u.id=$1 AND u.status='ACTIVE' AND p.code='av.scan')`, user).Scan(&allowed)
	return allowed, err
}

func (r *Repository) Create(ctx context.Context, user string, req Request) (Job, error) {
	if err := req.Validate(); err != nil {
		return Job{}, err
	}
	ids, _ := req.TargetIDs()
	// Acquire agent locks in a stable order for overlapping multi-agent jobs.
	sort.Strings(ids)
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	for _, id := range ids {
		var agent string
		if err = tx.QueryRowContext(ctx, `SELECT id::text FROM agents WHERE id=$1 AND status<>'DISABLED' FOR UPDATE`, id).Scan(&agent); err != nil {
			return Job{}, fmt.Errorf("agent unavailable: %w", err)
		}
	}
	job := Job{Targets: make([]Target, 0, len(ids))}
	if err = tx.QueryRowContext(ctx, `INSERT INTO av_jobs(requested_by,scan_type,path) VALUES($1,$2,$3) RETURNING id::text`, user, req.ScanType, req.Path).Scan(&job.ID); err != nil {
		return Job{}, err
	}
	payload, _ := json.Marshal(map[string]string{"scan_type": req.ScanType, "path": req.Path})
	for _, agent := range ids {
		var id string
		if err = tx.QueryRowContext(ctx, `INSERT INTO commands(agent_id,issued_by,command_type,payload) VALUES($1,$2,'virus_scan',$3) RETURNING id::text`, agent, user, string(payload)).Scan(&id); err != nil {
			return Job{}, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO av_scan_results(job_id,agent_id,command_id,scan_type,status) VALUES($1,$2,$3,$4,'PENDING')`, job.ID, agent, id, req.ScanType); err != nil {
			return Job{}, err
		}
		job.Targets = append(job.Targets, Target{AgentID: agent, RequestID: id})
	}
	detail, _ := json.Marshal(job)
	if _, err = tx.ExecContext(ctx, `INSERT INTO logs(user_id,action,detail) VALUES($1,'virus_scan_requested',$2)`, user, string(detail)); err != nil {
		return Job{}, err
	}
	if err = tx.Commit(); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (r *Repository) Dispatched(ctx context.Context, id string, sendErr error) error {
	if sendErr != nil {
		// A failed write can still have reached the agent. Keep the command open for a late result.
		_, err := r.DB.ExecContext(ctx, `UPDATE commands SET result_message='Dispatch uncertain; do not automatically retry',updated_at=NOW() WHERE id=$1 AND status='QUEUED'`, id)
		return err
	}
	_, err := r.DB.ExecContext(ctx, `UPDATE commands SET status='DELIVERED',updated_at=NOW() WHERE id=$1 AND status='QUEUED'`, id)
	return err
}

func (r *Repository) Apply(ctx context.Context, agent string, e Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status, user, scanType, jobID string
	err = tx.QueryRowContext(ctx, `SELECT c.status,j.requested_by::text,j.scan_type,j.id::text FROM commands c JOIN av_scan_results a ON a.command_id=c.id AND a.agent_id=c.agent_id JOIN av_jobs j ON j.id=a.job_id WHERE c.id=$1 AND c.agent_id=$2 AND c.command_type='virus_scan' FOR UPDATE OF c`, e.RequestID, agent).Scan(&status, &user, &scanType, &jobID)
	if err != nil {
		return err
	}
	if scanType != e.ScanType {
		return fmt.Errorf("scan_type mismatch")
	}
	if status == "SUCCEEDED" || status == "FAILED" || status == "CANCELLED" || status == "EXPIRED" {
		return nil
	}
	commandStatus := map[string]string{"running": "RUNNING", "completed": "SUCCEEDED", "failed": "FAILED", "rejected": "FAILED", "cancelled": "CANCELLED"}[e.Status]
	scanStatus := strings.ToUpper(e.Status)
	if e.Status == "rejected" {
		scanStatus = "FAILED"
	}
	raw, _ := json.Marshal(e)
	if _, err = tx.ExecContext(ctx, `UPDATE commands SET status=$2,started_at=COALESCE(started_at,$3),completed_at=$4,result_message=$5,updated_at=NOW() WHERE id=$1`, e.RequestID, commandStatus, e.StartedAt, e.FinishedAt, e.Error); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE av_scan_results SET status=$2,started_at=COALESCE(started_at,$3),finished_at=$4,threat_details=$5 WHERE command_id=$1 AND agent_id=$6`, e.RequestID, scanStatus, e.StartedAt, e.FinishedAt, string(raw), agent)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("expected one scan record")
	}
	// Final outcomes only; the complete report remains in av_scan_results.
	if e.Type == "virus_scan_result" {
		summary, _ := json.Marshal(map[string]any{"request_id": e.RequestID, "status": e.Status, "scan_type": e.ScanType})
		if _, err = tx.ExecContext(ctx, `INSERT INTO logs(user_id,action,target_agent_id,detail) VALUES($1,$2,$3,$4::jsonb || jsonb_build_object('job_id',$5::text))`, user, e.Type, agent, string(summary), jobID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) List(ctx context.Context, user string, req Request) ([]Record, error) {
	limit := req.Limit
	if limit == 0 {
		limit = 20
		if req.JobID != "" {
			limit = 100
		}
	}
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("limit must be between 1 and 100")
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT c.id::text,c.agent_id::text,j.scan_type,j.path,c.status,c.created_at,a.started_at,a.finished_at,a.threat_details,COALESCE(c.result_message,''),j.id::text FROM commands c JOIN av_scan_results a ON a.command_id=c.id AND a.agent_id=c.agent_id JOIN av_jobs j ON j.id=a.job_id WHERE j.requested_by=$1 AND ($2='' OR c.agent_id::text=$2) AND ($3='' OR c.id::text=$3) AND ($5='' OR j.id::text=$5) ORDER BY c.created_at DESC,c.id DESC LIMIT $4`, user, strings.ToLower(req.AgentID), strings.ToLower(req.RequestID), limit, strings.ToLower(req.JobID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []Record{}
	for rows.Next() {
		var rec Record
		var raw []byte
		if err = rows.Scan(&rec.RequestID, &rec.AgentID, &rec.ScanType, &rec.Path, &rec.Status, &rec.CreatedAt, &rec.StartedAt, &rec.FinishedAt, &raw, &rec.Message, &rec.JobID); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			if err = json.Unmarshal(raw, &rec.Result); err != nil {
				return nil, err
			}
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

package distribution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Repository struct{ DB *sql.DB }

func (r *Repository) Authenticate(ctx context.Context, tokenHash string) (string, bool, error) {
	return r.AuthenticatePermission(ctx, tokenHash, "files.distribute")
}

// AuthenticatePermission keeps the same session validity rules for every permission.
func (r *Repository) AuthenticatePermission(ctx context.Context, tokenHash, permission string) (string, bool, error) {
	var id string
	var allowed bool
	err := r.DB.QueryRowContext(ctx, `SELECT u.id::text, EXISTS(SELECT 1 FROM role_permissions rp JOIN permissions p ON p.id=rp.permission_id WHERE rp.role_id=u.role_id AND p.code=$2) FROM user_sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>NOW() AND u.status='ACTIVE'`, tokenHash, permission).Scan(&id, &allowed)
	return id, allowed, err
}

func (r *Repository) Create(ctx context.Context, user string, req CreateRequest) (Job, File, []Agent, bool, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, File{}, nil, false, err
	}
	defer tx.Rollback()
	if req.RequestID != "" {
		var j Job
		err = tx.QueryRowContext(ctx, `SELECT j.id::text,j.file_id::text,j.status,j.total_targets,count(*) FILTER(WHERE t.status<>'OFFLINE'),count(*) FILTER(WHERE t.status='OFFLINE') FROM file_distribution_jobs j LEFT JOIN file_distribution_targets t ON t.job_id=j.id WHERE j.requested_by=$1 AND j.request_id=$2 GROUP BY j.id`, user, req.RequestID).Scan(&j.ID, &j.FileID, &j.Status, &j.Total, &j.Online, &j.Offline)
		if err == nil {
			return j, File{}, nil, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return Job{}, File{}, nil, false, err
		}
	}
	var f File
	err = tx.QueryRowContext(ctx, `SELECT id::text,filename,file_size,storage_path,hash_sha256 FROM files WHERE id=$1`, req.FileID).Scan(&f.ID, &f.Filename, &f.Size, &f.StoragePath, &f.SHA256)
	if err != nil {
		return Job{}, File{}, nil, false, fmt.Errorf("file unavailable: %w", err)
	}
	var rows *sql.Rows
	if req.Target.Type == "ROOM" {
		var exists bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM rooms WHERE id=$1)`, req.Target.RoomID).Scan(&exists); err != nil || !exists {
			return Job{}, File{}, nil, false, fmt.Errorf("room not found")
		}
		rows, err = tx.QueryContext(ctx, `SELECT id::text,hostname FROM agents WHERE room_id=$1 AND status<>'DISABLED'`, req.Target.RoomID)
	} else {
		rows, err = tx.QueryContext(ctx, `SELECT id::text,hostname FROM agents WHERE id = ANY($1::uuid[]) AND status<>'DISABLED'`, "{"+strings.Join(req.Target.AgentIDs, ",")+"}")
	}
	if err != nil {
		return Job{}, File{}, nil, false, err
	}
	defer rows.Close()
	agents := []Agent{}
	for rows.Next() {
		var a Agent
		if err = rows.Scan(&a.ID, &a.Hostname); err != nil {
			return Job{}, File{}, nil, false, err
		}
		agents = append(agents, a)
	}
	if err = rows.Err(); err != nil {
		return Job{}, File{}, nil, false, err
	}
	if req.Target.Type == "AGENTS" && len(agents) != len(req.Target.AgentIDs) {
		return Job{}, File{}, nil, false, fmt.Errorf("one or more agents do not exist or are disabled")
	}
	var j Job
	room := any(nil)
	if req.Target.Type == "ROOM" {
		room = req.Target.RoomID
	}
	requestID := any(nil)
	if req.RequestID != "" {
		requestID = req.RequestID
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO file_distribution_jobs(file_id,requested_by,request_id,target_type,room_id,total_targets,status) VALUES($1,$2,$3,$4,$5,$6,'DISPATCHING') RETURNING id::text,status`, f.ID, user, requestID, req.Target.Type, room, len(agents)).Scan(&j.ID, &j.Status)
	if err != nil {
		return Job{}, File{}, nil, false, err
	}
	j.FileID = f.ID
	j.Total = len(agents)
	for _, a := range agents {
		if _, err = tx.ExecContext(ctx, `INSERT INTO file_distribution_targets(job_id,agent_id) VALUES($1,$2)`, j.ID, a.ID); err != nil {
			return Job{}, File{}, nil, false, err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO logs(user_id,action,detail) VALUES($1,'job_created',jsonb_build_object('job_id',$2::text,'file_id',$3::text,'targets',$4::int))`, user, j.ID, f.ID, len(agents))
	if err != nil {
		return Job{}, File{}, nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return Job{}, File{}, nil, false, err
	}
	return j, f, agents, false, nil
}

func (r *Repository) Grant(ctx context.Context, job, file, agent, hash string, expiry time.Time) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO file_download_grants(token_hash,job_id,file_id,agent_id,expires_at) VALUES($1,$2,$3,$4,$5)`, hash, job, file, agent, expiry)
	return err
}
func (r *Repository) MarkDispatch(ctx context.Context, job, agent, status string) error {
	_, err := r.DB.ExecContext(ctx, `UPDATE file_distribution_targets SET status=$3,updated_at=NOW() WHERE job_id=$1 AND agent_id=$2 AND status='PENDING'`, job, agent, status)
	return err
}
func (r *Repository) UpdateProgress(ctx context.Context, job, agent, status string, p int, b int64) (bool, error) {
	res, err := r.DB.ExecContext(ctx, `UPDATE file_distribution_targets SET status=$3,progress=$4,downloaded_bytes=$5,started_at=COALESCE(started_at,NOW()),updated_at=NOW() WHERE job_id=$1 AND agent_id=$2 AND status NOT IN ('COMPLETED','FAILED','CANCELLED') AND ($4>=progress+5 OR status<>$3)`, job, agent, status, p, b)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
func (r *Repository) Finish(ctx context.Context, job, agent, file, status, reportedHash, code, msg string, b int64) (string, error) {
	if status == "COMPLETED" {
		var expected string
		if err := r.DB.QueryRowContext(ctx, `SELECT hash_sha256 FROM files WHERE id=$1`, file).Scan(&expected); err != nil {
			return "", fmt.Errorf("load expected checksum for file %s: %w", file, err)
		}
		if !strings.EqualFold(expected, reportedHash) {
			status, code, msg = "FAILED", "HASH_MISMATCH", "reported checksum does not match file"
		}
	}
	res, err := r.DB.ExecContext(ctx, `WITH finished AS (UPDATE file_distribution_targets t SET status=$4::varchar,progress=CASE WHEN $4::text='COMPLETED' THEN 100 ELSE progress END,downloaded_bytes=$7,error_code=NULLIF($5::text,''),error_message=NULLIF($6::text,''),completed_at=NOW(),updated_at=NOW() FROM file_distribution_jobs j WHERE t.job_id=j.id AND j.id=$1 AND t.agent_id=$2 AND j.file_id=$3 AND t.status NOT IN ('COMPLETED','FAILED','CANCELLED') RETURNING j.requested_by,t.agent_id,t.job_id,t.status) INSERT INTO logs(user_id,action,target_agent_id,detail) SELECT requested_by,'file_download_result',agent_id,jsonb_build_object('job_id',job_id,'status',status,'file_id',$3::text) FROM finished`, job, agent, file, status, code, msg, b)
	if err != nil {
		return "", fmt.Errorf("update distribution target job=%s agent=%s file=%s: %w", job, agent, file, err)
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return "", fmt.Errorf("distribution target not found, already completed, or cancelled (job=%s agent=%s file=%s rows=%d)", job, agent, file, n)
	}
	if err := r.Recalculate(ctx, job); err != nil {
		return status, fmt.Errorf("target saved as %s but job recalculation failed: %w", status, err)
	}
	return status, nil
}
func (r *Repository) Recalculate(ctx context.Context, job string) error {
	_, err := r.DB.ExecContext(ctx, `WITH c AS (SELECT count(*) total,count(*) FILTER(WHERE status='COMPLETED') ok,count(*) FILTER(WHERE status IN ('FAILED','OFFLINE','CANCELLED')) bad,count(*) FILTER(WHERE status IN ('PENDING','SENT','DOWNLOADING','VERIFYING')) active FROM file_distribution_targets WHERE job_id=$1) UPDATE file_distribution_jobs j SET completed_targets=c.ok,failed_targets=c.bad,status=CASE WHEN c.active>0 THEN 'IN_PROGRESS' WHEN c.total>0 AND c.ok=c.total THEN 'COMPLETED' WHEN c.ok>0 THEN 'PARTIAL_FAILED' ELSE 'FAILED' END,completed_at=CASE WHEN c.active=0 THEN NOW() ELSE NULL END,updated_at=NOW() FROM c WHERE j.id=$1`, job)
	return err
}
func (r *Repository) ResolveGrant(ctx context.Context, hash, agent string) (File, error) {
	var f File
	err := r.DB.QueryRowContext(ctx, `SELECT f.id::text,f.filename,f.file_size,f.storage_path,f.hash_sha256 FROM file_download_grants g JOIN files f ON f.id=g.file_id JOIN file_distribution_targets t ON t.job_id=g.job_id AND t.agent_id=g.agent_id WHERE g.token_hash=$1 AND g.agent_id=$2 AND g.revoked_at IS NULL AND g.expires_at>NOW() AND t.status NOT IN ('CANCELLED','FAILED')`, hash, agent).Scan(&f.ID, &f.Filename, &f.Size, &f.StoragePath, &f.SHA256)
	return f, err
}

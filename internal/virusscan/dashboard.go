package virusscan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Summary struct {
	LoadedJobs int `json:"loaded_jobs"`
	Pending    int `json:"pending_results"`
	Succeeded  int `json:"succeeded_results"`
	Failed     int `json:"failed_results"`
}

type JobRecord struct {
	ID        string    `json:"job_id"`
	ScanType  string    `json:"scan_type"`
	Path      string    `json:"path,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Scans     []Record  `json:"scans"`
}

type Dashboard struct {
	Type    string      `json:"type"`
	Summary Summary     `json:"summary"`
	Jobs    []JobRecord `json:"jobs"`
}

func NewDashboard(jobs []JobRecord) Dashboard {
	if jobs == nil {
		jobs = []JobRecord{}
	}
	d := Dashboard{Type: "virus_scan_snapshot", Jobs: jobs, Summary: Summary{LoadedJobs: len(jobs)}}
	for _, job := range jobs {
		for _, scan := range job.Scans {
			switch scan.Status {
			case "SUCCEEDED":
				d.Summary.Succeeded++
			case "FAILED", "CANCELLED", "EXPIRED":
				d.Summary.Failed++
			default:
				d.Summary.Pending++
			}
		}
	}
	return d
}

// Snapshot limits jobs, not targets: every selected job includes all its machines.
func (r *Repository) Snapshot(ctx context.Context, user string, req Request) (Dashboard, error) {
	limit := req.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return Dashboard{}, fmt.Errorf("limit must be between 1 and 100")
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT j.id::text,j.scan_type,j.path,j.created_at,
	COALESCE((SELECT jsonb_agg(jsonb_build_object(
	'job_id',j.id,'request_id',c.id,'agent_id',c.agent_id,'scan_type',j.scan_type,'path',j.path,
	'status',c.status,'created_at',c.created_at,'started_at',a.started_at,'finished_at',a.finished_at,
	'result',a.threat_details,'message',COALESCE(c.result_message,'')) ORDER BY c.id)
	FROM av_scan_results a JOIN commands c ON c.id=a.command_id AND c.agent_id=a.agent_id
	WHERE a.job_id=j.id),'[]'::jsonb)
	FROM av_jobs j WHERE j.requested_by=$1 AND ($2='' OR j.id::text=$2)
	ORDER BY j.created_at DESC,j.id DESC LIMIT $3`, user, strings.ToLower(req.JobID), limit)
	if err != nil {
		return Dashboard{}, err
	}
	defer rows.Close()
	jobs := []JobRecord{}
	for rows.Next() {
		var job JobRecord
		var raw []byte
		if err := rows.Scan(&job.ID, &job.ScanType, &job.Path, &job.CreatedAt, &raw); err != nil {
			return Dashboard{}, err
		}
		if err := json.Unmarshal(raw, &job.Scans); err != nil {
			return Dashboard{}, err
		}
		jobs = append(jobs, job)
	}
	return NewDashboard(jobs), rows.Err()
}

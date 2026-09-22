package distribution

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"
	"ws-rat/internal/downloadtoken"
)

var uuidRE = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type Sender interface {
	Send(agentID string, value any) error
	Online(agentID string) bool
}
type Service struct {
	Repo    *Repository
	Sender  Sender
	BaseURL string
	TTL     time.Duration
	Logger  *log.Logger
}

func (s *Service) logf(format string, args ...any) {
	if s.Logger != nil {
		s.Logger.Printf(format, args...)
	}
}

func redactedDownloadURL(value string) string {
	u, err := url.Parse(value)
	if err != nil {
		return "<invalid-url>"
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 0 {
		parts[len(parts)-1] = "<token>"
		u.Path = "/" + strings.Join(parts, "/")
	}
	return u.String()
}

func Validate(r *CreateRequest) error {
	if !uuidRE.MatchString(r.FileID) {
		return fmt.Errorf("invalid file_id")
	}
	if r.RequestID != "" && !uuidRE.MatchString(r.RequestID) {
		return fmt.Errorf("invalid request_id")
	}
	r.Target.Type = strings.ToUpper(r.Target.Type)
	switch r.Target.Type {
	case "ROOM":
		if !uuidRE.MatchString(r.Target.RoomID) {
			return fmt.Errorf("invalid room_id")
		}
		r.Target.AgentIDs = nil
	case "AGENTS":
		seen := map[string]bool{}
		out := []string{}
		for _, id := range r.Target.AgentIDs {
			if !uuidRE.MatchString(id) {
				return fmt.Errorf("invalid agent_id")
			}
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
		if len(out) == 0 {
			return fmt.Errorf("agent_ids is required")
		}
		r.Target.AgentIDs = out
		r.Target.RoomID = ""
	default:
		return fmt.Errorf("target.type must be ROOM or AGENTS")
	}
	return nil
}
func (s *Service) Create(ctx context.Context, user string, r CreateRequest) (Job, error) {
	s.logf("distribution create start user=%q file_id=%q request_id=%q target_type=%q room_id=%q agent_count=%d base_url=%q ttl=%s",
		user, r.FileID, r.RequestID, r.Target.Type, r.Target.RoomID, len(r.Target.AgentIDs), strings.TrimRight(s.BaseURL, "/"), s.TTL)
	if err := Validate(&r); err != nil {
		s.logf("distribution create rejected reason=validation_error err=%q", err)
		return Job{}, err
	}
	j, f, agents, duplicate, err := s.Repo.Create(ctx, user, r)
	s.logf("distribution repository create job_id=%q file_id=%q filename=%q storage_path=%q size=%d target_count=%d duplicate=%t err=%v",
		j.ID, f.ID, f.Filename, f.StoragePath, f.Size, len(agents), duplicate, err)
	if err != nil || duplicate {
		if duplicate {
			s.logf("distribution create reused existing job job_id=%q status=%q", j.ID, j.Status)
		}
		return j, err
	}
	for _, a := range agents {
		s.logf("distribution target start job_id=%q agent_id=%q hostname=%q online=%t", j.ID, a.ID, a.Hostname, s.Sender.Online(a.ID))
		if !s.Sender.Online(a.ID) {
			j.Offline++
			if markErr := s.Repo.MarkDispatch(ctx, j.ID, a.ID, "OFFLINE"); markErr != nil {
				s.logf("distribution target mark_failed job_id=%q agent_id=%q status=OFFLINE err=%q", j.ID, a.ID, markErr)
			}
			s.logf("distribution target skipped job_id=%q agent_id=%q reason=agent_offline", j.ID, a.ID)
			continue
		}
		raw, hash, err := downloadtoken.New()
		if err != nil {
			s.logf("distribution target token_failed job_id=%q agent_id=%q err=%q", j.ID, a.ID, err)
			_ = s.Repo.MarkDispatch(ctx, j.ID, a.ID, "FAILED")
			continue
		}
		expiry := time.Now().UTC().Add(s.TTL)
		s.logf("distribution target token_created job_id=%q agent_id=%q token_length=%d expires_at=%s hash_prefix=%s",
			j.ID, a.ID, len(raw), expiry.Format(time.RFC3339), hash[:12])
		if err = s.Repo.Grant(ctx, j.ID, f.ID, a.ID, hash, expiry); err != nil {
			s.logf("distribution target grant_failed job_id=%q agent_id=%q err=%q", j.ID, a.ID, err)
			_ = s.Repo.MarkDispatch(ctx, j.ID, a.ID, "FAILED")
			continue
		}
		cmd := DownloadCommand{Type: "DOWNLOAD_FILE", JobID: j.ID, FileID: f.ID, Filename: f.Filename, Size: f.Size, SHA256: f.SHA256, DownloadURL: strings.TrimRight(s.BaseURL, "/") + "/files/download/" + url.PathEscape(raw), ExpiresAt: expiry}
		s.logf("distribution target command_ready job_id=%q agent_id=%q file_id=%q filename=%q size=%d url=%q expires_at=%s",
			j.ID, a.ID, f.ID, f.Filename, f.Size, redactedDownloadURL(cmd.DownloadURL), expiry.Format(time.RFC3339))
		if err = s.Sender.Send(a.ID, cmd); err != nil {
			s.logf("distribution target send_failed job_id=%q agent_id=%q err=%q", j.ID, a.ID, err)
			if markErr := s.Repo.MarkDispatch(ctx, j.ID, a.ID, "OFFLINE"); markErr != nil {
				s.logf("distribution target mark_failed job_id=%q agent_id=%q status=OFFLINE err=%q", j.ID, a.ID, markErr)
			}
			j.Offline++
			continue
		}
		if markErr := s.Repo.MarkDispatch(ctx, j.ID, a.ID, "SENT"); markErr != nil {
			s.logf("distribution target mark_failed job_id=%q agent_id=%q status=SENT err=%q", j.ID, a.ID, markErr)
		}
		s.logf("distribution target command_sent job_id=%q agent_id=%q", j.ID, a.ID)
		j.Online++
	}
	if err := s.Repo.Recalculate(ctx, j.ID); err != nil {
		s.logf("distribution recalculate_failed job_id=%q err=%q", j.ID, err)
	} else {
		s.logf("distribution recalculate_done job_id=%q online=%d offline=%d", j.ID, j.Online, j.Offline)
	}
	if j.Online > 0 {
		j.Status = "IN_PROGRESS"
	} else {
		j.Status = "FAILED"
	}
	s.logf("distribution create complete job_id=%q file_id=%q status=%q online=%d offline=%d total=%d", j.ID, j.FileID, j.Status, j.Online, j.Offline, j.Total)
	return j, nil
}

package distribution

import (
	"context"
	"fmt"
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
	if err := Validate(&r); err != nil {
		return Job{}, err
	}
	j, f, agents, duplicate, err := s.Repo.Create(ctx, user, r)
	if err != nil || duplicate {
		return j, err
	}
	for _, a := range agents {
		if !s.Sender.Online(a.ID) {
			j.Offline++
			_ = s.Repo.MarkDispatch(ctx, j.ID, a.ID, "OFFLINE")
			continue
		}
		raw, hash, err := downloadtoken.New()
		if err != nil {
			_ = s.Repo.MarkDispatch(ctx, j.ID, a.ID, "FAILED")
			continue
		}
		expiry := time.Now().UTC().Add(s.TTL)
		if err = s.Repo.Grant(ctx, j.ID, f.ID, a.ID, hash, expiry); err != nil {
			_ = s.Repo.MarkDispatch(ctx, j.ID, a.ID, "FAILED")
			continue
		}
		cmd := DownloadCommand{Type: "DOWNLOAD_FILE", JobID: j.ID, FileID: f.ID, Filename: f.Filename, Size: f.Size, SHA256: f.SHA256, DownloadURL: strings.TrimRight(s.BaseURL, "/") + "/files/download/" + url.PathEscape(raw), ExpiresAt: expiry}
		if err = s.Sender.Send(a.ID, cmd); err != nil {
			_ = s.Repo.MarkDispatch(ctx, j.ID, a.ID, "OFFLINE")
			j.Offline++
			continue
		}
		_ = s.Repo.MarkDispatch(ctx, j.ID, a.ID, "SENT")
		j.Online++
	}
	_ = s.Repo.Recalculate(ctx, j.ID)
	if j.Online > 0 {
		j.Status = "IN_PROGRESS"
	} else {
		j.Status = "FAILED"
	}
	return j, nil
}

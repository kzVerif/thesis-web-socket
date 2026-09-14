package wsserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
	"ws-rat/internal/distribution"
)

func (s *Server) Online(id string) bool { _, ok := s.registry.Get(id); return ok }
func (s *Server) Send(id string, v any) error {
	c, ok := s.registry.Get(id)
	if !ok {
		return fmt.Errorf("agent offline")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.WriteJSON(ctx, v)
}
func hashToken(v string) string { x := sha256.Sum256([]byte(v)); return hex.EncodeToString(x[:]) }
func (s *Server) handleDistributionFrontend(f *FrontendClient, user string, raw json.RawMessage) error {
	var req distribution.CreateRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return fmt.Errorf("invalid request")
	}
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	j, err := s.distributions.Create(ctx, user, req)
	if err != nil {
		return err
	}
	return f.WriteJSON(map[string]any{"type": "FILE_DISTRIBUTION_CREATED", "job_id": j.ID, "file_id": j.FileID, "total_targets": j.Total, "online_targets": j.Online, "offline_targets": j.Offline, "status": j.Status})
}
func (s *Server) handleAgentDistribution(client *Client, raw json.RawMessage, kind string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	if kind == "FILE_DOWNLOAD_PROGRESS" {
		var p distribution.Progress
		if json.Unmarshal(raw, &p) != nil || p.AgentID != client.Info.ID || p.Progress < 0 || p.Progress > 100 || p.DownloadedBytes < 0 || p.TotalBytes < 0 {
			return true
		}
		_, err := s.distributionRepo.UpdateProgress(ctx, p.JobID, client.Info.ID, "DOWNLOADING", p.Progress, p.DownloadedBytes)
		if err == nil {
			s.subscriptions.PublishDistribution(distribution.Update{Type: "FILE_DISTRIBUTION_TARGET_UPDATE", JobID: p.JobID, AgentID: client.Info.ID, Hostname: client.Info.Hostname, Status: "DOWNLOADING", Progress: p.Progress, DownloadedBytes: p.DownloadedBytes, TotalBytes: p.TotalBytes})
		}
		return true
	}
	if kind == "FILE_DOWNLOAD_RESULT" {
		var r distribution.Result
		if err := json.Unmarshal(raw, &r); err != nil {
			s.logger.Printf("invalid FILE_DOWNLOAD_RESULT from agent %s: decode JSON: %v raw=%s", client.Info.ID, err, raw)
			return true
		}
		if r.AgentID != client.Info.ID {
			s.logger.Printf("rejected FILE_DOWNLOAD_RESULT: connected agent=%s payload agent_id=%s job=%s", client.Info.ID, r.AgentID, r.JobID)
			return true
		}
		if r.Status != "COMPLETED" && r.Status != "FAILED" {
			s.logger.Printf("rejected FILE_DOWNLOAD_RESULT from agent %s: status=%q; expected COMPLETED or FAILED", client.Info.ID, r.Status)
			return true
		}
		persistedStatus, err := s.distributionRepo.Finish(ctx, r.JobID, client.Info.ID, r.FileID, r.Status, r.SHA256, r.ErrorCode, r.ErrorMessage, r.BytesDownloaded)
		if err != nil {
			s.logger.Printf("cannot finish file distribution for agent %s job=%s file=%s: %v", client.Info.ID, r.JobID, r.FileID, err)
			return true
		}
		if persistedStatus != r.Status {
			s.logger.Printf("file distribution result changed from %s to %s for agent %s job=%s (reported checksum did not match)", r.Status, persistedStatus, client.Info.ID, r.JobID)
		}
		s.logger.Printf("file distribution target finished: agent=%s job=%s file=%s status=%s", client.Info.ID, r.JobID, r.FileID, persistedStatus)
		s.subscriptions.PublishDistribution(distribution.Update{Type: "FILE_DISTRIBUTION_TARGET_UPDATE", JobID: r.JobID, AgentID: client.Info.ID, Hostname: client.Info.Hostname, Status: persistedStatus, Progress: map[bool]int{true: 100}[persistedStatus == "COMPLETED"], DownloadedBytes: r.BytesDownloaded})
		return true
	}
	return false
}

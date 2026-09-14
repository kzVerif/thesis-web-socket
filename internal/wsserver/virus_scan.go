package wsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"ws-rat/internal/virusscan"
)

type VirusScanStore interface {
	Allowed(context.Context, string) (bool, error)
	Create(context.Context, string, virusscan.Request) (virusscan.Job, error)
	Dispatched(context.Context, string, error) error
	Apply(context.Context, string, virusscan.Event) error
	List(context.Context, string, virusscan.Request) ([]virusscan.Record, error)
}

func (s *Server) ConfigureVirusScan(repo VirusScanStore) { s.virusScans = repo }

func (s *Server) handleVirusScanFrontend(f *FrontendClient, user string, raw json.RawMessage) error {
	if s.virusScans == nil || user == "" {
		return fmt.Errorf("virus scan unavailable")
	}
	var req virusscan.Request
	if json.Unmarshal(raw, &req) != nil {
		return fmt.Errorf("invalid request")
	}
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	allowed, err := s.virusScans.Allowed(ctx, user)
	if err != nil {
		s.logger.Printf("scan authorization: %v", err)
		return fmt.Errorf("cannot authorize scan")
	}
	if !allowed {
		return fmt.Errorf("ไม่มี permission")
	}
	if req.Type == "virus_scan_list" {
		if req.AgentID != "" && !virusscan.ValidID(req.AgentID) {
			return fmt.Errorf("invalid agent_id")
		}
		if req.JobID != "" && !virusscan.ValidID(req.JobID) {
			return fmt.Errorf("invalid job_id")
		}
		if len(req.AgentIDs) > 0 {
			return fmt.Errorf("agent_ids is only supported when creating a job")
		}
		if req.RequestID != "" && !virusscan.ValidID(req.RequestID) {
			return fmt.Errorf("invalid request_id")
		}
		if req.Limit < 0 || req.Limit > 100 {
			return fmt.Errorf("limit must be between 1 and 100")
		}
		records, err := s.virusScans.List(ctx, user, req)
		if err != nil {
			s.logger.Printf("list scans: %v", err)
			return fmt.Errorf("cannot load scans")
		}
		return f.WriteJSON(map[string]any{"type": "virus_scan_list", "job_id": req.JobID, "agent_id": req.AgentID, "request_id": req.RequestID, "scans": records})
	}
	if err = req.Validate(); err != nil {
		return err
	}
	ids, _ := req.TargetIDs()
	for _, id := range ids {
		if !s.Online(id) {
			return fmt.Errorf("agent is offline")
		}
	}
	job, err := s.virusScans.Create(ctx, user, req)
	if err != nil {
		s.logger.Printf("create scan: %v", err)
		return fmt.Errorf("cannot create scan")
	}
	// All targets are committed before any agent receives a command.
	var wg sync.WaitGroup
	for i := range job.Targets {
		wg.Add(1)
		go func(target *virusscan.Target) {
			defer wg.Done()
			command := map[string]any{"type": "virus_scan", "request_id": target.RequestID, "scan_type": req.ScanType}
			if req.Path != "" {
				command["path"] = req.Path
			}
			sendErr := s.Send(target.AgentID, command)
			persistCtx, persistCancel := context.WithTimeout(context.Background(), databaseTimeout)
			defer persistCancel()
			if err := s.virusScans.Dispatched(persistCtx, target.RequestID, sendErr); err != nil {
				s.logger.Printf("mark scan dispatched %s: %v", target.RequestID, err)
			}
			target.Dispatch = "sent"
			if sendErr != nil {
				target.Dispatch = "uncertain"
			}
		}(&job.Targets[i])
	}
	wg.Wait()
	response := map[string]any{"type": "virus_scan_accepted", "job_id": job.ID, "scan_type": req.ScanType, "targets": job.Targets, "total_targets": len(job.Targets)}
	// Keep the original acknowledgement fields for single-agent callers.
	if len(job.Targets) == 1 {
		target := job.Targets[0]
		response["request_id"] = target.RequestID
		response["agent_id"] = target.AgentID
		response["dispatch"] = target.Dispatch
	}
	return f.WriteJSON(response)
}

func (s *Server) handleAgentVirusScan(client *Client, raw json.RawMessage, kind string) bool {
	if kind != "virus_scan_status" && kind != "virus_scan_result" {
		return false
	}
	if s.virusScans == nil {
		return true
	}
	var e virusscan.Event
	if err := json.Unmarshal(raw, &e); err != nil {
		s.logger.Printf("invalid scan JSON from %s: %v", client.Info.ID, err)
		return true
	}
	if err := e.Validate(); err != nil {
		s.logger.Printf("invalid scan event from %s: %v", client.Info.ID, err)
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	if err := s.virusScans.Apply(ctx, client.Info.ID, e); err != nil {
		s.logger.Printf("persist scan %s from %s: %v", e.RequestID, client.Info.ID, err)
	}
	return true
}

package distribution

import (
	"encoding/json"
	"time"
)

type File struct {
	ID, Filename, StoragePath, SHA256 string
	Size                              int64
}
type Agent struct{ ID, Hostname string }
type CreateRequest struct {
	DestinationPath string `json:"destination_path,omitempty"`
	RequestID       string `json:"request_id"`
	FileID          string `json:"file_id"`
	Target          Target `json:"target"`
}
type Target struct {
	Type     string   `json:"type"`
	RoomID   string   `json:"room_id"`
	AgentIDs []string `json:"agent_ids"`
}
type Job struct {
	ID, FileID, Status     string
	Total, Online, Offline int
}
type DownloadCommand struct {
	DestinationPath string    `json:"destination_path,omitempty"`
	Type            string    `json:"type"`
	JobID           string    `json:"job_id"`
	FileID          string    `json:"file_id"`
	Filename        string    `json:"filename"`
	Size            int64     `json:"size"`
	SHA256          string    `json:"sha256"`
	DownloadURL     string    `json:"download_url"`
	ExpiresAt       time.Time `json:"expires_at"`
}
type Progress struct {
	Type            string `json:"type"`
	JobID           string `json:"job_id"`
	AgentID         string `json:"agent_id"`
	DownloadedBytes int64  `json:"downloaded_bytes"`
	TotalBytes      int64  `json:"total_bytes"`
	Progress        int    `json:"progress"`
}
type Result struct {
	Type            string `json:"type"`
	JobID           string `json:"job_id"`
	AgentID         string `json:"agent_id"`
	FileID          string `json:"file_id"`
	Status          string `json:"status"`
	SHA256          string `json:"sha256"`
	ErrorCode       string `json:"error_code"`
	ErrorMessage    string `json:"error_message"`
	BytesDownloaded int64  `json:"bytes_downloaded"`
}

// UnmarshalJSON accepts agent error aliases while keeping the canonical fields
// used by persistence and outgoing JSON.
func (r *Result) UnmarshalJSON(data []byte) error {
	type result Result
	var payload struct {
		result
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	if payload.ErrorCode == "" {
		payload.ErrorCode = payload.Code
	}
	if payload.ErrorMessage == "" {
		payload.ErrorMessage = payload.Error
	}
	*r = Result(payload.result)
	return nil
}

type Update struct {
	Type            string `json:"type"`
	JobID           string `json:"job_id"`
	AgentID         string `json:"agent_id"`
	Hostname        string `json:"hostname,omitempty"`
	Status          string `json:"status"`
	Progress        int    `json:"progress"`
	DownloadedBytes int64  `json:"downloaded_bytes"`
	TotalBytes      int64  `json:"total_bytes"`
}

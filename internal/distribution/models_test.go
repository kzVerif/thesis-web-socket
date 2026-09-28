package distribution

import (
	"encoding/json"
	"testing"
)

func TestResultErrorFields(t *testing.T) {
	const message = "destination_path must be an allowed absolute local directory"
	for _, tc := range []struct {
		name, fields, code, message string
	}{
		{"agent aliases", `"code":"INVALID_DESTINATION_PATH","error":"` + message + `"`, "INVALID_DESTINATION_PATH", message},
		{"canonical fields", `"error_code":"WRITE_FAILED","error_message":"cannot write"`, "WRITE_FAILED", "cannot write"},
		{"canonical precedence", `"error_code":"WRITE_FAILED","error_message":"cannot write","code":"OTHER","error":"other"`, "WRITE_FAILED", "cannot write"},
		{"independent fallback", `"error_code":"WRITE_FAILED","error":"cannot write"`, "WRITE_FAILED", "cannot write"},
		{"empty canonical fallback", `"error_code":"","error_message":"","code":"WRITE_FAILED","error":"cannot write"`, "WRITE_FAILED", "cannot write"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var result Result
			data := `{"type":"FILE_DOWNLOAD_RESULT","job_id":"job","agent_id":"agent","file_id":"file","status":"FAILED","bytes_downloaded":12,` + tc.fields + `}`
			if err := json.Unmarshal([]byte(data), &result); err != nil {
				t.Fatal(err)
			}
			if result.ErrorCode != tc.code || result.ErrorMessage != tc.message {
				t.Fatalf("error fields = (%q, %q), want (%q, %q)", result.ErrorCode, result.ErrorMessage, tc.code, tc.message)
			}
			if result.Type != "FILE_DOWNLOAD_RESULT" || result.JobID != "job" || result.AgentID != "agent" || result.FileID != "file" || result.Status != "FAILED" || result.BytesDownloaded != 12 {
				t.Fatalf("result fields lost: %+v", result)
			}
			// Reusing a result must not retain errors from a previous message.
			if err := json.Unmarshal([]byte(`{"status":"COMPLETED","sha256":"checksum"}`), &result); err != nil {
				t.Fatal(err)
			}
			if result.ErrorCode != "" || result.ErrorMessage != "" || result.SHA256 != "checksum" {
				t.Fatalf("unexpected completed result: %+v", result)
			}
		})
	}
}

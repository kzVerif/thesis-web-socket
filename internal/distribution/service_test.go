package distribution

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateDestinationPath(t *testing.T) {
	for _, tc := range []struct {
		path  string
		valid bool
	}{
		{"", true},
		{`C:\Downloads`, true},
		{`D:\ไฟล์งาน\บทเรียน`, true},
		{`C:/Shared Files/`, true},
		{`C:\`, true},
		{"/", true},
		{"/srv/shared files", true},
		{"relative/folder", false},
		{"   ", false},
		{`C:Downloads`, false},
		{`\Downloads`, false},
		{`\\server\share`, false},
		{`\\?\C:\Downloads`, false},
		{"//server/share", false},
		{`C:\Downloads\..\Windows`, false},
		{"/srv/../etc", false},
		{"/srv/./files", false},
		{`C:\Downloads\.. \Windows`, false},
		{`C:\Downloads:stream`, false},
		{`C:\Down*loads`, false},
		{"/srv/files\x00", false},
		{"/srv/files\n", false},
		{"/" + strings.Repeat("a", 4096), false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := CreateRequest{FileID: "11111111-1111-4111-8111-111111111111", Target: Target{Type: "ROOM", RoomID: "22222222-2222-4222-8222-222222222222"}, DestinationPath: tc.path}
			err := Validate(&r)
			if (err == nil) != tc.valid {
				t.Fatalf("Validate() = %v, want valid=%t", err, tc.valid)
			}
			if r.DestinationPath != tc.path {
				t.Fatal("destination path was changed")
			}
		})
	}
}

func TestDestinationPathProtocol(t *testing.T) {
	var req CreateRequest
	if err := json.Unmarshal([]byte(`{"destination_path":"D:\\Shared Files"}`), &req); err != nil {
		t.Fatal(err)
	}
	if req.DestinationPath != `D:\Shared Files` {
		t.Fatalf("decoded path = %q", req.DestinationPath)
	}
	for _, path := range []string{"", req.DestinationPath} {
		data, err := json.Marshal(DownloadCommand{Type: "DOWNLOAD_FILE", DestinationPath: path})
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatal(err)
		}
		value, present := fields["destination_path"]
		if path == "" && present {
			t.Fatal("legacy commands must omit destination_path")
		}
		if path != "" && value != path {
			t.Fatalf("command path = %v, want %q", value, path)
		}
	}
}

func TestValidateDeduplicatesAgents(t *testing.T) {
	r := CreateRequest{FileID: "11111111-1111-4111-8111-111111111111", Target: Target{Type: "AGENTS", AgentIDs: []string{"22222222-2222-4222-8222-222222222222", "22222222-2222-4222-8222-222222222222"}}}
	if err := Validate(&r); err != nil {
		t.Fatal(err)
	}
	if len(r.Target.AgentIDs) != 1 {
		t.Fatalf("got %d", len(r.Target.AgentIDs))
	}
}
func TestValidateRejectsInvalidUUID(t *testing.T) {
	r := CreateRequest{FileID: "bad", Target: Target{Type: "ROOM", RoomID: "bad"}}
	if Validate(&r) == nil {
		t.Fatal("expected error")
	}
}
